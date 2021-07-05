package miner

import (
	"context"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/poc/engine.v2"
	"massnet.org/mass/poc/engine.v2/pocminer"
	"massnet.org/mass/poc/engine.v2/spacekeeper"
)

const (
	TypeSyncMiner = "chiapos.sync"
)

func NewSyncMiner(args ...interface{}) (pocminer.PoCMiner, error) {
	allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses, keystore, err := parsArgs(args...)
	if err != nil {
		return nil, err
	}
	m := NewPoCMiner(TypeSyncMiner, allowSolo, chain, syncManager, sk, newBlockCh, payoutAddresses, keystore)
	m.getBestProof = m.syncGetBestProof
	return m, nil
}

func parsArgs(args ...interface{}) (allowSolo bool, chain Chain, syncManager SyncManager, sk spacekeeper.SpaceKeeper, newBlockCh chan *wire.Hash, payoutAddresses []massutil.Address, keystore *chiawallet.Keystore, err error) {
	var failure = func() (bool, Chain, SyncManager, spacekeeper.SpaceKeeper, chan *wire.Hash, []massutil.Address, *chiawallet.Keystore, error) {
		expectedTypes := []reflect.Type{reflect.TypeOf(chain), reflect.TypeOf(syncManager), reflect.TypeOf(sk), reflect.TypeOf(newBlockCh), reflect.TypeOf(payoutAddresses)}
		actualTypes := make([]reflect.Type, len(args))
		for i, arg := range args {
			actualTypes[i] = reflect.TypeOf(arg)
		}
		logging.CPrint(logging.ERROR, "invalid miner arg types", logging.LogFormat{"expected": expectedTypes, "actual": actualTypes, "err": pocminer.ErrInvalidMinerArgs})
		return false, nil, nil, nil, nil, nil, nil, pocminer.ErrInvalidMinerArgs
	}

	if len(args) != 7 {
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

	var bestChiaQualityIndex int
	var bestQuality = big.NewInt(0)
	var challenge = pocutil.Hash(pocTemplate.Challenge)
	ticker := time.NewTicker(time.Second * pocSlot / 4)
	defer ticker.Stop()

	queryStart := time.Now()
	skChiaQualities, err := m.SpaceKeeper.GetQualities(context.TODO(), engine.SFMining, challenge)
	if err != nil {
		return nil, err
	}
	queryElapsed := time.Since(queryStart)
	chiaQualities := getValidChiaQualities(skChiaQualities)
	var validCount = len(chiaQualities)
	chiaQualities = getBindingChiaQualities(chiaQualities, pocTemplate)
	var bindingCount = len(chiaQualities)
	logging.CPrint(logging.INFO, "find qualities for next block, waiting for proper slot",
		logging.LogFormat{"height": pocTemplate.Height, "binding_count": bindingCount, "valid_count": validCount,
			"total_count": len(skChiaQualities), "query_time": queryElapsed.Seconds()})
	if len(chiaQualities) == 0 {
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
				qualities, err := getMASSQualities(chiaQualities, challenge, workSlot, pocTemplate.Height)
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
						bestChiaQualityIndex = i
					}
				}

				// compare with target
				if bestQuality.Cmp(pocTemplate.GetTarget(pocTemplate.Timestamp)) > 0 {
					bestChiaQuality := chiaQualities[bestChiaQualityIndex]
					proof, err := m.SpaceKeeper.GetProof(context.TODO(), bestChiaQuality.SpaceID, challenge, bestChiaQuality.Index)
					if err != nil {
						return nil, err
					}
					return &ProofTemplate{
						chiaQuality: bestChiaQuality,
						proof:       proof,
						time:        pocTemplate.Timestamp,
						quality:     bestQuality,
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

func getValidChiaQualities(chiaQualities []*engine.WorkSpaceQuality) []*engine.WorkSpaceQuality {
	result := make([]*engine.WorkSpaceQuality, 0)
	for i := range chiaQualities {
		if chiaQualities[i].Error == nil {
			result = append(result, chiaQualities[i])
		}
	}
	return result
}

func getBindingChiaQualities(qualities []*engine.WorkSpaceQuality, template *blockchain.PoCTemplate) []*engine.WorkSpaceQuality {
	result := make([]*engine.WorkSpaceQuality, 0)
	for i := range qualities {
		if template.PassBinding(qualities[i]) {
			result = append(result, qualities[i])
		}
	}
	return result
}

func getMASSQualities(chiaQualities []*engine.WorkSpaceQuality, challenge pocutil.Hash, slot, height uint64) ([]*big.Int, error) {
	massQualities := make([]*big.Int, len(chiaQualities))
	for i, chiaQuality := range chiaQualities {
		chiaHashVal := poc.HashValChia(chiaQuality.Quality, slot, height)
		massQualities[i] = poc.GetQuality(poc.Q1FactorChia(chiaQuality.KSize), chiaHashVal)
	}
	return massQualities, nil
}

func init() {
	pocminer.AddPoCMinerBackend(pocminer.Backend{
		Typ:         TypeSyncMiner,
		NewPoCMiner: NewSyncMiner,
	})
}
