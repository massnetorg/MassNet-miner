package wirepb

import (
	"bytes"
	"io"
)

func (m *Block) Write(w io.Writer) (int, error) {
	var count int

	n, err := m.Header.Write(w)
	count += n
	if err != nil {
		return count, err
	}

	n, err = m.Proposals.Write(w)
	count += n
	if err != nil {
		return count, err
	}

	for _, tx := range m.Transactions {
		n, err := tx.Write(w)
		count += n
		if err != nil {
			return count, err
		}
	}

	return count, nil
}

func (m *Block) Bytes() []byte {
	var buf bytes.Buffer
	m.Write(&buf)
	return buf.Bytes()
}
