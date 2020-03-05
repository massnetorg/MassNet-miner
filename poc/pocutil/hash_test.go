package pocutil_test

import (
	"errors"
	"testing"

	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/testutil"
)

func TestMASSHash_String(t *testing.T) {
	var h = pocutil.MASSSHA256([]byte("TestMASSHash_String"))
	var testRound = 10000

	for i := 0; i < testRound; i++ {
		h = pocutil.MASSSHA256(h[:])
		str := h.String()
		if h != mustDecodeStringToHash(str) {
			t.Error("Hash String decode error")
		}
	}

	for i := 0; i < testRound; i++ {
		h = pocutil.MASSDoubleSHA256(h[:])
		str := h.String()
		if h != mustDecodeStringToHash(str) {
			t.Error("Hash String decode error")
		}
	}
}

func TestHash_String(t *testing.T) {
	var h = pocutil.SHA256([]byte("TestHash_String"))
	var testRound = 10000

	for i := 0; i < testRound; i++ {
		h = pocutil.SHA256(h[:])
		str := h.String()
		if h != mustDecodeStringToHash(str) {
			t.Error("Hash String decode error")
		}
	}

	for i := 0; i < testRound; i++ {
		h = pocutil.DoubleSHA256(h[:])
		str := h.String()
		if h != mustDecodeStringToHash(str) {
			t.Error("Hash String decode error")
		}
	}
}

func TestDecodeStringToHash(t *testing.T) {
	tests := []*struct {
		str string
		err error
	}{
		{
			str: "0123456789",
			err: pocutil.ErrInvalidHashLength,
		},
		{
			str: "01234567890123456789012345678901234567890123456789012345678901234",
			err: pocutil.ErrInvalidHashLength,
		},
		{
			str: "0123456789012345678901234567890123456789012345678901234567890123",
			err: nil,
		},
		{
			str: "g123456789012345678901234567890123456789012345678901234567890123",
			err: errors.New("encoding/hex: invalid byte: U+0067 'g'"),
		},
	}

	for i, test := range tests {
		if _, err := pocutil.DecodeStringToHash(test.str); !testutil.SameErrorString(err, test.err) {
			t.Errorf("%d, DecodeStringToHash error not match, got = %v, want = %v", i, err, test.err)
		}
	}
}

func mustDecodeStringToHash(str string) pocutil.Hash {
	h, err := pocutil.DecodeStringToHash(str)
	if err != nil {
		panic(err)
	}
	return h
}
