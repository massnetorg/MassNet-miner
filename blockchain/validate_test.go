package blockchain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
)

// TestCheckConnectBlock tests the CheckConnectBlock function to ensure it
// fails
func TestCheckConnectBlock(t *testing.T) {
	db, err := newTestChainDb()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	// Create a new database and chain instance to run tests against.
	chain, err := newTestBlockchain(db, "testdata")
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}

	// The genesis block should fail to connect since it's already
	// inserted.
	blk0, err := loadNthBlk(1)
	assert.Nil(t, err)
	genesisHash := blk0.Hash()
	err = chain.checkConnectBlock(NewBlockNode(&blk0.MsgBlock().Header, genesisHash, BFNone), blk0)
	assert.Equal(t, ErrConnectGenesis, err)

	blk1, err := loadNthBlk(2)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), blk1.Height())
	blk1Hash := blk1.Hash()
	err = chain.checkConnectBlock(NewBlockNode(&blk1.MsgBlock().Header, blk1Hash, BFNone), blk1)
	assert.Nil(t, err)
}

// TestCheckBlockSanity tests the CheckBlockSanity function to ensure it works
// as expected.
func TestCheckBlockSanity(t *testing.T) {
	pocLimit := config.ChainParams.PocLimit
	block, err := loadNthBlk(22)
	if err != nil {
		t.Fatal(err)
	}

	blk0, err := loadNthBlk(1)
	assert.Nil(t, err)

	err = CheckBlockSanity(block, blk0.MsgBlock().Header.ChainID, pocLimit)
	assert.Nil(t, err)

	// Ensure a block that has a timestamp with a precision higher than one
	// second fails.
	timestamp := block.MsgBlock().Header.Timestamp
	block.MsgBlock().Header.Timestamp = timestamp.Add(time.Nanosecond)
	err = CheckBlockSanity(block, blk0.MsgBlock().Header.ChainID, pocLimit)
	assert.Equal(t, ErrInvalidTime, err)
}

// TestCheckSerializedHeight tests the checkSerializedHeight function with
// various serialized heights and also does negative tests to ensure errors
// and handled properly.
func TestCheckSerializedHeight(t *testing.T) {

	tests := []struct {
		name       string
		blkNth     int    // block index in blks50.dat
		wantHeight uint64 // Expected height
		err        error  // Expected error type
	}{
		{
			"height 1",
			2, 1, nil,
		},
		{
			"height 21",
			22, 21, nil,
		},
		{
			"height 25",
			26, 25, nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block, err := loadNthBlk(test.blkNth)
			if err != nil {
				t.Fatal(err)
			}
			coinbaseTx := block.MsgBlock().Transactions[0]
			tx := massutil.NewTx(coinbaseTx)

			err = TstCheckSerializedHeight(tx, test.wantHeight)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestCalcBlockSubsidy(t *testing.T) {
	amount0 := massutil.ZeroAmount()
	amount614399, err := massutil.NewAmountFromInt(614399)
	if err != nil {
		t.Fatalf("failed to new amount 614399, %v", err)
	}
	amount614400, err := massutil.NewAmountFromInt(614400)
	if err != nil {
		t.Fatalf("failed to new amount 614400, %v", err)
	}
	amount9600000000, err := massutil.NewAmountFromInt(9600000000)
	if err != nil {
		t.Fatalf("failed to new amount 9600000000, %v", err)
	}
	amount19200000000, err := massutil.NewAmountFromInt(19200000000)
	if err != nil {
		t.Fatalf("failed to new amount 19200000000, %v", err)
	}
	amount83200000000, err := massutil.NewAmountFromInt(83200000000)
	if err != nil {
		t.Fatalf("failed to new amount 83200000000, %v", err)
	}
	tests := []struct {
		name         string
		height       uint64
		totalBinding massutil.Amount
		numRank      int
		bitLength    int
		miner        massutil.Amount
		superNode    massutil.Amount
	}{
		{
			name:         "case 1",
			height:       uint64(13440),
			totalBinding: amount614399,
			numRank:      0,
			bitLength:    24,
			miner:        amount19200000000,
			superNode:    amount0,
		},
		{
			name:         "case 2",
			height:       uint64(13441),
			totalBinding: amount614399,
			numRank:      0,
			bitLength:    24,
			miner:        amount9600000000,
			superNode:    amount0,
		},
		{
			name:         "case 3",
			height:       uint64(13440),
			totalBinding: amount614399,
			numRank:      10,
			bitLength:    24,
			miner:        amount19200000000,
			superNode:    amount83200000000,
		},
		{
			name:         "case 4",
			height:       uint64(13440),
			totalBinding: amount614400,
			numRank:      0,
			bitLength:    24,
			miner:        amount83200000000,
			superNode:    amount0,
		},
		{
			name:         "case 5",
			height:       uint64(13440),
			totalBinding: amount614400,
			numRank:      10,
			bitLength:    24,
			miner:        amount83200000000,
			superNode:    amount19200000000,
		},
		{
			name:         "case 6",
			height:       uint64(13440),
			totalBinding: amount0,
			numRank:      10,
			bitLength:    24,
			miner:        amount19200000000,
			superNode:    amount83200000000,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resMiner, resSuperNode, err := CalcBlockSubsidy(test.height, &config.ChainParams,
				test.totalBinding, test.numRank, test.bitLength)
			if err != nil {
				t.Errorf("failed to calculate block subsidy, %v", err)
			}
			// t.Logf("result: miner: %v, superNode: %v, expect: miner: %v, superNode: %v", resMiner, resSuperNode, data.miner, data.superNode)
			assert.Equal(t, resMiner.String(), test.miner.String())
			assert.Equal(t, resSuperNode.String(), test.superNode.String())
		})
	}
}
