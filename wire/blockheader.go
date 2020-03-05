package wire

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/golang/protobuf/proto"
	"massnet.org/mass/logging"
	"massnet.org/mass/poc"
	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/pocec"
	wirepb "massnet.org/mass/wire/pb"
)

// BlockVersion is the current latest supported block version.
const BlockVersion = 1

const MinBlockHeaderPayload = blockHeaderMinPlainSize

type BlockHeader struct {
	ChainID         Hash
	Version         uint64
	Height          uint64
	Timestamp       time.Time
	Previous        Hash
	TransactionRoot Hash
	WitnessRoot     Hash
	ProposalRoot    Hash
	Target          *big.Int
	Challenge       Hash
	PubKey          *pocec.PublicKey
	Proof           *poc.Proof
	Signature       *pocec.Signature
	BanList         []*pocec.PublicKey
}

// blockHeaderMinPlainSize is a constant that represents the number of bytes for a block
// header.
// Length = 32(ChainID) + 8(Version) + 8(Height) + 8(Timestamp) + 32(Previous) + 32(TransactionRoot)
//        + 32(WitnessRoot)+ 32(ProposalRoot) + 32(Target) + 32(Challenge) + 33(PubKey) + 16(Proof)
//        + 72(Signature) + (n*33)(BanList)
//        = 369 Bytes
const blockHeaderMinPlainSize = 369

// BlockHash computes the block identifier hash for the given block header.
func (h *BlockHeader) BlockHash() Hash {
	buf, _ := h.Bytes(ID)
	return DoubleHashH(buf)
}

// Decode decodes r using the given protocol encoding into the receiver.
func (h *BlockHeader) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	var buf bytes.Buffer
	n64, err := buf.ReadFrom(r)
	n = int(n64)
	if err != nil {
		return n, err
	}

	switch mode {
	case DB, Packet:
		pb := new(wirepb.BlockHeader)
		err = proto.Unmarshal(buf.Bytes(), pb)
		if err != nil {
			return n, err
		}
		return n, h.FromProto(pb)

	default:
		return n, ErrInvalidCodecMode
	}
}

// Encode encodes the receiver to w using the given protocol encoding.
func (h *BlockHeader) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	pb := h.ToProto()

	switch mode {
	case DB, Packet:
		content, err := proto.Marshal(pb)
		if err != nil {
			return 0, err
		}
		return w.Write(content)

	case Plain, ID:
		// Write every elements of blockHeader
		return pb.Write(w)

	case PoCID:
		// Write elements excepts for Signature
		return pb.WritePoC(w)

	case ChainID:
		if pb.Height > 0 {
			logging.CPrint(logging.FATAL, "ChainID only be calc for genesis block", logging.LogFormat{"height": pb.Height})
			panic(nil) // unreachable
		}
		// Write elements excepts for ChainID
		return w.Write(pb.BytesChainID())

	default:
		return 0, ErrInvalidCodecMode
	}
}

func (h *BlockHeader) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(h, mode)
}

func (h *BlockHeader) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(h, bs, mode)
}

func (h *BlockHeader) PlainSize() int {
	return getPlainSize(h)
}

// Quality
func (h *BlockHeader) Quality() *big.Int {
	pubKeyHash := pocutil.PubKeyHash(h.PubKey)
	quality, err := h.Proof.GetVerifiedQuality(pocutil.Hash(pubKeyHash), pocutil.Hash(h.Challenge), uint64(h.Timestamp.Unix())/poc.PoCSlot, h.Height)
	if err != nil && h.Height != 0 {
		logging.CPrint(logging.FATAL, "fail to get header quality", logging.LogFormat{"err": err, "height": h.Height, "pubKey": hex.EncodeToString(h.PubKey.SerializeCompressed())})
	}
	return quality
}

// GetChainID calc chainID, only block with 0 height can be calc
func (h *BlockHeader) GetChainID() (Hash, error) {
	if h.Height > 0 {
		return Hash{}, errors.New(fmt.Sprintf("invalid height %d to calc chainID", h.Height))
	}
	buf, err := h.Bytes(ChainID)
	if err != nil {
		return Hash{}, err
	}
	return DoubleHashH(buf), nil
}

func NewBlockHeaderFromBytes(bhBytes []byte, mode CodecMode) (*BlockHeader, error) {
	bh := NewEmptyBlockHeader()

	err := bh.SetBytes(bhBytes, mode)
	if err != nil {
		return nil, err
	}

	return bh, nil
}

func NewEmptyBigInt() *big.Int {
	return new(big.Int).SetUint64(0)
}

func NewEmptyPoCSignature() *pocec.Signature {
	return &pocec.Signature{
		R: NewEmptyBigInt(),
		S: NewEmptyBigInt(),
	}
}

func NewEmptyPoCPublicKey() *pocec.PublicKey {
	return &pocec.PublicKey{
		X: NewEmptyBigInt(),
		Y: NewEmptyBigInt(),
	}
}

func NewEmptyBlockHeader() *BlockHeader {
	return &BlockHeader{
		Timestamp: time.Unix(0, 0),
		Target:    NewEmptyBigInt(),
		Proof:     poc.NewEmptyProof(),
		PubKey:    NewEmptyPoCPublicKey(),
		Signature: NewEmptyPoCSignature(),
		BanList:   make([]*pocec.PublicKey, 0),
	}
}

// PoCHash generate hash of all PoC needed elements in block header
func (h *BlockHeader) PoCHash() (Hash, error) {
	buf, err := h.Bytes(PoCID)
	if err != nil {
		return Hash{}, err
	}

	return DoubleHashH(buf), nil
}

// ToProto get proto BlockHeader from wire BlockHeader
func (h *BlockHeader) ToProto() *wirepb.BlockHeader {
	banList := make([]*wirepb.PublicKey, len(h.BanList))
	for i, pub := range h.BanList {
		banList[i] = wirepb.PublicKeyToProto(pub)
	}

	return &wirepb.BlockHeader{
		ChainID:         h.ChainID.ToProto(),
		Version:         h.Version,
		Height:          h.Height,
		Timestamp:       uint64(h.Timestamp.Unix()),
		Previous:        h.Previous.ToProto(),
		TransactionRoot: h.TransactionRoot.ToProto(),
		WitnessRoot:     h.WitnessRoot.ToProto(),
		ProposalRoot:    h.ProposalRoot.ToProto(),
		Target:          wirepb.BigIntToProto(h.Target),
		Challenge:       h.Challenge.ToProto(),
		PubKey:          wirepb.PublicKeyToProto(h.PubKey),
		Proof:           wirepb.ProofToProto(h.Proof),
		Signature:       wirepb.SignatureToProto(h.Signature),
		BanList:         banList,
	}
}

// FromProto load proto BlockHeader into wire BlockHeader
func (h *BlockHeader) FromProto(pb *wirepb.BlockHeader) error {
	if pb == nil {
		return errors.New("nil proto block_header")
	}
	var unmarshalHash = func(h []*Hash, pb []*wirepb.Hash) (err error) {
		for i := range h {
			if err = h[i].FromProto(pb[i]); err != nil {
				return err
			}
		}
		return nil
	}
	chainID, previous, transactionRoot, witnessRoot, proposalRoot, challenge := new(Hash), new(Hash), new(Hash), new(Hash), new(Hash), new(Hash)
	var err error
	if err = unmarshalHash([]*Hash{chainID, previous, transactionRoot, witnessRoot, proposalRoot, challenge},
		[]*wirepb.Hash{pb.ChainID, pb.Previous, pb.TransactionRoot, pb.WitnessRoot, pb.ProposalRoot, pb.Challenge}); err != nil {
		return err
	}
	target := new(big.Int)
	if err = wirepb.ProtoToBigInt(pb.Target, target); err != nil {
		return err
	}
	pub := new(pocec.PublicKey)
	if err = wirepb.ProtoToPublicKey(pb.PubKey, pub); err != nil {
		return err
	}
	proof := new(poc.Proof)
	if err = wirepb.ProtoToProof(pb.Proof, proof); err != nil {
		return err
	}
	sig := new(pocec.Signature)
	if err = wirepb.ProtoToSignature(pb.Signature, sig); err != nil {
		return err
	}
	banList := make([]*pocec.PublicKey, len(pb.BanList))
	for i, pk := range pb.BanList {
		pub := new(pocec.PublicKey)
		if err = wirepb.ProtoToPublicKey(pk, pub); err != nil {
			return err
		}
		banList[i] = pub
	}

	h.ChainID = *chainID
	h.Version = pb.Version
	h.Height = pb.Height
	h.Timestamp = time.Unix(int64(pb.Timestamp), 0)
	h.Previous = *previous
	h.TransactionRoot = *transactionRoot
	h.WitnessRoot = *witnessRoot
	h.ProposalRoot = *proposalRoot
	h.Target = target
	h.Challenge = *challenge
	h.PubKey = pub
	h.Proof = proof
	h.Signature = sig
	h.BanList = banList

	return nil
}

// NewBlockHeaderFromProto get wire BlockHeader from proto BlockHeader
func NewBlockHeaderFromProto(pb *wirepb.BlockHeader) (*BlockHeader, error) {
	h := new(BlockHeader)
	err := h.FromProto(pb)
	if err != nil {
		return nil, err
	}
	return h, nil
}
