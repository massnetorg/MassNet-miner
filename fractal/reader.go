package fractal

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/massnetorg/mass-core/logging"
	"massnet.org/mass/fractal/connection"
	"massnet.org/mass/fractal/protocol"
)

type MessageReader interface {
	Read(context.Context) (protocol.Message, error)
}

type RemoteRequestReader struct {
	receiver *MessageReceiver
}

func NewRemoteRequestReader(ctx context.Context, conn *connection.Conn) *RemoteRequestReader {
	receiver, _ := NewMessageReceiver(ctx, conn, map[protocol.MsgType]bool{
		protocol.MsgTypeRequestQualities: false,
		protocol.MsgTypeRequestProof:     true,
		protocol.MsgTypeRequestSignature: true,
	})
	return &RemoteRequestReader{
		receiver: receiver,
	}
}

func (remote *RemoteRequestReader) Read(ctx context.Context) (protocol.Message, error) {
	return remote.receiver.SafeReadChannel(ctx)
}

type RemoteReportReader struct {
	receiver *MessageReceiver
}

func NewRemoteReportReader(ctx context.Context, conn *connection.Conn) *RemoteReportReader {
	receiver, _ := NewMessageReceiver(ctx, conn, map[protocol.MsgType]bool{
		protocol.MsgTypeReportQualities: false,
		protocol.MsgTypeReportProof:     true,
		protocol.MsgTypeReportSignature: true,
	})
	return &RemoteReportReader{
		receiver: receiver,
	}
}

func (remote *RemoteReportReader) Read(ctx context.Context) (protocol.Message, error) {
	return remote.receiver.SafeReadChannel(ctx)
}

type MessageReceiver struct {
	wg           sync.WaitGroup
	ctx          context.Context
	ctxCanceller context.CancelFunc
	stopping     int32 // atomic
	stopped      int32 // atomic
	messageCh    chan protocol.Message
	priorities   map[protocol.MsgType]bool
	conn         *connection.Conn
}

func NewMessageReceiver(ctx context.Context, conn *connection.Conn, priorities map[protocol.MsgType]bool) (*MessageReceiver, context.CancelFunc) {
	cCtx, cancel := context.WithCancel(ctx)
	sender := &MessageReceiver{
		ctx:          cCtx,
		ctxCanceller: cancel,
		messageCh:    make(chan protocol.Message, 10),
		priorities:   priorities,
		conn:         conn,
	}
	return sender, sender.run()
}

func (receiver *MessageReceiver) SafeReadChannel(ctx context.Context) (msg protocol.Message, err error) {
	var ok bool
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case msg, ok = <-receiver.messageCh:
		if !ok {
			err = fmt.Errorf("MessageReceiver.SafeReadChannel: %w", io.EOF)
		}
	}
	return
}

func (receiver *MessageReceiver) run() context.CancelFunc {
	receiver.wg.Add(1)
	go receiver.messageProcessor()

	return func() {
		receiver.waitStop()
	}
}

func (receiver *MessageReceiver) waitStop() {
	if atomic.LoadInt32(&receiver.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&receiver.stopping, 0, 1) {
		receiver.wg.Wait()
		return
	}

	receiver.ctxCanceller()
	receiver.wg.Wait()
	atomic.StoreInt32(&receiver.stopped, 1)
}

func (receiver *MessageReceiver) messageProcessor() {
	defer receiver.wg.Done()
	defer close(receiver.messageCh)

	var doneCh = receiver.ctx.Done()
process:
	for {
		msg, err := receiver.readRemoteMessage(receiver.ctx)
		if err != nil {
			go receiver.waitStop()
			break process
		}
		// process message
		_, allowed := receiver.priorities[msg.MsgType()]
		if !allowed {
			logging.CPrint(logging.WARN, "unsupported fractal protocol msg type", logging.LogFormat{
				"type": msg.MsgType(),
			})
			continue process
		}
		select {
		case <-doneCh:
			break process
		case receiver.messageCh <- msg:
		}
	}
}

func (receiver *MessageReceiver) readRemoteMessage(ctx context.Context) (protocol.Message, error) {
	data, err := receiver.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	return protocol.DecodeMessage(data)
}
