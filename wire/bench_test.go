package wire

import (
	"bytes"
	"io/ioutil"
	"testing"
	"time"

	"massnet.org/mass/poc"
	"massnet.org/mass/pocec"
)

var tstGenesisCoinbaseTx = MsgTx{
	Version: 1,
	TxIn: []*TxIn{
		{
			PreviousOutPoint: OutPoint{
				Hash:  Hash{},
				Index: MaxPrevOutIndex,
			},
			Sequence: MaxTxInSequenceNum,
			Witness:  TxWitness{},
		},
	},
	TxOut: []*TxOut{
		{
			Value:    0x47868c000,
			PkScript: mustDecodeString("0020ba60494593fe65bea35fe9e118c129e5478ce660cec07c8ea8a7e2ec841fccd2"),
		},
	},
	LockTime: 0,
	Payload:  mustDecodeString("000000000000000000000000"),
}

var tstGenesisHeader = BlockHeader{
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

var tstGenesisBlock = MsgBlock{
	Header: tstGenesisHeader,
	Proposals: ProposalArea{
		PunishmentArea: make([]*FaultPubKey, 0),
		OtherArea:      make([]*NormalProposal, 0),
	},
	Transactions: []*MsgTx{&tstGenesisCoinbaseTx},
}

var tstGenesisHash = mustDecodeHash("ee26300e0f068114a680a772e080507c0f9c0ca4335c382c42b78e2eafbebaa3")

var tstGenesisChainID = mustDecodeHash("5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7")

func BenchmarkEncodeBlock(b *testing.B) {
	b.StopTimer()

	blk := mockBlock(3000)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := blk.Encode(bytes.NewBuffer([]byte("")), DB)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncodeBlock(t *testing.T) {
	blk := mockBlock(3000)

	var N = 1000
	start := time.Now()
	for i := 0; i < N; i++ {
		n, err := blk.Encode(bytes.NewBuffer([]byte("")), DB)
		if err != nil {
			t.Fatal(i, err, n)
		}
	}
	t.Log(time.Since(start))
}

func BenchmarkDecodeBlock(b *testing.B) {
	b.StopTimer()

	blk := mockBlock(2000)
	var wBuf bytes.Buffer
	_, err := blk.Encode(&wBuf, DB)
	if err != nil {
		b.Fatal(err)
	}
	buf := wBuf.Bytes()
	b.Log(len(buf))

	var newBlk = NewEmptyMsgBlock()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		n, err := newBlk.Decode(bytes.NewReader(buf), DB)
		if err != nil {
			b.Fatal(i, err, n)
		}
	}
}

func TestDecodeBlock(t *testing.T) {
	blk := mockBlock(2000)
	var wBuf bytes.Buffer
	_, err := blk.Encode(&wBuf, DB)
	if err != nil {
		t.Fatal(err)
	}
	buf := wBuf.Bytes()
	t.Log(len(buf))

	var newBlk = NewEmptyMsgBlock()
	var N = 1000
	start := time.Now()
	for i := 0; i < N; i++ {
		n, err := newBlk.Decode(bytes.NewReader(buf), DB)
		if err != nil {
			t.Fatal(i, err, n)
		}
	}
	t.Log(time.Since(start))
}

func BenchmarkEncodeTx(b *testing.B) {
	b.StopTimer()

	tx := mockTx()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := tx.Encode(bytes.NewBuffer([]byte("")), DB)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncodeTx(t *testing.T) {
	tx := mockTx()

	var N = 1000
	start := time.Now()
	for i := 0; i < N; i++ {
		_, err := tx.Encode(bytes.NewBuffer([]byte("")), DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log(time.Since(start))
}

func BenchmarkDecodeTx(b *testing.B) {
	b.StopTimer()

	tx := mockTx()
	var wBuf bytes.Buffer
	_, err := tx.Encode(&wBuf, DB)
	if err != nil {
		b.Fatal(err)
	}
	buf := wBuf.Bytes()
	b.Log(len(buf))

	var newTx = NewMsgTx()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		err := newTx.SetBytes(buf, DB)
		if err != nil {
			b.Fatal(err)
		}
	}

}

func TestDecodeTx(t *testing.T) {
	tx := mockTx()
	var wBuf bytes.Buffer
	_, err := tx.Encode(&wBuf, DB)
	if err != nil {
		t.Fatal(err)
	}
	buf := wBuf.Bytes()
	t.Log(len(buf))

	var newTx = NewMsgTx()
	var N = 1000
	start := time.Now()
	for i := 0; i < N; i++ {
		err := newTx.SetBytes(buf, DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log(time.Since(start))
}

// BenchmarkWriteVarInt1 performs a benchmark on how long it takes to write
// a single byte variable length integer.
func BenchmarkWriteVarInt1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WriteUint64(ioutil.Discard, 1)
	}
}

// BenchmarkWriteVarInt3 performs a benchmark on how long it takes to write
// a three byte variable length integer.
func BenchmarkWriteVarInt3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WriteUint64(ioutil.Discard, 65535)
	}
}

// BenchmarkWriteVarInt5 performs a benchmark on how long it takes to write
// a five byte variable length integer.
func BenchmarkWriteVarInt5(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WriteUint64(ioutil.Discard, 4294967295)
	}
}

// BenchmarkWriteVarInt9 performs a benchmark on how long it takes to write
// a nine byte variable length integer.
func BenchmarkWriteVarInt9(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WriteUint64(ioutil.Discard, 18446744073709551615)
	}
}

// BenchmarkReadVarInt1 performs a benchmark on how long it takes to read
// a single byte variable length integer.
func BenchmarkReadVarInt1(b *testing.B) {
	buf := []byte{0x01}
	for i := 0; i < b.N; i++ {
		ReadUint64(bytes.NewReader(buf), 0)
	}
}

// BenchmarkReadVarInt3 performs a benchmark on how long it takes to read
// a three byte variable length integer.
func BenchmarkReadVarInt3(b *testing.B) {
	buf := []byte{0x0fd, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		ReadUint64(bytes.NewReader(buf), 0)
	}
}

// BenchmarkReadVarInt5 performs a benchmark on how long it takes to read
// a five byte variable length integer.
func BenchmarkReadVarInt5(b *testing.B) {
	buf := []byte{0xfe, 0xff, 0xff, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		ReadUint64(bytes.NewReader(buf), 0)
	}
}

// BenchmarkReadVarInt9 performs a benchmark on how long it takes to read
// a nine byte variable length integer.
func BenchmarkReadVarInt9(b *testing.B) {
	buf := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		ReadUint64(bytes.NewReader(buf), 0)
	}
}

// BenchmarkWriteTxOut performs a benchmark on how long it takes to write
// a transaction output.
func BenchmarkWriteTxOut(b *testing.B) {
	txOut := tstGenesisBlock.Transactions[0].TxOut[0]
	for i := 0; i < b.N; i++ {
		WriteTxOut(ioutil.Discard, txOut)
	}
}

// BenchmarkDeserializeTx performs a benchmark on how long it takes to
// deserialize a transaction.
func BenchmarkDeserializeTx(b *testing.B) {
	buf, err := mockTx().Bytes(DB)
	if err != nil {
		b.Fatal(err)
	}
	var tx = new(MsgTx)
	for i := 0; i < b.N; i++ {
		tx.SetBytes(buf, DB)

	}
}

// BenchmarkSerializeTx performs a benchmark on how long it takes to serialize
// a transaction.
func BenchmarkSerializeTx(b *testing.B) {
	tx := tstGenesisBlock.Transactions[0]
	for i := 0; i < b.N; i++ {
		tx.Encode(ioutil.Discard, DB)

	}
}

// BenchmarkDecodeBlockHeader performs a benchmark on how long it takes to
// deserialize a block header.
func BenchmarkDecodeBlockHeader(b *testing.B) {
	buf, err := mockHeader().Bytes(DB)
	if err != nil {
		b.Fatal(err)
	}
	var header = NewEmptyBlockHeader()
	for i := 0; i < b.N; i++ {
		header.Decode(bytes.NewReader(buf), DB)
	}
}

// BenchmarkWriteBlockHeader performs a benchmark on how long it takes to
// serialize a block header.
func BenchmarkWriteBlockHeader(b *testing.B) {
	header := tstGenesisBlock.Header
	for i := 0; i < b.N; i++ {
		header.Encode(ioutil.Discard, DB)
	}
}

// BenchmarkTxSha performs a benchmark on how long it takes to hash a
// transaction.
func BenchmarkTxSha(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tstGenesisCoinbaseTx.TxHash()
	}
}

// BenchmarkDoubleSha256 performs a benchmark on how long it takes to perform a
// double sha 256 returning a byte slice.
func BenchmarkDoubleSha256(b *testing.B) {
	b.StopTimer()
	var buf bytes.Buffer
	if _, err := tstGenesisCoinbaseTx.Encode(&buf, ID); err != nil {
		b.Errorf("Serialize: unexpected error: %v", err)
		return
	}
	txBytes := buf.Bytes()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		_ = DoubleHashB(txBytes)
	}
}

// BenchmarkDoubleSha256SH performs a benchmark on how long it takes to perform
// a double sha 256 returning a Hash.
func BenchmarkDoubleSha256SH(b *testing.B) {
	b.StopTimer()
	var buf bytes.Buffer
	if _, err := tstGenesisCoinbaseTx.Encode(&buf, ID); err != nil {
		b.Errorf("Serialize: unexpected error: %v", err)
		return
	}
	txBytes := buf.Bytes()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		_ = DoubleHashH(txBytes)
	}
}
