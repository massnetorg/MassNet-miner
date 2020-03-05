// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that the hashes in the standard library implement
// BinaryMarshaler, BinaryUnmarshaler,
// and lock in the current representations.

package hash_test

import (
	"bytes"
	"encoding"
	"encoding/hex"
	"testing"

	"massnet.org/mass/poc/pocutil/crypto/hash"
	"massnet.org/mass/poc/pocutil/crypto/sha256"
)

func fromHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

var marshalTests = []struct {
	name   string
	new    func() hash.Hash
	golden []byte
}{
	{"sha224", sha256.New224, fromHex("73686102031791250e3f91a89273a334a5cf8ca508de325a6316dc42fde638de31266102c0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f80000000000000000000000000000f9")},
	{"sha256", sha256.New, fromHex("7368610397f872e448f6e23b1457c93a01271ad204824cf2af32530910eebdbe4b6d2faac0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f80000000000000000000000000000f9")},
}

func TestMarshalHash(t *testing.T) {
	for _, tt := range marshalTests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 256)
			for i := range buf {
				buf[i] = byte(i)
			}

			h := tt.new()
			h.Write(buf[:256])
			sum := h.Sum(nil)

			h2 := tt.new()
			h3 := tt.new()
			const split = 249
			for i := 0; i < split; i++ {
				h2.Write(buf[i : i+1])
			}
			h2m, ok := h2.(encoding.BinaryMarshaler)
			if !ok {
				t.Fatalf("Hash does not implement MarshalBinary")
			}
			enc, err := h2m.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}
			if !bytes.Equal(enc, tt.golden) {
				t.Errorf("MarshalBinary = %x, want %x", enc, tt.golden)
			}
			h3u, ok := h3.(encoding.BinaryUnmarshaler)
			if !ok {
				t.Fatalf("Hash does not implement UnmarshalBinary")
			}
			if err := h3u.UnmarshalBinary(enc); err != nil {
				t.Fatalf("UnmarshalBinary: %v", err)
			}
			h2.Write(buf[split:])
			h3.Write(buf[split:])
			sum2 := h2.Sum(nil)
			sum3 := h3.Sum(nil)
			if !bytes.Equal(sum2, sum) {
				t.Fatalf("Sum after MarshalBinary = %x, want %x", sum2, sum)
			}
			if !bytes.Equal(sum3, sum) {
				t.Fatalf("Sum after UnmarshalBinary = %x, want %x", sum3, sum)
			}
		})
	}
}
