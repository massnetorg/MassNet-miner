package ldb_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database/ldb"
)

func TestPubkblRecords(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer func() {
		tearDown()
		for _, block := range blks200[:100] {
			block.MsgBlock().Header.Proof.BitLength = 24
		}
	}()

	type BH struct {
		height uint64
		bl     int
	}

	bls := []int{24, 26, 28, 30, 32, 34}

	m := make(map[string][]BH)
	for _, block := range blks200[:100] {
		block.Hash()
		i := rand.Intn(len(bls))

		pk := block.MsgBlock().Header.PubKey.SerializeCompressed()

		ex, ok := m[string(pk)]
		if !ok {
			m[string(pk)] = append(m[string(pk)], BH{
				height: block.Height(),
				bl:     block.MsgBlock().Header.Proof.BitLength,
			})
			continue
		}
		if bls[i] > ex[len(ex)-1].bl {
			block.MsgBlock().Header.Proof.BitLength = bls[i]
			m[string(pk)] = append(m[string(pk)], BH{
				height: block.Height(),
				bl:     block.MsgBlock().Header.Proof.BitLength,
			})
		} else {
			block.MsgBlock().Header.Proof.BitLength = ex[len(ex)-1].bl
		}
	}

	err = initBlocks(db, 100)
	assert.Nil(t, err)

	for _, block := range blks200[:100] {
		slice, err := db.GetPubkeyBlRecord(block.MsgBlock().Header.PubKey)
		ex := m[string(block.MsgBlock().Header.PubKey.SerializeCompressed())]
		assert.Nil(t, err)
		assert.Equal(t, len(ex), len(slice))
		for i, bl := range slice {
			assert.Equal(t, ex[i].height, bl.BlkHeight)
			assert.Equal(t, ex[i].bl, bl.BitLength)
		}
	}

	// test reindex
	err = db.IndexPubkbl(true)
	assert.Nil(t, err)
	for _, block := range blks200[:100] {
		slice, err := db.GetPubkeyBlRecord(block.MsgBlock().Header.PubKey)
		ex := m[string(block.MsgBlock().Header.PubKey.SerializeCompressed())]
		assert.Nil(t, err)
		assert.Equal(t, len(ex), len(slice))
		for i, bl := range slice {
			assert.Equal(t, ex[i].height, bl.BlkHeight)
			assert.Equal(t, ex[i].bl, bl.BitLength)
		}
	}

	// test delete
	for i := range blks200[:100] {
		err := db.DeleteBlock(blks200[99-i].Hash())
		assert.Nil(t, err, i)
		db.(*ldb.ChainDb).Batch(1).Set(*blks200[99-i].Hash())
		db.(*ldb.ChainDb).Batch(1).Done()
		err = db.Commit(*blks200[99-i].Hash())
		assert.Nil(t, err, i)
	}

	for _, block := range blks200[:100] {
		slice, err := db.GetPubkeyBlRecord(block.MsgBlock().Header.PubKey)
		assert.Nil(t, err)
		assert.Zero(t, len(slice))
	}
}
