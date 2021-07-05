package keystore

import (
	"encoding/hex"
	"testing"

	"github.com/massnetorg/mass-core/pocec"
	"massnet.org/mass/config"
)

func TestNewPoCAddress(t *testing.T) {
	buf, err := hex.DecodeString(samplePublicKey)
	if err != nil {
		t.Fatalf("failed to decode hex string, %v", err)
	}
	samplePk, err := pocec.ParsePubKey(buf, pocec.S256())
	if err != nil {
		t.Fatalf("failed to parse public key, %v", err)
	}

	// nil
	_, _, err = NewPoCAddress(nil, config.ChainParams)
	if err != ErrNilPointer {
		t.Fatalf("failed to catch error, %v", err)
	}

	scriptHash, address, err := NewPoCAddress(samplePk, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new poc address, %v", err)
	}
	t.Logf("scriptHash: %v", scriptHash)
	t.Logf("address: %v", address)
}
