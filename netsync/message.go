package netsync

import (
	"bytes"
	"errors"
	"fmt"

	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
	gowire "github.com/massnetorg/tendermint/go-wire"
)

//protocol msg byte
const (
	BlockchainChannel = byte(0x40)

	BlockRequestByte    = byte(0x10)
	BlockResponseByte   = byte(0x11)
	HeadersRequestByte  = byte(0x12)
	HeadersResponseByte = byte(0x13)
	BlocksRequestByte   = byte(0x14)
	BlocksResponseByte  = byte(0x15)
	HeaderRequestByte   = byte(0x16)
	HeaderResponseByte  = byte(0x17)
	StatusRequestByte   = byte(0x20)
	StatusResponseByte  = byte(0x21)
	NewTransactionByte  = byte(0x30)
	NewMineBlockByte    = byte(0x40)
	FilterLoadByte      = byte(0x50)
	FilterAddByte       = byte(0x51)
	FilterClearByte     = byte(0x52)
	MerkleRequestByte   = byte(0x60)
	MerkleResponseByte  = byte(0x61)

	maxBlockchainResponseSize = 4000000
)

//BlockchainMessage is a generic message for this reactor.
type BlockchainMessage interface{}

var _ = gowire.RegisterInterface(
	struct{ BlockchainMessage }{},
	gowire.ConcreteType{&GetBlockMessage{}, BlockRequestByte},
	gowire.ConcreteType{&BlockMessage{}, BlockResponseByte},
	gowire.ConcreteType{&GetHeadersMessage{}, HeadersRequestByte},
	gowire.ConcreteType{&HeadersMessage{}, HeadersResponseByte},
	gowire.ConcreteType{&GetBlocksMessage{}, BlocksRequestByte},
	gowire.ConcreteType{&BlocksMessage{}, BlocksResponseByte},
	gowire.ConcreteType{&GetHeaderMessage{}, HeaderRequestByte},
	gowire.ConcreteType{&HeaderMessage{}, HeaderResponseByte},
	gowire.ConcreteType{&StatusRequestMessage{}, StatusRequestByte},
	gowire.ConcreteType{&StatusResponseMessage{}, StatusResponseByte},
	gowire.ConcreteType{&TransactionMessage{}, NewTransactionByte},
	gowire.ConcreteType{&MineBlockMessage{}, NewMineBlockByte},
	gowire.ConcreteType{&FilterLoadMessage{}, FilterLoadByte},
	gowire.ConcreteType{&FilterAddMessage{}, FilterAddByte},
	gowire.ConcreteType{&FilterClearMessage{}, FilterClearByte},
	//gowire.ConcreteType{&GetMerkleBlockMessage{}, MerkleRequestByte},
	//gowire.ConcreteType{&MerkleBlockMessage{}, MerkleResponseByte},
)

//DecodeMessage decode msg
func DecodeMessage(bz []byte) (msgType byte, msg BlockchainMessage, err error) {
	msgType = bz[0]
	n := int(0)
	r := bytes.NewReader(bz)
	msg = gowire.ReadBinary(struct{ BlockchainMessage }{}, r, maxBlockchainResponseSize, &n, &err).(struct{ BlockchainMessage }).BlockchainMessage
	if err != nil && n != len(bz) {
		err = errors.New("DecodeMessage() had bytes left over")
	}
	return
}

//GetBlockMessage request blocks from remote peers by height/hash
type GetBlockMessage struct {
	Height  uint64
	RawHash [32]byte
}

//GetHash reutrn the hash of the request
func (m *GetBlockMessage) GetHash() *wire.Hash {
	hash, _ := wire.NewHash(m.RawHash[:])
	return hash
}

//String convert msg to string
func (m *GetBlockMessage) String() string {
	if m.Height > 0 {
		return fmt.Sprintf("GetBlockMessage{Height: %d}", m.Height)
	}
	hash := m.GetHash()
	return fmt.Sprintf("GetBlockMessage{Hash: %s}", hash.String())
}

//BlockMessage response get block msg
type BlockMessage struct {
	RawBlock []byte
}

//NewBlockMessage construct bock response msg
func NewBlockMessage(block *massutil.Block) (*BlockMessage, error) {
	rawBlock, err := block.Bytes(wire.Packet)
	if err != nil {
		return nil, err
	}
	return &BlockMessage{RawBlock: rawBlock}, nil
}

//GetBlock get block from msg
func (m *BlockMessage) GetBlock() (*massutil.Block, error) {
	block, err := massutil.NewBlockFromBytes(m.RawBlock, wire.Packet)
	if err != nil {
		return nil, err
	}
	return block, nil
}

//String convert msg to string
func (m *BlockMessage) String() string {
	return fmt.Sprintf("BlockMessage{Size: %d}", len(m.RawBlock))
}

//GetHeaderMessage request header from remote peers by height/hash
type GetHeaderMessage struct {
	Height  uint64
	RawHash [32]byte
}

//GetHash reutrn the hash of the request
func (m *GetHeaderMessage) GetHash() *wire.Hash {
	hash, _ := wire.NewHash(m.RawHash[:])
	return hash
}

//String convert msg to string
func (m *GetHeaderMessage) String() string {
	if m.Height > 0 {
		return fmt.Sprintf("GetHeaderMessage{Height: %d}", m.Height)
	}
	hash := m.GetHash()
	return fmt.Sprintf("GetHeaderMessage{Hash: %s}", hash.String())
}

//HeaderMessage response get header msg
type HeaderMessage struct {
	RawHeader []byte
}

//NewHeaderMessage construct bock response msg
func NewHeaderMessage(header *wire.BlockHeader) (*HeaderMessage, error) {
	rawHeader, err := header.Bytes(wire.Packet)
	if err != nil {
		return nil, err
	}
	return &HeaderMessage{RawHeader: rawHeader}, nil
}

//GetHeader get header from msg
func (m *HeaderMessage) GetHeader() (*wire.BlockHeader, error) {
	header, err := wire.NewBlockHeaderFromBytes(m.RawHeader, wire.Packet)
	if err != nil {
		return nil, err
	}
	return header, nil
}

//String convert msg to string
func (m *HeaderMessage) String() string {
	return fmt.Sprintf("HeaderMessage{Size: %d}", len(m.RawHeader))
}

//GetHeadersMessage is one of the mass msg type
type GetHeadersMessage struct {
	RawBlockLocator [][32]byte
	RawStopHash     [32]byte
}

//NewGetHeadersMessage return a new GetHeadersMessage
func NewGetHeadersMessage(blockLocator []*wire.Hash, stopHash *wire.Hash) *GetHeadersMessage {
	msg := &GetHeadersMessage{
		RawStopHash: *stopHash,
	}
	for _, hash := range blockLocator {
		msg.RawBlockLocator = append(msg.RawBlockLocator, *hash)
	}
	return msg
}

//GetBlockLocator return the locator of the msg
func (msg *GetHeadersMessage) GetBlockLocator() []*wire.Hash {
	blockLocator := []*wire.Hash{}
	for _, rawHash := range msg.RawBlockLocator {
		hash, _ := wire.NewHash(rawHash[:])
		blockLocator = append(blockLocator, hash)
	}
	return blockLocator
}

//GetStopHash return the stop hash of the msg
func (msg *GetHeadersMessage) GetStopHash() *wire.Hash {
	hash, _ := wire.NewHash(msg.RawStopHash[:])
	return hash
}

//HeadersMessage is one of the mass msg type
type HeadersMessage struct {
	RawHeaders [][]byte
}

//NewHeadersMessage create a new HeadersMessage
func NewHeadersMessage(headers []*wire.BlockHeader) (*HeadersMessage, error) {
	RawHeaders := [][]byte{}
	for _, header := range headers {
		data, err := header.Bytes(wire.Packet)
		if err != nil {
			return nil, err
		}

		RawHeaders = append(RawHeaders, data)
	}
	return &HeadersMessage{RawHeaders: RawHeaders}, nil
}

//GetHeaders return the headers in the msg
func (msg *HeadersMessage) GetHeaders() ([]*wire.BlockHeader, error) {
	headers := []*wire.BlockHeader{}
	for _, data := range msg.RawHeaders {
		if header, err := wire.NewBlockHeaderFromBytes(data, wire.Packet); err != nil {
			return nil, err
		} else {
			headers = append(headers, header)
		}
	}
	return headers, nil
}

//GetBlocksMessage is one of the mass msg type
type GetBlocksMessage struct {
	RawBlockLocator [][32]byte
	RawStopHash     [32]byte
}

//NewGetBlocksMessage create a new GetBlocksMessage
func NewGetBlocksMessage(blockLocator []*wire.Hash, stopHash *wire.Hash) *GetBlocksMessage {
	msg := &GetBlocksMessage{
		RawStopHash: *stopHash,
	}
	for _, hash := range blockLocator {
		msg.RawBlockLocator = append(msg.RawBlockLocator, *hash)
	}
	return msg
}

//GetBlockLocator return the locator of the msg
func (msg *GetBlocksMessage) GetBlockLocator() []*wire.Hash {
	blockLocator := []*wire.Hash{}
	for _, rawHash := range msg.RawBlockLocator {
		hash, _ := wire.NewHash(rawHash[:])
		blockLocator = append(blockLocator, hash)
	}
	return blockLocator
}

//GetStopHash return the stop hash of the msg
func (msg *GetBlocksMessage) GetStopHash() *wire.Hash {
	hash, _ := wire.NewHash(msg.RawStopHash[:])
	return hash
}

//BlocksMessage is one of the mass msg type
type BlocksMessage struct {
	RawBlocks [][]byte
}

//NewBlocksMessage create a new BlocksMessage
func NewBlocksMessage(blocks []*massutil.Block) (*BlocksMessage, error) {
	rawBlocks := [][]byte{}
	for _, block := range blocks {
		data, err := block.Bytes(wire.Packet)
		if err != nil {
			return nil, err
		}

		rawBlocks = append(rawBlocks, data)
	}
	return &BlocksMessage{RawBlocks: rawBlocks}, nil
}

//GetBlocks returns the blocks in the msg
func (msg *BlocksMessage) GetBlocks() ([]*massutil.Block, error) {
	blocks := []*massutil.Block{}
	for _, data := range msg.RawBlocks {
		block, err := massutil.NewBlockFromBytes(data, wire.Packet)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}
	return blocks, nil
}

//StatusRequestMessage status request msg
type StatusRequestMessage struct{}

//String
func (m *StatusRequestMessage) String() string {
	return "StatusRequestMessage"
}

//StatusResponseMessage get status response msg
type StatusResponseMessage struct {
	Height      uint64
	RawHash     [32]byte
	GenesisHash [32]byte
}

//NewStatusResponseMessage construct get status response msg
func NewStatusResponseMessage(blockHeader *wire.BlockHeader, hash *wire.Hash) *StatusResponseMessage {
	return &StatusResponseMessage{
		Height:      blockHeader.Height,
		RawHash:     blockHeader.BlockHash(),
		GenesisHash: *hash,
	}
}

//GetHash get hash from msg
func (m *StatusResponseMessage) GetHash() *wire.Hash {
	hash, _ := wire.NewHash(m.RawHash[:])
	return hash
}

//GetGenesisHash get hash from msg
func (m *StatusResponseMessage) GetGenesisHash() *wire.Hash {
	hash, _ := wire.NewHash(m.GenesisHash[:])
	return hash
}

//String convert msg to string
func (m *StatusResponseMessage) String() string {
	hash := m.GetHash()
	genesisHash := m.GetGenesisHash()
	return fmt.Sprintf("StatusResponseMessage{Height: %d, Best hash: %s, Genesis hash: %s}", m.Height, hash.String(), genesisHash.String())
}

//TransactionMessage notify new tx msg
type TransactionMessage struct {
	RawTx []byte
}

//NewTransactionMessage construct notify new tx msg
func NewTransactionMessage(tx *massutil.Tx) (*TransactionMessage, error) {
	rawTx, err := tx.Bytes(wire.Packet)
	if err != nil {
		return nil, err
	}
	return &TransactionMessage{RawTx: rawTx}, nil
}

//GetTransaction get tx from msg
func (m *TransactionMessage) GetTransaction() (*massutil.Tx, error) {
	tx, err := massutil.NewTxFromBytes(m.RawTx, wire.Packet)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

//String
func (m *TransactionMessage) String() string {
	return fmt.Sprintf("TransactionMessage{Size: %d}", len(m.RawTx))
}

//MineBlockMessage new mined block msg
type MineBlockMessage struct {
	RawBlock []byte
}

//NewMinedBlockMessage construct new mined block msg
func NewMinedBlockMessage(block *massutil.Block) (*MineBlockMessage, error) {
	rawBlock, err := block.Bytes(wire.Packet)
	if err != nil {
		return nil, err
	}
	return &MineBlockMessage{RawBlock: rawBlock}, nil
}

//GetMineBlock get mine block from msg
func (m *MineBlockMessage) GetMineBlock() (*massutil.Block, error) {
	block, err := massutil.NewBlockFromBytes(m.RawBlock, wire.Packet)
	if err != nil {
		return nil, err
	}
	return block, nil
}

//String convert msg to string
func (m *MineBlockMessage) String() string {
	return fmt.Sprintf("NewMineBlockMessage{Size: %d}", len(m.RawBlock))
}

//FilterLoadMessage tells the receiving peer to filter the transactions according to address.
type FilterLoadMessage struct {
	Addresses [][]byte
}

// FilterAddMessage tells the receiving peer to add address to the filter.
type FilterAddMessage struct {
	Address []byte
}

//FilterClearMessage tells the receiving peer to remove a previously-set filter.
type FilterClearMessage struct{}
