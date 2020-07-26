package ldb

import (
	"bytes"
	"encoding/binary"

	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

var (
	faultPkShaDataPrefix   = []byte("BANPUB") // Sha to Data
	faultPkHeightShaPrefix = []byte("BANHGT") // Height to Sha(s)
)

func shaFaultPkToKey(sha *wire.Hash) []byte {
	key := make([]byte, len(sha)+len(faultPkShaDataPrefix))
	copy(key, faultPkShaDataPrefix)
	copy(key[len(faultPkShaDataPrefix):], sha[:])
	return key
}

func faultPkHeightToKey(blkHeight uint64) []byte {
	var b8 [8]byte
	binary.LittleEndian.PutUint64(b8[0:8], blkHeight)
	key := make([]byte, 8+len(faultPkHeightShaPrefix))
	copy(key, faultPkHeightShaPrefix)
	copy(key[len(faultPkHeightShaPrefix):], b8[:])
	return key
}

//  Structure of fault PubKey Data
//   ------------------------------------------------------------
//  | Appear Height | PubKey Bytes | Testimony 01 | Testimony 02 |
//  |---------------------------------------------|--------------
//  |    8 Byte     |    33 Byte   | HeaderLength | HeaderLength |
//   ------------------------------------------------------------

// FetchFaultPkBySha - return a banned pubKey along with corresponding testimony
func (db *ChainDb) FetchFaultPkBySha(sha *wire.Hash) (fpk *wire.FaultPubKey, height uint64, err error) {
	return db.fetchFaultPkBySha(sha)
}

// FetchFaultPkBySha - return a banned pubKey along with corresponding testimony
// Must be called with db lock held.
func (db *ChainDb) fetchFaultPkBySha(sha *wire.Hash) (fpk *wire.FaultPubKey, height uint64, err error) {
	height, data, err := db.getFaultPkData(sha)
	if err != nil {
		return
	}
	fpk, err = wire.NewFaultPubKeyFromBytes(data, wire.DB)
	return
}

func (db *ChainDb) FetchAllFaultPks() ([]*wire.FaultPubKey, []uint64, error) {

	return db.fetchAllFaultPks()
}

func (db *ChainDb) fetchAllFaultPks() ([]*wire.FaultPubKey, []uint64, error) {
	fpkList := make([]*wire.FaultPubKey, 0)
	heightList := make([]uint64, 0)
	iter := db.stor.NewIterator(storage.BytesPrefix(faultPkShaDataPrefix))
	defer iter.Release()
	for iter.Next() {
		height := binary.LittleEndian.Uint64(iter.Value()[:8])
		heightList = append(heightList, height)
		fpk, err := wire.NewFaultPubKeyFromBytes(iter.Value()[41:], wire.DB)
		if err != nil {
			return nil, nil, err
		}
		fpkList = append(fpkList, fpk)
	}
	if err := iter.Error(); err != nil {
		return nil, nil, err
	}
	return fpkList, heightList, nil
}

// FetchFaultPkListByHeight - return newly banned PubKey list on specific height
func (db *ChainDb) FetchFaultPkListByHeight(blkHeight uint64) ([]*wire.FaultPubKey, error) {
	return db.fetchFaultPkListByHeight(blkHeight)
}

// FetchFaultPkListByHeight - return newly banned PubKey list on specific height
// Must be called with db lock held.
func (db *ChainDb) fetchFaultPkListByHeight(blkHeight uint64) ([]*wire.FaultPubKey, error) {
	bufArr, err := db.getFaultPkDataByHeight(blkHeight)
	if err != nil {
		return nil, err
	}
	pkList := make([]*wire.FaultPubKey, 0, len(bufArr))
	for _, buf := range bufArr {
		fpk, err := wire.NewFaultPubKeyFromBytes(buf, wire.DB)
		if err != nil {
			return nil, err
		}
		pkList = append(pkList, fpk)
	}
	return pkList, nil
}

// getFaultPkLoc - return since which height is this PubKey banned
// Must be called with db lock held.
func (db *ChainDb) getFaultPkLoc(sha *wire.Hash) (uint64, error) {
	height, _, err := db.getFaultPkData(sha)
	return height, err
}

// Structure of height to fault PK sha List
//   -----------------------------------
//  | Total Count | PubKey Sha | ...... |
//  |-------------|------------|--------|
//  |    2 Byte   |   32 Byte  | ...... |
//   -----------------------------------
// Total Count may be Zero, thus the Length = 2 Bytes + Count * 32 Bytes

// getFaultPkDataByHeight - return FaultPkData List by height
// Must be called with db lock held.
func (db *ChainDb) getFaultPkDataByHeight(blkHeight uint64) ([][]byte, error) {
	shaList, err := db.getFaultPkShasByHeight(blkHeight)
	if err != nil {
		return nil, err
	}
	bufArr := make([][]byte, 0)
	for _, sha := range shaList {
		_, buf, err := db.getFaultPkData(sha)
		if err != nil {
			return nil, err
		}
		bufArr = append(bufArr, buf)
	}
	return bufArr, nil
}

// getFaultPkShasByHeight - return FaultPk List by height
// Must be called with db lock held.
func (db *ChainDb) getFaultPkShasByHeight(blkHeight uint64) ([]*wire.Hash, error) {
	index := faultPkHeightToKey(blkHeight)
	data, err := db.stor.Get(index)
	if err != nil {
		if err == storage.ErrNotFound {
			return nil, nil
		}
		logging.CPrint(logging.TRACE, "failed to find faultPk list on block height", logging.LogFormat{"height": blkHeight})
		return nil, err
	}

	count := binary.LittleEndian.Uint16(data[:2])

	data = data[2:]

	shaList := make([]*wire.Hash, 0, count)

	for i := uint16(0); i < count; i++ {
		sha, err := wire.NewHash(data[i*32 : (i+1)*32])
		if err != nil {
			return nil, err
		}
		shaList = append(shaList, sha)
	}
	return shaList, nil

}

// getFaultPkData - return FaultPkData by sha
// Must be called with db lock held.
func (db *ChainDb) getFaultPkData(sha *wire.Hash) (uint64, []byte, error) {
	index := shaFaultPkToKey(sha)
	faultPKData, err := db.stor.Get(index)
	if err != nil {
		logging.CPrint(logging.TRACE, "failed to find faultPk by hash", logging.LogFormat{"hash": sha.String()})
		return 0, nil, err
	}

	height := binary.LittleEndian.Uint64(faultPKData[:8])

	return height, faultPKData[41:], nil
}

// insertFaultPks - insert newly banned faultPk list on specific height
// Must be called with db lock held.
// DEPRECATED since version 1.1.0
func insertFaultPks(batch storage.Batch, blkHeight uint64, faultPks []*wire.FaultPubKey) error {
	count := len(faultPks)
	var b2 [2]byte
	binary.LittleEndian.PutUint16(b2[0:2], uint16(count))

	var shaListData bytes.Buffer
	shaListData.Write(b2[:])

	for _, fpk := range faultPks {
		sha := wire.DoubleHashH(fpk.PubKey.SerializeUncompressed())
		shaListData.Write(sha.Bytes())
		err := insertFaultPk(batch, blkHeight, fpk, &sha)
		if err != nil {
			return err
		}
	}

	if count > 0 {
		heightIndex := faultPkHeightToKey(uint64(blkHeight))
		return batch.Put(heightIndex, shaListData.Bytes())
	}
	return nil
}

// insertFaultPk - insert newly banned faultPk on specific height
// Must be called with db lock held.
func insertFaultPk(batch storage.Batch, blkHeight uint64, faultPk *wire.FaultPubKey, sha *wire.Hash) error {
	data, err := faultPk.Bytes(wire.DB)
	if err != nil {
		return err
	}

	var lh [8]byte
	binary.LittleEndian.PutUint64(lh[0:8], blkHeight)

	var buf bytes.Buffer
	buf.Write(lh[:])
	buf.Write(faultPk.PubKey.SerializeCompressed())
	buf.Write(data)

	key := shaFaultPkToKey(sha)
	return batch.Put(key, buf.Bytes())
}

func (db *ChainDb) dropFaultPksByHeight(batch storage.Batch, blkHeight uint64) error {
	shaList, err := db.getFaultPkShasByHeight(blkHeight)
	if err != nil {
		return err
	}
	for _, sha := range shaList {
		err := dropFaultPkBySha(batch, sha)
		if err != nil {
			return err
		}
	}
	if len(shaList) > 0 {
		index := faultPkHeightToKey(blkHeight)
		return batch.Delete(index)
	}
	return nil
}

func dropFaultPkBySha(batch storage.Batch, sha *wire.Hash) error {
	index := shaFaultPkToKey(sha)
	return batch.Delete(index)
}

// ExistsFaultPk - check whether a specific PubKey has been banned, returns true if banned
func (db *ChainDb) ExistsFaultPk(sha *wire.Hash) (bool, error) {
	return db.faultPkExists(sha)
}

// ExistsFaultPk - check whether a specific PubKey has been banned, returns true if banned
// Must be called with db lock held.
func (db *ChainDb) faultPkExists(sha *wire.Hash) (bool, error) {
	key := shaFaultPkToKey(sha)

	return db.stor.Has(key)
}
