package miner

import (
	"context"
	"errors"
	"math"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/consensus/forks"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/fractal"
	"massnet.org/mass/fractal/protocol"
	"massnet.org/mass/poc/engine.v2"
	"massnet.org/mass/poc/engine.v2/pocminer"
	"massnet.org/mass/poc/engine.v2/spacekeeper"
)

const (
	TypeChiaPosMiner = "chiapos"
)

func NewChiaPosMiner(args ...interface{}) (pocminer.PoCMiner, error) {
	allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses, keystore, superior, err := parseArgs(args...)
	if err != nil {
		return nil, err
	}
	m := NewPoCMiner(TypeChiaPosMiner, allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses, keystore, superior)
	return m, nil
}

func parseArgs(args ...interface{}) (allowSolo bool, chain Chain, syncManager SyncManager, sk spacekeeper.SpaceKeeper,
	newBlockCh chan *wire.Hash, payoutAddresses []massutil.Address, keystore *chiawallet.Keystore, superior *fractal.LocalSuperior, err error) {

	var failure = func() (bool, Chain, SyncManager, spacekeeper.SpaceKeeper, chan *wire.Hash, []massutil.Address, *chiawallet.Keystore, *fractal.LocalSuperior, error) {
		expectedTypes := []reflect.Type{reflect.TypeOf(chain), reflect.TypeOf(syncManager), reflect.TypeOf(superior), reflect.TypeOf(newBlockCh), reflect.TypeOf(payoutAddresses)}
		actualTypes := make([]reflect.Type, len(args))
		for i, arg := range args {
			actualTypes[i] = reflect.TypeOf(arg)
		}
		logging.CPrint(logging.ERROR, "invalid miner arg types", logging.LogFormat{"expected": expectedTypes, "actual": actualTypes, "err": pocminer.ErrInvalidMinerArgs})
		return false, nil, nil, nil, nil, nil, nil, nil, pocminer.ErrInvalidMinerArgs
	}

	if len(args) != 8 {
		return failure()
	}
	var ok bool
	allowSolo, ok = args[0].(bool)
	if !ok {
		return failure()
	}
	chain, ok = args[1].(Chain)
	if !ok {
		return failure()
	}
	syncManager, ok = args[2].(SyncManager)
	if !ok {
		return failure()
	}
	sk, ok = args[3].(spacekeeper.SpaceKeeper)
	if !ok {
		return failure()
	}
	newBlockCh, ok = args[4].(chan *wire.Hash)
	if !ok {
		return failure()
	}
	payoutAddresses, ok = args[5].([]massutil.Address)
	if !ok {
		return failure()
	}
	keystore, ok = args[6].(*chiawallet.Keystore)
	if !ok {
		return failure()
	}
	superior, ok = args[7].(*fractal.LocalSuperior)
	if !ok {
		return failure()
	}
	return
}

func (m *PoCMiner) solveBlock(payoutAddresses []massutil.Address, quit chan struct{}) (*wire.MsgBlock, massutil.Amount, error) {
	var failure = func(err error) (*wire.MsgBlock, massutil.Amount, error) {
		logging.CPrint(logging.INFO, "quit solve block", logging.LogFormat{"err": err})
		return nil, massutil.ZeroAmount(), err
	}

	if !forks.EnforceMASSIP0002(m.chain.BestBlockHeight() + 1) {
		time.Sleep(3 * time.Second)
		return nil, massutil.Amount{}, errNotMassip2Block
	}

	// Step 1: request for poc & body template
	logging.CPrint(logging.INFO, "Step 1: request for poc & body template")
	templateCh := make(chan interface{}, 2)
	if err := m.chain.NewBlockTemplate(payoutAddresses, templateCh); err != nil {
		return failure(err)
	}

	// Step 2: wait for poc template
	logging.CPrint(logging.INFO, "Step 2: wait for poc template")
	pocTemplateI, err := getTemplate(quit, templateCh, reflect.TypeOf(&blockchain.PoCTemplate{}))
	if err != nil {
		return failure(err)
	}
	pocTemplate := pocTemplateI.(*blockchain.PoCTemplate)

	// Step 3: check for double mining
	logging.CPrint(logging.INFO, "Step 3: check for double mining")
	if _, ok := m.minedHeight[pocTemplate.Height]; ok {
		time.Sleep(time.Second)
		logging.CPrint(logging.INFO, "sleep mining for 1 sec to avoid double mining", logging.LogFormat{"height": pocTemplate.Height})
		return failure(errAvoidDoubleMining)
	}

	// Step 4: get best proof
	logging.CPrint(logging.INFO, "Step 4: get best proof", logging.LogFormat{
		"height":    pocTemplate.Height,
		"previous":  pocTemplate.Previous,
		"timestamp": pocTemplate.Timestamp.Unix(),
		"challenge": pocTemplate.Challenge,
	})
	tProof, collectorID, err := m.getBestProof(pocTemplate, quit)
	if err != nil {
		if err == errNoValidProof {
			time.Sleep(time.Second * pocSlot)
			logging.CPrint(logging.INFO, "sleep mining for 3 sec to wait for valid poofs", logging.LogFormat{"height": pocTemplate.Height})
		}
		return failure(err)
	}

	// Step 5: wait for chain template
	logging.CPrint(logging.INFO, "Step 5: wait for chain template")
	blockTemplateI, err := getTemplate(quit, templateCh, reflect.TypeOf(&blockchain.BlockTemplate{}))
	if err != nil {
		return failure(err)
	}
	blockTemplate := blockTemplateI.(*blockchain.BlockTemplate)

	// Step 6: assemble full block
	logging.CPrint(logging.INFO, "Step 6: assemble full block")
	block, minerReward, err := assembleFullBlock(blockTemplate, pocTemplate, tProof)
	if err != nil {
		return failure(err)
	}

	// Step 7: get plot signature for poc hash
	signer := chiapos.NewAugSchemeMPL()
	logging.CPrint(logging.INFO, "Step 7: get plot signature for poc hash")
	pocHash, err := block.Header.PoCHash()
	if err != nil {
		return failure(err)
	}
	sigReq := &protocol.RequestSignature{
		TaskID:  uuid.New(),
		Height:  pocTemplate.Height,
		SpaceID: tProof.proof.SpaceID,
		Hash:    pocutil.SHA256(pocHash[:]),
	}
	sigCh := m.superior.AddTask(context.Background(), collectorID, sigReq)
	defer m.superior.RemoveTask(sigReq.TaskID)
	var localSig *chiapos.G2Element
	select {
	case <-time.After(time.Second * 5):
		return failure(errors.New("request_signature timeout"))
	case newMsg := <-sigCh:
		localSig = newMsg.Msg.(*protocol.ReportSignature).Signature
	}

	// Step 8: get farmer signature for poc hash
	logging.CPrint(logging.INFO, "Step 8: get farmer signature for poc hash")
	minerKey, err := m.keystore.GetMinerKeyByPoolPublicKey(tProof.proof.Proof.PoolPublicKey)
	if err != nil {
		return failure(err)
	}
	farmerSig, err := signer.SignPrepend(minerKey.FarmerPrivateKey, wire.HashB(pocHash[:]), tProof.proof.PublicKey)
	if err != nil {
		return failure(err)
	}
	block.Header.Signature, err = signer.Aggregate(localSig, farmerSig)
	if err != nil {
		return failure(err)
	}

	logging.CPrint(logging.INFO, "Step 9: return")
	return block, minerReward, nil
}

func (m *PoCMiner) getBestProof(pocTemplate *blockchain.PoCTemplate, quit chan struct{}) (*ProofTemplate, uuid.UUID, error) {
	var workSlot = uint64(pocTemplate.Timestamp.Unix()) / pocSlot

	cancelMonitor, staled, err := runStaleMonitor(m.chain, &workSlot, &pocTemplate.Previous)
	if err != nil {
		return nil, uuid.Nil, err
	}
	// cancel stale monitor
	defer cancelMonitor()

	var challenge = pocutil.Hash(pocTemplate.Challenge)
	ticker := time.NewTicker(time.Second * pocSlot / 4)
	defer ticker.Stop()

	// submit job
	qualityReq := &protocol.RequestQualities{
		TaskID:       uuid.New(),
		Challenge:    challenge,
		ParentTarget: pocTemplate.GetTarget(pocTemplate.Timestamp.Add(pocSlot * 10)),
		ParentSlot:   uint64(pocTemplate.Timestamp.Unix())/pocSlot - 1,
		Height:       pocTemplate.Height,
	}
	qualityCh := m.superior.AddTask(context.Background(), uuid.Nil, qualityReq)
	logging.CPrint(logging.INFO, "submit request_qualities task", logging.LogFormat{
		"height":  pocTemplate.Height,
		"task_id": qualityReq.TaskID,
	})
	defer m.superior.RemoveTask(qualityReq.TaskID)
	var (
		bestWorkSpaceQuality *engine.WorkSpaceQuality
		bestQuality                 = big.NewInt(-1)
		bestSlot             uint64 = math.MaxUint64
		bestSpaceID          string
		bestIndex            uint32
		bestCollector        uuid.UUID
	)
try:
	for {
		select {
		case <-quit:
			return nil, uuid.Nil, errQuitSolveBlock

		case newMsg := <-qualityCh:
			newQualities := newMsg.Msg.(*protocol.ReportQualities)
			for i := range newQualities.Qualities {
				pq := newQualities.Qualities[i]
				chiaHashVal := poc.HashValChia(pq.Quality, pq.Slot, pocTemplate.Height)
				quality := poc.GetQuality(poc.Q1FactorChia(pq.KSize), chiaHashVal)
				target := pocTemplate.GetTarget(time.Unix(int64(pq.Slot*pocSlot), 0))
				if quality.Cmp(target) <= 0 {
					// not satisfy target
					continue
				}
				// check binding & select best quality
				if pq.Slot < bestSlot || (pq.Slot == bestSlot && quality.Cmp(bestQuality) > 0) {
					if !pocTemplate.PassBinding(pq.WorkSpaceQuality) {
						continue
					}
					bestWorkSpaceQuality = pq.WorkSpaceQuality
					bestQuality = quality
					bestSlot = pq.Slot
					bestSpaceID = pq.SpaceID
					bestIndex = pq.Index
					bestCollector = newMsg.CollectorID
					atomic.StoreUint64(&workSlot, bestSlot)
					logging.CPrint(logging.INFO, "update best quality", logging.LogFormat{
						"task_id":      qualityReq.TaskID,
						"slot":         bestSlot,
						"height":       pocTemplate.Height,
						"quality":      bestQuality.Text(10),
						"space_id":     bestSpaceID,
						"collector_id": bestCollector,
					})
				}
			}

		case <-ticker.C:
			if staled() {
				logging.CPrint(logging.INFO, "give up current mining block due to better received block",
					logging.LogFormat{
						"height":   pocTemplate.Height,
						"previous": pocTemplate.Previous,
					})
				return nil, uuid.Nil, errQuitSolveBlock
			}
			// finalize best_collector
			if bestCollector != uuid.Nil && time.Now().Add(allowAhead).Unix() >= int64(bestSlot)*pocSlot {
				break try
			}
		}
	}

	m.superior.RemoveTask(qualityReq.TaskID)

	if bestCollector == uuid.Nil {
		return nil, uuid.Nil, errNoValidProof
	}

	proofReq := &protocol.RequestProof{
		TaskID:    uuid.New(),
		Height:    qualityReq.Height,
		SpaceID:   bestSpaceID,
		Challenge: challenge,
		Index:     bestIndex,
	}
	logging.CPrint(logging.INFO, "submit request_proof task", logging.LogFormat{
		"height":   pocTemplate.Height,
		"job_id":   qualityReq.ID,
		"peer":     bestCollector,
		"space_id": bestSpaceID,
		"index":    bestIndex,
	})
	proofCh := m.superior.AddTask(context.Background(), bestCollector, proofReq)
	defer m.superior.RemoveTask(proofReq.TaskID)
	var protocolProof *protocol.Proof
	select {
	case <-time.After(time.Second * 5):
		return nil, uuid.Nil, errors.New("request_proof timeout")
	case newMsg := <-proofCh:
		protocolProof = newMsg.Msg.(*protocol.ReportProof).Proof
	}

	pocTemplate.Timestamp = time.Unix(int64(bestSlot)*pocSlot, 0)
	return &ProofTemplate{
		chiaQuality: bestWorkSpaceQuality,
		proof:       protocolProof.WorkSpaceProof(),
		time:        pocTemplate.Timestamp,
		quality:     bestQuality,
	}, bestCollector, nil
}

func init() {
	pocminer.AddPoCMinerBackend(pocminer.Backend{
		Typ:         TypeChiaPosMiner,
		NewPoCMiner: NewChiaPosMiner,
	})
}
