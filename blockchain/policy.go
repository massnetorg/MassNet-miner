package blockchain

import (
	"time"

	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	// maxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	maxStandardTxSize = 100000

	// maxStandardWitnessSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a 15-of-15 CHECKMULTISIG pay-to-witness-script-hash with
	// compressed keys.
	//
	// The form of the overall script is: <15 signatures>
	// [OP_15 <15 pubkeys> OP_15 OP_CHECKMULTISIG]
	//
	// For the p2wsh script portion, each of the 15 compressed pubkeys are
	// 33 bytes (plus one for the OP_DATA_33 opcode), and the thus it totals
	// to (15*34)+3 = 513 bytes.  Next, each of the 15 signatures is a max
	// of 73 bytes (plus one for the OP_DATA_73 opcode).  Also, there is one
	// extra byte for the initial extra OP_0 push and 3 bytes for the
	// OP_PUSHDATA2 needed to specify the 513 bytes for the script push.
	// That brings the total to (15*74)+513 = 1623.  This value also
	// adds a few extra bytes to provide a little buffer.
	// 15*74 + (15*34 + 3) + 27 = 1650
	maxStandardWitnessSize = 1650
)

// CalcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func CalcMinRequiredTxRelayFee(serializedSize int64, minTxRelayFee massutil.Amount) (massutil.Amount, error) {
	// Calculate the minimum fee for a transaction to be allowed into the
	// txPool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee). minTxRelayFee is in Maxwell/kB so
	// multiply by serializedSize (which is in bytes) and divide by 1000 to get
	// minimum Maxwells.
	u, err := minTxRelayFee.Value().MulInt(serializedSize)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	required, err := u.DivInt(1000)
	if err != nil {
		return massutil.ZeroAmount(), err
	}

	if required.IsZero() && !minTxRelayFee.IsZero() {
		return minTxRelayFee, nil
	}

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	amt, err := massutil.NewAmount(required)
	if err == massutil.ErrMaxAmount {
		return massutil.MaxAmount(), nil
	}
	return amt, err
}

// calcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func calcPriority(tx *massutil.Tx, inputValueAge *safetype.Uint128) float64 {
	// In order to encourage spending multiple old unspent transaction
	// outputs thereby reducing the total set, don't count the constant
	// overhead for each input as well as enough bytes of the signature
	// script to cover a pay-to-script-hash redemption with a compressed
	// pubkey.  This makes additional inputs free by boosting the priority
	// of the transaction accordingly.  No more incentive is given to avoid
	// encouraging gaming future transactions through the use of junk
	// outputs.  This is the same logic used in the reference
	// implementation.
	//
	// The constant overhead for a txin is 44 bytes since the previous
	// outpoint is 36 bytes + 8 bytes for the sequence
	//
	// A compressed pubkey pay-to-witness-script-hash redemption with a maximum len
	// signature is of the form:
	// [OP_DATA_72 <72-byte sig> + OP_1 OP_DATA_33 <33 byte compresed pubkey> + OP_1 + OP_CHECKMULTISIGVERIFY}]
	//
	// Thus 1 + 72 + 1 + 1 + 33 + 1 + 1 = 110
	overhead := 0
	for _, txIn := range tx.MsgTx().TxIn {
		// Max inputs + size can't possibly overflow here.
		overhead += 44 + minInt(110, txIn.Witness.PlainSize())
	}

	serializedTxSize := tx.MsgTx().PlainSize()
	if overhead >= serializedTxSize {
		return 0.0
	}
	return inputValueAge.Float64() / float64(serializedTxSize-overhead)
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(tx *massutil.Tx, txStore TxStore, nextBlockHeight uint64) (*safetype.Uint128, massutil.Amount, error) {

	totalInputAge := safetype.NewUint128()
	totalInputValue := massutil.ZeroAmount()
	var err error
	for _, txIn := range tx.MsgTx().TxIn {
		// Don't attempt to accumulate the total input age if the txIn
		// in question doesn't exist.
		if txData, exists := txStore[txIn.PreviousOutPoint.Hash]; exists && txData.Tx != nil {
			// Inputs with dependencies currently in the mempool
			// have their block height set to a special constant.
			// Their input age should computed as zero since their
			// parent hasn't made it into a block yet.
			delta := uint64(0)
			if txData.BlockHeight != mempoolHeight {
				delta = nextBlockHeight - txData.BlockHeight
			}

			// Sum the input value times age.
			originTxOut := txData.Tx.MsgTx().TxOut[txIn.PreviousOutPoint.Index]

			totalInputValue, err = totalInputValue.AddInt(originTxOut.Value)
			if err != nil {
				return nil, massutil.ZeroAmount(), err
			}

			inputAge, err := safetype.NewUint128FromUint(delta).MulInt(originTxOut.Value)
			if err != nil {
				return nil, massutil.ZeroAmount(), err
			}
			totalInputAge, err = totalInputAge.Add(inputAge)
			if err != nil {
				return nil, massutil.ZeroAmount(), err
			}
		}
	}

	return totalInputAge, totalInputValue, nil
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input is one that
// that consumes the expected number of elements from the stack and that number
// is the same as the output script pushes.  This help prevent resource
// exhaustion attacks by "creative" use of scripts that are super expensive to
// process like OP_DUP OP_CHECKSIG OP_DROP repeated a large number of times
// followed by a final OP_TRUE.
func checkInputsStandard(tx *massutil.Tx, txStore TxStore) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	for i, txIn := range tx.MsgTx().TxIn {
		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		prevOut := txIn.PreviousOutPoint
		originTx := txStore[prevOut.Hash].Tx.MsgTx()
		originPkScript := originTx.TxOut[prevOut.Index].PkScript
		var wit = txIn.Witness
		if len(wit) != 2 {
			logging.CPrint(logging.ERROR, "Invalid witness length",
				logging.LogFormat{"transaction": tx.Hash(), "index": i, "previousOutPoint": txIn.PreviousOutPoint,
					"input witness length": len(wit), "required witness length": 2})
			return ErrWitnessLength
		}
		// Calculate stats for the script pair.
		scriptInfo, err := txscript.CalcScriptInfo(originPkScript, txIn.Witness)
		if err != nil {
			logging.CPrint(logging.ERROR, "transaction input script parse failure",
				logging.LogFormat{"index": i, "err": err})
			return ErrParseInputScript
		}

		// A negative value for expected inputs indicates the script is
		// non-standard in some way.
		if scriptInfo.ExpectedInputs < 0 {
			logging.CPrint(logging.ERROR, "transaction expected input less than 0",
				logging.LogFormat{"inputs": i, "ExpectedInputs": scriptInfo.ExpectedInputs})
			return ErrExpectedSignInput
		}

		// The script pair is non-standard if the number of available
		// inputs does not match the number of expected inputs.
		//if scriptInfo.NumInputs != scriptInfo.ExpectedInputs
		if scriptInfo.NumInputs != scriptInfo.ExpectedInputs {
			logging.CPrint(logging.ERROR, "transaction expected input is not equal to reference output script provides",
				logging.LogFormat{"inputs": i, "ExpectedInputs": scriptInfo.ExpectedInputs, "numInputs": scriptInfo.NumInputs})
			return ErrExpectedSignInput
		}
	}

	return nil
}

func checkParsePkScript(tx *massutil.Tx, txStore TxStore) (err error) {
	checkedBinding := false
	for i, txOut := range tx.TxOut() {
		psi := tx.GetPkScriptInfo(i)
		txOutClass := txscript.ScriptClass(psi.Class)

		switch txOutClass {
		case txscript.StakingScriptHashTy:
			if !wire.IsValidStakingValue(txOut.Value) {
				logging.CPrint(logging.ERROR, "lock value is invalid", logging.LogFormat{"value": txOut.Value})
				return ErrInvalidStakingTxValue
			}
			if !wire.IsValidFrozenPeriod(psi.FrozenPeriod) {
				logging.CPrint(logging.ERROR, "lock frozen period is invalid", logging.LogFormat{"frozen_period": psi.FrozenPeriod})
				return ErrInvalidFrozenPeriod
			}

		case txscript.BindingScriptHashTy:
			if !checkedBinding {
				if !IsCoinBaseTx(tx.MsgTx()) {
					for j, txIn := range tx.MsgTx().TxIn {
						txInHash := txIn.PreviousOutPoint.Hash
						txInIndex := txIn.PreviousOutPoint.Index

						if txStore[txInHash].Tx == nil {
							return ErrBindingInputMissing
						}
						// txStore has been checked, so do not perform that check again
						originTx := txStore[txInHash].Tx.MsgTx()
						prePKScript := originTx.TxOut[txInIndex].PkScript
						txInClass := txscript.GetScriptClass(prePKScript)
						if txInClass == txscript.BindingScriptHashTy {
							// cache[txscript.BindingScriptHashTy] = true
							logging.CPrint(logging.ERROR, "input and output of tx are all binding tx",
								logging.LogFormat{"txInIndex": j})
							return ErrStandardBindingTx
						}
					}
				}
				checkedBinding = true
			}
		case txscript.NonStandardTy,
			txscript.MultiSigTy:
			logging.CPrint(logging.ERROR, "non-standard script form",
				logging.LogFormat{"script type": txOutClass})
			return ErrNonStandardType
		}
	}
	return nil
}

// checkPkScriptStandard performs a series of checks on a transaction ouput
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
func checkPkScriptStandard(txOut *wire.TxOut, msgTx *wire.MsgTx,
	cache map[txscript.ScriptClass]bool, txStore TxStore) (txscript.ScriptClass, error) {
	txOutClass, pops := txscript.GetScriptInfo(txOut.PkScript)
	switch txOutClass {
	case txscript.StakingScriptHashTy:
		frozenPeriod, _, err := txscript.GetParsedOpcode(pops, txscript.StakingScriptHashTy)
		if err != nil {
			return txOutClass, err
		}
		if !wire.IsValidStakingValue(txOut.Value) {
			logging.CPrint(logging.ERROR, "lock value is invalid", logging.LogFormat{"value": txOut.Value})
			return txOutClass, ErrInvalidStakingTxValue
		}
		if !wire.IsValidFrozenPeriod(frozenPeriod) {
			logging.CPrint(logging.ERROR, "lock frozen period is invalid", logging.LogFormat{"frozen_period": frozenPeriod})
			return txOutClass, ErrInvalidFrozenPeriod
		}

	case txscript.BindingScriptHashTy:
		_, checked := cache[txscript.BindingScriptHashTy]
		if !checked {
			if !IsCoinBaseTx(msgTx) {
				for j, txIn := range msgTx.TxIn {
					txInHash := txIn.PreviousOutPoint.Hash
					txInIndex := txIn.PreviousOutPoint.Index

					if txStore[txInHash].Tx == nil {
						return txOutClass, ErrBindingInputMissing
					}
					// txStore has been checked, so do not perform that check again
					originTx := txStore[txInHash].Tx.MsgTx()
					prePKScript := originTx.TxOut[txInIndex].PkScript
					txInClass := txscript.GetScriptClass(prePKScript)
					if txInClass == txscript.BindingScriptHashTy {
						// cache[txscript.BindingScriptHashTy] = true
						logging.CPrint(logging.ERROR, "input and output of tx are all binding tx",
							logging.LogFormat{"txInIndex": j})
						return txOutClass, ErrStandardBindingTx
					}
				}
			}
			// binding script is not allowed to be tx input here
			cache[txscript.BindingScriptHashTy] = false
		}

	case txscript.NonStandardTy,
		txscript.MultiSigTy: // multi-signature is not supported
		logging.CPrint(logging.ERROR, "non-standard script form",
			logging.LogFormat{"script type": txOutClass})
		return txOutClass, ErrNonStandardType
	}
	return txOutClass, nil
}

// isDust returns whether or not the passed transaction output amount is
// considered dust or not based on the passed minimum transaction relay fee.
// Dust is defined in terms of the minimum transaction relay fee.  In
// particular, if the cost to the network to spend coins is more than 1/3 of the
// minimum transaction relay fee, it is considered dust.
func isDust(txOut *wire.TxOut, minRelayTxFee massutil.Amount) (bool, error) {
	// Unspendable outputs are considered dust.
	if txscript.IsUnspendable(txOut.PkScript) {
		return true, nil
	}

	// The total serialized size consists of the output and the associated
	// input script to redeem it.  Since there is no input script
	// to redeem it yet, use the minimum size of a typical input script.
	//
	// Pay-to-witness-script-hash bytes breakdown:

	//  Input with compressed pubkey (154 bytes):
	//   36 prev outpoint, 8 sequence, 110 witness stack [1 OP_DATA_72, 72 sig,
	//	OP_1, 1 OP_DATA_33, OP_1 33 compressed pubkey, OP_1, OP_CHECKMULTISIGVERIFY]

	//
	// The most common scripts are 1-1 pay-to-script-hash, and as per the above
	// breakdown, the minimum size of a p2wsh input script is 154 bytes.  So
	// that figure is used.

	totalSize := txOut.PlainSize() + 154

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum free transaction relay fee.
	// minFreeTxRelayFee is in Maxwell/KB, so multiply by 1000 to
	// convert to bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum free transaction relay
	// fee of 1000, this equates to values less than 546 maxwell being
	// considered dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.
	v, err := safetype.NewUint128FromInt(txOut.Value)
	if err != nil {
		return false, err
	}
	v, err = v.MulInt(1000) // txOut.Value*1000
	if err != nil {
		// overflow is impossible
		return false, err
	}
	v, err = v.DivInt(3 * int64(totalSize)) // txOut.Value*1000/(3*int64(totalSize))
	if err != nil {
		return false, err
	}
	return v.Lt(minRelayTxFee.Value()), nil
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func checkTransactionStandard(tx *massutil.Tx, height uint64, minRelayTxFee massutil.Amount, txStore TxStore) error {
	// The transaction must be a currently supported version.
	msgTx := tx.MsgTx()
	if msgTx.Version > wire.TxVersion || msgTx.Version < 1 {
		logging.CPrint(logging.ERROR, "transaction version is invalid",
			logging.LogFormat{"txVersion": msgTx.Version})
		return ErrInvalidTxVersion
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	if !IsFinalizedTransaction(tx, height, time.Now()) {
		return ErrUnfinalizedTx
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.

	serializedLen := msgTx.PlainSize()
	if serializedLen > maxStandardTxSize {
		logging.CPrint(logging.ERROR, "transaction size is larger than max allowed size",
			logging.LogFormat{"serializedLen": serializedLen, "maxStandardTxSize": maxStandardTxSize})
		return ErrNonStandardTxSize
	}

	//txWeight := blockchain.GetTransactionWeight(tx)
	//if txWeight > maxStandardTxWeight {
	//	str := fmt.Sprintf("weight of transaction %v is larger than max "+
	//		"allowed weight of %v", txWeight, maxStandardTxWeight)
	//	return txRuleError(wire.RejectNonstandard, str)
	//}
	for i, txIn := range msgTx.TxIn {
		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardWitnessSize for more details.
		var wit [][]byte
		wit = txIn.Witness
		if len(wit) != 2 {
			logging.CPrint(logging.ERROR, "Invalid witness length",
				logging.LogFormat{"transaction": tx.Hash(), "index": i, "previousOutPoint": txIn.PreviousOutPoint,
					"input witness length": len(wit), "required witness length": 2})
			return ErrWitnessLength
		}
		witnessLen := txIn.Witness.PlainSize()
		if witnessLen > maxStandardWitnessSize {
			logging.CPrint(logging.ERROR, "transaction input witness size is large than max allowed size",
				logging.LogFormat{"index": i, "witnessLen": witnessLen, "maxStandardWitnessSize": maxStandardWitnessSize})
			return ErrWitnessSize
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !txscript.IsPushOnlyScript(txIn.Witness[0]) {
			logging.CPrint(logging.ERROR, "transaction input witness is not push only",
				logging.LogFormat{"index": i})
			return ErrSignaturePushOnly
		}
	}

	err := checkParsePkScript(tx, txStore)
	if err != nil {
		// Attempt to extract a reject code from the error so
		// it can be retained.  When not possible, fall back to
		// a non standard error.
		logging.CPrint(logging.ERROR, "checkParsePkScript error", logging.LogFormat{"tx": tx.Hash(), "err": err})
		return err
	}
	// None of the output public key scripts can be a non-standard script or
	// be "dust" (except when the script is a null data script).
	numNullDataOutputs := 0
	// containsBindingTxIn := make(map[txscript.ScriptClass]bool)
	for i, txOut := range tx.TxOut() {
		// scriptClass, err := checkPkScriptStandard(txOut, msgTx, containsBindingTxIn, txStore)
		// if err != nil {
		// 	// Attempt to extract a reject code from the error so
		// 	// it can be retained.  When not possible, fall back to
		// 	// a non standard error.
		// 	logging.CPrint(logging.ERROR, "checkPkScriptStandard error",
		// 		logging.LogFormat{"index": i, "err": err})
		// 	return err
		// }

		// Accumulate the number of outputs which only carry data.  For
		// all other script types, ensure the output value is not
		// "dust".
		if tx.GetPkScriptInfo(i).Class == byte(txscript.NullDataTy) {
			numNullDataOutputs++
		} else {
			isDust, err := isDust(txOut, minRelayTxFee)
			if err != nil {
				logging.CPrint(logging.ERROR, "arithmetic error",
					logging.LogFormat{"output": i, "value": txOut.Value, "err": err})
				return err
			}
			if isDust {
				logging.CPrint(logging.ERROR, "transaction output payment is dust",
					logging.LogFormat{"output": i, "value": txOut.Value})
				return ErrDust
			}
		}
	}

	// A standard transaction must not have more than one output script that
	// only carries data.
	if numNullDataOutputs > 1 {
		logging.CPrint(logging.ERROR, "more than one transaction output in a nulldata script",
			logging.LogFormat{"numNullDataOutputs": numNullDataOutputs})
		return ErrNuLLDataScript
	}

	return nil
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func GetMaxStandardTxSize() int {
	return maxStandardTxSize
}
