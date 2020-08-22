package blockchain

import (
	"container/heap"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/poc"
	"massnet.org/mass/pocec"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	// generatedBlockVersion is the version of the block being generated.
	// It is defined as a constant here rather than using the
	// wire.BlockVersion constant since a change in the block version
	// will require changes to the generated block.  Using the wire constant
	// for generated block version could allow creation of invalid blocks
	// for the updated version.
	generatedBlockVersion = wire.BlockVersion

	// blockHeaderOverhead is the min number of bytes it takes to serialize
	// a block header.
	blockHeaderOverhead = wire.MinBlockHeaderPayload

	// maximum number of binding evidence(recommended)
	MaxBindingNum = 10

	// 36 prev outpoint, 8 sequence
	bindingPayload = 44 * MaxBindingNum

	// minHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	minHighPriority = consensus.MinHighPriority

	PriorityProposalSize = wire.MaxBlockPayload / 20
)

var (
	anyoneRedeemableScript []byte
)

func init() {
	var err error
	anyoneRedeemableScript, err = txscript.NewScriptBuilder().AddOp(txscript.OP_TRUE).Script()
	if err != nil {
		panic("init anyoneRedeemableScript: " + err.Error())
	}
}

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	tx       *massutil.Tx
	fee      int64
	priority float64
	feePerKB float64

	// dependsOn holds a map of transaction hashes which this one depends
	// on.  It will only be set when the transaction references other
	// transactions in the memory pool and hence must come after them in
	// a block.
	dependsOn map[wire.Hash]struct{}
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

type CoinbasePayload struct {
	height           uint64
	numStakingReward uint32
}

func (p *CoinbasePayload) NumStakingReward() uint32 {
	return p.numStakingReward
}

func (p *CoinbasePayload) Bytes() []byte {
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint64(buf[:8], p.height)
	binary.LittleEndian.PutUint32(buf[8:12], p.numStakingReward)
	return buf
}

func (p *CoinbasePayload) SetBytes(data []byte) error {
	if len(data) < 12 {
		return errIncompleteCoinbasePayload
	}
	p.height = binary.LittleEndian.Uint64(data[0:8])
	p.numStakingReward = binary.LittleEndian.Uint32(data[8:12])
	return nil
}

func NewCoinbasePayload() *CoinbasePayload {
	return &CoinbasePayload{
		height:           0,
		numStakingReward: 0,
	}
}

func standardCoinbasePayload(nextBlockHeight uint64, numStakingReward uint32) []byte {
	p := &CoinbasePayload{
		height:           nextBlockHeight,
		numStakingReward: numStakingReward,
	}
	return p.Bytes()
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// txPQByPriority sorts a txPriorityQueue by transaction priority and then fees
// per kilobyte.
func txPQByPriority(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest priority item as opposed
	// to the lowest.  Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}
	return pq.items[i].priority > pq.items[j].priority

}

// txPQByFee sorts a txPriorityQueue by fees per kilobyte and then transaction
// priority.
func txPQByFee(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses either
// the txPQByPriority or the txPQByFee compare function depending on the
// sortByFee parameter and is already initialized for use with heap.Push/Pop.
// The priority queue can grow larger than the reserved space, but extra copies
// of the underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, sortByFee bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	if sortByFee {
		pq.SetLessFunc(txPQByFee)
	} else {
		pq.SetLessFunc(txPQByPriority)
	}
	return pq
}

type PoCTemplate struct {
	Height        uint64
	Timestamp     time.Time
	Previous      wire.Hash
	Challenge     wire.Hash
	GetTarget     func(time.Time) *big.Int
	RewardAddress []database.Rank
	GetCoinbase   func(*pocec.PublicKey, massutil.Amount, int) (*massutil.Tx, error)
	Err           error
}

type BlockTemplate struct {
	Block              *wire.MsgBlock
	TotalFee           massutil.Amount
	SigOpCounts        []int64
	Height             uint64
	ValidPayAddress    bool
	MerkleCache        []*wire.Hash
	WitnessMerkleCache []*wire.Hash
	Err                error
}

func reCreateCoinbaseTx(coinbase *wire.MsgTx, bindingTxListReply []*database.BindingTxReply, nextBlockHeight uint64,
	bitLength int, rewardAddresses []database.Rank, totalFee massutil.Amount) (err error) {

	minerPkScript := coinbase.TxOut[len(coinbase.TxOut)-1].PkScript
	coinbase.RemoveAllTxOut()

	totalBinding := massutil.ZeroAmount()
	// means still have reward in coinbase
	// originMiner cannot be smaller than diff
	// has guranty tx
	txIns := make([]*wire.TxIn, 0)
	hasValidBinding := false
	if len(bindingTxListReply) > 0 {
		valueRequired, ok := bindingRequiredAmount[bitLength]
		// valid bit length
		if ok {
			var witness [][]byte
			bindingNum := 0

			for _, bindingTx := range bindingTxListReply {
				txHash := bindingTx.TxSha
				index := bindingTx.Index

				blocksSincePrev := nextBlockHeight - bindingTx.Height
				if bindingTx.IsCoinbase {
					if blocksSincePrev < consensus.CoinbaseMaturity {
						logging.CPrint(logging.WARN, "the txIn is not mature", logging.LogFormat{"txid": txHash.String(), "index": index})
						continue
					}
				} else {
					if blocksSincePrev < consensus.TransactionMaturity {
						logging.CPrint(logging.WARN, "the txIn is not mature", logging.LogFormat{"txid": txHash.String(), "index": index})
						continue
					}
				}

				totalBinding, err = totalBinding.AddInt(bindingTx.Value)
				if err != nil {
					return err
				}
				prevOut := wire.NewOutPoint(txHash, index)
				txIn := wire.NewTxIn(prevOut, witness)
				txIns = append(txIns, txIn)
				if totalBinding.Cmp(valueRequired) >= 0 {
					hasValidBinding = true
					break
				}
				bindingNum++
				if bindingNum >= MaxBindingNum {
					break
				}
			}
		} else {
			if bitLength != bitLengthMissing {
				logging.CPrint(logging.DEBUG, "invalid bit length",
					logging.LogFormat{"bitLength": bitLength})
			}
		}
	} else {
		logging.CPrint(logging.INFO, "No binding tx in the pubkey")
	}

	miner, superNode, err := CalcBlockSubsidy(nextBlockHeight, &config.ChainParams, totalBinding, len(rewardAddresses), bitLength)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on CalcBlockSubsidy", logging.LogFormat{
			"err":                    err,
			"height":                 nextBlockHeight,
			"total_binding":          totalBinding,
			"reward_addresses_count": len(rewardAddresses),
			"bit_length":             bitLength,
		})
		return err
	}
	// mint
	diff := massutil.ZeroAmount()
	if !miner.IsZero() {
		if hasValidBinding {
			for _, txIn := range txIns {
				coinbase.AddTxIn(txIn)
			}
		}

		totalWeight := safetype.NewUint128()
		for _, v := range rewardAddresses {
			if nextBlockHeight < consensus.Massip1Activation { // by value
				totalWeight, err = totalWeight.AddInt(v.Value)
			} else { // by weight
				totalWeight, err = totalWeight.Add(v.Weight)
			}
			if err != nil {
				return err
			}
		}

		logging.CPrint(logging.INFO, "show the count of stakingTx", logging.LogFormat{"count": len(rewardAddresses)})

		// calc reward
		totalSNValue := massutil.ZeroAmount()
		for i := 0; i < len(rewardAddresses); i++ {
			key := make([]byte, sha256.Size)
			copy(key, rewardAddresses[i].ScriptHash[:])
			pkScriptSuperNode, err := txscript.PayToWitnessScriptHashScript(key)
			if err != nil {
				return err
			}

			nodeWeight := rewardAddresses[i].Weight
			if nextBlockHeight < consensus.Massip1Activation {
				nodeWeight, err = safetype.NewUint128FromInt(rewardAddresses[i].Value)
				if err != nil {
					return err
				}
			}
			nodeReward, err := calcNodeReward(superNode, totalWeight, nodeWeight)
			if err != nil {
				return err
			}

			if nodeReward.IsZero() {
				// break loop as rewordAddress is in descending order by value
				break
			}
			totalSNValue, err = totalSNValue.Add(nodeReward)
			if err != nil {
				return err
			}
			coinbase.AddTxOut(&wire.TxOut{
				Value:    nodeReward.IntValue(),
				PkScript: pkScriptSuperNode,
			})
		}
		coinbase.SetPayload(standardCoinbasePayload(nextBlockHeight, uint32(len(coinbase.TxOut))))

		diff, err = superNode.Sub(totalSNValue)
		if err != nil {
			return err
		}

		// add diff from staking tx
		miner, err = miner.Add(diff)
		if err != nil {
			return err
		}
	}

	miner, err = miner.Add(totalFee)
	if err != nil {
		return err
	}
	coinbase.AddTxOut(&wire.TxOut{
		PkScript: minerPkScript,
		Value:    miner.IntValue(),
	})

	return
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate subsidy
// based on the passed block height to the provided wallet.  When the wallet
// is nil, the coinbase transaction will instead be redeemable by anyone.
//
// See the comment for NewBlockTemplate for more information about why the nil
// address handling is useful.

func createCoinbaseTx(nextBlockHeight uint64, addr massutil.Address, rewardAddresses []database.Rank) (*massutil.Tx, error) {
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&wire.Hash{},
			wire.MaxPrevOutIndex),
		Sequence: wire.MaxTxInSequenceNum,
	})
	tx.SetPayload(standardCoinbasePayload(nextBlockHeight, 0))

	// Create a script for paying to the miner if one was specified.
	// Otherwise create a script that allows the coinbase to be
	// redeemable by anyone.
	pkScriptMiner := anyoneRedeemableScript
	var err error
	if addr != nil {
		pkScriptMiner, err = txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, err
		}
	}

	miner, _, err := CalcBlockSubsidy(nextBlockHeight,
		&config.ChainParams, massutil.ZeroAmount(), len(rewardAddresses), bitLengthMissing)
	if err != nil {
		return nil, err
	}
	// no longer mint
	if miner.IsZero() {
		tx.AddTxOut(&wire.TxOut{
			Value:    miner.IntValue(),
			PkScript: pkScriptMiner,
		})
		return massutil.NewTx(tx), nil
	}

	// mint

	//diff := safetype.NewUint64()
	//totalStakingValue := safetype.NewUint64()
	//for _, v := range rewardAddresses {
	//	totalStakingValue, err = totalStakingValue.AddInt(v.Value)
	//	if err != nil {
	//		return nil, err
	//	}
	//}

	// calc reward
	//totalSNValue := safetype.NewUint64()
	for i := 0; i < len(rewardAddresses); i++ {
		key := make([]byte, sha256.Size)
		copy(key, rewardAddresses[i].ScriptHash[:])
		pkScriptSuperNode, err := txscript.PayToWitnessScriptHashScript(key)
		if err != nil {
			return nil, err
		}
		//superNodeValue, err := calcSuperNodeReward(superNode, totalStakingValue, rewardAddresses[i].Value)
		//if err != nil {
		//	return nil, err
		//}
		//if superNodeValue.IsZero() {
		//	// break loop as rewardAddresses is in descending order by value
		//	break
		//}
		//totalSNValue, err = totalSNValue.Add(superNodeValue)
		//if err != nil {
		//	return nil, err
		//}
		tx.AddTxOut(&wire.TxOut{
			Value:    0,
			PkScript: pkScriptSuperNode,
		})
	}

	//miner, err = miner.Add(diff)
	//if err != nil {
	//	return nil, err
	//}
	tx.AddTxOut(&wire.TxOut{
		Value:    miner.IntValue(),
		PkScript: pkScriptMiner,
	})

	return massutil.NewTx(tx), nil
}

func calcNodeReward(totalReward massutil.Amount, totalWeight, nodeWeight *safetype.Uint128) (massutil.Amount, error) {
	u, err := totalReward.Value().Mul(nodeWeight)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	u, err = u.Div(totalWeight)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	return massutil.NewAmount(u)
}

// logSkippedDeps logs any dependencies which are also skipped as a result of
// skipping a transaction while generating a block template at the trace level.
func logSkippedDeps(tx *massutil.Tx, deps *list.List) {
	if deps == nil {
		return
	}

	for e := deps.Front(); e != nil; e = e.Next() {
		item := e.Value.(*txPrioItem)
		logging.CPrint(logging.TRACE, "skipping tx since it depends on tx which is already skipped", logging.LogFormat{"txid": item.tx.Hash().String(), "depend": tx.Hash().String()})

	}
}

// NewBlockTemplate returns a new block template that is ready to be solved
// using the transactions from the passed transaction memory pool and a coinbase
// that either pays to the passed address if it is not nil, or a coinbase that
// is redeemable by anyone if the passed wallet is nil.  The nil wallet
// functionality is useful since there are cases such as the getblocktemplate
// RPC where external mining software is responsible for creating their own
// coinbase which will replace the one generated for the block template.  Thus
// the need to have configured address can be avoided.
//
// The transactions selected and included are prioritized according to several
// factors.  First, each transaction has a priority calculated based on its
// value, age of inputs, and size.  Transactions which consist of larger
// amounts, older inputs, and small sizes have the highest priority.  Second, a
// fee per kilobyte is calculated for each transaction.  Transactions with a
// higher fee per kilobyte are preferred.  Finally, the block generation related
// configuration options are all taken into account.
//
// Transactions which only spend outputs from other transactions already in the
// block chain are immediately added to a priority queue which either
// prioritizes based on the priority (then fee per kilobyte) or the fee per
// kilobyte (then priority) depending on whether or not the BlockPrioritySize
// configuration option allots space for high-priority transactions.
// Transactions which spend outputs from other transactions in the memory pool
// are added to a dependency map so they can be added to the priority queue once
// the transactions they depend on have been included.
//
// Once the high-priority area (if configured) has been filled with transactions,
// or the priority falls below what is considered high-priority, the priority
// queue is updated to prioritize by fees per kilobyte (then priority).
//
// When the fees per kilobyte drop below the TxMinFreeFee configuration option,
// the transaction will be skipped unless there is a BlockMinSize set, in which
// case the block will be filled with the low-fee/free transactions until the
// block size reaches that minimum size.
//
// Any transactions which would cause the block to exceed the BlockMaxSize
// configuration option, exceed the maximum allowed signature operations per
// block, or otherwise cause the Block to be invalid are skipped.
//
// Given the above, a block generated by this function is of the following form:
//
//   -----------------------------------  --  --
//  |      Coinbase Transaction         |   |   |
//  |-----------------------------------|   |   |
//  |                                   |   |   | ----- cfg.BlockPrioritySize
//  |   High-priority Transactions      |   |   |
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |--- cfg.BlockMaxSize
//  |  Transactions prioritized by fee  |   |
//  |  until <= cfg.TxMinFreeFee        |   |
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |
//  |-----------------------------------|   |
//  |  Low-fee/Non high-priority (free) |   |
//  |  transactions (while block size   |   |
//  |  <= cfg.BlockMinSize)             |   |
//   -----------------------------------  --

func (chain *Blockchain) NewBlockTemplate(payoutAddress massutil.Address, templateCh chan interface{}) error {
	chain.l.Lock()
	defer chain.l.Unlock()

	// Get snapshot of chain/txPool
	bestNode := chain.blockTree.bestBlockNode()
	txs := chain.txPool.TxDescs()
	punishments := chain.proposalPool.PunishmentProposals()
	rewardAddress, err := chain.db.FetchUnexpiredStakingRank(bestNode.Height+1, true)
	if err != nil {
		return err
	}

	// run newBlockTemplate as goroutine
	go newBlockTemplate(chain, payoutAddress, templateCh, bestNode, txs, punishments, rewardAddress)
	return nil
}

func newBlockTemplate(chain *Blockchain, payoutAddress massutil.Address, templateCh chan interface{},
	bestNode *BlockNode, mempoolTxns []*TxDesc, proposals []*PunishmentProposal, rewardAddresses []database.Rank) {

	nextBlockHeight := bestNode.Height + 1
	challenge, err := calcNextChallenge(bestNode)
	if err != nil {
		templateCh <- &PoCTemplate{
			Err: err,
		}
		return
	}

	coinbaseTx, err := createCoinbaseTx(nextBlockHeight, payoutAddress, rewardAddresses)
	if err != nil {
		templateCh <- &PoCTemplate{
			Err: err,
		}
		return
	}

	getCoinbaseTx := func(pubkey *pocec.PublicKey, totalFee massutil.Amount, bitLength int) (*massutil.Tx, error) {
		pkScriptHash, err := pkToScriptHash(pubkey.SerializeCompressed(), &config.ChainParams)
		if err != nil {
			return nil, err
		}

		BindingTxListReply, err := chain.db.FetchScriptHashRelatedBindingTx(pkScriptHash, &config.ChainParams)
		if err != nil {
			return nil, err
		}
		err = reCreateCoinbaseTx(coinbaseTx.MsgTx(), BindingTxListReply, nextBlockHeight, bitLength, rewardAddresses, totalFee)
		if err != nil {
			return nil, err
		}
		return coinbaseTx, nil
	}

	getTarget := func(timestamp time.Time) *big.Int {
		target, _ := calcNextTarget(bestNode, timestamp)
		return target
	}

	templateCh <- &PoCTemplate{
		Height:        nextBlockHeight,
		Timestamp:     bestNode.Timestamp.Add(1 * poc.PoCSlot * time.Second),
		Previous:      *bestNode.Hash,
		Challenge:     *challenge,
		GetTarget:     getTarget,
		RewardAddress: rewardAddresses,
		GetCoinbase:   getCoinbaseTx,
	}

	numCoinbaseSigOps := int64(CountSigOps(coinbaseTx))

	// Get the current memory pool transactions and create a priority queue
	// to hold the transactions which are ready for inclusion into a block
	// along with some priority related and fee metadata.  Reserve the same
	// number of items that are in the memory pool for the priority queue.
	// Also, choose the initial sort order for the priority queue based on
	// whether or not there is an area allocated for high-priority
	// transactions.
	depMap := make(map[wire.Hash]struct{})
	for _, txDesc := range mempoolTxns {
		txhash := txDesc.Tx.Hash()
		if _, exist := depMap[*txhash]; !exist {
			depMap[*txhash] = struct{}{}
		}
	}
	sortedByFee := config.BlockPrioritySize == 0
	priorityQueue := newTxPriorityQueue(len(mempoolTxns), sortedByFee)

	// Create a slice to hold the transactions to be included in the
	// generated block with reserved space.  Also create a transaction
	// store to house all of the input transactions so multiple lookups
	// can be avoided.
	blockTxns := make([]*wire.MsgTx, 0, len(mempoolTxns))
	blockTxns = append(blockTxns, coinbaseTx.MsgTx())
	//blockTxStore := make(TxStore)
	punishProposals := make([]*wire.FaultPubKey, 0, len(proposals))
	otherProposals := make([]*wire.NormalProposal, 0)

	var totalProposalsSize int
	totalProposalsSize += 4

	// dependers is used to track transactions which depend on another
	// transaction in the memory pool.  This, in conjunction with the
	// dependsOn map kept with each dependent transaction helps quickly
	// determine which dependent transactions are now eligible for inclusion
	// in the block once each transaction has been included.
	dependers := make(map[wire.Hash]*list.List)

	// Create slices to hold the fees and number of signature operations
	// for each of the selected transactions and add an entry for the
	// coinbase.  This allows the code below to simply append details about
	// a transaction as it is selected for inclusion in the final block.
	// However, since the total fees aren't known yet, use a dummy value for
	// the coinbase fee which will be updated later.
	txSigOpCounts := make([]int64, 0, len(mempoolTxns))
	txSigOpCounts = append(txSigOpCounts, numCoinbaseSigOps)

	logging.CPrint(logging.DEBUG, "Considering transactions in mempool for inclusion to new block", logging.LogFormat{"tx count": len(mempoolTxns)})

	banList := make([]*pocec.PublicKey, 0)
	if len(proposals) != 0 {
		for _, proposal := range proposals {
			if totalProposalsSize+proposal.Size <= PriorityProposalSize {
				totalProposalsSize += proposal.Size
				punishProposals = append(punishProposals, proposal.FaultPubKey)
				banList = append(banList, proposal.PubKey)
			}
		}
	} else {
		totalProposalsSize += wire.HeaderSizePerPlaceHolder * 2
	}

	//mempoolLoop:
	for _, txDesc := range mempoolTxns {
		// A block can't have more than one coinbase or contain
		// non-finalized transactions.
		tx := txDesc.Tx

		// Setup dependencies for any transactions which reference
		// other transactions in the mempool so they can be properly
		// ordered below.
		prioItem := &txPrioItem{tx: txDesc.Tx}
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			// because mempoolTxns is snapshot of mempool, its tx can not orphan
			if _, exist := depMap[*originHash]; exist {
				// The transaction is referencing another
				// transaction in the memory pool, so setup an
				// ordering dependency.
				depList, exists := dependers[*originHash]
				if !exists {
					depList = list.New()
					dependers[*originHash] = depList
				}
				depList.PushBack(prioItem)
				if prioItem.dependsOn == nil {
					prioItem.dependsOn = make(
						map[wire.Hash]struct{})
				}
				prioItem.dependsOn[*originHash] = struct{}{}
			}
		}

		// Calculate the final transaction priority using the input
		// value age sum as well as the adjusted transaction size.  The
		// formula is: sum(inputValue * inputAge) / adjustedTxSize
		prioItem.priority = float64(txDesc.totalInputValue.IntValue())*(float64(nextBlockHeight)-float64(txDesc.Height)) + txDesc.startingPriority

		// Calculate the fee in Maxwell/KB.
		// NOTE: This is a more precise value than the one calculated
		// during calcMinRelayFee which rounds up to the nearest full
		// kilobyte boundary.  This is beneficial since it provides an
		// incentive to create smaller transactions.
		txSize := tx.MsgTx().PlainSize()
		prioItem.feePerKB = float64(txDesc.Fee.IntValue()) / (float64(txSize) / 1000)
		prioItem.fee = txDesc.Fee.IntValue()

		// Add the transaction to the priority queue to mark it ready
		// for inclusion in the block unless it has dependencies.
		if prioItem.dependsOn == nil {
			heap.Push(priorityQueue, prioItem)
		}
	}

	logging.CPrint(logging.TRACE, "Check the length of priority queue and dependent",
		logging.LogFormat{"priority": priorityQueue.Len(), "dependent": len(dependers)})

	// The starting block size is the size of the Block header plus the max
	// possible transaction count size, plus the size of the coinbase
	// transaction.
	//modify: 360 is coinbase`s binding txIn
	blockSize := uint32(blockHeaderOverhead+int64(len(punishProposals)*33)+int64(coinbaseTx.PlainSize())+int64(totalProposalsSize)) + bindingPayload
	blockSigOps := numCoinbaseSigOps
	totalFee := massutil.ZeroAmount()

	// Choose which transactions make it into the block.
	for priorityQueue.Len() > 0 {
		// Grab the highest priority (or highest fee per kilobyte
		// depending on the sort order) transaction.
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		tx := prioItem.tx

		// Grab the list of transactions which depend on this one (if
		// any) and remove the entry for this transaction as it will
		// either be included or skipped, but in either case the deps
		// are no longer needed.
		deps := dependers[*tx.Hash()]
		delete(dependers, *tx.Hash())
		// Enforce maximum block size.  Also check for overflow.
		txSize := uint32(tx.PlainSize())
		blockPlusTxSize := blockSize + txSize
		// Enforce maximum block size.  Also check for overflow.

		if blockPlusTxSize < blockSize ||
			blockPlusTxSize >= config.BlockMaxSize {
			logging.CPrint(logging.TRACE, "Skipping tx because it would exceed the max block weight",
				logging.LogFormat{"txid": tx.Hash().String()})
			logSkippedDeps(tx, deps)
			continue
		}

		// Enforce maximum signature operations per block.  Also check
		// for overflow.
		numSigOps := int64(CountSigOps(tx))
		if blockSigOps+numSigOps < blockSigOps || blockSigOps+numSigOps > MaxSigOpsPerBlock {
			logging.CPrint(logging.TRACE, "Skipping tx because it would exceed the maximum sigops per block",
				logging.LogFormat{"txid": tx.Hash().String()})
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip free transactions once the block is larger than the
		// minimum block size.
		if sortedByFee &&
			prioItem.feePerKB < float64(consensus.MinRelayTxFee) &&
			blockPlusTxSize >= config.BlockMinSize {

			logging.CPrint(logging.TRACE, "Skipping tx with feePerKB < TxMinFreeFee and block weight >= minBlockSize",
				logging.LogFormat{"txid": tx.Hash().String(), "feePerKB": prioItem.feePerKB, "TxMinFreeFee": consensus.MinRelayTxFee, "block weight": blockPlusTxSize, "minBlockSize": config.BlockMinSize})
			logSkippedDeps(tx, deps)
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxSize >= config.BlockPrioritySize ||
			prioItem.priority <= minHighPriority) {

			logging.CPrint(logging.TRACE, "Switching to sort by fees per kilobyte since blockSize >= BlockPrioritySize || priority <= minHighPriority",
				logging.LogFormat{
					"block size":        blockPlusTxSize,
					"BlockPrioritySize": config.BlockPrioritySize,
					"priority":          fmt.Sprintf("%.2f", prioItem.priority),
					"minHighPriority":   fmt.Sprintf("%.2f", minHighPriority)})

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-priortized by fees if it won't
			// fit into the high-priority section or the priority
			// is too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxSize > config.BlockPrioritySize ||
				prioItem.priority < minHighPriority {

				heap.Push(priorityQueue, prioItem)
				continue
			}
		}

		temp, err := totalFee.AddInt(prioItem.fee)
		if err != nil {
			logging.CPrint(logging.ERROR, "calc total fee error",
				logging.LogFormat{
					"err":  err,
					"txid": tx.Hash().String(),
				})
			continue
		}

		// Add the transaction to the block, increment counters, and
		// save the fees and signature operation counts to the block
		// template.
		blockTxns = append(blockTxns, tx.MsgTx())
		blockSize += txSize
		blockSigOps += numSigOps
		totalFee = temp
		txSigOpCounts = append(txSigOpCounts, numSigOps)

		logging.CPrint(logging.TRACE, "Adding tx",
			logging.LogFormat{"txid": tx.Hash().String(),
				"priority": fmt.Sprintf("%.2f", prioItem.priority),
				"feePerKB": fmt.Sprintf("%.2f", prioItem.feePerKB)})

		// Add transactions which depend on this one (and also do not
		// have any other unsatisified dependencies) to the priority
		// queue.
		if deps != nil {
			for e := deps.Front(); e != nil; e = e.Next() {
				// Add the transaction to the priority queue if
				// there are no more dependencies after this
				// one.
				item := e.Value.(*txPrioItem)
				delete(item.dependsOn, *tx.Hash())
				if len(item.dependsOn) == 0 {
					heap.Push(priorityQueue, item)
				}
			}
		}
	}

	// Next, obtain the merkle root of a tree which consists of the
	// wtxid of all transactions in the block. The coinbase
	// transaction will have a special wtxid of all zeroes.
	witnessMerkleTree := wire.BuildMerkleTreeStoreTransactions(blockTxns, true)
	witnessMerkleRoot := *witnessMerkleTree[len(witnessMerkleTree)-1]

	// Create a new block ready to be solved.
	// we will get chainID, Version, Height, Previous, TransactionRoot, CommitRoot,
	// ProposalRoot, BanList, tx and proposal
	merkles := wire.BuildMerkleTreeStoreTransactions(blockTxns, false)
	var msgBlock = wire.NewEmptyMsgBlock()
	msgBlock.Header = *wire.NewEmptyBlockHeader()
	msgBlock.Header.ChainID = bestNode.ChainID
	msgBlock.Header.Version = generatedBlockVersion
	msgBlock.Header.Height = nextBlockHeight
	msgBlock.Header.Previous = *bestNode.Hash
	msgBlock.Header.TransactionRoot = *merkles[len(merkles)-1]
	msgBlock.Header.WitnessRoot = witnessMerkleRoot

	merklesCache := make([]*wire.Hash, 0)
	merklesCache = append(merklesCache, merkles[0])
	if len(merkles) > 1 {
		base := (len(merkles) + 1) / 2
		for i := 1; i < len(merkles) && base > 0; {
			merklesCache = append(merklesCache, merkles[i])
			i += base
			base = base / 2
		}
	}

	witnessMerklesCache := make([]*wire.Hash, 0)
	witnessMerklesCache = append(witnessMerklesCache, witnessMerkleTree[0])
	if len(witnessMerkleTree) > 1 {
		base := (len(witnessMerkleTree) + 1) / 2
		for i := 1; i < len(witnessMerkleTree) && base > 0; {
			witnessMerklesCache = append(witnessMerklesCache, witnessMerkleTree[i])
			i += base
			base = base / 2
		}
	}

	proposalArea, err := wire.NewProposalArea(punishProposals, otherProposals)
	if err != nil {
		templateCh <- &BlockTemplate{
			Err: err,
		}
		return
	}
	proposalMerkles := wire.BuildMerkleTreeStoreForProposal(proposalArea)
	proposalRoot := proposalMerkles[len(proposalMerkles)-1]
	msgBlock.Header.ProposalRoot = *proposalRoot
	msgBlock.Proposals = *proposalArea

	msgBlock.Header.BanList = banList

	for _, tx := range blockTxns {
		msgBlock.AddTransaction(tx)
	}

	// Finally, perform a full check on the created block against the chain
	// consensus rules to ensure it properly connects to the current best
	// chain with no issues.
	block := massutil.NewBlock(msgBlock)
	block.SetHeight(nextBlockHeight)

	//if !msgBlock.Header.Previous.IsEqual(chain.GetBestChainHash()) {
	//	return nil, errors.New("block template stale")
	//}

	//timeSource := NewMedianTime()
	if _, err := chain.execProcessBlock(block, BFNoPoCCheck); err != nil {
		templateCh <- &BlockTemplate{
			Err: err,
		}
		return
	}

	logging.CPrint(logging.DEBUG, "Created new block template",
		logging.LogFormat{
			"tx count":                  len(msgBlock.Transactions),
			"total fee":                 totalFee,
			"signature operations cost": blockSigOps,
			"block size":                blockSize,
			"target difficulty":         fmt.Sprintf("%064x", msgBlock.Header.Target)})
	templateCh <- &BlockTemplate{
		Block:              msgBlock,
		TotalFee:           totalFee,
		SigOpCounts:        txSigOpCounts,
		Height:             nextBlockHeight,
		ValidPayAddress:    payoutAddress != nil,
		MerkleCache:        merklesCache,
		WitnessMerkleCache: witnessMerklesCache,
	}
}
