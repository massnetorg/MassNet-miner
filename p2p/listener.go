package p2p

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	configpb "massnet.org/mass/config/pb"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/p2p/upnp"
	cmn "github.com/massnetorg/tendermint/tmlibs/common"
)

const (
	numBufferedConnections = 10
	defaultExternalPort    = 8770
	tryListenTimes         = 5
)

//Listener subset of the methods of DefaultListener
type Listener interface {
	Connections() <-chan net.Conn
	InternalAddress() *NetAddress
	ExternalAddress() *NetAddress
	String() string
	Stop() bool
}

//getUPNPExternalAddress UPNP external address discovery & port mapping
func getUPNPExternalAddress(externalPort, internalPort int) (*NetAddress, error) {
	nat, err := upnp.Discover()
	if err != nil {
		return nil, errors.Wrap(err, "could not perform UPNP discover")
	}

	ext, err := nat.GetExternalAddress()
	if err != nil {
		return nil, errors.Wrap(err, "could not perform UPNP external address")
	}

	if externalPort == 0 {
		externalPort = defaultExternalPort
	}
	externalPort, err = nat.AddPortMapping("tcp", externalPort, internalPort, "mass tcp", 0)
	if err != nil {
		return nil, errors.Wrap(err, "could not add tcp UPNP port mapping")
	}
	externalPort, err = nat.AddPortMapping("udp", externalPort, internalPort, "mass udp", 0)
	if err != nil {
		return nil, errors.Wrap(err, "could not add udp UPNP port mapping")
	}
	return NewNetAddressIPPort(ext, uint16(externalPort)), nil
}

func getNaiveExternalAddress(port int, settleForLocal bool) *NetAddress {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logging.CPrint(logging.FATAL, "Could not fetch interface addresses", logging.LogFormat{"err": err})
	}

	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 == nil || (!settleForLocal && v4[0] == 127) {
			continue
		}
		return NewNetAddressIPPort(ipnet.IP, uint16(port))
	}

	logging.CPrint(logging.INFO, "node may not be connected to internet, settling for local address", logging.LogFormat{"post": port, "settle_local": settleForLocal})
	return getNaiveExternalAddress(port, true)
}

func splitHostPort(addr string) (host string, port int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		logging.CPrint(logging.FATAL, "err in net.SplitHostPort", logging.LogFormat{"err": err, "addr": addr})
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		logging.CPrint(logging.FATAL, "err in strconv.Atoi", logging.LogFormat{"err": err, "portStr": portStr})
	}
	return host, port
}

//DefaultListener Implements mass server Listener
type DefaultListener struct {
	cmn.BaseService

	listener    net.Listener
	intAddr     *NetAddress
	extAddr     *NetAddress
	connections chan net.Conn
}

// Defaults to tcp
func protocolAndAddress(listenAddr string) (string, string) {
	p, address := "tcp", listenAddr
	parts := strings.SplitN(address, "://", 2)
	if len(parts) == 2 {
		p, address = parts[0], parts[1]
	}
	return p, address
}

//NewDefaultListener create a default listener
func NewDefaultListener(cfg *configpb.Config) (Listener, bool) {
	protocol, lAddr := protocolAndAddress(cfg.Network.P2P.ListenAddress)
	skipUPNP := cfg.Network.P2P.SkipUpnp
	// Local listen IP & port
	lAddrIP, lAddrPort := splitHostPort(lAddr)

	listener, err := net.Listen(protocol, lAddr)
	for i := 0; i < tryListenTimes && err != nil; i++ {
		time.Sleep(time.Second * 1)
		listener, err = net.Listen(protocol, lAddr)
	}
	if err != nil {
		logging.CPrint(logging.FATAL, err.Error(), logging.LogFormat{"protocol": protocol, "lAddr": lAddr})
	}

	intAddr, err := NewNetAddressString(lAddr)
	if err != nil {
		logging.CPrint(logging.FATAL, err.Error(), logging.LogFormat{"lAddr": lAddr})
	}

	// Actual listener local IP & port
	listenerIP, listenerPort := splitHostPort(listener.Addr().String())
	logging.CPrint(logging.INFO, "Local listener", logging.LogFormat{" ip:": listenerIP, " port:": listenerPort})

	// Determine external address...
	var extAddr *NetAddress
	var upnpMap bool
	if !skipUPNP && (lAddrIP == "" || lAddrIP == "0.0.0.0") {
		extAddr, err = getUPNPExternalAddress(lAddrPort, listenerPort)
		upnpMap = err == nil
		logging.CPrint(logging.INFO, "get UPNP external address", logging.LogFormat{"err": err})
	}

	if extAddr == nil {
		if address := GetIP(); address.Success {
			extAddr = NewNetAddressIPPort(net.ParseIP(address.IP), uint16(lAddrPort))
		}
	}
	if extAddr == nil {
		extAddr = getNaiveExternalAddress(listenerPort, false)
	}
	if extAddr == nil {
		logging.CPrint(logging.FATAL, "could not determine external address!")
	}

	dl := &DefaultListener{
		listener:    listener,
		intAddr:     intAddr,
		extAddr:     extAddr,
		connections: make(chan net.Conn, numBufferedConnections),
	}
	dl.BaseService = *cmn.NewBaseService(nil, "DefaultListener", dl)
	dl.Start() // Started upon construction
	if upnpMap {
		return dl, true
	}

	conn, err := net.DialTimeout("tcp", extAddr.String(), 3*time.Second)
	if err != nil {
		return dl, false
	}
	conn.Close()
	return dl, true
}

//OnStart start listener
func (l *DefaultListener) OnStart() error {
	l.BaseService.OnStart()
	go l.listenRoutine()
	return nil
}

//OnStop stop listener
func (l *DefaultListener) OnStop() {
	l.BaseService.OnStop()
	l.listener.Close()
}

//listenRoutine Accept connections and pass on the channel
func (l *DefaultListener) listenRoutine() {
	for {
		conn, err := l.listener.Accept()
		if !l.IsRunning() {
			break // Go to cleanup
		}
		// listener wasn't stopped,
		// yet we encountered an error.
		if err != nil {
			logging.CPrint(logging.FATAL, err.Error())
		}
		l.connections <- conn
	}
	// Cleanup
	close(l.connections)
}

//Connections a channel of inbound connections. It gets closed when the listener closes.
func (l *DefaultListener) Connections() <-chan net.Conn {
	return l.connections
}

//InternalAddress listener internal address
func (l *DefaultListener) InternalAddress() *NetAddress {
	return l.intAddr
}

//ExternalAddress listener external address for remote peer dial
func (l *DefaultListener) ExternalAddress() *NetAddress {
	return l.extAddr
}

// NetListener the returned listener is already Accept()'ing. So it's not suitable to pass into http.Serve().
func (l *DefaultListener) NetListener() net.Listener {
	return l.listener
}

//String string of default listener
func (l *DefaultListener) String() string {
	return fmt.Sprintf("Listener(@%v)", l.extAddr)
}
