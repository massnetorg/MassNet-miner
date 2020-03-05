package wire

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	wirepb "massnet.org/mass/wire/pb"
)

// HashSize is the array size used to store sha hashes.  See Hash.
const HashSize = 32

// MaxHashStringSize is the maximum length of a Hash hash string.
const MaxHashStringSize = HashSize * 2

// ErrHashStrSize describes an error that indicates the caller specified a hash
// string that has too many characters.
var ErrHashStrSize = fmt.Errorf("max hash string length is %v bytes", MaxHashStringSize)

// Hash is used in several of the mass messages and common structures.  It
// typically represents the double sha256 of data.
type Hash [HashSize]byte

type WitnessRedeemScriptHash [sha256.Size]byte

// String returns the Hash as the hexadecimal string of the byte-reversed
// hash.
func (hash Hash) String() string {
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
//
// NOTE: This makes a copy of the bytes and should have probably been named
// CloneBytes.  It is generally cheaper to just slice the hash directly thereby
// reusing the same bytes rather than calling this method.
func (hash *Hash) Bytes() []byte {
	newHash := make([]byte, HashSize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash.  An error is returned if
// the number of bytes passed in is not HashSize.
func (hash *Hash) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != HashSize {
		return fmt.Errorf("invalid sha length of %v, want %v", nhlen,
			HashSize)
	}
	copy(hash[:], newHash)

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *Hash) IsEqual(target *Hash) bool {
	if hash == nil && target == nil {
		return true
	}
	if hash == nil || target == nil {
		return false
	}
	return *hash == *target
}

// NewHash returns a new Hash from a byte slice.  An error is returned if
// the number of bytes passed in is not HashSize.
func NewHash(newHash []byte) (*Hash, error) {
	var sh Hash
	err := sh.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &sh, err
}

// NewHashFromStr creates a Hash from a hash string.  The string should be
// the hexadecimal string of a byte-reversed hash, but any missing characters
// result in zero padding at the end of the Hash.
func NewHashFromStr(hash string) (*Hash, error) {
	ret := new(Hash)
	err := Decode(ret, hash)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func NewHashFromHash(h Hash) *Hash {
	var hash = Hash{}
	copy(hash[:], h[:])
	return &hash
}

// Decode decodes the byte-reversed hexadecimal string encoding of a Hash to a
// destination.
func Decode(dst *Hash, src string) error {
	// Return error if hash string is too long.
	if len(src) > MaxHashStringSize {
		return ErrHashStrSize
	}

	// Hex decoder expects the hash to be a multiple of two.  When not, pad
	// with a leading zero.
	var srcBytes []byte
	if len(src)%2 == 0 {
		srcBytes = []byte(src)
	} else {
		srcBytes = make([]byte, 1+len(src))
		srcBytes[0] = '0'
		copy(srcBytes[1:], src)
	}

	// Hex decode the source bytes to a temporary destination.
	var result Hash
	_, err := hex.Decode(result[HashSize-hex.DecodedLen(len(srcBytes)):], srcBytes)
	if err != nil {
		return err
	}

	copy((*dst)[:], result[:])

	return nil
}

func (hash Hash) Ptr() *Hash {
	return &hash
}

// ToProto get proto Hash from Hash
func (hash *Hash) ToProto() *wirepb.Hash {
	return &wirepb.Hash{
		S0: binary.BigEndian.Uint64(hash[0:8]),
		S1: binary.BigEndian.Uint64(hash[8:16]),
		S2: binary.BigEndian.Uint64(hash[16:24]),
		S3: binary.BigEndian.Uint64(hash[24:32]),
	}
}

// FromProto load proto Hash into wire Hash
func (hash *Hash) FromProto(pb *wirepb.Hash) error {
	if pb == nil {
		return errors.New("nil proto hash")
	}
	binary.BigEndian.PutUint64(hash[0:8], pb.S0)
	binary.BigEndian.PutUint64(hash[8:16], pb.S1)
	binary.BigEndian.PutUint64(hash[16:24], pb.S2)
	binary.BigEndian.PutUint64(hash[24:32], pb.S3)
	return nil
}

// NewHashFromProto get Hash From proto Hash
func NewHashFromProto(pb *wirepb.Hash) (*Hash, error) {
	hash := new(Hash)
	if err := hash.FromProto(pb); err != nil {
		return nil, err
	}
	return hash, nil
}
