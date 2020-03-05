package pocutil

import (
	"encoding/binary"
)

// PoCValue represents the value type of PoC Record.
type PoCValue uint64

const (
	// PoCPrefixLen represents the byte length of PoCPrefix.
	PoCPrefixLen = 4
)

var (
	// PoCPrefix is the prefix for PoC Calculation.
	PoCPrefix = []byte("MASS")
)

// P calculates MASSSHA256(PoCPrefix || PubKeyHash || X).CutByBitLength(bitLength),
// it takes in x as PoCValue.
func P(x PoCValue, bl int, pubKeyHash Hash) PoCValue {
	var xb [8]byte
	binary.LittleEndian.PutUint64(xb[:], uint64(x))

	return PB(xb[:], bl, pubKeyHash)
}

// PB calculates MASSSHA256(PoCPrefix || PubKeyHash || X).CutByBitLength(bitLength),
// it takes in x as Byte Slice.
func PB(x []byte, bl int, pubKeyHash Hash) PoCValue {
	var raw [PoCPrefixLen + 32 + 8]byte
	copy(raw[:], PoCPrefix[:])
	copy(raw[PoCPrefixLen:], pubKeyHash[:])
	copy(raw[PoCPrefixLen+32:], x)
	NormalizePoCBytes(raw[PoCPrefixLen+32:], bl)

	return CutHash(MASSSHA256(raw[:]), bl)
}

// F calculates MASSSHA256(PoCPrefix || PubKeyHash || X || XP).CutByBitLength(bitLength),
// it takes in (x, xp) as PoCValue.
func F(x, xp PoCValue, bl int, pubKeyHash Hash) PoCValue {
	var xb, xpb [8]byte
	binary.LittleEndian.PutUint64(xb[:], uint64(x))
	binary.LittleEndian.PutUint64(xpb[:], uint64(xp))

	return FB(xb[:], xpb[:], bl, pubKeyHash)
}

// FB calculates MASSSHA256(PoCPrefix || PubKeyHash || X || XP).CutByBitLength(bitLength),
// it takes in (x, xp) as Byte Slice.
func FB(x, xp []byte, bl int, pubKeyHash Hash) PoCValue {
	var raw [PoCPrefixLen + 32 + 8*2]byte
	copy(raw[:], PoCPrefix[:])
	copy(raw[PoCPrefixLen:], pubKeyHash[:])
	copy(raw[PoCPrefixLen+32:], x)
	copy(raw[PoCPrefixLen+32+8:], xp)
	NormalizePoCBytes(raw[PoCPrefixLen+32:PoCPrefixLen+32+8], bl)
	NormalizePoCBytes(raw[PoCPrefixLen+32+8:], bl)

	return CutHash(MASSSHA256(raw[:]), bl)
}
