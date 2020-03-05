package blockchain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/crypto/ripemd160"
	"massnet.org/mass/database"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

const (
	txIndexKeyLen       = sha256.Size + 4 + 4
	btxIndexKeyLen      = ripemd160.Size + 4 + 4 + 4
	btxSpentIndexKeyLen = ripemd160.Size + 4 + 4 + 8 + 4 + 4 + 4
)

type Server interface {
	Stop() error
}

// AddrIndexer provides a concurrent service for indexing the transactions of
// target blocks based on the addresses involved in the transaction.
type AddrIndexer struct {
	server      Server
	blockLogger *BlockProgressLogger
	db          database.Db
	sync.Mutex
}

type shTxLoc map[[txIndexKeyLen]byte]struct{}

type btxIndex map[[btxIndexKeyLen]byte]struct{}

type btxSpentIndex map[[btxSpentIndexKeyLen]byte]struct{}

func mustEncodeTxIndexKey(scriptHash []byte, txOffset, txLen int) []byte {
	key := make([]byte, txIndexKeyLen)
	copy(key[:sha256.Size], scriptHash)
	binary.LittleEndian.PutUint32(key[sha256.Size:sha256.Size+4], uint32(txOffset))
	binary.LittleEndian.PutUint32(key[sha256.Size+4:txIndexKeyLen], uint32(txLen))
	return key
}

func mustDecodeTxIndexKey(key [txIndexKeyLen]byte) ([sha256.Size]byte, int, int) {
	var scriptHash [sha256.Size]byte
	copy(scriptHash[:], key[:sha256.Size])
	txOffset := binary.LittleEndian.Uint32(key[sha256.Size : sha256.Size+4])
	txLen := binary.LittleEndian.Uint32(key[sha256.Size+4 : txIndexKeyLen])
	return scriptHash, int(txOffset), int(txLen)
}

func mustEncodeGtxIndexKey(scriptHash []byte, txOffset, txLen, index int) []byte {
	key := make([]byte, btxIndexKeyLen)
	copy(key[:ripemd160.Size], scriptHash)
	binary.LittleEndian.PutUint32(key[ripemd160.Size:ripemd160.Size+4], uint32(txOffset))
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4:ripemd160.Size+4+4], uint32(txLen))
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4+4:btxIndexKeyLen], uint32(index))
	return key
}

func mustDecodeGtxIndexKey(key [btxIndexKeyLen]byte) ([ripemd160.Size]byte, int, int, uint32) {
	var scriptHash [ripemd160.Size]byte
	copy(scriptHash[:], key[:ripemd160.Size])
	txOffset := binary.LittleEndian.Uint32(key[ripemd160.Size : ripemd160.Size+4])
	txLen := binary.LittleEndian.Uint32(key[ripemd160.Size+4 : ripemd160.Size+4+4])
	index := binary.LittleEndian.Uint32(key[ripemd160.Size+4+4 : btxIndexKeyLen])
	return scriptHash, int(txOffset), int(txLen), index
}

func mustEncodeGtxSpentIndexKey(scriptHash []byte, stxTxOffset, stxTxLen int, btxHeight uint64, btxTxOffset, btxTxLen int, gtxTxOutIndex uint32) []byte {
	key := make([]byte, btxSpentIndexKeyLen)
	copy(key[:ripemd160.Size], scriptHash)
	binary.LittleEndian.PutUint32(key[ripemd160.Size:ripemd160.Size+4], uint32(stxTxOffset))
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4:ripemd160.Size+4+4], uint32(stxTxLen))
	binary.LittleEndian.PutUint64(key[ripemd160.Size+4+4:ripemd160.Size+4+4+8], btxHeight)
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4+4+8:ripemd160.Size+4+4+8+4], uint32(btxTxOffset))
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4+4+8+4:ripemd160.Size+4+4+8+4+4], uint32(btxTxLen))
	binary.LittleEndian.PutUint32(key[ripemd160.Size+4+4+8+4+4:btxSpentIndexKeyLen], gtxTxOutIndex)
	return key
}

func mustDecodeGtxSpentIndexKey(key [btxSpentIndexKeyLen]byte) ([ripemd160.Size]byte, int, int, uint64, int, int, uint32) {
	var scriptHash [ripemd160.Size]byte
	copy(scriptHash[:], key[:ripemd160.Size])
	stxTxOffset := binary.LittleEndian.Uint32(key[ripemd160.Size : ripemd160.Size+4])
	stxTxLen := binary.LittleEndian.Uint32(key[ripemd160.Size+4 : ripemd160.Size+4+4])
	btxHeight := binary.LittleEndian.Uint64(key[ripemd160.Size+4+4 : ripemd160.Size+4+4+8])
	btxTxOffset := binary.LittleEndian.Uint32(key[ripemd160.Size+4+4+8 : ripemd160.Size+4+4+8+4])
	btxTxLen := binary.LittleEndian.Uint32(key[ripemd160.Size+4+4+8+4 : ripemd160.Size+4+4+8+4+4])
	btxTxOutIndex := binary.LittleEndian.Uint32(key[ripemd160.Size+4+4+8+4+4 : btxSpentIndexKeyLen])
	return scriptHash, int(stxTxOffset), int(stxTxLen), btxHeight, int(btxTxOffset), int(btxTxLen), btxTxOutIndex
}

// newAddrIndexer creates a new block address indexer.
// Use Start to begin processing incoming index jobs.
func NewAddrIndexer(db database.Db, server Server) (*AddrIndexer, error) {
	_, _, err := db.FetchAddrIndexTip()
	if err != nil && err != database.ErrAddrIndexDoesNotExist {
		return nil, err
	}

	ai := &AddrIndexer{
		db:          db,
		server:      server,
		blockLogger: NewBlockProgressLogger("process"),
	}
	return ai, nil
}

// insertTxAddressIndex synchronously queues a newly solved block to have its
// transactions indexed by address.
func (a *AddrIndexer) insertTxAddressIndex(block *massutil.Block, txStore TxStore) error {
	currentIndexSha, currentIndexTip, err := a.db.FetchAddrIndexTip()
	if err != nil {
		return err
	}
	if block.Height() == currentIndexTip+1 && block.MsgBlock().Header.Previous.IsEqual(currentIndexSha) {
		sha := block.Hash()
		height := block.Height()

		txAddrIndex, btxAddrIndex, btxSpentIndex, err := a.indexBlockAddrs(block, txStore)
		if err != nil {
			logging.CPrint(logging.ERROR,
				"Unable to index transactions of block",
				logging.LogFormat{
					"block hash": sha.String(),
					"height":     height,
					"error":      err})
			a.stopServer()
			return err
		}

		addrIndexData := &database.AddrIndexData{
			TxIndex:             txAddrIndex,
			BindingTxIndex:      btxAddrIndex,
			BindingTxSpentIndex: btxSpentIndex,
		}
		err = a.db.SubmitAddrIndex(sha, height, addrIndexData)
		if err != nil {
			logging.CPrint(logging.ERROR, "Unable to write index for block",
				logging.LogFormat{
					"block hash": sha.String(),
					"height":     height,
					"error":      err,
				})
			a.stopServer()
			return err
		}
		return nil
	} else {
		logging.CPrint(logging.ERROR, errUnexpectedHeight.Error(), logging.LogFormat{
			"excepted height(insert)":        currentIndexTip + 1,
			"block Height":                   block.Height(),
			"excepted previous hash(insert)": currentIndexSha,
			"block hash":                     block.MsgBlock().Header.Previous,
		})
		return errUnexpectedHeight
	}

}

func (a *AddrIndexer) deleteTxAddressIndex(blkSha *wire.Hash, blkHeight uint64) error {
	currentIndexSha, currentIndexTip, err := a.db.FetchAddrIndexTip()
	if err != nil {
		return err
	}
	if blkHeight == currentIndexTip && blkSha.IsEqual(currentIndexSha) {
		err := a.db.DeleteAddrIndex(blkSha, blkHeight)
		if err != nil {
			logging.CPrint(logging.ERROR, "Unable to write index for block",
				logging.LogFormat{
					"block hash": blkSha.String(),
					"height":     blkHeight,
					"error":      err,
				})
			a.stopServer()
			return err
		}
		return nil
	} else {
		logging.CPrint(logging.ERROR, errUnexpectedHeight.Error(), logging.LogFormat{
			"excepted height(delete)": currentIndexTip,
			"block height":            blkHeight,
		})
		return errUnexpectedHeight
	}

}

func indexScriptPubKeyForTxIn(txAddrIndex shTxLoc, btxSpentIndex btxSpentIndex, scriptPubKey []byte, locInBlock *wire.TxLoc,
	blkHeightBefore uint64, txLocBefore *wire.TxLoc, txBeforeIndex uint32) error {
	scriptClass, pops := txscript.GetScriptInfo(scriptPubKey)
	switch scriptClass {
	case txscript.WitnessV0ScriptHashTy, txscript.StakingScriptHashTy, txscript.BindingScriptHashTy:
	default:
		logging.CPrint(logging.DEBUG, "nonstandard tx")
		return nil
	}

	if scriptClass == txscript.BindingScriptHashTy {
		holderScriptHash, bindingScriptHash, err := txscript.GetParsedBindingOpcode(pops)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to parse binding opcode", logging.LogFormat{"error": err})
			return err
		}
		var txKey [txIndexKeyLen]byte
		copy(txKey[:], mustEncodeTxIndexKey(holderScriptHash, locInBlock.TxStart, locInBlock.TxLen))
		if _, ok := txAddrIndex[txKey]; !ok {
			txAddrIndex[txKey] = struct{}{}
		}

		var btxsKey [btxSpentIndexKeyLen]byte
		copy(btxsKey[:], mustEncodeGtxSpentIndexKey(bindingScriptHash, locInBlock.TxStart, locInBlock.TxLen, blkHeightBefore, txLocBefore.TxStart, txLocBefore.TxLen, txBeforeIndex))
		if _, ok := btxSpentIndex[btxsKey]; !ok {
			btxSpentIndex[btxsKey] = struct{}{}
		}
	} else {
		// WitnessV0ScriptHashTy or StakingScriptHashTy
		// TODO: more rigorous inspection
		_, rsh, err := txscript.GetParsedOpcode(pops, scriptClass)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to parse opcode")
			return err
		}
		var key [txIndexKeyLen]byte
		copy(key[:], mustEncodeTxIndexKey(rsh[:], locInBlock.TxStart, locInBlock.TxLen))
		if _, ok := txAddrIndex[key]; !ok {
			txAddrIndex[key] = struct{}{}
		}
	}
	return nil
}

func indexScriptPubKeyForTxOut(txAddrIndex shTxLoc, btxAddrIndex btxIndex, scriptPubKey []byte, locInBlock *wire.TxLoc, index int) error {
	scriptClass, pops := txscript.GetScriptInfo(scriptPubKey)
	switch scriptClass {
	case txscript.WitnessV0ScriptHashTy, txscript.StakingScriptHashTy, txscript.BindingScriptHashTy:
	default:
		logging.CPrint(logging.DEBUG, "nonstandard tx")
		return nil
	}

	if scriptClass == txscript.BindingScriptHashTy {
		holderScriptHash, bindingScriptHash, err := txscript.GetParsedBindingOpcode(pops)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to parse binding opcode", logging.LogFormat{"error": err})
			return err
		}
		var txAddrIndexKey [txIndexKeyLen]byte
		copy(txAddrIndexKey[:], mustEncodeTxIndexKey(holderScriptHash, locInBlock.TxStart, locInBlock.TxLen))
		if _, ok := txAddrIndex[txAddrIndexKey]; !ok {
			txAddrIndex[txAddrIndexKey] = struct{}{}
		}

		var btxAddrIndexKey [btxIndexKeyLen]byte
		copy(btxAddrIndexKey[:], mustEncodeGtxIndexKey(bindingScriptHash, locInBlock.TxStart, locInBlock.TxLen, index))
		if _, ok := btxAddrIndex[btxAddrIndexKey]; !ok {
			btxAddrIndex[btxAddrIndexKey] = struct{}{}
		}
	} else {
		_, rsh, err := txscript.GetParsedOpcode(pops, scriptClass)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to parse opcode")
			return err
		}
		var key [txIndexKeyLen]byte
		copy(key[:], mustEncodeTxIndexKey(rsh[:], locInBlock.TxStart, locInBlock.TxLen))
		if _, ok := txAddrIndex[key]; !ok {
			txAddrIndex[key] = struct{}{}
		}
	}
	return nil
}

func (a *AddrIndexer) indexBlockAddrs(blk *massutil.Block, txStore TxStore) (database.TxAddrIndex, database.BindingTxAddrIndex, database.BindingTxSpentAddrIndex, error) {
	addrIndex := make(database.TxAddrIndex)
	bindingTxAddrIndex := make(database.BindingTxAddrIndex)
	bindingTxSpentIndex := make(database.BindingTxSpentAddrIndex)
	txLocs, err := blk.TxLoc()
	if err != nil {
		return nil, nil, nil, err
	}

	txAddrIndex := make(shTxLoc)
	btxSpentIndex := make(btxSpentIndex)
	btxAddrIndex := make(btxIndex)

	txRecord := make(map[wire.Hash]int)
	for txIdx, tx := range blk.Transactions() {
		// Tx's offset and length in the block.
		txSha := tx.Hash()
		txRecord[*txSha] = txIdx

		locInBlock := &txLocs[txIdx]

		// Coinbases don't have any inputs.
		if !IsCoinBase(tx) {
			// Index the SPK's of each input's previous outpoint
			// transaction.
			for _, txIn := range tx.MsgTx().TxIn {
				// Lookup and fetch the referenced output's tx.
				prevOut := txIn.PreviousOutPoint
				txD, ok := txStore[prevOut.Hash]
				if !ok {
					return nil, nil, nil, fmt.Errorf("transaction %v not found",
						prevOut.Hash)
				}
				if txD.Err != nil {
					return nil, nil, nil, txD.Err
				}

				blkHeightBefore, txOffsetBefore, txLenBefore, err := a.db.GetUnspentTxData(&prevOut.Hash)
				if err != nil {
					txIdx, ok := txRecord[prevOut.Hash]
					if !ok {
						return nil, nil, nil, fmt.Errorf("transaction %v not found in both db and this block",
							prevOut.Hash)
					}
					txBeforeLoc := txLocs[txIdx]
					blkHeightBefore = blk.Height()
					txOffsetBefore = txBeforeLoc.TxStart
					txLenBefore = txBeforeLoc.TxLen
				}
				txBeforeLoc := &wire.TxLoc{
					TxStart: txOffsetBefore,
					TxLen:   txLenBefore,
				}
				err = indexScriptPubKeyForTxIn(txAddrIndex, btxSpentIndex, txD.Tx.MsgTx().TxOut[prevOut.Index].PkScript, locInBlock, blkHeightBefore, txBeforeLoc, prevOut.Index)
				if err != nil {
					// TODO: Assess the risk of this error
					return nil, nil, nil, err
				}

			}
		}

		for index, txOut := range tx.MsgTx().TxOut {
			err := indexScriptPubKeyForTxOut(txAddrIndex, btxAddrIndex, txOut.PkScript, locInBlock, index)
			if err != nil {
				return nil, nil, nil, err
			}
		}
	}

	for key := range txAddrIndex {
		scriptHash, txOffset, txLen := mustDecodeTxIndexKey(key)
		addrIndex[scriptHash] = append(addrIndex[scriptHash], &wire.TxLoc{TxStart: txOffset, TxLen: txLen})
	}

	for key := range btxAddrIndex {
		scriptHash, txOffset, txLen, index := mustDecodeGtxIndexKey(key)
		txLoc := &wire.TxLoc{TxStart: txOffset, TxLen: txLen}
		bindingTxAddrIndex[scriptHash] = append(bindingTxAddrIndex[scriptHash], &database.AddrIndexOutPoint{TxLoc: txLoc, Index: index})
	}

	for key := range btxSpentIndex {
		scriptHash, stxTxOffset, stxTxLen, btxHeight, btxTxOffset, btxTxLen, btxTxOutIndex := mustDecodeGtxSpentIndexKey(key)
		bindingTxSpentIndex[scriptHash] = append(bindingTxSpentIndex[scriptHash], &database.BindingTxSpent{
			SpentTxLoc:   &wire.TxLoc{TxStart: stxTxOffset, TxLen: stxTxLen},
			BTxBlkHeight: btxHeight,
			BTxLoc:       &wire.TxLoc{TxStart: btxTxOffset, TxLen: btxTxLen},
			BTxIndex:     btxTxOutIndex,
		})
	}

	return addrIndex, bindingTxAddrIndex, bindingTxSpentIndex, nil
}

func (a *AddrIndexer) SyncAttachBlock(block *massutil.Block, txStore TxStore) error {
	a.Lock()
	defer a.Unlock()
	if err := a.insertTxAddressIndex(block, txStore); err != nil {
		return err
	}
	a.logBlockHeight(block)
	return nil
}

func (a *AddrIndexer) SyncDetachBlock(block *massutil.Block) error {
	a.Lock()
	defer a.Unlock()
	return a.deleteTxAddressIndex(block.Hash(), block.Height())
}

func (a *AddrIndexer) stopServer() {
	go func() {
		if err := a.server.Stop(); err != nil {
			logging.CPrint(logging.FATAL, "AddrIndexer stop Server duet to error", logging.LogFormat{"err": err})
		}
	}()
}

func (a *AddrIndexer) logBlockHeight(blk *massutil.Block) {
	a.blockLogger.LogBlockHeight(blk)
}
