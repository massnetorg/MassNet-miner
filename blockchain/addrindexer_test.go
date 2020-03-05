package blockchain

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/database"
	"massnet.org/mass/database/ldb"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

type blkhexJson struct {
	BLKHEX string
}

type server struct {
	started bool
}

func (s *server) Stop() error {
	s.started = false
	return nil
}

type logger struct {
	lastLogHeight uint64
}

func (l *logger) LogBlockHeight(blk *massutil.Block) {
	l.lastLogHeight = blk.MsgBlock().Header.Height
	fmt.Printf("log block height: %v, hash: %s\n", blk.MsgBlock().Header.Height, blk.Hash())
}

type chain struct {
	db database.Db
}

func (c *chain) GetBlockByHeight(height uint64) (*massutil.Block, error) {
	sha, err := c.db.FetchBlockShaByHeight(height)
	if err != nil {
		return nil, err
	}
	blk, err := c.db.FetchBlockBySha(sha)
	if err != nil {
		return nil, err
	}
	return blk, nil
}

func (c *chain) NewestSha() (sha *wire.Hash, height uint64, err error) {
	sha, height1, err := c.db.NewestSha()
	if err != nil {
		return nil, 0, err
	}
	height = uint64(height1)
	return sha, height, nil
}

func decodeBlockFromString(blkhex string) (*massutil.Block, error) {
	blkbuf, err := hex.DecodeString(blkhex)
	if err != nil {
		return nil, err
	}
	blk, err := massutil.NewBlockFromBytes(blkbuf, wire.Packet)
	if err != nil {
		return nil, err
	}

	return blk, nil
}

func preCommit(db database.Db, sha *wire.Hash) {
	chainDb := db.(*ldb.ChainDb)
	chainDb.Batch(0).Set(*sha)
	chainDb.Batch(0).Done()
	chainDb.Batch(1).Set(*sha)
	chainDb.Batch(1).Done()
}

func TestSubmitBlock(t *testing.T) {
	db, err := newTestChainDb()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	defer db.Close()

	blks, err := loadTopNBlk(8)
	assert.Nil(t, err)

	for i := 1; i < 8; i++ {
		err = db.SubmitBlock(blks[i])
		assert.Nil(t, err)

		blkHash := blks[i].Hash()
		preCommit(db, blkHash)

		err = db.Commit(*blkHash)
		assert.Nil(t, err)

		_, height, err := db.NewestSha()
		assert.Nil(t, err)
		assert.Equal(t, blks[i].Height(), height)
	}
}

func TestAddrIndexer(t *testing.T) {
	bc, teardown, err := newBlockChain()
	assert.Nil(t, err)
	defer teardown()

	adxr := bc.addrIndexer

	// block height is 3
	blk, err := loadNthBlk(4)
	assert.Nil(t, err)

	// error will be returned for block 1 & block2 not connected
	err = adxr.SyncAttachBlock(blk, nil)
	assert.Equal(t, errUnexpectedHeight, err)

	blks, err := loadTopNBlk(22)
	assert.Nil(t, err)

	presentScriptHash := make(map[string]struct{})
	for i := 1; i < 21; i++ {
		blk := blks[i]
		isOrphan, err := bc.processBlock(blk, BFNone)
		assert.Nil(t, err)
		assert.False(t, isOrphan)

		for _, tx := range blk.Transactions() {
			for _, txout := range tx.MsgTx().TxOut {
				class, ops := txscript.GetScriptInfo(txout.PkScript)
				_, sh, err := txscript.GetParsedOpcode(ops, class)
				assert.Nil(t, err)
				presentScriptHash[wire.Hash(sh).String()] = struct{}{}
			}
		}
	}
	assert.Equal(t, uint64(20), bc.BestBlockHeight())
	t.Log("total present script hash in first 20 blocks:", presentScriptHash)

	// index block 21
	notPresentBefore21 := [][]byte{}
	cache := make(map[string]int)
	for i, tx := range blks[21].Transactions() {
		for j, txout := range tx.MsgTx().TxOut {
			class, ops := txscript.GetScriptInfo(txout.PkScript)
			_, sh, err := txscript.GetParsedOpcode(ops, class)
			assert.Nil(t, err)
			if _, ok := presentScriptHash[wire.Hash(sh).String()]; !ok {
				if _, ok2 := cache[wire.Hash(sh).String()]; !ok2 {
					notPresentBefore21 = append(notPresentBefore21, sh[:])
					cache[wire.Hash(sh).String()] = i*10 + j
				}
			}
		}
	}
	t.Log("total only present in block 21:", cache)
	if len(notPresentBefore21) == 0 {
		t.Fatal("choose another block to continue test")
	}

	// before indexing block 21
	mp, err := bc.db.FetchScriptHashRelatedTx(notPresentBefore21, 0, 21, &config.ChainParams)
	assert.Nil(t, err)
	assert.Zero(t, len(mp))

	node := NewBlockNode(&blks[21].MsgBlock().Header, blks[21].Hash(), BFNone)
	txStore, err := bc.fetchInputTransactions(node, blks[21])
	assert.Nil(t, err)

	err = bc.db.SubmitBlock(blks[21])
	assert.Nil(t, err)
	err = adxr.SyncAttachBlock(blks[21], txStore)
	assert.Nil(t, err)
	blkHash := blks[21].Hash()
	preCommit(bc.db, blkHash)
	err = bc.db.Commit(*blkHash)
	assert.Nil(t, err)

	// after indexing block 21
	mp, err = bc.db.FetchScriptHashRelatedTx(notPresentBefore21, 0, 22, &config.ChainParams)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mp))
	// assert.Equal(t, 3, len(mp[21]))
}

func TestGetAdxr(t *testing.T) {
	db, err := newTestChainDb()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	defer db.Close()

	// genesis
	blk, err := loadNthBlk(1)
	assert.Nil(t, err)

	sha, height, err := db.FetchAddrIndexTip()
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), height)
	assert.Equal(t, blk.Hash(), sha)
}
