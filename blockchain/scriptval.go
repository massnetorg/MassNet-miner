package blockchain

import (
	"math"
	"runtime"

	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

// txValidateItem holds a transaction along with which input to validate.
type txValidateItem struct {
	txInIndex int
	txIn      *wire.TxIn
	tx        *massutil.Tx
	sigHashes *txscript.TxSigHashes
}

// txValidator provides a type which asynchronously validates transaction
// inputs.  It provides several channels for communication and a processing
// function that is intended to be in run multiple goroutines.
type txValidator struct {
	validateChan chan *txValidateItem
	quitChan     chan struct{}
	resultChan   chan error
	txStore      TxStore
	flags        txscript.ScriptFlags
	sigCache     *txscript.SigCache
	hashCache    *txscript.HashCache
}

// sendResult sends the result of a script pair validation on the internal
// result channel while respecting the quit channel.  The allows orderly
// shutdown when the validation process is aborted early due to a validation
// error in one of the other goroutines.
func (v *txValidator) sendResult(result error) {
	select {
	case v.resultChan <- result:
	case <-v.quitChan:
	}
}

// validateHandler consumes items to validate from the internal validate channel
// and returns the result of the validation on the internal result channel. It
// must be run as a goroutine.
func (v *txValidator) validateHandler() {
out:
	for {
		select {
		case txVI := <-v.validateChan:
			// Ensure the referenced input transaction is available.
			txIn := txVI.txIn
			originTxHash := &txIn.PreviousOutPoint.Hash
			originTx, exists := v.txStore[*originTxHash]
			if !exists || originTx.Err != nil || originTx.Tx == nil {
				logging.CPrint(logging.ERROR, "unable to find input transaction",
					logging.LogFormat{"input transaction ": originTxHash, "transaction": txVI.tx.Hash()})
				v.sendResult(ErrMissingTx)
				break out
			}
			originMsgTx := originTx.Tx.MsgTx()

			// Ensure the output index in the referenced transaction
			// is available.
			originTxIndex := txIn.PreviousOutPoint.Index
			if originTxIndex >= uint32(len(originMsgTx.TxOut)) {
				logging.CPrint(logging.ERROR, "out of bounds input index in referenced transaction",
					logging.LogFormat{"input index": originTxIndex, "originTx": originTxHash, "transaction": txVI.tx.Hash()})
				v.sendResult(ErrBadTxInput)
				break out
			}

			// Create a new script engine for the script pair.
			//sigScript := txIn.SignatureScript

			//check witness length
			witness := txIn.Witness
			if len(witness) != 2 {
				logging.CPrint(logging.ERROR, "Invalid witness length",
					logging.LogFormat{"transaction": txVI.tx.Hash(), "index": txVI.txInIndex, "previousOutPoint": txIn.PreviousOutPoint,
						"input witness length": len(witness), "required witness length": 2})
				v.sendResult(ErrWitnessLength)
				break out
			}

			pkScript := originMsgTx.TxOut[originTxIndex].PkScript
			inputAmount := originMsgTx.TxOut[originTxIndex].Value
			//vm, err := txscript.NewEngine(pkScript, txVI.tx.MsgTx(),
			//	txVI.txInIndex, v.flags, v.sigCache)
			vm, err := txscript.NewEngine(pkScript, txVI.tx.MsgTx(),
				txVI.txInIndex, v.flags, v.sigCache, txVI.sigHashes,
				inputAmount)
			if err != nil {
				logging.CPrint(logging.ERROR, "failed to construct vm engine",
					logging.LogFormat{"transaction": txVI.tx.Hash(), "index": txVI.txInIndex, "previousOutPoint": txIn.PreviousOutPoint,
						"input witness": witness, "reference pkScript": pkScript, "err": err})
				v.sendResult(ErrScriptMalformed)
				break out
			}

			// Execute the script pair.
			if err := vm.Execute(); err != nil {
				logging.CPrint(logging.ERROR, "failed to validate signature",
					logging.LogFormat{"transaction": txVI.tx.Hash(), "index": txVI.txInIndex, "previousOutPoint": txIn.PreviousOutPoint,
						"input witness": witness, "reference pkScript": pkScript, "err": err})
				v.sendResult(ErrScriptValidation)
				break out
			}

			// Validation succeeded.
			v.sendResult(nil)

		case <-v.quitChan:
			break out
		}
	}
}

// Validate validates the scripts for all of the passed transaction inputs using
// multiple goroutines.
func (v *txValidator) Validate(items []*txValidateItem) error {
	if len(items) == 0 {
		return nil
	}

	// Limit the number of goroutines to do script validation based on the
	// number of processor cores.  This help ensure the system stays
	// reasonably responsive under heavy load.
	maxGoRoutines := runtime.NumCPU() * 3
	if maxGoRoutines <= 0 {
		maxGoRoutines = 1
	}
	if maxGoRoutines > len(items) {
		maxGoRoutines = len(items)
	}

	// Start up validation handlers that are used to asynchronously
	// validate each transaction input.
	for i := 0; i < maxGoRoutines; i++ {
		go v.validateHandler()
	}

	// Validate each of the inputs.  The quit channel is closed when any
	// errors occur so all processing goroutines exit regardless of which
	// input had the validation error.
	numInputs := len(items)
	currentItem := 0
	processedItems := 0
	for processedItems < numInputs {
		// Only send items while there are still items that need to
		// be processed.  The select statement will never select a nil
		// channel.
		var validateChan chan *txValidateItem
		var item *txValidateItem
		if currentItem < numInputs {
			validateChan = v.validateChan
			item = items[currentItem]
		}

		select {
		case validateChan <- item:
			currentItem++

		case err := <-v.resultChan:
			processedItems++
			if err != nil {
				close(v.quitChan)
				return err
			}
		}
	}

	close(v.quitChan)
	return nil
}

// newTxValidator returns a new instance of txValidator to be used for
// validating transaction scripts asynchronously.
func newTxValidator(txStore TxStore, flags txscript.ScriptFlags, sigCache *txscript.SigCache, hashCache *txscript.HashCache) *txValidator {
	return &txValidator{
		validateChan: make(chan *txValidateItem),
		quitChan:     make(chan struct{}),
		resultChan:   make(chan error),
		txStore:      txStore,
		sigCache:     sigCache,
		hashCache:    hashCache,
		flags:        flags,
	}
}

// ValidateTransactionScripts validates the scripts for the passed transaction
// using multiple goroutines.
func ValidateTransactionScripts(tx *massutil.Tx, txStore TxStore, flags txscript.ScriptFlags, sigCache *txscript.SigCache, hashCache *txscript.HashCache) error {
	// Collect all of the transaction inputs and required information for
	// validation.

	if !hashCache.ContainsHashes(tx.Hash()) {
		hashCache.AddSigHashes(tx.MsgTx())
	}

	var cachedHashes *txscript.TxSigHashes
	cachedHashes, _ = hashCache.GetSigHashes(tx.Hash())

	txIns := tx.MsgTx().TxIn
	txValItems := make([]*txValidateItem, 0, len(txIns))

	for txInIdx, txIn := range txIns {
		// Skip coinbases.

		if txIn.PreviousOutPoint.Index == math.MaxUint32 {
			continue
		}

		txVI := &txValidateItem{
			txInIndex: txInIdx,
			txIn:      txIn,
			tx:        tx,
			sigHashes: cachedHashes,
		}
		txValItems = append(txValItems, txVI)
	}

	// Validate all of the inputs.
	validator := newTxValidator(txStore, flags, sigCache, hashCache)
	if err := validator.Validate(txValItems); err != nil {
		return err
	}

	return nil
}

// checkBlockScripts executes and validates the scripts for all transactions in
// the passed block.
func checkBlockScripts(block *massutil.Block, txStore TxStore,
	scriptFlags txscript.ScriptFlags, sigCache *txscript.SigCache, hashCache *txscript.HashCache) error {

	// Collect all of the transaction inputs and required information for
	// validation for all transactions in the block into a single slice.
	numInputs := 0
	for _, tx := range block.Transactions()[1:] {

		numInputs += len(tx.MsgTx().TxIn)
	}
	txValItems := make([]*txValidateItem, 0, numInputs)
	for _, tx := range block.Transactions()[1:] {

		hash := tx.Hash()
		// If the HashCache is present, and it doesn't yet contain the
		// partial sighashes for this transaction, then we add the
		// sighashes for the transaction. This allows us to take
		// advantage of the potential speed savings due to the new
		// digest algorithm (BIP0143).
		if hashCache != nil && !hashCache.ContainsHashes(hash) {

			hashCache.AddSigHashes(tx.MsgTx())
		}
		var cachedHashes *txscript.TxSigHashes
		if true {
			if hashCache != nil {
				cachedHashes, _ = hashCache.GetSigHashes(hash)
			} else {
				cachedHashes = txscript.NewTxSigHashes(tx.MsgTx())
			}
		}

		for txInIdx, txIn := range tx.MsgTx().TxIn {
			// Skip coinbases.
			if txIn.PreviousOutPoint.Index == math.MaxUint32 {
				continue
			}

			txVI := &txValidateItem{
				txInIndex: txInIdx,
				txIn:      txIn,
				tx:        tx,
				sigHashes: cachedHashes,
			}
			txValItems = append(txValItems, txVI)
		}
	}

	// Validate all of the inputs.
	validator := newTxValidator(txStore, scriptFlags, sigCache, hashCache)
	//start := time.Now()
	if err := validator.Validate(txValItems); err != nil {
		return err
	}

	// If the HashCache is present, once we have validated the block, we no
	// longer need the cached hashes for these transactions, so we purge
	// them from the cache.
	if hashCache != nil {
		for _, tx := range block.Transactions() {
			hashCache.PurgeSigHashes(tx.Hash())
		}
	}

	return nil
}
