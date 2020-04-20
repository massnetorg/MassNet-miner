package ldb

import (
	"encoding/binary"

	"massnet.org/mass/database/storage"
	"massnet.org/mass/pocec"
)

var (
	minedBlockIndexPrefix = []byte("MBP")

	// prefix + pk + height
	minedBlockKeyLen = 3 + 33 + 8

	minedBlockSearchKeyLen = 3 + 33
)

func minedBlockIndexToKey(pubKey *pocec.PublicKey, height uint64) []byte {
	key := make([]byte, minedBlockKeyLen)
	copy(key, minedBlockIndexPrefix)
	copy(key[3:36], pubKey.SerializeCompressed())
	binary.LittleEndian.PutUint64(key[36:44], height)
	return key
}

func minedBlockIndexSearchKey(pubKey *pocec.PublicKey) []byte {
	key := make([]byte, minedBlockSearchKeyLen)
	copy(key, minedBlockIndexPrefix)
	copy(key[3:36], pubKey.SerializeCompressed())
	return key
}

func updateMinedBlockIndex(batch storage.Batch, connecting bool, pubKey *pocec.PublicKey, height uint64) error {
	if connecting {
		return batch.Put(minedBlockIndexToKey(pubKey, height), blankData)
	} else {
		return batch.Delete(minedBlockIndexToKey(pubKey, height))
	}
}

func (db *ChainDb) FetchMinedBlocks(pubKey *pocec.PublicKey) ([]uint64, error) {
	minedHeights := make([]uint64, 0)
	keyPrefix := minedBlockIndexSearchKey(pubKey)
	iter := db.stor.NewIterator(storage.BytesPrefix(keyPrefix))
	defer iter.Release()
	for iter.Next() {
		key := iter.Key()
		height := binary.LittleEndian.Uint64(key[36:44])
		minedHeights = append(minedHeights, height)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return minedHeights, nil
}
