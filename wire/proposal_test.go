package wire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestProposalArea tests the ProposalArea
func TestProposalArea(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		pa := mockProposalArea()
		var wBuf bytes.Buffer
		n, err := pa.Encode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, n)
		}

		newPa := new(ProposalArea)
		m, err := newPa.Decode(&wBuf, DB)
		if err != nil {
			t.Fatal(i, err, m)
		}

		// compare pa and newPa
		if !reflect.DeepEqual(pa, newPa) {
			t.Errorf("%d, pa and newPa is not equal\n%v\n%v", i, pa, newPa)
		}

		bs, err := pa.Bytes(Plain)
		if err != nil {
			t.Fatal(i, err)
		}
		// compare size
		if size := pa.PlainSize(); (pa.PunishmentCount() == 0 && len(bs)+PlaceHolderSize != size) || (pa.PunishmentCount() != 0 && len(bs) != size) {
			t.Fatal(i, "proposalArea size incorrect", n, m, len(bs), size)
		}
	}
}
