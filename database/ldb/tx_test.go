package ldb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database"
	"massnet.org/mass/wire"
)

func fetchBlockTx(db database.Db, height uint64) (*wire.Hash, error) {
	sha, err := db.FetchBlockShaByHeight(height)
	if err != nil {
		return nil, err
	}
	blk, err := db.FetchBlockBySha(sha)
	if err != nil {
		return nil, err
	}
	hash, err := blk.TxHash(0)
	if err != nil {
		return nil, err
	}
	return hash, nil
}

func TestLevelDb_FetchTxByShaList(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 10)
	assert.Nil(t, err)

	hash1, err := fetchBlockTx(db, 1)
	assert.Nil(t, err)
	hash2, err := fetchBlockTx(db, 2)
	assert.Nil(t, err)
	hash3, err := fetchBlockTx(db, 3)
	assert.Nil(t, err)

	var txList []*wire.Hash
	txList = append(txList, hash1)
	txList = append(txList, hash2)
	txList = append(txList, hash3)
	txList = append(txList, hash1)
	txReply := db.FetchTxByShaList(txList)
	assert.Equal(t, 4, len(txReply))
	for i, hash := range txList {
		assert.Equal(t, hash, txReply[i].Sha)
	}
}

func TestLevelDb_FetchUnSpentTxByShaList(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 10)
	assert.Nil(t, err)

	hash1, err := fetchBlockTx(db, 1)
	assert.Nil(t, err)
	hash2, err := fetchBlockTx(db, 2)
	assert.Nil(t, err)
	hash3, err := fetchBlockTx(db, 3)
	assert.Nil(t, err)

	var txList []*wire.Hash
	txList = append(txList, hash1)
	txList = append(txList, hash2)
	txList = append(txList, hash3)
	txList = append(txList, hash2)
	txReply := db.FetchUnSpentTxByShaList(txList)
	assert.Equal(t, 4, len(txReply))
	for i, hash := range txList {
		assert.Equal(t, hash, txReply[i].Sha)
	}
}

func TestLevelDb_FetchTxBySha(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 10)
	assert.Nil(t, err)

	hash1, err := fetchBlockTx(db, 1)
	assert.Nil(t, err)
	txReply, err := db.FetchTxBySha(hash1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(txReply))
	assert.Equal(t, txReply[0].Sha, hash1)
}

func TestLevelDb_ExistsTxSha(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 10)
	assert.Nil(t, err)

	blockHash, _, err := db.NewestSha()
	assert.Nil(t, err)

	exist, err := db.ExistsSha(blockHash)
	assert.Nil(t, err)
	assert.True(t, exist)
}

// func TestLevelDb_DeleteAddrIndex(t *testing.T) {
// 	db, tearDown, err := GetDb("DbTest")
// 	assert.Nil(t, err)
// 	defer tearDown()

// 	err = initBlocks(db, 10)
// 	assert.Nil(t, err)

// 	blk, err := loadNthBlk(10)
// 	assert.Nil(t, err)
// 	assert.Equal(t, uint64(9), blk.Height())
// 	blksha := blk.Hash()

// 	hash, err := fetchBlockTx(db, 9)
// 	assert.Nil(t, err)

// 	err = db.DeleteAddrIndex(blksha, 9)
// 	assert.Nil(t, err)

// 	txReply, err := db.FetchTxBySha(hash)
// 	assert.Nil(t, err)
// 	assert.Equal(t, 1, len(txReply))
// 	assert.Equal(t, txReply[0].Sha, hash)
// }
