package challenge

import (
	"errors"

	"massnet.org/mass/wire"
)

const (
	// New challenge is calculated based on 10 blocks before.
	ChallengeInterval = 10
	MaxReferredBlocks = ChallengeInterval * 2
)

// calcNextChallenge calculates the required challenge for the block
// after the passed previous block node based on the challenge adjustment rules.
// This function differs from the exported CalcNextChallenge in that,
// the exported version uses the current best chain as the previous block node
// while this function accepts any block node.
//
// The calculation of next challenge accords to rules as followed:
//
//  1. If the best block height ranges in [0, <ChallengeInterval>), then we use
//     the block hash of last block as next Challenge.
//
//  2. Else we calc (M = <best block height> mod <ChallengeInterval>),
//     operating M times of Hash function to proof hash of certain block,
//     use the result as next Challenge.
//
func CalcNextChallenge(lastHeader *wire.BlockHeader, referredHeaders []*wire.BlockHeader) (*wire.Hash, error) {
	//Deal with genesis block.
	if lastHeader == nil {
		return nil, errors.New("nil pointer to lastHeader")
	}

	// Deal with initial <ChallengeInterval> blocks.
	if lastHeader.Height < ChallengeInterval {
		hash := lastHeader.BlockHash()
		h := wire.HashH((&hash).Bytes())
		return &h, nil
	}

	// Deal with normal blocks.
	M := lastHeader.Height % ChallengeInterval
	if len(referredHeaders) < int(M)+ChallengeInterval+1 {
		return nil, errors.New("invalid referred headers")
	}
	referredBlock := referredHeaders[M+ChallengeInterval]

	// Get the proof hash of referred block.
	proofHash := wire.HashB(referredBlock.Proof.Encode())
	// Operate M times of hash to proofHash.
	for i := uint64(0); i < M; i++ {
		proofHash = wire.HashB(proofHash)
	}
	return wire.NewHash(proofHash)
}
