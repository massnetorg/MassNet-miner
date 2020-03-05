package ldbstorage

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
)

type levelDB struct {
	db *leveldb.DB
}

type levelBatch struct {
	b *leveldb.Batch
}

type levelIterator struct {
	iter  iterator.Iterator
	slice *storage.Range
}

func init() {
	storage.RegisterDriver(storage.StorageDriver{
		DbType:        "leveldb",
		OpenStorage:   OpenDB,
		CreateStorage: CreateDB,
	})
}

func CreateDB(path string, args ...interface{}) (storage.Storage, error) {
	return newLevelDB(path, true)
}

func OpenDB(path string, args ...interface{}) (storage.Storage, error) {
	return newLevelDB(path, false)
}

func newLevelDB(path string, create bool) (store storage.Storage, err error) {
	var ldb *leveldb.DB

	opts := &opt.Options{
		Filter:             filter.NewBloomFilter(10),
		WriteBuffer:        16 * opt.MiB,
		BlockSize:          4 * opt.KiB,
		BlockCacheCapacity: 128 * opt.MiB,
		BlockCacher:        opt.DefaultBlockCacher,
		OpenFilesCacher:    opt.DefaultOpenFilesCacher,
		Compression:        opt.DefaultCompression,
		ErrorIfMissing:     !create,
		ErrorIfExist:       create,
	}

	ldb, err = leveldb.OpenFile(path, opts)
	if err != nil {
		logging.CPrint(logging.ERROR, "init leveldb error", logging.LogFormat{
			"path":   path,
			"create": create,
			"err":    err,
		})
		return nil, err
	}

	logging.CPrint(logging.INFO, "init leveldb", logging.LogFormat{
		"path":   path,
		"create": create,
	})
	return &levelDB{db: ldb}, nil
}

func (l *levelDB) Close() error {
	return l.db.Close()
}

func (l *levelDB) Get(key []byte) ([]byte, error) {
	value, err := l.db.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return value, nil
}

func (l *levelDB) Put(key, value []byte) error {
	if len(key) == 0 {
		return storage.ErrInvalidKey
	}
	return l.db.Put(key, value, nil)
}

func (l *levelDB) Has(key []byte) (bool, error) {
	_, err := l.Get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (l *levelDB) Delete(key []byte) error {
	return l.db.Delete(key, nil)
}

func (l *levelDB) NewBatch() storage.Batch {
	return &levelBatch{
		b: new(leveldb.Batch),
	}
}

func (l *levelDB) Write(batch storage.Batch) error {
	lb, ok := batch.(*levelBatch)
	if !ok {
		return storage.ErrInvalidBatch
	}
	return l.db.Write(lb.b, nil)
}

func (l *levelDB) NewIterator(slice *storage.Range) storage.Iterator {
	if slice == nil {
		slice = &storage.Range{}
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
		return storage.ErrInvalidKey
	}
	b.b.Put(key, value)
	return nil
}

func (b *levelBatch) Delete(key []byte) error {
	if len(key) == 0 {
		return storage.ErrInvalidKey
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
	k := it.iter.Key()
	data := make([]byte, len(k))
	copy(data, k)
	return data
}

func (it *levelIterator) Value() []byte {
	v := it.iter.Value()
	data := make([]byte, len(v))
	copy(data, v)
	return data
}

func (it *levelIterator) Release() {
	it.iter.Release()
}

func (it *levelIterator) Error() error {
	return it.iter.Error()
}
