package netsync

import (
	"reflect"

	"massnet.org/mass/blockchain"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/p2p"
	"massnet.org/mass/wire"
	cmn "github.com/massnetorg/tendermint/tmlibs/common"
)

const (
	maxTxChanSize         = 10000
	maxFilterAddressSize  = 50
	maxFilterAddressCount = 1000
)

type Chain interface {
	BestBlockHeader() *wire.BlockHeader
	BestBlockHeight() uint64
	GetBlockByHash(*wire.Hash) (*massutil.Block, error)
	GetBlockByHeight(uint64) (*massutil.Block, error)
	GetHeaderByHash(*wire.Hash) (*wire.BlockHeader, error)
	GetHeaderByHeight(uint64) (*wire.BlockHeader, error)
	InMainChain(wire.Hash) bool
	ProcessBlock(*massutil.Block) (bool, error)
	ProcessTx(*massutil.Tx) (bool, error)
	ChainID() *wire.Hash
}

type TxPool interface {
	TxDescs() []*blockchain.TxDesc
	SetNewTxCh(chan *massutil.Tx)
}

//SyncManager Sync Manager is responsible for the business layer information synchronization
type SyncManager struct {
	sw          *p2p.Switch
	genesisHash wire.Hash

	// privKey      crypto.PrivKeyEd25519 // local node's p2p key
	chain        Chain
	txPool       TxPool
	blockFetcher *blockFetcher
	blockKeeper  *blockKeeper
	peers        *peerSet

	newTxCh    chan *massutil.Tx
	newBlockCh chan *wire.Hash
	txSyncCh   chan *txSyncMsg
	quitSync   chan struct{}
	config     *config.Config
}

//NewSyncManager create a sync manager
func NewSyncManager(config *config.Config, chain Chain, txPool *blockchain.TxPool, newBlockCh chan *wire.Hash) (*SyncManager, error) {
	genesisHeader, err := chain.GetHeaderByHeight(0)
	if err != nil {
		return nil, err
	}

	sw, err := p2p.NewSwitch(config)
	if err != nil {
		return nil, err
	}
	peers := newPeerSet(sw)
	manager := &SyncManager{
		sw:          sw,
		genesisHash: genesisHeader.BlockHash(),
		txPool:      txPool,
		chain:       chain,
		// privKey:      crypto.GenPrivKeyEd25519(),
		blockFetcher: newBlockFetcher(chain, peers),
		blockKeeper:  newBlockKeeper(chain, peers),
		peers:        peers,
		newTxCh:      make(chan *massutil.Tx, maxTxChanSize),
		newBlockCh:   newBlockCh,
		txSyncCh:     make(chan *txSyncMsg),
		quitSync:     make(chan struct{}),
		config:       config,
	}

	manager.txPool.SetNewTxCh(manager.newTxCh)

	protocolReactor := NewProtocolReactor(manager, manager.peers)
	manager.sw.AddReactor("PROTOCOL", protocolReactor)

	return manager, nil
	// // Create & add listener
	// var listenerStatus bool
	// var l p2p.Listener
	// if !config.Network.P2P.VaultMode {
	// 	l, listenerStatus = p2p.NewDefaultListener(config.Config)
	// 	manager.sw.AddListener(l)

	// 	discv, err := initDiscover(config.Config, &manager.privKey, l.ExternalAddress().Port)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	manager.sw.SetDiscv(discv)
	// }
	// manager.sw.SetNodeInfo(manager.makeNodeInfo(listenerStatus))
	// manager.sw.SetNodePrivKey(manager.privKey)
}

//BestPeer return the highest p2p peerInfo
func (sm *SyncManager) BestPeer() *PeerInfo {
	bestPeer := sm.peers.bestPeer(consensus.SFFullNode)
	if bestPeer != nil {
		return bestPeer.getPeerInfo()
	}
	return nil
}

// GetNewTxCh return a unconfirmed transaction feed channel
func (sm *SyncManager) GetNewTxCh() chan *massutil.Tx {
	return sm.newTxCh
}

//GetPeerInfos return peer info of all peers
func (sm *SyncManager) GetPeerInfos() []*PeerInfo {
	return sm.peers.getPeerInfos()
}

//GetPeerInfos return peer info of all peers
func (sm *SyncManager) PeerCount() int {
	return sm.peers.getPeerCount()
}

//IsCaughtUp check wheather the peer finish the sync
func (sm *SyncManager) IsCaughtUp() bool {
	peer := sm.peers.bestPeer(consensus.SFFullNode)
	return peer == nil || peer.Height() <= sm.chain.BestBlockHeight()
}

//NodeInfo get P2P peer node info
func (sm *SyncManager) NodeInfo() *p2p.NodeInfo {
	return sm.sw.NodeInfo()
}

// NodePubKeyS get P2P peer node pubKey string
func (sm *SyncManager) NodePubKeyS() string {
	return sm.NodeInfo().PubKey.KeyString()
}

//StopPeer try to stop peer by given ID
func (sm *SyncManager) StopPeer(peerID string) error {
	if peer := sm.peers.getPeer(peerID); peer == nil {
		return errPeerIDNotExists
	}
	sm.peers.removePeer(peerID)
	return nil
}

//Switch get sync manager switch
func (sm *SyncManager) Switch() *p2p.Switch {
	return sm.sw
}

func (sm *SyncManager) handleBlockMsg(peer *peer, msg *BlockMessage) {
	block, err := msg.GetBlock()
	if err != nil {
		return
	}

	sm.blockKeeper.processBlock(peer.ID(), block)
}

func (sm *SyncManager) handleBlocksMsg(peer *peer, msg *BlocksMessage) {
	blocks, err := msg.GetBlocks()
	if err != nil {
		logging.CPrint(logging.DEBUG, "fail on handleBlocksMsg GetBlocks", logging.LogFormat{"err": err})
		return
	}

	sm.blockKeeper.processBlocks(peer.ID(), blocks)
}

func (sm *SyncManager) handleFilterAddMsg(peer *peer, msg *FilterAddMessage) {
	peer.addFilterAddress(msg.Address)
}

func (sm *SyncManager) handleFilterClearMsg(peer *peer) {
	peer.filterAdds.Clear()
}

func (sm *SyncManager) handleFilterLoadMsg(peer *peer, msg *FilterLoadMessage) {
	peer.addFilterAddresses(msg.Addresses)
}

func (sm *SyncManager) handleGetBlockMsg(peer *peer, msg *GetBlockMessage) {
	var block *massutil.Block
	var err error
	if msg.Height != 0 {
		block, err = sm.chain.GetBlockByHeight(msg.Height)
	} else {
		block, err = sm.chain.GetBlockByHash(msg.GetHash())
	}
	if err != nil {
		logging.CPrint(logging.WARN, "fail on handleGetBlockMsg get block from chain", logging.LogFormat{"err": err})
		return
	}

	ok, err := peer.sendBlock(block)
	if !ok {
		sm.peers.removePeer(peer.ID())
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleGetBlockMsg sentBlock", logging.LogFormat{"err": err})
	}
}

func (sm *SyncManager) handleGetBlocksMsg(peer *peer, msg *GetBlocksMessage) {
	headers, err := sm.blockKeeper.locateHeaders(msg.GetBlockLocator(), msg.GetStopHash(), maxBatchSyncBlocksPerRound)
	if err != nil || len(headers) == 0 {
		return
	}

	// (size+4)*N+3, 4*2048+3<10000
	totalSize := 10000
	sendBlockHashes := []*wire.Hash{}
	rawBlocks := [][]byte{}
	for _, header := range headers {
		headerHash := header.BlockHash()
		block, err := sm.blockKeeper.chain.GetBlockByHash(&headerHash)
		if err != nil {
			// reorganized
			return
		}
		rawData, err := block.Bytes(wire.Packet)
		if err != nil {
			logging.CPrint(logging.ERROR, "fail on handleGetBlocksMsg marshal block", logging.LogFormat{"err": err})
			continue
		}

		if totalSize+len(rawData) > maxBlockchainResponseSize {
			if len(sendBlockHashes) == 0 {
				totalSize += len(rawData)
				sendBlockHashes = append(sendBlockHashes, block.Hash())
				rawBlocks = append(rawBlocks, rawData)
			}
			break
		}
		totalSize += len(rawData)
		sendBlockHashes = append(sendBlockHashes, block.Hash())
		rawBlocks = append(rawBlocks, rawData)
	}

	ok, err := peer.sendBlocks(sendBlockHashes, rawBlocks)
	if !ok {
		sm.peers.removePeer(peer.ID())
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleGetBlocksMsg sentBlock", logging.LogFormat{"err": err})
	}
}

func (sm *SyncManager) handleGetHeaderMsg(peer *peer, msg *GetHeaderMessage) {
	var header *wire.BlockHeader
	var err error
	if msg.Height != 0 {
		header, err = sm.chain.GetHeaderByHeight(msg.Height)
	} else {
		header, err = sm.chain.GetHeaderByHash(msg.GetHash())
	}
	if err != nil {
		logging.CPrint(logging.WARN, "fail on handleGetBlockMsg get block from chain", logging.LogFormat{"err": err})
		return
	}

	ok, err := peer.sendHeader(header)
	if !ok {
		sm.peers.removePeer(peer.ID())
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleGetHeaderMsg sentHeader", logging.LogFormat{"err": err})
	}
}

func (sm *SyncManager) handleHeaderMsg(peer *peer, msg *HeaderMessage) {
	header, err := msg.GetHeader()
	if err != nil {
		return
	}

	sm.blockKeeper.processHeader(peer.ID(), header)
}

func (sm *SyncManager) handleGetHeadersMsg(peer *peer, msg *GetHeadersMessage) {
	headers, err := sm.blockKeeper.locateHeaders(msg.GetBlockLocator(), msg.GetStopHash(), maxBlockHeadersPerMsg)
	if err != nil || len(headers) == 0 {
		logging.CPrint(logging.DEBUG, "fail on handleGetHeadersMsg locateHeaders", logging.LogFormat{"err": err})
		return
	}

	ok, err := peer.sendHeaders(headers)
	if !ok {
		sm.peers.removePeer(peer.ID())
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleGetHeadersMsg sentBlock", logging.LogFormat{"err": err})
	}
}

func (sm *SyncManager) handleHeadersMsg(peer *peer, msg *HeadersMessage) {
	headers, err := msg.GetHeaders()
	if err != nil {
		logging.CPrint(logging.DEBUG, "fail on handleHeadersMsg GetHeaders", logging.LogFormat{"err": err})
		return
	}

	sm.blockKeeper.processHeaders(peer.ID(), headers)
}

func (sm *SyncManager) handleMineBlockMsg(peer *peer, msg *MineBlockMessage) {
	block, err := msg.GetMineBlock()
	if err != nil {
		logging.CPrint(logging.WARN, "fail on handleMineBlockMsg GetMineBlock", logging.LogFormat{"err": err})
		return
	}

	hash := block.Hash()
	peer.markBlock(hash)
	sm.blockFetcher.processNewBlock(&blockMsg{peerID: peer.ID(), block: block})
	peer.setStatus(block.MsgBlock().Header.Height, hash)
}

func (sm *SyncManager) handleStatusRequestMsg(peer BasePeer) {
	bestHeader := sm.chain.BestBlockHeader()
	genesisBlock, err := sm.chain.GetBlockByHeight(0)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on handleStatusRequestMsg get genesis", logging.LogFormat{"err": err})
		return
	}

	genesisHash := genesisBlock.Hash()
	msg := NewStatusResponseMessage(bestHeader, genesisHash)
	if ok := peer.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
		sm.peers.removePeer(peer.ID())
	}
}

func (sm *SyncManager) handleStatusResponseMsg(basePeer BasePeer, msg *StatusResponseMessage) {
	if peer := sm.peers.getPeer(basePeer.ID()); peer != nil {
		peer.setStatus(msg.Height, msg.GetHash())
		return
	}

	if genesisHash := msg.GetGenesisHash(); sm.genesisHash != *genesisHash {
		logging.CPrint(logging.WARN, "fail hand shake due to different genesis",
			logging.LogFormat{"remote_genesis": genesisHash.String(), "local_genesis": sm.genesisHash.String(),
				"peer_ip": basePeer.Addr(), "peer_id": basePeer.ID(), "outbound": basePeer.IsOutbound()})
		return
	}

	sm.peers.addPeer(basePeer, msg.Height, msg.GetHash())
}

func (sm *SyncManager) handleTransactionMsg(peer *peer, msg *TransactionMessage) {
	tx, err := msg.GetTransaction()
	if err != nil {
		sm.peers.addBanScore(peer.ID(), 0, 10, "fail on get tx from message")
		return
	}

	if isOrphan, err := sm.chain.ProcessTx(tx); err != nil && !isOrphan {
		if err == errors.ErrTxAlreadyExists {
			return
		}
		logging.CPrint(logging.ERROR, "process tx fail", logging.LogFormat{"err": err, "txid": tx.Hash().String()})
		sm.peers.addBanScore(peer.ID(), 10, 0, "fail on process transaction")
	}
}

func (sm *SyncManager) processMsg(basePeer BasePeer, msgType byte, msg BlockchainMessage) {
	peer := sm.peers.getPeer(basePeer.ID())
	if peer == nil && msgType != StatusResponseByte && msgType != StatusRequestByte {
		return
	}

	switch msg := msg.(type) {
	case *GetHeaderMessage:
		sm.handleGetHeaderMsg(peer, msg)

	case *HeaderMessage:
		sm.handleHeaderMsg(peer, msg)

	case *GetBlockMessage:
		sm.handleGetBlockMsg(peer, msg)

	case *BlockMessage:
		sm.handleBlockMsg(peer, msg)

	case *StatusRequestMessage:
		sm.handleStatusRequestMsg(basePeer)

	case *StatusResponseMessage:
		sm.handleStatusResponseMsg(basePeer, msg)

	case *TransactionMessage:
		sm.handleTransactionMsg(peer, msg)

	case *MineBlockMessage:
		sm.handleMineBlockMsg(peer, msg)

	case *GetHeadersMessage:
		sm.handleGetHeadersMsg(peer, msg)

	case *HeadersMessage:
		sm.handleHeadersMsg(peer, msg)

	case *GetBlocksMessage:
		sm.handleGetBlocksMsg(peer, msg)

	case *BlocksMessage:
		sm.handleBlocksMsg(peer, msg)

	case *FilterLoadMessage:
		sm.handleFilterLoadMsg(peer, msg)

	case *FilterAddMessage:
		sm.handleFilterAddMsg(peer, msg)

	case *FilterClearMessage:
		sm.handleFilterClearMsg(peer)

	default:
		logging.CPrint(logging.ERROR, "unknown message type", logging.LogFormat{"typ": reflect.TypeOf(msg)})
	}
}

// //Deprecated
// func (sm *SyncManager) makeNodeInfo(listenerStatus bool) *p2p.NodeInfo {
// 	nodeInfo := &p2p.NodeInfo{
// 		PubKey:  sm.privKey.PubKey().Unwrap().(crypto.PubKeyEd25519),
// 		Moniker: config.Moniker,
// 		Network: config.ChainTag,
// 		Version: version.Version,
// 		Other:   []string{strconv.FormatUint(uint64(consensus.DefaultServices), 10)},
// 	}

// 	if !sm.sw.IsListening() {
// 		return nodeInfo
// 	}

// 	p2pListener := sm.sw.Listeners()[0]

// 	// We assume that the rpcListener has the same ExternalAddress.
// 	// This is probably true because both P2P and RPC listeners use UPnP,
// 	// except of course if the api is only bound to localhost
// 	if listenerStatus {
// 		nodeInfo.ListenAddr = cmn.Fmt("%v:%v", p2pListener.ExternalAddress().IP.String(), p2pListener.ExternalAddress().Port)
// 	} else {
// 		nodeInfo.ListenAddr = cmn.Fmt("%v:%v", p2pListener.InternalAddress().IP.String(), p2pListener.InternalAddress().Port)
// 	}
// 	return nodeInfo
// }

//Start start sync manager service
func (sm *SyncManager) Start() {
	if _, err := sm.sw.Start(); err != nil {
		cmn.Exit(cmn.Fmt("fail on start SyncManager: %v", err))
	}
	// broadcast transactions
	go sm.txBroadcastLoop()
	go sm.minedBroadcastLoop()
	go sm.txSyncLoop()
}

//Stop stop sync manager
func (sm *SyncManager) Stop() {
	close(sm.quitSync)
	sm.sw.Stop()
	logging.CPrint(logging.INFO, "SyncManager stopped")
}

func (sm *SyncManager) minedBroadcastLoop() {
	for {
		select {
		case blockHash := <-sm.newBlockCh:
			block, err := sm.chain.GetBlockByHash(blockHash)
			if err != nil {
				logging.CPrint(logging.INFO, "fail on get block by hash",
					logging.LogFormat{"hash": blockHash})
				continue
			}
			if err := sm.peers.broadcastMinedBlock(block); err != nil {
				logging.CPrint(logging.ERROR, "fail on broadcast mined block",
					logging.LogFormat{"err": err})
				continue
			}
		case <-sm.quitSync:
			return
		}
	}
}
