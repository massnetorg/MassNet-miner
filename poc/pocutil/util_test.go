package pocutil_test

import (
	"bytes"
	"encoding/hex"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/pocec"
)

func TestCutBigInt(t *testing.T) {
	tests := []*struct {
		bi  *big.Int
		val pocutil.PoCValue
		bl  int
	}{
		{
			bi:  big.NewInt(0x0),
			val: 0x0,
			bl:  24,
		},
		{
			bi:  big.NewInt(0x1),
			val: 0x1,
			bl:  24,
		},
		{
			bi:  big.NewInt(0xffffff),
			val: 0xffffff,
			bl:  24,
		},
		{
			bi:  big.NewInt(0x1ffffff),
			val: 0xffffff,
			bl:  24,
		},
		{
			bi:  big.NewInt(0xfffffff),
			val: 0xfffffff,
			bl:  28,
		},
		{
			bi:  big.NewInt(0x1fffffff),
			val: 0xfffffff,
			bl:  28,
		},
		{
			bi:  new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(0x1234568)),
			val: 0x1234567,
			bl:  28,
		},
		{
			bi:  big.NewInt(0x0123456789),
			val: 0x0123456789,
			bl:  40,
		},
		{
			bi:  big.NewInt(0x10123456789),
			val: 0x0123456789,
			bl:  40,
		},
	}

	for i, test := range tests {
		if v := pocutil.CutBigInt(test.bi, test.bl); v != test.val {
			t.Errorf("%d, CutBigInt not equal, got = 0x%x, want = 0x%x", i, v, test.val)
		}
	}
}

func TestCutHash(t *testing.T) {
	tests := []*struct {
		hash pocutil.Hash
		val  pocutil.PoCValue
		bl   int
	}{
		{
			hash: pocutil.Hash{0x0},
			val:  0x0,
			bl:   24,
		},
		{
			hash: pocutil.Hash{0x1},
			val:  0x1,
			bl:   24,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff},
			val:  0xffffff,
			bl:   24,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff, 0x01},
			val:  0xffffff,
			bl:   24,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff, 0x0f},
			val:  0xfffffff,
			bl:   28,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff, 0x1f},
			val:  0xfffffff,
			bl:   28,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff, 0xff, 0xff},
			val:  0xffffffffff,
			bl:   40,
		},
		{
			hash: pocutil.Hash{0xff, 0xff, 0xff, 0xff, 0xff, 0x01},
			val:  0xffffffffff,
			bl:   40,
		},
	}

	for i, test := range tests {
		if v := pocutil.CutHash(test.hash, test.bl); v != test.val {
			t.Errorf("%d, CutHash not equal, got = 0x%x, want = 0x%x", i, v, test.val)
		}
	}
}

func TestFlipValue(t *testing.T) {
	var startBL = 20
	var endBL = 64
	var testRound = 200000

	for bl := startBL; bl <= endBL; bl += 2 {
		max := 1 << uint(bl)
		for i := 0; i < testRound; i++ {
			var v pocutil.PoCValue
			if bl == 64 {
				v = pocutil.PoCValue(rand.Uint64())
			} else {
				v = pocutil.PoCValue(rand.Intn(max))
			}
			vp := pocutil.FlipValue(v, bl)
			if v+vp != pocutil.PoCValue(max-1) {
				t.Error(i, vp)
			}
		}
	}
}

func TestPoCValue2Bytes(t *testing.T) {
	var startBL = 20
	var endBL = 64
	var testRound = 200000

	for bl := startBL; bl <= endBL; bl += 2 {
		max := 1 << uint(bl)
		for i := 0; i < testRound; i++ {
			var v pocutil.PoCValue
			if bl == 64 {
				v = pocutil.PoCValue(rand.Uint64())
			} else {
				v = pocutil.PoCValue(rand.Intn(max))
			}
			vb := pocutil.PoCValue2Bytes(v, bl)
			if v != pocutil.Bytes2PoCValue(vb, bl) {
				t.Error("cannot get equal value")
			}
		}
	}
}

func TestBytes2PoCValue(t *testing.T) {
	tests := []*struct {
		v  pocutil.PoCValue
		vb []byte
		bl int
	}{
		{
			v:  0,
			vb: nil,
			bl: 24,
		},
		{
			v:  0,
			vb: []byte{},
			bl: 24,
		},
		{
			v:  0x81,
			vb: []byte{0x81},
			bl: 24,
		},
		{
			v:  0x818181,
			vb: []byte{0x81, 0x81, 0x81},
			bl: 24,
		},
		{
			v:  0x818181,
			vb: []byte{0x81, 0x81, 0x81},
			bl: 28,
		},
		{
			v:  0x1818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81},
			bl: 28,
		},
		{
			v:  0x1818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81},
			bl: 28,
		},
		{
			v:  0x1818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81},
			bl: 28,
		},
		{
			v:  0x81818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81},
			bl: 40,
		},
		{
			v:  0x8181818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81, 0x81},
			bl: 40,
		},
		{
			v:  0x8181818181,
			vb: []byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81},
			bl: 40,
		},
	}

	for i, test := range tests {
		if result := pocutil.Bytes2PoCValue(test.vb, test.bl); result != test.v {
			t.Errorf("%d, Bytes2PoCValue not equal, got = 0x%x, want = 0x%x", i, result, test.v)
		}
	}
}

func TestNormalizePoCBytes(t *testing.T) {
	tests := []*struct {
		vb   []byte
		want []byte
		bl   int
	}{
		{
			vb:   []byte{0x11, 0x22},
			want: []byte{0x11, 0x22},
			bl:   24,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33},
			want: []byte{0x11, 0x22, 0x33},
			bl:   24,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44},
			want: []byte{0x11, 0x22, 0x33, 0x00},
			bl:   24,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33},
			want: []byte{0x11, 0x22, 0x33},
			bl:   28,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44},
			want: []byte{0x11, 0x22, 0x33, 0x04},
			bl:   28,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44, 0x55},
			want: []byte{0x11, 0x22, 0x33, 0x04, 0x00},
			bl:   28,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44},
			want: []byte{0x11, 0x22, 0x33, 0x44},
			bl:   40,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44, 0x55},
			want: []byte{0x11, 0x22, 0x33, 0x44, 0x55},
			bl:   40,
		},
		{
			vb:   []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
			want: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x00},
			bl:   40,
		},
	}
	for i, test := range tests {
		if got := pocutil.NormalizePoCBytes(test.vb, test.bl); !bytes.Equal(got, test.want) {
			t.Errorf("%d, NormalizePoCBytes not equal, got = %x, want = %x", i, got, test.want)
		}
	}
}

func TestPubKeyHash(t *testing.T) {
	pubBytes, _ := hex.DecodeString("020830127d390cd46bf48b7402c8159ded971e4fe6e1705495f95e818944ee44f9")
	pub, _ := pocec.ParsePubKey(pubBytes, pocec.S256())

	tests := []*struct {
		pubKey     *pocec.PublicKey
		pubKeyHash pocutil.Hash
	}{
		{
			pubKey:     pub,
			pubKeyHash: mustDecodeStringToHash("c6d5ec6c5e2cf5068168aacd574301958bf276ca2ba718b35b4000e12dfea741"),
		},
		{
			pubKey:     nil,
			pubKeyHash: pocutil.Hash{},
		},
	}

	for i, test := range tests {
		if hash := pocutil.PubKeyHash(test.pubKey); hash != test.pubKeyHash {
			t.Errorf("%d, PubKeyHash not equal, got = %s, want = %s", i, hash, test.pubKeyHash)
		}
	}
}
