package wirepb

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/pocec"
)

// TestBigInt tests encode/decode BigInt.
func TestBigInt(t *testing.T) {
	x := new(big.Int).SetUint64(uint64(0xffffffffffffffff))
	x = x.Mul(x, x)
	pb := BigIntToProto(x)
	y := new(big.Int)
	if err := ProtoToBigInt(pb, y); err != nil {
		t.Error("proto to big int error", err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Error("obj BigInt not equal")
	}
}

// TestPublicKey test encode/decode PublicKey.
func TestPublicKey(t *testing.T) {
	btcPriv, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	btcPub := btcPriv.PubKey()
	btcProto := PublicKeyToProto(btcPub)
	btcPubNew := new(btcec.PublicKey)
	if err := ProtoToPublicKey(btcProto, btcPubNew); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(btcPub, btcPubNew) {
		t.Error("obj btcec PublicKey not equal")
	}

	pocPriv, err := pocec.NewPrivateKey(pocec.S256())
	if err != nil {
		t.Fatal(err)
	}
	pocPub := pocPriv.PubKey()
	pocProto := PublicKeyToProto(pocPub)
	pocPubNew := new(pocec.PublicKey)
	if err := ProtoToPublicKey(pocProto, pocPubNew); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(pocPub, pocPubNew) {
		t.Error("obj pocec PublicKey not equal")
	}
}

// TestSignature tests encode/decode Signature.
func TestSignature(t *testing.T) {
	btcSig := &btcec.Signature{
		R: new(big.Int).SetUint64(rand.Uint64()),
		S: new(big.Int).SetUint64(rand.Uint64()),
	}
	btcProto := SignatureToProto(btcSig)
	btcSigNew := new(btcec.Signature)
	if err := ProtoToSignature(btcProto, btcSigNew); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(btcSig, btcSigNew) {
		t.Error("obj btcec Signature not equal")
	}

	pocSig := &pocec.Signature{
		R: new(big.Int).SetUint64(rand.Uint64()),
		S: new(big.Int).SetUint64(rand.Uint64()),
	}
	pocProto := SignatureToProto(pocSig)
	pocSigNew := new(pocec.Signature)
	if err := ProtoToSignature(pocProto, pocSigNew); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(pocSig, pocSigNew) {
		t.Error("obj pocec Signature not equal")
	}
}
