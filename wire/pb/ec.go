package wirepb

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/pocec"
)

// BigIntToProto get proto BigInt from golang big.Int
func BigIntToProto(x *big.Int) *BigInt {
	if x == nil {
		return nil
	}
	pb := new(BigInt)
	pb.Raw = x.Bytes()
	return pb
}

// ProtoToBigInt get golang big.Int from proto BigInt
func ProtoToBigInt(pb *BigInt, bi *big.Int) error {
	if pb == nil {
		return errors.New("nil proto big_int")
	}
	bi.SetBytes(pb.Raw)
	return nil
}

func (m *BigInt) Bytes() []byte {
	buf := make([]byte, len(m.Raw))
	copy(buf, m.Raw)
	return buf
}

// NewEmptyPublicKey returns new empty initialized proto PublicKey
func NewEmptyPublicKey() *PublicKey {
	return &PublicKey{
		Raw: make([]byte, 0),
	}
}

// PublicKeyToProto accespts a btcec/pocec PublicKey, returns a proto PublicKey
func PublicKeyToProto(pub interface{}) *PublicKey {
	if pub == nil {
		return nil
	}
	pb := NewEmptyPublicKey()
	switch p := pub.(type) {
	case *btcec.PublicKey:
		pb.Raw = p.SerializeCompressed()
	case *pocec.PublicKey:
		pb.Raw = p.SerializeCompressed()
	}
	return pb
}

// ProtoToPublicKey accepts a proto PublicKey and a btcec/pocec PublicKey,
// fills content into the latter
func ProtoToPublicKey(pb *PublicKey, pub interface{}) error {
	if pb == nil {
		return errors.New("nil proto public_key")
	}
	switch p := pub.(type) {
	case *btcec.PublicKey:
		key, err := btcec.ParsePubKey(pb.Raw, btcec.S256())
		if err != nil {
			return err
		}
		*p = *key
	case *pocec.PublicKey:
		key, err := pocec.ParsePubKey(pb.Raw, pocec.S256())
		if err != nil {
			return err
		}
		*p = *key
	}
	return nil
}

func (m *PublicKey) Bytes() []byte {
	return m.Raw
}

// NewEmptySignature returns new empty initialized proto Signature
func NewEmptySignature() *Signature {
	return &Signature{
		Raw: make([]byte, 0),
	}
}

// SignatureToProto accepts a btcec/pocec Signature, returns a proto Signature
func SignatureToProto(sig interface{}) *Signature {
	if sig == nil {
		return nil
	}
	pb := NewEmptySignature()
	switch s := sig.(type) {
	case *btcec.Signature:
		pb.Raw = s.Serialize()
	case *pocec.Signature:
		pb.Raw = s.Serialize()
	}
	return pb
}

// ProtoToSignature accepts a proto Signture and a btcec/pocec Signture,
// fills content into the latter
func ProtoToSignature(pb *Signature, sig interface{}) error {
	if pb == nil {
		return errors.New("nil proto signature")
	}
	switch s := sig.(type) {
	case *btcec.Signature:
		result, err := btcec.ParseDERSignature(pb.Raw, btcec.S256())
		if err != nil {
			return err
		}
		*s = *result
	case *pocec.Signature:
		result, err := pocec.ParseDERSignature(pb.Raw, pocec.S256())
		if err != nil {
			return err
		}
		*s = *result
	}
	return nil
}

func (m *Signature) Bytes() []byte {
	return m.Raw
}

func (m *Hash) Bytes() []byte {
	var s0, s1, s2, s3 [8]byte
	binary.LittleEndian.PutUint64(s0[:], m.S0)
	binary.LittleEndian.PutUint64(s1[:], m.S1)
	binary.LittleEndian.PutUint64(s2[:], m.S2)
	binary.LittleEndian.PutUint64(s3[:], m.S3)
	return combineBytes(s0[:], s1[:], s2[:], s3[:])
}
