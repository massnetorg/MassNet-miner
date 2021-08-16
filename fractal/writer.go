package fractal

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/massnetorg/mass-core/logging"
	"massnet.org/mass/fractal/connection"
	"massnet.org/mass/fractal/protocol"
)

type RequestWriter interface {
	WriteRequestQualities(context.Context, *protocol.RequestQualities) error
	WriteRequestProof(context.Context, *protocol.RequestProof) error
	WriteRequestSignature(context.Context, *protocol.RequestSignature) error
}

type RemoteRequestWriter struct {
	sender *MessageSender
}

func NewRemoteRequestWriter(ctx context.Context, conn *connection.Conn) *RemoteRequestWriter {
	sender, _ := NewMessageSender(ctx, conn, map[protocol.MsgType]bool{
		protocol.MsgTypeRequestQualities: false,
		protocol.MsgTypeRequestProof:     true,
		protocol.MsgTypeRequestSignature: true,
	})
	return &RemoteRequestWriter{
		sender: sender,
	}
}

func (remote *RemoteRequestWriter) WriteRequestQualities(ctx context.Context, req *protocol.RequestQualities) error {
	return remote.sendRequest(ctx, req)
}

func (remote *RemoteRequestWriter) WriteRequestProof(ctx context.Context, req *protocol.RequestProof) error {
	return remote.sendRequest(ctx, req)
}

func (remote *RemoteRequestWriter) WriteRequestSignature(ctx context.Context, req *protocol.RequestSignature) error {
	return remote.sendRequest(ctx, req)
}

func (remote *RemoteRequestWriter) sendRequest(ctx context.Context, req protocol.Message) error {
	return remote.sender.SafeSendChannel(ctx, req)
}

type ReportWriter interface {
	WriteReportQualities(context.Context, *protocol.ReportQualities) error
	WriteReportProof(context.Context, *protocol.ReportProof) error
	WriteReportSignature(context.Context, *protocol.ReportSignature) error
}

type RemoteReportWriter struct {
	sender *MessageSender
}

func NewRemoteReportWriter(ctx context.Context, conn *connection.Conn) *RemoteReportWriter {
	sender, _ := NewMessageSender(ctx, conn, map[protocol.MsgType]bool{
		protocol.MsgTypeReportQualities: false,
		protocol.MsgTypeReportProof:     true,
		protocol.MsgTypeReportSignature: true,
	})
	return &RemoteReportWriter{
		sender: sender,
	}
}

func (remote *RemoteReportWriter) WriteReportQualities(ctx context.Context, resp *protocol.ReportQualities) error {
	return remote.sendReport(ctx, resp)
}

func (remote *RemoteReportWriter) WriteReportProof(ctx context.Context, resp *protocol.ReportProof) error {
	return remote.sendReport(ctx, resp)
}

func (remote *RemoteReportWriter) WriteReportSignature(ctx context.Context, resp *protocol.ReportSignature) error {
	return remote.sendReport(ctx, resp)
}

func (remote *RemoteReportWriter) sendReport(ctx context.Context, resp protocol.Message) error {
	return remote.sender.SafeSendChannel(ctx, resp)
}

type MessageSender struct {
	wg           sync.WaitGroup
	ctx          context.Context
	ctxCanceller context.CancelFunc
	stopping     int32 // atomic
	stopped      int32 // atomic
	messageCh    chan protocol.Message
	priorities   map[protocol.MsgType]bool
	conn         *connection.Conn
}

func NewMessageSender(ctx context.Context, conn *connection.Conn, priorities map[protocol.MsgType]bool) (*MessageSender, context.CancelFunc) {
	cCtx, cancel := context.WithCancel(ctx)
	sender := &MessageSender{
		ctx:          cCtx,
		ctxCanceller: cancel,
		messageCh:    make(chan protocol.Message, 10),
		priorities:   priorities,
		conn:         conn,
	}
	return sender, sender.run()
}

func (sender *MessageSender) SafeSendChannel(ctx context.Context, msg protocol.Message) (err error) {
	defer func() {
		if recover() != nil {
			err = io.ErrClosedPipe
		}
	}()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case sender.messageCh <- msg:
	}
	// panic if ch is closed
	return err
}

func (sender *MessageSender) run() context.CancelFunc {
	sender.wg.Add(1)
	go sender.messageProcessor()

	return func() {
		sender.waitStop()
	}
}

func (sender *MessageSender) waitStop() {
	if atomic.LoadInt32(&sender.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&sender.stopping, 0, 1) {
		sender.wg.Wait()
		return
	}

	sender.ctxCanceller()
	sender.wg.Wait()
	atomic.StoreInt32(&sender.stopped, 1)
}

func (sender *MessageSender) messageProcessor() {
	defer sender.wg.Done()
	defer close(sender.messageCh)

	var doneCh = sender.ctx.Done()
process:
	for {
		var msg protocol.Message
		select {
		case <-doneCh:
			break process
		case msg = <-sender.messageCh:
		}
		// process message
		priority, allowed := sender.priorities[msg.MsgType()]
		if !allowed {
			logging.CPrint(logging.WARN, "unsupported fractal protocol msg type", logging.LogFormat{
				"type": msg.MsgType(),
			})
			continue process
		}
		if err := sender.sendRemoteMessage(sender.ctx, msg, priority); err != nil {
			logging.CPrint(logging.ERROR, "fail to send remote message", logging.LogFormat{
				"err":      err,
				"type":     msg.MsgType(),
				"priority": priority,
			})
			go sender.waitStop()
		}
	}
}

func (sender *MessageSender) sendRemoteMessage(ctx context.Context, msg protocol.Message, priority bool) error {
	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		return err
	}
	if priority {
		err = sender.conn.SendPriority(ctx, data)
	} else {
		err = sender.conn.Send(ctx, data)
	}
	return err
}
