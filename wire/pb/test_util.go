package wirepb

import (
	crand "crypto/rand"
	"math"
	"math/rand"
)

// mockBlock mocks a block with given txCount.
func mockBlock(txCount int) *Block {
	txs := make([]*Tx, txCount)
	for i := 0; i < txCount; i++ {
		txs[i] = mockTx()
	}

	block := &Block{
		Header:       mockHeader(),
		Proposals:    mockProposalArea(),
		Transactions: txs,
	}
	for i := 0; i < len(block.Proposals.Punishments); i++ {
		block.Header.BanList = append(block.Header.BanList, mockPublicKey())
	}

	return block
}

// mockHeader mocks a blockHeader.
func mockHeader() *BlockHeader {
	return &BlockHeader{
		ChainID:         mockHash(),
		Version:         1,
		Height:          rand.Uint64(),
		Timestamp:       rand.Uint64(),
		Previous:        mockHash(),
		TransactionRoot: mockHash(),
		WitnessRoot:     mockHash(),
		ProposalRoot:    mockHash(),
		Target:          mockBigInt(),
		Challenge:       mockHash(),
		PubKey:          mockPublicKey(),
		Proof: &Proof{
			X:         mockLenBytes(3),
			XPrime:    mockLenBytes(3),
			BitLength: (rand.Uint32()%20)*2 + 20,
		},
		Signature: mockSignature(),
		BanList:   make([]*PublicKey, 0),
	}
}

func mockBigInt() *BigInt {
	return &BigInt{
		Raw: mockLenBytes(32),
	}
}

func mockPublicKey() *PublicKey {
	return &PublicKey{
		Raw: mockLenBytes(33),
	}
}

func mockSignature() *Signature {
	return &Signature{
		Raw: mockLenBytes(72),
	}
}

func mockProposalArea() *ProposalArea {
	punishmentCount := rand.Intn(10)
	punishments := make([]*Punishment, punishmentCount)
	for i := range punishments {
		punishments[i] = mockPunishment()
	}

	proposalCount := rand.Intn(5)
	proposals := make([]*Proposal, proposalCount)
	for i := range proposals {
		proposals[i] = mockProposal()
	}

	pa := new(ProposalArea)
	pa.Punishments = punishments
	pa.OtherProposals = proposals

	return pa
}

func mockPunishment() *Punishment {
	return &Punishment{
		Version:    1,
		Type:       0,
		TestimonyA: mockHeader(),
		TestimonyB: mockHeader(),
	}
}

func mockProposal() *Proposal {
	return &Proposal{
		Version: 1,
		Type:    1 + rand.Uint32()%10,
		Content: mockLenBytes(10 + rand.Intn(20)),
	}
}

// mockTx mocks a tx (scripts are random bytes).
func mockTx() *Tx {
	return &Tx{
		Version: 1,
		TxIn: []*TxIn{
			{
				PreviousOutPoint: &OutPoint{
					Hash:  mockHash(),
					Index: 0xffffffff,
				},
				Witness:  [][]byte{mockLenBytes(rand.Intn(50) + 100), mockLenBytes(rand.Intn(50) + 100)},
				Sequence: math.MaxUint64,
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
func mockHash() *Hash {
	pb := new(Hash)
	pb.S0 = rand.Uint64()
	pb.S1 = rand.Uint64()
	pb.S2 = rand.Uint64()
	pb.S3 = rand.Uint64()
	return pb
}

// mockLenBytes mocks bytes with given length.
func mockLenBytes(len int) []byte {
	buf := make([]byte, len)
	crand.Read(buf)
	return buf
}
