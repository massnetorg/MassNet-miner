package massutil

import (
	"crypto/sha256"
	"hash"

	"golang.org/x/crypto/ripemd160"
)

// Calculate the hash of hasher over buf.
func calcHash(buf []byte, hasher hash.Hash) []byte {
	hasher.Write(buf)
	return hasher.Sum(nil)
}

// Hash160 returns ripemd160(sha256(b)).
func Hash160(data []byte) []byte {
	s := sha256.Sum256(data)
	return calcHash(s[:], ripemd160.New())
}

// Hash256 returns sha256(sha256(data))
func Hash256(data []byte) []byte {
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	return h2[:]
}

// Sha256 returns sha256(data)
func Sha256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// Ripemd160 return ripemd16(data)
func Ripemd160(data []byte) []byte {
	return calcHash(data, ripemd160.New())
}
