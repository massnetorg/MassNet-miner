package mock

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/poc/wallet/keystore"
	"massnet.org/mass/pocec"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	bitLengthMissing = -1

	limit24 = 0.00046
	limit26 = 0.00244
	limit28 = 0.00977
	limit30 = 0.03906
	limit32 = 0.15625
	limit34 = 0.78125
	limit36 = 3.125
	limit38 = 12.5
	limit40 = 50

	// BaseSubsidy is the starting subsidy amount for mined blocks.  This
	// value is halved every SubsidyHalvingInterval blocks.
)

var (
	baseSubsidy      = safetype.NewUint128FromUint(consensus.BaseSubsidy)
	minHalvedSubsidy = safetype.NewUint128FromUint(consensus.MinHalvedSubsidy)

	bindingRequiredMass = map[int]float64{
		24: 0.006144,
		26: 0.026624,
		28: 0.112,
		30: 0.48,
		32: 2.048,
		34: 8.704,
		36: 36.864,
		38: 152,
		40: 640,
	}
	bindingRequiredAmount = map[int]massutil.Amount{}
)

func init() {
	rand.Seed(time.Now().UnixNano())
	for k, limit := range bindingRequiredMass {
		amt, err := massutil.NewAmountFromMass(limit)
		if err != nil {
			panic(err)
		}
		bindingRequiredAmount[k] = amt
	}
}

func calcRshNum(height uint64) uint {
	t := (height-1)/consensus.SubsidyHalvingInterval + 1
	i := uint(0)
	for {
		t = t >> 1
		if t != 0 {
			i++
		} else {
			return i
		}
	}
}

func calBlockSubsidy(subsidy *safetype.Uint128, hasValidBinding, hasSuperNode bool) (
	massutil.Amount, massutil.Amount, error) {

	var err error
	temp := safetype.NewUint128()
	miner := safetype.NewUint128()
	superNode := safetype.NewUint128()

	switch {
	case !hasSuperNode && !hasValidBinding:
		// miner get 18.75%
		temp, err = subsidy.MulInt(1875)
		if err != nil {
			break
		}
		miner, err = temp.DivInt(10000)
	case !hasSuperNode && hasValidBinding:
		// miner get 81.25%
		temp, err = subsidy.MulInt(8125)
		if err != nil {
			break
		}
		miner, err = temp.DivInt(10000)
	case hasSuperNode && !hasValidBinding:
		// miner get 18.75%
		// superNode get 81.25%
		temp, err = subsidy.MulInt(1875)
		if err != nil {
			break
		}
		miner, err = temp.DivInt(10000)
		if err != nil {
			break
		}
		superNode, err = subsidy.Sub(miner)
	default:
		// hasSuperNode && hasValidBinding
		// miner get 81.25%
		// superNode get 18.75%
		temp, err = subsidy.MulInt(8125)
		if err != nil {
			break
		}
		miner, err = temp.DivInt(10000)
		if err != nil {
			break
		}
		superNode, err = subsidy.Sub(miner)
	}
	if err != nil {
		return massutil.ZeroAmount(), massutil.ZeroAmount(), err
	}

	m, err := massutil.NewAmount(miner)
	if err != nil {
		return massutil.ZeroAmount(), massutil.ZeroAmount(), err
	}
	sn, err := massutil.NewAmount(superNode)
	if err != nil {
		return massutil.ZeroAmount(), massutil.ZeroAmount(), err
	}
	return m, sn, nil
}

func CalcBlockSubsidy(height uint64, chainParams *config.Params, totalBinding massutil.Amount, numRank, bitLength int) (
	miner, superNode massutil.Amount, err error) {

	subsidy := baseSubsidy
	if chainParams.SubsidyHalvingInterval != 0 {
		subsidy = baseSubsidy.Rsh(calcRshNum(height))
		if subsidy.Lt(minHalvedSubsidy) {
			subsidy = safetype.NewUint128()
		}
	}

	hasValidBinding := false
	valueRequired, ok := bindingRequiredAmount[bitLength]
	if !ok {
		if bitLength != bitLengthMissing {
			logging.CPrint(logging.DEBUG, "invalid bitlength",
				logging.LogFormat{"bitLength": bitLength})
		}
	} else {
		if totalBinding.Cmp(valueRequired) >= 0 {
			hasValidBinding = true
		}
	}
	hasSuperNode := numRank > 0

	return calBlockSubsidy(subsidy, hasValidBinding, hasSuperNode)
}

type BindingTxOutput struct {
	Value int64            // transfer value
	To    massutil.Address // transfer to which address
	Key   *btcec.PrivateKey
	PoC   massutil.Address // poc address
}

type bindingUtxo struct {
	utxo           *utxo
	bindingAddress massutil.Address
}

type gUtxoMgr struct {
	c             *Chain
	pocAddr2utxos map[string][]*bindingUtxo // Key: poc pk address
	op2gutxo      map[wire.OutPoint]*bindingUtxo
}

func (m *gUtxoMgr) isBindingUtxo(u *utxo) bool {
	_, ok := m.op2gutxo[*u.prePoint]
	return ok
}

func (m *gUtxoMgr) calcCoinbaseOut(blkHeight uint64, coinbaseTx *wire.MsgTx, pocpk *pocec.PublicKey,
	numRank, bitLength int) (miner, superNode massutil.Amount, err error) {

	_, addr, err := keystore.NewPoCAddress(pocpk, &config.ChainParams)
	if err != nil {
		return massutil.ZeroAmount(), massutil.ZeroAmount(), err
	}

	gValue := massutil.ZeroAmount()
	var witness [][]byte

	req, ok := bindingRequiredAmount[bitLength]
	if ok {
		for _, gutxo := range m.getGuaranties(addr.EncodeAddress(), blkHeight) {
			gValue, err = gValue.AddInt(gutxo.utxo.value)
			if err != nil {
				return massutil.ZeroAmount(), massutil.ZeroAmount(), err
			}

			txIn := wire.NewTxIn(gutxo.utxo.prePoint, witness)
			coinbaseTx.AddTxIn(txIn)

			if gValue.Cmp(req) >= 0 {
				break
			}
		}
		if gValue.Cmp(req) < 0 {
			coinbaseTx.TxIn = coinbaseTx.TxIn[:1]
		}
	} else {
		logging.CPrint(logging.DEBUG, "invalid bitlength",
			logging.LogFormat{"bitLength": bitLength})
	}
	return CalcBlockSubsidy(blkHeight, &config.ChainParams, gValue, numRank, bitLength)
}

func (m *gUtxoMgr) getGuaranties(addr string, blkHeight uint64) []*bindingUtxo {
	list := make([]*bindingUtxo, 0)
	for _, gutxo := range m.pocAddr2utxos[addr] {
		if gutxo.utxo.spent {
			continue
		}
		if gutxo.utxo.blockHeight+gutxo.utxo.maturity > blkHeight { // unusable
			continue
		}
		list = append(list, gutxo)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].utxo.value > list[j].utxo.value
	})
	return list
}

func (m *gUtxoMgr) constructBindingTx(blkHeight uint64, blkIndex int) (mtx *wire.MsgTx, fee int64, err error) {
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
	gLimit, ok := bindingRequiredAmount[m.c.opt.BitLength]
	if !ok {
		return nil, 0, fmt.Errorf("unknown bitlength: %d", m.c.opt.BitLength)
	}
	for ; cur <= stop; cur++ {
		i := cur % len(owners)
		utxos := m.c.owner2utxos[owners[i]]

		for _, utxo := range utxos {
			if utxo.spent {
				continue
			}

			if m.isBindingUtxo(utxo) || m.c.lUtxoMgr.isStakingUtxo(utxo) {
				// NOTE: Staking UTXO is available for binding input in real rules
				// binding utxos are not allowed in this mock
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

			if totalIn*95/100 >= gLimit.IntValue() {
				sufficient = true
				break
			}
		}
		if sufficient {
			break
		}
	}
	if sufficient {
		outputs := make([]BindingTxOutput, 0)
		// choose a random payment address
		addr, key, err := randomAddress()
		if err != nil {
			return nil, 0, err
		}
		poc, _, err := randomPoCAddress()
		if err != nil {
			return nil, 0, err
		}
		output := BindingTxOutput{
			Value: totalIn * 95 / 100, // 5 % of miner fee
			To:    addr,
			Key:   key,
			PoC:   poc,
		}
		outputs = append(outputs, output)
		// construct full tx
		mtx, err = m.createBindingTx(blkHeight, inputs, outputs, blkIndex)
		if err != nil {
			return nil, 0, err
		}
		if output.Value == 0 {
			return nil, 0, fmt.Errorf("binding output value is 0")
		}
		totalOut += output.Value
	} else {
		for _, utxo := range chosen {
			utxo.spent = false
		}
	}
	fee = totalIn - totalOut
	if fee < 0 {
		return nil, 0, errors.New("negative fee")
	}
	return
}

func (m *gUtxoMgr) createBindingTx(blockHeight uint64, ins []TxInput, outs []BindingTxOutput, blockIndex int) (*wire.MsgTx, error) {

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
		pkScript, err := txscript.PayToBindingScriptHashScript(out.To.ScriptAddress(), out.PoC.ScriptAddress())
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
			maturity:     consensus.TransactionMaturity,
			pocAddr:      out.PoC,
		}

		owner := out.To.EncodeAddress()
		pocOwner := out.PoC.EncodeAddress()
		m.c.owner2utxos[owner] = append(m.c.owner2utxos[owner], txo)

		gutxo := &bindingUtxo{
			utxo:           txo,
			bindingAddress: out.PoC,
		}
		m.op2gutxo[*txo.prePoint] = gutxo
		m.pocAddr2utxos[pocOwner] = append(m.pocAddr2utxos[pocOwner], gutxo)
		m.c.utxos[blockHeight] = append(m.c.utxos[blockHeight], txo)
	}

	return tx, nil
}
