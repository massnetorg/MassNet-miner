package wirepb

import (
	"bytes"
	"encoding/binary"
	"io"
)

func (m *ProposalArea) Write(w io.Writer) (int, error) {
	var count int

	for _, punishment := range m.Punishments {
		n, err := punishment.Write(w)
		count += n
		if err != nil {
			return count, err
		}
	}

	for _, proposal := range m.OtherProposals {
		n, err := proposal.Write(w)
		count += n
		if err != nil {
			return count, err
		}
	}

	return count, nil
}

func (m *ProposalArea) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}

func (m *Punishment) Write(w io.Writer) (int, error) {
	var version [4]byte
	binary.LittleEndian.PutUint32(version[:], uint32(m.Version))
	var mType [4]byte
	binary.LittleEndian.PutUint32(mType[:], uint32(m.Type))

	var count int

	n, err := writeBytes(w, version[:], mType[:])
	count += n
	if err != nil {
		return count, err
	}

	n, err = m.TestimonyA.Write(w)
	count += n
	if err != nil {
		return count, err
	}

	n, err = m.TestimonyB.Write(w)
	count += n
	if err != nil {
		return count, err
	}

	return count, nil
}

func (m *Punishment) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}

func (m *Proposal) Write(w io.Writer) (int, error) {
	var version [4]byte
	binary.LittleEndian.PutUint32(version[:], uint32(m.Version))
	var mType [4]byte
	binary.LittleEndian.PutUint32(mType[:], uint32(m.Type))

	return writeBytes(w, version[:], mType[:], m.Content)
}

func (m *Proposal) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}
