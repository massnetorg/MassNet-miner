package wire

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
)

// ReadUint64 reads a variable from r for 8 Bytes and returns it as a uint64.
func ReadUint64(r io.Reader, pver uint32) (uint64, int, error) {
	var buf [8]byte
	n, err := r.Read(buf[0:8])
	if err != nil {
		return 0, n, err
	}
	return binary.LittleEndian.Uint64(buf[0:8]), n, nil
}

// WriteUint64 serializes val to 8 Bytes
func WriteUint64(w io.Writer, val uint64) (int, error) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[0:8], val)
	return w.Write(buf[0:8])
}

func WriteVarBytes(w io.Writer, bytes []byte) error {
	_, err := w.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}

func MoveBytes(bs []byte) []byte {
	if len(bs) == 0 {
		return make([]byte, 0)
	}
	return bs
}

// HashB calculates hash(b) and returns the resulting bytes.
func HashB(b []byte) []byte {
	hash := sha256.Sum256(b)
	return hash[:]
}

// HashH calculates hash(b) and returns the resulting bytes as a Hash.
func HashH(b []byte) Hash {
	return Hash(sha256.Sum256(b))
}

// DoubleHashB calculates sha256(sha256(b)) and returns the resulting bytes.
func DoubleHashB(b []byte) []byte {
	first := sha256.Sum256(b)
	second := sha256.Sum256(first[:])
	return second[:]
}

// DoubleHashH calculates sha256(sha256(b)) and returns the resulting bytes
// as a Hash.
func DoubleHashH(b []byte) Hash {
	first := sha256.Sum256(b)
	return Hash(sha256.Sum256(first[:]))
}
