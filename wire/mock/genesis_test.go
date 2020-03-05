package mock

import (
	"fmt"
	"math/big"
	"testing"
)

func TestMockGenesisBlock(t *testing.T) {
	genesis := mockGenesisBlock(1557750000, big.NewInt(1<<28))
	fmt.Println(genesisToString(genesis))
}
