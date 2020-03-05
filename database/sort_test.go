package database

import (
	"crypto/sha256"
	"testing"
)

var (
	scriptHash1 = [sha256.Size]byte{
		121, 73, 216, 97, 126, 31, 138, 156, 113, 105, 115, 246, 231, 165, 81, 239, 255, 87, 109, 52,
	}
	scriptHash2 = [sha256.Size]byte{
		167, 144, 16, 63, 0, 8, 70, 233, 100, 171, 219, 210, 238, 138, 233, 108, 68, 12, 196, 161,
	}
	scriptHash3 = [sha256.Size]byte{
		90, 218, 168, 121, 30, 157, 3, 3, 184, 70, 131, 242, 159, 111, 105, 2, 67, 193, 240, 191,
	}
	scriptHash4 = [sha256.Size]byte{
		111, 167, 223, 9, 103, 116, 23, 216, 88, 220, 163, 221, 42, 86, 148, 221, 125, 138, 218, 118,
	}
	scriptHash5 = [sha256.Size]byte{
		18, 153, 140, 48, 85, 7, 47, 11, 249, 239, 27, 194, 34, 20, 84, 130, 137, 127, 145, 252,
	}
)

func TestSortMapByValue(t *testing.T) {
	var tests = map[[sha256.Size]byte][]StakingTxInfo{
		scriptHash1: {
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    101,
			},
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    201,
			},
			{
				Value:        2000,
				FrozenPeriod: 1000,
				BlkHeight:    401,
			},
		},
		scriptHash2: {
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    101,
			},
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    201,
			},
			{
				Value:        2000,
				FrozenPeriod: 1000,
				BlkHeight:    401,
			},
		},
		scriptHash3: {
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    101,
			},
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    201,
			},
			{
				Value:        2000,
				FrozenPeriod: 1000,
				BlkHeight:    501,
			},
		},
		scriptHash4: {
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    101,
			},
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    201,
			},
			{
				Value:        2000,
				FrozenPeriod: 1000,
				BlkHeight:    501,
			},
		},
		scriptHash5: {
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    101,
			},
			{
				Value:        1000,
				FrozenPeriod: 1000,
				BlkHeight:    201,
			},
			{
				Value:        2000,
				FrozenPeriod: 1000,
				BlkHeight:    401,
			},
		},
	}
	pairs, err := SortMapByValue(tests, 900, true)
	if err != nil {
		t.Fatalf("failed to sort, %v", err)
	}
	for _, pair := range pairs {
		t.Logf("key: %v", pair.Key)
		t.Logf("value: %v", pair.Value)
		t.Logf("weight: %v", pair.Weight)
		t.Logf("")
	}
	//for i := 0; i < 500; i++ {
	//	pairsT, err := SortMapByValue(tests, 900, true)
	//	if err != nil {
	//		t.Fatalf("failed to sort, %v", err)
	//	}
	//	equal := reflect.DeepEqual(pairs, pairsT)
	//	if !equal {
	//		t.Fatalf("no equal")
	//	}
	//	t.Logf("equal")
	//}
}
