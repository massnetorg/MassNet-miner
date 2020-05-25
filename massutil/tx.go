package massutil

import (
	"bytes"
	"io"

	"massnet.org/mass/wire"
)

// Registered by txscript
var WitPkScriptParseFunc func(script []byte) (class byte, frozen uint64, scriptHash [32]byte)

// TxIndexUnknown is the value returned for a transaction index that is unknown.
// This is typically because the transaction has not been inserted into a block
// yet.
const TxIndexUnknown = -1

type PkScriptInfo struct {
	Class        byte // txscript.ScriptClass
	ScriptHash   [32]byte
	FrozenPeriod uint64
}

type Tx struct {
	msgTx         *wire.MsgTx // Underlying MsgTx
	txHash        *wire.Hash  // Cached transaction hash
	txHashWitness *wire.Hash  // Cached transaction witness hash
	txIndex       int         // Position within a block or TxIndexUnknown

	pkscriptInfos map[int]*PkScriptInfo
}

// MsgTx returns the underlying wire.MsgTx for the transaction.
func (t *Tx) MsgTx() *wire.MsgTx {
	// Return the cached transaction.
	return t.msgTx
}

func (t *Tx) TxOut() []*wire.TxOut {
	return t.MsgTx().TxOut
}

func (t *Tx) GetPkScriptInfo(i int) *PkScriptInfo {
	psi, ok := t.pkscriptInfos[i]
	if !ok {
		class, frozen, sh := WitPkScriptParseFunc(t.MsgTx().TxOut[i].PkScript)
		psi = &PkScriptInfo{
			Class:        class,
			ScriptHash:   sh,
			FrozenPeriod: frozen,
		}
		t.pkscriptInfos[i] = psi
	}
	return psi
}

func (t *Tx) Hash() *wire.Hash {
	// Return the cached hash if it has already been generated.
	if t.txHash != nil {
		return t.txHash
	}

	// Cache the hash and return it.
	hash := t.msgTx.TxHash()
	t.txHash = &hash
	return &hash
}

func (t *Tx) WitnessHash() *wire.Hash {
	// Return the cached hash if it has already been generated.
	if t.txHashWitness != nil {
		return t.txHashWitness
	}

	// Cache the hash and return it.
	hash := t.msgTx.WitnessHash()
	t.txHashWitness = &hash
	return &hash
}

func (t *Tx) Bytes(mode wire.CodecMode) ([]byte, error) {
	return t.msgTx.Bytes(mode)
}

func (t *Tx) PlainSize() int {
	return t.msgTx.PlainSize()
}

func (t *Tx) PacketSize() int {
	data, _ := t.Bytes(wire.Packet)
	return len(data)
}

// Index returns the saved index of the transaction within a block.  This value
// will be TxIndexUnknown if it hasn't already explicitly been set.
func (t *Tx) Index() int {
	return t.txIndex
}

// SetIndex sets the index of the transaction in within a block.
func (t *Tx) SetIndex(index int) {
	t.txIndex = index
}

// NewTx returns a new instance of a Mass transaction given an underlying
// wire.MsgTx.  See Tx.
func NewTx(msgTx *wire.MsgTx) *Tx {
	return &Tx{
		msgTx:         msgTx,
		txIndex:       TxIndexUnknown,
		pkscriptInfos: make(map[int]*PkScriptInfo),
	}
}

// NewTxFromBytes returns a new instance of a Mass transaction given the
// serialized bytes.  See Tx.
func NewTxFromBytes(serializedTx []byte, mode wire.CodecMode) (*Tx, error) {
	br := bytes.NewReader(serializedTx)
	return NewTxFromReader(br, mode)
}

// NewTxFromReader returns a new instance of a Mass transaction given a
// Reader to deserialize the transaction.  See Tx.
func NewTxFromReader(r io.Reader, mode wire.CodecMode) (*Tx, error) {
	// Deserialize the bytes into a MsgTx.
	var msgTx wire.MsgTx
	_, err := msgTx.Decode(r, mode)
	if err != nil {
		return nil, err
	}

	return NewTx(&msgTx), nil
}
