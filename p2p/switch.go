package p2p

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/p2p/connection"
	"massnet.org/mass/p2p/discover"
	"massnet.org/mass/p2p/trust"
	"massnet.org/mass/version"
	crypto "github.com/massnetorg/tendermint/go-crypto"
	cmn "github.com/massnetorg/tendermint/tmlibs/common"
)

const (
	bannedPeerKey       = "BannedPeer"
	defaultBanDuration  = time.Hour * 1
	minNumOutboundPeers = 5

	maxBannedPeerPerIP = 50
	pollingDuration    = 30 * time.Minute

	peerIDFileName = "peer.key"
)

//pre-define errors for connecting fail
var (
	ErrDuplicatePeer     = errors.New("Duplicate peer")
	ErrConnectSelf       = errors.New("Connect self")
	ErrConnectBannedPeer = errors.New("Connect banned peer")
	ErrConnectBannedIP   = errors.New("Connect banned ip")
	ErrConnectSpvPeer    = errors.New("Outbound connect spv peer")
)

type bannedPeerInfo struct {
	Time time.Time `json:"time"`
	IP   string    `json:"ip"`
}

// Switch handles peer connections and exposes an API to receive incoming messages
// on `Reactors`.  Each `Reactor` is responsible for handling incoming messages of one
// or more `Channels`.  So while sending outgoing messages is typically performed on the peer,
// incoming messages are received on the reactor.
type Switch struct {
	cmn.BaseService

	conf         *config.Config
	peerConfig   *PeerConfig
	listeners    []Listener
	reactors     map[string]Reactor
	chDescs      []*connection.ChannelDescriptor
	reactorsByCh map[byte]Reactor
	peers        *PeerSet
	dialing      *cmn.CMap
	nodeInfo     *NodeInfo             // local node info
	nodePrivKey  crypto.PrivKeyEd25519 // local node's p2p key
	discv        *discover.Network
	bannedPeer   map[string]*bannedPeerInfo
	ipCache      map[string]map[string]struct{}
	db           discover.NetworkDB
	mtx          sync.Mutex
}

// NewSwitch creates a new Switch with the given config.
func NewSwitch(conf *config.Config) (*Switch, error) {
	sw := &Switch{
		conf:         conf,
		peerConfig:   DefaultPeerConfig(conf),
		reactors:     make(map[string]Reactor),
		chDescs:      make([]*connection.ChannelDescriptor, 0),
		reactorsByCh: make(map[byte]Reactor),
		peers:        NewPeerSet(),
		dialing:      cmn.NewCMap(),
		nodeInfo:     nil,
		nodePrivKey:  getNodeKey(path.Join(conf.Db.DataDir, peerIDFileName)),
		bannedPeer:   make(map[string]*bannedPeerInfo),
		ipCache:      make(map[string]map[string]struct{}),
	}
	sw.BaseService = *cmn.NewBaseService(nil, "P2P Switch", sw)

	// init db
	pubKey := sw.nodePrivKey.PubKey().Unwrap().(crypto.PubKeyEd25519)
	nodeDB, err := discover.NewNetworkDB(conf.Config, discover.NodeID(pubKey))
	if err != nil {
		return nil, err
	}
	sw.db = nodeDB

	dataJson, err := sw.db.Get([]byte(bannedPeerKey))
	if err != nil {
		return nil, err
	}
	if dataJson != nil {
		if err := json.Unmarshal(dataJson, &sw.bannedPeer); err != nil {
			return nil, err
		}
	}
	for peerID, info := range sw.bannedPeer {
		if time.Now().After(info.Time) {
			delete(sw.bannedPeer, peerID)
			continue
		}
		if _, ok := sw.ipCache[info.IP]; !ok {
			sw.ipCache[info.IP] = make(map[string]struct{})
		}
		sw.ipCache[info.IP][peerID] = struct{}{}
	}
	trust.Init()

	// init listener
	var listenerStatus bool
	if !conf.Network.P2P.VaultMode {
		var l Listener
		l, listenerStatus = NewDefaultListener(conf.Config)
		sw.AddListener(l)

		discv, err := initDiscover(sw, l.ExternalAddress().Port)
		if err != nil {
			return nil, err
		}
		sw.discv = discv
	}

	// init node info
	sw.nodeInfo = &NodeInfo{
		PubKey:  pubKey,
		Moniker: config.Moniker,
		Network: config.ChainTag,
		Version: version.GetVersion(),
		Other:   []string{strconv.FormatUint(uint64(consensus.DefaultServices), 10)},
	}

	if sw.IsListening() {
		p2pListener := sw.Listeners()[0]

		// We assume that the rpcListener has the same ExternalAddress.
		// This is probably true because both P2P and RPC listeners use UPnP,
		// except of course if the api is only bound to localhost
		if listenerStatus {
			sw.nodeInfo.ListenAddr = cmn.Fmt("%v:%v", p2pListener.ExternalAddress().IP.String(), p2pListener.ExternalAddress().Port)
		} else {
			sw.nodeInfo.ListenAddr = cmn.Fmt("%v:%v", p2pListener.InternalAddress().IP.String(), p2pListener.InternalAddress().Port)
		}
	}

	return sw, nil
}

func getNodeKey(name string) crypto.PrivKeyEd25519 {
	privKey, err := readKeyFile(name)
	if os.IsNotExist(err) {
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			logging.CPrint(logging.FATAL, "failed to open file", logging.LogFormat{"err": err})
		}
		defer file.Close()

		privKey = crypto.GenPrivKeyEd25519()
		writer := bufio.NewWriter(file)
		_, err = writer.Write(privKey.Bytes())
		if err != nil {
			logging.CPrint(logging.FATAL, "failed to write file", logging.LogFormat{"err": err})
		}
		err = writer.Flush()
		if err != nil {
			logging.CPrint(logging.FATAL, "failed to flush", logging.LogFormat{"err": err})
		}
		return privKey
	}
	if err != nil {
		logging.CPrint(logging.FATAL, "getNodeKey failed", logging.LogFormat{"err": err})
	}
	return privKey
}

func readKeyFile(name string) (crypto.PrivKeyEd25519, error) {
	var privKey crypto.PrivKeyEd25519
	file, err := os.Open(name)
	if err != nil {
		return privKey, err
	}
	defer file.Close()
	keyBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return privKey, err
	}
	key, err := crypto.PrivKeyFromBytes(keyBytes)
	if err != nil {
		return privKey, err
	}
	return key.Unwrap().(crypto.PrivKeyEd25519), nil
}

func initDiscover(sw *Switch, port uint16) (*discover.Network, error) {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("0.0.0.0", strconv.FormatUint(uint64(port), 10)))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	ntab, err := discover.ListenUDP(&sw.nodePrivKey, conn, sw.db, nil)
	if err != nil {
		return nil, err
	}

	// add the seeds node to the discover table
	if sw.conf.Network.P2P.Seeds == "" {
		return ntab, nil
	}
	nodes := []*discover.Node{}
	for _, seed := range strings.Split(sw.conf.Network.P2P.Seeds, ",") {
		url := "enode://" + hex.EncodeToString(crypto.Sha256([]byte(seed))) + "@" + seed
		nodes = append(nodes, discover.MustParseNode(url))
	}
	if err = ntab.SetFallbackNodes(nodes); err != nil {
		return nil, err
	}
	return ntab, nil
}

// OnStart implements BaseService. It starts all the reactors, peers, and listeners.
func (sw *Switch) OnStart() error {
	for _, reactor := range sw.reactors {
		if _, err := reactor.Start(); err != nil {
			return err
		}
	}
	for _, listener := range sw.listeners {
		go sw.listenerRoutine(listener)
	}
	go sw.ensureOutboundPeersRoutine()
	go sw.removeExpireBannedPeer()
	return nil
}

// OnStop implements BaseService. It stops all listeners, peers, and reactors.
func (sw *Switch) OnStop() {
	for _, listener := range sw.listeners {
		listener.Stop()
	}
	sw.listeners = nil

	for _, peer := range sw.peers.List() {
		peer.Stop()
		sw.peers.Remove(peer)
	}

	for _, reactor := range sw.reactors {
		reactor.Stop()
	}
	sw.discv.Close()
	sw.db.Close()
	logging.CPrint(logging.INFO, "Network db closed")
}

//AddBannedPeer add peer to blacklist
func (sw *Switch) AddBannedPeer(peerID, ip string) error {
	sw.mtx.Lock()
	defer sw.mtx.Unlock()

	expiredTime := time.Now().Add(defaultBanDuration)
	sw.bannedPeer[peerID] = &bannedPeerInfo{Time: expiredTime, IP: ip}
	if _, ok := sw.ipCache[ip]; !ok {
		sw.ipCache[ip] = make(map[string]struct{})
	}
	sw.ipCache[ip][peerID] = struct{}{}

	dataJson, err := json.Marshal(sw.bannedPeer)
	if err != nil {
		return err
	}

	return sw.db.Put([]byte(bannedPeerKey), dataJson)
}

// AddPeer performs the P2P handshake with a peer
// that already has a SecretConnection. If all goes well,
// it starts the peer and adds it to the switch.
// NOTE: This performs a blocking handshake before the peer is added.
// CONTRACT: If error is returned, peer is nil, and conn is immediately closed.
func (sw *Switch) AddPeer(pc *peerConn) error {
	peerNodeInfo, err := pc.HandshakeTimeout(sw.nodeInfo, time.Duration(sw.peerConfig.HandshakeTimeout))
	if err != nil {
		return err
	}

	if err := sw.nodeInfo.CompatibleWith(peerNodeInfo); err != nil {
		return err
	}

	peer := newPeer(pc, peerNodeInfo, sw.reactorsByCh, sw.chDescs, sw.StopPeerForError)
	if err := sw.filterConnByPeer(peer); err != nil {
		return err
	}

	if pc.outbound && !peer.ServiceFlag().IsEnable(consensus.SFFullNode) {
		return ErrConnectSpvPeer
	}

	if err = sw.peers.Add(peer); err != nil {
		return err
	}
	// Start peer
	if sw.IsRunning() {
		if err := sw.startInitPeer(peer); err != nil {
			sw.peers.Remove(peer)
			return err
		}
	}
	return nil
}

// AddReactor adds the given reactor to the switch.
// NOTE: Not goroutine safe.
func (sw *Switch) AddReactor(name string, reactor Reactor) Reactor {
	// Validate the reactor.
	// No two reactors can share the same channel.
	for _, chDesc := range reactor.GetChannels() {
		chID := chDesc.ID
		if sw.reactorsByCh[chID] != nil {
			logging.CPrint(logging.FATAL, fmt.Sprintf("Channel %X has multiple reactors %v & %v", chID, sw.reactorsByCh[chID], reactor))
		}
		sw.chDescs = append(sw.chDescs, chDesc)
		sw.reactorsByCh[chID] = reactor
	}
	sw.reactors[name] = reactor
	reactor.SetSwitch(sw)
	return reactor
}

// AddListener adds the given listener to the switch for listening to incoming peer connections.
// NOTE: Not goroutine safe.
func (sw *Switch) AddListener(l Listener) {
	sw.listeners = append(sw.listeners, l)
}

//DialPeerWithAddress dial node from net address
func (sw *Switch) DialPeerWithAddress(addr *NetAddress) error {
	logging.CPrint(logging.DEBUG, "dialing peer address", logging.LogFormat{"addr": addr})
	sw.dialing.Set(addr.IP.String(), addr)
	defer sw.dialing.Delete(addr.IP.String())
	if err := sw.filterConnByIP(addr.IP.String()); err != nil {
		return err
	}

	pc, err := newOutboundPeerConn(addr, sw.nodePrivKey, sw.peerConfig)
	if err != nil {
		logging.CPrint(logging.DEBUG, "dialPeer fail on newOutboundPeerConn", logging.LogFormat{"addr": addr, "err": err})
		return err
	}

	if err = sw.AddPeer(pc); err != nil {
		logging.CPrint(logging.DEBUG, "dialPeer fail on switch AddPeer", logging.LogFormat{"addr": addr, "err": err})
		pc.CloseConn()
		return err
	}
	logging.CPrint(logging.DEBUG, "dialPeer added peer", logging.LogFormat{"addr": addr})
	return nil
}

//IsDialing prevent duplicate dialing
func (sw *Switch) IsDialing(addr *NetAddress) bool {
	return sw.dialing.Has(addr.IP.String())
}

// IsListening returns true if the switch has at least one listener.
// NOTE: Not goroutine safe.
func (sw *Switch) IsListening() bool {
	return len(sw.listeners) > 0
}

// Listeners returns the list of listeners the switch listens on.
// NOTE: Not goroutine safe.
func (sw *Switch) Listeners() []Listener {
	return sw.listeners
}

// NumPeers Returns the count of outbound/inbound and outbound-dialing peers.
func (sw *Switch) NumPeers() (outbound, inbound, dialing int) {
	peers := sw.peers.List()
	for _, peer := range peers {
		if peer.outbound {
			outbound++
		} else {
			inbound++
		}
	}
	dialing = sw.dialing.Size()
	return
}

// NodeInfo returns the switch's NodeInfo.
// NOTE: Not goroutine safe.
func (sw *Switch) NodeInfo() *NodeInfo {
	return sw.nodeInfo
}

//Peers return switch peerset
func (sw *Switch) Peers() *PeerSet {
	return sw.peers
}

// SetNodeInfo sets the switch's NodeInfo for checking compatibility and handshaking with other nodes.
// NOTE: Not goroutine safe.
func (sw *Switch) SetNodeInfo(nodeInfo *NodeInfo) {
	sw.nodeInfo = nodeInfo
}

// SetNodePrivKey sets the switch's private key for authenticated encryption.
// NOTE: Not goroutine safe.
func (sw *Switch) SetNodePrivKey(nodePrivKey crypto.PrivKeyEd25519) {
	sw.nodePrivKey = nodePrivKey
	if sw.nodeInfo != nil {
		pubKey := nodePrivKey.PubKey().Unwrap().(crypto.PubKeyEd25519)
		sw.nodeInfo.PubKey = pubKey
	}
}

// StopPeerForError disconnects from a peer due to external error.
func (sw *Switch) StopPeerForError(peer *Peer, reason interface{}) {
	logging.CPrint(logging.DEBUG, "stopping peer for error", logging.LogFormat{"peer": peer, "err": reason})
	sw.stopAndRemovePeer(peer, reason)
}

// StopPeerGracefully disconnect from a peer gracefully.
func (sw *Switch) StopPeerGracefully(peerID string) {
	if peer := sw.peers.Get(peerID); peer != nil {
		sw.stopAndRemovePeer(peer, nil)
	}
}

func (sw *Switch) addPeerWithConnection(conn net.Conn) error {
	logging.CPrint(logging.DEBUG, "receive an inbound conn", logging.LogFormat{"remoteAddr": conn.RemoteAddr().String()})
	peerConn, err := newInboundPeerConn(conn, sw.nodePrivKey, sw.conf)
	if err != nil {
		conn.Close()
		return err
	}

	if err = sw.AddPeer(peerConn); err != nil {
		logging.CPrint(logging.DEBUG, "add inbound peer failed", logging.LogFormat{"remoteAddr": conn.RemoteAddr().String(), "err": err})
		conn.Close()
		return err
	}
	logging.CPrint(logging.DEBUG, "inbound peer added", logging.LogFormat{"remoteAddr": conn.RemoteAddr().String()})
	return nil
}

func (sw *Switch) checkBannedPeer(peerID string) error {
	sw.mtx.Lock()
	defer sw.mtx.Unlock()

	if info, ok := sw.bannedPeer[peerID]; ok {
		if time.Now().Before(info.Time) {
			return ErrConnectBannedPeer
		}
		return sw.delBannedPeer(peerID)
	}
	return nil
}

func (sw *Switch) checkBannedIP(ip string) error {
	sw.mtx.Lock()
	defer sw.mtx.Unlock()

	if len(sw.ipCache[ip]) > maxBannedPeerPerIP {
		return ErrConnectBannedIP
	}
	return nil
}

func (sw *Switch) delBannedPeer(peerID string) error {
	info := sw.bannedPeer[peerID]
	delete(sw.ipCache[info.IP], peerID)
	if len(sw.ipCache[info.IP]) == 0 {
		delete(sw.ipCache, info.IP)
	}
	delete(sw.bannedPeer, peerID)

	logging.CPrint(logging.INFO, "remove banned peer for expiration", logging.LogFormat{
		"id": peerID,
		"ip": info.IP,
	})

	dataJson, err := json.Marshal(sw.bannedPeer)
	if err != nil {
		return err
	}

	return sw.db.Put([]byte(bannedPeerKey), dataJson)
}

func (sw *Switch) filterConnByIP(ip string) error {
	if ip == sw.nodeInfo.ListenHost() {
		return ErrConnectSelf
	}
	return sw.checkBannedIP(ip)
}

func (sw *Switch) filterConnByPeer(peer *Peer) error {
	if err := sw.checkBannedPeer(peer.ID()); err != nil {
		logging.CPrint(logging.WARN, "checkBannedPeer error", logging.LogFormat{"err": err, "address": peer.Addr().String(), "id": peer.ID()})
		return err
	}

	ip, _, _ := net.SplitHostPort(peer.Addr().String())
	if err := sw.checkBannedIP(ip); err != nil {
		logging.CPrint(logging.WARN, "checkBannedIP error", logging.LogFormat{"err": err, "address": peer.Addr().String(), "id": peer.ID()})
		return err
	}

	if sw.nodeInfo.PubKey.Equals(peer.PubKey().Wrap()) {
		return ErrConnectSelf
	}

	if sw.peers.Has(peer.ID()) {
		return ErrDuplicatePeer
	}
	return nil
}

func (sw *Switch) listenerRoutine(l Listener) {
	for {
		inConn, ok := <-l.Connections()
		if !ok {
			break
		}

		// disconnect if we alrady have MaxNumPeers
		if sw.peers.Size() >= config.MaxPeers {
			inConn.Close()
			logging.CPrint(logging.INFO, "ignoring inbound connection: already have enough peers")
			continue
		}

		// New inbound connection!
		if err := sw.addPeerWithConnection(inConn); err != nil {
			logging.CPrint(logging.INFO, "ignoring inbound connection, error while adding peer", logging.LogFormat{"addr": inConn.RemoteAddr().String(), "err": err})
			continue
		}
	}
}

func (sw *Switch) dialPeerWorker(a *NetAddress, wg *sync.WaitGroup) {
	if err := sw.DialPeerWithAddress(a); err != nil {
		logging.CPrint(logging.WARN, "dialPeerWorker failed", logging.LogFormat{"addr": a, "err": err})
	}
	wg.Done()
}

func (sw *Switch) ensureOutboundPeers() {
	numOutPeers, _, numDialing := sw.NumPeers()
	numToDial := minNumOutboundPeers - (numOutPeers + numDialing)
	logging.CPrint(logging.INFO, "ensure peers", logging.LogFormat{"num_out_peers": numOutPeers, "num_dialing": numDialing, "num_to_dial": numToDial})
	if numToDial <= 0 {
		return
	}

	connectedPeers := make(map[string]struct{})
	for _, peer := range sw.Peers().List() {
		connectedPeers[peer.RemoteAddrHost()] = struct{}{}
	}

	var wg sync.WaitGroup
	nodes := make([]*discover.Node, numToDial)
	n := sw.discv.ReadRandomNodes(nodes)
	for i := 0; i < n; i++ {
		logging.CPrint(logging.INFO, "p2p random nodes", logging.LogFormat{
			"node": nodes[i].IP,
			"port": nodes[i].TCP,
		})
		try := NewNetAddressIPPort(nodes[i].IP, nodes[i].TCP)
		if sw.NodeInfo().ListenAddr == try.String() {
			continue
		}
		if dialling := sw.IsDialing(try); dialling {
			continue
		}
		if _, ok := connectedPeers[try.IP.String()]; ok {
			continue
		}

		wg.Add(1)
		go sw.dialPeerWorker(try, &wg)
	}
	wg.Wait()
}

func (sw *Switch) ensureOutboundPeersRoutine() {
	sw.ensureOutboundPeers()
	sw.ensureInitialAddPeers()
	var initialDialCount = 0
	var initialDialLimit = 10

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sw.ensureOutboundPeers()
			if initialDialCount < initialDialLimit {
				sw.ensureInitialAddPeers()
				initialDialCount++
			}
		case <-sw.Quit:
			return
		}
	}
}

func (sw *Switch) ensureInitialAddPeers() {
	connectedPeers := make(map[string]struct{})
	for _, peer := range sw.Peers().List() {
		connectedPeers[peer.RemoteAddrHost()] = struct{}{}
	}

	var wg sync.WaitGroup

	for _, addr := range sw.conf.Network.P2P.AddPeer {
		strIP, strPort, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		port, err := strconv.ParseUint(strPort, 10, 16)
		if err != nil {
			continue
		}
		try := NewNetAddressIPPort(net.ParseIP(strIP), uint16(port))
		if sw.NodeInfo().ListenAddr == try.String() {
			continue
		}
		if dialling := sw.IsDialing(try); dialling {
			continue
		}
		if _, ok := connectedPeers[try.IP.String()]; ok {
			continue
		}

		wg.Add(1)
		go sw.dialPeerWorker(try, &wg)
	}
	wg.Wait()

}

func (sw *Switch) startInitPeer(peer *Peer) error {
	peer.Start() // spawn send/recv routines
	for _, reactor := range sw.reactors {
		if err := reactor.AddPeer(peer); err != nil {
			return err
		}
	}
	return nil
}

func (sw *Switch) stopAndRemovePeer(peer *Peer, reason interface{}) {
	sw.peers.Remove(peer)
	for _, reactor := range sw.reactors {
		reactor.RemovePeer(peer, reason)
	}
	peer.Stop()
}

func (sw *Switch) removeExpireBannedPeer() {
	ticker := time.NewTicker(pollingDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sw.mtx.Lock()
			for peerID, info := range sw.bannedPeer {
				if time.Now().After(info.Time) {
					if len(sw.ipCache[info.IP]) == 0 { //trace code
						logging.CPrint(logging.ERROR, "unexpected missing", logging.LogFormat{
							"id": peerID,
							"ip": info.IP,
						})
						continue
					}
					delete(sw.ipCache[info.IP], peerID)
					if len(sw.ipCache[info.IP]) == 0 {
						delete(sw.ipCache, info.IP)
					}
					delete(sw.bannedPeer, peerID)
					logging.CPrint(logging.INFO, "remove banned peer for expiration", logging.LogFormat{
						"id": peerID,
						"ip": info.IP,
					})
				}
			}
			logging.CPrint(logging.INFO, "ban list stat", logging.LogFormat{
				"bannedIP":   len(sw.ipCache),
				"bannedPeer": len(sw.bannedPeer),
			})
			sw.mtx.Unlock()
		case <-sw.Quit:
			return
		}
	}
}
