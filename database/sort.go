package database

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"massnet.org/mass/consensus"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
)

type StakingTxInfo struct {
	Value        uint64
	FrozenPeriod uint64
	BlkHeight    uint64
}

type Rank struct {
	Rank       int32
	Value      int64
	ScriptHash [sha256.Size]byte
	Weight     *safetype.Uint128
	StakingTx  []StakingTxInfo
}

type Pair struct {
	Key    [sha256.Size]byte
	Value  int64
	Weight *safetype.Uint128
}

// A slice of Pairs that implements sort.Interface to sort by Value.

type Pairs []Pair

type PairList struct {
	pairs       Pairs
	weightFirst bool
}

func (pl PairList) Swap(i, j int) { pl.pairs[i], pl.pairs[j] = pl.pairs[j], pl.pairs[i] }
func (pl PairList) Len() int      { return len(pl.pairs) }
func (pl PairList) Less(i, j int) bool {
	p := pl.pairs
	if pl.weightFirst {
		if p[i].Weight.Lt(p[j].Weight) {
			return true
		} else if p[i].Weight.Gt(p[j].Weight) {
			return false
		} else {
			if p[i].Value < p[j].Value {
				return true
			} else if p[i].Value > p[j].Value {
				return false
			} else {
				key1 := make([]byte, 32)
				copy(key1, p[i].Key[:])
				key2 := make([]byte, 32)
				copy(key2, p[j].Key[:])
				return bytes.Compare(key1, key2) < 0
			}
		}
	} else {
		if p[i].Value < p[j].Value {
			return true
		} else if p[i].Value > p[j].Value {
			return false
		} else {
			if p[i].Weight.Lt(p[j].Weight) {
				return true
			} else if p[i].Weight.Gt(p[j].Weight) {
				return false
			} else {
				key1 := make([]byte, 32)
				copy(key1, p[i].Key[:])
				key2 := make([]byte, 32)
				copy(key2, p[j].Key[:])
				return bytes.Compare(key1, key2) < 0
			}
		}
	}
}

func (p Pairs) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p Pairs) Len() int      { return len(p) }
func (p Pairs) Less(i, j int) bool {
	if p[i].Value < p[j].Value {
		return true
	} else if p[i].Value > p[j].Value {
		return false
	} else {
		if p[i].Weight.Lt(p[j].Weight) {
			return true
		} else if p[i].Weight.Gt(p[j].Weight) {
			return false
		} else {
			key1 := make([]byte, 32)
			copy(key1, p[i].Key[:])
			key2 := make([]byte, 32)
			copy(key2, p[j].Key[:])
			return bytes.Compare(key1, key2) < 0
		}
	}
	//key1 := make([]byte, 20)
	//copy(key1, p[i].Key[:])
	//key2 := make([]byte, 20)
	//copy(key2, p[j].Key[:])
	//return string(key1) < string(key2)
}

func SortMap(m map[[sha256.Size]byte][]StakingTxInfo, newestHeight uint64, isOnlyReward bool) (Pairs, error) {
	length := len(m)
	if length == 0 {
		return Pairs{}, nil
	}
	ps := make(Pairs, length)
	pl := PairList{
		pairs:       ps,
		weightFirst: newestHeight >= consensus.Massip1Activation,
	}
	i := 0

	for k, stakingTxs := range m {
		totalValue := massutil.ZeroAmount()
		totalWeight := safetype.NewUint128()
		for _, stakingTx := range stakingTxs {
			va, err := massutil.NewAmountFromUint(stakingTx.Value)
			if err != nil {
				logging.CPrint(logging.ERROR, "invalid value", logging.LogFormat{
					"value":        stakingTx.Value,
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}
			totalValue, err = totalValue.Add(va)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc total value error", logging.LogFormat{
					"value":        va.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}

			if newestHeight < stakingTx.BlkHeight+consensus.StakingTxRewardStart {
				logging.CPrint(logging.ERROR, "try to reward a staking tx before allow height", logging.LogFormat{
					"newestHeight": newestHeight,
					"blkHeight":    stakingTx.BlkHeight,
					"startHeight":  stakingTx.BlkHeight + consensus.StakingTxRewardStart,
				})
				return nil, errors.New("try to reward a staking tx before allow height")
			}

			if stakingTx.BlkHeight+stakingTx.FrozenPeriod < newestHeight {
				logging.CPrint(logging.ERROR, "expired staking tx found", logging.LogFormat{
					"newestHeight": newestHeight,
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
				})
				return nil, errors.New("expired staking tx found")
			}

			var period uint64
			if newestHeight < consensus.Massip1Activation {
				period = stakingTx.BlkHeight + stakingTx.FrozenPeriod - newestHeight + 1
			} else {
				period = stakingTx.FrozenPeriod
				if period > consensus.MaxValidPeriod {
					period = consensus.MaxValidPeriod
				}
			}
			uPeriod := safetype.NewUint128FromUint(period)
			uWeight, err := va.Value().Mul(uPeriod)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc weight error", logging.LogFormat{
					"value":        stakingTx.Value,
					"period":       uPeriod.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}

			totalWeight, err = totalWeight.Add(uWeight)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc total weight error", logging.LogFormat{
					"value":        stakingTx.Value,
					"weight":       uWeight.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}
		}
		pl.pairs[i] = Pair{k, totalValue.IntValue(), totalWeight}
		i++
	}
	sort.Stable(sort.Reverse(pl))

	if isOnlyReward && len(pl.pairs) > consensus.MaxStakingRewardNum {
		var length int
		for i := consensus.MaxStakingRewardNum; i > 0; i-- {
			if pl.pairs[i].Value == pl.pairs[i-1].Value && pl.pairs[i].Weight.Eq(pl.pairs[i-1].Weight) {
				continue
			}
			length = i
			break
		}
		return pl.pairs[:length], nil
	}

	return pl.pairs, nil
}

// A function to turn a map into a PairList, then sort and return it.
/*
func SortMapByValue(m map[[sha256.Size]byte][]StakingTxInfo, newestHeight uint64, isOnlyReward bool) (Pairs, error) {
	length := len(m)
	if length == 0 {
		return Pairs{}, nil
	}
	ps := make(Pairs, length)
	i := 0
	for k, stakingTxs := range m {
		totalValue := massutil.ZeroAmount()
		totalWeight := safetype.NewUint128()
		for _, stakingTx := range stakingTxs {
			va, err := massutil.NewAmountFromUint(stakingTx.Value)
			if err != nil {
				logging.CPrint(logging.ERROR, "invalid value", logging.LogFormat{
					"value":        stakingTx.Value,
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}
			totalValue, err = totalValue.Add(va)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc total value error", logging.LogFormat{
					"value":        va.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}

			if newestHeight < stakingTx.BlkHeight+consensus.StakingTxRewardStart {
				logging.CPrint(logging.ERROR, "try to reward a staking tx before allow height", logging.LogFormat{
					"newestHeight": newestHeight,
					"blkHeight":    stakingTx.BlkHeight,
					"startHeight":  stakingTx.BlkHeight + consensus.StakingTxRewardStart,
				})
				return nil, errors.New("try to reward a staking tx before allow height")
			}

			if stakingTx.BlkHeight+stakingTx.FrozenPeriod < newestHeight {
				logging.CPrint(logging.ERROR, "expired staking tx found", logging.LogFormat{
					"newestHeight": newestHeight,
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
				})
				return nil, errors.New("expired staking tx found")
			}

			uHeight := safetype.NewUint128FromUint(stakingTx.BlkHeight + stakingTx.FrozenPeriod - newestHeight + 1)
			uWeight, err := va.Value().Mul(uHeight)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc weight error", logging.LogFormat{
					"value":        stakingTx.Value,
					"height":       uHeight.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}

			totalWeight, err = totalWeight.Add(uWeight)
			if err != nil {
				logging.CPrint(logging.ERROR, "calc total weight error", logging.LogFormat{
					"value":        stakingTx.Value,
					"weight":       uWeight.String(),
					"blkHeight":    stakingTx.BlkHeight,
					"frozenPeriod": stakingTx.FrozenPeriod,
					"newestHeight": newestHeight,
					"isOnlyReward": isOnlyReward,
					"err":          err,
				})
				return nil, err
			}
		}
		ps[i] = Pair{k, totalValue.IntValue(), totalWeight}
		i++
	}
	sort.Stable(sort.Reverse(ps))

	if isOnlyReward && len(ps) > consensus.MaxStakingRewardNum {
		var length int
		for i := consensus.MaxStakingRewardNum; i > 0; i-- {
			if ps[i].Value == ps[i-1].Value && ps[i].Weight.Eq(ps[i-1].Weight) {
				continue
			}
			length = i
			break
		}
		return ps[:length], nil
	}

	return ps, nil
}
*/
