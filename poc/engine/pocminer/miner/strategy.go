package miner

import (
	"context"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/consensus/forks"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/engine/pocminer"
	"massnet.org/mass/poc/engine/spacekeeper"
)

const (
	TypeSyncMiner = "sync"
)

func NewSyncMiner(args ...interface{}) (pocminer.PoCMiner, error) {
	allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses, err := parsArgs(args...)
	if err != nil {
		return nil, err
	}
	m := NewPoCMiner(TypeSyncMiner, allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses)
	m.getBestProof = m.syncGetBestProof
	return m, nil
}

func parsArgs(args ...interface{}) (allowSolo bool, chain Chain, syncManager SyncManager, sk spacekeeper.SpaceKeeper, newBlockCh chan *wire.Hash, payoutAddresses []massutil.Address, err error) {
	var failure = func() (bool, Chain, SyncManager, spacekeeper.SpaceKeeper, chan *wire.Hash, []massutil.Address, error) {
		expectedTypes := []reflect.Type{reflect.TypeOf(chain), reflect.TypeOf(syncManager), reflect.TypeOf(sk), reflect.TypeOf(newBlockCh), reflect.TypeOf(payoutAddresses)}
		actualTypes := make([]reflect.Type, len(args))
		for i, arg := range args {
			actualTypes[i] = reflect.TypeOf(arg)
		}
		logging.CPrint(logging.ERROR, "invalid miner arg types", logging.LogFormat{"expected": expectedTypes, "actual": actualTypes, "err": pocminer.ErrInvalidMinerArgs})
		return false, nil, nil, nil, nil, nil, pocminer.ErrInvalidMinerArgs
	}

	if len(args) != 6 {
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
	return
}

func (m *PoCMiner) syncGetBestProof(pocTemplate *blockchain.PoCTemplate, quit chan struct{}) (*ProofTemplate, error) {
	var workSlot = uint64(pocTemplate.Timestamp.Unix()) / pocSlot

	cancelMonitor, staled, err := runStaleMonitor(m.chain, &workSlot, &pocTemplate.Previous)
	if err != nil {
		return nil, err
	}
	// cancel stale monitor
	defer cancelMonitor()

	var bestProofIndex int
	var bestQuality = big.NewInt(0)
	var challenge = pocutil.Hash(pocTemplate.Challenge)
	ticker := time.NewTicker(time.Second * pocSlot / 4)
	defer ticker.Stop()

	queryStart := time.Now()
	skProofs, err := m.SpaceKeeper.GetProofs(context.TODO(), engine.SFMining, challenge, forks.EnforceMASSIP0002(pocTemplate.Height))
	if err != nil {
		return nil, err
	}
	queryElapsed := time.Since(queryStart)
	proofs := getValidProofs(skProofs)
	var validCount = len(proofs)
	proofs = getBindingProofs(proofs, pocTemplate)
	var bindingCount = len(proofs)
	logging.CPrint(logging.INFO, "find proofs for next block, waiting for proper slot",
		logging.LogFormat{"height": pocTemplate.Height, "binding_count": bindingCount, "valid_count": validCount, "total_count": len(skProofs), "query_time": queryElapsed.Seconds()})
	if len(proofs) == 0 {
		return nil, errNoValidProof
	}

try:
	for {
		select {
		case <-quit:
			return nil, errQuitSolveBlock
		case <-ticker.C:
			logging.CPrint(logging.DEBUG, "ticker received")

			nowSlot := uint64(time.Now().Unix()) / pocSlot
			if workSlot > nowSlot+allowAhead {
				logging.CPrint(logging.DEBUG, "mining too far in the future",
					logging.LogFormat{"nowSlot": nowSlot, "workSlot": workSlot})
				continue try
			}

			// Try to solve, until workSlot reaches nowSlot+allowAhead
			for i := workSlot; i <= nowSlot+allowAhead; i++ {
				// Escape from loop when quit received.
				select {
				case <-quit:
					return nil, errQuitSolveBlock
				default:
					if staled() {
						logging.CPrint(logging.INFO, "give up current mining block due to better received block",
							logging.LogFormat{
								"height":   pocTemplate.Height,
								"previous": pocTemplate.Previous,
							})
						return nil, errQuitSolveBlock
					}
				}

				// Ensure there are valid qualities
				qualities, err := getQualities(proofs, challenge, forks.EnforceMASSIP0002(pocTemplate.Height), workSlot, pocTemplate.Height)
				if err != nil {
					return nil, err
				}
				if len(qualities) == 0 {
					continue try
				}

				// find best quality
				bestQuality.SetUint64(0)
				for i, quality := range qualities {
					if quality.Cmp(bestQuality) > 0 {
						bestQuality = quality
						bestProofIndex = i
					}
				}

				// compare with target
				if bestQuality.Cmp(pocTemplate.GetTarget(pocTemplate.Timestamp)) > 0 {
					return &ProofTemplate{
						proof:   proofs[bestProofIndex],
						time:    pocTemplate.Timestamp,
						quality: bestQuality,
					}, nil
				}

				// increase slot and header Timestamp
				atomic.AddUint64(&workSlot, 1)
				pocTemplate.Timestamp = pocTemplate.Timestamp.Add(pocSlot * time.Second)
			}
			continue try
		}
	}
}

func getValidProofs(proofs []*engine.WorkSpaceProof) []*engine.WorkSpaceProof {
	result := make([]*engine.WorkSpaceProof, 0)
	for i := range proofs {
		if proofs[i].Error == nil {
			result = append(result, proofs[i])
		}
	}
	return result
}

func getBindingProofs(proofs []*engine.WorkSpaceProof, template *blockchain.PoCTemplate) []*engine.WorkSpaceProof {
	result := make([]*engine.WorkSpaceProof, 0)
	for i := range proofs {
		if template.PassBinding(proofs[i]) {
			result = append(result, proofs[i])
		}
	}
	return result
}

func getQualities(proofs []*engine.WorkSpaceProof, challenge pocutil.Hash, filter bool, slot, height uint64) ([]*big.Int, error) {
	qualities := make([]*big.Int, len(proofs))
	for i, proof := range proofs {
		quality, err := proof.Proof.VerifiedQuality(pocutil.PubKeyHash(proof.PublicKey), challenge, filter, slot, height)
		if err != nil {
			return nil, err
		}
		qualities[i] = quality
	}
	return qualities, nil
}

func init() {
	pocminer.AddPoCMinerBackend(pocminer.Backend{
		Typ:         TypeSyncMiner,
		NewPoCMiner: NewSyncMiner,
	})
}
