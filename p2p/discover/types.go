package discover

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

const (
	HashLength = 32
)

type Hash [HashLength]byte

func (h Hash) Hex() string   { return "0x" + hex.EncodeToString(h[:]) }
func (h Hash) Str() string   { return string(h[:]) }
func (h Hash) Bytes() []byte { return h[:] }

func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

func Sha3256Hash(data ...[]byte) (h Hash) {
	d := sha3.New256()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	return h
}

// Sets the hash to the value of b. If b is larger than len(h) it will panic
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}
