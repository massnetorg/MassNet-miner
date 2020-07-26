package ldb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/debug"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
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
		if debug.DevMode() {
			aSha := msgtx.TxHash()
			if !bytes.Equal(aSha[:], txsha[:]) {
				logging.CPrint(logging.FATAL, fmt.Sprintf("mismatched tx hash, expect: %v, actual: %v", txsha, aSha))
			}
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
	if debug.DevMode() {
		aSha := rtx.TxHash()
		if !bytes.Equal(aSha[:], txsha[:]) {
			logging.CPrint(logging.FATAL, fmt.Sprintf("mismatched tx hash, expect: %v, actual: %v", txsha, aSha))
		}
	}
	return
}

// fetchTxDataByLoc returns several pieces of data regarding the given tx
// located by the block/offset/size location
func (db *ChainDb) fetchTxDataByLoc(blkHeight uint64, txOff int, txLen int) (rtx *wire.MsgTx, rblksha *wire.Hash, err error) {

	blksha, fileNo, blkOffset, _, err := db.getBlkLocByHeight(blkHeight)
	if err != nil {
		return nil, nil, err
	}
	buf, err := db.blkFileKeeper.ReadRawTx(fileNo, blkOffset, int64(txOff), txLen)
	if err != nil {
		return nil, nil, err
	}

	var tx wire.MsgTx
	err = tx.SetBytes(buf, wire.DB)
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

func (db *ChainDb) FetchTxByFileLoc(blkLoc *database.BlockLoc, txLoc *wire.TxLoc) (*wire.MsgTx, error) {
	buf, err := db.blkFileKeeper.ReadRawTx(blkLoc.File, int64(blkLoc.Offset), int64(txLoc.TxStart), txLoc.TxLen)
	if err != nil {
		return nil, err
	}

	var tx wire.MsgTx
	err = tx.SetBytes(buf, wire.DB)
	if err != nil {
		logging.CPrint(logging.WARN, "unable to decode tx",
			logging.LogFormat{
				"err":     err,
				"blkfile": blkLoc.File,
				"blkoff":  blkLoc.Offset,
				"blklen":  blkLoc.Length,
				"txoff":   txLoc.TxStart,
				"txlen":   txLoc.TxLen,
			})
		return nil, err
	}
	return &tx, nil
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
		if debug.DevMode() {
			aSha := tx.TxHash()
			if !bytes.Equal(aSha[:], txsha[:]) {
				logging.CPrint(logging.FATAL, fmt.Sprintf("mismatched tx hash, expect: %v, actual: %v", txsha, aSha))
			}
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
