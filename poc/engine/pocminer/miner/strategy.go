package miner

import (
	"context"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"

	"massnet.org/mass/blockchain"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/engine/pocminer"
	"massnet.org/mass/poc/engine/spacekeeper"
	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/wire"
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

	var printed bool
	var bestProofIndex int
	var bestQuality = big.NewInt(0)
	var challenge = pocutil.Hash(pocTemplate.Challenge)
	ticker := time.NewTicker(time.Second * pocSlot / 4)
	defer ticker.Stop()

try:
	for {
		select {
		case <-quit:
			return nil, errQuitSolveBlock
		case <-ticker.C:
			logging.CPrint(logging.DEBUG, "ticker received")

			skProofs, err := m.SpaceKeeper.GetProofs(context.TODO(), engine.SFMining, challenge)
			if err != nil {
				return nil, err
			}
			proofs := getValidProofs(skProofs)
			if !printed {
				logging.CPrint(logging.INFO, "find proof for next block, waiting for proper slot", logging.LogFormat{"height": pocTemplate.Height, "valid_count": len(proofs), "total_count": len(skProofs)})
				printed = true
			}

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
				qualities, err := getQualities(proofs, challenge, workSlot, pocTemplate.Height)
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

func getQualities(proofs []*engine.WorkSpaceProof, challenge pocutil.Hash, slot, height uint64) ([]*big.Int, error) {
	qualities := make([]*big.Int, len(proofs))
	for i, proof := range proofs {
		quality, err := proof.Proof.GetVerifiedQuality(pocutil.PubKeyHash(proof.PublicKey), challenge, slot, height)
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
