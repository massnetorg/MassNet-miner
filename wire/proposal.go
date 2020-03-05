package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"

	"github.com/golang/protobuf/proto"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire/pb"
)

const (
	ProposalVersion          = 1
	HeadersPerProposal       = 2
	HeaderSizePerPlaceHolder = 500
	PlaceHolderSize          = HeadersPerProposal * HeaderSizePerPlaceHolder
)

type ProposalType uint32

const (
	typeFaultPubKey ProposalType = iota
	typePlaceHolder
	typeAnyMessage
)

// General Proposal Interface
type Proposal interface {
	Bytes(CodecMode) ([]byte, error)
	SetBytes([]byte, CodecMode) error
	Hash(int) Hash
}

func GetProposalHash(proposal Proposal, index int) Hash {
	var b8 [8]byte
	binary.LittleEndian.PutUint64(b8[:], uint64(index))
	buf, _ := proposal.Bytes(ID)
	return DoubleHashH(append(buf, b8[:]...))
}

// FaultPubKey implements Proposal, representing for punishment-double-mining-pubKey
type FaultPubKey struct {
	version   uint32
	PubKey    *pocec.PublicKey
	Testimony [HeadersPerProposal]*BlockHeader
}

func (fpk *FaultPubKey) Version() uint32 {
	return fpk.version
}

func (fpk *FaultPubKey) Type() ProposalType {
	return typeFaultPubKey
}

func (fpk *FaultPubKey) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	pb := fpk.ToProto()

	switch mode {
	case Packet, DB:
		buf, err := proto.Marshal(pb)
		if err != nil {
			return 0, err
		}
		return w.Write(buf)

	case Plain, ID:
		return pb.Write(w)

	default:
		return n, ErrInvalidCodecMode
	}
}

func (fpk *FaultPubKey) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	switch mode {
	case Packet, DB:
		var buf bytes.Buffer
		n64, err := buf.ReadFrom(r)
		n = int(n64)
		if err != nil {
			return n, err
		}
		pb := new(wirepb.Punishment)
		if err := proto.Unmarshal(buf.Bytes(), pb); err != nil {
			return n, err
		}
		return n, fpk.FromProto(pb)

	default:
		return n, ErrInvalidCodecMode
	}
}

func (fpk *FaultPubKey) ToProto() *wirepb.Punishment {
	return &wirepb.Punishment{
		Version:    fpk.Version(),
		Type:       uint32(typeFaultPubKey),
		TestimonyA: fpk.Testimony[0].ToProto(),
		TestimonyB: fpk.Testimony[1].ToProto(),
	}
}

func (fpk *FaultPubKey) FromProto(pb *wirepb.Punishment) error {
	if pb == nil {
		return errors.New("nil proto punishment")
	}
	fpk.version = pb.Version
	headerA, headerB := new(BlockHeader), new(BlockHeader)
	if err := headerA.FromProto(pb.TestimonyA); err != nil {
		return err
	}
	if err := headerB.FromProto(pb.TestimonyB); err != nil {
		return err
	}
	fpk.Testimony = [HeadersPerProposal]*BlockHeader{headerA, headerB}
	fpk.PubKey = fpk.Testimony[0].PubKey
	// TODO: check public_key equal here?
	if !reflect.DeepEqual(fpk.Testimony[0].PubKey, fpk.Testimony[1].PubKey) {
		return errInvalidFaultPubKey
	}
	return nil
}

func (fpk *FaultPubKey) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(fpk, mode)
}

func (fpk *FaultPubKey) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(fpk, bs, mode)
}

// PlainSize returns the number of bytes the FaultPubKey contains.
func (fpk *FaultPubKey) PlainSize() int {
	return getPlainSize(fpk)
}

func (fpk *FaultPubKey) Hash(index int) Hash {
	return GetProposalHash(fpk, index)
}

func (fpk *FaultPubKey) IsValid() error {
	if fpk.PubKey == nil {
		return errFaultPubKeyNoPubKey
	}
	if len(fpk.Testimony) < 2 {
		return errFaultPubKeyNoTestimony
	}
	h0 := fpk.Testimony[0]
	h1 := fpk.Testimony[1]
	// check validity of testimony
	if h0.Height != h1.Height {
		return errFaultPubKeyWrongHeight
	}
	if h0.Proof.BitLength != h1.Proof.BitLength {
		return errFaultPubKeyWrongBigLength
	}
	if h0.BlockHash() == h1.BlockHash() {
		return errFaultPubKeySameBlock
	}
	if !reflect.DeepEqual(h0.PubKey.SerializeUncompressed(), h1.PubKey.SerializeUncompressed()) {
		return errFaultPubKeyWrongPubKey
	}
	pocHash1, err0 := h0.PoCHash()
	pocHash2, err1 := h1.PoCHash()
	if err0 != nil || err1 != nil {
		return errFaultPubKeyNoHash
	}
	pass0 := h0.Signature.Verify(HashB(pocHash1[:]), h0.PubKey)
	pass1 := h1.Signature.Verify(HashB(pocHash2[:]), h1.PubKey)
	if !(pass0 && pass1) {
		return errFaultPubKeyWrongSignature
	}
	return nil
}

func NewEmptyFaultPubKey() *FaultPubKey {
	var t [HeadersPerProposal]*BlockHeader
	for i := 0; i < HeadersPerProposal; i++ {
		bh := NewEmptyBlockHeader()
		t[i] = bh
	}
	return &FaultPubKey{
		version:   ProposalVersion,
		PubKey:    new(pocec.PublicKey),
		Testimony: t,
	}
}

func NewFaultPubKeyFromBytes(bs []byte, mode CodecMode) (*FaultPubKey, error) {
	fpk := new(FaultPubKey)
	err := fpk.SetBytes(bs, mode)
	if err != nil {
		return nil, err
	}
	return fpk, nil
}

// NormalProposal implements Proposal, representing for all proposals
// with no consensus proposalType.
type NormalProposal struct {
	version      uint32
	proposalType ProposalType
	content      []byte
}

func (np *NormalProposal) Version() uint32 {
	return np.version
}

func (np *NormalProposal) Type() ProposalType {
	return np.proposalType
}

func (np *NormalProposal) Content() []byte {
	return np.content
}

func (np *NormalProposal) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	pb := np.ToProto()

	switch mode {
	case Packet, DB:
		buf, err := proto.Marshal(pb)
		if err != nil {
			return n, err
		}
		return w.Write(buf)

	case Plain, ID:
		return pb.Write(w)

	default:
		return n, ErrInvalidCodecMode
	}
}

func (np *NormalProposal) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	switch mode {
	case Packet, DB:
		var buf bytes.Buffer
		n64, err := buf.ReadFrom(r)
		n = int(n64)
		if err != nil {
			return n, err
		}
		pb := new(wirepb.Proposal)
		if err = proto.Unmarshal(buf.Bytes(), pb); err != nil {
			return n, err
		}
		if err = np.FromProto(pb); err != nil {
			return n, err
		}
		return n, nil

	default:
		return n, ErrInvalidCodecMode
	}
}

func (np *NormalProposal) ToProto() *wirepb.Proposal {
	return &wirepb.Proposal{
		Version: np.Version(),
		Type:    uint32(np.Type()),
		Content: np.Content(),
	}
}

func (np *NormalProposal) FromProto(pb *wirepb.Proposal) error {
	if pb == nil {
		return errors.New("nil proto proposal")
	}
	np.version = pb.Version
	np.proposalType = ProposalType(pb.Type)
	np.content = MoveBytes(pb.Content)
	return nil
}

func (np *NormalProposal) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(np, mode)
}

func (np *NormalProposal) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(np, bs, mode)
}

func (np *NormalProposal) Hash(index int) Hash {
	return GetProposalHash(np, index)
}

func (np *NormalProposal) PlainSize() int {
	return getPlainSize(np)
}

// PlaceHolder prevents miner from eliminating all punishment proposals,
// because that is more profitable to save block size for transactions.
func NewPlaceHolder() *NormalProposal {
	return &NormalProposal{
		version:      ProposalVersion,
		proposalType: typePlaceHolder,
		content:      make([]byte, 2*HeaderSizePerPlaceHolder),
	}
}

// ProposalArea represents for proposals in blocks.
type ProposalArea struct {
	PunishmentArea []*FaultPubKey
	OtherArea      []*NormalProposal
}

func (pa *ProposalArea) Count() int {
	return len(pa.PunishmentArea) + len(pa.OtherArea)
}

func (pa *ProposalArea) PunishmentCount() int {
	return len(pa.PunishmentArea)
}

func (pa *ProposalArea) OtherCount() int {
	return len(pa.OtherArea)
}

func (pa *ProposalArea) Encode(w io.Writer, mode CodecMode) (n int, err error) {
	pb, err := pa.ToProto()
	if err != nil {
		return n, err
	}

	switch mode {
	case Packet, DB:
		buf, err := proto.Marshal(pb)
		if err != nil {
			return n, err
		}
		return w.Write(buf)

	case Plain:
		return pb.Write(w)

	default:
		return n, ErrInvalidCodecMode
	}
}

func (pa *ProposalArea) Decode(r io.Reader, mode CodecMode) (n int, err error) {
	switch mode {
	case Packet, DB:
		var buf bytes.Buffer
		n64, err := buf.ReadFrom(r)
		n = int(n64)
		if err != nil {
			return n, err
		}
		pb := new(wirepb.ProposalArea)
		err = proto.Unmarshal(buf.Bytes(), pb)
		if err != nil {
			return n, err
		}

		err = pa.FromProto(pb)
		return n, err

	default:
		return n, ErrInvalidCodecMode
	}
}

func (pa *ProposalArea) Bytes(mode CodecMode) ([]byte, error) {
	return getBytes(pa, mode)
}

func (pa *ProposalArea) SetBytes(bs []byte, mode CodecMode) error {
	return setFromBytes(pa, bs, mode)
}

func (pa *ProposalArea) PlainSize() int {
	size := getPlainSize(pa)
	if pa.PunishmentCount() == 0 {
		size += PlaceHolderSize
	}
	return size
}

func newEmptyProposalArea() *ProposalArea {
	return &ProposalArea{
		PunishmentArea: []*FaultPubKey{},
		OtherArea:      []*NormalProposal{},
	}
}

func NewProposalArea(punishmentArea []*FaultPubKey, otherArea []*NormalProposal) (*ProposalArea, error) {
	for _, other := range otherArea {
		if other.proposalType < typeAnyMessage {
			return nil, errWrongProposalType
		}
	}

	return &ProposalArea{
		PunishmentArea: punishmentArea,
		OtherArea:      otherArea,
	}, nil
}

// ToProto get proto ProposalArea from wire ProposalArea
func (pa *ProposalArea) ToProto() (*wirepb.ProposalArea, error) {
	punishments := make([]*wirepb.Punishment, pa.PunishmentCount())
	for i, fpk := range pa.PunishmentArea {
		punishments[i] = fpk.ToProto()
	}

	others := make([]*wirepb.Proposal, len(pa.OtherArea))
	for i, proposal := range pa.OtherArea {
		if proposal.proposalType < typeAnyMessage {
			return nil, errWrongProposalType
		}
		others[i] = proposal.ToProto()
	}

	return &wirepb.ProposalArea{
		Punishments:    punishments,
		OtherProposals: others,
	}, nil
}

// FromProto load proto ProposalArea into wire ProposalArea,
// if error happens, old content is still immutable
func (pa *ProposalArea) FromProto(pb *wirepb.ProposalArea) error {
	if pb == nil {
		return errors.New("nil proto proposal_area")
	}

	var err error
	punishments := make([]*FaultPubKey, len(pb.Punishments))
	for i, v := range pb.Punishments {
		fpk := new(FaultPubKey)
		if err = fpk.FromProto(v); err != nil {
			return err
		}
		punishments[i] = fpk
	}

	others := make([]*NormalProposal, len(pb.OtherProposals))
	for i, v := range pb.OtherProposals {
		if ProposalType(v.Type) < typeAnyMessage {
			return errWrongProposalType
		}
		np := new(NormalProposal)
		if err = np.FromProto(v); err != nil {
			return err
		}
		others[i] = np
	}

	pa.PunishmentArea = punishments
	pa.OtherArea = others
	return nil
}

// NewProposalAreaFromProto get wire ProposalArea from proto ProposalArea
func NewProposalAreaFromProto(pb *wirepb.ProposalArea) (*ProposalArea, error) {
	pa := new(ProposalArea)
	err := pa.FromProto(pb)
	if err != nil {
		return nil, err
	}
	return pa, nil
}
