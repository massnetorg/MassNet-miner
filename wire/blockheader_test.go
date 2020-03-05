package wire

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"
)

// TestBlockHeader tests the BlockHeader API.
func TestBlockHeader(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		header := mockHeader()
		var wBuf bytes.Buffer
		n, err := header.Encode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, n)
		}

		newHeader := new(BlockHeader)
		m, err := newHeader.Decode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, m)
		}

		// compare header and newHeader
		if !reflect.DeepEqual(header, newHeader) {
			t.Errorf("%d, header and newHeader is not equal\n%v\n%v", i, header, newHeader)
		}

		bs, err := header.Bytes(Plain)
		if err != nil {
			t.Fatal(i, err)
		}
		// compare size
		if size := header.PlainSize(); len(bs) != size {
			t.Fatal(i, "header size incorrect", n, m, len(bs), size)
		}
	}
}

func TestBlockHeader_BlockHash(t *testing.T) {
	if hash := tstGenesisHeader.BlockHash(); !hash.IsEqual(&tstGenesisHash) {
		t.Errorf("BlockHeader.BlockHash not equal, got = %v, want = %v", hash, tstGenesisHash)
	}
}

func TestBlockHeader_Quality(t *testing.T) {
	var tstGenesisQuality = big.NewInt(2406673284404964)
	if quality := tstGenesisHeader.Quality(); quality == nil || quality.Cmp(tstGenesisQuality) != 0 {
		t.Errorf("BlockHeader.Quality not equal, got = %v, want = %v", quality, tstGenesisQuality)
	}
}

func TestBlockHeader_GetChainID(t *testing.T) {
	if chainID, _ := tstGenesisHeader.GetChainID(); !chainID.IsEqual(&tstGenesisChainID) {
		t.Errorf("BlockHeader.GetChainID not equal, got = %v, want = %v", chainID, tstGenesisChainID)
	}
}

func TestBlockHeader_PoCHash(t *testing.T) {
	var tstGenesisPoCHash = mustDecodeHash("fc9d36f158ce572d49a094fecefc781dd2d553fd65fc0374f11b9a3bd2bb72fd")
	if pocHash, _ := tstGenesisHeader.PoCHash(); !pocHash.IsEqual(&tstGenesisPoCHash) {
		t.Errorf("BlockHeader.BlockHash not equal, got = %v, want = %v", pocHash, tstGenesisPoCHash)
	}
}
