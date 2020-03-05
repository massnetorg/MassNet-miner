package wire

import (
	"bytes"
	"errors"
	"io"

	"github.com/golang/protobuf/proto"
	"massnet.org/mass/logging"
	wirepb "massnet.org/mass/wire/pb"
)

// defaultTransactionAlloc is the default size used for the backing array
// for transactions.  The transaction array will dynamically grow as needed, but
// this figure is intended to provide enough space for the number of
// transactions in the vast majority of blocks without needing to grow the
// backing array multiple times.
const defaultTransactionAlloc = 2048

// MaxBlockPayload is the maximum bytes a block message can be in bytes.
// 2MB.
const MaxBlockPayload = 2000000

// maxTxPerBlock is the maximum number of transactions that could
// possibly fit into a block.
const MaxTxPerBlock = (MaxBlockPayload / minTxPayload) + 1

// TxLoc holds locator data for the offset and length of where a transaction is
// located within a MsgBlock data buffer.
type TxLoc struct {
	TxStart int
	TxLen   int
}

// MsgBlock implements the Message interface and represents a mass
// block message.  It is used to deliver block and transaction information in
// response to a getdata message (MsgGetData) for a given block hash.
type MsgBlock struct {
	Header       BlockHeader
	Proposals    ProposalArea
	Transactions []*MsgTx
}

func NewEmptyMsgBlock() *MsgBlock {
	return &MsgBlock{
		Header:       *NewEmptyBlockHeader(),
		Proposals:    *newEmptyProposalArea(),
		Transactions: make([]*MsgTx, 0),
	}
}

// AddTransaction adds a transaction to the message.
func (msg *MsgBlock) AddTransaction(tx *MsgTx) {
	msg.Transactions = append(msg.Transactions, tx)
	return
}

// ClearTransactions removes all transactions from the message.
func (msg *MsgBlock) ClearTransactions() {
	msg.Transactions = make([]*MsgTx, 0, defaultTransactionAlloc)
}

// Decode decodes r using the given protocol encoding into the receiver.
func (msg *MsgBlock) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	switch mode {
	case DB:
		// Read BLockBase length
		blockBaseLength, n64, err := ReadUint64(r, 0)
		n += int(n64)
		if err != nil {
			return n, err
		}

		// Read BlockBase
		baseData := make([]byte, blockBaseLength)
		nn, err := r.Read(baseData)
		n += nn
		if err != nil {
			return n, err
		}
		basePb := new(wirepb.BlockBase)
		err = proto.Unmarshal(baseData, basePb)
		if err != nil {
			return n, err
		}
		base, err := NewBlockBaseFromProto(basePb)
		if err != nil {
			return n, err
		}

		// Read txCount
		txCount, n64, err := ReadUint64(r, 0)
		n += int(n64)
		if err != nil {
			return n, err
		}

		// Read Transactions
		txs := make([]*MsgTx, txCount)
		for i := 0; i < len(txs); i++ {
			// Read tx length
			txLen, n64, err := ReadUint64(r, 0)
			n += int(n64)
			if err != nil {
				return n, err
			}
			// Read tx
			tx := new(MsgTx)
			buf := make([]byte, txLen)
			nn, err = r.Read(buf)
			n += nn
			if err != nil {
				return n, err
			}
			err = tx.SetBytes(buf, DB)
			if err != nil {
				return n, err
			}
			txs[i] = tx
		}

		// fill element
		msg.Header = base.Header
		msg.Proposals = base.Proposals
		msg.Transactions = txs

	case Packet:
		var buf bytes.Buffer
		n64, err := buf.ReadFrom(r)
		n = int(n64)
		if err != nil {
			return n, err
		}

		pb := new(wirepb.Block)
		err = proto.Unmarshal(buf.Bytes(), pb)
		if err != nil {
			return n, err
		}

		err = msg.FromProto(pb)
		if err != nil {
			return n, err
		}

	default:
		return n, ErrInvalidCodecMode
	}

	// Prevent more transactions than could possibly fit into a block.
	txCount := len(msg.Transactions)
	if txCount > MaxTxPerBlock {
		logging.CPrint(logging.ERROR, "too many transactions to fit into a block on decode",
			logging.LogFormat{"count": txCount, "max": MaxTxPerBlock, "hash": msg.BlockHash()})
		return n, errTooManyTxsInBlock
	}

	return n, nil
}

func (msg *MsgBlock) DeserializeTxLoc(r *bytes.Buffer) ([]TxLoc, error) {
	fullLen := r.Len()

	// Read BLockBase length
	blockBaseLength, _, err := ReadUint64(r, 0)
	if err != nil {
		return nil, err
	}

	// Read blockBase
	baseData := make([]byte, blockBaseLength)
	if n, err := r.Read(baseData); uint64(n) != blockBaseLength || err != nil {
		return nil, err
	}

	// Read txCount
	txCount, _, err := ReadUint64(r, 0)
	if err != nil {
		return nil, err
	}

	// Prevent more transactions than could possibly fit into a block.
	if txCount > MaxTxPerBlock {
		logging.CPrint(logging.ERROR, "too many transactions to fit into a block on DeserializeTxLoc",
			logging.LogFormat{"count": txCount, "max": MaxTxPerBlock, "hash": msg.BlockHash()})
		return nil, errTooManyTxsInBlock
	}

	// Deserialize each transaction while keeping track of its location
	// within the byte stream.
	msg.Transactions = make([]*MsgTx, 0, txCount)
	txLocs := make([]TxLoc, txCount)
	for i := uint64(0); i < txCount; i++ {
		// Read tx length
		txLen, _, err := ReadUint64(r, 0)
		if err != nil {
			return nil, err
		}
		// Set txLoc
		txLocs[i].TxStart = fullLen - r.Len()
		txLocs[i].TxLen = int(txLen)
		// Read tx
		tx := new(MsgTx)
		buf := make([]byte, txLen)
		_, err = r.Read(buf)
		if err != nil {
			return nil, err
		}
		err = tx.SetBytes(buf, DB)
		if err != nil {
			return nil, err
		}
		msg.Transactions = append(msg.Transactions, tx)
	}

	return txLocs, nil
}

// Encode encodes the receiver to w using the given protocol encoding.
func (msg *MsgBlock) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	switch mode {
	case DB:
		base := BlockBase{Header: msg.Header, Proposals: msg.Proposals}
		pb, err := base.ToProto()
		if err != nil {
			return 0, err
		}
		baseData, err := proto.Marshal(pb)
		if err != nil {
			return 0, err
		}

		// Write BlockBase length & data
		n, err = WriteUint64(w, uint64(len(baseData)))
		if err != nil {
			return n, err
		}
		nn, err := w.Write(baseData)
		n += nn
		if err != nil {
			return n, err
		}

		// Write txCount
		nn, err = WriteUint64(w, uint64(len(msg.Transactions)))
		n += nn
		if err != nil {
			return n, err
		}

		// Write Transactions
		for i := 0; i < len(msg.Transactions); i++ {
			txPb := msg.Transactions[i].ToProto()
			txData, err := proto.Marshal(txPb)
			if err != nil {
				return n, err
			}
			nn, err = WriteUint64(w, uint64(len(txData)))
			n += nn
			if err != nil {
				return n, err
			}
			nn, err = w.Write(txData)
			n += nn
			if err != nil {
				return n, err
			}
		}
		return n, nil

	case Packet:
		pb, err := msg.ToProto()
		if err != nil {
			return n, err
		}

		buf, err := proto.Marshal(pb)
		if err != nil {
			return n, err
		}

		return w.Write(buf)

	case Plain:
		pb, err := msg.ToProto()
		if err != nil {
			return n, err
		}
		return pb.Write(w)

	default:
		return n, ErrInvalidCodecMode
	}

}

func (msg *MsgBlock) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(msg, mode)
}

func (msg *MsgBlock) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(msg, bs, mode)
}

// PlainSize returns the number of plain bytes the block contains.
func (msg *MsgBlock) PlainSize() int {
	size := getPlainSize(msg)
	if msg.Proposals.PunishmentCount() == 0 {
		size += PlaceHolderSize
	}
	return size
}

// BlockHash computes the block identifier hash for this block.
func (msg *MsgBlock) BlockHash() Hash {
	return msg.Header.BlockHash()
}

// TxHashes returns a slice of hashes of all of transactions in this block.
func (msg *MsgBlock) TxHashes() ([]Hash, error) {
	hashList := make([]Hash, 0, len(msg.Transactions))
	for _, tx := range msg.Transactions {
		hashList = append(hashList, tx.TxHash())
	}
	return hashList, nil
}

// NewMsgBlock returns a new mass block message that conforms to the
// Message interface.  See MsgBlock for details.
func NewMsgBlock(blockHeader *BlockHeader) *MsgBlock {
	return &MsgBlock{
		Header:       *blockHeader,
		Proposals:    *newEmptyProposalArea(),
		Transactions: make([]*MsgTx, 0, defaultTransactionAlloc),
	}
}

// ToProto get proto Block from wire Block
func (msg *MsgBlock) ToProto() (*wirepb.Block, error) {
	pa, err := msg.Proposals.ToProto()
	if err != nil {
		return nil, err
	}
	h := msg.Header.ToProto()
	txs := make([]*wirepb.Tx, len(msg.Transactions))
	for i, tx := range msg.Transactions {
		txs[i] = tx.ToProto()
	}

	return &wirepb.Block{
		Header:       h,
		Proposals:    pa,
		Transactions: txs,
	}, nil
}

// FromProto load proto Block into wire Block,
// if error happens, old content is still immutable
func (msg *MsgBlock) FromProto(pb *wirepb.Block) error {
	if pb == nil {
		return errors.New("nil proto block")
	}
	pa := ProposalArea{}
	err := pa.FromProto(pb.Proposals)
	if err != nil {
		return err
	}
	h := BlockHeader{}
	if err = h.FromProto(pb.Header); err != nil {
		return err
	}
	txs := make([]*MsgTx, len(pb.Transactions))
	for i, v := range pb.Transactions {
		tx := new(MsgTx)
		if err = tx.FromProto(v); err != nil {
			return err
		}
		txs[i] = tx
	}

	msg.Header = h
	msg.Proposals = pa
	msg.Transactions = txs

	return nil
}

// NewBlockFromProto get wire Block from proto Block
func NewBlockFromProto(pb *wirepb.Block) (*MsgBlock, error) {
	block := new(MsgBlock)
	err := block.FromProto(pb)
	if err != nil {
		return nil, err
	}
	return block, nil
}

type BlockBase struct {
	Header    BlockHeader
	Proposals ProposalArea
}

func (base *BlockBase) ToProto() (*wirepb.BlockBase, error) {
	pa, err := base.Proposals.ToProto()
	if err != nil {
		return nil, err
	}
	h := base.Header.ToProto()

	return &wirepb.BlockBase{
		Header:    h,
		Proposals: pa,
	}, nil
}

func (base *BlockBase) FromProto(pb *wirepb.BlockBase) error {
	if pb == nil {
		return errors.New("nil proto block_base")
	}
	pa := ProposalArea{}
	err := pa.FromProto(pb.Proposals)
	if err != nil {
		return err
	}
	h := new(BlockHeader)
	if err := h.FromProto(pb.Header); err != nil {
		return err
	}

	base.Header = *h
	base.Proposals = pa

	return nil
}

func NewBlockBaseFromProto(pb *wirepb.BlockBase) (*BlockBase, error) {
	base := new(BlockBase)
	err := base.FromProto(pb)
	if err != nil {
		return nil, err
	}
	return base, nil
}
