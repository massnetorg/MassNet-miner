package mock

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/pocec"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

var (
	errIncompleteCoinbasePayload = errors.New("size of coinbase payload less than block height need")
)

type CoinbasePayload struct {
	height           uint64
	numStakingReward uint32
}

func (p CoinbasePayload) NumStakingReward() uint32 {
	return p.numStakingReward
}

func (p CoinbasePayload) Bytes() []byte {
	buf := make([]byte, 12, 12)
	binary.LittleEndian.PutUint64(buf[:8], p.height)
	binary.LittleEndian.PutUint32(buf[8:12], p.numStakingReward)
	return buf
}

func (p CoinbasePayload) SetBytes(data []byte) error {
	if len(data) < 12 {
		return errIncompleteCoinbasePayload
	}
	p.height = binary.LittleEndian.Uint64(data[0:8])
	p.numStakingReward = binary.LittleEndian.Uint32(data[8:12])
	return nil
}

func standardCoinbasePayload(nextBlockHeight uint64, numStakingReward uint32) []byte {
	p := CoinbasePayload{
		height:           nextBlockHeight,
		numStakingReward: numStakingReward,
	}
	return p.Bytes()
}

func isCoinBaseTx(msgTx *wire.MsgTx) bool {
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || !prevOut.Hash.IsEqual(&wire.Hash{}) {
		return false
	}

	return true
}

func (c *Chain) retrieveCoinbase(blk *wire.MsgBlock, height uint64) error {
	cb := blk.Transactions[0]
	if !isCoinBaseTx(cb) {
		return fmt.Errorf("not coinbase : %d", blk.Header.Height)
	}

	_, addrs, _, _, err := txscript.ExtractPkScriptAddrs(cb.TxOut[0].PkScript, &config.ChainParams)
	if err != nil {
		return err
	}

	key, ok := scriptToWalletKey[string(addrs[0].ScriptAddress())]
	if !ok {
		return fmt.Errorf("key not found: %d, %s, %d", blk.Header.Height, addrs[0].EncodeAddress(), cb.TxOut[0].Value)
	}

	redeem, addr, err := newWitnessScriptAddress([]*btcec.PublicKey{key.PubKey()}, 1, &config.ChainParams)
	if err != nil {
		return err
	}

	if blk.Header.Height != height {
		return fmt.Errorf("invalid height %d:%d", blk.Header.Height, height)
	}

	txo := &utxo{
		prePoint: &wire.OutPoint{
			Hash:  cb.TxHash(),
			Index: 0,
		},
		blockIndex:   0,
		txIndex:      0,
		redeemScript: redeem,
		privateKey:   key,
		blockHeight:  blk.Header.Height,
		spent:        false,
		value:        cb.TxOut[0].Value,
		maturity:     consensus.CoinbaseMaturity,
	}
	c.utxos[blk.Header.Height] = append(c.utxos[blk.Header.Height], txo)
	c.owner2utxos[addr.EncodeAddress()] = append(c.owner2utxos[addr.EncodeAddress()], txo)
	c.txIndex[cb.TxHash()] = &txLoc{
		blockHeight: blk.Header.Height,
		blockIndex:  0,
		tx:          cb,
	}
	return nil
}

func (c *Chain) createCoinbaseTx(blockHeight uint64, pocpk *pocec.PublicKey) (*massutil.Tx, wire.Hash, error) {
	tx := wire.NewMsgTx()

	// add txIn
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&wire.Hash{},
			wire.MaxPrevOutIndex),
		Sequence: wire.MaxTxInSequenceNum,
	})

	ranks, err := c.lUtxoMgr.constructRewardTxOut(blockHeight, tx)
	if err != nil {
		return nil, wire.Hash{}, err
	}

	// calc reward
	minerReward, superReward, err := c.gUtxoMgr.calcCoinbaseOut(blockHeight, tx, pocpk, len(ranks), c.opt.BitLength)
	if err != nil {
		return nil, wire.Hash{}, err
	}

	txHash := tx.TxHash()
	// staking reward
	change := massutil.ZeroAmount()
	if len(ranks) > consensus.MaxStakingRewardNum {
		return nil, wire.Hash{}, errors.New("too many ranks")
	}
	if len(ranks) > 0 {
		totalStakingValue := massutil.ZeroAmount()
		for _, rank := range ranks {
			totalStakingValue, err = totalStakingValue.AddInt(rank.addressValue)
			if err != nil {
				return nil, wire.Hash{}, err
			}
		}

		actualTotalReward := massutil.ZeroAmount()
		utxos := make([]*utxo, 0)
		flag := false
		for i, rank := range ranks {
			superNodeValue, err := calcSuperNodeReward(superReward.Value(), totalStakingValue.Value(), rank.addressValue)
			if err != nil {
				return nil, wire.Hash{}, err
			}
			if superNodeValue.IsZero() {
				flag = true
				continue
			}
			if flag { // debug code
				return nil, wire.Hash{}, errors.New("unexpected error")
			}
			actualTotalReward, err = actualTotalReward.Add(superNodeValue)
			if err != nil {
				return nil, wire.Hash{}, err
			}
			tx.AddTxOut(&wire.TxOut{
				Value:    superNodeValue.IntValue(),
				PkScript: rank.txout.PkScript,
			})
			rank.newutxo.prePoint.Hash = txHash
			rank.newutxo.prePoint.Index = uint32(i)
			rank.newutxo.blockIndex = 0
			rank.newutxo.value = superNodeValue.IntValue()
			rank.newutxo.txIndex = uint32(i)
			_, addr, err := newWitnessScriptAddress([]*btcec.PublicKey{rank.newutxo.privateKey.PubKey()}, 1, &config.ChainParams)
			if err != nil {
				return nil, wire.Hash{}, err
			}
			c.owner2utxos[addr.EncodeAddress()] = append(c.owner2utxos[addr.EncodeAddress()], rank.newutxo)
			utxos = append(utxos, rank.newutxo)
		}
		c.utxos[blockHeight] = utxos
		change, err = superReward.Sub(actualTotalReward)
		if err != nil {
			return nil, wire.Hash{}, err
		}
	}

	numStakingReward := len(tx.TxOut)
	tx.SetPayload(standardCoinbasePayload(blockHeight, uint32(numStakingReward)))

	// create outputs for miner
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := r.Intn(len(pkStrToWalletKey))
	cycle := 0
	for _, key := range pkStrToWalletKey {
		if cycle != n {
			cycle++
			continue
		}
		redeem, addr, err := newWitnessScriptAddress([]*btcec.PublicKey{key.PubKey()}, 1, &config.ChainParams)
		if err != nil {
			return nil, wire.Hash{}, err
		}
		pkScriptAddr, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, wire.Hash{}, err
		}
		tx.AddTxOut(&wire.TxOut{
			Value:    minerReward.IntValue() + change.IntValue(),
			PkScript: pkScriptAddr,
		})
		txo := &utxo{
			prePoint: &wire.OutPoint{
				Hash:  txHash,
				Index: uint32(numStakingReward),
			},
			blockIndex:   0,
			txIndex:      uint32(numStakingReward),
			redeemScript: redeem,
			privateKey:   key,
			blockHeight:  blockHeight,
			spent:        false,
			value:        minerReward.IntValue() + change.IntValue(),
			maturity:     consensus.CoinbaseMaturity,
		}
		c.utxos[blockHeight] = append(c.utxos[blockHeight], txo)
		c.owner2utxos[addr.EncodeAddress()] = append(c.owner2utxos[addr.EncodeAddress()], txo)
		break
	}

	// fill chain txIndex
	c.txIndex[txHash] = &txLoc{
		blockHeight: blockHeight,
		blockIndex:  0,
		tx:          tx,
	}

	if len(c.utxos[blockHeight]) != len(tx.TxOut) {
		return nil, wire.Hash{}, errors.New("utxo error")
	}

	return massutil.NewTx(tx), txHash, nil
}

func (c *Chain) reCalcCoinbaseUtxo(blk *wire.MsgBlock, oldHash wire.Hash) {
	coinbaseHash := blk.Transactions[0].TxHash()

	for _, utxo := range c.utxos[blk.Header.Height] {
		if oldHash == utxo.prePoint.Hash {
			utxo.prePoint.Hash = coinbaseHash
		}
	}

	txloc, ok := c.txIndex[oldHash]
	if ok {
		c.txIndex[coinbaseHash] = txloc
		delete(c.txIndex, oldHash)
	}
}

func calcSuperNodeReward(superNode, totalStakingValue *safetype.Uint128, value int64) (massutil.Amount, error) {
	u, err := superNode.MulInt(value)
	if err != nil {
		return massutil.ZeroAmount(), err
	}

	u, err = u.Div(totalStakingValue)
	if err != nil {
		return massutil.ZeroAmount(), err
	}

	return massutil.NewAmount(u)
}
