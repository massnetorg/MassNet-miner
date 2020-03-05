package wirepb

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

// TestBlock tests encode/decode blocks with different size.
func TestBlock(t *testing.T) {
	var maxTxCount = 20000
	var step = 1000 // txCount increment step

	for txCount := 0; txCount <= maxTxCount; txCount += step {
		block := mockBlock(txCount)
		buf, err := proto.Marshal(block)
		if err != nil {
			t.Fatal(err)
		}

		newBlock := new(Block)
		err = proto.Unmarshal(buf, newBlock)
		if err != nil {
			t.Fatal(err)
		}

		// compare block and newBlock
		if !proto.Equal(block, newBlock) {
			t.Error("block and newBlock is not equal")
		}
	}
}
