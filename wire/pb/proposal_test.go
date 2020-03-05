package wirepb

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

func TestProposalArea(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		pa := mockProposalArea()
		buf, err := proto.Marshal(pa)
		if err != nil {
			t.Fatal(err)
		}

		newPa := new(ProposalArea)
		err = proto.Unmarshal(buf, newPa)
		if err != nil {
			t.Fatal(err)
		}

		// compare pa and newPa
		if !proto.Equal(pa, newPa) {
			t.Error("pa and newPa is not equal")
		}
	}
}
