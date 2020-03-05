package netsync

import (
	"container/list"
	"time"

	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

const (
	syncCycle            = 5 * time.Second
	blockProcessChSize   = 1024
	blocksProcessChSize  = 128
	headerProcessChSize  = 1024
	headersProcessChSize = 2048
)

var (
	maxRegularBlocksPerRound   = uint64(512)
	maxBatchBlocksPerMsg       = uint64(2048)
	maxBlockHeadersPerMsg      = uint64(2048)
	maxBatchSyncBlocksPerRound = maxBlockHeadersPerMsg
	syncTimeout                = 30 * time.Second

	errAppendHeaders  = errors.New("fail to append list due to order dismatch")
	errRequestTimeout = errors.New("request timeout")
	errPeerDropped    = errors.New("Peer dropped")
	errPeerMisbehave  = errors.New("peer is misbehave")
)

type blockMsg struct {
	block  *massutil.Block
	peerID string
}

type blocksMsg struct {
	blocks []*massutil.Block
	peerID string
}

type headerMsg struct {
	header *wire.BlockHeader
	peerID string
}

type headersMsg struct {
	headers []*wire.BlockHeader
	peerID  string
}

type blockKeeper struct {
	chain Chain
	peers *peerSet

	syncPeer         *peer
	blockProcessCh   chan *blockMsg
	blocksProcessCh  chan *blocksMsg
	headerProcessCh  chan *headerMsg
	headersProcessCh chan *headersMsg

	headerList *list.List
}

func newBlockKeeper(chain Chain, peers *peerSet) *blockKeeper {
	bk := &blockKeeper{
		chain:            chain,
		peers:            peers,
		blockProcessCh:   make(chan *blockMsg, blockProcessChSize),
		blocksProcessCh:  make(chan *blocksMsg, blocksProcessChSize),
		headerProcessCh:  make(chan *headerMsg, headerProcessChSize),
		headersProcessCh: make(chan *headersMsg, headersProcessChSize),
		headerList:       list.New(),
	}
	bk.resetHeaderState()
	go bk.syncWorker()
	return bk
}

func (bk *blockKeeper) appendHeaderList(headers []*wire.BlockHeader) error {
	for _, header := range headers {
		prevHeader := bk.headerList.Back().Value.(*wire.BlockHeader)
		if prevHeader.BlockHash() != header.Previous {
			return errAppendHeaders
		}
		bk.headerList.PushBack(header)
	}
	return nil
}

func (bk *blockKeeper) blockLocator() []*wire.Hash {
	header := bk.chain.BestBlockHeader()
	locator := []*wire.Hash{}

	step := uint64(1)
	for header != nil {
		headerHash := header.BlockHash()
		locator = append(locator, &headerHash)
		if header.Height == 0 {
			break
		}

		var err error
		if header.Height < step {
			header, err = bk.chain.GetHeaderByHeight(0)
		} else {
			header, err = bk.chain.GetHeaderByHeight(header.Height - step)
		}
		if err != nil {
			logging.CPrint(logging.ERROR, "blockKeeper fail on get blockLocator", logging.LogFormat{"err": err})
			break
		}

		if len(locator) >= 9 {
			step *= 2
		}
	}
	return locator
}

func (bk *blockKeeper) fastBlockSync(checkPoint *config.Checkpoint) error {
	bk.resetHeaderState()
	lastHeader := bk.headerList.Back().Value.(*wire.BlockHeader)
	for ; lastHeader.BlockHash() != *checkPoint.Hash; lastHeader = bk.headerList.Back().Value.(*wire.BlockHeader) {
		if lastHeader.Height >= checkPoint.Height {
			return errors.Wrap(errPeerMisbehave, "peer is not in the checkpoint branch")
		}

		lastHash := lastHeader.BlockHash()
		headers, err := bk.requireHeaders([]*wire.Hash{&lastHash}, checkPoint.Hash)
		if err != nil {
			return err
		}

		if len(headers) == 0 {
			return errors.Wrap(errPeerMisbehave, "requireHeaders return empty list")
		}

		if err := bk.appendHeaderList(headers); err != nil {
			return err
		}
	}

	fastHeader := bk.headerList.Front()
	for bk.chain.BestBlockHeight() < checkPoint.Height {
		locator := bk.blockLocator()
		blocks, err := bk.requireBlocks(locator, checkPoint.Hash)
		if err != nil {
			return err
		}

		if len(blocks) == 0 {
			return errors.Wrap(errPeerMisbehave, "requireBlocks return empty list")
		}

		for _, block := range blocks {
			if fastHeader = fastHeader.Next(); fastHeader == nil {
				return errors.New("get block that is higher than checkpoint")
			}

			blockHash := *block.Hash()
			if blockHash != fastHeader.Value.(*wire.BlockHeader).BlockHash() {
				return errPeerMisbehave
			}

			_, err = bk.chain.ProcessBlock(block)

			if err != nil {
				return errors.Wrap(err, "fail on fastBlockSync process block")
			}
		}
	}
	return nil
}

func (bk *blockKeeper) batchBlockSync(targetHeight uint64) error {
	bk.resetBatchHeaderState()
	lastHeader := bk.headerList.Back().Value.(*wire.BlockHeader)
	lastHash := lastHeader.BlockHash()

	targetHeader, err := bk.requireHeader(targetHeight)
	if err != nil {
		return err
	}
	targetHash := targetHeader.BlockHash()

	headers, err := bk.requireHeaders(bk.blockLocator(), &targetHash)
	if err != nil {
		return err
	}

	if len(headers) == 0 {
		return errors.Wrap(errPeerMisbehave, "requireHeaders return empty list")
	}

	// Maybe peer is on a forked chain
	if lastHash != headers[0].Previous {
		rootHeader, err := bk.chain.GetHeaderByHash(&headers[0].Previous)
		if err != nil {
			return errors.Wrap(errPeerMisbehave, "peer created a misleading header list")
		}
		return bk.batchForkedSync(rootHeader, headers, targetHeight, &targetHash)
	}

	if err := bk.appendHeaderList(headers); err != nil {
		return err
	}

	batchHeader := bk.headerList.Front()
	syncHeight := bk.headerList.Back().Value.(*wire.BlockHeader).Height
	for bk.chain.BestBlockHeight() < syncHeight {
		locator := bk.blockLocator()
		blocks, err := bk.requireBlocks(locator, &targetHash)
		if err != nil {
			return err
		}

		if len(blocks) == 0 {
			return errors.Wrap(errPeerMisbehave, "requireBlocks return empty list")
		}

		for _, block := range blocks {
			if batchHeader = batchHeader.Next(); batchHeader == nil {
				return errors.New("get block that is higher than target height")
			}

			blockHash := *block.Hash()
			if blockHash != batchHeader.Value.(*wire.BlockHeader).BlockHash() {
				return errPeerMisbehave
			}

			_, err = bk.chain.ProcessBlock(block)
			if err != nil {
				return errors.Wrap(err, "fail on batchBlockSync process block")
			}
		}
	}
	return nil
}

func (bk *blockKeeper) batchForkedSync(root *wire.BlockHeader, diverged []*wire.BlockHeader, targetHeight uint64, targetHash *wire.Hash) error {
	// clear headerList, and push root into list
	bk.headerList.Remove(bk.headerList.Back())
	bk.headerList.PushBack(root)

	if err := bk.appendHeaderList(diverged); err != nil {
		return err
	}

	lastHeader := bk.headerList.Back().Value.(*wire.BlockHeader)

	for ; lastHeader.BlockHash() != *targetHash; lastHeader = bk.headerList.Back().Value.(*wire.BlockHeader) {
		if lastHeader.Height >= targetHeight {
			return errors.Wrap(errPeerMisbehave, "peer switched to another forked chain")
		}

		lastHash := lastHeader.BlockHash()
		headers, err := bk.requireHeaders([]*wire.Hash{&lastHash}, targetHash)
		if err != nil {
			return err
		}

		if len(headers) == 0 {
			return errors.Wrap(errPeerMisbehave, "requireHeaders return empty list")
		}

		if err := bk.appendHeaderList(headers); err != nil {
			return err
		}
	}

	batchHeader := bk.headerList.Front()
	syncHeight := bk.headerList.Back().Value.(*wire.BlockHeader).Height
	lastProcessed := root
	for bk.chain.BestBlockHeight() < syncHeight {
		//locator := bk.blockLocator()
		lastHash := lastProcessed.BlockHash()
		blocks, err := bk.requireBlocks([]*wire.Hash{&lastHash}, targetHash)
		if err != nil {
			return err
		}

		if len(blocks) == 0 {
			return errors.Wrap(errPeerMisbehave, "requireBlocks return empty list")
		}

		for _, block := range blocks {
			if batchHeader = batchHeader.Next(); batchHeader == nil {
				return errors.New("get block that is higher than target height")
			}

			blockHash := *block.Hash()
			if blockHash != batchHeader.Value.(*wire.BlockHeader).BlockHash() {
				return errPeerMisbehave
			}

			_, err = bk.chain.ProcessBlock(block)
			if err != nil {
				return errors.Wrap(err, "fail on batchBlockSync process block")
			}

			lastProcessed = &block.MsgBlock().Header
		}
	}
	return nil
}

func (bk *blockKeeper) locateBlocks(locator []*wire.Hash, stopHash *wire.Hash) ([]*massutil.Block, error) {
	headers, err := bk.locateHeaders(locator, stopHash, maxBatchBlocksPerMsg)
	if err != nil {
		return nil, err
	}

	blocks := []*massutil.Block{}
	for i, header := range headers {
		if uint64(i) >= maxBatchBlocksPerMsg {
			break
		}

		headerHash := header.BlockHash()
		block, err := bk.chain.GetBlockByHash(&headerHash)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (bk *blockKeeper) locateHeaders(locator []*wire.Hash, stopHash *wire.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	stopHeader, err := bk.chain.GetHeaderByHash(stopHash)
	if err != nil {
		return nil, err
	}

	startHeader, err := bk.chain.GetHeaderByHeight(0)
	if err != nil {
		return nil, err
	}

	for _, hash := range locator {
		header, err := bk.chain.GetHeaderByHash(hash)
		if err == nil && bk.chain.InMainChain(header.BlockHash()) {
			startHeader = header
			break
		}
	}

	totalHeaders := stopHeader.Height - startHeader.Height
	if totalHeaders > maxHeaders {
		totalHeaders = maxHeaders
	}

	headers := []*wire.BlockHeader{}
	for i := uint64(1); i <= totalHeaders; i++ {
		header, err := bk.chain.GetHeaderByHeight(startHeader.Height + i)
		if err != nil {
			return nil, err
		}

		headers = append(headers, header)
	}
	return headers, nil
}

func (bk *blockKeeper) nextCheckpoint() *config.Checkpoint {
	height := bk.chain.BestBlockHeader().Height
	checkpoints := config.ChainParams.Checkpoints
	if len(checkpoints) == 0 || height >= checkpoints[len(checkpoints)-1].Height {
		return nil
	}

	nextCheckpoint := &checkpoints[len(checkpoints)-1]
	for i := len(checkpoints) - 2; i >= 0; i-- {
		if height >= checkpoints[i].Height {
			break
		}
		nextCheckpoint = &checkpoints[i]
	}
	return nextCheckpoint
}

func (bk *blockKeeper) processBlock(peerID string, block *massutil.Block) {
	bk.blockProcessCh <- &blockMsg{block: block, peerID: peerID}
}

func (bk *blockKeeper) processBlocks(peerID string, blocks []*massutil.Block) {
	bk.blocksProcessCh <- &blocksMsg{blocks: blocks, peerID: peerID}
}

func (bk *blockKeeper) processHeader(peerID string, header *wire.BlockHeader) {
	bk.headerProcessCh <- &headerMsg{header: header, peerID: peerID}
}

func (bk *blockKeeper) processHeaders(peerID string, headers []*wire.BlockHeader) {
	bk.headersProcessCh <- &headersMsg{headers: headers, peerID: peerID}
}

func (bk *blockKeeper) regularBlockSync(wantHeight uint64) error {
	i := bk.chain.BestBlockHeight() + 1
	for i <= wantHeight {
		block, err := bk.requireBlock(i)
		if err != nil {
			return err
		}

		isOrphan, err := bk.chain.ProcessBlock(block)
		if err != nil {
			return err
		}

		if isOrphan {
			i--
			continue
		}
		i = bk.chain.BestBlockHeight() + 1
	}
	return nil
}

func (bk *blockKeeper) requireBlock(height uint64) (*massutil.Block, error) {
	if ok := bk.syncPeer.getBlockByHeight(height); !ok {
		return nil, errPeerDropped
	}

	waitTicker := time.NewTimer(syncTimeout)
	for {
		select {
		case msg := <-bk.blockProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.block.MsgBlock().Header.Height != height {
				continue
			}
			if err := preventBlockFromFuture(msg.block); err != nil {
				return msg.block, err
			}
			return msg.block, nil
		case <-waitTicker.C:
			return nil, errors.Wrap(errRequestTimeout, "requireBlock")
		}
	}
}

func (bk *blockKeeper) requireBlocks(locator []*wire.Hash, stopHash *wire.Hash) ([]*massutil.Block, error) {
	if ok := bk.syncPeer.getBlocks(locator, stopHash); !ok {
		return nil, errPeerDropped
	}

	waitTicker := time.NewTimer(syncTimeout)
	for {
		select {
		case msg := <-bk.blocksProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if err := preventBlocksFromFuture(msg.blocks); err != nil {
				return msg.blocks, err
			}
			return msg.blocks, nil
		case <-waitTicker.C:
			return nil, errors.Wrap(errRequestTimeout, "requireBlocks")
		}
	}
}

func (bk *blockKeeper) requireHeader(height uint64) (*wire.BlockHeader, error) {
	if ok := bk.syncPeer.getHeaderByHeight(height); !ok {
		return nil, errPeerDropped
	}

	waitTicker := time.NewTimer(syncTimeout)
	for {
		select {
		case msg := <-bk.headerProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.header.Height != height {
				continue
			}
			return msg.header, nil
		case <-waitTicker.C:
			return nil, errors.Wrap(errRequestTimeout, "requireHeader")
		}
	}
}

func (bk *blockKeeper) requireHeaders(locator []*wire.Hash, stopHash *wire.Hash) ([]*wire.BlockHeader, error) {
	if ok := bk.syncPeer.getHeaders(locator, stopHash); !ok {
		return nil, errPeerDropped
	}

	waitTicker := time.NewTimer(syncTimeout)
	for {
		select {
		case msg := <-bk.headersProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			return msg.headers, nil
		case <-waitTicker.C:
			return nil, errors.Wrap(errRequestTimeout, "requireHeaders")
		}
	}
}

// resetHeaderState sets the headers-first mode state to values appropriate for
// syncing from a new peer.
func (bk *blockKeeper) resetHeaderState() {
	header := bk.chain.BestBlockHeader()
	bk.headerList.Init()
	if bk.nextCheckpoint() != nil {
		bk.headerList.PushBack(header)
	}
}

// resetBatchHeaderState sets the headers-first batch sync mode state to values appropriate for
// syncing from a new peer.
func (bk *blockKeeper) resetBatchHeaderState() {
	header := bk.chain.BestBlockHeader()
	bk.headerList.Init()
	bk.headerList.PushBack(header)
}

func (bk *blockKeeper) startSync() bool {
	checkPoint := bk.nextCheckpoint()
	peer := bk.peers.bestPeer(consensus.SFFastSync | consensus.SFFullNode)
	if peer == nil {
		return false
	}

	// fastBlockSync
	if checkPoint != nil && peer.Height() >= checkPoint.Height {
		bk.syncPeer = peer
		if err := bk.fastBlockSync(checkPoint); err != nil {
			logging.CPrint(logging.WARN, "fail on fastBlockSync", logging.LogFormat{"err": err, "peer": peer.Addr()})
			bk.peers.errorHandler(peer.ID(), err)
			return false
		}
		return true
	}

	localHeight := bk.chain.BestBlockHeight()
	if peer.Height() <= localHeight {
		return false
	}

	bk.syncPeer = peer
	diff := peer.Height() - localHeight
	// batchBlockSync
	if diff > maxRegularBlocksPerRound {
		targetHeight := localHeight + maxBatchSyncBlocksPerRound
		diff = diff - maxRegularBlocksPerRound
		if diff < maxBatchSyncBlocksPerRound {
			targetHeight = localHeight + diff
		}

		if err := bk.batchBlockSync(targetHeight); err != nil {
			logging.CPrint(logging.WARN, "fail on batchBlockSync", logging.LogFormat{"err": err, "peer": peer.Addr()})
			bk.peers.errorHandler(peer.ID(), err)
			return false
		}
		return true
	}

	// regularBlockSync
	if err := bk.regularBlockSync(peer.Height()); err != nil {
		logging.CPrint(logging.WARN, "fail on regularBlockSync", logging.LogFormat{"err": err, "peer": peer.Addr()})
		bk.peers.errorHandler(peer.ID(), err)
		return false
	}
	return true
}

func (bk *blockKeeper) syncWorker() {
	genesisBlock, err := bk.chain.GetBlockByHeight(0)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleStatusRequestMsg get genesis", logging.LogFormat{"err": err})
		return
	}
	syncTicker := time.NewTicker(syncCycle)
	for {
		<-syncTicker.C
		if update := bk.startSync(); !update {
			continue
		}

		block, err := bk.chain.GetBlockByHeight(bk.chain.BestBlockHeight())
		if err != nil {
			logging.CPrint(logging.ERROR, "fail on syncWorker get best block", logging.LogFormat{"err": err})
			continue
		}

		if err := bk.peers.broadcastMinedBlock(block); err != nil {
			logging.CPrint(logging.ERROR, "fail on syncWorker broadcast new block", logging.LogFormat{"err": err})
			continue
		}

		if err = bk.peers.broadcastNewStatus(block, genesisBlock); err != nil {
			logging.CPrint(logging.ERROR, "fail on syncWorker broadcast new status", logging.LogFormat{"err": err})
			continue
		}
	}
}
