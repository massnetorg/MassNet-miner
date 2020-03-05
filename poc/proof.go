package poc

import (
	"encoding/binary"
	"math"
	"math/big"

	"massnet.org/mass/poc/pocutil"
)

const (
	// PoCSlot represents the unit Slot of PoC
	PoCSlot = 3

	// KiB represents KiByte
	KiB = 1024

	// MiB represents MiByte
	MiB = 1024 * KiB

	// MinValidBitLength represents smallest BitLength
	MinValidBitLength = 24

	// MaxValidBitLength represents biggest BitLength
	MaxValidBitLength = 40
)

var (
	// BitLengthDiskSize stores the disk size in byte of different BitLengths.
	BitLengthDiskSize map[int]int

	// MinDiskSize represents the disk size in byte of the smallest BitLength.
	MinDiskSize = pocutil.RecordSize(MinValidBitLength) * 2 * (1 << uint(MinValidBitLength))

	// validBitLength represents a slice of valid BitLength in increasing order.
	validBitLength []int
)

func init() {
	BitLengthDiskSize = make(map[int]int)
	for i := MinValidBitLength; i <= MaxValidBitLength; i = i + 2 {
		validBitLength = append(validBitLength, i)
	}
	for _, bl := range validBitLength {
		BitLengthDiskSize[bl] = pocutil.RecordSize(bl) * 2 * (1 << uint(bl))
	}
}

// Proof represents a single PoC Proof.
type Proof struct {
	X         []byte
	XPrime    []byte
	BitLength int
}

// EnsureBitLength returns whether it is a valid bitLength.
func EnsureBitLength(bl int) bool {
	if bl >= MinValidBitLength && bl <= MaxValidBitLength && bl%2 == 0 {
		return true
	}
	return false
}

// ValidBitLength returns a slice of valid BitLength in increasing order.
func ValidBitLength() []int {
	sli := make([]int, len(validBitLength))
	copy(sli, validBitLength)
	return sli
}

// Encode encodes proof to 17 bytes:
// |    X    | XPrime  | BitLength |
// | 8 bytes | 8 bytes |   1 byte  |
// X & XPrime is encoded in little endian
func (proof *Proof) Encode() []byte {
	var data [17]byte
	copy(data[:8], proof.X)
	copy(data[8:16], proof.XPrime)
	data[16] = byte(proof.BitLength)

	return data[:]
}

// Decode decodes proof from a 17-byte slice:
// |    X    | XPrime  | BitLength |
// | 8 bytes | 8 bytes |   1 byte  |
// X & XPrime is encoded in little endian
func (proof *Proof) Decode(data []byte) error {
	if len(data) != 17 {
		return ErrProofDecodeDataSize
	}
	proof.BitLength = int(data[16])
	proof.X = pocutil.PoCValue2Bytes(pocutil.PoCValue(binary.LittleEndian.Uint64(data[:8])), proof.BitLength)
	proof.XPrime = pocutil.PoCValue2Bytes(pocutil.PoCValue(binary.LittleEndian.Uint64(data[8:16])), proof.BitLength)
	return nil
}

// VerifyProof verifies proof:
// (1) make sure BitLength is Valid. Should be integer even number in [24, 40].
// (2) perform function P on x and x_prime, the corresponding result
//     y and y_prime should be a bit-flip pair.
// (3) perform function F on x and x_prime, the result z should
//     be equal to the bit-length-cut challenge.
// It returns nil when proof is verified.
func VerifyProof(proof *Proof, pubKeyHash pocutil.Hash, challenge pocutil.Hash) error {
	bl := proof.BitLength
	if !EnsureBitLength(bl) {
		return ErrProofInvalidBitLength
	}

	y := pocutil.PB(proof.X, bl, pubKeyHash)
	yp := pocutil.PB(proof.XPrime, bl, pubKeyHash)
	if y != pocutil.FlipValue(yp, bl) {
		return ErrProofInvalidFlipValue
	}

	cShort := pocutil.CutHash(challenge, bl)
	z := pocutil.FB(proof.X, proof.XPrime, bl, pubKeyHash)
	if cShort != z {
		return ErrProofInvalidChallenge
	}

	return nil
}

// GetQuality produces the relative quality of a proof.
//
// Here we define:
// (1) H: (a hash value) as an (32-byte-big-endian-encoded) integer ranges in 0 ~ 2^256 - 1.
// (2) SIZE: the volume of record of certain BitLength, which equals to 2^BitLength.
//
// The standard quality is : quality = (H / 2^256) ^ [1 / (SIZE * BitLength)],
// which means the more space you have, the bigger prob you get to
// generate a higher quality.
//
// In MASS we use an equivalent quality formula : Quality = (SIZE * BitLength) / [256 - log2(H)],
// which means the more space you have, the bigger prob you get to
// generate a higher Quality.
//
// A proof is considered as valid when Quality >= target.
func (proof *Proof) GetQuality(slot, height uint64) *big.Int {
	hashVal := proof.GetHashVal(slot, height)
	// Note: Q1 = SIZE * BL
	Q1 := big.NewFloat(float64(uint64(1 << uint(proof.BitLength) * proof.BitLength)))

	// Note: FH = H in big.Float
	FH := new(big.Float).SetInt(new(big.Int).SetBytes(hashVal[:]))
	F64H, _ := FH.Float64()

	// Note: log2FH = log2(H)
	log2FH := big.NewFloat(math.Log2(F64H))

	// Note: Q2 = 256 - log2(H)
	Q2 := big.NewFloat(256)
	Q2.Sub(Q2, log2FH)
	if Q2.Cmp(big.NewFloat(0)) <= 0 {
		return big.NewInt(0)
	}

	Quality, _ := Q1.Quo(Q1, Q2).Int(nil)
	return Quality
}

// GetVerifiedQuality verifies the proof and then calculates its quality.
func (proof *Proof) GetVerifiedQuality(pubKeyHash pocutil.Hash, challenge pocutil.Hash, slot, height uint64) (*big.Int, error) {
	if err := VerifyProof(proof, pubKeyHash, challenge); err != nil {
		return nil, err
	}
	return proof.GetQuality(slot, height), nil
}

// GetHashVal returns SHA256(t//s,x,x',height).
func (proof *Proof) GetHashVal(slot uint64, height uint64) pocutil.Hash {
	var b32 [32]byte
	binary.LittleEndian.PutUint64(b32[:], slot)
	copy(b32[8:], proof.X)
	copy(b32[16:], proof.XPrime)
	binary.LittleEndian.PutUint64(b32[24:], height)

	return pocutil.SHA256(b32[:])
}

// NewEmptyProof returns a new Proof struct.
func NewEmptyProof() *Proof {
	return &Proof{
		X:         make([]byte, 0, 8),
		XPrime:    make([]byte, 0, 8),
		BitLength: 0,
	}
}
