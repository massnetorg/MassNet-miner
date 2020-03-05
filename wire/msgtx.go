package wire

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strconv"

	"github.com/golang/protobuf/proto"
	"massnet.org/mass/consensus"
	wirepb "massnet.org/mass/wire/pb"
)

const (
	// TxVersion is the current latest supported transaction version.
	TxVersion = 1

	// MaxTxInSequenceNum is the maximum sequence number the sequence field
	// of a transaction input can be.
	MaxTxInSequenceNum uint64 = math.MaxUint64

	// MaxPrevOutIndex is the maximum index the index field of a previous
	// outpoint can be.
	MaxPrevOutIndex uint32 = math.MaxUint32

	// SequenceLockTimeDisabled is a flag that if set on a transaction
	// input's sequence number, the sequence number will not be interpreted
	// as a relative locktime.
	SequenceLockTimeDisabled uint64 = 1 << 63

	// SequenceLockTimeIsSeconds is a flag thabt if set on a transaction
	// input's sequence number, the relative locktime has units of 32
	// seconds(close to block interval).
	SequenceLockTimeIsSeconds uint64 = 1 << 38

	// SequenceLockTimeMask is a mask that extracts the relative locktime
	// when masked against the transaction input sequence number.
	SequenceLockTimeMask uint64 = 0x00000000ffffffff

	// SequenceLockTimeGranularity is the defined time based granularity
	// for seconds-based relative time locks. When converting from seconds
	// to a sequence number, the value is right shifted by this amount,
	// therefore the granularity of relative time locks in 32 or 2^5
	// seconds. Enforced relative lock times are multiples of 32 seconds.
	SequenceLockTimeGranularity = 5
)

// defaultTxInOutAlloc is the default size used for the backing array for
// transaction inputs and outputs.  The array will dynamically grow as needed,
// but this figure is intended to provide enough space for the number of
// inputs and outputs in a typical transaction without needing to grow the
// backing array multiple times.
const defaultTxInOutAlloc = 15

const (
	// minTxPayload is the minimum payload size for a transaction.  Note
	// that any realistically usable transaction must have at least one
	// input or output, but that is a rule enforced at a higher layer, so
	// it is intentionally not included here.
	// Version 4 bytes + LockTime 8 bytes + min input payload + min output payload.
	minTxPayload = 12
)

// zeroHash is the zero value for a wire.Hash and is defined as
// a package level variable to avoid the need to create a new instance
// every time a check is needed.
var zeroHash = &Hash{}

// OutPoint defines a mass data type that is used to track previous
// transaction outputs.

// OutPoint defines a mass data type that is used to track previous
// transaction outputs.
type OutPoint struct {
	Hash  Hash
	Index uint32
}

// NewOutPoint returns a new mass transaction outpoint point with the
// provided hash and index.
func NewOutPoint(hash *Hash, index uint32) *OutPoint {
	return &OutPoint{
		Hash:  *hash,
		Index: index,
	}
}

// String returns the OutPoint in the human-readable form "hash:index".
func (o OutPoint) String() string {
	// Allocate enough for hash string, colon, and 10 digits.  Although
	// at the time of writing, the number of digits can be no greater than
	// the length of the decimal representation of maxTxOutPerMessage, the
	// maximum message payload may increase in the future and this
	// optimization may go unnoticed, so allocate space for 10 decimal
	// digits, which will fit any uint32.
	buf := make([]byte, 2*HashSize+1, 2*HashSize+1+10)
	copy(buf, o.Hash.String())
	buf[2*HashSize] = ':'
	buf = strconv.AppendUint(buf, uint64(o.Index), 10)
	return string(buf)
}

// TxIn defines a mass transaction input.
type TxIn struct {
	PreviousOutPoint OutPoint
	Witness          TxWitness
	Sequence         uint64
}

// PlainSize returns the number of bytes it would take to serialize the
// the transaction input.
func (t *TxIn) PlainSize() int {
	var n = 0
	n += 4 + 32                // OutPoint
	n += t.Witness.PlainSize() // Witness
	n += 8                     // Sequence
	return n
}

// NewTxIn returns a new mass transaction input with the provided
// previous outpoint point and signature script with a default sequence of
// MaxTxInSequenceNum.
func NewTxIn(prevOut *OutPoint, witness [][]byte) *TxIn {
	return &TxIn{
		PreviousOutPoint: *prevOut,
		Witness:          witness,
		Sequence:         MaxTxInSequenceNum,
	}
}

// TxWitness defines the witness for a TxIn. A witness is to be interpreted as
// a slice of byte slices, or a stack with one or many elements.
type TxWitness [][]byte

// PlainSize returns the number of bytes it would take to serialize the the
// transaction input's witness.
func (t TxWitness) PlainSize() int {
	var n = 0

	for _, witItem := range t {
		n += len(witItem)
	}

	return n
}

// TxOut defines a mass transaction output.
type TxOut struct {
	Value    int64
	PkScript []byte
}

// PlainSize returns the number of bytes it would take to serialize the
// the transaction output.
func (t *TxOut) PlainSize() int {
	// Value 8 bytes + serialized varint size for the length of PkScript +
	// PkScript bytes.
	return 8 + len(t.PkScript)
}

// NewTxOut returns a new mass transaction output with the provided
// transaction value and public key script.
func NewTxOut(value int64, pkScript []byte) *TxOut {
	return &TxOut{
		Value:    value,
		PkScript: pkScript,
	}
}

// MsgTx implements the Message interface and represents a mass tx message.
// It is used to deliver transaction information in response to a getdata
// message (MsgGetData) for a given transaction.
//
// Use the AddTxIn and AddTxOut functions to build up the list of transaction
// inputs and outputs.
type MsgTx struct {
	Version  uint32
	TxIn     []*TxIn
	TxOut    []*TxOut
	LockTime uint64
	Payload  []byte
}

// AddTxIn adds a transaction input to the message.
func (msg *MsgTx) AddTxIn(ti *TxIn) {
	msg.TxIn = append(msg.TxIn, ti)
}

// AddTxOut adds a transaction output to the message.
func (msg *MsgTx) AddTxOut(to *TxOut) {
	msg.TxOut = append(msg.TxOut, to)
}

func (msg *MsgTx) RemoveTxOut(index int) {
	msg.TxOut = append(msg.TxOut[:index], msg.TxOut[index+1:]...)
}

func (msg *MsgTx) RemoveAllTxOut() {
	msg.TxOut = make([]*TxOut, 0, defaultTxInOutAlloc)
}

func (msg *MsgTx) SetPayload(payload []byte) {
	msg.Payload = make([]byte, len(payload))
	copy(msg.Payload, payload)
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBase in that it works with a raw wire
// transaction as opposed to a higher level util transaction.
func (msg *MsgTx) IsCoinBaseTx() bool {
	// A coin base must only have at least one transaction input.
	if len(msg.TxIn) == 0 {
		return false
	}

	// The previous output of a coin base must have a max value index and
	// a zero hash.
	prevOut := &msg.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || !prevOut.Hash.IsEqual(zeroHash) {
		return false
	}

	return true
}

// TxHash generates the Hash for the transaction.
func (msg *MsgTx) TxHash() Hash {
	buf, err := msg.Bytes(ID)
	if err != nil {
		return Hash{}
	}
	return DoubleHashH(buf)
}

// WitnessHash generates the hash of the transaction serialized according to
// the new witness serialization defined in BIP0141 and BIP0144. The final
// output is used within the Segregated Witness commitment of all the witnesses
// within a block. If a transaction has no witness data, then the witness hash,
// is the same as its txid.
func (msg *MsgTx) WitnessHash() Hash {
	var buf bytes.Buffer
	_, err := msg.Encode(&buf, WitnessID)
	if err != nil {
		return Hash{}
	}
	return DoubleHashH(buf.Bytes())
}

func (msg *MsgTx) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	var buf bytes.Buffer
	n64, err := buf.ReadFrom(r)
	n = int(n64)
	if err != nil {
		return n, err
	}

	switch mode {
	case DB, Packet:
		pb := new(wirepb.Tx)
		if err = proto.Unmarshal(buf.Bytes(), pb); err != nil {
			return n, err
		}
		if err = msg.FromProto(pb); err != nil {
			return n, err
		}
		return n, nil

	default:
		return n, ErrInvalidCodecMode
	}

}

func (msg *MsgTx) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	pb := msg.ToProto()

	switch mode {
	case DB, Packet:
		content, err := proto.Marshal(pb)
		if err != nil {
			return n, err
		}
		return w.Write(content)

	case Plain, WitnessID:
		// Write all elements of a transaction
		return pb.Write(w)

	case ID:
		return pb.WriteNoWitness(w)

	default:
		return n, ErrInvalidCodecMode
	}

}

func (msg *MsgTx) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(msg, mode)
}

func (msg *MsgTx) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(msg, bs, mode)
}

// PlainSize returns the number of bytes the transaction contains.
func (msg *MsgTx) PlainSize() int {
	var n = 0

	// Version
	n += 4

	// TxIns
	for _, ti := range msg.TxIn {
		n += ti.PlainSize()
	}

	// TxOuts
	for _, to := range msg.TxOut {
		n += to.PlainSize()
	}

	// LockTime
	n += 8

	//Payload
	n += len(msg.Payload)

	return n
}

// NewMsgTx returns a new mass tx message that conforms to the Message
// interface.  The return instance has a default version of TxVersion and there
// are no transaction inputs or outputs.  Also, the lock time is set to zero
// to indicate the transaction is valid immediately as opposed to some time in
// future.
func NewMsgTx() *MsgTx {
	return &MsgTx{
		Version: TxVersion,
		TxIn:    make([]*TxIn, 0, defaultTxInOutAlloc),
		TxOut:   make([]*TxOut, 0, defaultTxInOutAlloc),
		Payload: make([]byte, 0, 8),
	}
}

// WriteTxOut encodes to into the mass protocol encoding for a transaction
// output (TxOut) to w.
func WriteTxOut(w io.Writer, to *TxOut) error {
	_, err := WriteUint64(w, uint64(to.Value))
	if err != nil {
		return err
	}

	return WriteVarBytes(w, to.PkScript)
}

// ToProto get proto Tx from wire Tx
func (msg *MsgTx) ToProto() *wirepb.Tx {
	txIns := make([]*wirepb.TxIn, len(msg.TxIn))
	for i, ti := range msg.TxIn {
		witness := make([][]byte, len(ti.Witness))
		for j, data := range ti.Witness {
			witness[j] = data
		}
		txIn := &wirepb.TxIn{
			PreviousOutPoint: &wirepb.OutPoint{
				Hash:  ti.PreviousOutPoint.Hash.ToProto(),
				Index: ti.PreviousOutPoint.Index,
			},
			Witness:  witness,
			Sequence: ti.Sequence,
		}
		txIns[i] = txIn
	}

	txOuts := make([]*wirepb.TxOut, len(msg.TxOut))
	for i, to := range msg.TxOut {
		txOut := &wirepb.TxOut{
			Value:    to.Value,
			PkScript: to.PkScript,
		}
		txOuts[i] = txOut
	}

	return &wirepb.Tx{
		Version:  msg.Version,
		TxIn:     txIns,
		TxOut:    txOuts,
		LockTime: msg.LockTime,
		Payload:  msg.Payload,
	}
}

// FromProto load proto TX into wire Tx
func (msg *MsgTx) FromProto(pb *wirepb.Tx) error {
	if pb == nil {
		return errors.New("nil proto tx")
	}
	txIns := make([]*TxIn, len(pb.TxIn))
	for i, ti := range pb.TxIn {
		if ti == nil {
			return errors.New("nil proto tx_in")
		}
		if ti.PreviousOutPoint == nil {
			return errors.New("nil proto out_point")
		}
		witness := make([][]byte, len(ti.Witness))
		for j, data := range ti.Witness {
			witness[j] = MoveBytes(data)
		}
		opHash := new(Hash)
		if err := opHash.FromProto(ti.PreviousOutPoint.Hash); err != nil {
			return err
		}
		txIn := &TxIn{
			PreviousOutPoint: OutPoint{
				Hash:  *opHash,
				Index: ti.PreviousOutPoint.Index,
			},
			Witness:  witness,
			Sequence: ti.Sequence,
		}
		txIns[i] = txIn
	}

	txOuts := make([]*TxOut, len(pb.TxOut))
	for i, to := range pb.TxOut {
		if to == nil {
			return errors.New("nil proto tx_out")
		}
		txOut := &TxOut{
			Value:    to.Value,
			PkScript: MoveBytes(to.PkScript),
		}
		txOuts[i] = txOut
	}

	msg.Version = pb.Version
	msg.TxIn = txIns
	msg.TxOut = txOuts
	msg.LockTime = pb.LockTime
	msg.Payload = MoveBytes(pb.Payload)
	return nil
}

// NewTxFromProto get wire Tx from proto Tx
func NewTxFromProto(pb *wirepb.Tx) (*MsgTx, error) {
	msg := new(MsgTx)
	if err := msg.FromProto(pb); err != nil {
		return nil, err
	}
	return msg, nil
}

func IsValidFrozenPeriod(height uint64) bool {
	return height >= consensus.MinFrozenPeriod && height <= SequenceLockTimeMask-1
}

func IsValidStakingValue(value int64) bool {
	return value >= int64(consensus.MinStakingValue)
}
