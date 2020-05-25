package blockchain

import (
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
	"massnet.org/mass/config"
	"massnet.org/mass/database"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/pocec"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	maxProcessBlockChSize = 1024
	sigCacheMaxSize       = 50000
	hashCacheMaxSize      = sigCacheMaxSize
	blockErrCacheSize     = 500
)

type chainInfo struct {
	genesisBlock *massutil.Block
	genesisHash  wire.Hash
	genesisTime  time.Time
	chainID      wire.Hash
}

type processBlockResponse struct {
	isOrphan bool
	err      error
}

type processBlockMsg struct {
	block *massutil.Block
	flags BehaviorFlags
	reply chan processBlockResponse
}

type Listener interface {
	OnBlockConnected(*wire.MsgBlock) error
	OnTransactionReceived(tx *wire.MsgTx) error
}

type Blockchain struct {
	l              sync.RWMutex
	cond           sync.Cond
	db             database.Db           // database for main chain
	blockCache     *blockCache           // cache storing side chain blocks
	blockTree      *BlockTree            // tree index consists of blocks
	txPool         *TxPool               // pool of transactions
	proposalPool   *ProposalPool         // pool of proposals
	addrIndexer    *AddrIndexer          // address indexer
	dmd            *DoubleMiningDetector // double mining detector
	processBlockCh chan *processBlockMsg
	listeners      map[Listener]struct{}

	info      *chainInfo
	errCache  *lru.Cache
	sigCache  *txscript.SigCache
	hashCache *txscript.HashCache
}

func NewBlockchain(db database.Db, dbPath string, server Server) (*Blockchain, error) {
	chain := &Blockchain{
		db:             db,
		blockTree:      NewBlockTree(),
		dmd:            NewDoubleMiningDetector(db),
		processBlockCh: make(chan *processBlockMsg, maxProcessBlockChSize),
		errCache:       lru.New(blockErrCacheSize),
		hashCache:      txscript.NewHashCache(hashCacheMaxSize),
		listeners:      make(map[Listener]struct{}),
	}
	chain.cond.L = &sync.Mutex{}

	var err error
	if chain.blockCache, err = initBlockCache(dbPath); err != nil {
		return nil, err
	}

	chain.txPool = NewTxPool(chain, chain.sigCache, chain.hashCache)

	if punishments, err := chain.RetrievePunishment(); err == nil {
		chain.proposalPool = NewProposalPool(punishments)
	} else {
		return nil, err
	}

	if chain.addrIndexer, err = NewAddrIndexer(db, server); err != nil {
		return nil, err
	}

	genesisHash, err := db.FetchBlockShaByHeight(0)
	if err != nil {
		return nil, err
	}
	genesisBlock, err := db.FetchBlockBySha(genesisHash)
	if err != nil {
		return nil, err
	}
	chain.info = &chainInfo{
		genesisBlock: genesisBlock,
		genesisHash:  *genesisBlock.Hash(),
		genesisTime:  genesisBlock.MsgBlock().Header.Timestamp,
		chainID:      genesisBlock.MsgBlock().Header.ChainID,
	}

	if err := chain.generateInitialIndex(); err != nil {
		return nil, err
	}

	go chain.blockProcessor()

	return chain, nil
}

func (chain *Blockchain) generateInitialIndex() error {
	// Return an error if the has already been modified.
	if chain.blockTree.rootBlockNode() != nil {
		return errIndexAlreadyInitialized
	}

	// Grab the latest block height for the main chain from the database.
	_, endHeight, err := chain.db.NewestSha()
	if err != nil {
		return err
	}

	// Calculate the starting height based on the minimum number of nodes
	// needed in memory.
	var startHeight uint64
	if endHeight >= minMemoryNodes {
		startHeight = endHeight - minMemoryNodes
	}

	// Loop forwards through each block loading the node into the index for
	// the block.
	// FetchBlockBySha multiple times with the appropriate indices as needed.
	for start := startHeight; start <= endHeight; {
		hashList, err := chain.db.FetchHeightRange(start, endHeight+1)
		if err != nil {
			return err
		}

		// The database did not return any further hashes.  Break out of
		// the loop now.
		if len(hashList) == 0 {
			break
		}

		// Loop forwards through each block loading the node into the
		// index for the block.
		for _, hash := range hashList {
			// Make a copy of the hash to make sure there are no
			// references into the list so it can be freed.
			hashCopy := hash
			node, err := chain.loadBlockNode(&hashCopy)
			if err != nil {
				return err
			}

			// This node is now the end of the best chain.
			chain.blockTree.setBestBlockNode(node)
		}

		// Start at the next block after the latest one on the next loop
		// iteration.
		start += uint64(len(hashList))
	}

	return nil
}

func (chain *Blockchain) blockExists(hash *wire.Hash) bool {
	// Check memory chain first (could be main chain or side chain blocks).
	if chain.blockTree.nodeExists(hash) {
		return true
	}
	// Check in database (rest of main chain not in memory).
	exists, err := chain.db.ExistsSha(hash)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to check block existence from db",
			logging.LogFormat{"hash": hash, "err": err})
	}
	return exists
}

func (chain *Blockchain) loadBlockNode(hash *wire.Hash) (*BlockNode, error) {
	blockHeader, err := chain.db.FetchBlockHeaderBySha(hash)
	if err != nil {
		return nil, err
	}

	node := NewBlockNode(blockHeader, hash, BFNone)
	node.InMainChain = true

	// deal with leaf node
	if chain.blockTree.nodeExists(&node.Previous) {
		if err := chain.blockTree.attachBlockNode(node); err != nil {
			return nil, err
		}
		return node, nil
	}

	// deal with expand root
	if root := chain.blockTree.rootBlockNode(); root != nil && node.Hash.IsEqual(&root.Previous) {
		if err := chain.blockTree.expandRootBlockNode(node); err != nil {
			return nil, err
		}
		return node, nil
	}

	// deal with set root
	if root := chain.blockTree.rootBlockNode(); root == nil {
		if err := chain.blockTree.setRootBlockNode(node); err != nil {
			return nil, err
		}
		return node, nil
	}

	// deal with orphan node
	return nil, errExpandOrphanRootBlockNode
}

func (chain *Blockchain) getPrevNodeFromBlock(block *massutil.Block) (*BlockNode, error) {
	// Genesis block.
	prevHash := &block.MsgBlock().Header.Previous
	if prevHash.IsEqual(zeroHash) {
		return nil, nil
	}

	// Return the existing previous block node if it's already there.
	if bn, ok := chain.blockTree.getBlockNode(prevHash); ok {
		return bn, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	prevBlockNode, err := chain.loadBlockNode(prevHash)
	if err != nil {
		return nil, err
	}
	return prevBlockNode, nil
}

func (chain *Blockchain) getPrevNodeFromNode(node *BlockNode) (*BlockNode, error) {
	// Return the existing previous block node if it's already there.
	if node.Parent != nil {
		return node.Parent, nil
	}

	// Return node in blockTree index
	if parent, exists := chain.blockTree.getBlockNode(&node.Previous); exists {
		return parent, nil
	}

	// Genesis block.
	if node.Hash.IsEqual(config.ChainParams.GenesisHash) {
		return nil, nil
	}

	// Dynamically load the previous block from the block database, create
	// a new block node for it, and update the memory chain accordingly.
	prevBlockNode, err := chain.loadBlockNode(&node.Previous)
	if err != nil {
		return nil, err
	}

	return prevBlockNode, nil
}

func (chain *Blockchain) blockProcessor() {
	for msg := range chain.processBlockCh {
		isOrphan, err := chain.processBlock(msg.block, msg.flags)
		msg.reply <- processBlockResponse{isOrphan: isOrphan, err: err}
	}
}

// processBlock is the entry for handle block insert
func (chain *Blockchain) execProcessBlock(block *massutil.Block, flags BehaviorFlags) (bool, error) {
	reply := make(chan processBlockResponse, 1)
	chain.processBlockCh <- &processBlockMsg{block: block, flags: flags, reply: reply}
	response := <-reply
	return response.isOrphan, response.err
}

func (chain *Blockchain) BestBlockNode() *BlockNode {
	return chain.blockTree.bestBlockNode()
}

func (chain *Blockchain) BestBlockHeader() *wire.BlockHeader {
	return chain.blockTree.bestBlockNode().BlockHeader()
}

func (chain *Blockchain) BestBlockHeight() uint64 {
	return chain.blockTree.bestBlockNode().Height
}

func (chain *Blockchain) BestBlockHash() *wire.Hash {
	return chain.blockTree.bestBlockNode().Hash
}

func (chain *Blockchain) GetBlockByHash(hash *wire.Hash) (*massutil.Block, error) {
	return chain.db.FetchBlockBySha(hash)
}

func (chain *Blockchain) GetBlockByHeight(height uint64) (*massutil.Block, error) {
	hash, err := chain.db.FetchBlockShaByHeight(height)
	if err != nil {
		return nil, err
	}
	return chain.db.FetchBlockBySha(hash)
}

func (chain *Blockchain) GetHeaderByHash(hash *wire.Hash) (*wire.BlockHeader, error) {
	if node, exists := chain.blockTree.getBlockNode(hash); exists {
		return node.BlockHeader(), nil
	}
	return chain.db.FetchBlockHeaderBySha(hash)
}

func (chain *Blockchain) GetHeaderByHeight(height uint64) (*wire.BlockHeader, error) {
	hash, err := chain.db.FetchBlockShaByHeight(height)
	if err != nil {
		return nil, err
	}
	return chain.db.FetchBlockHeaderBySha(hash)
}

func (chain *Blockchain) GetBlockHashByHeight(height uint64) (*wire.Hash, error) {
	return chain.db.FetchBlockShaByHeight(height)
}

func (chain *Blockchain) GetTransactionInDB(hash *wire.Hash) ([]*database.TxReply, error) {
	return chain.db.FetchTxBySha(hash)
}

func (chain *Blockchain) InMainChain(hash wire.Hash) bool {
	if node, exists := chain.blockTree.getBlockNode(&hash); exists {
		return node.InMainChain
	}
	height, err := chain.db.FetchBlockHeightBySha(&hash)
	if err != nil {
		return false
	}
	dbHash, err := chain.db.FetchBlockShaByHeight(height)
	if err != nil {
		return false
	}
	return *dbHash == hash
}

func (chain *Blockchain) FetchMinedBlocks(pubKey *pocec.PublicKey) ([]uint64, error) {
	return chain.db.FetchMinedBlocks(pubKey)
}

func (chain *Blockchain) GetTransaction(hash *wire.Hash) (*wire.MsgTx, error) {
	txList, err := chain.db.FetchTxBySha(hash)
	if err != nil {
		return nil, err
	}
	if len(txList) == 0 {
		return nil, database.ErrTxShaMissing
	}
	return txList[0].Tx, nil
}

// ProcessBlock is the entry for chain update
func (chain *Blockchain) ProcessBlock(block *massutil.Block) (bool, error) {
	return chain.execProcessBlock(block, BFNone)
}

func (chain *Blockchain) ProcessTx(tx *massutil.Tx) (bool, error) {
	return chain.txPool.ProcessTransaction(tx, true, false)
}

func (chain *Blockchain) ChainID() *wire.Hash {
	return wire.NewHashFromHash(chain.info.chainID)
}

func (chain *Blockchain) GetTxPool() *TxPool {
	return chain.txPool
}

func (chain *Blockchain) BlockWaiter(height uint64) (<-chan *BlockNode, error) {
	chain.l.RLock()
	defer chain.l.RUnlock()

	if chain.blockTree.bestBlockNode().Height > height {
		return nil, errWaitForOldBlockHeight
	}

	ch := make(chan *BlockNode, 1)
	go func() {
		chain.cond.L.Lock()
		defer chain.cond.L.Unlock()
		var node *BlockNode
		for {
			chain.cond.Wait()
			node = chain.blockTree.bestBlockNode()
			if node.Height >= height {
				break
			}
		}
		ch <- node
		close(ch)
	}()

	return ch, nil
}

func (chain *Blockchain) RegisterListener(listener Listener) {
	chain.l.Lock()
	defer chain.l.Unlock()

	chain.listeners[listener] = struct{}{}
}

func (chain *Blockchain) UnregisterListener(listener Listener) {
	chain.l.Lock()
	defer chain.l.Unlock()

	delete(chain.listeners, listener)
}

func (chain *Blockchain) notifyBlockConnected(block *massutil.Block) error {
	if block == nil {
		return errNilArgument
	}
	for listener := range chain.listeners {
		err := listener.OnBlockConnected(block.MsgBlock())
		if err != nil {
			return err
		}
	}
	return nil
}

func (chain *Blockchain) notifyTransactionReceived(tx *massutil.Tx) error {
	if tx == nil {
		return errNilArgument
	}
	for listener := range chain.listeners {
		err := listener.OnTransactionReceived(tx.MsgTx())
		if err != nil {
			return err
		}
	}
	return nil
}

func (chain *Blockchain) CurrentIndexHeight() uint64 {
	return chain.blockTree.bestBlockNode().Height
}

func (chain *Blockchain) GetRewardStakingTx(height uint64) ([]database.Reward, uint32, error) {
	return chain.db.FetchRewardStakingTx(height)
}

func (chain *Blockchain) GetInStakingTx(height uint64) ([]database.Reward, uint32, error) {
	return chain.db.FetchInStakingTx(height)
}

func (chain *Blockchain) FetchScriptHashRelatedBindingTx(scriptHash []byte, chainParams *config.Params) ([]*database.BindingTxReply, error) {
	return chain.db.FetchScriptHashRelatedBindingTx(scriptHash, chainParams)
}
