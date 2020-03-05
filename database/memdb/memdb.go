package memdb

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
	"massnet.org/mass/database"
	"massnet.org/mass/database/ldb"
	dbstorage "massnet.org/mass/database/storage"
)

type memLevelDB struct {
	db *leveldb.DB
}

type levelBatch struct {
	b *leveldb.Batch
}

type levelIterator struct {
	iter  iterator.Iterator
	slice *dbstorage.Range
}

func NewMemDb() (database.Db, error) {
	stor, err := newMemStorage()
	if err != nil {
		return nil, err
	}
	return ldb.NewChainDb(stor)
}

func newMemStorage() (store dbstorage.Storage, err error) {
	mdb, err := leveldb.Open(storage.NewMemStorage(), &opt.Options{})
	if err != nil {
		return nil, err
	}
	return &memLevelDB{db: mdb}, nil
}

func (l *memLevelDB) Close() error {
	return l.db.Close()
}

func (l *memLevelDB) Get(key []byte) ([]byte, error) {
	value, err := l.db.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, dbstorage.ErrNotFound
		}
		return nil, err
	}
	return value, nil
}

func (l *memLevelDB) Put(key, value []byte) error {
	if len(key) == 0 {
		return dbstorage.ErrInvalidKey
	}
	return l.db.Put(key, value, nil)
}

func (l *memLevelDB) Has(key []byte) (bool, error) {
	_, err := l.Get(key)
	if err != nil {
		if err == dbstorage.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (l *memLevelDB) Delete(key []byte) error {
	return l.db.Delete(key, nil)
}

func (l *memLevelDB) NewBatch() dbstorage.Batch {
	return &levelBatch{
		b: new(leveldb.Batch),
	}
}

func (l *memLevelDB) Write(batch dbstorage.Batch) error {
	lb, ok := batch.(*levelBatch)
	if !ok {
		return dbstorage.ErrInvalidBatch
	}
	return l.db.Write(lb.b, nil)
}

func (l *memLevelDB) NewIterator(slice *dbstorage.Range) dbstorage.Iterator {
	if slice == nil {
		slice = &dbstorage.Range{}
	} else {
		if len(slice.Start) == 0 {
			slice.Start = nil
		}
		if len(slice.Limit) == 0 {
			slice.Limit = nil
		}
	}
	return &levelIterator{
		slice: slice,
		iter: l.db.NewIterator(&util.Range{
			Start: slice.Start,
			Limit: slice.Limit,
		}, nil),
	}
}

// -------------levelBatch-------------

func (b *levelBatch) Put(key, value []byte) error {
	if len(key) == 0 {
		return dbstorage.ErrInvalidKey
	}
	b.b.Put(key, value)
	return nil
}

func (b *levelBatch) Delete(key []byte) error {
	if len(key) == 0 {
		return dbstorage.ErrInvalidKey
	}
	b.b.Delete(key)
	return nil
}

func (b *levelBatch) Reset() {
	b.b.Reset()
}

func (b *levelBatch) Release() {
	b.b = nil
}

// -----------------levelIterator-----------------

func (it *levelIterator) Seek(key []byte) bool {
	return it.iter.Seek(key)
}

func (it *levelIterator) Next() bool {
	return it.iter.Next()
}

func (it *levelIterator) Key() []byte {
	return it.iter.Key()
}

func (it *levelIterator) Value() []byte {
	return it.iter.Value()
}

func (it *levelIterator) Release() {
	it.iter.Release()
}

func (it *levelIterator) Error() error {
	return it.iter.Error()
}
