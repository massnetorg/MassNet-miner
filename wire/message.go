package wire

import (
	"bytes"
	"io"
	"io/ioutil"
)

// Mode to encode/decode data
type CodecMode uint64

const (
	DB CodecMode = iota
	Packet
	Plain
	ID
	WitnessID
	PoCID
	ChainID
)

type Message interface {
	Encode(io.Writer, CodecMode) (int, error)
	Decode(io.Reader, CodecMode) (int, error)
	Bytes(CodecMode) ([]byte, error)
	SetBytes([]byte, CodecMode) error
	PlainSize() int
}

func getBytes(msg Message, mode CodecMode) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := msg.Encode(&buf, mode); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setFromBytes(msg Message, bs []byte, mode CodecMode) error {
	r := bytes.NewReader(bs)
	_, err := msg.Decode(r, mode)
	return err
}

func getPlainSize(msg Message) int {
	n, _ := msg.Encode(ioutil.Discard, Plain)
	return n
}
