// +build !network

package p2p

import (
	"bytes"
	"testing"

	configpb "massnet.org/mass/config/pb"
	"massnet.org/mass/testutil"
)

func TestListener(t *testing.T) {
	testutil.SkipCI(t)
	// Create a listener
	cfg := &configpb.Config{
		Network: &configpb.NetworkConfig{
			P2P: &configpb.P2PConfig{
				SkipUpnp:      true,
				ListenAddress: "tcp://0.0.0.0:43480",
			},
		},
	}
	l, _ := NewDefaultListener(cfg)

	// Dial the listener
	lAddr := l.ExternalAddress()
	connOut, err := lAddr.Dial()
	if err != nil {
		t.Fatalf("Could not connect to listener address %v", lAddr)
	} else {
		t.Logf("Created a connection to listener address %v", lAddr)
	}
	connIn, ok := <-l.Connections()
	if !ok {
		t.Fatalf("Could not get inbound connection from listener")
	}

	msg := []byte("hi!")
	go connIn.Write(msg)
	b := make([]byte, 32)
	n, err := connOut.Read(b)
	if err != nil {
		t.Fatalf("Error reading off connection: %v", err)
	}

	b = b[:n]
	if !bytes.Equal(msg, b) {
		t.Fatalf("Got %s, expected %s", b, msg)
	}

	// Close the server, no longer needed.
	l.Stop()
}
