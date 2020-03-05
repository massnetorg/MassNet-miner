package pocutil

import (
	"encoding/binary"
	"math/big"

	"massnet.org/mass/pocec"
)

// CutBigInt cuts big.Int to uint64, masking result to bitLength value.
func CutBigInt(bi *big.Int, bl int) PoCValue {
	mask := PoCValue((1 << uint(bl)) - 1)
	return mask & PoCValue(bi.Uint64())
}

// CutHash cuts the least significant 8 bytes of Hash, decoding it
// as little endian uint64, and masking result to bitLength value.
func CutHash(hash Hash, bl int) PoCValue {
	mask := PoCValue((1 << uint(bl)) - 1)
	h64 := binary.LittleEndian.Uint64(hash[:8])
	return mask & PoCValue(h64)
}

// FlipValue flips(bit-flip) the bitLength bits of a PoCValue.
func FlipValue(v PoCValue, bl int) PoCValue {
	mask := PoCValue((1 << uint(bl)) - 1)
	return mask & (mask ^ v)
}

// PoCValue2Bytes encodes a PoCValue to bytes,
// size depending on its bitLength.
func PoCValue2Bytes(v PoCValue, bl int) []byte {
	size := RecordSize(bl)
	mask := PoCValue((1 << uint(bl)) - 1)
	v = mask & v
	var vb [8]byte
	binary.LittleEndian.PutUint64(vb[:], uint64(v))
	return vb[:size]
}

// Bytes2PoCValue decodes a PoCValue from bytes,
// size depending on its bitLength.
func Bytes2PoCValue(vb []byte, bl int) PoCValue {
	mask := PoCValue((1 << uint(bl)) - 1)
	var b8 [8]byte
	copy(b8[:], vb)
	v := PoCValue(binary.LittleEndian.Uint64(b8[:8]))
	return mask & v
}

// RecordSize returns the size of a bitLength record,
// in byte unit.
func RecordSize(bl int) int {
	return (bl + 7) >> 3
}

// NormalizePoCBytes sets vb and returns vb according to its bitLength,
// upper bits would be set to zero.
func NormalizePoCBytes(vb []byte, bl int) []byte {
	size := RecordSize(bl)
	if len(vb) < size {
		return vb
	}
	if rem := bl % 8; rem != 0 {
		vb[size-1] &= 1<<uint(rem) - 1
	}
	for i := size; i < len(vb); i++ {
		vb[i] = 0
	}
	return vb
}

// PubKeyHash returns the hash of pubKey.
func PubKeyHash(pubKey *pocec.PublicKey) Hash {
	if pubKey == nil {
		return Hash{}
	}
	return DoubleSHA256(pubKey.SerializeCompressed())
}
