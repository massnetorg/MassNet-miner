package difficulty_test

import (
	"math/big"
	"testing"
	"time"

	"massnet.org/mass/consensus/difficulty"
	"massnet.org/mass/poc"
	"massnet.org/mass/wire"
)

func TestCalcNextRequiredDifficulty(t *testing.T) {
	tests := []struct {
		lastTarget     *big.Int
		lastSlot       int64
		nextSlot       int64
		expectedTarget *big.Int
	}{
		{
			big.NewInt(2048),
			10000,
			10001,
			big.NewInt(2049),
		},
		{
			big.NewInt(2048),
			10000,
			10009,
			big.NewInt(2049),
		},
		{
			big.NewInt(2048),
			10000,
			10010,
			big.NewInt(2048),
		},
		{
			big.NewInt(2048),
			10000,
			10019,
			big.NewInt(2048),
		},
		{
			big.NewInt(2048),
			10000,
			10020,
			big.NewInt(2047),
		},
		{
			big.NewInt(2048),
			10000,
			11010,
			big.NewInt(1948),
		},
		{
			big.NewInt(2048),
			10000,
			11999,
			big.NewInt(1850),
		},
		{
			big.NewInt(2048),
			10000,
			12000,
			big.NewInt(1849),
		},
		{
			big.NewInt(2048),
			10000,
			12009,
			big.NewInt(1849),
		},
		{
			big.NewInt(2048),
			10000,
			12010,
			big.NewInt(1849),
		},
		{
			big.NewInt(2048),
			10000,
			12011,
			big.NewInt(1849),
		},
	}

	for i, test := range tests {
		lastHeader := &wire.BlockHeader{
			Timestamp: time.Unix(test.lastSlot*poc.PoCSlot, 0),
			Target:    test.lastTarget,
		}
		target, err := difficulty.CalcNextRequiredDifficulty(lastHeader, time.Unix(test.nextSlot*poc.PoCSlot, 0))
		if err != nil {
			t.Error(i, "fail to calc next target", err)
		}
		if target.Cmp(test.expectedTarget) != 0 {
			t.Error(i, "get unexpected target", "expected", test.expectedTarget, "got", target)
		}
	}
}
