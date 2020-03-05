package blockchain

import (
	"fmt"
	"time"

	"massnet.org/mass/config"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

func (chain *Blockchain) maybeAcceptBlock(block *massutil.Block, flags BehaviorFlags) error {
	// Get a block node for the block previous to this one.  Will be nil
	// if this is the genesis block.
	prevNode, err := chain.getPrevNodeFromBlock(block)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on getPrevNodeFromBlock",
			logging.LogFormat{
				"err":     err,
				"preHash": block.MsgBlock().Header.Previous,
				"hash":    block.Hash(),
				"flags":   fmt.Sprintf("%b", flags),
			})
		return err
	}
	if prevNode == nil {
		logging.CPrint(logging.ERROR, "prev node not found",
			logging.LogFormat{
				"preHash": block.MsgBlock().Header.Previous,
				"hash":    block.Hash(),
				"flags":   fmt.Sprintf("%b", flags),
			})
		return fmt.Errorf("prev node not found")
	}

	// The height of this block is one more than the referenced previous
	// block.
	block.SetHeight(prevNode.Height + 1)

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.
	err = chain.checkBlockContext(block, prevNode, flags)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on checkBlockContext",
			logging.LogFormat{
				"err": err, "preHash": block.MsgBlock().Header.Previous,
				"hash":  block.Hash(),
				"flags": fmt.Sprintf("%b", flags),
			})
		return err
	}

	// Create a new block node for the block and add it to the in-memory
	// block chain (could be either a side chain or the main chain).
	blockHeader := &block.MsgBlock().Header

	if flags.isFlagSet(BFNoPoCCheck) {
		return chain.checkConnectBlock(NewBlockNode(blockHeader, nil, BFNoPoCCheck), block)
	}

	newNode := NewBlockNode(blockHeader, block.Hash(), BFNone)
	if prevNode != nil {
		newNode.Parent = prevNode
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	err = chain.connectBestChain(newNode, block, flags)
	if err != nil {
		return err
	}

	return nil
}

func (chain *Blockchain) processOrphans(hash *wire.Hash, flags BehaviorFlags) error {
	for _, orphan := range chain.blockTree.orphanBlockPool.getOrphansByPrevious(hash) {
		logging.CPrint(logging.INFO, "process orphan",
			logging.LogFormat{
				"parent_hash":  hash,
				"child_hash":   orphan.block.Hash(),
				"child_height": orphan.block.Height(),
			})
		if err := chain.maybeAcceptBlock(orphan.block, BFNone); err != nil {
			chain.errCache.Add(orphan.block.Hash().String(), err)
			return err
		}

		chain.blockTree.orphanBlockPool.removeOrphanBlock(orphan)

		if err := chain.processOrphans(orphan.block.Hash(), BFNone); err != nil {
			return err
		}
	}

	return nil
}

func (chain *Blockchain) processBlock(block *massutil.Block, flags BehaviorFlags) (isOrphan bool, err error) {
	var startProcessing = time.Now()

	if flags.isFlagSet(BFNoPoCCheck) {
		// Perform preliminary sanity checks on the block and its transactions.
		err := checkBlockSanity(block, chain.info.chainID, config.ChainParams.PocLimit, flags)
		if err != nil {
			return false, err
		}

		// The block has passed all context independent checks and appears sane
		// enough to potentially accept it into the block chain.
		if err := chain.maybeAcceptBlock(block, flags); err != nil {
			logging.CPrint(logging.ERROR, "fail on maybeAcceptBlock with BFNoPoCCheck", logging.LogFormat{
				"err":      err,
				"previous": block.MsgBlock().Header.Previous,
				"height":   block.Height(),
				"elapsed":  time.Since(startProcessing),
				"flags":    fmt.Sprintf("%b", flags),
			})
			return false, err
		}
		return false, nil
	}

	blockHash := block.Hash()
	logging.CPrint(logging.TRACE, "processing block", logging.LogFormat{
		"hash":     blockHash,
		"height":   block.Height(),
		"tx_count": len(block.Transactions()),
		"flags":    fmt.Sprintf("%b", flags),
	})

	// The block must not already exist in the main chain or side chains.
	if chain.blockExists(blockHash) {
		return false, nil
	}

	// The block must not already exist as an orphan.
	if chain.blockTree.orphanExists(blockHash) {
		return true, nil
	}

	// Return fail if block error already been cached.
	if v, ok := chain.errCache.Get(blockHash.String()); ok {
		return false, v.(error)
	}

	// Perform preliminary sanity checks on the block and its transactions.
	err = checkBlockSanity(block, chain.info.chainID, config.ChainParams.PocLimit, flags)
	if err != nil {
		chain.errCache.Add(blockHash.String(), err)
		return false, err
	}

	blockHeader := &block.MsgBlock().Header

	// Handle orphan blocks.
	prevHash := &blockHeader.Previous
	if !prevHash.IsEqual(zeroHash) {
		prevHashExists := chain.blockExists(prevHash)

		if !prevHashExists {
			logging.CPrint(logging.INFO, "Adding orphan block with Parent", logging.LogFormat{
				"orphan":  blockHash,
				"height":  block.Height(),
				"parent":  prevHash,
				"elapsed": time.Since(startProcessing),
				"flags":   fmt.Sprintf("%b", flags),
			})
			chain.blockTree.orphanBlockPool.addOrphanBlock(block)
			return true, nil
		}
	}

	// The block has passed all context independent checks and appears sane
	// enough to potentially accept it into the block chain.
	if err := chain.maybeAcceptBlock(block, flags); err != nil {
		chain.errCache.Add(blockHash.String(), err)
		return false, err
	}

	// Accept any orphan blocks that depend on this block (they are
	// no longer orphans) and repeat for those accepted blocks until
	// there are no more.
	if err := chain.processOrphans(blockHash, flags); err != nil {
		return false, err
	}

	logging.CPrint(logging.DEBUG, "accepted block", logging.LogFormat{
		"hash":     blockHash,
		"height":   block.Height(),
		"tx_count": len(block.Transactions()),
		"elapsed":  time.Since(startProcessing),
		"flags":    fmt.Sprintf("%b", flags),
	})

	return false, nil
}
