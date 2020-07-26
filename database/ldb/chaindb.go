package ldb

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sync"

	"massnet.org/mass/database"
	"massnet.org/mass/database/disk"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

const (
	// params for ldb batch
	dbBatchCount   = 2
	blockBatch     = 0
	addrIndexBatch = 1

	blockStorageMetaDataLength = 40
)

var (
	dbStorageMetaDataKey = []byte("DBSTORAGEMETA")
)

type dbStorageMeta struct {
	currentHeight uint64
	currentHash   wire.Hash
}

func decodeDBStorageMetaData(bs []byte) (meta dbStorageMeta, err error) {
	if length := len(bs); length != blockStorageMetaDataLength {
		logging.CPrint(logging.ERROR, "invalid blockStorageMetaData", logging.LogFormat{"length": length, "data": bs})
		return dbStorageMeta{}, database.ErrInvalidBlockStorageMeta
	}
	copy(meta.currentHash[:], bs[:32])
	meta.currentHeight = binary.LittleEndian.Uint64(bs[32:])
	return meta, nil
}

func encodeDBStorageMetaData(meta dbStorageMeta) []byte {
	bs := make([]byte, blockStorageMetaDataLength)
	copy(bs, meta.currentHash[:])
	binary.LittleEndian.PutUint64(bs[32:], meta.currentHeight)
	return bs
}

// ChainDb holds internal state for databse.
type ChainDb struct {
	// lock preventing multiple entry
	dbLock sync.Mutex

	stor          storage.Storage
	blkFileKeeper *disk.BlockFileKeeper

	dbBatch       storage.Batch
	batches       [dbBatchCount]*LBatch
	dbStorageMeta dbStorageMeta

	txUpdateMap      map[wire.Hash]*txUpdateObj
	txSpentUpdateMap map[wire.Hash]*spentTxUpdate

	stakingTxMap        map[stakingTxMapKey]*stakingTx
	expiredStakingTxMap map[stakingTxMapKey]*stakingTx
}

func init() {
	for _, dbtype := range storage.RegisteredDbTypes() {
		tp := dbtype
		database.AddDBDriver(database.DriverDB{
			DbType: tp,
			CreateDB: func(args ...interface{}) (database.Db, error) {
				if len(args) < 1 {
					return nil, storage.ErrInvalidArgument
				}
				dbpath, ok := args[0].(string)
				if !ok {
					return nil, storage.ErrInvalidArgument
				}

				stor, err := storage.CreateStorage(tp, dbpath, nil)
				if err != nil {
					return nil, err
				}
				return NewChainDb(dbpath, stor)
			},
			OpenDB: func(args ...interface{}) (database.Db, error) {
				if len(args) < 1 {
					return nil, storage.ErrInvalidArgument
				}
				dbpath, ok := args[0].(string)
				if !ok {
					return nil, storage.ErrInvalidArgument
				}

				stor, err := storage.OpenStorage(tp, dbpath, nil)
				if err != nil {
					return nil, err
				}
				return NewChainDb(dbpath, stor)
			},
		})
	}
}

func NewChainDb(dbpath string, stor storage.Storage) (*ChainDb, error) {
	cdb := &ChainDb{
		stor:                stor,
		dbBatch:             stor.NewBatch(),
		txUpdateMap:         make(map[wire.Hash]*txUpdateObj),
		txSpentUpdateMap:    make(map[wire.Hash]*spentTxUpdate),
		stakingTxMap:        make(map[stakingTxMapKey]*stakingTx),
		expiredStakingTxMap: make(map[stakingTxMapKey]*stakingTx),
	}

	blockMeta, err := cdb.getBlockStorageMeta()
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		blockMeta = dbStorageMeta{
			currentHeight: UnknownHeight,
		}
		// if err = cdb.stor.Put(dbStorageMetaDataKey, encodeDBStorageMetaData(blockMeta)); err != nil {
		// 	return nil, err
		// }
	}
	cdb.dbStorageMeta = blockMeta

	// Load address indexer
	if err = cdb.checkAddrIndexVersion(); err == nil {
		logging.CPrint(logging.INFO, "address index good, continuing")
	} else {
		if err != storage.ErrNotFound {
			return nil, err
		}
		var b2 [2]byte
		binary.LittleEndian.PutUint16(b2[:], uint16(addrIndexCurrentVersion))
		if err = cdb.stor.Put(addrIndexVersionKey, b2[:]); err != nil {
			return nil, err
		}
	}

	// init or load blkXXXXX.dat
	blkDir := filepath.Join(filepath.Dir(dbpath), "blocks")
	fi, err := os.Stat(blkDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		err = os.MkdirAll(blkDir, 0755)
		if err != nil {
			return nil, err
		}
	} else if !fi.IsDir() {
		return nil, errors.New("file already exist: " + blkDir)
	}
	records, err := cdb.getAllBlockFileMeta()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		file0, err := cdb.initBlockFileMeta()
		if err != nil {
			logging.CPrint(logging.ERROR, "init block file meta failed", logging.LogFormat{"err": err})
			return nil, err
		}
		records = append(records, file0)
	}
	cdb.blkFileKeeper = disk.NewBlockFileKeeper(blkDir, records)
	return cdb, nil
}

func (db *ChainDb) close() error {
	return db.stor.Close()
}

// Sync verifies that the database is coherent on disk,
// and no outstanding transactions are in flight.
func (db *ChainDb) Sync() error {

	// while specified by the API, does nothing
	// however does grab lock to verify it does not return until other operations are complete.
	return nil
}

// Close cleanly shuts down database, syncing all data.
func (db *ChainDb) Close() error {
	db.blkFileKeeper.Close()
	return db.close()
}

func (db *ChainDb) getBlockStorageMeta() (dbStorageMeta, error) {
	if data, err := db.stor.Get(dbStorageMetaDataKey); err == nil {
		return decodeDBStorageMetaData(data)
	} else {
		return dbStorageMeta{}, err
	}
}

func (db *ChainDb) expire(currentHeight uint64) error {
	searchKey := stakingTxSearchKey(currentHeight)

	iter := db.stor.NewIterator(storage.BytesPrefix(searchKey))
	defer iter.Release()

	for iter.Next() {
		mapKeyBytes := iter.Key()[stakingTxSearchKeyLength:]
		value := iter.Value()
		mapKey := mustDecodeStakingTxMapKey(mapKeyBytes)

		scriptHash, val := mustDecodeStakingTxValue(value)

		// stakingTxMap
		db.stakingTxMap[mapKey] = &stakingTx{
			txSha:         &mapKey.txID,
			index:         mapKey.index,
			expiredHeight: currentHeight,
			rsh:           scriptHash,
			value:         val,
			blkHeight:     mapKey.blockHeight,
			delete:        true,
		}

		// expiredStakingTxMap
		db.expiredStakingTxMap[mapKey] = &stakingTx{
			txSha:         &mapKey.txID,
			index:         mapKey.index,
			expiredHeight: currentHeight,
			rsh:           scriptHash,
			value:         val,
			blkHeight:     mapKey.blockHeight,
		}

	}
	if err := iter.Error(); err != nil {
		return err
	}
	return nil
}

func (db *ChainDb) freeze(currentHeight uint64) error {
	searchKey := expiredStakingTxSearchKey(currentHeight)

	iter := db.stor.NewIterator(storage.BytesPrefix(searchKey))
	defer iter.Release()

	for iter.Next() {
		mapKeyBytes := iter.Key()[stakingTxSearchKeyLength:]
		value := iter.Value()
		mapKey := mustDecodeStakingTxMapKey(mapKeyBytes)

		scriptHash, val := mustDecodeStakingTxValue(value)

		// expiredStakingTxMap
		db.expiredStakingTxMap[mapKey] = &stakingTx{
			txSha:         &mapKey.txID,
			index:         mapKey.index,
			expiredHeight: currentHeight,
			rsh:           scriptHash,
			value:         val,
			blkHeight:     mapKey.blockHeight,
			delete:        true,
		}

		// stakingTxMap
		db.stakingTxMap[mapKey] = &stakingTx{
			txSha:         &mapKey.txID,
			index:         mapKey.index,
			expiredHeight: currentHeight,
			rsh:           scriptHash,
			value:         val,
			blkHeight:     mapKey.blockHeight,
		}

	}

	if err := iter.Error(); err != nil {
		return err
	}
	return nil
}

// doSpend iterates all TxIn in a mass transaction marking each associated
// TxOut as spent.
func (db *ChainDb) doSpend(tx *wire.MsgTx) error {
	if isCoinBaseTx(tx) {
		return nil
	}

	for _, txIn := range tx.TxIn {
		inTxSha := txIn.PreviousOutPoint.Hash
		inTxIdx := txIn.PreviousOutPoint.Index

		if inTxIdx == ^uint32(0) {
			continue
		}

		err := db.setSpentData(&inTxSha, inTxIdx)
		if err != nil {
			return err
		}
	}
	return nil
}

// unSpend iterates all TxIn in a mass transaction marking each associated
// TxOut as unspent.
func (db *ChainDb) unSpend(tx *wire.MsgTx) error {
	for txinidx := range tx.TxIn {
		txin := tx.TxIn[txinidx]

		inTxSha := txin.PreviousOutPoint.Hash
		inTxidx := txin.PreviousOutPoint.Index

		if inTxidx == ^uint32(0) {
			continue
		}

		err := db.clearSpentData(&inTxSha, inTxidx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *ChainDb) setSpentData(sha *wire.Hash, idx uint32) error {
	return db.setclearSpentData(sha, idx, true)
}

func (db *ChainDb) clearSpentData(sha *wire.Hash, idx uint32) error {
	return db.setclearSpentData(sha, idx, false)
}

func (db *ChainDb) setclearSpentData(txsha *wire.Hash, idx uint32, set bool) error {
	var txUo *txUpdateObj
	var ok bool

	if txUo, ok = db.txUpdateMap[*txsha]; !ok {
		// not cached, load from db
		var txU txUpdateObj
		blkHeight, txOff, txLen, spentData, err := db.getTxData(txsha)
		if err != nil {
			// setting a fully spent tx is an error.
			if err != storage.ErrNotFound || set {
				return err
			}
			// if we are clearing a tx and it wasn't found
			// in the tx table, it could be in the fully spent
			// (duplicates) table.
			spentTxList, err := db.getTxFullySpent(txsha)
			if err != nil {
				return err
			}

			// need to reslice the list to exclude the most recent.
			sTx := spentTxList[len(spentTxList)-1]
			if len(spentTxList) == 1 {
				// write entry to delete tx from spent pool
				db.txSpentUpdateMap[*txsha] = &spentTxUpdate{delete: true}
			} else {
				db.txSpentUpdateMap[*txsha] = &spentTxUpdate{txl: spentTxList[:len(spentTxList)-1]}
			}
			// Create 'new' Tx update data.
			blkHeight = sTx.blkHeight
			txOff = sTx.txoff
			txLen = sTx.txlen
			spentbuflen := (sTx.numTxO + 7) / 8
			spentData = make([]byte, spentbuflen)
			for i := range spentData {
				spentData[i] = ^byte(0)
			}
		}

		txU.txSha = txsha
		txU.blkHeight = blkHeight
		txU.txoff = txOff
		txU.txlen = txLen
		txU.spentData = spentData

		txUo = &txU
	}

	byteidx := idx / 8
	byteoff := idx % 8

	if set {
		txUo.spentData[byteidx] |= (byte(1) << byteoff)
	} else {
		txUo.spentData[byteidx] &= ^(byte(1) << byteoff)
	}

	// check for fully spent Tx
	fullySpent := true
	for _, val := range txUo.spentData {
		if val != ^byte(0) {
			fullySpent = false
			break
		}
	}
	if fullySpent {
		var txSu *spentTxUpdate
		// Look up Tx in fully spent table
		if txSuOld, ok := db.txSpentUpdateMap[*txsha]; ok {
			txSu = txSuOld
		} else {
			txSu = &spentTxUpdate{}
			txSuOld, err := db.getTxFullySpent(txsha)
			if err == nil {
				txSu.txl = txSuOld
			} else if err != storage.ErrNotFound {
				return err
			}
		}

		// Fill in spentTx
		var sTx spentTx
		sTx.blkHeight = txUo.blkHeight
		sTx.txoff = txUo.txoff
		sTx.txlen = txUo.txlen
		// XXX -- there is no way to comput the real TxOut
		// from the spent array.
		sTx.numTxO = 8 * len(txUo.spentData)

		// append this txdata to fully spent txlist
		txSu.txl = append(txSu.txl, &sTx)

		// mark txsha as deleted in the txUpdateMap
		logging.CPrint(logging.TRACE, "tx %v is fully spent", logging.LogFormat{"txHash": txsha})

		db.txSpentUpdateMap[*txsha] = txSu

		txUo.delete = true
	}

	db.txUpdateMap[*txsha] = txUo
	return nil
}

func makeBlockHeightKey(height uint64) []byte {
	var bs [blockHeightKeyLength]byte
	copy(bs[:], blockHeightKeyPrefix)
	binary.LittleEndian.PutUint64(bs[blockHeightKeyPrefixLength:], height)
	return bs[:]
}

func makeBlockShaKey(sha *wire.Hash) []byte {
	var bs [blockShaKeyLength]byte
	copy(bs[:], blockShaKeyPrefix)
	copy(bs[blockShaKeyPrefixLength:], (*sha)[:])
	return bs[:]
}

var recordSuffixTx = []byte("TXD")
var recordSuffixSpentTx = []byte("TXS")

func shaTxToKey(sha *wire.Hash) []byte {
	key := make([]byte, len(recordSuffixTx)+len(sha))
	copy(key, recordSuffixTx)
	copy(key[len(recordSuffixTx):], sha[:])
	return key
}

func shaSpentTxToKey(sha *wire.Hash) []byte {
	key := make([]byte, len(recordSuffixSpentTx)+len(sha))
	copy(key, recordSuffixSpentTx)
	copy(key[len(recordSuffixSpentTx):], sha[:])
	return key
}

func isCoinBaseTx(msgTx *wire.MsgTx) bool {
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || !prevOut.Hash.IsEqual(&wire.Hash{}) {
		return false
	}

	return true
}

func (db *ChainDb) Batch(index int) *LBatch {
	if db.batches[index] == nil {
		db.batches[index] = NewLBatch(db.dbBatch)
	}
	return db.batches[index]
}

// For testing purpose
func (db *ChainDb) TestExportDbEntries() map[string][]byte {
	it := db.stor.NewIterator(nil)
	defer it.Release()

	all := make(map[string][]byte)
	for it.Next() {
		all[string(it.Key())] = it.Value()
	}
	return all
}
