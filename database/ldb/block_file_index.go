package ldb

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"massnet.org/mass/database/disk"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/massutil"
)

var (
	// value is 4-bytes number of blkXXXXX.dat
	// LittleEndian
	latestBlockFileNumKey = []byte("fblatest")

	// meta of each blkXXXXX.dat, 'XXXXX' is number of file
	// LittleEndian
	//
	// key is 6 bytes:
	//      [0:2]   - blockFilePrefix
	//		[2:6] 	- number of file
	// value's structure is:
	// 		[0:4] 	- number of file
	//      [4:8] 	- number of valid blocks in this file
	//		[8:16]  - total size of valid blocks
	//		[16:24] - lowest block
	//		[24:32] - highest block
	//		[32:40] - timestamp of lowest block
	//		[40:48] - timestamp of highest block
	blockFilePrefix = []byte("fb")
)

func putRawBlockIndex(batch storage.Batch, blk *massutil.Block, blkFile *disk.BlockFile, offset, blkSize int64) error {
	sha := blk.Hash()

	blkShaKey := makeBlockShaKey(sha)
	blkHgtKey := makeBlockHeightKey(blk.Height())

	err := batch.Put(blkShaKey, blkHgtKey[len(blkHgtKey)-8:])
	if err != nil {
		return err
	}

	blkHgtValue := make([]byte, len(sha)+4+8+8)
	copy(blkHgtValue[0:], sha[:])
	binary.LittleEndian.PutUint32(blkHgtValue[len(sha):], blkFile.Number())
	binary.LittleEndian.PutUint64(blkHgtValue[len(sha)+4:], uint64(offset))
	binary.LittleEndian.PutUint64(blkHgtValue[len(sha)+12:], uint64(blkSize))

	err = batch.Put(blkHgtKey, blkHgtValue)
	if err != nil {
		return err
	}

	return putLatestBlockFileMeta(batch, blkFile.Bytes())
}

// putLatestBlockFileMeta saves meta about lastest block file
func putLatestBlockFileMeta(batch storage.Batch, meta []byte) error {
	if len(meta) != 48 {
		return ErrInvalidBlockFileMeta
	}
	key := append(blockFilePrefix, meta[0:4]...)
	err := batch.Put(key, meta)
	if err != nil {
		return err
	}
	return batch.Put(latestBlockFileNumKey, meta[0:4])
}

// getLastestBlockFileNum returns latest block file number
func (db *ChainDb) getLastestBlockFileNum() (uint32, error) {
	value, err := db.stor.Get(latestBlockFileNumKey)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}

func (db *ChainDb) initBlockFileMeta() ([]byte, error) {
	var zeroNum [4]byte
	err := db.stor.Put(latestBlockFileNumKey, zeroNum[:])
	if err != nil {
		return nil, err
	}
	var file0 [48]byte
	key := append(blockFilePrefix, file0[0:4]...)
	return file0[:], db.stor.Put(key, file0[:])
}

func (db *ChainDb) getAllBlockFileMeta() ([][]byte, error) {

	metas := make([][]byte, 0)

	total := uint32(0)
	iter := db.stor.NewIterator(storage.BytesPrefix(blockFilePrefix))
	defer iter.Release()
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// meta
		if len(key) == 6 {
			if len(value) != 48 {
				return nil, ErrIncorrectValueLength
			}
			if !bytes.Equal(key[2:6], value[0:4]) {
				return nil, ErrIncorrectValue
			}
			metas = append(metas, value)
			continue
		}

		// counter
		if bytes.Equal(key, latestBlockFileNumKey) {
			if len(value) != 4 {
				return nil, ErrIncorrectValueLength
			}
			total = binary.LittleEndian.Uint32(value) + 1
			continue
		}
		return nil, fmt.Errorf("unknown block file key found: %v", key)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	if int(total) != len(metas) {
		return nil, fmt.Errorf("fb: expect %d entries(actual %d)", total, len(metas))
	}
	return metas, nil
}
