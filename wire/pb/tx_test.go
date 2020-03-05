package wirepb

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

// TestTx tests encode/decode tx.
func TestTx(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		tx := mockTx()
		buf, err := proto.Marshal(tx)
		if err != nil {
			t.Fatal(err)
		}

		newTx := new(Tx)
		err = proto.Unmarshal(buf, newTx)
		if err != nil {
			t.Fatal(err)
		}

		// compare tx and newTx
		if !proto.Equal(tx, newTx) {
			t.Error("tx and newTx is not equal")
		}
	}
}
