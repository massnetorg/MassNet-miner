package ldb

import (
	"crypto/sha256"
	"encoding/binary"

	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

var (
	// stakingTx
	recordStakingTx        = []byte("TXL")
	recordExpiredStakingTx = []byte("TXU")
)

const (
	// Each stakingTx/expiredStakingTx key is 55 bytes:
	// ----------------------------------------------------------------
	// |  Prefix | expiredHeight | blockHeight |   TxID   | TxOutIndex |
	// ----------------------------------------------------------------
	// | 3 bytes |    8 bytes   |   8 bytes  | 32 bytes |   4 bytes   |
	// ----------------------------------------------------------------
	stakingTxKeyLength = 55

	// Each stakingTx/expiredStakingTx value is 40 bytes:
	// -----------------------
	// | ScriptHash | Value  |
	// -----------------------
	// |  32 bytes  | 8 byte |
	// -----------------------

	stakingTxValueLength = 40

	// Each stakingTx/expiredStakingTx search key is 11 bytes:
	// ----------------------------
	// |   Prefix  | expiredHeight |
	// ----------------------------
	// |  3 bytes  |    8 byte    |
	// ----------------------------
	stakingTxSearchKeyLength = 11

	// Each stakingTx/expiredStakingTx value is 44 bytes:
	// ---------------------------------------
	// | blockHeight |   TxID   | TxOutIndex |
	// ---------------------------------------
	// |   8 bytes  | 32 bytes |    4 byte  |
	// ---------------------------------------
	stakingTxMapKeyLength = 44
)

type stakingTx struct {
	txSha         *wire.Hash
	index         uint32
	expiredHeight uint64
	rsh           [sha256.Size]byte
	value         uint64
	blkHeight     uint64
	delete        bool
}

func mustDecodeStakingTxValue(bs []byte) (scriptHash [sha256.Size]byte, value uint64) {
	if length := len(bs); length != stakingTxValueLength {
		logging.CPrint(logging.FATAL, "invalid raw StakingTxVale Data", logging.LogFormat{"length": length, "data": bs})
		panic("invalid raw StakingTxVale Data") // should not reach
	}
	copy(scriptHash[:], bs[:32])
	value = binary.LittleEndian.Uint64(bs[32:40])
	return
}

type stakingTxMapKey struct {
	blockHeight uint64
	txID        wire.Hash
	index       uint32
}

func mustDecodeStakingTxKey(buf []byte) (expiredHeight uint64, mapKey stakingTxMapKey) {
	if length := len(buf); length != stakingTxKeyLength {
		logging.CPrint(logging.FATAL, "invalid raw stakingTxKey Data", logging.LogFormat{"length": length, "data": buf})
		panic("invalid raw stakingTxKey Data") // should not reach
	}
	expiredHeight = binary.LittleEndian.Uint64(buf[3:11])
	height := binary.LittleEndian.Uint64(buf[11:19])
	var txid wire.Hash
	copy(txid[:], buf[19:51])
	index := binary.LittleEndian.Uint32(buf[51:])
	mapKey = stakingTxMapKey{
		blockHeight: height,
		txID:        txid,
		index:       index,
	}
	return
}

func mustDecodeStakingTxMapKey(bs []byte) stakingTxMapKey {
	if length := len(bs); length != stakingTxMapKeyLength {
		logging.CPrint(logging.FATAL, "invalid raw stakingTxMapKey Data", logging.LogFormat{"length": length, "data": bs})
		panic("invalid raw stakingTxMapKey Data") // should not reach
	}
	height := binary.LittleEndian.Uint64(bs[:8])
	var txid wire.Hash
	copy(txid[:], bs[8:40])
	return stakingTxMapKey{
		blockHeight: height,
		txID:        txid,
		index:       binary.LittleEndian.Uint32(bs[40:]),
	}
}

func mustEncodeStakingTxMapKey(mapKey stakingTxMapKey) []byte {
	var key [stakingTxMapKeyLength]byte
	binary.LittleEndian.PutUint64(key[:8], mapKey.blockHeight)
	copy(key[8:40], mapKey.txID[:])
	binary.LittleEndian.PutUint32(key[40:44], mapKey.index)
	return key[:]
}

func heightStakingTxToKey(expiredHeight uint64, mapKey stakingTxMapKey) []byte {
	mapKeyData := mustEncodeStakingTxMapKey(mapKey)
	key := make([]byte, stakingTxKeyLength)

	copy(key[:len(recordStakingTx)], recordStakingTx)
	binary.LittleEndian.PutUint64(key[len(recordStakingTx):len(recordStakingTx)+8], expiredHeight)
	copy(key[len(recordStakingTx)+8:], mapKeyData)
	return key
}

func heightExpiredStakingTxToKey(expiredHeight uint64, mapKey stakingTxMapKey) []byte {
	mapKeyData := mustEncodeStakingTxMapKey(mapKey)
	key := make([]byte, stakingTxKeyLength)

	copy(key[:len(recordExpiredStakingTx)], recordExpiredStakingTx)
	binary.LittleEndian.PutUint64(key[len(recordExpiredStakingTx):len(recordExpiredStakingTx)+8], expiredHeight)
	copy(key[len(recordExpiredStakingTx)+8:], mapKeyData)
	return key
}

func stakingTxSearchKey(expiredHeight uint64) []byte {
	prefix := make([]byte, stakingTxSearchKeyLength)
	copy(prefix[:len(recordStakingTx)], recordStakingTx)
	binary.LittleEndian.PutUint64(prefix[len(recordStakingTx):stakingTxSearchKeyLength], expiredHeight)
	return prefix
}

func expiredStakingTxSearchKey(expiredHeight uint64) []byte {
	prefix := make([]byte, stakingTxSearchKeyLength)
	copy(prefix[:len(recordExpiredStakingTx)], recordExpiredStakingTx)
	binary.LittleEndian.PutUint64(prefix[len(recordExpiredStakingTx):stakingTxSearchKeyLength], expiredHeight)
	return prefix
}

func (db *ChainDb) insertStakingTx(txSha *wire.Hash, index uint32, frozenPeriod uint64, blkHeight uint64, rsh [sha256.Size]byte, value int64) (err error) {
	var txL stakingTx
	var mapKey = stakingTxMapKey{
		blockHeight: blkHeight,
		txID:        *txSha,
		index:       index,
	}

	txL.txSha = txSha
	txL.index = index
	txL.expiredHeight = frozenPeriod + blkHeight
	txL.rsh = rsh
	txL.value = uint64(value)
	txL.blkHeight = blkHeight
	logging.CPrint(logging.DEBUG, "insertStakingTx in the height", logging.LogFormat{"startHeight": blkHeight, "expiredHeight": txL.expiredHeight})
	db.stakingTxMap[mapKey] = &txL

	return nil
}

func (db *ChainDb) formatSTx(stx *stakingTx) []byte {
	rsh := stx.rsh
	value := stx.value

	txW := make([]byte, 40)
	copy(txW[:32], rsh[:])
	binary.LittleEndian.PutUint64(txW[32:40], value)

	return txW
}

func (db *ChainDb) FetchExpiredStakingTxListByHeight(expiredHeight uint64) (database.StakingNodes, error) {

	nodes := make(database.StakingNodes)

	searchKey := expiredStakingTxSearchKey(expiredHeight)

	iter := db.stor.NewIterator(storage.BytesPrefix(searchKey))
	defer iter.Release()

	for iter.Next() {
		// key
		key := iter.Key()
		mapKey := mustDecodeStakingTxMapKey(key[11:])
		blockHeight := mapKey.blockHeight
		outPoint := wire.OutPoint{
			Hash:  mapKey.txID,
			Index: mapKey.index,
		}

		// data
		data := iter.Value()
		scriptHash, value := mustDecodeStakingTxValue(data)

		stakingInfo := database.StakingTxInfo{
			Value:        value,
			FrozenPeriod: expiredHeight - blockHeight,
			BlkHeight:    blockHeight,
		}

		if !nodes.Get(scriptHash).Put(outPoint, stakingInfo) {
			return nil, ErrCheckStakingDuplicated
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return nodes, nil
}

func (db *ChainDb) FetchStakingTxMap() (database.StakingNodes, error) {

	nodes := make(database.StakingNodes)

	iter := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iter.Release()

	for iter.Next() {

		// key
		key := iter.Key()
		expiredHeight, mapKey := mustDecodeStakingTxKey(key)
		blkHeight := mapKey.blockHeight
		outPoint := wire.OutPoint{
			Hash:  mapKey.txID,
			Index: mapKey.index,
		}

		// data
		data := iter.Value()
		scriptHash, value := mustDecodeStakingTxValue(data)

		stakingInfo := database.StakingTxInfo{
			Value:        value,
			FrozenPeriod: expiredHeight - blkHeight,
			BlkHeight:    blkHeight,
		}

		if !nodes.Get(scriptHash).Put(outPoint, stakingInfo) {
			return nil, ErrCheckStakingDuplicated
		}
	}

	if err := iter.Error(); err != nil {
		return nil, err
	}

	return nodes, nil
}

// fetchActiveStakingTxFromUnexpired returns currently unexpired staking at 'height'
func (db *ChainDb) fetchActiveStakingTxFromUnexpired(height uint64) (map[[sha256.Size]byte][]database.StakingTxInfo, error) {

	stakingTxInfos := make(map[[sha256.Size]byte][]database.StakingTxInfo)

	iter := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iter.Release()

	for iter.Next() {
		key := iter.Key()
		expiredHeight := binary.LittleEndian.Uint64(key[len(recordStakingTx) : len(recordStakingTx)+8])
		mapKey := mustDecodeStakingTxMapKey(key[len(recordStakingTx)+8:])
		blkHeight := mapKey.blockHeight

		value := iter.Value()
		scriptHash, amount := mustDecodeStakingTxValue(value)

		if height > blkHeight &&
			height <= expiredHeight &&
			height-blkHeight >= consensus.StakingTxRewardStart {
			stakingTxInfos[scriptHash] = append(stakingTxInfos[scriptHash], database.StakingTxInfo{
				Value:        amount,
				FrozenPeriod: expiredHeight - blkHeight,
				BlkHeight:    blkHeight})
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return stakingTxInfos, nil
}

// FetchUnexpiredStakingRank returns only currently unexpired staking rank at
// target height. This function is for mining and validating block.
func (db *ChainDb) FetchUnexpiredStakingRank(height uint64, onlyOnList bool) ([]database.Rank, error) {
	stakingTxInfos, err := db.fetchActiveStakingTxFromUnexpired(height)
	if err != nil {
		return nil, err
	}
	sortedStakingTx, err := database.SortMap(stakingTxInfos, height, onlyOnList)
	if err != nil {
		return nil, err
	}
	count := len(sortedStakingTx)
	rankList := make([]database.Rank, count)
	for i := 0; i < count; i++ {
		rankList[i].Rank = int32(i)
		rankList[i].Value = sortedStakingTx[i].Value
		rankList[i].ScriptHash = sortedStakingTx[i].Key
		rankList[i].Weight = sortedStakingTx[i].Weight
		rankList[i].StakingTx = stakingTxInfos[sortedStakingTx[i].Key]
	}
	return rankList, nil
}

// fetchActiveStakingTxFromExpired returns once unexpired staking at 'height'
func (db *ChainDb) fetchActiveStakingTxFromExpired(height uint64) (map[[sha256.Size]byte][]database.StakingTxInfo, error) {
	stakingTxInfos := make(map[[sha256.Size]byte][]database.StakingTxInfo)

	iter := db.stor.NewIterator(storage.BytesPrefix(recordExpiredStakingTx))
	defer iter.Release()

	for iter.Next() {
		key := iter.Key()
		expiredHeight := binary.LittleEndian.Uint64(key[len(recordExpiredStakingTx) : len(recordExpiredStakingTx)+8])
		mapKey := mustDecodeStakingTxMapKey(key[len(recordExpiredStakingTx)+8:])
		blkHeight := mapKey.blockHeight

		value := iter.Value()
		scriptHash, amount := mustDecodeStakingTxValue(value)

		if height > blkHeight &&
			height <= expiredHeight &&
			height-blkHeight >= consensus.StakingTxRewardStart {
			stakingTxInfos[scriptHash] = append(stakingTxInfos[scriptHash], database.StakingTxInfo{
				Value:        amount,
				FrozenPeriod: expiredHeight - blkHeight,
				BlkHeight:    blkHeight,
			})
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return stakingTxInfos, nil
}

// FetchStakingRank returns staking rank at any height. This
// function may be slow.
func (db *ChainDb) FetchStakingRank(height uint64, onlyOnList bool) ([]database.Rank, error) {
	stakingTxInfos, err := db.fetchActiveStakingTxFromUnexpired(height)
	if err != nil {
		return nil, err
	}

	expired, err := db.fetchActiveStakingTxFromExpired(height)
	if err != nil {
		return nil, err
	}
	for expiredK, expiredV := range expired {

		if _, ok := stakingTxInfos[expiredK]; !ok {
			stakingTxInfos[expiredK] = expiredV
			continue
		}
		stakingTxInfos[expiredK] = append(stakingTxInfos[expiredK], expiredV...)
	}

	sortedStakingTx, err := database.SortMap(stakingTxInfos, height, onlyOnList)
	if err != nil {
		return nil, err
	}
	count := len(sortedStakingTx)
	rankList := make([]database.Rank, count)
	for i := 0; i < count; i++ {
		rankList[i].Rank = int32(i)
		rankList[i].Value = sortedStakingTx[i].Value
		rankList[i].ScriptHash = sortedStakingTx[i].Key
		rankList[i].Weight = sortedStakingTx[i].Weight
		rankList[i].StakingTx = stakingTxInfos[sortedStakingTx[i].Key]
	}
	return rankList, nil
}
