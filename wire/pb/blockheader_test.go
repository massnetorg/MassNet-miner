package wirepb

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

// TestBlockHeader tests encode/decode blockHeader.
func TestBlockHeader(t *testing.T) {
	var testRound = 100

	for i := 0; i < testRound; i++ {
		header := mockHeader()
		buf, err := proto.Marshal(header)
		if err != nil {
			t.Fatal(err)
		}

		newHeader := new(BlockHeader)
		err = proto.Unmarshal(buf, newHeader)
		if err != nil {
			t.Fatal(err)
		}

		// compare header and newHeader
		if !proto.Equal(header, newHeader) {
			t.Error("header and newHeader is not equal")
		}
	}
}
