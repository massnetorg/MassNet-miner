package poc

import "errors"

var (
	// ErrProofDecodeDataSize indicates that the data length of serialized proof is invalid.
	ErrProofDecodeDataSize = errors.New("invalid data length on decode proof")

	// ErrProofInvalidBitLength indicates that the BitLength of Proof is invalid.
	ErrProofInvalidBitLength = errors.New("invalid bitLength")

	// ErrProofInvalidFlipValue indicates that x and x_prime is not matched.
	ErrProofInvalidFlipValue = errors.New("invalid flip value")

	// ErrProofInvalidChallenge indicates that challenge is not matched with proof.
	ErrProofInvalidChallenge = errors.New("invalid challenge")
)
