package ldb

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/wire"
)

func TestWriteBatch(t *testing.T) {
	stor, err := storage.CreateStorage("leveldb", "./testData/testBatch/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		assert.Nil(t, os.RemoveAll("./testData"))
	}()

	cdb, err := NewChainDb(stor)
	if err != nil {
		t.Fatal(err)
	}

	var mockBytes = func(n int) []byte {
		bs := make([]byte, n)
		rand.Read(bs)
		return bs
	}

	var mockHash = func() wire.Hash {
		var h = wire.Hash{}
		copy(h[:], mockBytes(32))
		return h
	}

	var testRound = 50
	start := time.Now()
	for i := 0; i < testRound; i++ {
		block := mockHash()

		batchA := cdb.Batch(blockBatch)
		batchA.Set(block)

		batchB := cdb.Batch(addrIndexBatch)
		batchB.Set(block)

		batchA.Batch().Put(mockBytes(8), mockBytes(800))
		batchA.Batch().Put(mockBytes(32), mockBytes(8))
		batchA.Batch().Put(mockBytes(10), mockBytes(40))
		batchA.Batch().Put(mockBytes(20), mockBytes(2))
		batchB.Batch().Put(mockBytes(40), mockBytes(40))
		batchB.Batch().Put(mockBytes(40), mockBytes(40))
		batchA.Done()

		batchB.Batch().Put(mockBytes(10), mockBytes(40))
		batchB.Batch().Put(mockBytes(40), mockBytes(1))
		batchB.Batch().Put(mockBytes(40), mockBytes(1))
		batchB.Batch().Put(mockBytes(40), mockBytes(1))
		batchB.Batch().Put(mockBytes(40), mockBytes(1))
		batchB.Done()

		if err := cdb.Commit(block); err != nil {
			t.Fatal(err)
		}
		batchA.Batch().Reset()
		batchB.Batch().Reset()

		if i%1000 == 0 {
			fmt.Println(i)
			time.Sleep(time.Second)
		}
	}
	t.Log("finished", time.Since(start))
}
