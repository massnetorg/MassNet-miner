package fractal

import (
	"context"
	"io"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/massnetorg/mass-core/consensus/difficulty"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/config"
	"massnet.org/mass/fractal/protocol"
	engine_v2 "massnet.org/mass/poc/engine.v2"
	"massnet.org/mass/poc/engine.v2/spacekeeper"
)

const (
	pocSlot    = poc.PoCSlot
	allowAhead = 10
)

type Collector interface {
	ID() uuid.UUID
	RequestQualities(context.Context, *protocol.RequestQualities) error
	RequestProof(context.Context, *protocol.RequestProof) error
	RequestSignature(context.Context, *protocol.RequestSignature) error
}

type LocalCollector struct {
	id                 uuid.UUID
	wg                 sync.WaitGroup
	ctx                context.Context
	ctxCanceller       context.CancelFunc
	stopping           int32 // atomic
	stopped            int32 // atomic
	requestCh          chan protocol.Message
	qualitiesCanceller func() // cancel previous RequestQuality
	superior           Superior
	sk                 spacekeeper.SpaceKeeper
}

func NewLocalCollector(ctx context.Context, superior Superior, sk spacekeeper.SpaceKeeper) (*LocalCollector, context.CancelFunc) {
	cCtx, cancel := context.WithCancel(ctx)
	lc := &LocalCollector{
		id:           uuid.New(),
		ctx:          cCtx,
		ctxCanceller: cancel,
		requestCh:    make(chan protocol.Message, 10),
		superior:     superior,
		sk:           sk,
	}
	return lc, lc.run()
}

func (lc *LocalCollector) ID() uuid.UUID {
	return lc.id
}

func (lc *LocalCollector) RequestQualities(ctx context.Context, req *protocol.RequestQualities) error {
	return lc.sendRequest(ctx, req)
}

func (lc *LocalCollector) RequestProof(ctx context.Context, req *protocol.RequestProof) error {
	return lc.sendRequest(ctx, req)
}

func (lc *LocalCollector) RequestSignature(ctx context.Context, req *protocol.RequestSignature) error {
	return lc.sendRequest(ctx, req)
}

func (lc *LocalCollector) sendRequest(ctx context.Context, req protocol.Message) error {
	return lc.safeSendChannel(ctx, req)
}

func (lc *LocalCollector) safeSendChannel(ctx context.Context, req protocol.Message) (err error) {
	defer func() {
		if recover() != nil {
			err = io.ErrClosedPipe
		}
	}()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case lc.requestCh <- req:
		// panic if ch is closed
	}
	return err
}

func (lc *LocalCollector) run() context.CancelFunc {
	lc.wg.Add(1)
	go lc.requestProcessor()

	return func() {
		lc.waitStop()
	}
}

func (lc *LocalCollector) waitStop() {
	if atomic.LoadInt32(&lc.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&lc.stopping, 0, 1) {
		lc.wg.Wait()
		return
	}

	lc.ctxCanceller()
	lc.wg.Wait()
	atomic.StoreInt32(&lc.stopped, 1)
}

func (lc *LocalCollector) requestProcessor() {
	defer lc.wg.Done()
	defer close(lc.requestCh)

	lc.superior.Subscribe(lc.ctx, lc)
	defer lc.superior.Unsubscribe(lc.ctx, lc)

	var doneCh = lc.ctx.Done()
process:
	for {
		var msg protocol.Message
		select {
		case <-doneCh:
			break process
		case msg = <-lc.requestCh:
		}
		// process message
		switch msg.MsgType() {
		case protocol.MsgTypeRequestQualities:
			if lc.qualitiesCanceller != nil {
				lc.qualitiesCanceller()
			}
			var cCtx context.Context
			cCtx, lc.qualitiesCanceller = context.WithCancel(lc.ctx)
			go lc.onRequestQualities(cCtx, msg.(*protocol.RequestQualities))

		case protocol.MsgTypeRequestProof:
			go lc.onRequestProof(lc.ctx, msg.(*protocol.RequestProof))

		case protocol.MsgTypeRequestSignature:
			go lc.onRequestSignature(lc.ctx, msg.(*protocol.RequestSignature))

		default:
			logging.CPrint(logging.WARN, "invalid fractal protocol msg type", logging.LogFormat{"type": msg.MsgType()})
			continue process
		}
	}
}

func (lc *LocalCollector) onRequestQualities(cCtx context.Context, req *protocol.RequestQualities) {
	logging.CPrint(logging.INFO, "request qualities", logging.LogFormat{
		"task_id":       req.TaskID,
		"challenge":     req.Challenge,
		"parent_target": req.ParentTarget.Text(16),
		"parent_slot":   req.ParentSlot,
		"height":        req.Height,
	})

	queryStart := time.Now()
	skChiaQualities, err := lc.sk.GetQualities(cCtx, engine_v2.SFMining, req.Challenge)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on sk.GetQualities", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
		})
		return
	}
	queryElapsed := time.Since(queryStart)

	chiaQualities := getValidChiaQualities(skChiaQualities)
	logging.CPrint(logging.INFO, "find qualities", logging.LogFormat{
		"task_id":     req.TaskID,
		"valid_count": len(chiaQualities),
		"total_count": len(skChiaQualities),
		"query_time":  queryElapsed.Seconds(),
	})
	if len(chiaQualities) == 0 {
		return
	}

	lastHeader := &wire.BlockHeader{
		Timestamp: time.Unix(int64(req.ParentSlot*pocSlot), 0),
		Target:    req.ParentTarget,
	}
	var calcTarget = func(slot uint64) *big.Int {
		target, _ := difficulty.CalcNextRequiredDifficulty(lastHeader, time.Unix(int64(slot*pocSlot), 0), config.ChainParams)
		return target
	}

	lc.trySlots(cCtx, req, chiaQualities, calcTarget)
}

func (lc *LocalCollector) trySlots(cCtx context.Context, req *protocol.RequestQualities,
	chiaQualities []*engine_v2.WorkSpaceQuality, calcTarget func(slot uint64) *big.Int) {
	var (
		workSlot = req.ParentSlot + 1
		doneCh   = cCtx.Done()
		ticker   = time.NewTicker(time.Second * pocSlot / 4)
	)
	defer ticker.Stop()

try:
	for {
		select {
		case <-doneCh:
			return
		case <-ticker.C:
			nowSlot := uint64(time.Now().Unix()) / pocSlot
			if workSlot > nowSlot+allowAhead {
				logging.CPrint(logging.DEBUG, "mining too far in the future",
					logging.LogFormat{"nowSlot": nowSlot, "workSlot": workSlot})
				continue try
			}

			// try to solve, until workSlot reaches nowSlot+allowAhead
			for i := workSlot; i <= nowSlot+allowAhead; i++ {
				// Escape from loop when quit received.
				select {
				case <-doneCh:
					return
				default:
				}

				// Ensure there are valid qualities
				qualities, _ := getMASSQualities(chiaQualities, workSlot, req.Height)

				// find qualities over target
				currentTarget := calcTarget(workSlot)
				var satisfiedQualities []*engine_v2.WorkSpaceQuality
				for j, quality := range qualities {
					if quality.Cmp(currentTarget) > 0 {
						satisfiedQualities = append(satisfiedQualities, chiaQualities[j])
					}
				}

				// submit satisfied qualities
				lc.reportQualities(cCtx, req, satisfiedQualities, workSlot)

				// increase slot and header Timestamp
				atomic.AddUint64(&workSlot, 1)
			}
			continue try
		}
	}
}

func (lc *LocalCollector) reportQualities(ctx context.Context, req *protocol.RequestQualities, wsqList []*engine_v2.WorkSpaceQuality, slot uint64) {
	if len(wsqList) == 0 {
		return
	}
	qualities := make([]*protocol.Quality, 0, len(wsqList))
	for i := range wsqList {
		qualities = append(qualities, &protocol.Quality{
			WorkSpaceQuality: wsqList[i],
			Slot:             slot,
		})
	}
	resp := &protocol.ReportQualities{
		TaskID:    req.TaskID,
		Qualities: qualities,
	}
	if err := lc.superior.ReportQualities(ctx, lc.id, resp); err != nil {
		logging.CPrint(logging.ERROR, "fail to submit qualities", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
			"count":   len(wsqList),
			"slot":    slot,
		})
	} else {
		logging.CPrint(logging.INFO, "submit qualities", logging.LogFormat{
			"task_id": req.TaskID,
			"count":   len(wsqList),
			"slot":    slot,
		})
	}
}

func (lc *LocalCollector) onRequestProof(cCtx context.Context, req *protocol.RequestProof) {
	logging.CPrint(logging.INFO, "request proof", logging.LogFormat{
		"task_id":   req.TaskID,
		"height":    req.Height,
		"space_id":  req.SpaceID,
		"challenge": req.Challenge,
		"index":     req.Index,
	})

	wsp, err := lc.sk.GetProof(cCtx, req.SpaceID, req.Challenge, req.Index)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on sk.GetProof", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
		})
		return
	}
	proof := (*protocol.Proof)(wsp)
	resp := &protocol.ReportProof{
		TaskID: req.TaskID,
		Proof:  proof,
	}
	if err = lc.superior.ReportProof(cCtx, lc.id, resp); err != nil {
		logging.CPrint(logging.ERROR, "fail to submit proof", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
		})
	} else {
		logging.CPrint(logging.INFO, "submit proof", logging.LogFormat{"task_id": req.TaskID})
	}
}

func (lc *LocalCollector) onRequestSignature(cCtx context.Context, req *protocol.RequestSignature) {
	logging.CPrint(logging.INFO, "request signature", logging.LogFormat{
		"task_id":  req.TaskID,
		"height":   req.Height,
		"space_id": req.SpaceID,
		"hash":     req.Hash,
	})

	sig, err := lc.sk.SignHash(req.SpaceID, req.Hash)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on sk.SignHash", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
		})
		return
	}
	resp := &protocol.ReportSignature{
		TaskID:    req.TaskID,
		SpaceID:   req.SpaceID,
		Hash:      req.Hash,
		Signature: sig,
	}
	if err = lc.superior.ReportSignature(cCtx, lc.id, resp); err != nil {
		logging.CPrint(logging.ERROR, "fail to submit signature", logging.LogFormat{
			"err":     err,
			"task_id": req.TaskID,
		})
	} else {
		logging.CPrint(logging.INFO, "submit signature", logging.LogFormat{"task_id": req.TaskID})
	}
}

func getValidChiaQualities(chiaQualities []*engine_v2.WorkSpaceQuality) []*engine_v2.WorkSpaceQuality {
	result := make([]*engine_v2.WorkSpaceQuality, 0)
	for i := range chiaQualities {
		if chiaQualities[i].Error == nil {
			result = append(result, chiaQualities[i])
		}
	}
	return result
}

func getMASSQualities(chiaQualities []*engine_v2.WorkSpaceQuality, slot, height uint64) ([]*big.Int, error) {
	massQualities := make([]*big.Int, len(chiaQualities))
	for i, chiaQuality := range chiaQualities {
		chiaHashVal := poc.HashValChia(chiaQuality.Quality, slot, height)
		massQualities[i] = poc.GetQuality(poc.Q1FactorChia(chiaQuality.KSize), chiaHashVal)
	}
	return massQualities, nil
}

type RemoteCollector struct {
	id           uuid.UUID
	wg           sync.WaitGroup
	ctx          context.Context
	ctxCanceller context.CancelFunc
	stopping     int32 // atomic
	stopped      int32 // atomic
	afterStopped func(Collector)
	superior     Superior
	writer       RequestWriter
	reader       MessageReader
	reportCh     chan protocol.Message
}

func NewRemoteCollector(ctx context.Context, superior Superior, writer RequestWriter, reader MessageReader, afterStopped func(Collector)) (*RemoteCollector, context.CancelFunc) {
	cCtx, cancel := context.WithCancel(ctx)
	rc := &RemoteCollector{
		id:           uuid.New(),
		ctx:          cCtx,
		ctxCanceller: cancel,
		afterStopped: afterStopped,
		superior:     superior,
		writer:       writer,
		reader:       reader,
		reportCh:     make(chan protocol.Message, 10),
	}
	return rc, rc.run()
}

func (rc *RemoteCollector) ID() uuid.UUID {
	return rc.id
}

func (rc *RemoteCollector) RequestQualities(ctx context.Context, req *protocol.RequestQualities) error {
	return rc.writer.WriteRequestQualities(ctx, req)
}

func (rc *RemoteCollector) RequestProof(ctx context.Context, req *protocol.RequestProof) error {
	return rc.writer.WriteRequestProof(ctx, req)
}

func (rc *RemoteCollector) RequestSignature(ctx context.Context, req *protocol.RequestSignature) error {
	return rc.writer.WriteRequestSignature(ctx, req)
}

func (rc *RemoteCollector) run() context.CancelFunc {
	rc.wg.Add(1)
	go rc.reportProcessor()

	return func() {
		rc.waitStop()
	}
}

func (rc *RemoteCollector) waitStop() {
	if atomic.LoadInt32(&rc.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&rc.stopping, 0, 1) {
		rc.wg.Wait()
		return
	}

	rc.ctxCanceller()
	rc.wg.Wait()
	atomic.StoreInt32(&rc.stopped, 1)

	if rc.afterStopped != nil {
		go rc.afterStopped(rc)
	}
}

func (rc *RemoteCollector) reportProcessor() {
	defer rc.wg.Done()
	defer close(rc.reportCh)

	rc.superior.Subscribe(rc.ctx, rc)
	defer rc.superior.Unsubscribe(rc.ctx, rc)

process:
	for {
		msg, err := rc.reader.Read(rc.ctx)
		if err != nil {
			go rc.waitStop()
			break process
		}
		// process message
		switch msg.MsgType() {
		case protocol.MsgTypeReportQualities:
			err = rc.superior.ReportQualities(rc.ctx, rc.id, msg.(*protocol.ReportQualities))

		case protocol.MsgTypeReportProof:
			err = rc.superior.ReportProof(rc.ctx, rc.id, msg.(*protocol.ReportProof))

		case protocol.MsgTypeReportSignature:
			err = rc.superior.ReportSignature(rc.ctx, rc.id, msg.(*protocol.ReportSignature))

		default:
			logging.CPrint(logging.WARN, "invalid fractal protocol msg type", logging.LogFormat{"type": msg.MsgType()})
			continue process
		}
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to report to superior", logging.LogFormat{
				"err":  err,
				"type": msg.MsgType(),
			})
		}
	}
}
