package pocutil_test

import (
	"testing"

	"massnet.org/mass/poc/pocutil"
)

func TestP(t *testing.T) {
	var pubKeyHash = mustDecodeStringToHash("c6d5ec6c5e2cf5068168aacd574301958bf276ca2ba718b35b4000e12dfea741")
	tests := []*struct {
		x  pocutil.PoCValue
		y  pocutil.PoCValue
		bl int
	}{
		{
			x:  0x0,
			y:  0xcfdf10,
			bl: 24,
		},
		{
			x:  0x1,
			y:  0x3bc4a1,
			bl: 24,
		},
		{
			x:  0xffffff,
			y:  0x968037,
			bl: 24,
		},
		{
			x:  0x1ffffff,
			y:  0x968037,
			bl: 24,
		},
		{
			x:  0x0,
			y:  0x7cfdf10,
			bl: 28,
		},
		{
			x:  0x1,
			y:  0x53bc4a1,
			bl: 28,
		},
		{
			x:  0xfffffff,
			y:  0x9ab1678,
			bl: 28,
		},
		{
			x:  0x1fffffff,
			y:  0x9ab1678,
			bl: 28,
		},
		{
			x:  0x0,
			y:  0x4757cfdf10,
			bl: 40,
		},
		{
			x:  0x1,
			y:  0xfc753bc4a1,
			bl: 40,
		},
		{
			x:  0xffffffffff,
			y:  0xfb798acdf8,
			bl: 40,
		},
		{
			x:  0x1ffffffffff,
			y:  0xfb798acdf8,
			bl: 40,
		},
	}

	for i, test := range tests {
		if y := pocutil.P(test.x, test.bl, pubKeyHash); y != test.y {
			t.Errorf("%d, P not equal, got = 0x%x, want = 0x%x", i, y, test.y)
		}
	}
}

func TestF(t *testing.T) {
	var pubKeyHash = mustDecodeStringToHash("c6d5ec6c5e2cf5068168aacd574301958bf276ca2ba718b35b4000e12dfea741")
	tests := []*struct {
		x  pocutil.PoCValue
		xp pocutil.PoCValue
		z  pocutil.PoCValue
		bl int
	}{
		{
			x:  0x0,
			xp: 0x0,
			z:  0xa94d63,
			bl: 24,
		},
		{
			x:  0xffffff,
			xp: 0xffffff,
			z:  0x1faf24,
			bl: 24,
		},
		{
			x:  0x1ffffff,
			xp: 0x1ffffff,
			z:  0x1faf24,
			bl: 24,
		},
		{
			x:  0x0,
			xp: 0x0,
			z:  0x7a94d63,
			bl: 28,
		},
		{
			x:  0xfffffff,
			xp: 0xfffffff,
			z:  0x530e6ee,
			bl: 28,
		},
		{
			x:  0x1fffffff,
			xp: 0x1fffffff,
			z:  0x530e6ee,
			bl: 28,
		},
		{
			x:  0x0,
			xp: 0x0,
			z:  0x237a94d63,
			bl: 40,
		},
		{
			x:  0xffffffffff,
			xp: 0xffffffffff,
			z:  0x4a04c5dca,
			bl: 40,
		},
		{
			x:  0x1ffffffffff,
			xp: 0x1ffffffffff,
			z:  0x4a04c5dca,
			bl: 40,
		},
	}

	for i, test := range tests {
		if z := pocutil.F(test.x, test.xp, test.bl, pubKeyHash); z != test.z {
			t.Errorf("%d, P not equal, got = 0x%x, want = 0x%x", i, z, test.z)
		}
	}
}
