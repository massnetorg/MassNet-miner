package wire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestTx tests the MsgTx API.
func TestTx(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		tx := mockTx()
		var wBuf bytes.Buffer
		n, err := tx.Encode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, n)
		}

		newTx := new(MsgTx)
		m, err := newTx.Decode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, m)
		}

		// compare tx and newTx
		if !reflect.DeepEqual(tx, newTx) {
			t.Error("tx and newTx is not equal")
		}

		bs, err := tx.Bytes(Plain)
		if err != nil {
			t.Fatal(i, err)
		}
		// compare size
		if size := tx.PlainSize(); len(bs) != size {
			t.Fatal(i, "tx size incorrect", n, m, len(bs), size)
		}
	}
}

// TestTxOverflowErrors performs tests to ensure deserializing transactions
// which are intentionally crafted to use large values for the variable number
// of inputs and outputs are handled properly.  This could otherwise potentially
// be used as an attack vector.
func TestTxOverflowErrors(t *testing.T) {
	// Use protocol version 70001 and transaction version 1 specifically
	// here instead of the latest values because the test data is using
	// bytes encoded with those versions.
	pver := uint32(70001)
	txVer := uint32(1)

	tests := []struct {
		tx      MsgTx
		version uint32 // Transaction version
		err     error  // Expected error
	}{
		// Transaction that claims to have ~uint64(0) inputs.
		{
			MsgTx{
				Version: 1,
			}, txVer, nil,
		},

		// Transaction that claims to have ~uint64(0) outputs.
		{
			MsgTx{
				Version: 1,
			}, txVer, nil,
		},

		// Transaction that has an input with a signature script that
		// claims to have ~uint64(0) length.
		{
			MsgTx{
				Version: 1,
				TxIn: []*TxIn{
					{
						PreviousOutPoint: OutPoint{
							Hash: Hash{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
							Index: 0xffffffff},
					},
				},
			}, txVer, nil,
		},

		// Transaction that has an output with a public key script
		// that claims to have ~uint64(0) length.
		{
			MsgTx{
				Version: 1,
				TxIn: []*TxIn{
					{
						PreviousOutPoint: OutPoint{
							Hash: Hash{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
								0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
							Index: 0xffffffff},
						Sequence: 0xffffffff,
					},
				},
				TxOut: []*TxOut{
					{
						Value: 0x7fffffffffffffff,
					},
				},
			}, pver, nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var tx MsgTx
		var wBuf bytes.Buffer
		_, err := tx.Encode(&wBuf, DB)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Decode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}

		// Decode from wire format.
		err = tx.SetBytes(wBuf.Bytes(), DB)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}
	}
}
