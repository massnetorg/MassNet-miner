/*
	This file is only used to check data correctness for 1.1.0
*/
package ldb

import (
	"encoding/binary"
	"fmt"

	"massnet.org/mass/database/storage"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

func (db *ChainDb) CheckTxIndex_1_1_0() error {

	type TxInfo struct {
		txhash       wire.Hash
		height       uint64
		loc          wire.TxLoc
		isFullySpent bool
		spentBuf     []byte
	}

	spentOutpoints := make(map[wire.OutPoint]wire.Hash)

	_, bestHeight, err := db.NewestSha()
	if err != nil {
		return err
	}

	prog := 0
	for height := uint64(0); height <= bestHeight; height++ {
		_, buf, err := db.getBlkByHeight(height)
		if err != nil {
			return err
		}
		block, err := massutil.NewBlockFromBytes(buf, wire.DB)
		if err != nil {
			return err
		}

		blkRelatedScriptHash := make(map[wire.Hash]map[wire.TxLoc]bool)

		for txIdx, tx := range block.MsgBlock().Transactions {
			var txInfo *TxInfo
			// query tx info
			list, err := db.getTxFullySpent(tx.TxHash().Ptr())
			if err == nil {
				for _, sptx := range list {
					if sptx.blkHeight == height {
						if sptx.numTxO <= 0 || sptx.numTxO%8 != 0 || sptx.numTxO < len(tx.TxOut) {
							return errors.New("TxOut number mismatch")
						}
						txInfo = &TxInfo{
							height: height,
							loc: wire.TxLoc{
								TxStart: sptx.txoff,
								TxLen:   sptx.txlen,
							},
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
				txInfo.height, txInfo.loc.TxStart, txInfo.loc.TxLen, txInfo.spentBuf, err = db.getTxData(tx.TxHash().Ptr())
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

			if txIdx != 0 { // skip coinbase
				for vin, txIn := range tx.TxIn {
					if scriptHash, ok := spentOutpoints[txIn.PreviousOutPoint]; ok {
						delete(spentOutpoints, txIn.PreviousOutPoint)
						if _, ok := blkRelatedScriptHash[scriptHash]; !ok {
							blkRelatedScriptHash[scriptHash] = make(map[wire.TxLoc]bool)
						}
						blkRelatedScriptHash[scriptHash][txInfo.loc] = true
					} else {
						logging.CPrint(logging.FATAL, "unexpected prev outpoint",
							logging.LogFormat{
								"height": height,
								"tx":     txInfo.txhash,
								"txidx":  txIdx,
								"vin":    vin,
							})
					}
				}
			}

			for vout, txOut := range tx.TxOut {
				class, pops := txscript.GetScriptInfo(txOut.PkScript)
				_, scriptHash, err := txscript.GetParsedOpcode(pops, class)
				if err != nil {
					return err
				}
				if _, ok := blkRelatedScriptHash[scriptHash]; !ok {
					blkRelatedScriptHash[scriptHash] = make(map[wire.TxLoc]bool)
				}
				blkRelatedScriptHash[scriptHash][txInfo.loc] = true

				spent := false
				if txInfo.isFullySpent {
					spent = true
				} else {
					byteidx := vout / 8
					byteoff := uint(vout % 8)
					spent = txInfo.spentBuf[byteidx]&(byte(1)<<byteoff) != 0
				}

				if spent {
					op := wire.OutPoint{Hash: txInfo.txhash, Index: uint32(vout)}
					spentOutpoints[op] = scriptHash
				}
			}
		}
		if err = checkTxIndexRelated(db, height, blkRelatedScriptHash); err != nil {
			return err
		}
		if height*100/bestHeight >= uint64(prog)+5 {
			prog = int(height * 100 / bestHeight)
			logging.CPrint(logging.INFO, fmt.Sprintf("check %d%%", prog), logging.LogFormat{})
		}
	}
	if len(spentOutpoints) != 0 {
		return errors.New("spentOutpoints not empty")
	}
	return nil
}

func checkTxIndexRelated(
	db *ChainDb,
	height uint64,
	blkRelatedScriptHash map[wire.Hash]map[wire.TxLoc]bool,
) error {
	for scriptHash, txlocs := range blkRelatedScriptHash {
		// check "HTS"
		htsKey, htsBoundary := encodeHTSKey(height, scriptHash)
		htsValue, err := db.stor.Get(htsKey)
		if err != nil || len(htsValue) != 4 ||
			htsBoundary != height/32*32 {
			logging.CPrint(logging.ERROR, "get hts error",
				logging.LogFormat{
					"err":       err,
					"height":    height,
					"value_len": len(htsValue),
				})
			return err
		}
		htsBitmap := binary.LittleEndian.Uint32(htsValue)
		if htsBitmap&(0x01<<(height-htsBoundary)) == 0 {
			logging.CPrint(logging.ERROR, "hts bitmap unset",
				logging.LogFormat{
					"height":      height,
					"bitmap":      htsValue,
					"htsBoundary": htsBoundary,
				})
			return ErrIncorrectDbData
		}

		// check "STL"
		stlKey, stlBoundary := encodeSTLKey(height, scriptHash)
		stlValue, err := db.stor.Get(stlKey)
		if err != nil || len(stlValue) < minTxIndexValueLen ||
			stlBoundary != height/16*16 {
			logging.CPrint(logging.ERROR, "get stl error",
				logging.LogFormat{
					"err":       err,
					"height":    height,
					"value_len": len(stlValue),
				})
			return err
		}
		stlBitmap := binary.LittleEndian.Uint32(stlValue[0:4])
		if stlBitmap&(0x01<<(height-stlBoundary)) == 0 {
			logging.CPrint(logging.ERROR, "stl bitmap unset",
				logging.LogFormat{
					"height":      height,
					"bitmap":      stlValue[0:4],
					"stlBoundary": stlBoundary,
				})
			return ErrIncorrectDbData
		}

		// locate
		cur := 4
		for i := 0; i < int(height-stlBoundary); i++ {
			if stlBitmap&(0x01<<i) != 0 {
				index := int(stlValue[cur])
				count := binary.LittleEndian.Uint16(stlValue[cur+1 : cur+3])
				if index != i || count == 0 {
					logging.CPrint(logging.ERROR, "stl check index failed",
						logging.LogFormat{
							"height": height,
							"actual": int(stlValue[cur]),
							"expect": i,
							"count":  count,
						})
					return ErrIncorrectDbData
				}
				cur += 3 + 8*int(count)
			}
		}
		// compare index and count
		index := int(stlValue[cur])
		count := binary.LittleEndian.Uint16(stlValue[cur+1 : cur+3])
		if index != int(height-stlBoundary) || int(count) != len(txlocs) {
			logging.CPrint(logging.ERROR, "stl check target index failed",
				logging.LogFormat{
					"height":     height,
					"index":      index,
					"count":      count,
					"txlocs_len": len(txlocs),
					"shiftN":     int(height - stlBoundary),
				})
			return ErrIncorrectDbData
		}
		cur += 3
		// compare TxLoc
		for i := 0; i < int(count); i++ {
			loc := wire.TxLoc{
				TxStart: int(binary.LittleEndian.Uint32(stlValue[cur : cur+4])),
				TxLen:   int(binary.LittleEndian.Uint32(stlValue[cur+4 : cur+8])),
			}
			if _, ok := txlocs[loc]; !ok {
				logging.CPrint(logging.ERROR, "stl txloc not found",
					logging.LogFormat{
						"height":  height,
						"txloc_i": i,
						"count":   count,
						"index":   index,
						"shiftN":  int(height - stlBoundary),
					})
				return ErrIncorrectDbData
			}
			cur += 8
		}
	}
	return nil
}
