package protocol

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/google/uuid"
	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
	engine_v2 "massnet.org/mass/poc/engine.v2"
)

// superior -> collector
type RequestQualities struct {
	TaskID       uuid.UUID
	Challenge    pocutil.Hash
	ParentTarget *big.Int
	ParentSlot   uint64
	Height       uint64
}

type MsgRequestQualities struct {
	TaskID       string `json:"task_id"`
	Challenge    string `json:"challenge"`
	ParentTarget string `json:"parent_target"`
	ParentSlot   uint64 `json:"parent_slot"`
	Height       uint64 `json:"height"`
}

func (req *RequestQualities) Msg() *MsgRequestQualities {
	return &MsgRequestQualities{
		TaskID:       req.TaskID.String(),
		Challenge:    req.Challenge.String(),
		ParentTarget: hex.EncodeToString(req.ParentTarget.Bytes()),
		ParentSlot:   req.ParentSlot,
		Height:       req.Height,
	}
}

func (req *RequestQualities) SetMsg(msg *MsgRequestQualities) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	challenge, err := pocutil.DecodeStringToHash(msg.Challenge)
	if err != nil {
		return err
	}
	targetBytes, err := hex.DecodeString(msg.ParentTarget)
	if err != nil {
		return err
	}
	parentTarget := new(big.Int).SetBytes(targetBytes)
	// set value
	req.TaskID = tid
	req.Challenge = challenge
	req.ParentTarget = parentTarget
	req.ParentSlot = msg.ParentSlot
	req.Height = msg.Height
	return nil
}

func (req *RequestQualities) Bytes() ([]byte, error) {
	return json.Marshal(req.Msg())
}

func (req *RequestQualities) SetBytes(data []byte) error {
	msg := &MsgRequestQualities{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return req.SetMsg(msg)
}

func (req *RequestQualities) MsgType() MsgType {
	return MsgTypeRequestQualities
}

func (req *RequestQualities) ID() uuid.UUID {
	return req.TaskID
}

func NewRequestQualities(msg *MsgRequestQualities) (*RequestQualities, error) {
	req := new(RequestQualities)
	if err := req.SetMsg(msg); err != nil {
		return nil, err
	}
	return req, nil
}

func (req *RequestQualities) Copy() *RequestQualities {
	return &RequestQualities{
		TaskID:       req.TaskID,
		Challenge:    req.Challenge,
		ParentTarget: new(big.Int).Set(req.ParentTarget),
		ParentSlot:   req.ParentSlot,
		Height:       req.Height,
	}
}

// collector -> superior
type ReportQualities struct {
	TaskID    uuid.UUID
	Qualities []*Quality
}

type MsgReportQualities struct {
	TaskID    string        `json:"task_id"`
	Qualities []*MsgQuality `json:"qualities"`
}

func (resp *ReportQualities) Msg() *MsgReportQualities {
	qualities := make([]*MsgQuality, 0, len(resp.Qualities))
	for i := range resp.Qualities {
		qualities = append(qualities, resp.Qualities[i].Msg())
	}
	return &MsgReportQualities{
		TaskID:    resp.TaskID.String(),
		Qualities: qualities,
	}
}

func (resp *ReportQualities) SetMsg(msg *MsgReportQualities) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	qualities := make([]*Quality, 0, len(msg.Qualities))
	for i := range msg.Qualities {
		quality, err := NewQuality(msg.Qualities[i])
		if err != nil {
			return err
		}
		qualities = append(qualities, quality)
	}
	// set value
	resp.TaskID = tid
	resp.Qualities = qualities
	return nil
}

func (resp *ReportQualities) Bytes() ([]byte, error) {
	return json.Marshal(resp.Msg())
}

func (resp *ReportQualities) SetBytes(data []byte) error {
	msg := &MsgReportQualities{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return resp.SetMsg(msg)
}

func (resp *ReportQualities) MsgType() MsgType {
	return MsgTypeReportQualities
}

func (resp *ReportQualities) ID() uuid.UUID {
	return resp.TaskID
}

func NewReportQualities(msg *MsgReportQualities) (*ReportQualities, error) {
	resp := new(ReportQualities)
	if err := resp.SetMsg(msg); err != nil {
		return nil, err
	}
	return resp, nil
}

// Quality is used by RequestQualities and ReportQualities
type Quality struct {
	*engine_v2.WorkSpaceQuality
	Slot uint64
}

type MsgQuality struct {
	SpaceID       string `json:"space_id"`
	PublicKey     string `json:"public_key"`
	PoolPublicKey string `json:"pool_public_key"`
	Index         uint32 `json:"index"`
	KSize         uint8  `json:"k_size"`
	Quality       string `json:"quality"`
	PlotID        string `json:"plot_id"`
	Slot          uint64 `json:"slot"`
}

func (q *Quality) Msg() *MsgQuality {
	wsq := q.WorkSpaceQuality
	return &MsgQuality{
		SpaceID:       wsq.SpaceID,
		PublicKey:     hex.EncodeToString(wsq.PublicKey.Bytes()),
		PoolPublicKey: hex.EncodeToString(wsq.PoolPublicKey.Bytes()),
		Index:         wsq.Index,
		KSize:         wsq.KSize,
		Quality:       hex.EncodeToString(wsq.Quality),
		PlotID:        hex.EncodeToString(q.PlotID[:]),
		Slot:          q.Slot,
	}
}

func (q *Quality) SetMsg(msg *MsgQuality) error {
	publicKey, err := newG1ElementFromString(msg.PublicKey)
	if err != nil {
		return err
	}
	poolPublicKey, err := newG1ElementFromString(msg.PoolPublicKey)
	if err != nil {
		return err
	}
	quality, err := hex.DecodeString(msg.Quality)
	if err != nil {
		return err
	}
	plotID, err := pocutil.DecodeStringToHash(msg.PlotID)
	if err != nil {
		return err
	}
	// set value
	wsq := &engine_v2.WorkSpaceQuality{
		SpaceID:       msg.SpaceID,
		PublicKey:     publicKey,
		PoolPublicKey: poolPublicKey,
		Index:         msg.Index,
		KSize:         msg.KSize,
		Quality:       quality,
		Error:         nil,
		PlotID:        [32]byte(plotID),
	}
	q.WorkSpaceQuality = wsq
	q.Slot = msg.Slot
	return nil
}

func NewQuality(msg *MsgQuality) (*Quality, error) {
	q := new(Quality)
	if err := q.SetMsg(msg); err != nil {
		return nil, err
	}
	return q, nil
}

// superior -> collector
type RequestProof struct {
	TaskID    uuid.UUID
	Height    uint64
	SpaceID   string
	Challenge pocutil.Hash
	Index     uint32
}

type MsgRequestProof struct {
	TaskID    string `json:"task_id"`
	Height    uint64 `json:"height"`
	SpaceID   string `json:"space_id"`
	Challenge string `json:"challenge"`
	Index     uint32 `json:"index"`
}

func (req *RequestProof) Msg() *MsgRequestProof {
	return &MsgRequestProof{
		TaskID:    req.TaskID.String(),
		Height:    req.Height,
		SpaceID:   req.SpaceID,
		Challenge: req.Challenge.String(),
		Index:     req.Index,
	}
}

func (req *RequestProof) SetMsg(msg *MsgRequestProof) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	challenge, err := pocutil.DecodeStringToHash(msg.Challenge)
	if err != nil {
		return err
	}
	// set value
	req.TaskID = tid
	req.Height = msg.Height
	req.SpaceID = msg.SpaceID
	req.Challenge = challenge
	req.Index = msg.Index
	return nil
}

func (req *RequestProof) Bytes() ([]byte, error) {
	return json.Marshal(req.Msg())
}

func (req *RequestProof) SetBytes(data []byte) error {
	msg := &MsgRequestProof{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return req.SetMsg(msg)
}

func (req *RequestProof) MsgType() MsgType {
	return MsgTypeRequestProof
}

func (req *RequestProof) ID() uuid.UUID {
	return req.TaskID
}

func (req *RequestProof) Copy() *RequestProof {
	return &RequestProof{
		TaskID:    req.TaskID,
		Height:    req.Height,
		SpaceID:   req.SpaceID,
		Challenge: req.Challenge,
		Index:     req.Index,
	}
}

func NewRequestProof(msg *MsgRequestProof) (*RequestProof, error) {
	req := new(RequestProof)
	if err := req.SetMsg(msg); err != nil {
		return nil, err
	}
	return req, nil
}

// collector -> superior
type ReportProof struct {
	TaskID uuid.UUID
	Proof  *Proof
}

type MsgReportProof struct {
	TaskID string    `json:"task_id"`
	Proof  *MsgProof `json:"proof"`
}

func (resp *ReportProof) Msg() *MsgReportProof {
	return &MsgReportProof{
		TaskID: resp.TaskID.String(),
		Proof:  resp.Proof.Msg(),
	}
}

func (resp *ReportProof) SetMsg(msg *MsgReportProof) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	proof, err := NewProof(msg.Proof)
	if err != nil {
		return err
	}
	// set value
	resp.TaskID = tid
	resp.Proof = proof
	return nil
}

func (resp *ReportProof) Bytes() ([]byte, error) {
	return json.Marshal(resp.Msg())
}

func (resp *ReportProof) SetBytes(data []byte) error {
	msg := &MsgReportProof{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return resp.SetMsg(msg)
}

func (resp *ReportProof) MsgType() MsgType {
	return MsgTypeReportProof
}

func (resp *ReportProof) ID() uuid.UUID {
	return resp.TaskID
}

func NewReportProof(msg *MsgReportProof) (*ReportProof, error) {
	resp := new(ReportProof)
	if err := resp.SetMsg(msg); err != nil {
		return nil, err
	}
	return resp, nil
}

// Proof is used by RequestProof and ReportProof
type Proof engine_v2.WorkSpaceProof

type MsgProof struct {
	SpaceID       string `json:"space_id"`
	Challenge     string `json:"challenge"`
	PoolPublicKey string `json:"pool_public_key"`
	PlotPublicKey string `json:"plot_public_key"`
	KSize         uint8  `json:"k_size"`
	Proof         string `json:"proof"`
}

func (p *Proof) Msg() *MsgProof {
	return &MsgProof{
		SpaceID:       p.SpaceID,
		Challenge:     hex.EncodeToString(p.Proof.Challenge[:]),
		PoolPublicKey: hex.EncodeToString(p.Proof.PoolPublicKey.Bytes()),
		PlotPublicKey: hex.EncodeToString(p.Proof.PlotPublicKey.Bytes()),
		KSize:         p.Proof.KSize,
		Proof:         hex.EncodeToString(p.Proof.Proof),
	}
}

func (p *Proof) SetMsg(msg *MsgProof) error {
	challenge, err := pocutil.DecodeStringToHash(msg.Challenge)
	if err != nil {
		return err
	}
	poolPublicKey, err := newG1ElementFromString(msg.PoolPublicKey)
	if err != nil {
		return err
	}
	plotPublicKey, err := newG1ElementFromString(msg.PlotPublicKey)
	if err != nil {
		return err
	}
	proof, err := hex.DecodeString(msg.Proof)
	if err != nil {
		return err
	}
	// set value
	pos := &chiapos.ProofOfSpace{
		Challenge:     challenge,
		PoolPublicKey: poolPublicKey,
		PlotPublicKey: plotPublicKey,
		KSize:         msg.KSize,
		Proof:         proof,
	}
	p.SpaceID = msg.SpaceID
	p.Proof = pos
	p.PublicKey = plotPublicKey
	p.Ordinal = engine_v2.UnknownOrdinal
	return nil
}

func NewProof(msg *MsgProof) (*Proof, error) {
	p := new(Proof)
	if err := p.SetMsg(msg); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Proof) WorkSpaceProof() *engine_v2.WorkSpaceProof {
	return (*engine_v2.WorkSpaceProof)(p)
}

type RequestSignature struct {
	TaskID  uuid.UUID
	Height  uint64
	SpaceID string
	Hash    pocutil.Hash
}

type MsgRequestSignature struct {
	TaskID  string `json:"task_id"`
	Height  uint64 `json:"height"`
	SpaceID string `json:"space_id"`
	Hash    string `json:"hash"`
}

func (req *RequestSignature) Msg() *MsgRequestSignature {
	return &MsgRequestSignature{
		TaskID:  req.TaskID.String(),
		Height:  req.Height,
		SpaceID: req.SpaceID,
		Hash:    hex.EncodeToString(req.Hash[:]),
	}
}

func (req *RequestSignature) SetMsg(msg *MsgRequestSignature) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	hash, err := pocutil.DecodeStringToHash(msg.Hash)
	if err != nil {
		return err
	}
	// set value
	req.TaskID = tid
	req.Height = msg.Height
	req.SpaceID = msg.SpaceID
	req.Hash = hash
	return nil
}

func (req *RequestSignature) Bytes() ([]byte, error) {
	return json.Marshal(req.Msg())
}

func (req *RequestSignature) SetBytes(data []byte) error {
	msg := &MsgRequestSignature{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return req.SetMsg(msg)
}

func (req *RequestSignature) MsgType() MsgType {
	return MsgTypeRequestSignature
}

func (req *RequestSignature) ID() uuid.UUID {
	return req.TaskID
}

func NewRequestSignature(msg *MsgRequestSignature) (*RequestSignature, error) {
	req := new(RequestSignature)
	if err := req.SetMsg(msg); err != nil {
		return nil, err
	}
	return req, nil
}

type ReportSignature struct {
	TaskID    uuid.UUID
	SpaceID   string
	Hash      pocutil.Hash
	Signature *chiapos.G2Element
}

type MsgReportSignature struct {
	TaskID    string `json:"task_id"`
	SpaceID   string `json:"space_id"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

func (resp *ReportSignature) Msg() *MsgReportSignature {
	return &MsgReportSignature{
		TaskID:    resp.TaskID.String(),
		SpaceID:   resp.SpaceID,
		Hash:      hex.EncodeToString(resp.Hash[:]),
		Signature: hex.EncodeToString(resp.Signature.Bytes()),
	}
}

func (resp *ReportSignature) SetMsg(msg *MsgReportSignature) error {
	tid, err := uuid.Parse(msg.TaskID)
	if err != nil {
		return err
	}
	hash, err := pocutil.DecodeStringToHash(msg.Hash)
	if err != nil {
		return err
	}
	sig, err := newG2ElementFromString(msg.Signature)
	if err != nil {
		return err
	}
	// set value
	resp.TaskID = tid
	resp.SpaceID = msg.SpaceID
	resp.Hash = hash
	resp.Signature = sig
	return nil
}

func (resp *ReportSignature) Bytes() ([]byte, error) {
	return json.Marshal(resp.Msg())
}

func (resp *ReportSignature) SetBytes(data []byte) error {
	msg := &MsgReportSignature{}
	if err := json.Unmarshal(data, msg); err != nil {
		return err
	}
	return resp.SetMsg(msg)
}

func (resp *ReportSignature) MsgType() MsgType {
	return MsgTypeReportSignature
}

func (resp *ReportSignature) ID() uuid.UUID {
	return resp.TaskID
}

func NewReportSignature(msg *MsgReportSignature) (*ReportSignature, error) {
	resp := new(ReportSignature)
	if err := resp.SetMsg(msg); err != nil {
		return nil, err
	}
	return resp, nil
}

func newG1ElementFromString(s string) (*chiapos.G1Element, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return chiapos.NewG1ElementFromBytes(data)
}

func newG2ElementFromString(s string) (*chiapos.G2Element, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return chiapos.NewG2ElementFromBytes(data)
}

func newPrivateKeyFromString(s string) (*chiapos.PrivateKey, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return chiapos.NewPrivateKeyFromBytes(data)
}
