package mock

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

func mockGenesisBlock(timestamp int64, target *big.Int) *wire.MsgBlock {
	// Mock chain
	opt := &Option{
		Mode:                Auto,
		TotalHeight:         20,
		TxPerBlock:          1,
		MinNormalTxPerBlock: 1,
	}
	chain, err := NewMockedChain(opt)
	if err != nil {
		panic(err)
	}

	// fetch block at height 10
	genesisBlock := chain.Blocks()[10]
	mockedTx := genesisBlock.Transactions[0]

	var coinbasePayload = func(height int64) []byte {
		//sb := txscript.NewScriptBuilder().AddCoinbaseHeight(height)
		sb := txscript.NewScriptBuilder().AddInt64(0)
		sb = sb.AddData([]byte("MASS TESTNET"))
		payload, err := sb.Script()
		if err != nil {
			panic(err)
		}
		return payload
	}

	genesisBlock.Transactions = []*wire.MsgTx{
		{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash:  wire.Hash{},
						Index: 0xffffffff,
					},
					Sequence: wire.MaxTxInSequenceNum,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value:    50e8,
					PkScript: mockedTx.TxOut[0].PkScript,
				},
			},
			LockTime: 0,
			Payload:  coinbasePayload(0),
		},
	}

	// // fill witness commitment
	// fillCoinbaseCommitment(massutil.NewBlock(genesisBlock))

	// fill block header
	mockFillGenesisHeader(genesisBlock, timestamp, target)

	return genesisBlock
}

func mockFillGenesisHeader(blk *wire.MsgBlock, timestamp int64, target *big.Int) {
	// calc transaction root
	merkles := wire.BuildMerkleTreeStoreTransactions(blk.Transactions, false)
	txRoot := *merkles[len(merkles)-1]
	// calc sig2
	key := pocKeys[pkStr(blk.Header.PubKey)]
	sig2, err := key.Sign(wire.HashB([]byte("MASS TESTNET")))
	if err != nil {
		panic(err)
	}

	header := &blk.Header
	header.Version = 1
	header.Height = 0
	header.Timestamp = time.Unix(timestamp, 0)
	header.Previous = wire.Hash{}
	header.TransactionRoot = txRoot
	header.Target = target
	header.Signature = sig2

	chainID, err := header.GetChainID()
	if err != nil {
		panic(err)
	}
	header.ChainID = chainID

	return
}

func genesisToString(genesis *wire.MsgBlock) string {
	var template = `
// genesisCoinbaseTx is the coinbase transaction for genesis block
var genesisCoinbaseTx = wire.MsgTx{
	Version: %d,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  mustDecodeHash("%s"),
				Index: 0x%x,
			},
			Sequence: 0x%x,
			Witness:  wire.TxWitness{mustDecodeString("%s")},
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value:    0x%x,
			PkScript: mustDecodeString("%s"),
		},
	},
	LockTime: %d,
	Payload:  mustDecodeString("%s"),
}

var genesisHeader = wire.BlockHeader{
	ChainID:         mustDecodeHash("%s"),
	Version:         %d,
	Height:          %d,
	Timestamp:       time.Unix(0x%x, 0), // %s Beijing, %d
	Previous:        mustDecodeHash("%s"),
	TransactionRoot: mustDecodeHash("%s"),
    WitnessRoot:     mustDecodeHash("%s"),
	ProposalRoot:    mustDecodeHash("%s"),
	Target:          hexToBigInt("%s"),
	Challenge:       mustDecodeHash("%s"),
	PubKey:          mustDecodePoCPublicKey("%s"),
	Proof: &poc.Proof{
		X:         mustDecodeString("%s"),
		XPrime:    mustDecodeString("%s"),
		BitLength: %d,
	},
	Signature: mustDecodePoCSignature("%s"),
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

var genesisHash = mustDecodeHash("%s")

var genesisChainID = mustDecodeHash("%s")
`

	header := genesis.Header
	txs := genesis.Transactions
	str := fmt.Sprintf(template,
		// Coinbase tx
		// Tx Version
		txs[0].Version,
		// TxIn
		txs[0].TxIn[0].PreviousOutPoint.Hash.String(),
		txs[0].TxIn[0].PreviousOutPoint.Index,
		txs[0].TxIn[0].Sequence,
		//hex.EncodeToString(txs[0].TxIn[0].Witness[0]),
		// TxOut
		txs[0].TxOut[0].Value,
		hex.EncodeToString(txs[0].TxOut[0].PkScript),
		// Tx LockTime
		txs[0].LockTime,
		// Tx Payload
		hex.EncodeToString(txs[0].Payload),

		// Header
		header.ChainID.String(),
		header.Version,
		header.Height,
		// Timestamp
		header.Timestamp.Unix(),
		header.Timestamp.String(),
		header.Timestamp.Unix(),
		header.Previous.String(),
		header.TransactionRoot.String(),
		header.WitnessRoot.String(),
		header.ProposalRoot.String(),
		hex.EncodeToString(header.Target.Bytes()),
		header.Challenge.String(),
		// PubKey
		hex.EncodeToString(header.PubKey.SerializeCompressed()),
		// Proof
		hex.EncodeToString(header.Proof.X),
		hex.EncodeToString(header.Proof.XPrime),
		header.Proof.BitLength,
		// Signature
		hex.EncodeToString(header.Signature.Serialize()),

		// Block Hash
		header.BlockHash().String(),

		// ChainID
		header.ChainID.String(),
	)

	return str
}
