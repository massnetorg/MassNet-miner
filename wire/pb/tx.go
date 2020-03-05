package wirepb

import (
	"bytes"
	"encoding/binary"
	"io"
)

func (m *Tx) Write(w io.Writer) (int, error) {
	var count int

	var version [4]byte
	binary.LittleEndian.PutUint32(version[:], m.Version)
	n, err := writeBytes(w, version[:])
	count += n
	if err != nil {
		return count, err
	}

	for _, in := range m.TxIn {
		n, err := writeBytes(w, in.Bytes())
		count += n
		if err != nil {
			return count, err
		}
	}

	for _, out := range m.TxOut {
		n, err := writeBytes(w, out.Bytes())
		count += n
		if err != nil {
			return count, err
		}
	}

	var lockTime [8]byte
	binary.LittleEndian.PutUint64(lockTime[:], m.LockTime)
	n, err = writeBytes(w, lockTime[:], m.Payload)
	count += n
	if err != nil {
		return count, err
	}

	return count, nil
}

func (m *Tx) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}

func (m *Tx) WriteNoWitness(w io.Writer) (int, error) {
	var count int

	var version [4]byte
	binary.LittleEndian.PutUint32(version[:], m.Version)
	n, err := writeBytes(w, version[:])
	count += n
	if err != nil {
		return count, err
	}

	for _, in := range m.TxIn {
		n, err := writeBytes(w, in.BytesNoWitness())
		count += n
		if err != nil {
			return count, err
		}
	}

	for _, out := range m.TxOut {
		n, err := writeBytes(w, out.Bytes())
		count += n
		if err != nil {
			return count, err
		}
	}

	var lockTime [8]byte
	binary.LittleEndian.PutUint64(lockTime[:], m.LockTime)
	n, err = writeBytes(w, lockTime[:], m.Payload)
	count += n
	if err != nil {
		return count, err
	}

	return count, nil
}

func (m *Tx) BytesNoWitness() []byte {
	var buf bytes.Buffer
	m.WriteNoWitness(&buf)
	return buf.Bytes()
}

func (m *TxIn) Bytes() []byte {
	var buf bytes.Buffer

	writeBytes(&buf, m.PreviousOutPoint.Bytes())
	for _, witness := range m.Witness {
		writeBytes(&buf, witness)
	}

	var sequence [8]byte
	binary.LittleEndian.PutUint64(sequence[:], m.Sequence)
	writeBytes(&buf, sequence[:])

	return buf.Bytes()
}

func (m *TxIn) BytesNoWitness() []byte {
	var sequence [8]byte
	binary.LittleEndian.PutUint64(sequence[:], m.Sequence)
	return combineBytes(m.PreviousOutPoint.Bytes(), sequence[:])
}

func (m *OutPoint) Bytes() []byte {
	var index [4]byte
	binary.LittleEndian.PutUint32(index[:], m.Index)
	return combineBytes(m.Hash.Bytes(), index[:])
}

func (m *TxOut) Bytes() []byte {
	var value [8]byte
	binary.LittleEndian.PutUint64(value[:], uint64(m.Value))
	return combineBytes(value[:], m.PkScript)
}
