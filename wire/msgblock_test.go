package wire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestBlock tests the MsgBlock API.
func TestBlock(t *testing.T) {
	var testRound = 500

	for i := 1; i < testRound; i += 20 {
		blk := mockBlock(2000 / i)
		var wBuf bytes.Buffer
		n, err := blk.Encode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, n)
		}

		newBlk := new(MsgBlock)
		m, err := newBlk.Decode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, m, blk.Proposals.PunishmentCount(), blk.Proposals.OtherCount(), blk.Proposals.PlainSize())
		}

		// compare blk and newBlk
		if !reflect.DeepEqual(blk, newBlk) {
			t.Error("blk and newBlk is not equal")
			if !reflect.DeepEqual(blk.Header, newBlk.Header) {
				t.Error("header not equal")
			}
			if !reflect.DeepEqual(blk.Proposals, newBlk.Proposals) {
				t.Error("pa not equal")
			}
			if len(blk.Transactions) != len(newBlk.Transactions) {
				t.Error("txs not equal", len(blk.Transactions), len(newBlk.Transactions))
				for j := 0; j < len(blk.Transactions); j++ {
					if !reflect.DeepEqual(blk.Transactions[j], newBlk.Transactions[j]) {
						t.Error("tx not equal", j)
					}
				}
			}
			t.FailNow()
		}

		bs, err := blk.Bytes(Plain)
		if err != nil {
			t.Fatal(i, err)
		}
		// compare size
		if size := blk.PlainSize(); (blk.Proposals.PunishmentCount() == 0 && len(bs)+PlaceHolderSize != size) || (blk.Proposals.PunishmentCount() != 0 && len(bs) != size) {
			t.Fatal(i, "block size incorrect", n, m, len(bs), size)
		}
	}
}
