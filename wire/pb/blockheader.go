package wirepb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"massnet.org/mass/poc"
)

func (m *BlockHeader) Write(w io.Writer) (int, error) {
	var version, height, timestamp [8]byte
	binary.LittleEndian.PutUint64(version[:], m.Version)
	binary.LittleEndian.PutUint64(height[:], m.Height)
	binary.LittleEndian.PutUint64(timestamp[:], uint64(m.Timestamp))

	var count int
	n, err := writeBytes(w, m.ChainID.Bytes(), version[:], height[:], timestamp[:],
		m.Previous.Bytes(), m.TransactionRoot.Bytes(), m.WitnessRoot.Bytes(),
		m.ProposalRoot.Bytes(), m.Target.Bytes(), m.Challenge.Bytes(),
		m.PubKey.Bytes(), m.Proof.Bytes(), m.Signature.Bytes())
	count += n
	if err != nil {
		return count, err
	}

	for i := 0; i < len(m.BanList); i++ {
		n, err := writeBytes(w, m.BanList[i].Bytes())
		count += n
		if err != nil {
			return count, err
		}
	}

	return count, err
}

func (m *BlockHeader) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}

func (m *BlockHeader) WritePoC(w io.Writer) (int, error) {
	var version, height, timestamp [8]byte
	binary.LittleEndian.PutUint64(version[:], m.Version)
	binary.LittleEndian.PutUint64(height[:], m.Height)
	binary.LittleEndian.PutUint64(timestamp[:], uint64(m.Timestamp))

	var count int
	n, err := writeBytes(w, m.ChainID.Bytes(), version[:], height[:], timestamp[:],
		m.Previous.Bytes(), m.TransactionRoot.Bytes(), m.WitnessRoot.Bytes(),
		m.ProposalRoot.Bytes(), m.Target.Bytes(), m.Challenge.Bytes(),
		m.PubKey.Bytes(), m.Proof.Bytes())
	count += n
	if err != nil {
		return count, err
	}

	for i := 0; i < len(m.BanList); i++ {
		n, err := writeBytes(w, m.BanList[i].Bytes())
		count += n
		if err != nil {
			return count, err
		}
	}

	return count, err
}

func (m *BlockHeader) BytesPoC() []byte {
	var buf bytes.Buffer
	m.WritePoC(&buf)
	return buf.Bytes()
}

func (m *BlockHeader) BytesChainID() []byte {
	var buf bytes.Buffer
	var version, height, timestamp [8]byte
	binary.LittleEndian.PutUint64(version[:], m.Version)
	binary.LittleEndian.PutUint64(height[:], m.Height)
	binary.LittleEndian.PutUint64(timestamp[:], uint64(m.Timestamp))

	writeBytes(&buf, version[:], height[:], timestamp[:],
		m.Previous.Bytes(), m.TransactionRoot.Bytes(), m.WitnessRoot.Bytes(),
		m.ProposalRoot.Bytes(), m.Target.Bytes(), m.Challenge.Bytes(),
		m.PubKey.Bytes(), m.Proof.Bytes())

	for i := 0; i < len(m.BanList); i++ {
		writeBytes(&buf, m.BanList[i].Bytes())
	}

	return buf.Bytes()
}

func ProtoToProof(pb *Proof, proof *poc.Proof) error {
	if pb == nil {
		return errors.New("nil proto proof")
	}
	proof.X = moveBytes(pb.X)
	proof.XPrime = moveBytes(pb.XPrime)
	proof.BitLength = int(pb.BitLength)
	return nil
}

func ProofToProto(proof *poc.Proof) *Proof {
	if proof == nil {
		return nil
	}
	return &Proof{
		X:         proof.X,
		XPrime:    proof.XPrime,
		BitLength: uint32(proof.BitLength),
	}
}

func (m *Proof) Bytes() []byte {
	var bl [4]byte
	binary.LittleEndian.PutUint32(bl[:], uint32(m.BitLength))
	return combineBytes(m.X, m.XPrime, bl[:])
}
