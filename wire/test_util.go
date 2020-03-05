package wire

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/big"
	"math/rand"
	"time"

	"massnet.org/mass/poc"
	"massnet.org/mass/pocec"
	wirepb "massnet.org/mass/wire/pb"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func mockBlock(txCount int) *MsgBlock {
	txs := make([]*MsgTx, txCount)
	txs[0] = mockCoinbaseTx()
	for i := 1; i < txCount; i++ {
		txs[i] = mockTx()
	}

	blk := &MsgBlock{
		Header:       *mockHeader(),
		Proposals:    *mockProposalArea(),
		Transactions: txs,
	}

	for _, fpk := range blk.Proposals.PunishmentArea {
		blk.Header.BanList = append(blk.Header.BanList, fpk.PubKey)
	}

	return blk
}

func MockHeader() *BlockHeader {
	return mockHeader()
}

// mockHeader mocks a blockHeader.
func mockHeader() *BlockHeader {
	return &BlockHeader{
		ChainID:         mockHash(),
		Version:         1,
		Height:          rand.Uint64(),
		Timestamp:       time.Unix(rand.Int63(), 0),
		Previous:        mockHash(),
		TransactionRoot: mockHash(),
		WitnessRoot:     mockHash(),
		ProposalRoot:    mockHash(),
		Target:          mockBigInt(),
		Challenge:       mockHash(),
		PubKey:          mockPublicKey(),
		Proof: &poc.Proof{
			X:         mockLenBytes(3),
			XPrime:    mockLenBytes(3),
			BitLength: rand.Intn(20)*2 + 20,
		},
		Signature: mockSignature(),
		BanList:   make([]*pocec.PublicKey, 0),
	}
}

// mockBigInt mocks *big.Int with 32 bytes.
func mockBigInt() *big.Int {
	return new(big.Int).SetBytes(mockLenBytes(32))
}

// mockPublicKey mocks *pocec.PublicKey.
func mockPublicKey() *pocec.PublicKey {
	priv, err := pocec.NewPrivateKey(pocec.S256())
	if err != nil {
		panic(err)
	}
	return priv.PubKey()
}

// mockSignature mocks *pocec.Signature
func mockSignature() *pocec.Signature {
	priv, err := pocec.NewPrivateKey(pocec.S256())
	if err != nil {
		panic(err)
	}
	hash := mockHash()
	sig, err := priv.Sign(hash[:])
	if err != nil {
		panic(err)
	}
	return sig
}

// mockProposalArea mocks ProposalArea.
func mockProposalArea() *ProposalArea {
	punishmentCount := rand.Intn(10)
	punishments := make([]*FaultPubKey, punishmentCount)
	for i := range punishments {
		punishments[i] = mockPunishment()
	}

	proposalCount := rand.Intn(5)
	proposals := make([]*NormalProposal, proposalCount)
	for i := range proposals {
		proposals[i] = mockProposal()
	}

	pa := new(ProposalArea)
	pa.PunishmentArea = punishments
	pa.OtherArea = proposals

	return pa
}

// mockPunishment mocks proposal in punishmentArea.
func mockPunishment() *FaultPubKey {
	fpk := &FaultPubKey{
		PubKey:    mockPublicKey(),
		Testimony: [2]*BlockHeader{mockHeader(), mockHeader()},
	}
	fpk.Testimony[0].PubKey = fpk.PubKey
	fpk.Testimony[1].PubKey = fpk.PubKey

	return fpk
}

// mockProposal mocks normal proposal.
func mockProposal() *NormalProposal {
	length := rand.Intn(30) + 10
	return &NormalProposal{
		version:      ProposalVersion,
		proposalType: typeAnyMessage,
		content:      mockLenBytes(length),
	}
}

// mockTx mocks a tx (scripts are random bytes).
func mockTx() *MsgTx {
	return &MsgTx{
		Version: 1,
		TxIn: []*TxIn{
			{
				PreviousOutPoint: OutPoint{
					Hash:  mockHash(),
					Index: rand.Uint32() % 20,
				},
				Witness:  [][]byte{mockLenBytes(rand.Intn(50) + 100), mockLenBytes(rand.Intn(50) + 100)},
				Sequence: MaxTxInSequenceNum,
			},
		},
		TxOut: []*TxOut{
			{
				Value:    rand.Int63(),
				PkScript: mockLenBytes(rand.Intn(10) + 20),
			},
		},
		LockTime: 0,
		Payload:  mockLenBytes(rand.Intn(20)),
	}
}

// mockCoinbaseTx mocks a coinbase tx.
func mockCoinbaseTx() *MsgTx {
	return &MsgTx{
		Version: 1,
		TxIn: []*TxIn{
			{
				PreviousOutPoint: OutPoint{
					Hash:  Hash{},
					Index: 0xffffffff,
				},
				Witness:  [][]byte{mockLenBytes(rand.Intn(50) + 100), mockLenBytes(rand.Intn(50) + 100)},
				Sequence: MaxTxInSequenceNum,
			},
		},
		TxOut: []*TxOut{
			{
				Value:    rand.Int63(),
				PkScript: mockLenBytes(rand.Intn(10) + 20),
			},
		},
		LockTime: 0,
		Payload:  mockLenBytes(rand.Intn(20)),
	}
}

// mockHash mocks a hash.
func mockHash() Hash {
	pb := new(wirepb.Hash)
	pb.S0 = rand.Uint64()
	pb.S1 = rand.Uint64()
	pb.S2 = rand.Uint64()
	pb.S3 = rand.Uint64()
	hash, _ := NewHashFromProto(pb)
	return *hash
}

// mockLenBytes mocks bytes with given length.
func mockLenBytes(len int) []byte {
	buf := make([]byte, len, len)
	crand.Read(buf)
	return buf
}

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

func mustDecodeHash(str string) Hash {
	h, err := NewHashFromStr(str)
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
