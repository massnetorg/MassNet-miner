package fractal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil/ccache"
	"github.com/panjf2000/ants/v2"
	"massnet.org/mass/fractal/connection"
	"massnet.org/mass/fractal/protocol"
)

type Superior interface {
	ReportQualities(context.Context, uuid.UUID, *protocol.ReportQualities) error
	ReportProof(context.Context, uuid.UUID, *protocol.ReportProof) error
	ReportSignature(context.Context, uuid.UUID, *protocol.ReportSignature) error
	Subscribe(context.Context, Collector)
	Unsubscribe(context.Context, Collector)
}

type superiorMsgHandler func(context.Context, uuid.UUID, protocol.Message) error

type baseSuperior struct {
	l          sync.RWMutex
	pool       *ants.Pool
	collectors map[uuid.UUID]Collector
	handlers   map[protocol.MsgType]superiorMsgHandler
}

func newBaseSuperior(handlers map[protocol.MsgType]superiorMsgHandler) *baseSuperior {
	pool, _ := ants.NewPool(128)
	return &baseSuperior{
		pool:       pool,
		collectors: make(map[uuid.UUID]Collector),
		handlers:   handlers,
	}
}

func (base *baseSuperior) Release() {
	base.pool.Release()
}

func (base *baseSuperior) ReportQualities(ctx context.Context, cid uuid.UUID, resp *protocol.ReportQualities) error {
	return base.onReport(ctx, cid, resp)
}

func (base *baseSuperior) ReportProof(ctx context.Context, cid uuid.UUID, resp *protocol.ReportProof) error {
	return base.onReport(ctx, cid, resp)
}

func (base *baseSuperior) ReportSignature(ctx context.Context, cid uuid.UUID, resp *protocol.ReportSignature) error {
	return base.onReport(ctx, cid, resp)
}

func (base *baseSuperior) Subscribe(ctx context.Context, c Collector) {
	base.l.Lock()
	defer base.l.Unlock()

	base.collectors[c.ID()] = c
}

func (base *baseSuperior) Unsubscribe(ctx context.Context, c Collector) {
	base.l.Lock()
	defer base.l.Unlock()

	delete(base.collectors, c.ID())
}

func (base *baseSuperior) Send(ctx context.Context, cid uuid.UUID, msg protocol.Message) {
	base.l.RLock()
	defer base.l.RUnlock()

	c, ok := base.collectors[cid]
	if !ok {
		return
	}
	if err := base.sendRequest(ctx, c, msg); err != nil {
		logging.CPrint(logging.WARN, "fail to request collector", logging.LogFormat{
			"err":          err,
			"collector_id": cid,
		})
	}
}

func (base *baseSuperior) Broadcast(ctx context.Context, msg protocol.Message) {
	base.l.RLock()
	defer base.l.RUnlock()

	for id, c := range base.collectors {
		id0 := id
		c0 := c
		base.pool.Submit(func() {
			if err := base.sendRequest(ctx, c0, msg); err != nil {
				logging.CPrint(logging.WARN, "fail to broadcast collector", logging.LogFormat{
					"err":          err,
					"collector_id": id0,
				})
			}
		})
	}
}

func (base *baseSuperior) onReport(ctx context.Context, cid uuid.UUID, msg protocol.Message) error {
	handler, ok := base.handlers[msg.MsgType()]
	if !ok {
		return fmt.Errorf("missing handler for msg type: %v", msg.MsgType())
	}
	return handler(ctx, cid, msg)
}

func (base *baseSuperior) sendRequest(ctx context.Context, c Collector, msg protocol.Message) error {
	switch msg.MsgType() {
	case protocol.MsgTypeRequestQualities:
		return c.RequestQualities(ctx, msg.(*protocol.RequestQualities))
	case protocol.MsgTypeRequestProof:
		return c.RequestProof(ctx, msg.(*protocol.RequestProof))
	case protocol.MsgTypeRequestSignature:
		return c.RequestSignature(ctx, msg.(*protocol.RequestSignature))
	default:
		return fmt.Errorf("unsupported msg type: %v", msg.MsgType())
	}
}

type CollectorMsg struct {
	CollectorID uuid.UUID
	Msg         protocol.Message
}

type LocalSuperior struct {
	*baseSuperior
	taskCache     *ccache.CCache // indexed by job_id
	taskCacheLock sync.Mutex
	latestTask    protocol.Message // latest request_qualities job
}

func NewLocalSuperior() *LocalSuperior {
	ls := &LocalSuperior{
		taskCache: ccache.NewCCache(100),
	}
	base := newBaseSuperior(map[protocol.MsgType]superiorMsgHandler{
		protocol.MsgTypeReportQualities: ls.onReportQualities,
		protocol.MsgTypeReportProof:     ls.onReportProof,
		protocol.MsgTypeReportSignature: ls.onReportSignature,
	})
	ls.baseSuperior = base
	return ls
}

func (ls *LocalSuperior) Subscribe(ctx context.Context, c Collector) {
	ls.baseSuperior.Subscribe(ctx, c)
	if task := ls.latestTask; task != nil {
		ls.Send(ctx, c.ID(), task)
	}
}

func (ls *LocalSuperior) AddTask(ctx context.Context, collectorID uuid.UUID, req protocol.Message) chan *CollectorMsg {
	ch := make(chan *CollectorMsg, 10)
	ls.taskCacheLock.Lock()
	ls.taskCache.Add(req.ID(), ch)
	ls.taskCacheLock.Unlock()

	if collectorID == uuid.Nil {
		ls.latestTask = req
		ls.Broadcast(ctx, req)
	} else {
		ls.Send(ctx, collectorID, req)
	}

	return ch
}

func (ls *LocalSuperior) RemoveTask(id uuid.UUID) {
	ls.taskCacheLock.Lock()
	defer ls.taskCacheLock.Unlock()
	v, ok := ls.taskCache.Get(id)
	if !ok {
		return
	}
	ch := v.(chan *CollectorMsg)
	close(ch)
	ls.taskCache.Remove(id)
	if ls.latestTask != nil && ls.latestTask.ID() == id {
		ls.latestTask = nil
	}
}

func (ls *LocalSuperior) submitCollectorMsg(ctx context.Context, resp *CollectorMsg) (err error) {
	ls.taskCacheLock.Lock()
	defer ls.taskCacheLock.Unlock()
	v, ok := ls.taskCache.Get(resp.Msg.ID())
	if !ok {
		// TODO: maybe return error
		return nil
	}
	ch := v.(chan *CollectorMsg)
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case ch <- resp:
	}
	return err
}

func (ls *LocalSuperior) onReportQualities(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	return ls.onTypeMsg(ctx, cid, resp, protocol.MsgTypeReportQualities)
}

func (ls *LocalSuperior) onReportProof(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	return ls.onTypeMsg(ctx, cid, resp, protocol.MsgTypeReportProof)
}

func (ls *LocalSuperior) onReportSignature(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	return ls.onTypeMsg(ctx, cid, resp, protocol.MsgTypeReportSignature)
}

func (ls *LocalSuperior) onTypeMsg(ctx context.Context, cid uuid.UUID, resp protocol.Message, typ protocol.MsgType) error {
	if resp.MsgType() != typ {
		return fmt.Errorf("unexpected msg type: want = %v, actual = %v", typ, resp.MsgType())
	}
	return ls.submitCollectorMsg(ctx, &CollectorMsg{CollectorID: cid, Msg: resp})
}

type RemoteSuperior struct {
	*baseSuperior
	wg           sync.WaitGroup
	ctx          context.Context
	ctxCanceller context.CancelFunc
	stopping     int32 // atomic
	stopped      int32 // atomic
	afterStopped func()
	reader       MessageReader
	writer       ReportWriter
	latestTask   protocol.Message // latest request_qualities job
}

func NewRemoteSuperior(ctx context.Context, reader MessageReader, writer ReportWriter, afterStopped func()) (*RemoteSuperior, context.CancelFunc) {
	cCtx, cancel := context.WithCancel(ctx)
	rs := &RemoteSuperior{
		ctx:          cCtx,
		ctxCanceller: cancel,
		afterStopped: afterStopped,
		reader:       reader,
		writer:       writer,
	}
	base := newBaseSuperior(map[protocol.MsgType]superiorMsgHandler{
		protocol.MsgTypeReportQualities: rs.onReportQualities,
		protocol.MsgTypeReportProof:     rs.onReportProof,
		protocol.MsgTypeReportSignature: rs.onReportSignature,
	})
	rs.baseSuperior = base
	return rs, rs.run()
}

func (rs *RemoteSuperior) Subscribe(ctx context.Context, c Collector) {
	rs.baseSuperior.Subscribe(ctx, c)
	if task := rs.latestTask; task != nil {
		rs.Send(ctx, c.ID(), task)
	}
}

func (rs *RemoteSuperior) Reborn(ctx context.Context, reader MessageReader, writer ReportWriter) context.CancelFunc {
	rs.waitStop()
	// reborn with new reader writer
	cCtx, cancel := context.WithCancel(ctx)
	rs.wg = sync.WaitGroup{}
	rs.ctx = cCtx
	rs.ctxCanceller = cancel
	rs.reader = reader
	rs.writer = writer
	return rs.reborn()
}

func (rs *RemoteSuperior) reborn() context.CancelFunc {
	rs.wg.Add(1)
	go rs.requestProcessor()

	// reset atomically
	atomic.StoreInt32(&rs.stopping, 0)
	atomic.StoreInt32(&rs.stopped, 0)

	return func() {
		rs.waitStop()
	}
}

func (rs *RemoteSuperior) run() context.CancelFunc {
	rs.wg.Add(1)
	go rs.requestProcessor()

	return func() {
		rs.waitStop()
	}
}

func (rs *RemoteSuperior) waitStop() {
	if atomic.LoadInt32(&rs.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&rs.stopping, 0, 1) {
		rs.wg.Wait()
		return
	}

	rs.ctxCanceller()
	rs.wg.Wait()
	atomic.StoreInt32(&rs.stopped, 1)

	if rs.afterStopped != nil {
		go rs.afterStopped()
	}
}

func (rs *RemoteSuperior) requestProcessor() {
	defer rs.wg.Done()
process:
	for {
		msg, err := rs.reader.Read(rs.ctx)
		if err != nil {
			go rs.waitStop()
			break process
		}
		if msg.MsgType() == protocol.MsgTypeRequestQualities {
			rs.latestTask = msg
		}
		rs.Broadcast(rs.ctx, msg)
	}
}

func (rs *RemoteSuperior) onReportQualities(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	if err := rs.checkMsgType(resp, protocol.MsgTypeReportQualities); err != nil {
		return err
	}
	return rs.writer.WriteReportQualities(ctx, resp.(*protocol.ReportQualities))
}

func (rs *RemoteSuperior) onReportProof(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	if err := rs.checkMsgType(resp, protocol.MsgTypeReportProof); err != nil {
		return err
	}
	return rs.writer.WriteReportProof(ctx, resp.(*protocol.ReportProof))
}

func (rs *RemoteSuperior) onReportSignature(ctx context.Context, cid uuid.UUID, resp protocol.Message) error {
	if err := rs.checkMsgType(resp, protocol.MsgTypeReportSignature); err != nil {
		return err
	}
	return rs.writer.WriteReportSignature(ctx, resp.(*protocol.ReportSignature))
}

func (rs *RemoteSuperior) checkMsgType(resp protocol.Message, typ protocol.MsgType) error {
	if resp.MsgType() != typ {
		return fmt.Errorf("unexpected msg type: want = %v, actual = %v", typ, resp.MsgType())
	}
	return nil
}

const (
	PersistentRemoteSuperiorRetryInterval = 30 * time.Second
)

type PersistentRemoteSuperior struct {
	*RemoteSuperior
	ctx           context.Context
	ctxCanceller  context.CancelFunc
	stopped       int32 // atomic
	dialer        *Dialer
	rsCanceller   context.CancelFunc
	connCanceller context.CancelFunc
}

func NewPersistentRemoteSuperior(ctx context.Context, opts ...connection.Option) (*PersistentRemoteSuperior, context.CancelFunc, error) {
	cCtx, cancel := context.WithCancel(ctx)
	opts = append(opts, connection.WithContext(cCtx), connection.KeepaliveInterval(0))
	prs := &PersistentRemoteSuperior{
		ctx:          cCtx,
		ctxCanceller: cancel,
		dialer:       NewDialer(opts...),
	}
	return prs.born()
}

func (prs *PersistentRemoteSuperior) born() (*PersistentRemoteSuperior, context.CancelFunc, error) {
	var (
		conn       *connection.Conn
		connCancel context.CancelFunc
		err        error
		timer      = time.NewTimer(PersistentRemoteSuperiorRetryInterval)
	)

	var doneCh = prs.ctx.Done()

	for {
		conn, connCancel, err = prs.dialer.Dial()
		if err == nil {
			logging.CPrint(logging.INFO, "remote_superior conn accepted", logging.LogFormat{
				"local_addr":  conn.LocalAddr(),
				"remote_addr": conn.RemoteAddr(),
			})
			break
		}
		logging.CPrint(logging.ERROR, "fail to dial remote_superior", logging.LogFormat{"err": err})
		timer.Reset(PersistentRemoteSuperiorRetryInterval)
		select {
		case <-doneCh:
			return nil, nil, prs.ctx.Err()
		case <-timer.C:
		}
	}

	reader := NewRemoteRequestReader(prs.ctx, conn)
	writer := NewRemoteReportWriter(prs.ctx, conn)
	rs, rsCancel := NewRemoteSuperior(prs.ctx, reader, writer, prs.reborn)
	prs.RemoteSuperior, prs.connCanceller, prs.rsCanceller = rs, connCancel, rsCancel
	return prs, prs.waitStop, nil
}

func (prs *PersistentRemoteSuperior) reborn() {
	var (
		conn       *connection.Conn
		connCancel context.CancelFunc
		err        error
		timer      = time.NewTimer(PersistentRemoteSuperiorRetryInterval)
	)

	var doneCh = prs.ctx.Done()

	for {
		select {
		case <-doneCh:
			return
		case <-timer.C:
		}
		conn, connCancel, err = prs.dialer.Dial()
		if err == nil {
			logging.CPrint(logging.INFO, "remote_superior conn accepted", logging.LogFormat{
				"local_addr":  conn.LocalAddr(),
				"remote_addr": conn.RemoteAddr(),
			})
			break
		}
		logging.CPrint(logging.ERROR, "fail to dial remote_superior", logging.LogFormat{"err": err})
		timer.Reset(PersistentRemoteSuperiorRetryInterval)
	}

	reader := NewRemoteRequestReader(prs.ctx, conn)
	writer := NewRemoteReportWriter(prs.ctx, conn)
	rsCancel := prs.RemoteSuperior.Reborn(prs.ctx, reader, writer)
	prs.connCanceller, prs.rsCanceller = connCancel, rsCancel
}

func (prs *PersistentRemoteSuperior) waitStop() {
	if atomic.LoadInt32(&prs.stopped) == 1 {
		return
	}

	prs.ctxCanceller()
	prs.connCanceller()
	prs.rsCanceller()

	atomic.StoreInt32(&prs.stopped, 1)
}
