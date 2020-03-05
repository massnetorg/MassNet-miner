package pocutil

import (
	gosha256 "crypto/sha256"
	"encoding/hex"
	"errors"

	"massnet.org/mass/poc/pocutil/crypto/sha256"
)

// ErrInvalidHashLength indicates the length of hash is invalid.
var ErrInvalidHashLength = errors.New("invalid length for hash")

// Hash represents a 32-byte hash value.
type Hash [32]byte

// SHA256 represents the standard sha256.
func SHA256(raw []byte) Hash {
	return gosha256.Sum256(raw)
}

// DoubleSHA256 represents the standard double sha256.
func DoubleSHA256(raw []byte) Hash {
	h := SHA256(raw)
	return SHA256(h[:])
}

// MASSSHA256 represents the unique MASS sha256.
func MASSSHA256(raw []byte) Hash {
	return sha256.Sum256(raw)
}

// MASSDoubleSHA256 represents the unique MASS double sha256.
func MASSDoubleSHA256(raw []byte) Hash {
	h := MASSSHA256(raw)
	return MASSSHA256(h[:])
}

// Bytes converts Hash to Byte Slice.
func (h Hash) Bytes() []byte {
	var bs Hash
	copy(bs[:], h[:])
	return bs[:]
}

// String converts Hash to String.
func (h Hash) String() string {
	return hex.EncodeToString(h.Bytes())
}

// DecodeStringToHash decodes a string value to Hash,
// the length of string value must be 64.
func DecodeStringToHash(str string) (Hash, error) {
	if len(str) != 64 {
		return Hash{}, ErrInvalidHashLength
	}
	hBytes, err := hex.DecodeString(str)
	if err != nil {
		return Hash{}, err
	}
	var h = Hash{}
	copy(h[:], hBytes)

	return h, nil
}
