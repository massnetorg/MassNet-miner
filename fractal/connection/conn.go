package connection

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/massnetorg/mass-core/logging"
)

var (
	ErrKeepaliveTimeout = errors.New("keepalive timeout")
	ErrRecvMsgTooLarge  = errors.New("recv msg too large")
)

type Conn struct {
	ctx            context.Context
	ctxCanceller   context.CancelFunc
	wg             sync.WaitGroup
	stopping       int32 // atomic
	stopped        int32 // atomic
	c              net.Conn
	opts           *options
	recvCh         chan []byte
	sendCh         chan []byte
	prioritySendCh chan []byte
	alivenessCh    chan struct{}
}

func NewConn(opt ...Option) (*Conn, context.CancelFunc, error) {
	opts := defaultOptions()
	for _, o := range opt {
		o.apply(opts)
	}
	c, err := makeNetConn(opts)
	if err != nil {
		return nil, nil, err
	}
	conn, closer := newConn(c, opts)
	return conn, closer, nil
}

func (conn *Conn) Stopped() bool {
	return atomic.LoadInt32(&conn.stopped) == 1
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.c.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.c.LocalAddr()
}

func (conn *Conn) Read(ctx context.Context) ([]byte, error) {
	return safeReadBytes(ctx, conn.recvCh)
}

func (conn *Conn) Send(ctx context.Context, data []byte) error {
	return conn.checkSend(ctx, conn.sendCh, data)
}

func (conn *Conn) SendPriority(ctx context.Context, data []byte) error {
	return conn.checkSend(ctx, conn.prioritySendCh, data)
}

func makeNetConn(opts *options) (net.Conn, error) {
	if opts.conn != nil {
		return opts.conn, nil
	}
	var ctx = opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	d := net.Dialer{Timeout: opts.dialTimeout}
	return d.DialContext(ctx, opts.dialNetwork, opts.dialAddress)
}

func newConn(c net.Conn, opts *options) (*Conn, context.CancelFunc) {
	var ctx = opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cCtx, cancel := context.WithCancel(ctx)
	conn := &Conn{
		ctx:            cCtx,
		ctxCanceller:   cancel,
		c:              c,
		opts:           opts,
		recvCh:         make(chan []byte, opts.recvQueueSize),
		sendCh:         make(chan []byte, opts.sendQueueSize),
		prioritySendCh: make(chan []byte, opts.prioritySendQueueSize),
		alivenessCh:    make(chan struct{}, opts.alivenessChannelSize),
	}
	return conn, conn.run()
}

func (conn *Conn) checkSend(ctx context.Context, ch chan []byte, data []byte) error {
	if maxSize := conn.opts.maxSendMsgSize; maxSize < uint32(len(data)) {
		return fmt.Errorf("msg oversize, max_size = %d, actual_size = %d", maxSize, len(data))
	}
	var copyData = data
	if conn.opts.copyMsg {
		copyData = make([]byte, len(data))
		copy(copyData, data)
	}
	return safeSendBytes(ctx, ch, copyData)
}

func (conn *Conn) waitStop(err error) {
	if atomic.LoadInt32(&conn.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&conn.stopping, 0, 1) {
		conn.wg.Wait()
		return
	}
	if err != nil {
		logging.CPrint(logging.WARN, "fractal connection closed", logging.LogFormat{
			"err":         err,
			"local_addr":  conn.c.LocalAddr(),
			"remote_addr": conn.c.RemoteAddr(),
		})
	}
	conn.ctxCanceller()
	conn.c.Close()
	conn.wg.Wait()
	atomic.StoreInt32(&conn.stopped, 1)
}

func (conn *Conn) run() context.CancelFunc {
	conn.wg.Add(4)
	go conn.receiveRoutine()
	go conn.sendRoutine()
	go conn.keepaliveRoutine()
	go conn.alivenessMonitor()

	return func() {
		conn.waitStop(nil)
	}
}

func (conn *Conn) keepaliveRoutine() {
	defer conn.wg.Done()

	var interval = conn.opts.keepaliveInterval
	if interval <= 0 {
		return
	}
	var ticker = time.NewTicker(interval)
	defer ticker.Stop()
	var doneCh = conn.ctx.Done()

routine:
	for {
		select {
		case <-doneCh:
			break routine
		case <-ticker.C:
			conn.sendKeepalivePacket()
		}
	}
}

func (conn *Conn) alivenessMonitor() {
	defer conn.wg.Done()
	defer close(conn.alivenessCh)

	var timeout = conn.opts.keepaliveTimeout
	if timeout <= 0 {
		return
	}
	var timer = time.NewTimer(timeout)
	var doneCh = conn.ctx.Done()

routine:
	for {
		select {
		case <-doneCh:
			break routine
		case <-timer.C:
			go conn.waitStop(ErrKeepaliveTimeout)
			break routine
		case <-conn.alivenessCh:
		}
		timer.Reset(timeout)
	}
}

func (conn *Conn) receiveRoutine() {
	defer conn.wg.Done()
	defer close(conn.recvCh)

	var msgSizeBytes [4]byte
	var err error
routine:
	for {
		if err = conn.readNetConn(msgSizeBytes[:]); err != nil {
			go conn.waitStop(err)
			break routine
		}
		size := bytesToMsgSize(msgSizeBytes[:])
		if size == 0 {
			conn.handleCtrlMsg()
			continue routine
		} else if size > conn.opts.maxRecvMsgSize {
			go conn.waitStop(ErrRecvMsgTooLarge)
			break routine
		}
		data := make([]byte, size)
		if err = conn.readNetConn(data[:]); err != nil {
			go conn.waitStop(err)
			break routine
		}
		conn.maintainKeepalive()
		conn.recvCh <- data
	}
}

func (conn *Conn) sendRoutine() {
	defer conn.wg.Done()
	defer close(conn.sendCh)
	defer close(conn.prioritySendCh)

	var msgSizeBytes [4]byte
	var data []byte
	var err error
	var doneCh = conn.ctx.Done()
routine:
	for {
		select {
		case data = <-conn.prioritySendCh:
		case <-doneCh:
			break routine
		default:
			select {
			case data = <-conn.prioritySendCh:
			case data = <-conn.sendCh:
			case <-doneCh:
				break routine
			}
		}
		size := uint32(len(data))
		msgSizeToBytes(size, msgSizeBytes[:])
		if err = conn.writeNetConn(msgSizeBytes[:]); err != nil {
			go conn.waitStop(err)
			break routine
		}
		if size == 0 {
			// ctrl msg
			continue routine
		}
		if err = conn.writeNetConn(data); err != nil {
			go conn.waitStop(err)
			break routine
		}
	}
}

func (conn *Conn) handleCtrlMsg() {
	// supports only ping-pong msg for now
	conn.maintainKeepalive()
	// response keepalive when keepalive is not auto outgoing
	if conn.opts.keepaliveInterval <= 0 {
		conn.sendKeepalivePacket()
	}
}

func (conn *Conn) maintainKeepalive() {
	if conn.opts.keepaliveTimeout <= 0 {
		return
	} else {
		safeSendStruct(conn.alivenessCh)
	}
}

func (conn *Conn) sendKeepalivePacket() {
	if err := safeSendBytes(conn.ctx, conn.prioritySendCh, nil); err != nil {
		go conn.waitStop(err)
	}
}

func (conn *Conn) readNetConn(data []byte) error {
	_, err := io.ReadFull(conn.c, data)
	return err
}

func (conn *Conn) writeNetConn(data []byte) error {
	_, err := conn.c.Write(data)
	return err
}

func bytesToMsgSize(data []byte) uint32 {
	return binary.BigEndian.Uint32(data)
}

func msgSizeToBytes(size uint32, data []byte) {
	binary.BigEndian.PutUint32(data, size)
}

func safeSendBytes(ctx context.Context, ch chan []byte, value []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = io.ErrClosedPipe
		}
	}()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case ch <- value: // panic if ch is closed
	}
	return err
}

func safeReadBytes(ctx context.Context, ch chan []byte) (data []byte, err error) {
	var ok bool
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case data, ok = <-ch:
		if !ok {
			err = io.EOF
		}
	}
	return
}

func safeSendStruct(ch chan struct{}) (closed bool) {
	defer func() {
		if recover() != nil {
			closed = true
		}
	}()

	ch <- struct{}{} // panic if ch is closed
	return false     // <=> closed = false; return
}

func safeClose(ch chan []byte) (justClosed bool) {
	defer func() {
		if recover() != nil {
			// The return result can be altered
			// in a defer function call.
			justClosed = false
		}
	}()

	// assume ch != nil here.
	close(ch)   // panic if ch is closed
	return true // <=> justClosed = true; return
}
