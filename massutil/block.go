package massutil

import (
	"bytes"
	"fmt"
	"io"

	"massnet.org/mass/wire"
)

// OutOfRangeError describes an error due to accessing an element that is out
// of range.
type OutOfRangeError string

// Error satisfies the error interface and prints human-readable errors.
func (e OutOfRangeError) Error() string {
	return string(e)
}

type Block struct {
	msgBlock              *wire.MsgBlock // Underlying MsgBlock
	serializedBlockDB     []byte         // Serialized bytes for the block in db format
	serializedBlockPacket []byte         // Serialized bytes for the block in packet format
	blockHash             *wire.Hash     // Cached block hash
	blockHeight           uint64         // Height in the main block chain
	transactions          []*Tx          // Transactions
	txsGenerated          bool           // ALL wrapped transactions generated
	size                  uint64         // Plain block size
	txLocs                []wire.TxLoc
}

// MsgBlock returns the underlying wire.MsgBlock for the Block.
func (b *Block) MsgBlock() *wire.MsgBlock {
	return b.msgBlock
}

// Bytes returns the serialized bytes for the Block.  This is equivalent to
// calling Serialize on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (b *Block) Bytes(mode wire.CodecMode) ([]byte, error) {
	// Return the cached serialized bytes if it has already been generated.
	switch mode {
	case wire.DB:
		if len(b.serializedBlockDB) != 0 {
			return b.serializedBlockDB, nil
		}
	case wire.Packet:
		if len(b.serializedBlockPacket) != 0 {
			return b.serializedBlockPacket, nil
		}
	}

	// Serialize the MsgBlock.
	var w bytes.Buffer
	_, err := b.msgBlock.Encode(&w, mode)
	if err != nil {
		return nil, err
	}
	serializedBlock := w.Bytes()

	// Cache the serialized bytes and return them.
	switch mode {
	case wire.DB:
		b.serializedBlockDB = serializedBlock
	case wire.Packet:
		b.serializedBlockPacket = serializedBlock
	}

	return serializedBlock, nil
}

// Sha returns the block identifier hash for the Block.  This is equivalent to
// calling BlockHash on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (b *Block) Hash() *wire.Hash {
	// Return the cached block hash if it has already been generated.
	if b.blockHash != nil {
		return b.blockHash
	}

	// Cache the block hash and return it.
	hash := b.msgBlock.BlockHash()
	b.blockHash = &hash
	return &hash
}

func (b *Block) Size() uint64 {
	if b.size != 0 {
		return b.size
	}

	b.size = uint64(b.msgBlock.PlainSize())
	return b.size
}

func (b *Block) PacketSize() int {
	data, _ := b.Bytes(wire.Packet)
	return len(data)
}

// Tx returns a wrapped transaction (massutil.Tx) for the transaction at the
// specified index in the Block.  The supplied index is 0 based.  That is to
// say, the first transaction in the block is txNum 0.  This is nearly
// equivalent to accessing the raw transaction (wire.MsgTx) from the
// underlying wire.MsgBlock, however the wrapped transaction has some helpful
// properties such as caching the hash so subsequent calls are more efficient.
func (b *Block) Tx(txNum int) (*Tx, error) {
	// Ensure the requested transaction is in range.
	numTx := uint64(len(b.msgBlock.Transactions))
	if txNum < 0 || uint64(txNum) > numTx {
		str := fmt.Sprintf("transaction index %d is out of range - max %d",
			txNum, numTx-1)
		return nil, OutOfRangeError(str)
	}

	// Generate slice to hold all of the wrapped transactions if needed.
	if len(b.transactions) == 0 {
		b.transactions = make([]*Tx, numTx)
	}

	// Return the wrapped transaction if it has already been generated.
	if b.transactions[txNum] != nil {
		return b.transactions[txNum], nil
	}

	// Generate and cache the wrapped transaction and return it.
	newTx := NewTx(b.msgBlock.Transactions[txNum])
	newTx.SetIndex(txNum)
	b.transactions[txNum] = newTx
	return newTx, nil
}

// Transactions returns a slice of wrapped transactions (massutil.Tx) for all
// transactions in the Block.  This is nearly equivalent to accessing the raw
// transactions (wire.MsgTx) in the underlying wire.MsgBlock, however it
// instead provides easy access to wrapped versions (massutil.Tx) of them.
func (b *Block) Transactions() []*Tx {
	// Return transactions if they have ALL already been generated.  This
	// flag is necessary because the wrapped transactions are lazily
	// generated in a sparse fashion.
	if b.txsGenerated {
		return b.transactions
	}

	// Generate slice to hold all of the wrapped transactions if needed.
	if len(b.transactions) == 0 {
		b.transactions = make([]*Tx, len(b.msgBlock.Transactions))
	}

	// Generate and cache the wrapped transactions for all that haven't
	// already been done.
	for i, tx := range b.transactions {
		if tx == nil {
			newTx := NewTx(b.msgBlock.Transactions[i])
			newTx.SetIndex(i)
			b.transactions[i] = newTx
		}
	}

	b.txsGenerated = true
	return b.transactions
}

func (b *Block) TxHash(txNum int) (*wire.Hash, error) {
	tx, err := b.Tx(txNum)
	if err != nil {
		return nil, err
	}

	return tx.Hash(), nil
}

// TxLoc returns the offsets and lengths of each transaction in a raw block.
// It is used to allow fast indexing into transactions within the raw byte
// stream.
func (b *Block) TxLoc() ([]wire.TxLoc, error) {
	if b.txLocs != nil {
		return b.txLocs, nil
	}
	rawMsg, err := b.Bytes(wire.DB)
	if err != nil {
		return nil, err
	}
	rBuf := bytes.NewBuffer(rawMsg)

	var mBlock = wire.NewEmptyMsgBlock()
	b.txLocs, err = mBlock.DeserializeTxLoc(rBuf)
	if err != nil {
		return nil, err
	}
	return b.txLocs, err
}

func (b *Block) SetSerializedBlockDB(data []byte) {
	b.serializedBlockDB = data
}

func (b *Block) ResetGenerated() {
	b.txsGenerated = false
	b.transactions = nil
}

// Height returns the saved height of the block in the block chain.  This value
// will be BlockHeightUnknown if it hasn't already explicitly been set.
func (b *Block) Height() uint64 {
	return b.blockHeight
}

// SetHeight sets the height of the block in the block chain.
func (b *Block) SetHeight(height uint64) {
	b.blockHeight = height
}

// NewBlock returns a new instance of a Mass block given an underlying
// wire.MsgBlock.  See Block.
func NewBlock(msgBlock *wire.MsgBlock) *Block {
	return &Block{
		msgBlock:    msgBlock,
		blockHeight: msgBlock.Header.Height,
	}
}

// NewBlockFromBytes returns a new instance of a Mass block given the
// serialized bytes.  See Block.
func NewBlockFromBytes(serializedBlock []byte, mode wire.CodecMode) (*Block, error) {
	br := bytes.NewReader(serializedBlock)
	b, err := NewBlockFromReader(br, mode)
	if err != nil {
		return nil, err
	}
	switch mode {
	case wire.DB:
		b.serializedBlockDB = serializedBlock
	case wire.Packet:
		b.serializedBlockPacket = serializedBlock
	}

	return b, nil
}

// NewBlockFromReader returns a new instance of a Mass block given a
// Reader to deserialize the block.  See Block.
func NewBlockFromReader(r io.Reader, mode wire.CodecMode) (*Block, error) {
	// Deserialize the bytes into a MsgBlock.
	var msgBlock = wire.NewEmptyMsgBlock()
	_, err := msgBlock.Decode(r, mode)
	if err != nil {
		return nil, err
	}

	b := Block{
		msgBlock:    msgBlock,
		blockHeight: msgBlock.Header.Height,
	}
	return &b, nil
}

// NewBlockFromBlockAndBytes returns a new instance of a Mass block given
// an underlying wire.MsgBlock and the serialized bytes for it.  See Block.
func NewBlockFromBlockAndBytes(msgBlock *wire.MsgBlock, serializedBlock []byte, mode wire.CodecMode) *Block {
	blk := &Block{
		msgBlock:    msgBlock,
		blockHeight: msgBlock.Header.Height,
	}
	switch mode {
	case wire.DB:
		blk.serializedBlockDB = serializedBlock
	case wire.Packet:
		blk.serializedBlockPacket = serializedBlock
	}
	return blk
}
