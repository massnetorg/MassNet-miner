package mock

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

type lUtxoMgr struct {
	c          *Chain
	addr2utxos map[string][]*utxo //key: staking address
	op2lutxo   map[wire.OutPoint]*utxo
}

type stakingRank struct {
	addressScriptHash []byte
	addressValue      int64
	weight            int64
	txout             *wire.TxOut
	newutxo           *utxo
}

func (m *lUtxoMgr) isStakingUtxo(u *utxo) bool {
	_, ok := m.op2lutxo[*u.prePoint]
	return ok
}

func (m *lUtxoMgr) constructRewardTxOut(blkHeight uint64, tx *wire.MsgTx) (out []*stakingRank, err error) {

	out = make([]*stakingRank, 0)

	for stakingAddr, utxos := range m.addr2utxos {
		lrank := &stakingRank{}
		for _, UTXO := range utxos {

			if UTXO.spent {
				continue
			}
			if blkHeight-UTXO.blockHeight < consensus.StakingTxRewardStart {
				continue
			}

			if UTXO.blockHeight+UTXO.maturity <= blkHeight { // expired
				continue
			}

			if lrank.txout == nil {
				addr, err := massutil.DecodeAddress(stakingAddr, &config.ChainParams)
				if err != nil {
					return nil, err
				}
				if !massutil.IsWitnessStakingAddress(addr) {
					return nil, fmt.Errorf("not staking address %s", stakingAddr)
				}
				//
				normalAddr, err := massutil.NewAddressWitnessScriptHash(addr.ScriptAddress(), &config.ChainParams)
				if err != nil {
					return nil, err
				}
				pkScript, err := txscript.PayToAddrScript(normalAddr)
				if err != nil {
					return nil, err
				}
				lrank.addressScriptHash = addr.ScriptAddress()

				redeem, _, err := newWitnessScriptAddress([]*btcec.PublicKey{UTXO.privateKey.PubKey()}, 1, &config.ChainParams)
				if err != nil {
					return nil, err
				}

				lrank.txout = &wire.TxOut{
					PkScript: pkScript,
				}

				lrank.newutxo = &utxo{
					prePoint:     &wire.OutPoint{},
					redeemScript: redeem,
					privateKey:   UTXO.privateKey,
					blockHeight:  blkHeight,
					spent:        false,
					maturity:     consensus.CoinbaseMaturity,
				}
			}
			lrank.addressValue += UTXO.value
			lrank.weight += UTXO.value * int64((UTXO.blockHeight + UTXO.maturity - blkHeight))
		}
		if lrank.txout != nil {
			out = append(out, lrank)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].addressValue > out[j].addressValue {
			return true
		} else if out[i].addressValue < out[j].addressValue {
			return false
		} else {
			if out[i].weight > out[j].weight {
				return true
			} else if out[i].weight < out[j].weight {
				return false
			} else {
				// key1 := make([]byte, 20)
				// copy(key1, p[i].Key[:])
				// key2 := make([]byte, 20)
				// copy(key2, p[j].Key[:])
				// return bytes.Compare(key1, key2) < 0
				return bytes.Compare(out[i].addressScriptHash, out[j].addressScriptHash) > 0
			}
		}
	})
	if len(out) > consensus.MaxStakingRewardNum {
		var length int
		for i := consensus.MaxStakingRewardNum; i > 0; i-- {
			if out[i].addressValue == out[i-1].addressValue && out[i].weight == out[i-1].weight {
				continue
			}
			length = i
			break
		}
		out = out[:length]
	}
	return
}

func (m *lUtxoMgr) constructStakingTx(blkHeight uint64, blkIndex int) (mtx *wire.MsgTx, fee int64, err error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	if len(m.c.owner2utxos) == 0 {
		return nil, 0, nil
	}
	if r.Intn(len(m.c.owner2utxos)+int(time.Now().UnixNano()))%2 == 0 {
		return nil, 0, nil
	}
	var totalIn, totalOut int64
	var owners []string
	for owner := range m.c.owner2utxos {
		owners = append(owners, owner)
	}

	cur := r.Intn(len(owners))
	stop := cur + len(owners) - 1
	sufficient := false
	inputs := make([]TxInput, 0)
	chosen := make([]*utxo, 0)
	for ; cur <= stop; cur++ {
		i := cur % len(owners)
		utxos := m.c.owner2utxos[owners[i]]

		for _, utxo := range utxos {

			if utxo.spent {
				continue
			}

			if m.c.gUtxoMgr.isBindingUtxo(utxo) || m.isStakingUtxo(utxo) {
				// NOTE: Staking/Binding UTXO is available for staking input in real rules
				// staking/binding utxos are not allowed in this mock
				continue
			}
			if utxo.blockHeight+utxo.maturity > blkHeight {
				continue
			}

			// create input
			inputs = append(inputs, TxInput{
				Height:     utxo.blockHeight,
				OutPoint:   *utxo.prePoint,
				PrivateKey: utxo.privateKey,
				Redeem:     utxo.redeemScript,
			})

			totalIn += utxo.value
			utxo.spent = true
			chosen = append(chosen, utxo)

			if uint64(totalIn*95/100) >= consensus.MinStakingValue {
				sufficient = true
				break
			}
		}
		if sufficient {
			break
		}
	}
	if sufficient {
		outputs := make([]TxOutput, 0)
		// choose a random payment address
		addr, key, err := randomAddress()
		if err != nil {
			return nil, 0, err
		}
		addr, err = massutil.NewAddressStakingScriptHash(addr.ScriptAddress(), &config.ChainParams)
		// create staking output
		output := TxOutput{
			Value: totalIn * 95 / 100, // 5 % of miner fee
			To:    addr,
			Key:   key,
		}
		outputs = append(outputs, output)
		// construct full tx
		mtx, err = m.createStakingTx(blkHeight, inputs, outputs, blkIndex)
		if err != nil {
			return nil, 0, err
		}
		if output.Value == 0 {
			return nil, 0, fmt.Errorf("staking output value is 0")
		}
		totalOut = output.Value
	} else {
		for _, utxo := range chosen {
			utxo.spent = false
		}
	}
	fee = totalIn - totalOut
	return
}

func (m *lUtxoMgr) createStakingTx(blockHeight uint64, ins []TxInput, outs []TxOutput, blockIndex int) (*wire.MsgTx, error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	frozenPeriod := uint64(r.Intn(int(m.c.opt.FrozenPeriodRange[1]-m.c.opt.FrozenPeriodRange[0]))) + uint64(m.c.opt.FrozenPeriodRange[0])

	tx := &wire.MsgTx{
		Version:  wire.TxVersion,
		TxIn:     make([]*wire.TxIn, len(ins), len(ins)),
		TxOut:    make([]*wire.TxOut, len(outs), len(outs)),
		LockTime: 0,
		Payload:  make([]byte, 0, 0),
	}

	// fill tx txIn
	for i := 0; i < len(ins); i++ {
		in := ins[i]
		txIn := wire.NewTxIn(&in.OutPoint, nil)
		tx.TxIn[i] = txIn
	}

	// fill tx txOut
	for i := 0; i < len(outs); i++ {
		out := outs[i]
		pkScript, err := txscript.PayToStakingAddrScript(out.To, frozenPeriod)
		if err != nil {
			return nil, err
		}
		txOut := wire.NewTxOut(out.Value, pkScript)
		tx.TxOut[i] = txOut
	}

	// sign tx
	err := m.c.signWitnessTx(tx)
	if err != nil {
		return nil, err
	}

	// fill chain utxo set
	txHash := tx.TxHash()
	for i, txOut := range tx.TxOut {
		out := outs[i]
		redeem, _, err := newWitnessScriptAddress([]*btcec.PublicKey{out.Key.PubKey()}, 1, &config.ChainParams)
		if err != nil {
			return nil, err
		}
		txo := &utxo{
			prePoint: &wire.OutPoint{
				Hash:  txHash,
				Index: uint32(i),
			},
			blockIndex:   blockIndex,
			txIndex:      uint32(i),
			redeemScript: redeem,
			privateKey:   out.Key,
			value:        txOut.Value,
			spent:        false,
			blockHeight:  blockHeight,
			maturity:     frozenPeriod + 1,
			isStaking:    true,
		}

		owner := out.To.EncodeAddress()
		m.c.owner2utxos[owner] = append(m.c.owner2utxos[owner], txo)

		m.op2lutxo[*txo.prePoint] = txo
		m.addr2utxos[owner] = append(m.addr2utxos[owner], txo)
		m.c.utxos[blockHeight] = append(m.c.utxos[blockHeight], txo)
	}

	return tx, nil
}
