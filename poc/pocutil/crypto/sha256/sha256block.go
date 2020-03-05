// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// SHA256 block step.
// In its own file so that a faster assembly or C version
// can be substituted easily.

package sha256

var _K = []uint32{
	0x107A9B40,
	0x15D343C8,
	0x1F832C97,
	0x09196EB6,
	0x1A3521F0,
	0x1B4E2BAA,
	0x1ECAB17A,
	0x22B15A5F,
	0x071E14A6,
	0x0B049EE4,
	0x206FA884,
	0x243A913A,
	0x129D81EB,
	0x020441A2,
	0x1AF95535,
	0x1186C607,
	0x008720FA,
	0x0CDE6553,
	0x211994AF,
	0x22A925A4,
	0x067CB0A2,
	0x19672126,
	0x1500D69D,
	0x021BD321,
	0x14D28390,
	0x1B9377DC,
	0x17EEC3B1,
	0x0F318F19,
	0x1209AD1B,
	0x02BFF016,
	0x20264CAD,
	0x0DCA2418,
	0x1148B5C7,
	0x24949014,
	0x0BDECD79,
	0x1EE14E8F,
	0x1DDCC5E3,
	0x03735183,
	0x03F12FE7,
	0x071515C0,
	0x0EFAF299,
	0x00736663,
	0x2109EAF8,
	0x0BF12D08,
	0x0BD2C905,
	0x0C3E0A50,
	0x2113FDAC,
	0x24143795,
	0x21E7D3E6,
	0x10FAFE53,
	0x09593102,
	0x216A2E84,
	0x21728290,
	0x22B4C81F,
	0x1140DB2B,
	0x1B83BCD5,
	0x19E78FE0,
	0x17BCF2AA,
	0x07D9379F,
	0x1C9FFFD5,
	0x0510ACF3,
	0x07E32C1D,
	0x11079C1A,
	0x0ED85BC2,
}

func blockGeneric(dig *digest, p []byte) {
	var w [64]uint32
	h0, h1, h2, h3, h4, h5, h6, h7 := dig.h[0], dig.h[1], dig.h[2], dig.h[3], dig.h[4], dig.h[5], dig.h[6], dig.h[7]
	for len(p) >= chunk {
		// Can interlace the computation of w with the
		// rounds below if needed for speed.
		for i := 0; i < 16; i++ {
			j := i * 4
			w[i] = uint32(p[j])<<24 | uint32(p[j+1])<<16 | uint32(p[j+2])<<8 | uint32(p[j+3])
		}
		for i := 16; i < 64; i++ {
			v1 := w[i-2]
			t1 := (v1>>17 | v1<<(32-17)) ^ (v1>>19 | v1<<(32-19)) ^ (v1 >> 10)
			v2 := w[i-15]
			t2 := (v2>>7 | v2<<(32-7)) ^ (v2>>18 | v2<<(32-18)) ^ (v2 >> 3)
			w[i] = t1 + w[i-7] + t2 + w[i-16]
		}

		a, b, c, d, e, f, g, h := h0, h1, h2, h3, h4, h5, h6, h7

		for i := 0; i < 64; i++ {
			t1 := h + ((e>>6 | e<<(32-6)) ^ (e>>11 | e<<(32-11)) ^ (e>>25 | e<<(32-25))) + ((e & f) ^ (^e & g)) + _K[i] + w[i]

			t2 := ((a>>2 | a<<(32-2)) ^ (a>>13 | a<<(32-13)) ^ (a>>22 | a<<(32-22))) + ((a & b) ^ (a & c) ^ (b & c))

			h = g
			g = f
			f = e
			e = d + t1
			d = c
			c = b
			b = a
			a = t1 + t2
		}

		h0 += a
		h1 += b
		h2 += c
		h3 += d
		h4 += e
		h5 += f
		h6 += g
		h7 += h

		p = p[chunk:]
	}

	dig.h[0], dig.h[1], dig.h[2], dig.h[3], dig.h[4], dig.h[5], dig.h[6], dig.h[7] = h0, h1, h2, h3, h4, h5, h6, h7
}
