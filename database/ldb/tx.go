package ldb

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
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

type txUpdateObj struct {
	txSha     *wire.Hash
	blkHeight uint64
	txoff     int
	txlen     int
	ntxout    int
	spentData []byte
	delete    bool
}

type spentTx struct {
	blkHeight uint64
	txoff     int
	txlen     int
	numTxO    int
	delete    bool
}

type spentTxUpdate struct {
	txl    []*spentTx
	delete bool
}

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

// insertTx inserts a tx hash and its associated data into the database.
// Must be called with db lock held.
func (db *ChainDb) insertTx(txSha *wire.Hash, height uint64, txoff int, txlen int, spentbuf []byte) (err error) {
	var txU txUpdateObj

	txU.txSha = txSha
	txU.blkHeight = height
	txU.txoff = txoff
	txU.txlen = txlen
	txU.spentData = spentbuf

	db.txUpdateMap[*txSha] = &txU

	return nil
}

// formatTx generates the value buffer for the Tx db.
func (db *ChainDb) formatTx(txu *txUpdateObj) []byte {
	blkHeight := uint64(txu.blkHeight)
	txOff := uint32(txu.txoff)
	txLen := uint32(txu.txlen)
	spentbuf := txu.spentData

	txW := make([]byte, 16+len(spentbuf))
	binary.LittleEndian.PutUint64(txW[0:8], blkHeight)
	binary.LittleEndian.PutUint32(txW[8:12], txOff)
	binary.LittleEndian.PutUint32(txW[12:16], txLen)
	copy(txW[16:], spentbuf)

	return txW[:]
}

func (db *ChainDb) formatSTx(stx *stakingTx) []byte {
	rsh := stx.rsh
	value := stx.value

	txW := make([]byte, 40)
	copy(txW[:32], rsh[:])
	binary.LittleEndian.PutUint64(txW[32:40], value)

	return txW

}

func (db *ChainDb) getTxData(txsha *wire.Hash) (uint64, int, int, []byte, error) {
	key := shaTxToKey(txsha)
	buf, err := db.stor.Get(key)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	blkHeight := binary.LittleEndian.Uint64(buf[0:8])
	txOff := binary.LittleEndian.Uint32(buf[8:12])
	txLen := binary.LittleEndian.Uint32(buf[12:16])

	spentBuf := make([]byte, len(buf)-16)
	copy(spentBuf, buf[16:])

	return blkHeight, int(txOff), int(txLen), spentBuf, nil
}

func (db *ChainDb) GetUnspentTxData(txsha *wire.Hash) (uint64, int, int, error) {
	// TODO: lock?
	height, txOffset, txLen, _, err := db.getTxData(txsha)
	if err != nil {
		return 0, 0, 0, err
	} else {
		return height, txOffset, txLen, nil
	}
}

// Returns database.ErrTxShaMissing if txsha not exist
func (db *ChainDb) FetchLastFullySpentTxBeforeHeight(txsha *wire.Hash,
	height uint64) (msgtx *wire.MsgTx, blkHeight uint64, blksha *wire.Hash, err error) {

	key := shaSpentTxToKey(txsha)
	value, err := db.stor.Get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			err = database.ErrTxShaMissing
		}
		return nil, 0, nil, err
	}
	if len(value)%20 != 0 {
		return nil, 0, nil, fmt.Errorf("expected multiple of 20 bytes, but get %d bytes", len(value))
	}

	i := len(value)/20 - 1
	for ; i >= 0; i-- {
		offset := i * 20
		blkHeight = binary.LittleEndian.Uint64(value[offset : offset+8])
		if blkHeight >= height {
			continue
		}
		txOffset := binary.LittleEndian.Uint32(value[offset+8 : offset+12])
		txLen := binary.LittleEndian.Uint32(value[offset+12 : offset+16])
		msgtx, blksha, err = db.fetchTxDataByLoc(blkHeight, int(txOffset), int(txLen))
		if err != nil {
			return nil, 0, nil, err
		}
		break
	}
	return
}

// Returns storage.ErrNotFound if txsha not exist
func (db *ChainDb) getTxFullySpent(txsha *wire.Hash) ([]*spentTx, error) {

	var badTxList, spentTxList []*spentTx

	key := shaSpentTxToKey(txsha)
	buf, err := db.stor.Get(key)
	if err != nil {
		return badTxList, err
	}
	txListLen := len(buf) / 20

	spentTxList = make([]*spentTx, txListLen)
	for i := range spentTxList {
		offset := i * 20

		blkHeight := binary.LittleEndian.Uint64(buf[offset : offset+8])
		txOff := binary.LittleEndian.Uint32(buf[offset+8 : offset+12])
		txLen := binary.LittleEndian.Uint32(buf[offset+12 : offset+16])
		numTxO := binary.LittleEndian.Uint32(buf[offset+16 : offset+20])

		sTx := spentTx{
			blkHeight: blkHeight,
			txoff:     int(txOff),
			txlen:     int(txLen),
			numTxO:    int(numTxO),
		}

		spentTxList[i] = &sTx
	}

	return spentTxList, nil
}

func (db *ChainDb) formatTxFullySpent(sTxList []*spentTx) []byte {
	txW := make([]byte, 20*len(sTxList))

	for i, sTx := range sTxList {
		blkHeight := uint64(sTx.blkHeight)
		txOff := uint32(sTx.txoff)
		txLen := uint32(sTx.txlen)
		numTxO := uint32(sTx.numTxO)
		offset := i * 20

		binary.LittleEndian.PutUint64(txW[offset:offset+8], blkHeight)
		binary.LittleEndian.PutUint32(txW[offset+8:offset+12], txOff)
		binary.LittleEndian.PutUint32(txW[offset+12:offset+16], txLen)
		binary.LittleEndian.PutUint32(txW[offset+16:offset+20], numTxO)
	}

	return txW
}

// ExistsTxSha returns if the given tx sha exists in the database
func (db *ChainDb) ExistsTxSha(txsha *wire.Hash) (bool, error) {

	return db.existsTxSha(txsha)
}

// existsTxSha returns if the given tx sha exists in the database.o
// Must be called with the db lock held.
func (db *ChainDb) existsTxSha(txSha *wire.Hash) (bool, error) {
	key := shaTxToKey(txSha)

	return db.stor.Has(key)
}

// FetchTxByShaList returns the most recent tx of the name fully spent or not
func (db *ChainDb) FetchTxByShaList(txShaList []*wire.Hash) []*database.TxReply {

	// until the fully spent separation of tx is complete this is identical
	// to FetchUnSpentTxByShaList
	replies := make([]*database.TxReply, len(txShaList))
	for i, txsha := range txShaList {
		tx, blockSha, height, txspent, err := db.fetchTxDataBySha(txsha)
		btxspent := []bool{}
		if err == nil {
			btxspent = make([]bool, len(tx.TxOut))
			for idx := range tx.TxOut {
				byteidx := idx / 8
				byteoff := uint(idx % 8)
				btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
			}
		}
		if err == database.ErrTxShaMissing {
			// if the unspent pool did not have the tx,
			// look in the fully spent pool (only last instance)
			tx, height, blockSha, err = db.FetchLastFullySpentTxBeforeHeight(txsha, math.MaxUint64)
			if err != nil && err != database.ErrTxShaMissing {
				logging.CPrint(logging.WARN, "FetchLastFullySpentTxBeforeHeight error",
					logging.LogFormat{"err": err, "tx": txsha.String()})
			}
			if err == nil {
				btxspent = make([]bool, len(tx.TxOut))
				for i := range btxspent {
					btxspent[i] = true
				}
			}
		}
		txlre := database.TxReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// FetchUnSpentTxByShaList given a array of Hash, look up the transactions
// and return them in a TxReply array.
func (db *ChainDb) FetchUnSpentTxByShaList(txShaList []*wire.Hash) []*database.TxReply {
	replies := make([]*database.TxReply, len(txShaList))
	for i, txsha := range txShaList {
		tx, blockSha, height, txspent, err := db.fetchTxDataBySha(txsha)
		btxspent := []bool{}
		if err == nil {
			btxspent = make([]bool, len(tx.TxOut))
			for idx := range tx.TxOut {
				byteidx := idx / 8
				byteoff := uint(idx % 8)
				btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
			}
		}
		txlre := database.TxReply{Sha: txsha, Tx: tx, BlkSha: blockSha, Height: height, TxSpent: btxspent, Err: err}
		replies[i] = &txlre
	}
	return replies
}

// fetchTxDataBySha returns several pieces of data regarding the given sha.
func (db *ChainDb) fetchTxDataBySha(txsha *wire.Hash) (rtx *wire.MsgTx, rblksha *wire.Hash, rheight uint64, rtxspent []byte, err error) {
	var txOff, txLen int

	rheight, txOff, txLen, rtxspent, err = db.getTxData(txsha)
	if err != nil {
		if err == storage.ErrNotFound {
			err = database.ErrTxShaMissing
		}
		return
	}
	rtx, rblksha, err = db.fetchTxDataByLoc(rheight, txOff, txLen)
	if err != nil {
		rtxspent = nil
	}
	return
}

// fetchTxDataByLoc returns several pieces of data regarding the given tx
// located by the block/offset/size location
func (db *ChainDb) fetchTxDataByLoc(blkHeight uint64, txOff int, txLen int) (rtx *wire.MsgTx, rblksha *wire.Hash, err error) {
	var blksha *wire.Hash
	var blkbuf []byte

	blksha, blkbuf, err = db.getBlkByHeight(blkHeight)
	if err != nil {
		if err == storage.ErrNotFound {
			err = database.ErrTxShaMissing
		}
		return
	}

	//log.Trace("transaction %v is at block %v %v txoff %v, txlen %v\n",
	//	txsha, blksha, blkHeight, txOff, txLen)

	if len(blkbuf) < txOff+txLen {
		err = database.ErrTxShaMissing
		return
	}

	var tx wire.MsgTx
	err = tx.SetBytes(blkbuf[txOff:txOff+txLen], wire.DB)
	if err != nil {
		logging.CPrint(logging.WARN, "unable to decode tx",
			logging.LogFormat{"blockHash": blksha, "blockHeight": blkHeight, "txoff": txOff, "txlen": txLen})
		return
	}

	return &tx, blksha, nil
}

func (db *ChainDb) FetchTxByLoc(blkHeight uint64, txOff int, txLen int) (*wire.MsgTx, error) {
	msgtx, _, err := db.fetchTxDataByLoc(blkHeight, txOff, txLen)
	if err != nil {
		return nil, err
	}
	return msgtx, nil
}

// FetchTxBySha returns some data for the given Tx Sha.
// Be careful, main chain may be revoked during invocation.
func (db *ChainDb) FetchTxBySha(txsha *wire.Hash) ([]*database.TxReply, error) {

	// fully spent
	sTxList, fSerr := db.getTxFullySpent(txsha)
	if fSerr != nil && fSerr != storage.ErrNotFound {
		return []*database.TxReply{}, fSerr
	}

	replies := make([]*database.TxReply, 0, len(sTxList)+1)
	for _, stx := range sTxList {
		tx, blksha, err := db.fetchTxDataByLoc(stx.blkHeight, stx.txoff, stx.txlen)
		if err != nil {
			if err != database.ErrTxShaMissing {
				return []*database.TxReply{}, err
			}
			continue
		}
		btxspent := make([]bool, len(tx.TxOut))
		for i := range btxspent {
			btxspent[i] = true

		}
		txlre := database.TxReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: stx.blkHeight, TxSpent: btxspent, Err: nil}
		replies = append(replies, &txlre)
	}

	// not fully spent
	tx, blksha, height, txspent, txerr := db.fetchTxDataBySha(txsha)
	if txerr != nil && txerr != database.ErrTxShaMissing {
		return []*database.TxReply{}, txerr
	}
	if txerr == nil {
		btxspent := make([]bool, len(tx.TxOut), len(tx.TxOut))
		for idx := range tx.TxOut {
			byteidx := idx / 8
			byteoff := uint(idx % 8)
			btxspent[idx] = (txspent[byteidx] & (byte(1) << byteoff)) != 0
		}

		txlre := database.TxReply{Sha: txsha, Tx: tx, BlkSha: blksha, Height: height, TxSpent: btxspent, Err: nil}
		replies = append(replies, &txlre)
	}
	return replies, nil
}

// addrIndexToKey serializes the passed txAddrIndex for storage within the DB.
// We want to use BigEndian to store at least block height and TX offset
// in order to ensure that the transactions are sorted in the index.
// This gives us the ability to use the index in more client-side
// applications that are order-dependent (specifically by dependency).
//func addrIndexToKey(index *txAddrIndex) []byte {
//	record := make([]byte, addrIndexKeyLength, addrIndexKeyLength)
//	copy(record[0:3], addrIndexKeyPrefix)
//	record[3] = index.addrVersion
//	copy(record[4:24], index.hash160[:])
//
//	// The index itself.
//	binary.LittleEndian.PutUint64(record[24:32], index.blkHeight)
//	binary.LittleEndian.PutUint32(record[32:36], uint32(index.txoffset))
//	binary.LittleEndian.PutUint32(record[36:40], uint32(index.txlen))
//	binary.LittleEndian.PutUint32(record[40:44], index.index)
//
//	return record
//}

// unpackTxIndex deserializes the raw bytes of a address tx index.
//func unpackTxIndex(rawIndex [20]byte) *txAddrIndex {
//	return &txAddrIndex{
//		blkHeight: binary.LittleEndian.Uint64(rawIndex[0:8]),
//		txoffset:  int(binary.LittleEndian.Uint32(rawIndex[8:12])),
//		txlen:     int(binary.LittleEndian.Uint32(rawIndex[12:16])),
//		index:     binary.LittleEndian.Uint32(rawIndex[16:20]),
//	}
//}

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

func (db *ChainDb) fetchInStakingTx(nowHeight uint64) (map[[sha256.Size]byte][]database.StakingTxInfo, error) {

	stakingTxInfos := make(map[[sha256.Size]byte][]database.StakingTxInfo)

	iter := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iter.Release()

	i := 0
	for iter.Next() {
		i++
		key := iter.Key()
		expiredHeight := binary.LittleEndian.Uint64(key[len(recordStakingTx) : len(recordStakingTx)+8])
		mapKey := mustDecodeStakingTxMapKey(key[len(recordStakingTx)+8:])
		blkHeight := mapKey.blockHeight

		value := iter.Value()
		scriptHash, amount := mustDecodeStakingTxValue(value)

		if nowHeight-blkHeight >= consensus.StakingTxRewardStart {
			stakingTxInfos[scriptHash] = append(stakingTxInfos[scriptHash], database.StakingTxInfo{
				Value:        amount,
				FrozenPeriod: expiredHeight - blkHeight,
				BlkHeight:    blkHeight})
		}
	}
	logging.CPrint(logging.DEBUG, "count of inStaking tx", logging.LogFormat{"number": i})
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return stakingTxInfos, nil
}

func (db *ChainDb) FetchRankStakingTx(height uint64) ([]database.Rank, error) {

	stakingTxInfos, err := db.fetchInStakingTx(height)
	if err != nil {
		return nil, err
	}
	//logging.CPrint(logging.INFO, "FetchRankStakingTx", logging.LogFormat{"staking_txs": stakingTxInfos})
	sortedStakingTx, err := database.SortMapByValue(stakingTxInfos, height, true)
	//logging.CPrint(logging.INFO, "after sort", logging.LogFormat{"sort": sortedStakingTx})
	if err != nil {
		return nil, err
	}

	count := len(sortedStakingTx)
	rankList := make([]database.Rank, count)
	for i := 0; i < count; i++ {
		rankList[i].ScriptHash = sortedStakingTx[i].Key
		rankList[i].Value = sortedStakingTx[i].Value

	}
	return rankList, nil
}

func (db *ChainDb) FetchRewardStakingTx(height uint64) ([]database.Reward, uint32, error) {
	stakingTxInfos, err := db.fetchInStakingTx(height)
	if err != nil {
		return nil, 0, err
	}
	SortedStakingTx, err := database.SortMapByValue(stakingTxInfos, height, true)
	if err != nil {
		return nil, 0, err
	}
	count := len(SortedStakingTx)
	reward := make([]database.Reward, count)
	for i := 0; i < count; i++ {
		reward[i].Rank = int32(i)
		reward[i].ScriptHash = SortedStakingTx[i].Key
		reward[i].Weight = SortedStakingTx[i].Weight
		reward[i].StakingTx = stakingTxInfos[SortedStakingTx[i].Key]
	}
	return reward, uint32(count), nil
}

func (db *ChainDb) FetchInStakingTx(height uint64) ([]database.Reward, uint32, error) {

	stakingTxInfos, err := db.fetchInStakingTx(height)
	if err != nil {
		return nil, 0, err
	}
	SortedStakingTx, err := database.SortMapByValue(stakingTxInfos, height, false)
	if err != nil {
		return nil, 0, err
	}
	count := len(SortedStakingTx)
	reward := make([]database.Reward, count)
	for i := 0; i < count; i++ {
		reward[i].Rank = int32(i)
		reward[i].ScriptHash = SortedStakingTx[i].Key
		reward[i].Weight = SortedStakingTx[i].Weight
		reward[i].StakingTx = stakingTxInfos[SortedStakingTx[i].Key]
	}
	return reward, uint32(count), nil
}
