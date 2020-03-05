package netsync

import (
	"encoding/hex"
	"net"
	"sync"

	set "gopkg.in/fatih/set.v0"
	"massnet.org/mass/consensus"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/ccache"
	"massnet.org/mass/p2p/trust"
	"massnet.org/mass/wire"
)

const (
	maxKnownTxs         = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks      = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
	defaultBanThreshold = uint64(100)
	maxBanScoreCache    = 1000
)

//BasePeer is the interface for connection level peer
type BasePeer interface {
	Addr() net.Addr
	ID() string
	ServiceFlag() consensus.ServiceFlag
	TrySend(byte, interface{}) bool
	IsOutbound() bool
}

//BasePeerSet is the intergace for connection level peer manager
type BasePeerSet interface {
	AddBannedPeer(string, string) error
	StopPeerGracefully(string)
}

// PeerInfo indicate peer status snap
type PeerInfo struct {
	ID         string `json:"peer_id"`
	RemoteAddr string `json:"remote_addr"`
	Height     uint64 `json:"height"`
	IsOutbound bool   `json:"is_outbound"`
	Delay      uint32 `json:"delay"`
}

type peer struct {
	BasePeer
	mtx         sync.RWMutex
	services    consensus.ServiceFlag
	height      uint64
	hash        *wire.Hash
	banScore    trust.DynamicBanScore
	knownTxs    *set.Set // Set of transaction hashes known to be known by this peer
	knownBlocks *set.Set // Set of block hashes known to be known by this peer
	filterAdds  *set.Set // Set of addresses that the spv node cares about.
}

func newPeer(height uint64, hash *wire.Hash, basePeer BasePeer) *peer {
	return &peer{
		BasePeer:    basePeer,
		services:    basePeer.ServiceFlag(),
		height:      height,
		hash:        hash,
		knownTxs:    set.New(set.ThreadSafe).(*set.Set),
		knownBlocks: set.New(set.ThreadSafe).(*set.Set),
		filterAdds:  set.New(set.ThreadSafe).(*set.Set),
	}
}

func (p *peer) Height() uint64 {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.height
}

func (p *peer) Hash() *wire.Hash {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	hash, _ := wire.NewHash(p.hash.Bytes())
	return hash
}

func (p *peer) addBanScore(persistent, transient uint64, reason string) bool {
	score := p.banScore.Increase(persistent, transient)
	if score > defaultBanThreshold {
		logging.CPrint(logging.ERROR, "banning and disconnecting", logging.LogFormat{"address": p.Addr(), "score": score, "reason": reason})
		return true
	}

	warnThreshold := defaultBanThreshold >> 1
	if score > warnThreshold {
		logging.CPrint(logging.WARN, "ban score increasing", logging.LogFormat{"address": p.Addr(), "score": score, "reason": reason})
	}
	return false
}

func (p *peer) addFilterAddress(address []byte) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if size := p.filterAdds.Size(); size >= maxFilterAddressCount {
		logging.CPrint(logging.WARN, "the count of filter addresses is greater than limit",
			logging.LogFormat{"size": size, "limit": maxFilterAddressCount})
		return
	}
	if size := len(address); size > maxFilterAddressSize {
		logging.CPrint(logging.WARN, "the size of filter address is greater than limit",
			logging.LogFormat{"size": size, "limit": maxFilterAddressSize})
		return
	}
	p.filterAdds.Add(hex.EncodeToString(address))
}

func (p *peer) addFilterAddresses(addresses [][]byte) {
	if !p.filterAdds.IsEmpty() {
		p.filterAdds.Clear()
	}
	for _, address := range addresses {
		p.addFilterAddress(address)
	}
}

func (p *peer) getBlockByHeight(height uint64) bool {
	msg := struct{ BlockchainMessage }{&GetBlockMessage{Height: height}}
	return p.TrySend(BlockchainChannel, msg)
}

func (p *peer) getBlocks(locator []*wire.Hash, stopHash *wire.Hash) bool {
	msg := struct{ BlockchainMessage }{NewGetBlocksMessage(locator, stopHash)}
	return p.TrySend(BlockchainChannel, msg)
}

func (p *peer) getHeaderByHeight(height uint64) bool {
	msg := struct{ BlockchainMessage }{&GetHeaderMessage{Height: height}}
	return p.TrySend(BlockchainChannel, msg)
}

func (p *peer) getHeaders(locator []*wire.Hash, stopHash *wire.Hash) bool {
	msg := struct{ BlockchainMessage }{NewGetHeadersMessage(locator, stopHash)}
	return p.TrySend(BlockchainChannel, msg)
}

func (p *peer) getPeerInfo() *PeerInfo {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return &PeerInfo{
		ID:         p.ID(),
		RemoteAddr: p.Addr().String(),
		Height:     p.height,
		IsOutbound: p.IsOutbound(),
	}
}

func (p *peer) isRelatedTx(tx *massutil.Tx) bool {
	/*for _, input := range tx.MsgTx().TxIn {
			if p.filterAdds.Has(hex.EncodeToString(input.PreviousOutPoint.Hash[:])) {
				return true
			}
		}
	for _, output := range tx.MsgTx().TxOut {
		if p.filterAdds.Has(hex.EncodeToString(output.PkScript)) {
			return true
		}
	}*/
	return false
}

func (p *peer) isSPVNode() bool {
	return !p.services.IsEnable(consensus.SFFullNode)
}

func (p *peer) markBlock(hash *wire.Hash) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for p.knownBlocks.Size() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash.String())
}

func (p *peer) markTransaction(hash *wire.Hash) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for p.knownTxs.Size() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash.String())
}

func (p *peer) sendBlock(block *massutil.Block) (bool, error) {
	msg, err := NewBlockMessage(block)
	if err != nil {
		return false, errors.Wrap(err, "fail on NewBlockMessage")
	}

	ok := p.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg})
	if ok {
		blcokHash := block.Hash()
		p.knownBlocks.Add(blcokHash.String())
	}
	return ok, nil
}

func (p *peer) sendBlocks(blockHashes []*wire.Hash, rawBlocks [][]byte) (bool, error) {
	msg := &BlocksMessage{rawBlocks}
	if ok := p.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
		return ok, nil
	}

	for _, blockHash := range blockHashes {
		p.knownBlocks.Add(blockHash.String())
	}
	return true, nil
}

func (p *peer) sendHeader(header *wire.BlockHeader) (bool, error) {
	msg, err := NewHeaderMessage(header)
	if err != nil {
		return false, errors.Wrap(err, "fail on NewHeaderMessage")
	}

	ok := p.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg})
	return ok, nil
}

func (p *peer) sendHeaders(headers []*wire.BlockHeader) (bool, error) {
	msg, err := NewHeadersMessage(headers)
	if err != nil {
		return false, errors.New("fail on NewHeadersMessage")
	}

	ok := p.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg})
	return ok, nil
}

func (p *peer) sendTransactions(txs []*massutil.Tx) (bool, error) {
	for _, tx := range txs {
		if p.isSPVNode() && !p.isRelatedTx(tx) {
			continue
		}
		msg, err := NewTransactionMessage(tx)
		if err != nil {
			return false, errors.Wrap(err, "failed to tx msg")
		}

		if p.knownTxs.Has(tx.Hash().String()) {
			continue
		}
		if ok := p.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
			return ok, nil
		}
		p.knownTxs.Add(tx.Hash().String())
	}
	return true, nil
}

func (p *peer) setStatus(height uint64, hash *wire.Hash) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.height = height
	p.hash = hash
}

type peerSet struct {
	BasePeerSet
	mtx           sync.RWMutex
	peers         map[string]*peer
	banScoreCache *ccache.CCache
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet(basePeerSet BasePeerSet) *peerSet {
	return &peerSet{
		BasePeerSet:   basePeerSet,
		peers:         make(map[string]*peer),
		banScoreCache: ccache.NewCCache(maxBanScoreCache),
	}
}

func (ps *peerSet) addBanScore(peerID string, persistent, transient uint64, reason string) {
	ps.mtx.Lock()
	peer := ps.peers[peerID]
	ps.mtx.Unlock()

	if peer == nil {
		return
	}
	if ban := peer.addBanScore(persistent, transient, reason); !ban {
		return
	}
	ip, _, _ := net.SplitHostPort(peer.Addr().String())
	if err := ps.AddBannedPeer(peerID, ip); err != nil {
		logging.CPrint(logging.ERROR, "fail on add ban peer", logging.LogFormat{"err": err})
	}
	logging.CPrint(logging.INFO, "add banned peer", logging.LogFormat{"id": peerID, "address": peer.Addr().String()})
	ps.removePeer(peerID)
	// if peer banned, no need to keep score
	ps.banScoreCache.Remove(peerID)
}

func (ps *peerSet) addPeer(peer BasePeer, height uint64, hash *wire.Hash) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if _, ok := ps.peers[peer.ID()]; !ok {
		ps.peers[peer.ID()] = newPeer(height, hash, peer)
		if value, ok := ps.banScoreCache.Get(peer.ID()); ok {
			banScore, ok := value.(*trust.DynamicBanScore)
			if ok {
				ps.peers[peer.ID()].banScore.ResetFrom(banScore)
			}
		}
		return
	}
	logging.CPrint(logging.WARN, "add existing peer to blockKeeper", logging.LogFormat{"id": peer.ID()})
}

func (ps *peerSet) bestPeer(flag consensus.ServiceFlag) *peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	var bestPeer *peer
	for _, p := range ps.peers {
		if !p.services.IsEnable(flag) {
			continue
		}
		if bestPeer == nil || p.height > bestPeer.height {
			bestPeer = p
		}
	}
	return bestPeer
}

func (ps *peerSet) broadcastMinedBlock(block *massutil.Block) error {
	msg, err := NewMinedBlockMessage(block)
	if err != nil {
		return errors.Wrap(err, "fail on broadcast mined block")
	}

	hash := block.Hash()
	peers := ps.peersWithoutBlock(hash)
	for _, peer := range peers {
		if peer.isSPVNode() {
			continue
		}
		if ok := peer.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
			ps.removePeer(peer.ID())
			continue
		}
		peer.markBlock(hash)
	}
	return nil
}

func (ps *peerSet) broadcastNewStatus(bestBlock, genesisBlock *massutil.Block) error {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	bestBlockHash := bestBlock.Hash()
	peers := ps.peersWithoutBlock(bestBlockHash)

	genesisHash := genesisBlock.Hash()
	msg := NewStatusResponseMessage(&bestBlock.MsgBlock().Header, genesisHash)
	for _, peer := range peers {
		if ok := peer.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
			ps.removePeer(peer.ID())
			continue
		}
	}
	return nil
}

func (ps *peerSet) broadcastTx(tx *massutil.Tx) error {
	msg, err := NewTransactionMessage(tx)
	if err != nil {
		return errors.Wrap(err, "fail on broadcast tx")
	}

	peers := ps.peersWithoutTx(tx.Hash())
	for _, peer := range peers {
		if peer.isSPVNode() && !peer.isRelatedTx(tx) {
			continue
		}
		if ok := peer.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg}); !ok {
			ps.removePeer(peer.ID())
			continue
		}
		peer.markTransaction(tx.Hash())
	}
	return nil
}

func (ps *peerSet) errorHandler(peerID string, err error) {
	if errors.Root(err) == errPeerMisbehave {
		ps.addBanScore(peerID, 20, 0, err.Error())
	} else {
		ps.removePeer(peerID)
	}
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) getPeer(id string) *peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.peers[id]
}

func (ps *peerSet) getPeerInfos() []*PeerInfo {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	result := []*PeerInfo{}
	for _, peer := range ps.peers {
		result = append(result, peer.getPeerInfo())
	}
	return result
}

func (ps *peerSet) getPeerCount() int {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	return len(ps.peers)
}

func (ps *peerSet) peersWithoutBlock(hash *wire.Hash) []*peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	peers := []*peer{}
	for _, peer := range ps.peers {
		if !peer.knownBlocks.Has(hash.String()) {
			peers = append(peers, peer)
		}
	}
	return peers
}

func (ps *peerSet) peersWithoutTx(hash *wire.Hash) []*peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	peers := []*peer{}
	for _, peer := range ps.peers {
		if !peer.knownTxs.Has(hash.String()) {
			peers = append(peers, peer)
		}
	}
	return peers
}

func (ps *peerSet) removePeer(peerID string) {
	ps.mtx.Lock()
	if p, ok := ps.peers[peerID]; ok {
		if p.banScore.Int() != 0 {
			ps.banScoreCache.Add(peerID, &p.banScore)
		}
		delete(ps.peers, peerID)
	}
	ps.mtx.Unlock()
	ps.StopPeerGracefully(peerID)
}
