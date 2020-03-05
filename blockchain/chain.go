package blockchain

import (
	"container/list"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"time"

	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

// SequenceLock represents the converted relative lock-time in seconds, and
// absolute block-height for a transaction input's relative lock-times.
// According to SequenceLock, after the referenced input has been confirmed
// within a block, a transaction spending that input can be included into a
// block either after 'seconds' (according to past median time), or once the
// 'BlockHeight' has been reached.
type SequenceLock struct {
	Seconds     int64
	BlockHeight uint64
}

// CalcSequenceLock computes a relative lock-time SequenceLock for the passed
// transaction using the passed UtxoViewpoint to obtain the past median time
// for blocks in which the referenced inputs of the transactions were included
// within. The generated SequenceLock lock can be used in conjunction with a
// block height, and adjusted median block time to determine if all the inputs
// referenced within a transaction have reached sufficient maturity allowing
// the candidate transaction to be included in a block.
//
// This function is safe for concurrent access.
func (chain *Blockchain) CalcSequenceLock(tx *massutil.Tx, txStore TxStore) (*SequenceLock, error) {
	return chain.calcSequenceLock(chain.blockTree.bestBlockNode(), tx, txStore)
}

// calcSequenceLock computes the relative lock-times for the passed
// transaction. See the exported version, CalcSequenceLock for further details.
//
// This function MUST be called with the chain state lock held (for writes).
func (chain *Blockchain) calcSequenceLock(node *BlockNode, tx *massutil.Tx, txStore TxStore) (*SequenceLock, error) {
	// A value of -1 for each relative lock type represents a relative time
	// lock value that will allow a transaction to be included in a block
	// at any given height or time. This value is returned as the relative
	// lock time in the case that BIP 68 is disabled, or has not yet been
	// activated.
	sequenceLock := &SequenceLock{Seconds: 0, BlockHeight: 0}
	mTx := tx.MsgTx()
	// Grab the next height from the PoV of the passed BlockNode to use for
	// inputs present in the mempool.
	nextHeight := node.Height + 1
	if IsCoinBase(tx) {
		return sequenceLock, nil
	}
	for txInIndex, txIn := range mTx.TxIn {
		utxo := txStore[txIn.PreviousOutPoint.Hash]
		if utxo == nil {
			logging.CPrint(logging.ERROR, "referenced transaction either does not exist or has already been spent",
				logging.LogFormat{"referenced transaction ": txIn.PreviousOutPoint, "transaction": tx.Hash(), "index": txInIndex})
			return sequenceLock, ErrMissingTx
		}
		// If the input height is set to the mempool height, then we
		// assume the transaction makes it into the next block when
		// evaluating its sequence blocks.
		coinHeight := utxo.BlockHeight
		if coinHeight == mempoolHeight {
			coinHeight = nextHeight
		}

		// Given a sequence number, we apply the relative time lock
		// mask in order to obtain the time lock delta required before
		// this input can be spent.
		sequenceNum := txIn.Sequence
		relativeLock := sequenceNum & wire.SequenceLockTimeMask

		switch {
		// Relative time locks are disabled for this input, so we can
		// skip any further calculation.
		case sequenceNum&wire.SequenceLockTimeDisabled == wire.SequenceLockTimeDisabled:
			continue
		case sequenceNum&wire.SequenceLockTimeIsSeconds == wire.SequenceLockTimeIsSeconds:
			// This input requires a relative time lock expressed
			// in seconds before it can be spent.  Therefore, we
			// need to query for the block prior to the one in
			// which this input was included within so we can
			// compute the past median time for the block prior to
			// the one which included this referenced output.
			prevInputHeight := coinHeight - 1
			if prevInputHeight < 0 {
				prevInputHeight = 0
			}
			blockNode := node.Ancestor(prevInputHeight)
			medianTime, _ := chain.calcPastMedianTime(blockNode)
			//medianTime := BlockNode.CalcPastMedianTime()

			// Time based relative time-locks as defined by BIP 68
			// have a time granularity of RelativeLockSeconds, so
			// we shift left by this amount to convert to the
			// proper relative time-lock. We also subtract one from
			// the relative lock to maintain the original lockTime
			// semantics.
			timeLockSeconds := (relativeLock << wire.SequenceLockTimeGranularity) - 1
			timeLock := medianTime.Unix() + int64(timeLockSeconds)
			if timeLock > sequenceLock.Seconds {
				sequenceLock.Seconds = timeLock
			}
		default:
			// The relative lock-time for this input is expressed
			// in blocks so we calculate the relative offset from
			// the input's height as its converted absolute
			// lock-time. We subtract one from the relative lock in
			// order to maintain the original lockTime semantics.
			blockHeight := coinHeight + relativeLock - 1
			if blockHeight > sequenceLock.BlockHeight {
				sequenceLock.BlockHeight = blockHeight
			}
		}
	}

	return sequenceLock, nil
}

// LockTimeToSequence converts the passed relative locktime to a sequence
// number in accordance to BIP-68.
// See: https://github.com/bitcoin/bips/blob/master/bip-0068.mediawiki
//  * (Compatibility)
func LockTimeToSequence(isSeconds bool, locktime uint64) uint64 {
	// If we're expressing the relative lock time in blocks, then the
	// corresponding sequence number is simply the desired input age.
	if !isSeconds {
		return locktime
	}

	// Set the 22nd bit which indicates the lock time is in seconds, then
	// shift the locktime over by 9 since the time granularity is in
	// 512-second intervals (2^9). This results in a max lock-time of
	// 33,553,920 seconds, or 1.1 years.
	return wire.SequenceLockTimeIsSeconds |
		locktime>>wire.SequenceLockTimeGranularity
}

// calcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the passed block node.  It is primarily used to
// validate new blocks have sane timestamps.
func (chain *Blockchain) calcPastMedianTime(startNode *BlockNode) (time.Time, error) {
	// Genesis block.
	if startNode == nil {
		return chain.info.genesisTime, nil
	}

	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]time.Time, medianTimeBlocks)
	numNodes := 0
	iterNode := startNode
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.Timestamp
		numNodes++

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.Parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = chain.getPrevNodeFromNode(iterNode)
		if err != nil {
			logging.CPrint(logging.ERROR, "getPrevNodeFromNode", logging.LogFormat{"err": err, "iterNode": iterNode})
			//	log.Errorf("getPrevNodeFromNode: %v", err)
			return time.Time{}, err
		}
	}

	// Prune the slice to the actual number of available timestamps which
	// will be fewer than desired near the beginning of the block chain
	// and sort them.
	timestamps = timestamps[:numNodes]
	sort.Sort(timeSorter(timestamps))

	// NOTE: bitcoind incorrectly calculates the median for even numbers of
	// blocks.  A true median averages the middle two elements for a set
	// with an even number of elements in it.   Since the constant for the
	// previous number of blocks to be used is odd, this is only an issue
	// for a few blocks near the beginning of the chain.  I suspect this is
	// an optimization even though the result is slightly wrong for a few
	// of the first blocks since after the first few blocks, there will
	// always be an odd number of blocks in the set per the constant.
	//
	// This code follows suit to ensure the same rules are used as bitcoind
	// however, be aware that should the medianTimeBlocks constant ever be
	// changed to an even number, this code will be wrong.
	medianTimestamp := timestamps[numNodes/2]
	return medianTimestamp, nil
}

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the end of the current best chain.  It is primarily
// used to ensure new blocks have sane timestamps.
//
// This function is NOT safe for concurrent access.
func (chain *Blockchain) CalcPastMedianTime() (time.Time, error) {
	return chain.calcPastMedianTime(chain.blockTree.bestBlockNode())
}

// getReorganizeNodes finds the fork point between the main chain and the passed
// node and returns a list of block nodes that would need to be detached from
// the main chain and a list of block nodes that would need to be attached to
// the fork point (which will be the end of the main chain after detaching the
// returned list of block nodes) in order to reorganize the chain such that the
// passed node is the new end of the main chain.  The lists will be empty if the
// passed node is not on a side chain.
func (chain *Blockchain) getReorganizeNodes(node *BlockNode) (*list.List, *list.List) {
	// Nothing to detach or attach if there is no node.
	attachNodes := list.New()
	detachNodes := list.New()
	if node == nil {
		return detachNodes, attachNodes
	}

	// Find the fork point (if any) adding each block to the list of nodes
	// to attach to the main tree.  Push them onto the list in reverse order
	// so they are attached in the appropriate order when iterating the list
	// later.
	ancestor := node
	for ; ancestor.Parent != nil; ancestor = ancestor.Parent {
		if ancestor.InMainChain {
			break
		}
		attachNodes.PushFront(ancestor)
	}

	// Start from the end of the main chain and work backwards until the
	// common ancestor adding each block to the list of nodes to detach from
	// the main chain.
	for n := chain.blockTree.bestBlockNode(); n != nil && n.Parent != nil; n = n.Parent {
		if n.Hash.IsEqual(ancestor.Hash) {
			break
		}
		detachNodes.PushBack(n)
	}

	return detachNodes, attachNodes
}

// connectBlock handles connecting the passed node/block to the end of the main
// (best) chain.
// TODO: perform benchmark test here
func (chain *Blockchain) connectBlock(node *BlockNode, block *massutil.Block) error {
	// Make sure it's extending the end of the best chain.
	prevHash := &block.MsgBlock().Header.Previous
	if !prevHash.IsEqual(chain.blockTree.bestBlockNode().Hash) {
		return errConnectMainChain
	}

	txInputStore, err := chain.fetchInputTransactions(node, block)
	if err != nil {
		return err
	}
	// Insert the block into the database which houses the main chain.
	if err = chain.db.SubmitBlock(block); err != nil {
		return err
	}
	if err = chain.addrIndexer.SyncAttachBlock(block, txInputStore); err != nil {
		return err
	}
	if err = chain.db.Commit(*node.Hash); err != nil {
		return err
	}

	// Add the new node to the memory main chain
	node.InMainChain = true

	// This node is now the end of the best chain.
	chain.blockTree.setBestBlockNode(node)

	// Expand banListTree
	if ok, hashA, hashB := chain.dmd.applyBlockNode(node); !ok {
		if err := chain.submitFaultPubKeyFromHash(hashA, hashB); err == nil {
			logging.CPrint(logging.INFO, "faultPubKey submitted to ProposalPool",
				logging.LogFormat{
					"hashA":     hashA,
					"hashB":     hashB,
					"pubKey":    hex.EncodeToString(node.PubKey.SerializeCompressed()),
					"bitLength": node.Proof.BitLength,
					"height":    node.Height,
				})
		} else {
			logging.CPrint(logging.ERROR, "fail to submit FaultPubKey", logging.LogFormat{
				"err":   err,
				"hashA": hashA,
				"hashB": hashB,
			})
		}
	}

	// wait for other modules to attach block
	chain.attachBlock(block)

	return nil
}

func (chain *Blockchain) attachBlock(block *massutil.Block) {
	chain.proposalPool.SyncAttachBlock(block)
	chain.txPool.SyncAttachBlock(block)
	return
}

// disconnectBlock handles disconnecting the passed node/block from the end of
// the main (best) chain.
func (chain *Blockchain) disconnectBlock(node *BlockNode, block *massutil.Block) error {
	// Make sure the node being disconnected is the end of the best chain.
	if !node.Hash.IsEqual(chain.blockTree.bestBlockNode().Hash) {
		return errDisconnectMainChain
	}

	// Remove the block from the database which houses the main chain.

	if err := chain.db.DeleteBlock(node.Hash); err != nil {
		return err
	}
	if err := chain.addrIndexer.SyncDetachBlock(block); err != nil {
		return err
	}
	if err := chain.db.Commit(*node.Hash); err != nil {
		return err
	}

	// Put block in the side chain cache.
	node.InMainChain = false
	chain.blockCache.addBlock(block)

	// This node's Parent is now the end of the best chain.
	chain.blockTree.setBestBlockNode(node.Parent)

	// detach block from other modules
	chain.detachBlock(block)

	return nil
}

func (chain *Blockchain) detachBlock(block *massutil.Block) {
	chain.proposalPool.SyncDetachBlock(block)
	chain.txPool.SyncDetachBlock(block)
	return
}

// reorganizeChain reorganizes the block chain by disconnecting the nodes in the
// detachNodes list and connecting the nodes in the attach list.  It expects
// that the lists are already in the correct order and are in sync with the
// end of the current best chain.  Specifically, nodes that are being
// disconnected must be in reverse order (think of popping them off
// the end of the chain) and nodes the are being attached must be in forwards
// order (think pushing them onto the end of the chain).
//
// The flags modify the behavior of this function as follows:
//  - BFDryRun: Only the checks which ensure the reorganize can be completed
//    successfully are performed.  The chain is not reorganized.
func (chain *Blockchain) reorganizeChain(detachNodes, attachNodes *list.List, flags BehaviorFlags) error {
	// Ensure all of the needed side chain blocks are in the cache.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*BlockNode)
		if _, err := chain.blockCache.getBlock(n.Hash); err != nil {
			return err
		}
	}

	// Perform several checks to verify each block that needs to be attached
	// to the main chain can be connected without violating any rules and
	// without actually connecting the block.
	//
	// NOTE: bitcoind does these checks directly when it connects a block.
	// The downside to that approach is that if any of these checks fail
	// after disconnecting some blocks or attaching others, all of the
	// operations have to be rolled back to get the chain back into the
	// state it was before the rule violation (or other failure).  There are
	// at least a couple of ways accomplish that rollback, but both involve
	// tweaking the chain.  This approach catches these issues before ever
	// modifying the chain.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*BlockNode)
		block, _ := chain.blockCache.getBlock(n.Hash)
		if err := chain.checkConnectBlock(n, block); err != nil {
			return err
		}
	}

	chain.l.Lock()
	defer chain.l.Unlock()
	// Disconnect blocks from the main chain.
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*BlockNode)
		block, err := chain.db.FetchBlockBySha(n.Hash)
		if err != nil {
			return err
		}
		err = chain.disconnectBlock(n, block)
		if err != nil {
			return err
		}
	}

	// Connect the new best chain blocks.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*BlockNode)
		block, _ := chain.blockCache.getBlock(n.Hash)
		if err := chain.connectBlock(n, block); err != nil {
			return err
		}
		// Do not delete, there's no need for this
		// chain.blockCache.delete(n.Hash)
	}

	// Log the point where the chain forked.
	firstAttachNode := attachNodes.Front().Value.(*BlockNode)
	forkNode, err := chain.getPrevNodeFromNode(firstAttachNode)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get pre node of forked chain", logging.LogFormat{
			"err":                 err,
			"first_attach_height": firstAttachNode.Height,
			"first_attach_hash":   firstAttachNode.Hash,
		})
		return err
	}

	// Log the old and new best chain heads.
	firstDetachNode := detachNodes.Front().Value.(*BlockNode)
	lastAttachNode := attachNodes.Back().Value.(*BlockNode)
	logging.CPrint(logging.INFO, "REORGANIZE: Chain forks", logging.LogFormat{
		"fork_hash":       forkNode.Hash,
		"fork_height":     forkNode.Height,
		"old_best_hash":   firstDetachNode.Hash,
		"old_best_height": firstDetachNode.Height,
		"new_best_hash":   lastAttachNode.Hash,
		"new_best_height": lastAttachNode.Height,
	})

	return nil
}

func (chain *Blockchain) connectBestChain(node *BlockNode, block *massutil.Block, flags BehaviorFlags) error {
	// extend block on current best chain
	if node.Parent.Hash.IsEqual(chain.blockTree.bestBlockNode().Hash) {
		// Perform several checks to verify the block can be connected
		// to the main chain (including whatever reorganization might
		// be necessary to get this node to the main chain) without
		// violating any rules and without actually connecting the
		// block.
		var err error
		if err = chain.checkConnectBlock(node, block); err != nil {
			return err
		}

		// Connect the block to the main chain.
		chain.l.Lock()
		if err = chain.connectBlock(node, block); err != nil {
			return err
		}
		// Connect the Parent node to this node.
		if err = chain.blockTree.attachBlockNode(node); err != nil {
			return err
		}
		chain.l.Unlock()

		// Broadcast new block on best chain
		chain.cond.Broadcast()

		return nil
	}

	// extend block on certain side chain, and decide new best chain
	logging.CPrint(logging.DEBUG, "adding block to side chain cache", logging.LogFormat{
		"block": node.Hash,
	})
	chain.blockCache.addBlock(block)
	node.InMainChain = false
	if err := chain.blockTree.attachBlockNode(node); err != nil {
		return err
	}

	// We're extending (or creating) a side chain, but
	// this new side chain is not enough to make it the new chain.
	if ok, _ := chain.isPotentialNewBestChain(node); !ok {
		// Find the fork point.
		fork := node
		for ; fork.Parent != nil; fork = fork.Parent {
			if fork.InMainChain {
				break
			}
		}

		// Log information about how the block is forking the chain.
		if fork.Hash.IsEqual(node.Parent.Hash) {
			logging.CPrint(logging.INFO, "FORK: Block forks the chain at height, but does not cause a reorganize",
				logging.LogFormat{
					"block_height": node.Height,
					"block_hash":   node.Hash,
					"fork_height":  fork.Height,
					"fork_hash":    fork.Hash,
				})
		} else {
			logging.CPrint(logging.INFO, "EXTEND FORK: Block extends a side chain which forks the chain",
				logging.LogFormat{
					"block_height": node.Height,
					"block_hash":   node.Hash,
					"fork_height":  fork.Height,
					"fork_hash":    fork.Hash,
				})
		}

		// Expand banListTree
		if ok, hashA, hashB := chain.dmd.applyBlockNode(node); !ok {
			if err := chain.submitFaultPubKeyFromHash(hashA, hashB); err == nil {
				logging.CPrint(logging.INFO, "faultPubKey submitted to ProposalPool",
					logging.LogFormat{
						"hashA":     hashA,
						"hashB":     hashB,
						"pubKey":    hex.EncodeToString(node.PubKey.SerializeCompressed()),
						"bitLength": node.Proof.BitLength,
						"height":    node.Height,
					})
			} else {
				logging.CPrint(logging.ERROR, "fail to submit FaultPubKey", logging.LogFormat{
					"err":   err,
					"hashA": hashA,
					"hashB": hashB,
				})
			}
		}

		return nil
	}

	// We're extending (or creating) a side chain and the cumulative work
	// for this new side chain is more than the old best chain, so this side
	// chain needs to become the main chain.  In order to accomplish that,
	// find the common ancestor of both sides of the fork, disconnect the
	// blocks that form the (now) old fork from the main chain, and attach
	// the blocks that form the new chain to the main chain starting at the
	// common ancestor (the point where the chain forked).
	detachNodes, attachNodes := chain.getReorganizeNodes(node)

	// Reorganize the chain.
	logging.CPrint(logging.INFO, "reorganize: block is causing a reorganize.", logging.LogFormat{
		"block": node.Hash,
	})
	err := chain.reorganizeChain(detachNodes, attachNodes, flags)
	if err != nil {
		return err
	}

	// Broadcast new block on best chain
	chain.cond.Broadcast()

	return nil
}

// isPotentialNewBestChain returns whether or not the side chain can be a new
// best chain. The Boolean is valid only when error is nil.
func (chain *Blockchain) isPotentialNewBestChain(sideChain *BlockNode) (bool, error) {
	// Create a copy of bestChain pointer
	bestChain := chain.blockTree.bestBlockNode()
	// These two chains must be valid
	if sideChain == nil || bestChain == nil {
		return false, fmt.Errorf("figure potential best chain: Cannot decide invalid chain")
	}
	// Step 1: Check CapSum, the larger the better
	if sideChain.CapSum.Cmp(bestChain.CapSum) != 0 {
		if sideChain.CapSum.Cmp(bestChain.CapSum) < 0 {
			return false, nil
		}
		return true, nil
	}
	// Step 2: Check timestamp, the earlier the better
	if sideChain.Timestamp.Unix() != bestChain.Timestamp.Unix() {
		if sideChain.Timestamp.Unix() > bestChain.Timestamp.Unix() {
			return false, nil
		}
		return true, nil
	}
	// Step 3: Check quality, choose the better one.
	if sideChain.Quality.Cmp(bestChain.Quality) != 0 {
		if sideChain.Quality.Cmp(bestChain.Quality) > 0 {
			return true, nil
		}
		return false, nil
	}
	// Step 4: Tie Break, choose small block hash.
	if new(big.Int).SetBytes(sideChain.Hash.Bytes()).Cmp(new(big.Int).SetBytes(bestChain.Hash.Bytes())) < 0 {
		return true, nil
	}
	return false, nil
}

// RetrievePunishment retrieves faultPks from database, sending them to memPool.
func (chain *Blockchain) RetrievePunishment() ([]*wire.FaultPubKey, error) {
	logging.CPrint(logging.DEBUG, "retrieving punishment pubKeys, this may take a while")
	fpks, err := chain.db.FetchAllPunishment()
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on retrieving punishments from database", logging.LogFormat{
			"err": err,
		})
		return nil, err
	}

	for i, fpk := range fpks {
		logging.CPrint(logging.DEBUG, "punishment pubKey", logging.LogFormat{
			"index":             i,
			"punishment pubKey": hex.EncodeToString(fpk.PubKey.SerializeCompressed()),
		})
	}

	return fpks, nil
}

func (chain *Blockchain) RetrieveFaultPks() {
	logging.CPrint(logging.DEBUG, "retrieving banned pubKeys, this may take a while")
	fpkList, heightList, err := chain.db.FetchAllFaultPks()
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on retrieving faultPks from database", logging.LogFormat{
			"err": err,
		})
		return
	}
	for i, fpk := range fpkList {
		logging.CPrint(logging.DEBUG, "banned pubKey", logging.LogFormat{
			"index":         i,
			"Banned pubKey": hex.EncodeToString(fpk.PubKey.SerializeCompressed()),
			"height":        heightList[i],
		})
	}
	return
}
