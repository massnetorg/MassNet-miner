package ldb_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
	wirepb "massnet.org/mass/wire/pb"
)

func mockHash() wire.Hash {
	pb := new(wirepb.Hash)
	pb.S0 = rand.Uint64()
	pb.S1 = rand.Uint64()
	pb.S2 = rand.Uint64()
	pb.S3 = rand.Uint64()
	hash, _ := wire.NewHashFromProto(pb)
	return *hash
}

func NewFaultPubKey() *wire.FaultPubKey {
	priv, _ := pocec.NewPrivateKey(pocec.S256())
	fpk := wire.NewEmptyFaultPubKey()
	fpk.PubKey = priv.PubKey()

	hash0 := mockHash()
	sig0, err := priv.Sign(hash0[:])
	if err != nil {
		panic(err)
	}
	hash1 := mockHash()
	sig1, err := priv.Sign(hash1[:])
	if err != nil {
		panic(err)
	}

	fpk.Testimony[0] = wire.MockHeader()
	fpk.Testimony[0].Signature = sig0
	fpk.Testimony[0].PubKey = fpk.PubKey
	fpk.Testimony[1] = wire.MockHeader()
	fpk.Testimony[1].Signature = sig1
	fpk.Testimony[1].PubKey = fpk.PubKey
	return fpk
}
func TestLevelDb_InsertPunishment(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	if err != nil {
		t.Errorf("get Db error%v", err)
	}
	defer tearDown()
	fpk := NewFaultPubKey()
	err = db.InsertPunishment(fpk)
	assert.Nil(t, err)
}

func TestLevelDb_ExistsPunishment(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	if err != nil {
		t.Errorf("get Db error%v", err)
	}
	defer tearDown()
	fpk := NewFaultPubKey()
	err = db.InsertPunishment(fpk)
	assert.Nil(t, err)
	exist, err := db.ExistsPunishment(fpk.PubKey)
	assert.True(t, exist)
	assert.Nil(t, err)
}

func TestLevelDb_FetchAllPunishment(t *testing.T) {
	db, tearDown, err := GetDb("DbTest")
	if err != nil {
		t.Errorf("get Db error%v", err)
	}
	defer tearDown()
	fpk1 := NewFaultPubKey()
	fpk2 := NewFaultPubKey()
	err = db.InsertPunishment(fpk1)
	assert.Nil(t, err)
	err = db.InsertPunishment(fpk2)
	assert.Nil(t, err)
	fpks, err := db.FetchAllPunishment()
	assert.Equal(t, 2, len(fpks))
	assert.Nil(t, err)
}
