package config

import (
	"encoding/hex"
	"math/big"
	"time"

	"massnet.org/mass/poc"

	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

// genesisCoinbaseTx is the coinbase transaction for genesis block
var genesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  wire.Hash{},
				Index: wire.MaxPrevOutIndex,
			},
			Sequence: wire.MaxTxInSequenceNum,
			Witness:  wire.TxWitness{},
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value:    0x47868c000,
			PkScript: mustDecodeString("0020ba60494593fe65bea35fe9e118c129e5478ce660cec07c8ea8a7e2ec841fccd2"),
		},
	},
	LockTime: 0,
	Payload:  mustDecodeString("000000000000000000000000"),
}

var genesisHeader = wire.BlockHeader{
	ChainID:         mustDecodeHash("5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7"),
	Version:         1,
	Height:          0,
	Timestamp:       time.Unix(0x5d6b653e, 0), // 2019-09-01 06:29:18 +0000 UTC, 1567319358
	Previous:        mustDecodeHash("0000000000000000000000000000000000000000000000000000000000000000"),
	TransactionRoot: mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb"),
	WitnessRoot:     mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb"),
	ProposalRoot:    mustDecodeHash("9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999"),
	Target:          hexToBigInt("b5e620f48000"), // 200000000000000
	Challenge:       mustDecodeHash("5eb91b2d9fd6d5920ccc9610f0695509b60ccf764fab693ecab112f2edf1e3f0"),
	PubKey:          mustDecodePoCPublicKey("02c121b2bb27f8af5b365f1c0d9e02c2044a731aad6d0a6951ab3af506a3792c9c"),
	Proof: &poc.Proof{
		X:         mustDecodeString("acc59996"),
		XPrime:    mustDecodeString("944f0116"),
		BitLength: 32,
	},
	Signature: mustDecodePoCSignature("304402204ab4d572324785f59119a5dce949a47edb5b05cbf065e255510c23bcc9f0c133022027242ece09dee99ef19fa22d72a85a3db0662da1605300ae7610f03eab1d1a79"),
	BanList:   make([]*pocec.PublicKey, 0),
}

// genesisBlock defines the genesis block of the block chain which serves as the
// public transaction ledger.
var genesisBlock = wire.MsgBlock{
	Header: genesisHeader,
	Proposals: wire.ProposalArea{
		PunishmentArea: make([]*wire.FaultPubKey, 0),
		OtherArea:      make([]*wire.NormalProposal, 0),
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

var genesisHash = mustDecodeHash("ee26300e0f068114a680a772e080507c0f9c0ca4335c382c42b78e2eafbebaa3")

var genesisChainID = mustDecodeHash("5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7")

func hexToBigInt(str string) *big.Int {
	return new(big.Int).SetBytes(mustDecodeString(str))
}

func mustDecodeString(str string) []byte {
	buf, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}
	return buf
}

func mustDecodeHash(str string) wire.Hash {
	h, err := wire.NewHashFromStr(str)
	if err != nil {
		panic(err)
	}
	return *h
}

func mustDecodePoCPublicKey(str string) *pocec.PublicKey {
	pub, err := pocec.ParsePubKey(mustDecodeString(str), pocec.S256())
	if err != nil {
		panic(err)
	}
	return pub
}

func mustDecodePoCSignature(str string) *pocec.Signature {
	sig, err := pocec.ParseSignature(mustDecodeString(str), pocec.S256())
	if err != nil {
		panic(err)
	}
	return sig
}
