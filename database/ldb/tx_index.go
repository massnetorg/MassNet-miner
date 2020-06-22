package ldb

import (
	"crypto/sha256"
	"encoding/binary"

	"golang.org/x/crypto/ripemd160"
	"massnet.org/mass/config"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/wire"
)

const (
	addrIndexCurrentVersion = 1
)

var (
	// Address index version is required to drop/rebuild address index if version
	// is older than current as the format of the index may have changed. This is
	// true when going from no version to version 1 as the address index is stored
	// as big endian in version 1 and little endian in the original code. Version
	// is stored as two bytes, little endian (to match all the code but the index).
	addrIndexVersionKey = []byte("ADDRINDEXVERSION")
	txIndexPrefix       = []byte("STL")
	shIndexPrefix       = []byte("HTS")
	// txIndexKeyLen = len(txIndexPrefix) + len(scriptHash) + len(height) + len(txOffset) + len(txLen)
	txIndexKeyLen = 3 + sha256.Size + 8 + 4 + 4
	// txIndexSearchKeyLen = len(txIndexPrefix) + len(scriptHash) + len(height)
	txIndexSearchKeyLen = 3 + sha256.Size + 8
	usedSearchKeyLen    = 3 + sha256.Size
	txIndexDeleteKeyLen = 3 + sha256.Size + 8
	shIndexKeyLen       = 3 + 8 + sha256.Size
	shIndexSearchKeyLen = 3 + 8
	blankData           []byte
)

type txIndex struct {
	scriptHash [sha256.Size]byte
	blkHeight  uint64
	txOffset   uint32
	txLen      uint32
}

type shIndex struct {
	blkHeight  uint64
	scriptHash [sha256.Size]byte
}

func txIndexToKey(txIndex *txIndex) []byte {
	key := make([]byte, txIndexKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], txIndex.scriptHash[:])
	binary.BigEndian.PutUint64(key[35:43], txIndex.blkHeight)
	binary.LittleEndian.PutUint32(key[43:47], txIndex.txOffset)
	binary.LittleEndian.PutUint32(key[47:51], txIndex.txLen)
	return key
}

func txIndexSearchKey(scriptHash [sha256.Size]byte, blkHeight uint64) []byte {
	key := make([]byte, txIndexSearchKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], scriptHash[:])
	binary.BigEndian.PutUint64(key[35:43], blkHeight)
	return key
}

func usedSearchKey(scriptHash [sha256.Size]byte) []byte {
	key := make([]byte, usedSearchKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], scriptHash[:])
	return key
}

func txIndexDeleteKey(scriptHash [sha256.Size]byte, blkHeight uint64) []byte {
	key := make([]byte, txIndexDeleteKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], scriptHash[:])
	binary.BigEndian.PutUint64(key[35:43], blkHeight)
	return key
}

func shIndexToKey(shIndex *shIndex) []byte {
	key := make([]byte, shIndexKeyLen)
	copy(key, shIndexPrefix)
	binary.LittleEndian.PutUint64(key[3:11], shIndex.blkHeight)
	copy(key[11:43], shIndex.scriptHash[:])
	return key
}

func shIndexSearchKey(blkHeight uint64) []byte {
	key := make([]byte, shIndexSearchKeyLen)
	copy(key, shIndexPrefix)
	binary.LittleEndian.PutUint64(key[3:11], blkHeight)
	return key
}

func (db *ChainDb) SubmitAddrIndex(hash *wire.Hash, height uint64, addrIndexData *database.AddrIndexData) error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	batch := db.Batch(addrIndexBatch)
	batch.Set(*hash)

	if err := db.preSubmit(addrIndexBatch); err != nil {
		return err
	}

	if err := db.submitAddrIndex(height, addrIndexData); err != nil {
		batch.Reset()
		return err
	}

	batch.Done()
	return nil
}

func (db *ChainDb) submitAddrIndex(height uint64, addrIndexData *database.AddrIndexData) error {
	// Write all data for the new address indexes in a single batch
	var batch = db.Batch(addrIndexBatch).Batch()

	txAddrIndex, btxAddrIndex, gtxSpentIndex := addrIndexData.TxIndex, addrIndexData.BindingTxIndex, addrIndexData.BindingTxSpentIndex

	// get and update the height||sh -> []byte and sh||blkHeigtht||txOffset||txLen -> []byte   key-value
	for addrKey, indexes := range txAddrIndex {
		txShIndex := &shIndex{blkHeight: height, scriptHash: addrKey}
		key := shIndexToKey(txShIndex)
		batch.Put(key, blankData)
		//heightToScriptHash = append(heightToScriptHash, &shIndex{blkHeight: blkHeight, scriptHash: addrKey})
		for _, index := range indexes {
			txIndexKey := &txIndex{
				scriptHash: addrKey,
				blkHeight:  height,
				txOffset:   uint32(index.TxStart),
				txLen:      uint32(index.TxLen),
			}
			// The index is stored purely in the key.
			packedIndex := txIndexToKey(txIndexKey)
			batch.Put(packedIndex, blankData)
		}
	}

	for scriptHash, outpoints := range btxAddrIndex {
		gshIndex := &bindingShIndex{
			blkHeight:  height,
			scriptHash: scriptHash,
		}
		gshKey := bindingShIndexToKey(gshIndex)
		batch.Put(gshKey, blankData)
		for _, op := range outpoints {
			btxIndex := &bindingTxIndex{
				scriptHash: scriptHash,
				blkHeight:  height,
				txOffset:   uint32(op.TxLoc.TxStart),
				txLen:      uint32(op.TxLoc.TxLen),
				index:      op.Index,
			}
			btxKey := bindingTxIndexToKey(btxIndex)
			batch.Put(btxKey, blankData)
		}
	}

	for scriptHash, btxss := range gtxSpentIndex {
		delCountMap := make(map[uint64]int)
		for _, btxs := range btxss {
			// insert ["STSG" || Height(spent) || txOffset(spent) || txLen(spent) || SH || Height(btx) || txOffset(btx) || txLen(btx) || index(btx)] -> []byte
			btxsIndex := &bindingTxSpentIndex{
				scriptHash:       scriptHash,
				blkHeightSpent:   height,
				txOffsetSpent:    uint32(btxs.SpentTxLoc.TxStart),
				txLenSpent:       uint32(btxs.SpentTxLoc.TxLen),
				blkHeightBinding: btxs.BTxBlkHeight,
				txOffsetBinding:  uint32(btxs.BTxLoc.TxStart),
				txLenBinding:     uint32(btxs.BTxLoc.TxLen),
				indexBinding:     btxs.BTxIndex,
			}
			btxsKey := bindingTxSpentIndexToKey(btxsIndex)
			batch.Put(btxsKey, blankData)
			// delete ["STG" || SH || Height || txOffset || txLen || index] -> []byte
			btxIndex := &bindingTxIndex{
				scriptHash: scriptHash,
				blkHeight:  btxs.BTxBlkHeight,
				txOffset:   uint32(btxs.BTxLoc.TxStart),
				txLen:      uint32(btxs.BTxLoc.TxLen),
				index:      btxs.BTxIndex,
			}
			btxKey := bindingTxIndexToKey(btxIndex)
			batch.Delete(btxKey)

			if _, exist := delCountMap[btxs.BTxBlkHeight]; !exist {
				delCountMap[btxs.BTxBlkHeight] = 1
			} else {
				delCountMap[btxs.BTxBlkHeight]++
			}
		}

		for btxHeight, delCount := range delCountMap {
			// check whether we should delete ["HTGS", Height, SH] -> []byte
			btxDeleteIndex := bindingTxIndexDeleteKey(scriptHash, btxHeight)
			iter := db.stor.NewIterator(storage.BytesPrefix(btxDeleteIndex))
			defer iter.Release()

			btxCount := 0
			for iter.Next() {
				btxCount++
				if btxCount > delCount {
					// more than one binding transaction for this scriptHash in this block
					break
				}
			}
			if err := iter.Error(); err != nil {
				return err
			}
			if btxCount < delCount {
				return ErrBindingIndexBroken
			}
			// no binding transaction for this scriptHash in this block after deletion
			if btxCount == delCount {
				gsh := &bindingShIndex{
					blkHeight:  btxHeight,
					scriptHash: scriptHash,
				}
				gshKey := bindingShIndexToKey(gsh)
				batch.Delete(gshKey)
			}
		}
	}

	// Ensure we're writing an address index version
	newIndexVersion := make([]byte, 2)
	binary.LittleEndian.PutUint16(newIndexVersion[0:2],
		uint16(addrIndexCurrentVersion))
	batch.Put(addrIndexVersionKey, newIndexVersion)

	return nil
}

func (db *ChainDb) DeleteAddrIndex(hash *wire.Hash, height uint64) error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	batch := db.Batch(addrIndexBatch)
	batch.Set(*hash)

	if err := db.preSubmit(addrIndexBatch); err != nil {
		return err
	}

	if err := db.deleteAddrIndex(height); err != nil {
		batch.Reset()
		return err
	}

	batch.Done()
	return nil
}

func (db *ChainDb) deleteAddrIndex(height uint64) error {
	var batch = db.Batch(addrIndexBatch).Batch()

	// delete txIndex and txShIndex
	shPrefix := shIndexSearchKey(height)

	iter := db.stor.NewIterator(storage.BytesPrefix(shPrefix))
	defer iter.Release()
	for iter.Next() {
		var scriptHash [sha256.Size]byte
		shIndexKey := iter.Key()
		copy(scriptHash[:], shIndexKey[11:])
		deletePrefix := txIndexDeleteKey(scriptHash, height)
		txIter := db.stor.NewIterator(storage.BytesPrefix(deletePrefix))
		for txIter.Next() {
			txIndexKey := txIter.Key()
			batch.Delete(txIndexKey)
		}
		txIter.Release()
		if err := txIter.Error(); err != nil {
			return err
		}
		batch.Delete(shIndexKey)
	}
	if err := iter.Error(); err != nil {
		return err
	}

	// delete bindingTxIndex and bindingShIndex
	btxShPrefix := bindingShIndexSearchKey(height)

	btxIter := db.stor.NewIterator(storage.BytesPrefix(btxShPrefix))
	defer btxIter.Release()
	for btxIter.Next() {
		var scriptHash [ripemd160.Size]byte
		shIndexKey := btxIter.Key()
		copy(scriptHash[:], shIndexKey[bindingShIndexSearchKeyLen:])
		deletePrefix := bindingTxIndexDeleteKey(scriptHash, height)
		deleteGtxIter := db.stor.NewIterator(storage.BytesPrefix(deletePrefix))
		for deleteGtxIter.Next() {
			btxIndexKey := deleteGtxIter.Key()
			batch.Delete(btxIndexKey)
		}
		deleteGtxIter.Release()
		if err := deleteGtxIter.Error(); err != nil {
			return err
		}
		batch.Delete(shIndexKey)
	}
	if err := btxIter.Error(); err != nil {
		return err
	}

	// delete bindingTxSpentIndex
	btxsPrefix := bindingTxSpentIndexSearchKey(height)
	btxsIter := db.stor.NewIterator(storage.BytesPrefix(btxsPrefix))
	defer btxsIter.Release()
	for btxsIter.Next() {
		btxsKey := btxsIter.Key()
		var scriptHash [ripemd160.Size]byte
		copy(scriptHash[:], btxsKey[20:40])
		btxHeight := binary.LittleEndian.Uint64(btxsKey[40:48])
		btxTxOffset := binary.LittleEndian.Uint32(btxsKey[48:52])
		btxTxLen := binary.LittleEndian.Uint32(btxsKey[52:56])
		btxIndex := binary.LittleEndian.Uint32(btxsKey[56:60])
		bindingTxIndex := &bindingTxIndex{
			scriptHash: scriptHash,
			blkHeight:  btxHeight,
			txOffset:   btxTxOffset,
			txLen:      btxTxLen,
			index:      btxIndex,
		}
		bindingTxIndexKey := bindingTxIndexToKey(bindingTxIndex)
		batch.Put(bindingTxIndexKey, blankData)

		bindingShIndex := &bindingShIndex{
			blkHeight:  btxHeight,
			scriptHash: scriptHash,
		}
		bindingShIndexKey := bindingShIndexToKey(bindingShIndex)
		batch.Put(bindingShIndexKey, blankData)
		batch.Delete(btxsKey)
	}
	if err := btxsIter.Error(); err != nil {
		return err
	}

	return nil
}

// from start to stop-1
func (db *ChainDb) FetchScriptHashRelatedTx(scriptHashes [][]byte, startBlock, stopBlock uint64,
	chainParams *config.Params) (map[uint64][]*wire.TxLoc, error) {
	allTx := make(map[[16]byte]struct{})
	result := make(map[uint64][]*wire.TxLoc)
	for _, v := range scriptHashes {
		var scriptHash [sha256.Size]byte
		if len(v) != sha256.Size {
			return nil, ErrWrongScriptHashLength
		}
		copy(scriptHash[:], v)
		start := txIndexSearchKey(scriptHash, startBlock)
		end := txIndexSearchKey(scriptHash, stopBlock)
		// interval [startBlock, stopBlock) stopBlock not include
		iter := db.stor.NewIterator(&storage.Range{Start: start, Limit: end})
		for iter.Next() {
			key := iter.Key()
			var txLoc [16]byte
			copy(txLoc[:], key[35:])
			if _, ok := allTx[txLoc]; !ok {
				allTx[txLoc] = struct{}{}
			}
		}
		iter.Release()

		if err := iter.Error(); err != nil {
			return nil, err
		}
	}
	for tx := range allTx {
		height := binary.BigEndian.Uint64(tx[:8])
		txOffset := binary.LittleEndian.Uint32(tx[8:12])
		txLen := binary.LittleEndian.Uint32(tx[12:])
		txLoc := &wire.TxLoc{TxStart: int(txOffset), TxLen: int(txLen)}
		result[height] = append(result[height], txLoc)
	}
	return result, nil
}

func (db *ChainDb) CheckScriptHashUsed(scriptHash []byte) (bool, error) {
	var v [sha256.Size]byte
	if len(scriptHash) != sha256.Size {
		return false, ErrWrongScriptHashLength
	}
	copy(v[:], scriptHash)
	prefix := usedSearchKey(v)

	iter := db.stor.NewIterator(storage.BytesPrefix(prefix))
	defer iter.Release()
	if iter.Next() {
		if err := iter.Error(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// checkAddrIndexVersion returns an error if the address index version stored
// in the database is less than the current version, or if it doesn't exist.
// This function is used on startup to signal OpenDB to drop the address index
// if it's in an old, incompatible format.
func (db *ChainDb) checkAddrIndexVersion() error {

	data, err := db.stor.Get(addrIndexVersionKey)
	if err != nil {
		return err
	}

	indexVersion := binary.LittleEndian.Uint16(data)

	if indexVersion != uint16(addrIndexCurrentVersion) {
		return database.ErrAddrIndexInvalidVersion
	}

	return nil
}

// FetchAddrIndexTip returns the hash and block height of the most recent
// block whose transactions have been indexed by address. It will return
// ErrAddrIndexDoesNotExist along with a zero hash, and UnknownHeight if the
// addrIndex hasn't yet been built up.
func (db *ChainDb) FetchAddrIndexTip() (*wire.Hash, uint64, error) {
	if db.dbStorageMeta.currentHeight == UnknownHeight {
		return &wire.Hash{}, UnknownHeight, database.ErrAddrIndexDoesNotExist
	}
	sha := db.dbStorageMeta.currentHash

	return &sha, db.dbStorageMeta.currentHeight, nil
}
