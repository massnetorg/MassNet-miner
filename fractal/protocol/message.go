package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type MsgType uint16

const (
	MsgTypeReserved         MsgType = 0
	MsgTypeRequestQualities MsgType = 1
	MsgTypeReportQualities  MsgType = 2
	MsgTypeRequestProof     MsgType = 3
	MsgTypeReportProof      MsgType = 4
	MsgTypeRequestSignature MsgType = 5
	MsgTypeReportSignature  MsgType = 6
)

const msgTypeByteSize = 2

func msgTypeToBytes(typ MsgType) []byte {
	bs := make([]byte, msgTypeByteSize)
	binary.BigEndian.PutUint16(bs, uint16(typ))
	return bs
}

func msgTypeFromBytes(bs []byte) (MsgType, error) {
	if len(bs) < msgTypeByteSize {
		return 0, errors.New("invalid msg type bytes")
	}
	return MsgType(binary.BigEndian.Uint16(bs)), nil
}

type Message interface {
	Bytes() ([]byte, error)
	SetBytes([]byte) error
	MsgType() MsgType
	ID() uuid.UUID
}

func EncodeMessage(msg Message) ([]byte, error) {
	msgBytes, err := msg.Bytes()
	if err != nil {
		return nil, err
	}
	typeBytes := msgTypeToBytes(msg.MsgType())
	return bytes.Join([][]byte{typeBytes, msgBytes}, nil), err
}

func DecodeMessage(data []byte) (Message, error) {
	typ, err := msgTypeFromBytes(data)
	if err != nil {
		return nil, err
	}
	var msg Message
	switch typ {
	case MsgTypeRequestQualities:
		msg = &RequestQualities{}
	case MsgTypeReportQualities:
		msg = &ReportQualities{}
	case MsgTypeRequestProof:
		msg = &RequestProof{}
	case MsgTypeReportProof:
		msg = &ReportProof{}
	case MsgTypeRequestSignature:
		msg = &RequestSignature{}
	case MsgTypeReportSignature:
		msg = &ReportSignature{}
	default:
		return nil, fmt.Errorf("unknown msg type: %v", typ)
	}
	err = msg.SetBytes(data[msgTypeByteSize:])
	return msg, err
}
