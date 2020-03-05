package mock

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

type TxInput struct {
	Height              uint64            // block height
	OutPoint            wire.OutPoint     // outPoint
	PrivateKey          *btcec.PrivateKey // privateKey to unlock this output
	Redeem              []byte            // redeem script
	IsStakingWithdrawal bool
	UtxoMaturity        uint64 // staking

}

type TxOutput struct {
	Value int64            // transfer value
	To    massutil.Address // transfer to which address
	Key   *btcec.PrivateKey
}

func (c *Chain) constructTxs1(blkHeight uint64) ([]*wire.MsgTx, map[TxType]int, int64, error) {
	txs := make([]*wire.MsgTx, 0)
	stat := make(map[TxType]int)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	var totalFee int64

	for len(txs) < c.opt.MinNormalTxPerBlock {
		mtx, fee, spentBroUtxo, noUtxo, err := c.constructNormalTx(blkHeight, len(txs)+1, false)
		if err != nil {
			return nil, nil, 0, err
		}
		if noUtxo {
			// no enough utxo
			break
		}
		if mtx == nil {
			continue
		}
		txs = append(txs, mtx)
		c.txIndex[mtx.TxHash()] = &txLoc{
			blockHeight: blkHeight,
			blockIndex:  len(txs), // 1 coinbase
			tx:          mtx,
		}
		stat[TxNormal] += 1
		if spentBroUtxo {
			stat[TxNormalSpentBroUtxo] += 1
		}
		totalFee += fee
	}

	re := c.randEmitter.Get()
	sub := c.opt.TxPerBlock - c.opt.MinNormalTxPerBlock
	max := 0
	if sub > 0 {
		max = (sub+re)%sub + 1
	}
	for i := 0; i < max; i++ {
		tp := Probability[r.Intn(len(Probability))]
		switch tp {
		case TxNormal, Txwithdrawal:
			mtx, fee, spentBroUtxo, noUtxo, err := c.constructNormalTx(blkHeight, len(txs)+1, tp == Txwithdrawal)
			if err != nil {
				return nil, nil, 0, err
			}
			if mtx == nil || noUtxo {
				break
			}
			txs = append(txs, mtx)
			c.txIndex[mtx.TxHash()] = &txLoc{
				blockHeight: blkHeight,
				blockIndex:  len(txs), // count coinbase, so do not minus one
				tx:          mtx,
			}
			stat[tp] += 1
			if spentBroUtxo {
				stat[TxNormalSpentBroUtxo] += 1
			}
			totalFee += fee
		case TxStaking:
			mtx, fee, err := c.lUtxoMgr.constructStakingTx(blkHeight, len(txs)+1)
			if err != nil {
				return nil, nil, 0, err
			}
			if mtx == nil {
				break
			}
			txs = append(txs, mtx)
			c.txIndex[mtx.TxHash()] = &txLoc{
				blockHeight: blkHeight,
				blockIndex:  len(txs), // count coinbase, so do not minus one
				tx:          mtx,
			}
			stat[tp] += 1
			totalFee += fee
		case TxBinding:
			mtx, fee, err := c.gUtxoMgr.constructBindingTx(blkHeight, len(txs)+1)
			if err != nil {
				return nil, nil, 0, err
			}
			if mtx == nil {
				break
			}
			txs = append(txs, mtx)
			c.txIndex[mtx.TxHash()] = &txLoc{
				blockHeight: blkHeight,
				blockIndex:  len(txs), // count coinbase, so do not minus one
				tx:          mtx,
			}
			stat[tp] += 1
			totalFee += fee
		default:
			return nil, nil, 0, fmt.Errorf("unexpected error")
		}
	}

	return txs, stat, totalFee, nil
}

func (c *Chain) constructNormalTx(blkHeight uint64, blkIndex int, withdrawal bool) (mtx *wire.MsgTx,
	fee int64, spentBroUtxo, noUtxo bool, err error) {
	if len(c.owner2utxos) == 0 {
		return nil, 0, false, false, errors.New("insufficient uxto")
	}
	if !withdrawal && (len(c.owner2utxos)+c.randEmitter.Get())%2 == 0 {
		return nil, 0, false, false, nil
	}
	allowSpendBroUtxo := !withdrawal && (int64(blkIndex)+time.Now().UnixNano())%2 == 0
	var totalIn, totalOut int64
	num := 1
	if !c.opt.NoNormalOutputSplit {
		num = (len(c.owner2utxos) + c.randEmitter.Get()) % len(c.owner2utxos)
	}
	owners := make([]string, 0)
	for owner := range c.owner2utxos {
		owners = append(owners, owner)
	}
	owners = append(owners[num:], owners[:num]...)

	inputs := make([]TxInput, 0)
	outputs := make([]TxOutput, 0)
out:
	for _, owner := range owners {
		utxos := c.owner2utxos[owner]
		for _, utxo := range utxos {
			if len(inputs) >= num/2+1 {
				break out
			}
			if utxo.spent {
				continue
			}
			if withdrawal {
				// withdraw use binding/staking utxo
				if !c.lUtxoMgr.isStakingUtxo(utxo) && !c.gUtxoMgr.isBindingUtxo(utxo) {
					continue
				}
			} else {
				if c.lUtxoMgr.isStakingUtxo(utxo) || c.gUtxoMgr.isBindingUtxo(utxo) {
					continue
				}
			}
			if utxo.blockHeight+utxo.maturity > blkHeight { // coinbase or staking output
				continue
			}
			if utxo.blockHeight == blkHeight {
				if !allowSpendBroUtxo {
					continue
				}
				spentBroUtxo = true
			}

			// choose a random payment address
			addr, key, err := randomAddress()
			if err != nil {
				return nil, 0, false, false, err
			}
			// create input
			input := TxInput{
				Height:              utxo.blockHeight,
				OutPoint:            *utxo.prePoint,
				PrivateKey:          utxo.privateKey,
				Redeem:              utxo.redeemScript,
				IsStakingWithdrawal: c.lUtxoMgr.isStakingUtxo(utxo) && withdrawal,
				UtxoMaturity:        utxo.maturity,
			}
			inputs = append(inputs, input)
			totalIn += utxo.value

			// create output
			val1 := utxo.value
			val2 := int64(0)
			if val1 >= 10000000000 && time.Now().UnixNano()%10 < 1 && !c.opt.NoNormalOutputSplit { // must >=100 mass
				val2 = val1 / 2
				val1 = val1 - val2
			}
			output := TxOutput{
				Value: val1 * 95 / 100, // 5 % of miner fee
				To:    addr,
				Key:   key,
			}
			if output.Value == 0 {
				return nil, 0, false, false, fmt.Errorf("normal output value is 0: %d", utxo.value)
			}
			outputs = append(outputs, output)
			totalOut += output.Value

			if val2 > 0 {
				output := TxOutput{
					Value: val2 * 95 / 100, // 5 % of miner fee
					To:    addr,
					Key:   key,
				}
				outputs = append(outputs, output)
				totalOut += output.Value
			}
			utxo.spent = true
		}
	}
	if len(inputs) > 0 {
		// construct full tx
		mtx, err = c.constructTx(blkHeight, inputs, outputs, blkIndex)
		if err != nil {
			return nil, 0, false, false, err
		}
	} else {
		return nil, 0, false, true, nil
	}
	fee = totalIn - totalOut
	return
}

func (c *Chain) constructTx(blockHeight uint64, ins []TxInput, outs []TxOutput, blockIndex int) (*wire.MsgTx, error) {
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
		if in.IsStakingWithdrawal {
			txIn.Sequence = in.UtxoMaturity
		}
	}

	// fill tx txOut
	for i := 0; i < len(outs); i++ {
		out := outs[i]
		pkScript, err := txscript.PayToAddrScript(out.To)
		if err != nil {
			return nil, err
		}
		txOut := wire.NewTxOut(out.Value, pkScript)
		tx.TxOut[i] = txOut
	}

	// sign tx
	err := c.signWitnessTx(tx)
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
			maturity:     0,
		}

		owner := out.To.EncodeAddress()
		c.owner2utxos[owner] = append(c.owner2utxos[owner], txo)
		c.utxos[blockHeight] = append(c.utxos[blockHeight], txo)
	}

	return tx, nil
}

func (c *Chain) signWitnessTx(tx *wire.MsgTx) error {
	hashType := txscript.SigHashAll
	hashCache := txscript.NewTxSigHashes(tx)
	params := &config.ChainParams
	additionalPrevScripts := make(map[wire.OutPoint][]byte)
	value := make(map[wire.OutPoint]int64)

	// fill two maps
	for _, in := range tx.TxIn {
		preTxOut := c.txIndex[in.PreviousOutPoint.Hash].tx.TxOut[in.PreviousOutPoint.Index]
		additionalPrevScripts[in.PreviousOutPoint] = preTxOut.PkScript
		value[in.PreviousOutPoint] = preTxOut.Value
	}

	// fill sig to every txIn
	for i, txIn := range tx.TxIn {
		prevOutScript, _ := additionalPrevScripts[txIn.PreviousOutPoint]
		getSign := txscript.SignClosure(func(pub *btcec.PublicKey, hash []byte) (*btcec.Signature, error) {
			key, exists := pkStrToWalletKey[pkStr(pub)]
			if !exists {
				return nil, errors.New("private key not found")
			}
			signature, err := key.Sign(hash)
			if err != nil {
				return nil, errors.New("fail to generate signature")
			}
			return signature, nil
		})

		getScript := txscript.ScriptClosure(func(addr massutil.Address) ([]byte, error) {
			loc, exists := c.txIndex[txIn.PreviousOutPoint.Hash]
			if !exists {
				return nil, errors.New("fail to find outPoint tx")
			}
			if loc.tx == nil {
				return nil, errors.New("outPoint tx is nil")
			}
			// find from chain utxo set
			for _, txo := range c.utxos[loc.blockHeight] {
				if txo.blockIndex == loc.blockIndex && txo.txIndex == txIn.PreviousOutPoint.Index {
					return txo.redeemScript, nil
				}
			}
			return nil, errors.New("fail to find redeem script from chain utxo set")
		})

		// SigHashSingle inputs can only be signed if there's a
		// corresponding output. However this could be already signed,
		// so we always verify the output.
		script, err := txscript.SignTxOutputWit(params, tx, i, value[txIn.PreviousOutPoint], prevOutScript, hashCache, hashType, getSign, getScript)
		if err != nil {
			return err
		}
		txIn.Witness = script

		// Either it was already signed or we just signed it.
		// Find out if it is completely satisfied or still needs more.
		vm, err := txscript.NewEngine(prevOutScript, tx, i,
			txscript.StandardVerifyFlags, nil, hashCache, value[txIn.PreviousOutPoint])
		if err == nil {
			err = vm.Execute()
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// generate witness address (bech32 encoded)
func newWitnessScriptAddress(pubkeys []*btcec.PublicKey, nrequired int, net *config.Params) ([]byte, massutil.Address, error) {
	var addressPubKeyStructs []*massutil.AddressPubKey
	for i := 0; i < len(pubkeys); i++ {
		pubKeySerial := pubkeys[i].SerializeCompressed()
		addressPubKeyStruct, err := massutil.NewAddressPubKey(pubKeySerial, net)
		if err != nil {
			return nil, nil, err
		}
		addressPubKeyStructs = append(addressPubKeyStructs, addressPubKeyStruct)
	}

	// get redeem script
	redeemScript, err := txscript.MultiSigScript(addressPubKeyStructs, nrequired)
	if err != nil {
		return nil, nil, err
	}

	// scriptHash is witnessProgram
	scriptHash := sha256.Sum256(redeemScript)
	witAddress, err := massutil.NewAddressWitnessScriptHash(scriptHash[:], net)
	if err != nil {
		return nil, nil, err
	}

	return redeemScript, witAddress, nil
}

// func (c *Chain) constructTxs(blockHeight uint64) ([]*wire.MsgTx, error) {
// 	endHeight := blockHeight - c.maturity
// 	txs := make([]*wire.MsgTx, 0)
// 	txCount := 0
// 	// try construct from chain utxo set
// 	for height := c.lastUtxoHeight; height <= endHeight; height++ {
// 		if txCount >= c.opt.TxPerBlock {
// 			break
// 		}
// 		if set, exists := c.utxos[height]; exists {
// 			for _, txo := range set {
// 				// checks
// 				if txCount >= c.opt.TxPerBlock {
// 					break
// 				}
// 				if txo.spent {
// 					continue
// 				}
// 				// choose a random payment address
// 				addr, key, err := randomAddress()
// 				if err != nil {
// 					return nil, err
// 				}
// 				// create input
// 				txIn := TxInput{
// 					Height:     height,
// 					OutPoint:   *txo.prePoint,
// 					PrivateKey: txo.privateKey,
// 					Redeem:     txo.redeemScript,
// 				}
// 				// create output
// 				txOut := TxOutput{
// 					Value: txo.value * 95 / 100, // 5 % of miner fee
// 					To:    addr,
// 					Key:   key,
// 				}
// 				// construct full tx
// 				tx, err := c.constructTx(blockHeight, []TxInput{txIn}, []TxOutput{txOut}, txCount+1)
// 				if err != nil {
// 					return nil, err
// 				}
// 				txs = append(txs, tx)
// 				// fill chain txIndex
// 				c.txIndex[tx.TxHash()] = &txLoc{
// 					blockHeight: blockHeight,
// 					blockIndex:  len(txs), // count coinbase, so do not minus one
// 					tx:          tx,
// 				}
// 				txo.spent = true
// 				c.utxoCount--
// 				txCount++
// 			}
// 			c.lastUtxoHeight++
// 		}
// 	}
// 	return txs, nil
// }
