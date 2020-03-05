package ldb

import (
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

// LBatch represents batch for submitting block/addrIndex
type LBatch struct {
	batch storage.Batch
	block wire.Hash
	done  bool
}

func (b *LBatch) Batch() storage.Batch {
	return b.batch
}

func (b *LBatch) Done() {
	b.done = true
}

func (b *LBatch) Set(block wire.Hash) {
	b.block = block
}

func (b *LBatch) Reset() {
	b.batch.Reset()
	b.block = wire.Hash{}
	b.done = false
}

func NewLBatch(batch storage.Batch) *LBatch {
	return &LBatch{
		batch: batch,
		block: wire.Hash{},
		done:  false,
	}
}

// // LTransaction represents for levelDB transaction with wrapped functions
// type LTransaction struct {
// 	tr *leveldb.Transaction
// }

// func (ltr *LTransaction) CommitClose(batches []*LBatch, wo *opt.WriteOptions) error {
// 	for i := range batches {
// 		if err := ltr.tr.Write(batches[i].Batch(), wo); err != nil {
// 			return err
// 		}
// 	}
// 	if err := ltr.tr.Commit(); err != nil {
// 		ltr.tr.Discard()
// 		return err
// 	}
// 	return nil
// }

// func OpenLTransaction(db *leveldb.DB) (*LTransaction, error) {
// 	tr, err := db.OpenTransaction()
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &LTransaction{
// 		tr: tr,
// 	}, nil
// }

// preSubmit performs several checks on LBatches, in case of unrelated block/addrIndex being submitted
func (db *ChainDb) preSubmit(index int) error {
	batch := db.batches[index]
	for i := 0; i < index-1; i++ {
		preBatch := db.Batch(i)
		if !preBatch.done {
			logging.CPrint(logging.ERROR, "fail on ChainDb preSubmit",
				logging.LogFormat{
					"err":   ErrPreBatchNotReady,
					"pre":   i,
					"batch": index,
				})
			return ErrPreBatchNotReady
		}
		if !(&preBatch.block).IsEqual(&batch.block) {
			logging.CPrint(logging.ERROR, "fail on ChainDb preSubmit",
				logging.LogFormat{
					"err":        ErrUnrelatedBatch,
					"pre":        i,
					"batch":      index,
					"preBlock":   preBatch.block,
					"batchBlock": batch.block,
				})
			return ErrUnrelatedBatch
		}
	}
	return nil
}

func (db *ChainDb) Commit(hash wire.Hash) error {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	defer func() {
		for i := range db.batches {
			db.Batch(i).Reset()
		}
	}()

	if err := db.preCommit(&hash); err != nil {
		return err
	}

	if err := db.stor.Write(db.dbBatch); err != nil {
		return err
	}

	storageMeta, err := db.stor.Get(dbStorageMetaDataKey)
	if err != nil {
		return err
	}

	db.dbStorageMeta, err = decodeDBStorageMetaData(storageMeta)
	if err != nil {
		return err
	}

	return nil
}

func (db *ChainDb) preCommit(hash *wire.Hash) error {
	for i := range db.batches {
		batch := db.Batch(i)
		if !hash.IsEqual(&batch.block) {
			return ErrCommitHashNotEqual
		}
		if !batch.done {
			return ErrCommitBatchNotReady
		}
	}
	return nil
}
