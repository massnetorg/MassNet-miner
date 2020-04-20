package ldb

import (
	"encoding/binary"
	"fmt"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/errors"
	"massnet.org/mass/pocec"
)

var (
	pubkblKeyPrefix = "PUBKBL"
	uppkblKey       = []byte("UPPKBL")

	// prefix + pk(compressed)
	pubkblKeyPrefixLen = len(pubkblKeyPrefix)
	pubkblKeyLen       = pubkblKeyPrefixLen + 33

	// bl + blkHeight
	blHeightLen = 1 + 8
)

func makePubkblKey(publicKey *pocec.PublicKey) []byte {
	key := make([]byte, pubkblKeyLen)
	copy(key, pubkblKeyPrefix)
	copy(key[pubkblKeyPrefixLen:pubkblKeyLen], publicKey.SerializeCompressed())
	return key
}

func serializeBLHeights(bitLength uint8, blkHeight uint64) []byte {
	buf := make([]byte, blHeightLen)
	buf[0] = bitLength
	binary.LittleEndian.PutUint64(buf[1:blHeightLen], blkHeight)
	return buf
}

func deserializeBLHeights(buf []byte) []*database.BLHeight {
	count := len(buf) / blHeightLen
	blhs := make([]*database.BLHeight, count)
	for i := 0; i < count; i++ {
		blhs[i] = &database.BLHeight{
			BitLength: int(buf[i*blHeightLen]),
			BlkHeight: binary.LittleEndian.Uint64(buf[i*blHeightLen+1 : (i+1)*blHeightLen]),
		}
	}
	return blhs
}

func (db *ChainDb) insertPubkblToBatch(batch storage.Batch, publicKey *pocec.PublicKey, bitLength int, blkHeight uint64) error {
	key := makePubkblKey(publicKey)
	buf := serializeBLHeights(uint8(bitLength), blkHeight)
	v, err := db.stor.Get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			return batch.Put(key, buf)
		}
		return err
	}
	blhs := deserializeBLHeights(v)
	lastBl := blhs[len(blhs)-1].BitLength
	if bitLength < lastBl {
		return errors.New(fmt.Sprintf("insertPubkblToBatch: unexpected bl %d, last %d, height %d",
			bitLength, lastBl, blkHeight))
	}
	if bitLength > lastBl {
		v = append(v, buf...)
		return batch.Put(key, v)
	}
	return nil
}

func (db *ChainDb) removePubkblWithCheck(batch storage.Batch, publicKey *pocec.PublicKey, bitLength int, blkHeight uint64) error {
	key := makePubkblKey(publicKey)
	buf, err := db.stor.Get(key)
	if err != nil {
		return err
	}
	l := len(buf)
	bl := buf[l-blHeightLen]
	h := binary.LittleEndian.Uint64(buf[l-blHeightLen+1:])
	if int(bl) == bitLength && h == blkHeight {
		if l == blHeightLen {
			return batch.Delete(key)
		}
		v := buf[:l-blHeightLen]
		return batch.Put(key, v)
	}
	return nil
}

func (db *ChainDb) GetPubkeyBlRecord(publicKey *pocec.PublicKey) ([]*database.BLHeight, error) {
	key := makePubkblKey(publicKey)
	v, err := db.stor.Get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			empty := make([]*database.BLHeight, 0)
			return empty, nil
		}
		return nil, err
	}
	blh := deserializeBLHeights(v)
	return blh, nil
}

func (db *ChainDb) insertPubkbl(publicKey *pocec.PublicKey, bitLength int, blkHeight uint64) error {
	key := makePubkblKey(publicKey)
	buf := serializeBLHeights(uint8(bitLength), blkHeight)
	v, err := db.stor.Get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			return db.stor.Put(key, buf)
		}
		return err
	}
	blhs := deserializeBLHeights(v)
	lastBl := blhs[len(blhs)-1].BitLength
	if bitLength < lastBl {
		return errors.New(fmt.Sprintf("insertPubkbl: unexpected bl %d, last %d, height %d",
			bitLength, lastBl, blkHeight))
	}
	if bitLength > lastBl {
		v = append(v, buf...)
		return db.stor.Put(key, v)
	}
	return nil
}

func (db *ChainDb) fetchPubkblIndexProgress() (uint64, error) {
	buf, err := db.stor.Get(uppkblKey)
	if err != nil {
		if err == storage.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

func (db *ChainDb) updatePubkblIndexProgress(height uint64) error {
	value := make([]byte, 8)
	binary.LittleEndian.PutUint64(value, height)
	return db.stor.Put(uppkblKey, value)
}

func (db *ChainDb) deletePubkblIndexProgress() error {
	return db.stor.Delete(uppkblKey)
}

func (db *ChainDb) clearPubkbl() error {
	iter := db.stor.NewIterator(storage.BytesPrefix([]byte(pubkblKeyPrefix)))
	defer iter.Release()

	for iter.Next() {
		err := db.stor.Delete(iter.Key())
		if err != nil {
			return err
		}
	}
	return iter.Error()
}
