package ldb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/wire"
)

func TestLevelDb_FetchFaultPkBySha(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 4)
	assert.Nil(t, err)

	blk4, err := loadNthBlk(4)
	assert.Nil(t, err)
	sha, err := db.FetchBlockShaByHeight(3)
	assert.Nil(t, err)
	assert.Equal(t, sha, blk4.Hash())

	_, _, err = db.FetchFaultPkBySha(sha)
	assert.Equal(t, storage.ErrNotFound, err)

	// insert fpk
	blk5, err := loadNthBlk(5)
	assert.Nil(t, err)
	fpk := NewFaultPubKey()
	blk5.MsgBlock().Proposals.PunishmentArea = []*wire.FaultPubKey{fpk}
	err = insertBlock(db, blk5)
	assert.Nil(t, err)

	sha, err = db.FetchBlockShaByHeight(4)
	assert.Nil(t, err)
	assert.Equal(t, sha, blk5.Hash())
	fpksha := wire.DoubleHashH(fpk.PubKey.SerializeUncompressed())
	rFpk, h, err := db.FetchFaultPkBySha(&fpksha)
	assert.Nil(t, err)
	assert.Equal(t, 4, int(h))
	readBuf, err := rFpk.Bytes(wire.DB)
	assert.Nil(t, err)
	writeBuf, err := fpk.Bytes(wire.DB)
	assert.Nil(t, err)
	assert.Equal(t, readBuf, writeBuf)
}

func TestLevelDb_FetchAllFaultPks(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 4)
	assert.Nil(t, err)

	fpks, _, err := db.FetchAllFaultPks()
	assert.Nil(t, err)
	assert.Zero(t, len(fpks))

	blk5, err := loadNthBlk(5)
	assert.Nil(t, err)
	fpk := NewFaultPubKey()
	blk5.MsgBlock().Proposals.PunishmentArea = []*wire.FaultPubKey{fpk}
	err = insertBlock(db, blk5)
	assert.Nil(t, err)

	_, heights, err := db.FetchAllFaultPks()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(heights))
	assert.Equal(t, uint64(4), heights[0])
}

func TestLevelDb_FetchFaultPkListByHeight(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 4)
	assert.Nil(t, err)

	blk5, err := loadNthBlk(5)
	assert.Nil(t, err)
	fpk := NewFaultPubKey()
	blk5.MsgBlock().Proposals.PunishmentArea = []*wire.FaultPubKey{fpk}
	err = insertBlock(db, blk5)
	assert.Nil(t, err)

	fpks, err := db.FetchFaultPkListByHeight(4)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(fpks))
	readBuf, err := fpks[0].Bytes(wire.DB)
	assert.Nil(t, err)
	writeBuf, err := fpk.Bytes(wire.DB)
	assert.Nil(t, err)
	assert.Equal(t, readBuf, writeBuf)
}

func TestLevelDb_ExistsFaultPk(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	assert.Nil(t, err)
	defer tearDown()

	err = initBlocks(db, 4)
	assert.Nil(t, err)

	_, hgt, err := db.NewestSha()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), hgt)

	blk5, err := loadNthBlk(5)
	assert.Nil(t, err)

	fpk := NewFaultPubKey()
	fpksha := wire.DoubleHashH(fpk.PubKey.SerializeUncompressed())

	exist, err := db.ExistsFaultPk(&fpksha)
	assert.Nil(t, err)
	assert.False(t, exist)

	blk5.MsgBlock().Proposals.PunishmentArea = []*wire.FaultPubKey{fpk}
	err = insertBlock(db, blk5)
	assert.Nil(t, err)

	_, hgt, err = db.NewestSha()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), hgt)

	exist, err = db.ExistsFaultPk(&fpksha)
	assert.Nil(t, err)
	assert.True(t, exist)
}
