package blockchain

import (
	"container/list"
	"math"
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store.
	mempoolHeight = math.MaxUint64

	// maxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	maxOrphanTxSize = 5000

	// maxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	maxSigOpsPerTx = MaxSigOpsPerBlock / 5

	// defaultBlockPrioritySize is the default size in bytes for high-
	// priority / low-fee transactions.  It is used to help determine which
	// are allowed into the mempool and consequently affects their relay and
	// inclusion when generating block templates.
	defaultBlockPrioritySize = 50000

	// MaxTxMsgPayload is the maximum bytes a transaction payload can be in bytes.
	// 200 bytes
	maxTxPoolTxPayload = 200
)

// TxDesc is a descriptor containing a transaction in the mempool and the
// metadata we store about it.
type TxDesc struct {
	Tx               *massutil.Tx    // Transaction.
	Added            time.Time       // Time when added to pool.
	Height           uint64          // Block height when added to pool.
	startingPriority float64         // Priority when added to the pool.
	Fee              massutil.Amount // Transaction fees.
	totalInputValue  massutil.Amount
}

// TxPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type TxPool struct {
	sync.RWMutex
	pool          map[wire.Hash]*TxDesc
	orphanTxPool  *OrphanTxPool
	errCache      *lru.Cache
	addrindex     map[string]map[wire.Hash]struct{} // maps address to txs
	outpoints     map[wire.OutPoint]*massutil.Tx
	lastUpdated   time.Time // last time pool was updated
	pennyTotal    float64   // exponentially decaying total for penny spends.
	lastPennyUnix int64     // unix time of last ``penny spend''
	chain         *Blockchain
	sigCache      *txscript.SigCache
	hashCache     *txscript.HashCache
	NewTxCh       chan *massutil.Tx
}

func (tp *TxPool) SetNewTxCh(ch chan *massutil.Tx) {
	tp.Lock()
	defer tp.Unlock()
	tp.NewTxCh = ch
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool and
// previous orphan index.
//
// This function is safe for concurrent access.
func (tp *TxPool) RemoveOrphan(txHash *wire.Hash) {
	tp.Lock()
	tp.orphanTxPool.removeOrphan(txHash)
	tp.Unlock()
}

// isTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (tp *TxPool) isTransactionInPool(hash *wire.Hash) bool {
	_, exists := tp.pool[*hash]
	return exists
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) IsTransactionInPool(hash *wire.Hash) bool {
	// Protect concurrent access.
	tp.RLock()
	defer tp.RUnlock()

	return tp.isTransactionInPool(hash)
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) IsOrphanInPool(hash *wire.Hash) bool {
	// Protect concurrent access.
	tp.RLock()
	defer tp.RUnlock()

	return tp.orphanTxPool.isOrphanInPool(hash)
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (tp *TxPool) haveTransaction(hash *wire.Hash) bool {
	return tp.isTransactionInPool(hash) || tp.orphanTxPool.isOrphanInPool(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) HaveTransaction(hash *wire.Hash) bool {
	// Protect concurrent access.
	tp.RLock()
	defer tp.RUnlock()

	return tp.haveTransaction(hash)
}

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) removeTransaction(tx *massutil.Tx, removeRedeemers bool) {
	txHash := tx.Hash()
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
			outpoint := wire.NewOutPoint(txHash, i)
			if txRedeemer, exists := tp.outpoints[*outpoint]; exists {
				tp.removeTransaction(txRedeemer, true)
			}
		}
	}

	// Remove the transaction and mark the referenced outpoints as unspent
	// by the pool.
	if txDesc, exists := tp.pool[*txHash]; exists {
		if config.AddrIndex {
			if err := tp.removeTransactionFromAddrIndex(tx); err != nil {
				logging.CPrint(logging.ERROR, "fail on removeTransactionFromAddrIndex", logging.LogFormat{
					"err":  err,
					"txid": tx.Hash(),
				})
			}
		}

		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(tp.outpoints, txIn.PreviousOutPoint)
		}
		delete(tp.pool, *txHash)
		tp.lastUpdated = time.Now()
	}
}

// removeTransactionFromAddrIndex removes the passed transaction from our
// address based index.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) removeTransactionFromAddrIndex(tx *massutil.Tx) error {
	previousOutputScripts, err := tp.fetchReferencedOutputScripts(tx)
	if err != nil {
		logging.CPrint(logging.ERROR, "Unable to obtain referenced output scripts for the passed tx ",
			logging.LogFormat{
				"addrindex": err,
			})
		return err
	}

	for _, pkScript := range previousOutputScripts {
		if err := tp.removeScriptFromAddrIndex(pkScript, tx); err != nil {
			return err
		}
	}

	for _, txOut := range tx.MsgTx().TxOut {
		if err := tp.removeScriptFromAddrIndex(txOut.PkScript, tx); err != nil {
			return err
		}
	}

	return nil
}

// removeScriptFromAddrIndex dissociates the address encoded by the
// passed pkScript from the passed tx in our address based tx index.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) removeScriptFromAddrIndex(pkScript []byte, tx *massutil.Tx) error {
	_, addresses, _, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		&config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "Unable to extract encoded addresses from script for addrindex",
			logging.LogFormat{
				"addrindex": err,
			})
		return err
	}
	for _, addr := range addresses {
		ea := addr.EncodeAddress()
		if m, exists := tp.addrindex[ea]; exists {
			delete(m, *tx.Hash())
			if len(m) == 0 {
				delete(tp.addrindex, ea)
			}
		}
	}

	return nil
}

// RemoveTransaction removes the passed transaction from the mempool. If
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphan.
//
// This function is safe for concurrent access.
func (tp *TxPool) RemoveTransaction(tx *massutil.Tx, removeRedeemers bool) {
	// Protect concurrent access.
	tp.Lock()
	defer tp.Unlock()

	tp.removeTransaction(tx, removeRedeemers)
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool.  Removing those transactions then
// leads to removing all transactions which rely on them, recursively.  This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool
//
// This function is safe for concurrent access.
func (tp *TxPool) RemoveDoubleSpends(tx *massutil.Tx) {
	// Protect concurrent access.
	tp.Lock()
	defer tp.Unlock()

	tp.removeDoubleSpends(tx)
}

func (tp *TxPool) removeDoubleSpends(tx *massutil.Tx) {
	for _, txIn := range tx.MsgTx().TxIn {
		if txRedeemer, ok := tp.outpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Hash().IsEqual(tx.Hash()) {
				tp.removeTransaction(txRedeemer, true)
			}
		}
	}
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) addTransaction(tx *massutil.Tx, height uint64, startingPriority float64,
	totalInputValue, fee massutil.Amount) error {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.

	if tp.pool == nil {
		return ErrTxPoolNil
	}

	tp.pool[*tx.Hash()] = &TxDesc{
		Tx:               tx,
		Added:            time.Now(),
		Height:           height,
		startingPriority: startingPriority,
		Fee:              fee,
		totalInputValue:  totalInputValue,
	}

	for _, txIn := range tx.MsgTx().TxIn {
		tp.outpoints[txIn.PreviousOutPoint] = tx
	}
	tp.lastUpdated = time.Now()

	tp.NewTxCh <- tx

	if config.AddrIndex {
		err := tp.addTransactionToAddrIndex(tx)
		return err
	}

	return nil
}

// addTransactionToAddrIndex adds all addresses related to the transaction to
// our in-memory wallet index. Note that this wallet is only populated when
// we're running with the optional address index activated.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) addTransactionToAddrIndex(tx *massutil.Tx) error {
	previousOutScripts, err := tp.fetchReferencedOutputScripts(tx)
	if err != nil {
		logging.CPrint(logging.ERROR, "Unable to obtain referenced output scripts for the passed tx ",
			logging.LogFormat{
				"addrindex": err,
			})
		return err
	}
	// Index addresses of all referenced previous output tx's.
	for _, pkScript := range previousOutScripts {
		if err := tp.indexScriptAddressToTx(pkScript, tx); err != nil {
			return err
		}
	}

	// Index addresses of all created outputs.
	for _, txOut := range tx.MsgTx().TxOut {
		if err := tp.indexScriptAddressToTx(txOut.PkScript, tx); err != nil {
			return err
		}
	}

	return nil
}

// fetchReferencedOutputScripts looks up and returns all the scriptPubKeys
// referenced by inputs of the passed transaction.
//
// This function MUST be called with the mempool lock held (for reads).
func (tp *TxPool) fetchReferencedOutputScripts(tx *massutil.Tx) ([][]byte, error) {
	txStore := tp.FetchInputTransactions(tx, false)
	if len(txStore) == 0 {
		return nil, ErrFindReferenceInput
	}

	previousOutScripts := make([][]byte, 0, len(tx.MsgTx().TxIn))
	for _, txIn := range tx.MsgTx().TxIn {
		outPoint := txIn.PreviousOutPoint
		if txStore[outPoint.Hash].Err == nil {
			referencedOutPoint := txStore[outPoint.Hash].Tx.MsgTx().TxOut[outPoint.Index]
			previousOutScripts = append(previousOutScripts, referencedOutPoint.PkScript)
		}
	}
	return previousOutScripts, nil
}

// indexScriptByAddress alters our wallet index by indexing the payment wallet
// encoded by the passed scriptPubKey to the passed transaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) indexScriptAddressToTx(pkScript []byte, tx *massutil.Tx) error {
	_, addresses, _, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		&config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "Unable to extract encoded addresses from script for addrindex ",
			logging.LogFormat{
				"addrindex": err,
			})
		return err
	}

	for _, addr := range addresses {
		ea := addr.EncodeAddress()
		m, exists := tp.addrindex[ea]
		if !exists {
			m = make(map[wire.Hash]struct{})
			tp.addrindex[ea] = m
		}
		m[*tx.Hash()] = struct{}{}
	}

	return nil
}

// TODO: should lazily update txD.totalInputValue, but that costs a lot.
func (txD *TxDesc) StartingPriority() (float64, error) {
	return txD.startingPriority, nil
}

func (txD *TxDesc) CurrentPriority(txStore TxStore, nextBlockHeight uint64) (float64, error) {
	priority, _, err := currentPriority(txD.Tx, txStore, nextBlockHeight)
	return priority, err
}

// TODO: should lazily update txD.totalInputValue, but that costs a lot.
func (txD *TxDesc) TotalInputAge() (massutil.Amount, error) {
	return txD.totalInputValue, nil
}

func currentPriority(tx *massutil.Tx, txStore TxStore, blockHeight uint64) (float64, massutil.Amount, error) {
	inputAge, totalInputValue, err := calcInputValueAge(tx, txStore, blockHeight)
	if err != nil {
		return 0, massutil.ZeroAmount(), err
	}
	return calcPriority(tx, inputAge), totalInputValue, nil
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (tp *TxPool) checkPoolDoubleSpend(tx *massutil.Tx) error {
	for _, txIn := range tx.MsgTx().TxIn {
		if txR, exists := tp.outpoints[txIn.PreviousOutPoint]; exists {
			logging.CPrint(logging.ERROR, "output already spent by transaction in the memory pool",
				logging.LogFormat{
					"output":      txIn.PreviousOutPoint,
					"transcation": txR.Hash(),
				})
			return ErrDoubleSpend
		}
	}

	return nil
}

// FetchInputTransactions fetches the input transactions referenced by the
// passed transaction.  First, it fetches from the main chain, then it tries to
// fetch any missing inputs from the transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (tp *TxPool) FetchInputTransactions(tx *massutil.Tx, includeSpent bool) TxStore {
	txStore := tp.chain.FetchTransactionStore(tx, includeSpent)

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txD := range txStore {
		if txD.Err != nil && txD.Err != database.ErrTxShaMissing {
			logging.CPrint(logging.ERROR, "FetchTransactionStore failed",
				logging.LogFormat{
					"err":         txD.Err,
					"transcation": tx.Hash().String(),
				})
		}
		if txD.Err == database.ErrTxShaMissing || txD.Tx == nil {
			if poolTxDesc, exists := tp.pool[*txD.Hash]; exists {
				poolTx := poolTxDesc.Tx
				txD.Tx = poolTx
				txD.BlockHeight = mempoolHeight
				txD.Spent = make([]bool, len(poolTx.MsgTx().TxOut))
				txD.Err = nil
			}
		}
	}

	return txStore
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include
// orphans.
//
// This function is safe for concurrent access.
func (tp *TxPool) FetchTransaction(txHash *wire.Hash) (*massutil.Tx, error) {
	// Protect concurrent access.
	tp.RLock()
	defer tp.RUnlock()

	if txDesc, exists := tp.pool[*txHash]; exists {
		return txDesc.Tx, nil
	}
	logging.CPrint(logging.DEBUG, "transaction is not in the pool",
		logging.LogFormat{"txHash": txHash})
	return nil, ErrTxExsit
}

// FilterTransactionsByAddress returns all transactions currently in the
// mempool that either create an output to the passed address or spend a
// previously created ouput to the address.
func (tp *TxPool) FilterTransactionsByAddress(addr massutil.Address) ([]*massutil.Tx, error) {
	// Protect concurrent access.
	tp.RLock()
	defer tp.RUnlock()

	if txs, exists := tp.addrindex[addr.EncodeAddress()]; exists {
		addressTxs := make([]*massutil.Tx, 0, len(txs))
		for txHash := range txs {
			if tx, exists := tp.pool[txHash]; exists {
				addressTxs = append(addressTxs, tx.Tx)
			}
		}
		return addressTxs, nil
	}
	logging.CPrint(logging.ERROR, "address does not have any transactions in the pool",
		logging.LogFormat{"address": addr})
	return nil, ErrFindTxByAddr
}

// maybeAcceptTransaction is the internal function which implements the public
// MaybeAcceptTransaction.  See the comment for MaybeAcceptTransaction for
// more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) maybeAcceptTransaction(tx *massutil.Tx, isNew, rateLimit bool) ([]*wire.Hash, error) {
	txHash := tx.Hash()

	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if tp.haveTransaction(txHash) {
		return nil, errors.ErrTxAlreadyExists
	}

	if len(tx.MsgTx().Payload) > maxTxPoolTxPayload {
		logging.CPrint(logging.ERROR, "transaction payload size is too big",
			logging.LogFormat{"size": len(tx.MsgTx().Payload), "max": maxTxPoolTxPayload})
		return nil, ErrTxMsgPayloadSize
	}

	// Perform preliminary sanity checks on the transaction.  This makes
	// use of chain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err := CheckTransactionSanity(tx)
	if err != nil {
		return nil, err
	}

	// A standalone transaction must not be a coinbase transaction.
	if IsCoinBase(tx) {
		logging.CPrint(logging.ERROR, "transaction is an individual coinbase",
			logging.LogFormat{"txHash": txHash})
		return nil, ErrCoinbaseTx
	}

	// Don't accept transactions with a lock time after the maximum int64
	// value for now.
	if tx.MsgTx().LockTime > math.MaxInt64 {
		logging.CPrint(logging.ERROR, "transaction has a lock time which is not accepted yet",
			logging.LogFormat{"txHash": txHash, "lockTime": tx.MsgTx().LockTime})
		return nil, ErrImmatureSpend
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so it's height is at least
	// one more than the current height.
	curHeight := tp.chain.blockTree.bestBlockNode().Height
	nextBlockHeight := curHeight + 1
	node := tp.chain.blockTree.bestBlockNode()
	medianTimePast, err := tp.chain.calcPastMedianTime(node)
	if err != nil {
		return nil, err
	}

	// Fetch all of the transactions referenced by the inputs to this
	// transaction.  This function also attempts to fetch the transaction
	// itself to be used for detecting a duplicate transaction without
	// needing to do a separate lookup.
	txStore := tp.FetchInputTransactions(tx, false)

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	if txD, exists := txStore[*txHash]; exists && txD.Err == nil {
		return nil, errors.ErrTxAlreadyExists
	}

	delete(txStore, *txHash)

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !config.ChainParams.RelayNonStdTxs {
		err = checkTransactionStandard(tx, nextBlockHeight, massutil.MinRelayTxFee(), txStore)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			logging.CPrint(logging.ERROR, "transaction is not standard",
				logging.LogFormat{"txHash": txHash, "err": err})
			return nil, err
		}
	}

	// The transaction may not use any of the same outputs as other
	// transactions already in the pool as that would ultimately result in a
	// double spend.  This check is intended to be quick and therefore only
	// detects double spends within the transaction pool itself.  The
	// transaction could still be double spending coins from the main chain
	// at this point.  There is a more in-depth check that happens later
	// after fetching the referenced transaction inputs from the main chain
	// which examines the actual spend data and prevents double spends.
	err = tp.checkPoolDoubleSpend(tx)
	if err != nil {
		return nil, err
	}

	// Transaction is an orphan if any of the referenced input transactions
	// don't exist.  Adding orphans to the orphan pool is not handled by
	// this function, and the caller should use maybeAddOrphan if this
	// behavior is desired.
	var missingParents []*wire.Hash
	for _, txD := range txStore {
		if txD.Err == database.ErrTxShaMissing {
			missingParents = append(missingParents, txD.Hash)
		}
	}
	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in chain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := CheckTransactionInputs(tx, nextBlockHeight, txStore)
	if err != nil {
		return nil, err
	}

	// stakingTx
	// Don't allow the transaction into the mempool unless its sequence
	// lock is active, meaning that it'll be allowed into the next block
	// with respect to its defined relative lock times.
	sequenceLock, err := tp.chain.CalcSequenceLock(tx, txStore)
	if err != nil {
		return nil, err
	}
	if !SequenceLockActive(sequenceLock, nextBlockHeight,
		medianTimePast) {
		return nil, ErrSequenceNotSatisfied
	}

	// Don't allow transactions with non-standard inputs if the network
	// parameters forbid their relaying.
	if !config.ChainParams.RelayNonStdTxs {
		err := checkInputsStandard(tx, txStore)
		if err != nil {
			return nil, err
		}
	}

	// NOTE: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.  Since
	// the coinbase address itself can contain signature operations, the
	// maximum allowed signature operations per transaction is less than
	// the maximum allowed signature operations per block.
	numSigOps := CountSigOps(tx)
	if numSigOps > maxSigOpsPerTx {
		logging.CPrint(logging.ERROR, "transaction contains too many signature operations",
			logging.LogFormat{"txHash": txHash, "numSigOps": numSigOps, "maxSigOpsPerTx": maxSigOpsPerTx})
		return nil, ErrTooManySigOps
	}

	// Don't allow transactions with fees too low to get into a mined block.
	//
	// Most miners allow a free transaction area in blocks they mine to go
	// alongside the area used for high-priority transactions as well as
	// transactions with fees.  A transaction size of up to 1000 bytes is
	// considered safe to go into this section.  Further, the minimum fee
	// calculated below on its own would encourage several small
	// transactions to avoid fees rather than one single larger transaction
	// which is more desirable.  Therefore, as long as the size of the
	// transaction does not exceeed 1000 less than the reserved space for
	// high-priority transactions, don't require a fee for it.

	serializedSize := int64(tx.MsgTx().PlainSize())
	requiredFee, err := CalcMinRequiredTxRelayFee(serializedSize, massutil.MinRelayTxFee())
	if err != nil {
		logging.CPrint(logging.ERROR, "CalcMinRequiredTxRelayFee error", logging.LogFormat{"err": err})
		return nil, err
	}

	if txFee.Cmp(requiredFee) < 0 {
		if serializedSize >= (defaultBlockPrioritySize - 1000) {
			logging.CPrint(logging.ERROR, "transaction`s fees is under the required amount",
				logging.LogFormat{"txHash": txHash, "txFee": txFee, "requiredFee": requiredFee})
			return nil, ErrInsufficientFee
		}

		// Require that free transactions have sufficient priority to be mined
		// in the next block.  Transactions which are being added back to the
		// memory pool from blocks that have been disconnected during a reorg
		// are exempted.
		if isNew && !config.NoRelayPriority {
			currentPriority, _, err := currentPriority(tx, txStore, nextBlockHeight)
			if err != nil {
				return nil, err
			}
			if currentPriority <= consensus.MinHighPriority {
				logging.CPrint(logging.ERROR, "transaction has insufficient priority",
					logging.LogFormat{"txHash": txHash, "currentPriority": currentPriority,
						"MinHighPriority": consensus.MinHighPriority})
				return nil, ErrInsufficientPriority
			}
		}

		// Free-to-relay transactions are rate limited here to prevent
		// penny-flooding with tiny transactions as a form of attack.
		if rateLimit {
			nowUnix := time.Now().Unix()
			// we decay passed data with an exponentially decaying ~10
			// minutes window - matches Massd handling.
			tp.pennyTotal *= math.Pow(1.0-1.0/600.0,
				float64(nowUnix-tp.lastPennyUnix))
			tp.lastPennyUnix = nowUnix

			// Are we still over the limit?
			if tp.pennyTotal >= config.FreeTxRelayLimit*10*1000 {
				logging.CPrint(logging.ERROR, "transaction has been rejected by the rate limiter due to low fees",
					logging.LogFormat{"txHash": txHash})
				return nil, ErrInsufficientFee
			}
			oldTotal := tp.pennyTotal

			tp.pennyTotal += float64(serializedSize)
			logging.CPrint(logging.TRACE, "rate limit", logging.LogFormat{
				"curTotal":  oldTotal,
				"nextTotal": tp.pennyTotal,
				"limit":     config.FreeTxRelayLimit * 10 * 1000,
			})

		}
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = ValidateTransactionScripts(tx, txStore,
		txscript.StandardVerifyFlags, tp.sigCache,
		tp.hashCache)
	if err != nil {
		return nil, err
	}

	// Add to transaction pool.
	startingPriority, totalInputValue, err := currentPriority(tx, txStore, curHeight)
	if err != nil {
		return nil, err
	}
	err = tp.addTransaction(tx, curHeight, startingPriority, totalInputValue, txFee)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// MaybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, detecting orphan transactions, and insertion into the memory pool.
//
// If the transaction is an orphan (missing Parent transactions), the
// transaction is NOT added to the orphan pool, but each unknown referenced
// Parent is returned.  Use ProcessTransaction instead if new orphans should
// be added to the orphan pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) MaybeAcceptTransaction(tx *massutil.Tx, isNew, rateLimit bool) ([]*wire.Hash, error) {
	// Protect concurrent access.
	tp.Lock()
	defer tp.Unlock()

	return tp.maybeAcceptTransaction(tx, isNew, rateLimit)
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (tp *TxPool) processOrphans(hash *wire.Hash) {
	// Start with processing at least the passed hash.
	processHashes := list.New()
	processHashes.PushBack(hash)
	for processHashes.Len() > 0 {
		// Pop the first hash to process.
		firstElement := processHashes.Remove(processHashes.Front())
		processHash := firstElement.(*wire.Hash)

		// Look up all orphans that are referenced by the transaction we
		// just accepted.  This will typically only be one, but it could
		// be multiple if the referenced transaction contains multiple
		// outputs.  Skip to the next item on the list of hashes to
		// process if there are none.
		orphans := tp.orphanTxPool.getOrphansByPrevious(processHash)

		for _, tx := range orphans {
			orphanHash := tx.Hash()
			tp.orphanTxPool.removeOrphan(orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			missingParents, err := tp.maybeAcceptTransaction(tx,
				true, true)
			if err != nil {
				logging.CPrint(logging.DEBUG, "Unable to move orphan transaction to mempool", logging.LogFormat{
					"orphan transaction": tx.Hash(),
					"err":                err,
				})
				continue
			}

			if len(missingParents) > 0 {
				// Transaction is still an orphan, so add it back.
				tp.orphanTxPool.addOrphan(tx)
				continue
			}

			// Add this transaction to the list of transactions to
			// process so any orphans that depend on this one are
			// handled too.
			processHashes.PushBack(orphanHash)
		}
	}
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction hash (it is possible that they are no longer orphans) and
// potentially accepts them to the memory pool.  It repeats the process for the
// newly accepted transactions (to detect further orphans which may no longer be
// orphans) until there are no more.
//
// This function is safe for concurrent access.
func (tp *TxPool) ProcessOrphans(hash *wire.Hash) {
	tp.Lock()
	tp.processOrphans(hash)
	tp.Unlock()
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) ProcessTransaction(tx *massutil.Tx, allowOrphan, rateLimit bool) (bool, error) {
	// Protect concurrent access.
	tp.Lock()
	defer tp.Unlock()

	// Potentially accept the transaction to the memory pool.
	missingParents, err := tp.maybeAcceptTransaction(tx, true, rateLimit)
	if err != nil {
		return false, err
	}

	var isOrphan bool

	if len(missingParents) == 0 {

		isOrphan = false
		// Accept any orphan transactions that depend on this
		// transaction (they may no longer be orphans if all inputs
		// are now available) and repeat for those accepted
		// transactions until there are no more.
		tp.processOrphans(tx.Hash())
	} else {

		isOrphan = true
		// The transaction is an orphan (has inputs missing).  Reject
		// it if the flag to allow orphans is not set.
		if !allowOrphan {
			// Only use the first missing Parent transaction in
			// the error message.
			//
			// NOTE: RejectDuplicate is really not an accurate
			// reject code here, but it matches the reference
			// implementation and there isn't a better choice due
			// to the limited number of reject codes.  Missing
			// inputs is assumed to mean they are already spent
			// which is not really always the case.
			logging.CPrint(logging.ERROR, "orphan transaction references transaction outputs is unknown or fully-spent",
				logging.LogFormat{"tx": tx.Hash(), "missingParents": missingParents[0]})
			return isOrphan, ErrProhibitionOrphanTx
		}

		// Potentially add the orphan transaction to the orphan pool.
		err := tp.orphanTxPool.maybeAddOrphan(tx)
		if err != nil {
			return isOrphan, err
		}
	}

	return isOrphan, nil
}

// Count returns the number of transactions in the main pool.  It does not
// include the orphan pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) Count() int {
	tp.RLock()
	defer tp.RUnlock()

	return len(tp.pool)
}

// TxShas returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) TxShas() []*wire.Hash {
	tp.RLock()
	defer tp.RUnlock()

	hashes := make([]*wire.Hash, len(tp.pool))
	i := 0
	for hash := range tp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// This function is safe for concurrent access.
func (tp *TxPool) TxDescs() []*TxDesc {
	tp.RLock()
	defer tp.RUnlock()

	descs := make([]*TxDesc, len(tp.pool))
	i := 0
	for _, desc := range tp.pool {
		descs[i] = desc
		i++
	}

	return descs
}

func (tp *TxPool) OrphanTxs() []*massutil.Tx {
	tp.RLock()
	defer tp.RUnlock()
	return tp.orphanTxPool.orphans()
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (tp *TxPool) LastUpdated() time.Time {
	tp.RLock()
	defer tp.RUnlock()

	return tp.lastUpdated
}

func (tp *TxPool) SyncAttachBlock(block *massutil.Block) {
	tp.Lock()
	defer tp.Unlock()

	for _, tx := range block.Transactions()[1:] {
		tp.removeTransaction(tx, false)
		tp.removeDoubleSpends(tx)
		tp.orphanTxPool.removeOrphan(tx.Hash())
		tp.processOrphans(tx.Hash())
	}
}

func (tp *TxPool) SyncDetachBlock(block *massutil.Block) {
	tp.Lock()
	defer tp.Unlock()

	for _, tx := range block.Transactions()[1:] {
		if _, err := tp.maybeAcceptTransaction(tx, false, false); err != nil {
			// Remove the transaction and all transactions
			// that depend on it if it wasn't accepted into
			// the transaction pool.
			tp.removeTransaction(tx, true)
		}
	}
}

// NewTxPool returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func NewTxPool(chain *Blockchain, sigCache *txscript.SigCache, hashCache *txscript.HashCache) *TxPool {
	memPool := &TxPool{
		sigCache:     sigCache,
		hashCache:    hashCache,
		chain:        chain,
		pool:         make(map[wire.Hash]*TxDesc),
		orphanTxPool: newOrphanTxPool(),
		errCache:     lru.New(500),
		outpoints:    make(map[wire.OutPoint]*massutil.Tx),
	}
	if config.AddrIndex {
		memPool.addrindex = make(map[string]map[wire.Hash]struct{})
	}
	return memPool
}
