package memdb_test

import (
	"math"
	"reflect"
	"testing"

	"massnet.org/mass/database/ldb"

	"massnet.org/mass/database/storage"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"massnet.org/mass/config"
	"massnet.org/mass/database/memdb"

	"massnet.org/mass/database"
	"massnet.org/mass/massutil"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

// TestClosed ensure calling the interface functions on a closed database
// returns appropriate errors for the interface functions that return errors
// and does not panic or otherwise misbehave for functions which do not return
// errors.
func TestClosed(t *testing.T) {
	db, err := memdb.NewMemDb()
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	// err = db.SubmitBlock(massutil.NewBlock(config.ChainParams.GenesisBlock))
	err = db.InitByGenesisBlock(massutil.NewBlock(config.ChainParams.GenesisBlock))
	if err != nil {
		t.Errorf("InsertBlock: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("Close: unexpected error %v", err)
	}

	genesisHash := config.ChainParams.GenesisHash
	if err := db.DeleteBlock(genesisHash); err != leveldb.ErrClosed {
		t.Errorf("DeleteBlock: unexpected error %v", err)
	}

	if _, err := db.ExistsSha(genesisHash); err != leveldb.ErrClosed {
		t.Errorf("ExistsSha: Unexpected error: %v", err)
	}

	if _, err := db.FetchBlockBySha(genesisHash); err != leveldb.ErrClosed {
		t.Errorf("FetchBlockBySha: unexpected error %v", err)
	}

	if _, err := db.FetchBlockShaByHeight(0); err != leveldb.ErrClosed {
		t.Errorf("FetchBlockShaByHeight: unexpected error %v", err)
	}

	if _, err := db.FetchHeightRange(0, 1); err != leveldb.ErrClosed {
		t.Errorf("FetchHeightRange: unexpected error %v", err)
	}

	genesisCoinbaseTx := config.ChainParams.GenesisBlock.Transactions[0]
	coinbaseHash := genesisCoinbaseTx.TxHash()
	if _, err := db.ExistsTxSha(&coinbaseHash); err != leveldb.ErrClosed {
		t.Errorf("ExistsTxSha: unexpected error %v", err)
	}

	if _, err := db.FetchTxBySha(genesisHash); err != leveldb.ErrClosed {
		t.Errorf("FetchTxBySha: unexpected error %v", err)
	}

	requestHashes := []*wire.Hash{genesisHash}
	reply := db.FetchTxByShaList(requestHashes)
	if len(reply) != len(requestHashes) {
		t.Errorf("FetchUnSpentTxByShaList unexpected number of replies "+
			"got: %d, want: %d", len(reply), len(requestHashes))
	}
	for i, txLR := range reply {
		wantReply := &database.TxReply{
			Sha:     requestHashes[i],
			Err:     leveldb.ErrClosed,
			TxSpent: make([]bool, 0),
		}
		if !reflect.DeepEqual(wantReply, txLR) {
			t.Errorf("FetchTxByShaList unexpected reply\ngot: %v\n"+
				"want: %v", txLR, wantReply)
		}
	}

	reply = db.FetchUnSpentTxByShaList(requestHashes)
	if len(reply) != len(requestHashes) {
		t.Errorf("FetchUnSpentTxByShaList unexpected number of replies "+
			"got: %d, want: %d", len(reply), len(requestHashes))
	}
	for i, txLR := range reply {
		wantReply := &database.TxReply{
			Sha:     requestHashes[i],
			Err:     leveldb.ErrClosed,
			TxSpent: make([]bool, 0),
		}
		if !reflect.DeepEqual(wantReply, txLR) {
			t.Errorf("FetchUnSpentTxByShaList unexpected reply\n"+
				"got: %v\nwant: %v", txLR, wantReply)
		}
	}

	// if _, _, err := db.NewestSha(); err != leveldb.ErrClosed {
	// 	t.Errorf("NewestSha: unexpected error %v", err)
	// }

	// if err := db.Sync(); err != leveldb.ErrClosed {
	// 	t.Errorf("Sync: unexpected error %v", err)
	// }

	if err := db.RollbackClose(); err != leveldb.ErrClosed {
		t.Errorf("RollbackClose: unexpected error %v", err)
	}

	if err := db.Close(); err != leveldb.ErrClosed {
		t.Errorf("Close: unexpected error %v", err)
	}
}

// test empty function
func TestMemDb(t *testing.T) {
	db, err := memdb.NewMemDb()
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}

	// test empty function
	// banlist.go test
	fpk, height, err := db.FetchFaultPkBySha(&wire.Hash{})
	assert.Equal(t, storage.ErrNotFound, err)
	assert.Equal(t, uint64(0), height)
	assert.Nil(t, fpk)

	fpks, err := db.FetchFaultPkListByHeight(0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fpks))

	exists, err := db.ExistsFaultPk(&wire.Hash{})
	assert.False(t, exists)
	assert.Nil(t, err)

	fpks, heights, err := db.FetchAllFaultPks()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fpks))
	assert.Equal(t, 0, len(heights))

	// punishment.go test
	fpks, err = db.FetchAllPunishment()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fpks))

	privKey, err := pocec.NewPrivateKey(pocec.S256())
	assert.Nil(t, err)
	exists, err = db.ExistsPunishment(privKey.PubKey())
	assert.False(t, exists)
	assert.Nil(t, err)

	fpk = wire.NewEmptyFaultPubKey()
	h0 := wire.NewEmptyBlockHeader()
	h0.PubKey = privKey.PubKey()
	h1 := wire.NewEmptyBlockHeader()
	fpk.PubKey = h0.PubKey
	fpk.Testimony[0] = h0
	fpk.Testimony[1] = h1
	err = db.InsertPunishment(fpk)
	assert.Nil(t, err)

	exists, err = db.ExistsPunishment(privKey.PubKey())
	assert.True(t, exists)
	assert.Nil(t, err)

	// memdb.go test
	db.FetchAddrIndexTip()
	db.SubmitAddrIndex(&wire.Hash{}, 0, &database.AddrIndexData{})
	db.DeleteAddrIndex(&wire.Hash{}, 0)

	err = db.InitByGenesisBlock(massutil.NewBlock(config.ChainParams.GenesisBlock))
	if err != nil {
		t.Errorf("InsertBlock: %v", err)
	}

	genesisHash := config.ChainParams.GenesisBlock.BlockHash()

	if _, err := db.ExistsSha(&genesisHash); err != nil {
		t.Errorf("ExistsSha: Unexpected error: %v", err)
	}
	if _, err := db.FetchBlockBySha(&genesisHash); err != nil {
		t.Errorf("FetchBlockBySha: unexpected error %v", err)
	}
	if _, err := db.FetchBlockHeightBySha(&genesisHash); err != nil {
		t.Errorf("FetchBlockShaByHeight: unexpected error %v", err)
	}
	if _, err := db.FetchBlockShaByHeight(0); err != nil {
		t.Errorf("FetchBlockShaByHeight: unexpected error %v", err)
	}
	if blkHashes, err := db.FetchHeightRange(0, 1); err != nil {
		t.Errorf("FetchHeightRange: unexpected error %v", err)
	} else {
		assert.Equal(t, 1, len(blkHashes))
	}
	if err := db.DeleteBlock(&genesisHash); err != nil {
		t.Errorf("DeleteBlock: unexpected error %v", err)
	}
	// delete
	chainDb := db.(*ldb.ChainDb)
	chainDb.Batch(1).Set(genesisHash)
	if err := db.Commit(genesisHash); err != nil {
		t.Errorf("Commit DeleteBlock: unexpected error %v", err)
	}
	hs, ht, err := db.NewestSha()
	assert.Nil(t, err)
	assert.Equal(t, &wire.Hash{}, hs)
	assert.Equal(t, uint64(math.MaxUint64), ht)

	db.Close()

}
