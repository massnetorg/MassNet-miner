package upnp

import (
	"errors"
	"fmt"
	"net"
	"time"

	"massnet.org/mass/logging"
)

type UPNPCapabilities struct {
	PortMapping bool
	Hairpin     bool
}

func makeUPNPListener(intPort int, extPort int) (NAT, net.Listener, net.IP, error) {
	nat, err := Discover()
	if err != nil {
		return nil, nil, nil, errors.New(fmt.Sprintf("NAT upnp could not be discovered: %v", err))
	}
	logging.CPrint(logging.INFO, "our IP", logging.LogFormat{"our_ip": nat.(*upnpNAT).ourIP})

	ext, err := nat.GetExternalAddress()
	if err != nil {
		return nat, nil, nil, errors.New(fmt.Sprintf("External address error: %v", err))
	}
	logging.CPrint(logging.INFO, "external address", logging.LogFormat{"address": ext})

	port, err := nat.AddPortMapping("tcp", extPort, intPort, "Tendermint UPnP Probe", 0)
	if err != nil {
		return nat, nil, ext, errors.New(fmt.Sprintf("Port mapping error: %v", err))
	}
	logging.CPrint(logging.INFO, "port mapping mapped", logging.LogFormat{"port": port})

	// also run the listener, open for all remote addresses.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", intPort))
	if err != nil {
		return nat, nil, ext, errors.New(fmt.Sprintf("Error establishing listener: %v", err))
	}
	return nat, listener, ext, nil
}

func testHairpin(listener net.Listener, extAddr string) (supportsHairpin bool) {
	// Listener
	go func() {
		inConn, err := listener.Accept()
		if err != nil {
			logging.CPrint(logging.ERROR, "Listener.Accept() error", logging.LogFormat{"err": err})
			return
		}
		logging.CPrint(logging.INFO, "accepted incoming connection", logging.LogFormat{
			"local_addr":  inConn.LocalAddr(),
			"remote_addr": inConn.RemoteAddr(),
		})
		buf := make([]byte, 1024)
		n, err := inConn.Read(buf)
		if err != nil {
			logging.CPrint(logging.ERROR, "incoming connection read error", logging.LogFormat{"err": err})
			return
		}
		logging.CPrint(logging.INFO, fmt.Sprintf("incoming connection read %v bytes: %X", n, buf), nil)
		if string(buf) == "test data" {
			supportsHairpin = true
			return
		}
	}()

	// Establish outgoing
	outConn, err := net.Dial("tcp", extAddr)
	if err != nil {
		logging.CPrint(logging.ERROR, "outgoing connection dial error", logging.LogFormat{"err": err})
		return
	}

	n, err := outConn.Write([]byte("test data"))
	if err != nil {
		logging.CPrint(logging.ERROR, "outgoing connection write error", logging.LogFormat{"err": err})
		return
	}
	logging.CPrint(logging.INFO, fmt.Sprintf("Outgoing connection wrote %v bytes", n), nil)

	// Wait for data receipt
	time.Sleep(1 * time.Second)
	return
}

func Probe() (caps UPNPCapabilities, err error) {
	logging.CPrint(logging.INFO, "probing for UPnP")

	intPort, extPort := 8001, 8001

	nat, listener, ext, err := makeUPNPListener(intPort, extPort)
	if err != nil {
		return
	}
	caps.PortMapping = true

	// Deferred cleanup
	defer func() {
		err = nat.DeletePortMapping("tcp", intPort, extPort)
		if err != nil {
			logging.CPrint(logging.ERROR, "port mapping delete error", logging.LogFormat{"err": err})
		}
		listener.Close()
	}()

	supportsHairpin := testHairpin(listener, fmt.Sprintf("%v:%v", ext, extPort))
	if supportsHairpin {
		caps.Hairpin = true
	}

	return
}
