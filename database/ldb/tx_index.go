package ldb

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"golang.org/x/crypto/ripemd160"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

const (
	addrIndexCurrentVersion = 1

	htsBoundaryDistance = 32
	stlBoundaryDistance = 16
	stlBitmapMask       = (0x01 << stlBoundaryDistance) - 1
)

var (
	// Address index version is required to drop/rebuild address index if version
	// is older than current as the format of the index may have changed. This is
	// true when going from no version to version 1 as the address index is stored
	// as big endian in version 1 and little endian in the original code. Version
	// is stored as two bytes, little endian (to match all the code but the index).
	addrIndexVersionKey = []byte("ADDRINDEXVERSION")

	//  | key prefix | script hash | lower boundary |      |       bitmap      | index  |  count  | tx offset | tx length | (4 + 4) x N | index  |  count  | tx offset | tx length | (4 + 4) x N |
	//   ------------------------------------------- -----> ---------------------------------------------------------------------------------------------------------------------
	//  |   3 bytes  |  32 bytes   |     8 bytes    |      |  4 bytes [31...0] | 1 byte | 2 bytes |  4 bytes  |  4 bytes  |     ...     | 1 byte | 2 bytes |  4 bytes  |  4 bytes  |     ...     |
	//
	//  <lower boundary> = height / 32 * 32    ---  BigEndian
	//  bitmap |= 0x01 << (height - <lower boundary>)
	txIndexPrefix = []byte("STL")

	//  | key prefix | lower boundary | script hash |      |       bitmap      |
	//   ------------------------------------------- -----> -------------------
	//  |  3 bytes   |    8 bytes     |   32 bytes  |      |  4 bytes [31...0] |
	shIndexPrefix = []byte("HTS")

	// txIndexKeyLen = len(txIndexPrefix) + len(scriptHash)
	usedSearchKeyLen = 3 + sha256.Size
	// txIndexKeyLen = len(txIndexPrefix) + len(scriptHash) + len(lower boundary)
	txIndexKeyLen = 3 + sha256.Size + 8
	// at least one wire.TxLoc
	minTxIndexValueLen = 4 + 1 + 2 + 4 + 4

	shIndexKeyLen       = 3 + 8 + sha256.Size
	shIndexSearchKeyLen = 3 + 8
	blankData           []byte
)

func calcHTSLowBoundary(blkHeight uint64) uint64 {
	return blkHeight / htsBoundaryDistance * htsBoundaryDistance
}

func calcSTLLowBoundary(blkHeight uint64) uint64 {
	return blkHeight / stlBoundaryDistance * stlBoundaryDistance
}

func usedSearchKey(scriptHash [sha256.Size]byte) []byte {
	key := make([]byte, usedSearchKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], scriptHash[:])
	return key
}

func encodeHTSKey(blkHeight uint64, scriptHash [sha256.Size]byte) (key []byte, lowBound uint64) {
	lowBound = calcHTSLowBoundary(blkHeight)
	key = make([]byte, shIndexKeyLen)
	copy(key, shIndexPrefix)
	binary.BigEndian.PutUint64(key[3:11], lowBound) // We use BigEndian here.
	copy(key[11:43], scriptHash[:])
	return
}

func encodeHTSSearchKeyPrefix(blkHeight uint64) (prefix []byte, lowBound uint64) {
	lowBound = calcHTSLowBoundary(blkHeight)
	prefix = make([]byte, shIndexSearchKeyLen)
	copy(prefix, shIndexPrefix)
	binary.BigEndian.PutUint64(prefix[3:11], lowBound)
	return
}

func encodeSTLKey(blkHeight uint64, scriptHash [sha256.Size]byte) (key []byte, lowBound uint64) {
	lowBound = calcSTLLowBoundary(blkHeight)
	key = make([]byte, txIndexKeyLen)
	copy(key, txIndexPrefix)
	copy(key[3:35], scriptHash[:])
	binary.BigEndian.PutUint64(key[35:43], lowBound) // We use BigEndian here.
	return
}

func parseSTLValue(boundary uint64, data []byte) (map[uint64][]*wire.TxLoc, error) {

	result := make(map[uint64][]*wire.TxLoc)

	bitmap := binary.LittleEndian.Uint32(data[0:4]) & stlBitmapMask
	dataLen := len(data)
	if bitmap == 0 || dataLen < minTxIndexValueLen {
		logging.CPrint(logging.ERROR, "unexpected invalid data",
			logging.LogFormat{
				"boundary":     boundary,
				"bitmap":       bitmap,
				"value_length": dataLen,
			})
		return nil, ErrIncorrectDbData
	}

	cur := 4 // bitmap
	shift := 0
	for bitmap != 0 && shift < stlBoundaryDistance {
		if bitmap&0x01 != 0 {
			index := data[cur]
			if shift != int(index) {
				logging.CPrint(logging.ERROR, "unexpected stl index",
					logging.LogFormat{
						"boundary": boundary,
						"index":    index,
						"shift":    shift,
						"cur":      cur,
					})
				return nil, ErrIncorrectDbData
			}
			count := binary.LittleEndian.Uint16(data[cur+1 : cur+3])
			cur += 3
			if dataLen < cur+int(count)*8 || count == 0 {
				logging.CPrint(logging.ERROR, "unexpected illegal stl value",
					logging.LogFormat{
						"boundary":     boundary,
						"count":        count,
						"value_length": dataLen,
						"cur":          cur,
					})
				return nil, ErrIncorrectDbData
			}
			height := boundary + uint64(shift)
			for i := 0; i < int(count); i++ {
				txloc := &wire.TxLoc{
					TxStart: int(binary.LittleEndian.Uint32(data[cur : cur+4])),
					TxLen:   int(binary.LittleEndian.Uint32(data[cur+4 : cur+8])),
				}
				result[height] = append(result[height], txloc)
				cur += 8
			}
		}
		shift++
		bitmap >>= 1
	}
	if cur != dataLen {
		logging.CPrint(logging.ERROR, "unexpected cur",
			logging.LogFormat{
				"boundary":     boundary,
				"cur":          cur,
				"value_length": dataLen,
			})
		return nil, ErrIncorrectDbData
	}
	return result, nil
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

	for addrKey, indexes := range txAddrIndex {
		if len(indexes) == 0 {
			continue
		}
		// put or update "HTS"
		htsBitmap := uint32(0)
		htsKey, htsLowBound := encodeHTSKey(height, addrKey)
		htsOldValue, err := db.stor.Get(htsKey)
		if err != nil {
			if err != storage.ErrNotFound {
				return err
			}
		} else {
			htsBitmap = binary.LittleEndian.Uint32(htsOldValue)
		}
		htsBit := uint32(0x01) << int(height-htsLowBound)
		if htsBitmap&htsBit == 0 {
			htsBitmap |= htsBit
			newValue := make([]byte, 4)
			binary.LittleEndian.PutUint32(newValue, htsBitmap)
			batch.Put(htsKey, newValue)
		} else {
			logging.CPrint(logging.ERROR, "hts bitmap should be unset",
				logging.LogFormat{
					"height":   height,
					"boundary": htsLowBound,
				})
			return ErrIncorrectDbData
		}

		// put or update "STL"
		stlBitmap := uint32(0)
		stlKey, stlLowBound := encodeSTLKey(height, addrKey)
		stlOldValue, err := db.stor.Get(stlKey)
		if err != nil {
			if err != storage.ErrNotFound {
				return err
			}
		} else {
			stlBitmap = binary.LittleEndian.Uint32(stlOldValue[0:4])
		}
		shiftN := int(height - stlLowBound)
		if stlBitmap&(0x01<<shiftN) != 0 {
			logging.CPrint(logging.ERROR, "stl bitmap should not be set",
				logging.LogFormat{
					"height":   height,
					"boundary": stlLowBound,
				})
			return ErrIncorrectDbData
		}

		// find where to put current TxLoc if old value exists.
		target := 0
		if len(stlOldValue) > 0 {
			target = 4 // bitmap length
			for i := 0; i < shiftN; i++ {
				if stlBitmap&(0x01<<i) != 0 {
					index := stlOldValue[target]
					if int(index) != i {
						logging.CPrint(logging.ERROR, "unexpected index", logging.LogFormat{
							"height":  height,
							"shift_i": i,
							"index":   index,
						})
						return ErrIncorrectDbData
					}
					N := binary.LittleEndian.Uint16(stlOldValue[target+1 : target+3])
					target += 3 + 8*int(N) // 1+2+8*N
				}
			}
		}

		// serialize TxLoc
		sort.Slice(indexes, func(i, j int) bool {
			return indexes[i].TxStart < indexes[j].TxStart
		})
		N := len(indexes)
		bufLen := 3 + 8*N // 1+2+8*N
		buf := make([]byte, bufLen)
		buf[0] = byte(shiftN)                              // put index
		binary.LittleEndian.PutUint16(buf[1:3], uint16(N)) // put count
		start := 3
		for _, index := range indexes {
			binary.LittleEndian.PutUint32(buf[start:start+4], uint32(index.TxStart)) // offset
			binary.LittleEndian.PutUint32(buf[start+4:start+8], uint32(index.TxLen)) // length
			start += 8
		}
		stlBitmap |= 0x01 << shiftN // set bitmap

		var stlNewValue []byte
		if target == 0 {
			stlNewValue = make([]byte, 4+bufLen)
			binary.LittleEndian.PutUint32(stlNewValue[0:4], stlBitmap)
			copy(stlNewValue[4:], buf)
		} else {
			stlNewValue = make([]byte, len(stlOldValue)+bufLen)
			binary.LittleEndian.PutUint32(stlNewValue[0:4], stlBitmap)
			copy(stlNewValue[4:], stlOldValue[4:target])
			copy(stlNewValue[target:target+bufLen], buf)
			copy(stlNewValue[target+bufLen:], stlOldValue[target:])
		}
		if err = batch.Put(stlKey, stlNewValue); err != nil {
			return err
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
	shPrefix, htsLowBound := encodeHTSSearchKeyPrefix(height)
	htsBit := uint32(0x01) << (height - htsLowBound)

	iter := db.stor.NewIterator(storage.BytesPrefix(shPrefix))
	defer iter.Release()
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// delete "HTS"
		bitmap := binary.LittleEndian.Uint32(value)
		if bitmap&htsBit == 0 {
			continue
		}
		bitmap &= ^htsBit
		if bitmap == 0 {
			batch.Delete(key)
		} else {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, bitmap)
			batch.Put(key, buf)
		}

		// delete "STL"
		var scriptHash [sha256.Size]byte
		copy(scriptHash[:], key[11:43])
		stlKey, stlLowBound := encodeSTLKey(height, scriptHash)
		stlOldValue, err := db.stor.Get(stlKey)
		if err != nil || len(stlOldValue) < minTxIndexValueLen {
			logging.CPrint(logging.ERROR, "unexpected error",
				logging.LogFormat{
					"err":          err,
					"height":       height,
					"value_length": len(stlOldValue),
				})
			return err
		}

		shiftN := height - stlLowBound
		stlBit := uint32(0x01) << shiftN
		stlBitmap := binary.LittleEndian.Uint32(stlOldValue[0:4])
		if stlBitmap&stlBit == 0 {
			logging.CPrint(logging.ERROR, "unexpected stl bitmap unset", logging.LogFormat{"height": height})
			return ErrIncorrectDbData
		}

		end := 4 // bitmap
		lastSkip := 0
		for i := 0; i <= int(shiftN); i++ {
			if stlBitmap&(0x01<<i) != 0 {
				index := stlOldValue[end]
				if int(index) != i {
					logging.CPrint(logging.ERROR, "unexpected index", logging.LogFormat{
						"height":  height,
						"shift_i": i,
						"index":   index,
					})
					return ErrIncorrectDbData
				}
				N := binary.LittleEndian.Uint16(stlOldValue[end+1 : end+3])
				lastSkip = 3 + 8*int(N) // 1+2+8*N
				end += lastSkip
			}
		}

		stlBitmap &= ^stlBit
		if stlBitmap&stlBitmapMask == 0 {
			// delete when empty
			batch.Delete(stlKey)
			continue
		}
		stlNewValue := make([]byte, len(stlOldValue)-lastSkip)
		binary.LittleEndian.PutUint32(stlNewValue, stlBitmap)
		copy(stlNewValue[4:end-lastSkip], stlOldValue[4:end-lastSkip])
		copy(stlNewValue[end-lastSkip:], stlOldValue[end:])
		batch.Put(stlKey, stlNewValue)
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
func (db *ChainDb) FetchScriptHashRelatedTx(
	scriptHashes [][]byte,
	startBlock, stopBlock uint64,
) (map[uint64][]*wire.TxLoc, error) {

	lowBoundary, upBoundary := calcSTLLowBoundary(startBlock), calcSTLLowBoundary(stopBlock-1)

	dedup := make(map[uint64]map[wire.TxLoc]struct{})
	for _, v := range scriptHashes {
		var scriptHash [sha256.Size]byte
		copy(scriptHash[:], v)
		for curBoundary := lowBoundary; curBoundary <= upBoundary; curBoundary += stlBoundaryDistance {
			key, _ := encodeSTLKey(curBoundary, scriptHash)
			value, err := db.stor.Get(key)
			if err != nil {
				if err == storage.ErrNotFound {
					continue
				}
				return nil, err
			}
			heightToList, err := parseSTLValue(curBoundary, value)
			if err != nil {
				return nil, err
			}
			for height, list := range heightToList {
				if height < startBlock || height >= stopBlock {
					continue
				}
				m, ok := dedup[height]
				if !ok {
					m = make(map[wire.TxLoc]struct{})
					dedup[height] = m
				}
				for _, loc := range list {
					m[*loc] = struct{}{}
				}
			}
		}
	}
	result := make(map[uint64][]*wire.TxLoc)
	for height, locs := range dedup {
		slice := make([]*wire.TxLoc, 0, len(locs))
		for loc := range locs {
			locCopy := loc
			slice = append(slice, &locCopy)
		}
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].TxStart < slice[j].TxStart
		})
		result[height] = slice
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
