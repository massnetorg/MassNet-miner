package ldb

import (
	"bytes"
	"encoding/binary"
	"sort"

	"golang.org/x/crypto/ripemd160"
	"massnet.org/mass/config"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
)

var (
	bindingTxIndexPrefix            = []byte("STG")
	bindingShIndexPrefix            = []byte("HTGS")
	bindingTxSpentIndexPrefix       = []byte("STSG")
	bindingTxIndexKeyLen            = 3 + ripemd160.Size + 8 + 4 + 4 + 4
	bindingTxIndexSearchKeyLen      = 3 + ripemd160.Size
	bindingTxIndexDeleteKeyLen      = 3 + ripemd160.Size + 8
	bindingTxSpentIndexKeyLen       = 4 + 8 + 4 + 4 + ripemd160.Size + 8 + 4 + 4 + 4
	bindingTxSpentIndexSearchKeyLen = 4 + 8
	bindingShIndexKeyLen            = 4 + 8 + ripemd160.Size
	bindingShIndexSearchKeyLen      = 4 + 8
)

type bindingTxIndex struct {
	scriptHash [ripemd160.Size]byte
	blkHeight  uint64
	txOffset   uint32
	txLen      uint32
	index      uint32
}

type bindingTxSpentIndex struct {
	scriptHash       [ripemd160.Size]byte
	blkHeightSpent   uint64
	txOffsetSpent    uint32
	txLenSpent       uint32
	blkHeightBinding uint64
	txOffsetBinding  uint32
	txLenBinding     uint32
	indexBinding     uint32
}

type bindingShIndex struct {
	blkHeight  uint64
	scriptHash [ripemd160.Size]byte
}

func bindingTxIndexToKey(btxIndex *bindingTxIndex) []byte {
	key := make([]byte, bindingTxIndexKeyLen, bindingTxIndexKeyLen)
	copy(key, bindingTxIndexPrefix)
	copy(key[3:23], btxIndex.scriptHash[:])
	binary.LittleEndian.PutUint64(key[23:31], btxIndex.blkHeight)
	binary.LittleEndian.PutUint32(key[31:35], btxIndex.txOffset)
	binary.LittleEndian.PutUint32(key[35:39], btxIndex.txLen)
	binary.LittleEndian.PutUint32(key[39:43], btxIndex.index)
	return key
}

func mustDecodeBindingTxIndexKey(key []byte) (*bindingTxIndex, error) {
	if len(key) != bindingTxIndexKeyLen {
		return nil, ErrWrongBindingTxIndexLen
	}

	start := 0
	prefix := make([]byte, len(bindingTxIndexPrefix))
	copy(prefix, key[start:start+len(bindingTxIndexPrefix)])
	if !bytes.Equal(prefix, bindingTxIndexPrefix) {
		return nil, ErrWrongBindingTxIndexPrefix
	}
	start += len(bindingTxIndexPrefix)

	var scriptHash [ripemd160.Size]byte
	copy(scriptHash[:], key[start:start+ripemd160.Size])
	start += ripemd160.Size

	blockHeight := binary.LittleEndian.Uint64(key[start : start+8])
	start += 8
	txOffset := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4
	txLen := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4
	index := binary.LittleEndian.Uint32(key[start:bindingTxIndexKeyLen])

	return &bindingTxIndex{
		scriptHash: scriptHash,
		blkHeight:  blockHeight,
		txOffset:   txOffset,
		txLen:      txLen,
		index:      index,
	}, nil
}

func bindingTxIndexSearchKey(scriptHash [ripemd160.Size]byte) []byte {
	key := make([]byte, bindingTxIndexSearchKeyLen, bindingTxIndexSearchKeyLen)
	copy(key, bindingTxIndexPrefix)
	copy(key[3:23], scriptHash[:])
	return key
}

func bindingTxIndexDeleteKey(scriptHash [ripemd160.Size]byte, blkHeight uint64) []byte {
	key := make([]byte, bindingTxIndexDeleteKeyLen, bindingTxIndexDeleteKeyLen)
	copy(key, bindingTxIndexPrefix)
	copy(key[3:23], scriptHash[:])
	binary.LittleEndian.PutUint64(key[23:31], blkHeight)
	return key
}

func bindingTxSpentIndexToKey(btxSpentIndex *bindingTxSpentIndex) []byte {
	key := make([]byte, bindingTxSpentIndexKeyLen, bindingTxSpentIndexKeyLen)
	copy(key, bindingTxSpentIndexPrefix)
	binary.LittleEndian.PutUint64(key[4:12], btxSpentIndex.blkHeightSpent)
	binary.LittleEndian.PutUint32(key[12:16], btxSpentIndex.txOffsetSpent)
	binary.LittleEndian.PutUint32(key[16:20], btxSpentIndex.txLenSpent)
	copy(key[20:40], btxSpentIndex.scriptHash[:])
	binary.LittleEndian.PutUint64(key[40:48], btxSpentIndex.blkHeightBinding)
	binary.LittleEndian.PutUint32(key[48:52], btxSpentIndex.txOffsetBinding)
	binary.LittleEndian.PutUint32(key[52:56], btxSpentIndex.txLenBinding)
	binary.LittleEndian.PutUint32(key[56:60], btxSpentIndex.indexBinding)
	return key
}

func mustDecodeBindingTxSpentIndexKey(key []byte) (*bindingTxSpentIndex, error) {
	if len(key) != bindingTxSpentIndexKeyLen {
		return nil, ErrWrongBindingTxSpentIndexLen
	}

	start := 0
	prefix := make([]byte, len(bindingTxSpentIndexPrefix))
	copy(prefix, key[start:start+len(bindingTxSpentIndexPrefix)])
	if !bytes.Equal(prefix, bindingTxSpentIndexPrefix) {
		return nil, ErrWrongBindingTxSpentIndexPrefix
	}
	start += len(bindingTxSpentIndexPrefix)

	stxBlockHeight := binary.LittleEndian.Uint64(key[start : start+8])
	start += 8
	stxTxOffset := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4
	stxTxLen := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4

	var scriptHash [ripemd160.Size]byte
	copy(scriptHash[:], key[start:start+ripemd160.Size])
	start += ripemd160.Size
	btxBlockHeight := binary.LittleEndian.Uint64(key[start : start+8])
	start += 8
	btxTxOffset := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4
	btxTxLen := binary.LittleEndian.Uint32(key[start : start+4])
	start += 4
	btxIndex := binary.LittleEndian.Uint32(key[start:bindingTxSpentIndexKeyLen])

	return &bindingTxSpentIndex{
		scriptHash:       scriptHash,
		blkHeightSpent:   stxBlockHeight,
		txOffsetSpent:    stxTxOffset,
		txLenSpent:       stxTxLen,
		blkHeightBinding: btxBlockHeight,
		txOffsetBinding:  btxTxOffset,
		txLenBinding:     btxTxLen,
		indexBinding:     btxIndex,
	}, nil
}

func bindingTxSpentIndexSearchKey(blkHeight uint64) []byte {
	key := make([]byte, bindingTxSpentIndexSearchKeyLen, bindingTxSpentIndexSearchKeyLen)
	copy(key, bindingTxSpentIndexPrefix)
	binary.LittleEndian.PutUint64(key[4:12], blkHeight)
	return key
}

func bindingShIndexToKey(bshIndex *bindingShIndex) []byte {
	key := make([]byte, bindingShIndexKeyLen)
	copy(key, bindingShIndexPrefix)
	binary.LittleEndian.PutUint64(key[4:12], bshIndex.blkHeight)
	copy(key[12:32], bshIndex.scriptHash[:])
	return key
}

func mustDecodeBindingShIndexKey(key []byte) (*bindingShIndex, error) {
	if len(key) != bindingShIndexKeyLen {
		return nil, ErrWrongBindingShIndexLen
	}

	start := 0
	prefix := make([]byte, len(bindingShIndexPrefix))
	copy(prefix, key[start:start+len(bindingShIndexPrefix)])
	if !bytes.Equal(prefix, bindingShIndexPrefix) {
		return nil, ErrWrongBindingShIndexPrefix
	}
	start += len(bindingShIndexPrefix)

	blockHeight := binary.LittleEndian.Uint64(key[start : start+8])
	start += 8

	var scriptHash [ripemd160.Size]byte
	copy(scriptHash[:], key[start:bindingShIndexKeyLen])

	return &bindingShIndex{
		blkHeight:  blockHeight,
		scriptHash: scriptHash,
	}, nil
}

func bindingShIndexSearchKey(blkHeight uint64) []byte {
	key := make([]byte, bindingShIndexSearchKeyLen, bindingShIndexSearchKeyLen)
	copy(key, bindingShIndexPrefix)
	binary.LittleEndian.PutUint64(key[4:12], blkHeight)
	return key
}

func (db *ChainDb) FetchScriptHashRelatedBindingTx(scriptHash []byte, chainParams *config.Params) ([]*database.BindingTxReply, error) {
	btxs := make([]*database.BindingTxReply, 0)
	var sh [ripemd160.Size]byte
	if len(scriptHash) != ripemd160.Size {
		return nil, ErrWrongScriptHashLength
	}
	copy(sh[:], scriptHash)
	btxSearchKeyPrefix := bindingTxIndexSearchKey(sh)
	iter := db.stor.NewIterator(storage.BytesPrefix(btxSearchKeyPrefix))
	defer iter.Release()
	for iter.Next() {
		key := iter.Key()
		blkHeight := binary.LittleEndian.Uint64(key[23:31])
		txOffset := binary.LittleEndian.Uint32(key[31:35])
		txLen := binary.LittleEndian.Uint32(key[35:39])
		index := binary.LittleEndian.Uint32(key[39:43])
		msgtx, _, err := db.fetchTxDataByLoc(blkHeight, int(txOffset), int(txLen))
		if err != nil {
			return nil, err
		}
		txsha := msgtx.TxHash()
		btxs = append(btxs, &database.BindingTxReply{
			Height: blkHeight,
			TxSha:  &txsha,
			Index:  index,
			Value:  msgtx.TxOut[index].Value,
		})
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	sort.Stable(sort.Reverse(btxList(btxs)))

	return btxs, nil
}

type btxList []*database.BindingTxReply

func (gl btxList) Swap(i, j int) {
	gl[i], gl[j] = gl[j], gl[i]
}

func (gl btxList) Len() int {
	return len(gl)
}

func (gl btxList) Less(i, j int) bool {
	return gl[i].Value < gl[j].Value
}
