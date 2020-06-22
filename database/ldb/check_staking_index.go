package ldb

import (
	"bytes"
	"fmt"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

var (
	ErrCheckStakingDuplicated = errors.New("duplicated staking")
	ErrCheckStakingDeletion   = errors.New("failed to delete staking")

	errStaking        = errors.New("add wrong staking index")
	errExpiredStaking = errors.New("add wrong expired staking index")
)

func (db *ChainDb) CheckStakingTxIndex(rebuild bool) error {
	_, bestHeight, err := db.NewestSha()
	if err != nil {
		return err
	}
	expectUnexpired := 0
	expectExpired := 0

	if rebuild {
		// delete
		if err := db.removeStakingTxIndex(); err != nil {
			return err
		}
	}
	// prepare
	prog := 0
	for i := uint64(1); i <= bestHeight; i++ {
		s, e, err := db.rebuildStakingIndex(i, bestHeight, rebuild)
		if err != nil {
			return err
		}
		expectUnexpired += s
		expectExpired += e

		if rebuild {
			if i*100/bestHeight >= uint64(prog)+5 {
				prog = int(i * 100 / bestHeight)
				logging.CPrint(logging.INFO, fmt.Sprintf("rebuild staking %d%%", prog), logging.LogFormat{})
			}
		}
	}
	logging.CPrint(logging.INFO, "check staking start", logging.LogFormat{
		"expectUnexpired": expectUnexpired,
		"expectExpired":   expectExpired,
	})
	// check
	return db.checkStakingIndex(bestHeight, expectUnexpired, expectExpired)
}

func (db *ChainDb) removeStakingTxIndex() error {
	// delete
	batch := db.stor.NewBatch()
	defer batch.Release()

	iterStaking := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iterStaking.Release()
	sCount := 0
	for iterStaking.Next() {
		mustDecodeStakingTxKey(iterStaking.Key())
		batch.Delete(iterStaking.Key())
		sCount++
	}
	logging.CPrint(logging.INFO, "remove \"TXL\"", logging.LogFormat{"count": sCount})

	iterExpired := db.stor.NewIterator(storage.BytesPrefix(recordExpiredStakingTx))
	defer iterExpired.Release()
	eCount := 0
	for iterExpired.Next() {
		mustDecodeStakingTxKey(iterExpired.Key())
		batch.Delete(iterExpired.Key())
		eCount++
	}

	if err := db.stor.Write(batch); err != nil {
		return err
	}
	logging.CPrint(logging.INFO, "remove \"TXU\"", logging.LogFormat{"count": eCount})

	iterNewStaking := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iterNewStaking.Release()
	for iterNewStaking.Next() {
		logging.CPrint(logging.ERROR, "find staking index after deleting", logging.LogFormat{})
		return ErrCheckStakingDeletion
	}
	iterNewExpired := db.stor.NewIterator(storage.BytesPrefix(recordExpiredStakingTx))
	defer iterNewExpired.Release()
	for iterNewExpired.Next() {
		logging.CPrint(logging.ERROR, "find expired index after deleting", logging.LogFormat{})
		return ErrCheckStakingDeletion
	}
	return nil
}

func (db *ChainDb) rebuildStakingIndex(blockHeight, bestHeight uint64, rebuild bool) (int, int, error) {
	_, buf, err := db.getBlkByHeight(blockHeight)
	if err != nil {
		return 0, 0, err
	}
	block, err := massutil.NewBlockFromBytes(buf, wire.DB)
	if err != nil {
		return 0, 0, err
	}
	sCount := 0
	eCount := 0

	if !rebuild {
		for txIdx, tx := range block.MsgBlock().Transactions {
			if txIdx == 0 {
				// coinbase
				continue
			}
			for _, txOut := range tx.TxOut {
				class, pops := txscript.GetScriptInfo(txOut.PkScript)
				if class == txscript.StakingScriptHashTy {
					frozenPeriod, _, err := txscript.GetParsedOpcode(pops, class)
					if err != nil {
						return 0, 0, err
					}
					expiredHeight := blockHeight + frozenPeriod
					if expiredHeight > bestHeight {
						sCount++
					} else {
						eCount++
					}
				}
			}
		}
		return sCount, eCount, nil
	}

	batch := db.stor.NewBatch()
	for txIdx, tx := range block.MsgBlock().Transactions {
		if txIdx == 0 {
			// coinbase
			continue
		}
		for index, txOut := range tx.TxOut {
			class, pops := txscript.GetScriptInfo(txOut.PkScript)
			if class == txscript.StakingScriptHashTy {
				frozenPeriod, rsh, err := txscript.GetParsedOpcode(pops, class)
				if err != nil {
					return 0, 0, err
				}
				expiredHeight := blockHeight + frozenPeriod
				mapKey := stakingTxMapKey{
					blockHeight: blockHeight,
					txID:        tx.TxHash(),
					index:       uint32(index),
				}
				stx := &stakingTx{
					rsh:   rsh,
					value: uint64(txOut.Value),
				}
				var key [stakingTxKeyLength]byte
				if expiredHeight > bestHeight {
					copy(key[:], heightStakingTxToKey(expiredHeight, mapKey))
					sCount++
				} else {
					copy(key[:], heightExpiredStakingTxToKey(expiredHeight, mapKey))
					eCount++
				}
				if err := batch.Put(key[:], db.formatSTx(stx)); err != nil {
					return 0, 0, err
				}
			}
		}
	}
	if err := db.stor.Write(batch); err != nil {
		return 0, 0, err
	}
	return sCount, eCount, nil
}

func (db *ChainDb) checkStakingIndex(bestHeight uint64, expectUnexpired, expectExpired int) error {
	sCount := 0
	eCount := 0

	logging.CPrint(logging.INFO, "check \"TXL\" ...", logging.LogFormat{"count": sCount})
	// unexpired
	iterStaking := db.stor.NewIterator(storage.BytesPrefix(recordStakingTx))
	defer iterStaking.Release()
	for iterStaking.Next() {
		expiredHeight, mapKey := mustDecodeStakingTxKey(iterStaking.Key())
		scriptHash, value := mustDecodeStakingTxValue(iterStaking.Value())
		tx, _, blockHeight, _, err := db.fetchTxDataBySha(&mapKey.txID)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to fetch tx data",
				logging.LogFormat{
					"error": err,
					"tx_id": &mapKey.txID,
				})
			return err
		}
		if blockHeight != mapKey.blockHeight {
			logging.CPrint(logging.ERROR, "block height mismatch", logging.LogFormat{
				"tx_id":              mapKey.txID.String(),
				"index":              mapKey.index,
				"block_height":       blockHeight,
				"index_block_height": mapKey.blockHeight,
			})
			return errStaking
		}
		if value != uint64(tx.TxOut[mapKey.index].Value) {
			logging.CPrint(logging.ERROR, "value mismatch", logging.LogFormat{
				"tx_id":       mapKey.txID.String(),
				"index":       mapKey.index,
				"value":       tx.TxOut[mapKey.index].Value,
				"index_value": value,
			})
			return errStaking
		}
		class, pops := txscript.GetScriptInfo(tx.TxOut[mapKey.index].PkScript)
		if class != txscript.StakingScriptHashTy {
			logging.CPrint(logging.ERROR, "script type mismatch", logging.LogFormat{
				"tx_id": mapKey.txID.String(),
				"index": mapKey.index,
			})
			return errStaking
		}
		frozenPeriod, rsh, err := txscript.GetParsedOpcode(pops, class)
		if err != nil {
			return err
		}
		if frozenPeriod+blockHeight != expiredHeight {
			logging.CPrint(logging.ERROR, "expired height mismatch", logging.LogFormat{
				"tx_id":                mapKey.txID.String(),
				"index":                mapKey.index,
				"expired_height":       frozenPeriod + blockHeight,
				"index_expired_height": expiredHeight,
			})
			return errStaking
		}
		if expiredHeight <= bestHeight {
			logging.CPrint(logging.ERROR, "expired staking tx", logging.LogFormat{
				"tx_id":          mapKey.txID.String(),
				"index":          mapKey.index,
				"expired_height": expiredHeight,
				"best_height":    bestHeight,
			})
			return errStaking
		}
		if bytes.Compare(rsh[:], scriptHash[:]) != 0 {
			logging.CPrint(logging.ERROR, "scriptHash mismatch", logging.LogFormat{
				"tx_id":                mapKey.txID.String(),
				"index":                mapKey.index,
				"expired_height":       rsh,
				"index_expired_height": scriptHash,
			})
			return errStaking
		}
		sCount++
	}
	if sCount != expectUnexpired {
		logging.CPrint(logging.ERROR, "missing staking index", logging.LogFormat{
			"expect": expectUnexpired,
			"found":  sCount,
		})
		return errStaking
	}

	logging.CPrint(logging.INFO, "check \"TXL\" done", logging.LogFormat{"count": sCount})

	logging.CPrint(logging.INFO, "check \"TXU\" ...", logging.LogFormat{"count": sCount})
	// expired
	iterExpired := db.stor.NewIterator(storage.BytesPrefix(recordExpiredStakingTx))
	defer iterExpired.Release()
	for iterExpired.Next() {
		expiredHeight, mapKey := mustDecodeStakingTxKey(iterExpired.Key())
		scriptHash, value := mustDecodeStakingTxValue(iterExpired.Value())
		tx, _, blockHeight, _, err := db.fetchTxDataBySha(&mapKey.txID)
		if err != nil {
			if err != database.ErrTxShaMissing {
				logging.CPrint(logging.ERROR, "failed to fetch expired staking tx data in unspent",
					logging.LogFormat{
						"error": err,
						"tx_id": &mapKey.txID,
					})
				return err
			}
			spentTxs, err := db.getTxFullySpent(&mapKey.txID)
			if err != nil {
				logging.CPrint(logging.ERROR, "failed to fetch expired staking tx data in both fully spent and unspent",
					logging.LogFormat{
						"error": err,
						"tx_id": &mapKey.txID,
					})
				return err
			}
			find := false
			for _, spentTx := range spentTxs {
				if spentTx.blkHeight == mapKey.blockHeight {
					tx, _, err = db.fetchTxDataByLoc(spentTx.blkHeight, spentTx.txoff, spentTx.txlen)
					if err != nil {
						logging.CPrint(logging.WARN, "failed to fetch expired staking tx data in fully spent",
							logging.LogFormat{
								"error":        err,
								"tx_id":        &mapKey.txID,
								"block_height": spentTx.blkHeight,
								"tx_offset":    spentTx.txoff,
								"tx_len":       spentTx.txlen,
							})
						return err
					}
					find = true
					blockHeight = mapKey.blockHeight
					break
				}
			}
			if !find {
				logging.CPrint(logging.WARN, "failed to fetch expired staking tx data in fully spent",
					logging.LogFormat{
						"tx_id": &mapKey.txID,
					})
				return errExpiredStaking
			}
		}

		if blockHeight != mapKey.blockHeight {
			logging.CPrint(logging.ERROR, "block height mismatch", logging.LogFormat{
				"tx_id":              mapKey.txID.String(),
				"block_height":       blockHeight,
				"index_block_height": mapKey.blockHeight,
			})
			return errExpiredStaking
		}
		if value != uint64(tx.TxOut[mapKey.index].Value) {
			logging.CPrint(logging.ERROR, "value mismatch", logging.LogFormat{
				"tx_id":       mapKey.txID.String(),
				"value":       tx.TxOut[mapKey.index].Value,
				"index_value": value,
			})
			return errExpiredStaking
		}
		class, pops := txscript.GetScriptInfo(tx.TxOut[mapKey.index].PkScript)
		if class != txscript.StakingScriptHashTy {
			logging.CPrint(logging.ERROR, "script type mismatch", logging.LogFormat{
				"tx_id": mapKey.txID.String(),
				"index": mapKey.index,
			})
			return errExpiredStaking
		}
		frozenPeriod, rsh, err := txscript.GetParsedOpcode(pops, class)
		if err != nil {
			return err
		}
		if frozenPeriod+blockHeight != expiredHeight {
			logging.CPrint(logging.ERROR, "expired height mismatch", logging.LogFormat{
				"tx_id":                mapKey.txID.String(),
				"expired_height":       frozenPeriod + blockHeight,
				"index_expired_height": expiredHeight,
			})
			return errExpiredStaking
		}
		if expiredHeight > bestHeight {
			logging.CPrint(logging.ERROR, "staking tx", logging.LogFormat{
				"tx_id":          mapKey.txID.String(),
				"expired_height": expiredHeight,
				"best_height":    bestHeight,
			})
			return errExpiredStaking
		}
		if bytes.Compare(rsh[:], scriptHash[:]) != 0 {
			logging.CPrint(logging.ERROR, "scriptHash mismatch", logging.LogFormat{
				"tx_id":                mapKey.txID.String(),
				"expired_height":       rsh,
				"index_expired_height": scriptHash,
			})
			return errExpiredStaking
		}
		eCount++
	}
	if eCount != expectExpired {
		logging.CPrint(logging.ERROR, "missing expired staking index", logging.LogFormat{
			"expect": expectExpired,
			"found":  eCount,
		})
		return errExpiredStaking
	}
	logging.CPrint(logging.INFO, "check \"TXU\" done", logging.LogFormat{"count": eCount})
	return nil
}
