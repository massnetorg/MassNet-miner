package ldb

import (
	"bytes"
	"errors"
	"fmt"

	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

func (db *ChainDb) CheckBindingIndex(rebuild bool) error {
	_, bestHeight, err := db.NewestSha()
	if err != nil {
		return err
	}

	if rebuild {
		if err = db.removeBindingIndex(); err != nil {
			return err
		}
	}

	type TxInfo struct {
		txhash       wire.Hash
		height       uint64
		offset       int
		len          int
		isFullySpent bool
		spentBuf     []byte
	}
	spentBinding := make(map[wire.OutPoint]*bindingTxIndex)

	// count binding
	totalBinding := 0
	rebuildProg := 0
	rebuildStsg := 0
	rebuildHtgs := 0
	rebuildStg := 0
	for height := uint64(1); height <= bestHeight; height++ {
		_, buf, err := db.getBlkByHeight(height)
		if err != nil {
			return err
		}
		block, err := massutil.NewBlockFromBytes(buf, wire.DB)
		if err != nil {
			return err
		}

		for txIdx, tx := range block.MsgBlock().Transactions {
			if txIdx == 0 {
				// coinbase
				continue
			}

			var txInfo *TxInfo

			if rebuild {
				// query tx info
				list, err := db.getTxFullySpent(tx.TxHash().Ptr())
				if err == nil {
					for _, sptx := range list {
						if sptx.blkHeight == height {
							if sptx.numTxO <= 0 || sptx.numTxO%8 != 0 || sptx.numTxO < len(tx.TxOut) {
								return errors.New("TxOut number mismatch")
							}
							txInfo = &TxInfo{
								height:       height,
								offset:       sptx.txoff,
								len:          sptx.txlen,
								isFullySpent: true,
							}
							break
						}
					}
				}
				if err != nil && err != storage.ErrNotFound {
					logging.CPrint(logging.ERROR, "getTxFullySpent error", logging.LogFormat{
						"err":    err,
						"height": height,
						"tx":     tx.TxHash(),
					})
					return err
				}
				if txInfo == nil {
					txInfo = &TxInfo{}
					txInfo.height, txInfo.offset, txInfo.len, txInfo.spentBuf, err = db.getTxData(tx.TxHash().Ptr())
					if err != nil ||
						txInfo.height != height ||
						len(txInfo.spentBuf)*8 < len(tx.TxOut) {
						logging.CPrint(logging.ERROR, "getTxData error",
							logging.LogFormat{
								"err":      err,
								"height":   height,
								"tx":       tx.TxHash(),
								"txHeight": txInfo.height,
							})
						return errors.New("getTxData error")
					}
				}
				txInfo.txhash = tx.TxHash()

				// put "STSG"
				for _, txIn := range tx.TxIn {
					stgIndex, ok := spentBinding[txIn.PreviousOutPoint]
					if ok {
						stsgIndex := &bindingTxSpentIndex{
							scriptHash:       stgIndex.scriptHash,
							blkHeightSpent:   height,
							txOffsetSpent:    uint32(txInfo.offset),
							txLenSpent:       uint32(txInfo.len),
							blkHeightBinding: stgIndex.blkHeight,
							txOffsetBinding:  stgIndex.txOffset,
							txLenBinding:     stgIndex.txLen,
							indexBinding:     stgIndex.index,
						}
						btxsKey := bindingTxSpentIndexToKey(stsgIndex)
						if err = db.stor.Put(btxsKey, blankData); err != nil {
							return err
						}
						delete(spentBinding, txIn.PreviousOutPoint)
						rebuildStsg++
					}
				}
			}

			for vout, txOut := range tx.TxOut {
				class, pops := txscript.GetScriptInfo(txOut.PkScript)
				if class == txscript.BindingScriptHashTy {
					totalBinding++

					if rebuild {
						_, pocsha, err := txscript.GetParsedBindingOpcode(pops)
						if err != nil {
							return err
						}

						// if txout spent
						spent := false
						if txInfo.isFullySpent {
							spent = true
						} else {
							byteidx := vout / 8
							byteoff := uint(vout % 8)
							spent = txInfo.spentBuf[byteidx]&(byte(1)<<byteoff) != 0
						}

						var scriptHash [20]byte
						copy(scriptHash[:], pocsha)

						stgIndex := &bindingTxIndex{
							scriptHash: scriptHash,
							blkHeight:  height,
							txOffset:   uint32(txInfo.offset),
							txLen:      uint32(txInfo.len),
							index:      uint32(vout),
						}

						if !spent {
							// put "HTGS"
							htgsIndex := &bindingShIndex{
								blkHeight:  height,
								scriptHash: scriptHash,
							}
							htgsKey := bindingShIndexToKey(htgsIndex)
							if err = db.stor.Put(htgsKey, blankData); err != nil {
								return err
							}
							rebuildHtgs++

							// put "STG"
							stgKey := bindingTxIndexToKey(stgIndex)
							if err = db.stor.Put(stgKey, blankData); err != nil {
								return err
							}
							rebuildStg++
						} else {
							// already spent
							op := wire.OutPoint{Hash: txInfo.txhash, Index: uint32(vout)}
							spentBinding[op] = stgIndex
						}
					}
				}
			}
		}
		// print progress
		if rebuild && height*100/bestHeight >= uint64(rebuildProg)+5 {
			rebuildProg = int(height * 100 / bestHeight)
			logging.CPrint(logging.INFO, fmt.Sprintf("rebuild binding %d%%", rebuildProg), logging.LogFormat{})
		}
	}
	if len(spentBinding) != 0 {
		return errors.New("spentBinding is not empty")
	}

	logging.CPrint(logging.INFO, "check binding start", logging.LogFormat{
		"rebuild":      rebuild,
		"totalBinding": totalBinding,
		"rebuildStg":   rebuildStg,
		"rebuildStsg":  rebuildStsg,
		"rebuildHtgs":  rebuildHtgs,
	})

	return db.checkBindingIndex(bestHeight, totalBinding)
}

func (db *ChainDb) removeBindingIndex() error {

	count := 0
	iterHtgs := db.stor.NewIterator(storage.BytesPrefix(bindingShIndexPrefix))
	defer iterHtgs.Release()
	for iterHtgs.Next() {
		err := db.stor.Delete(iterHtgs.Key())
		if err != nil {
			logging.CPrint(logging.INFO, "remove htgs error", logging.LogFormat{"err": err})
			return err
		}
		count++
	}
	logging.CPrint(logging.INFO, "remove \"HTGS\"", logging.LogFormat{"count": count})

	count = 0
	iterStg := db.stor.NewIterator(storage.BytesPrefix(bindingTxIndexPrefix))
	defer iterStg.Release()
	for iterStg.Next() {
		err := db.stor.Delete(iterStg.Key())
		if err != nil {
			logging.CPrint(logging.INFO, "remove stg error", logging.LogFormat{"err": err})
			return err
		}
		count++
	}
	logging.CPrint(logging.INFO, "remove \"STG\"", logging.LogFormat{"count": count})

	count = 0
	iterStsg := db.stor.NewIterator(storage.BytesPrefix(bindingTxSpentIndexPrefix))
	defer iterStsg.Release()
	for iterStsg.Next() {
		err := db.stor.Delete(iterStsg.Key())
		if err != nil {
			logging.CPrint(logging.INFO, "remove stsg error", logging.LogFormat{"err": err})
			return err
		}
		count++
	}
	logging.CPrint(logging.INFO, "remove \"STSG\"", logging.LogFormat{"count": count})

	iter1 := db.stor.NewIterator(storage.BytesPrefix(bindingShIndexPrefix))
	defer iter1.Release()
	if iter1.Next() {
		return errors.New("incomplete deletion - htgs")
	}

	iter2 := db.stor.NewIterator(storage.BytesPrefix(bindingTxIndexPrefix))
	defer iter2.Release()
	if iter2.Next() {
		return errors.New("incomplete deletion - stg")
	}

	iter3 := db.stor.NewIterator(storage.BytesPrefix(bindingTxSpentIndexPrefix))
	defer iter3.Release()
	if iter3.Next() {
		return errors.New("incomplete deletion - stsg")
	}

	return nil
}

func (db *ChainDb) checkBindingIndex(bestHeight uint64, totalBinding int) error {

	loadBlock := func(height uint64) (msgBlk *wire.MsgBlock, rawBlk []byte, err error) {
		_, rawBlk, err = db.getBlkByHeight(height)
		if err != nil {
			logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
			return nil, nil, err
		}

		blk, err := massutil.NewBlockFromBytes(rawBlk, wire.DB)
		if err != nil {
			return nil, nil, err
		}
		return blk.MsgBlock(), rawBlk, nil
	}

	countHtgs := 0
	countStg := 0
	countStsg := 0
	cacheBlocks := make(map[uint64]struct {
		msgBlk *wire.MsgBlock
		rawBlk []byte
	})

	logging.CPrint(logging.INFO, "check \"HTGS\" ...", logging.LogFormat{})
	//  "HTGS"
	iter1 := db.stor.NewIterator(storage.BytesPrefix(bindingShIndexPrefix))
	defer iter1.Release()
	for iter1.Next() {
		htgsKey, err := mustDecodeBindingShIndexKey(iter1.Key())
		if err != nil {
			logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
			return err
		}

		// load block
		cacheBlock, ok := cacheBlocks[htgsKey.blkHeight]
		if !ok {
			mb, rb, err := loadBlock(htgsKey.blkHeight)
			if err != nil {
				logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
				return err
			}
			cacheBlock = struct {
				msgBlk *wire.MsgBlock
				rawBlk []byte
			}{
				msgBlk: mb,
				rawBlk: rb,
			}
			cacheBlocks[htgsKey.blkHeight] = cacheBlock
		}

		found := false
	loop:
		for i, tx := range cacheBlock.msgBlk.Transactions {
			if i == 0 {
				continue
			}
			for j, txout := range tx.TxOut {
				class, pops := txscript.GetScriptInfo(txout.PkScript)
				if class == txscript.BindingScriptHashTy {
					_, pocsh, err := txscript.GetParsedBindingOpcode(pops)
					if err != nil {
						logging.CPrint(logging.ERROR, "GetParsedBindingOpcode error", logging.LogFormat{
							"height":   cacheBlock.msgBlk.Header.Height,
							"tx":       tx.TxHash(),
							"tx_index": i,
							"vout":     j,
						})
						return err
					}
					if bytes.Equal(pocsh, htgsKey.scriptHash[:]) {
						found = true
						break loop
					}
				}
			}
		}
		if !found {
			logging.CPrint(logging.ERROR, "binding not found", logging.LogFormat{
				"height":     htgsKey.blkHeight,
				"scripthash": htgsKey.scriptHash,
			})
			return errors.New("binding not found")
		}
		countHtgs++
	}

	logging.CPrint(logging.INFO, "check \"HTGS\" done", logging.LogFormat{"total_HTGS": countHtgs})

	logging.CPrint(logging.INFO, "check \"STG\" ...", logging.LogFormat{})
	//  "STG"
	iter2 := db.stor.NewIterator(storage.BytesPrefix(bindingTxIndexPrefix))
	defer iter2.Release()
	for iter2.Next() {
		stgKey, err := mustDecodeBindingTxIndexKey(iter2.Key())
		if err != nil {
			logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
			return err
		}
		cacheBlock, ok := cacheBlocks[stgKey.blkHeight]
		if !ok {
			logging.CPrint(logging.ERROR, "stg block missing", logging.LogFormat{
				"height":     stgKey.blkHeight,
				"scripthash": stgKey.scriptHash,
			})
			return errors.New("block missing")
		}
		var tx wire.MsgTx
		err = tx.SetBytes(cacheBlock.rawBlk[stgKey.txOffset:stgKey.txOffset+stgKey.txLen], wire.DB)
		if err != nil {
			logging.CPrint(logging.ERROR, "unable to decode tx",
				logging.LogFormat{
					"err":         err,
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
				})
			return err
		}

		// check script
		class, pops := txscript.GetScriptInfo(tx.TxOut[stgKey.index].PkScript)
		if class != txscript.BindingScriptHashTy {
			logging.CPrint(logging.ERROR, "not a binding",
				logging.LogFormat{
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
					"vout":        stgKey.index,
				})
			return errors.New("not a binding")
		}
		_, pocsh, err := txscript.GetParsedBindingOpcode(pops)
		if err != nil {
			logging.CPrint(logging.ERROR, "GetParsedBindingOpcode error",
				logging.LogFormat{
					"err":         err,
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
					"vout":        stgKey.index,
				})
			return err
		}
		if !bytes.Equal(pocsh, stgKey.scriptHash[:]) {
			logging.CPrint(logging.ERROR, "poc scripthash mismatch",
				logging.LogFormat{
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
					"vout":        stgKey.index,
				})
			return errors.New("poc scripthash mismatch")
		}

		// check spent
		height, offset, txlen, spentBuf, err := db.getTxData(tx.TxHash().Ptr())
		if err != nil {
			logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
			return err
		}
		if stgKey.blkHeight != height ||
			int(stgKey.txOffset) != offset ||
			int(stgKey.txLen) != txlen {
			logging.CPrint(logging.ERROR, "binding tx loc error",
				logging.LogFormat{
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
					"vout":        stgKey.index,
				})
			return errors.New("binding tx loc error")
		}
		byteidx := stgKey.index / 8
		byteoff := uint(stgKey.index % 8)
		if spentBuf[byteidx]&(byte(1)<<byteoff) != 0 {
			logging.CPrint(logging.ERROR, "binding already spent",
				logging.LogFormat{
					"blockHash":   cacheBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stgKey.blkHeight,
					"txoff":       stgKey.txOffset,
					"txlen":       stgKey.txLen,
					"vout":        stgKey.index,
				})
			return errors.New("binding already spent")
		}
		countStg++
	}

	logging.CPrint(logging.INFO, "check \"STG\" done", logging.LogFormat{"total_STG": countStg})

	logging.CPrint(logging.INFO, "check \"STSG\" ...", logging.LogFormat{})
	// "STSG"
	iter3 := db.stor.NewIterator(storage.BytesPrefix(bindingTxSpentIndexPrefix))
	defer iter3.Release()
	for iter3.Next() {
		stsgKey, err := mustDecodeBindingTxSpentIndexKey(iter3.Key())
		if err != nil {
			logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
			return err
		}

		if stsgKey.blkHeightBinding >= stsgKey.blkHeightSpent {
			logging.CPrint(logging.ERROR, "incorrent binding spent height",
				logging.LogFormat{
					"bind":  stsgKey.blkHeightBinding,
					"spent": stsgKey.blkHeightSpent,
				})
			return errors.New("incorrent binding spent height")
		}

		// block this binding withrawn
		spentBlock, ok := cacheBlocks[stsgKey.blkHeightSpent]
		if !ok {
			mb, rb, err := loadBlock(stsgKey.blkHeightSpent)
			if err != nil {
				logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
				return err
			}
			spentBlock = struct {
				msgBlk *wire.MsgBlock
				rawBlk []byte
			}{
				msgBlk: mb,
				rawBlk: rb,
			}
			cacheBlocks[stsgKey.blkHeightSpent] = spentBlock
		}
		// block this binding bound
		bindBlock, ok := cacheBlocks[stsgKey.blkHeightBinding]
		if !ok {
			mb, rb, err := loadBlock(stsgKey.blkHeightBinding)
			if err != nil {
				logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
				return err
			}
			bindBlock = struct {
				msgBlk *wire.MsgBlock
				rawBlk []byte
			}{
				msgBlk: mb,
				rawBlk: rb,
			}
			cacheBlocks[stsgKey.blkHeightBinding] = bindBlock
		}
		var bindTx wire.MsgTx
		err = bindTx.SetBytes(bindBlock.rawBlk[stsgKey.txOffsetBinding:stsgKey.txOffsetBinding+stsgKey.txLenBinding], wire.DB)
		if err != nil {
			logging.CPrint(logging.ERROR, "unable to decode bindTx",
				logging.LogFormat{
					"err": err,
				})
			return err
		}
		var spentTx wire.MsgTx
		err = spentTx.SetBytes(spentBlock.rawBlk[stsgKey.txOffsetSpent:stsgKey.txOffsetSpent+stsgKey.txLenSpent], wire.DB)
		if err != nil {
			logging.CPrint(logging.ERROR, "unable to decode spentTx",
				logging.LogFormat{
					"err": err,
				})
			return err
		}
		bindOp := wire.OutPoint{Hash: bindTx.TxHash(), Index: stsgKey.indexBinding}
		refBind := false
		for _, spentTxIn := range spentTx.TxIn {
			if spentTxIn.PreviousOutPoint == bindOp {
				refBind = true
				break
			}
		}
		if !refBind {
			logging.CPrint(logging.ERROR, "incorrect stsg item", logging.LogFormat{})
			return errors.New("incorrect stsg item")
		}

		// check binding script
		class, pops := txscript.GetScriptInfo(bindTx.TxOut[stsgKey.indexBinding].PkScript)
		if class != txscript.BindingScriptHashTy {
			logging.CPrint(logging.ERROR, "not a binding - stsg",
				logging.LogFormat{
					"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stsgKey.blkHeightBinding,
					"txoff":       stsgKey.txOffsetBinding,
					"txlen":       stsgKey.txLenBinding,
					"vout":        stsgKey.indexBinding,
				})
			return errors.New("not a binding")
		}
		_, pocsh, err := txscript.GetParsedBindingOpcode(pops)
		if err != nil {
			logging.CPrint(logging.ERROR, "GetParsedBindingOpcode error - stsg",
				logging.LogFormat{
					"err":         err,
					"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stsgKey.blkHeightBinding,
					"txoff":       stsgKey.txOffsetBinding,
					"txlen":       stsgKey.txLenBinding,
					"vout":        stsgKey.indexBinding,
				})
			return err
		}
		if !bytes.Equal(pocsh, stsgKey.scriptHash[:]) {
			logging.CPrint(logging.ERROR, "poc scripthash mismatch - stsg",
				logging.LogFormat{
					"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
					"blockHeight": stsgKey.blkHeightBinding,
					"txoff":       stsgKey.txOffsetBinding,
					"txlen":       stsgKey.txLenBinding,
					"vout":        stsgKey.indexBinding,
				})
			return errors.New("poc scripthash mismatch")
		}

		// check spent
		height, offset, txlen, spentBuf, err := db.getTxData(bindTx.TxHash().Ptr())
		if err != nil {
			if err != storage.ErrNotFound {
				logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
				return err
			}

			list, err := db.getTxFullySpent(bindTx.TxHash().Ptr())
			if err != nil {
				logging.CPrint(logging.ERROR, "--->", logging.LogFormat{"err": err})
				return err
			}
			found := false
			for _, sptTx := range list {
				if stsgKey.blkHeightBinding == sptTx.blkHeight &&
					int(stsgKey.txOffsetBinding) == sptTx.txoff &&
					int(stsgKey.txLenBinding) == sptTx.txlen &&
					int(stsgKey.indexBinding) < sptTx.numTxO {
					found = true
					break
				}
			}
			if !found {
				logging.CPrint(logging.ERROR, "binding tx loc error - stsg",
					logging.LogFormat{
						"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
						"blockHeight": stsgKey.blkHeightBinding,
						"txoff":       stsgKey.txOffsetBinding,
						"txlen":       stsgKey.txLenBinding,
						"vout":        stsgKey.indexBinding,
					})
				return errors.New("binding tx loc error - stsg")
			}
		} else {
			// tx is not fully spent
			if stsgKey.blkHeightBinding != height ||
				int(stsgKey.txOffsetBinding) != offset ||
				int(stsgKey.txLenBinding) != txlen {
				logging.CPrint(logging.ERROR, "binding tx loc error - stsg",
					logging.LogFormat{
						"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
						"blockHeight": stsgKey.blkHeightBinding,
						"txoff":       stsgKey.txOffsetBinding,
						"txlen":       stsgKey.txLenBinding,
						"vout":        stsgKey.indexBinding,
					})
				return errors.New("binding tx loc error - stsg")
			}
			byteidx := stsgKey.indexBinding / 8
			byteoff := uint(stsgKey.indexBinding % 8)
			if spentBuf[byteidx]&(byte(1)<<byteoff) == 0 {
				logging.CPrint(logging.ERROR, "output should not be unspent - stsg",
					logging.LogFormat{
						"blockHash":   bindBlock.msgBlk.Header.BlockHash(),
						"blockHeight": stsgKey.blkHeightBinding,
						"txoff":       stsgKey.txOffsetBinding,
						"txlen":       stsgKey.txLenBinding,
						"vout":        stsgKey.indexBinding,
					})
				return errors.New("output should not be unspent - stsg")
			}
		}
		countStsg++

	}
	logging.CPrint(logging.INFO, "check \"STSG\" done", logging.LogFormat{"total_STSG": countStsg})

	if countStg+countStsg != totalBinding {
		return errors.New("total_STSG+total_STG not equal totalBinding")
	}
	return nil
}
