// +build darwin linux
// +build rocksdb

package rdb

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/massnetorg/mass-core/logging"
	"massnet.org/mass/poc/wallet/db"

	"github.com/tecbot/gorocksdb"
)

const (

	// e.g.
	// 1. top level bucket (created in LevelDB) index entry is like (the 2nd number means bucket depth, just '1'):
	//          <b_1_top1, top1>
	//          <b_1_top2, top2>
	//
	//	  the k/v entry in top level bucket is like (the 1st number means bucket depth):
	//					<1_top1_key1, value1>
	//					<1_top1_key2, value2>
	//
	//
	// 2. sub bucket (created in Bucket) index entry is like (the 2nd number means bucket depth, from '2' on):
	//          <b_2_top1_sub1, sub1>
	//          <b_2_top1_sub2, sub2>
	//
	//	  the k/v entry in sub bucket is like:
	//					<2_top1_sub1_key1, value1>
	//					<2_top1_sub2_key1, value1>
	//
	bucketNameBucket    = "b"
	bucketPathSep       = "_"
	topLevelBucketDepth = "1"

	maxBucketNameLen = 256

	KiB = 1024
	MiB = KiB * 1024
	GiB = MiB * 1024
)

// RocksDB ...
type RocksDB struct {
	tdb *gorocksdb.TransactionDB

	ro *gorocksdb.ReadOptions
	wo *gorocksdb.WriteOptions
	to *gorocksdb.TransactionOptions
}

type transaction struct {
	readOnly bool
	rdb      *RocksDB
	tr       *gorocksdb.Transaction
}

type rocksBucket struct {
	tx      *transaction
	name    string
	path    string
	pathLen int
	depth   int
}

type rocksBucketMeta struct {
	paths []string
}

// Paths ...
func (m *rocksBucketMeta) Paths() []string {
	return m.paths
}

// Name ...
func (m *rocksBucketMeta) Name() string {
	return m.paths[m.Depth()]
}

// Depth ...
func (m *rocksBucketMeta) Depth() int {
	depth, _ := strconv.Atoi(m.paths[0])
	return depth
}

func joinBucketPath(arr ...string) string {
	return strings.Join(arr, bucketPathSep)
}

func isValidBucketName(name string) bool {
	return len(name) > 0 && len(name) <= maxBucketNameLen && strings.Index(name, bucketPathSep) < 0
}

// read & free key/value
func readKeyValue(it *gorocksdb.Iterator) (k, v []byte) {
	key := it.Key()
	value := it.Value()

	k0 := key.Data()
	v0 := value.Data()
	k1 := make([]byte, len(k0))
	v1 := make([]byte, len(v0))
	copy(k1, k0)
	copy(v1, v0)

	key.Free()
	value.Free()

	return k1, v1
}

func init() {
	db.RegisterDriver(db.DBDriver{
		Type:     "rocksdb",
		OpenDB:   OpenDB,
		CreateDB: CreateDB,
	})
}

func parseDbPath(args ...interface{}) (string, error) {
	if len(args) != 1 {
		return "", db.ErrInvalidArgument
	}
	path, ok := args[0].(string)
	if !ok {
		return "", db.ErrInvalidArgument
	}
	return path, nil
}

func CreateDB(args ...interface{}) (db.DB, error) {
	path, err := parseDbPath(args...)
	if err != nil {
		return nil, err
	}
	return newRocksDB(path, true)
}

func OpenDB(args ...interface{}) (db.DB, error) {
	path, err := parseDbPath(args...)
	if err != nil {
		return nil, err
	}
	return newRocksDB(path, false)
}

func newRocksDB(path string, create bool) (db.DB, error) {

	filter := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetFilterPolicy(filter)
	cache := gorocksdb.NewLRUCache(64 << 20)
	bbto.SetBlockCache(cache)
	bbto.SetBlockSize(4 * KiB)

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	if create {
		opts.SetCreateIfMissing(true)
		opts.SetErrorIfExists(true)
	}
	opts.SetMaxOpenFiles(512)
	opts.SetWriteBufferSize(16 * MiB)
	opts.IncreaseParallelism(runtime.NumCPU())

	txOpts := gorocksdb.NewDefaultTransactionDBOptions()
	txOpts.SetTransactionLockTimeout(10)

	tdb, err := gorocksdb.OpenTransactionDb(opts, txOpts, path)
	if err != nil {
		logging.CPrint(logging.ERROR, "newRocksDB failed",
			logging.LogFormat{
				"err":    err,
				"create": create,
				"path":   path,
			})
		if create {
			return nil, db.ErrCreateDBFailed
		}
		return nil, db.ErrOpenDBFailed
	}

	rdb := &RocksDB{
		tdb: tdb,
		ro:  gorocksdb.NewDefaultReadOptions(),
		wo:  gorocksdb.NewDefaultWriteOptions(),
		to:  gorocksdb.NewDefaultTransactionOptions(),
	}

	//go RecordMetrics(storage)
	return rdb, nil
}

// Close ...
func (rdb *RocksDB) Close() error {
	rdb.tdb.Close()
	return nil
}

// BeginTx ...
func (rdb *RocksDB) BeginTx() (db.DBTransaction, error) {
	return &transaction{
		readOnly: false,
		rdb:      rdb,
		tr:       rdb.tdb.TransactionBegin(rdb.wo, rdb.to, nil),
	}, nil
}

// BeginReadTx ...
func (rdb *RocksDB) BeginReadTx() (db.ReadTransaction, error) {
	return &transaction{
		readOnly: true,
		rdb:      rdb,
		tr:       rdb.tdb.TransactionBegin(rdb.wo, rdb.to, nil),
	}, nil
}

// TopLevelBucket ...
func (tx *transaction) TopLevelBucket(name string) db.Bucket {
	bucketPath := joinBucketPath(topLevelBucketDepth, name)

	key := []byte(joinBucketPath(bucketNameBucket, bucketPath))
	v, err := tx.tr.Get(tx.rdb.ro, key)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to get top bucket", logging.LogFormat{"err": err, "bucket": name})
		return nil
	}
	defer v.Free()
	if !v.Exists() {
		return nil
	}

	bucket := &rocksBucket{
		tx:    tx,
		name:  name,
		path:  bucketPath,
		depth: 1,
	}
	bucket.pathLen = len(bucket.path)
	return bucket
}

// BucketNames ...
func (tx *transaction) BucketNames() (names []string, err error) {
	names = make([]string, 0)
	prefix := []byte(joinBucketPath(bucketNameBucket, topLevelBucketDepth, ""))
	iter := tx.tr.NewIterator(tx.rdb.ro)
	defer iter.Close()
	iter.Seek(prefix)
	for ; iter.ValidForPrefix(prefix); iter.Next() {
		key, value := readKeyValue(iter)
		ss := strings.Split(string(key), bucketPathSep)
		if len(ss) != 3 || ss[2] != string(value) {
			return nil, db.ErrIllegalValue
		}
		names = append(names, string(value))
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

// FetchBucket ...
func (tx *transaction) FetchBucket(meta db.BucketMeta) db.Bucket {
	path := joinBucketPath(meta.Paths()...)
	key := []byte(joinBucketPath(bucketNameBucket, path))
	v, err := tx.tr.Get(tx.rdb.ro, key)
	if err != nil {
		logging.CPrint(logging.WARN, "failed to fetch bucket", logging.LogFormat{"err": err, "bucket": meta.Paths()})
		return nil
	}
	defer v.Free()
	if !v.Exists() {
		return nil
	}

	bucket := &rocksBucket{
		tx:    tx,
		name:  meta.Name(),
		path:  path,
		depth: meta.Depth(),
	}
	bucket.pathLen = len(bucket.path)

	return bucket
}

// CreateTopLevelBucket ...
func (tx *transaction) CreateTopLevelBucket(name string) (db.Bucket, error) {
	if tx.readOnly {
		return nil, db.ErrWriteNotAllowed
	}

	if !isValidBucketName(name) {
		return nil, db.ErrInvalidBucketName
	}

	bucketPath := joinBucketPath(topLevelBucketDepth, name)

	// check exist
	key := []byte(joinBucketPath(bucketNameBucket, bucketPath))
	v, err := tx.tr.Get(tx.rdb.ro, key)
	if err != nil {
		return nil, err
	}
	defer v.Free()
	if v.Exists() {
		return nil, db.ErrBucketExist
	}

	bucket := &rocksBucket{
		tx:    tx,
		name:  name,
		path:  bucketPath,
		depth: 1,
	}
	bucket.pathLen = len(bucket.path)

	if err = tx.tr.Put(key, []byte(name)); err != nil {
		return nil, err
	}
	return bucket, nil
}

// DeleteTopLevelBucket ...
func (tx *transaction) DeleteTopLevelBucket(name string) error {
	return db.ErrNotSupported
}

// Rollback ...
func (tx *transaction) Rollback() error {
	defer tx.tr.Destroy()
	if tx.readOnly {
		return nil
	}
	return tx.tr.Rollback()
}

// Commit ...
func (tx *transaction) Commit() error {
	defer tx.tr.Destroy()
	if tx.readOnly {
		return nil
	}
	return tx.tr.Commit()
}

// NewBucket create sub bucket
func (b *rocksBucket) NewBucket(name string) (db.Bucket, error) {
	if b.tx.readOnly {
		return nil, db.ErrWriteNotAllowed
	}

	sub, err := b.subBucket(name)
	if err != nil {
		return nil, err
	}

	key := []byte(joinBucketPath(bucketNameBucket, sub.path))
	v, err := b.tx.tr.Get(b.tx.rdb.ro, key)
	if err != nil {
		return nil, err
	}
	defer v.Free()
	if v.Exists() {
		return nil, db.ErrBucketExist
	}

	// if string(v) == name {
	// 	return nil, ErrBucketExist
	// }

	if err = b.tx.tr.Put(key, []byte(name)); err != nil {
		return nil, err
	}
	logging.CPrint(logging.DEBUG, "new sub bucket",
		logging.LogFormat{
			"bucket": string(key),
		})
	return sub, nil
}

func (b *rocksBucket) subBucket(name string) (*rocksBucket, error) {
	if !isValidBucketName(name) {
		return nil, db.ErrInvalidBucketName
	}
	sub := &rocksBucket{
		tx:    b.tx,
		name:  name,
		depth: b.depth + 1,
	}
	ss := strings.Split(b.path, bucketPathSep)
	if len(ss) < 2 {
		return nil, db.ErrIllegalBucketPath
	}
	// e.g.  1_top  -->  2_top_child
	ss[0] = strconv.Itoa(sub.depth)
	ss = append(ss, sub.name)
	sub.path = joinBucketPath(ss...)
	sub.pathLen = len(sub.path)
	return sub, nil
}

// Bucket ...
func (b *rocksBucket) Bucket(name string) db.Bucket {
	sub, err := b.subBucket(name)
	if err != nil {
		logging.CPrint(logging.ERROR, "subBucket error",
			logging.LogFormat{
				"parent": b.path,
				"sub":    name,
				"err":    err,
			})
		return nil
	}

	key := []byte(joinBucketPath(bucketNameBucket, sub.path))
	v, err := b.tx.tr.Get(b.tx.rdb.ro, key)
	if err != nil {
		logging.CPrint(logging.ERROR, "get bucket error",
			logging.LogFormat{
				"bucket": string(key),
				"err":    err,
			})
		return nil
	}
	defer v.Free()
	if !v.Exists() {
		return nil
	}

	if string(v.Data()) != name {
		logging.CPrint(logging.ERROR, "bucket name conflicts",
			logging.LogFormat{
				"bucket": string(key),
				"expect": name,
				"actual": string(v.Data()),
			})
		return nil
	}
	return sub
}

// BucketNames ...
func (b *rocksBucket) BucketNames() (names []string, err error) {
	ss := strings.Split(b.path, bucketPathSep)
	if len(ss) < 2 {
		return nil, db.ErrIllegalBucketPath
	}

	names = make([]string, 0)

	ss[0] = strconv.Itoa(b.depth + 1)
	ss = append(ss, "")
	prefix := []byte(joinBucketPath(bucketNameBucket, joinBucketPath(ss...)))
	iter := b.tx.tr.NewIterator(b.tx.rdb.ro)
	defer iter.Close()
	iter.Seek(prefix)
	for ; iter.ValidForPrefix(prefix); iter.Next() {
		key, value := readKeyValue(iter)
		ss := strings.Split(string(key), bucketPathSep)
		if len(ss) != b.depth+3 || ss[b.depth+2] != string(value) {
			return nil, db.ErrIllegalValue
		}
		names = append(names, string(value))
	}
	if err = iter.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

// DeleteBucket ...
func (b *rocksBucket) DeleteBucket(name string) error {
	if b.tx.readOnly {
		return db.ErrWriteNotAllowed
	}

	sub := b.Bucket(name)
	if sub == nil {
		return nil
	}
	err := deleteBucket(sub.(*rocksBucket))
	if err != nil {
		logging.CPrint(logging.ERROR, "delete bucket error",
			logging.LogFormat{
				"bucket": sub.(*rocksBucket).path,
				"err":    err,
			})
	}
	return err
}

func deleteBucket(b *rocksBucket) error {
	if b.depth == 1 {
		return db.ErrNotSupported
	}

	// delete sub bucket
	subnames, err := b.BucketNames()
	if err != nil {
		return err
	}
	for _, subname := range subnames {
		sub := b.Bucket(subname)
		if sub == nil {
			continue
		}
		err = deleteBucket(sub.(*rocksBucket))
		if err != nil {
			return err
		}
	}

	// delete k/v in bucket
	err = b.Clear()
	if err != nil {
		return err
	}

	// delete bucket
	path := joinBucketPath(bucketNameBucket, b.path)
	return b.tx.tr.Delete([]byte(path))
}

func (b *rocksBucket) innerKey(key []byte, asPrefix bool) ([]byte, error) {
	kl := len(key)
	if !asPrefix && kl == 0 {
		return nil, db.ErrIllegalKey
	}
	buf := make([]byte, b.pathLen+kl+1)
	copy(buf[:], []byte(b.path))
	copy(buf[b.pathLen:b.pathLen+1], []byte(bucketPathSep))
	if kl > 0 {
		copy(buf[b.pathLen+1:], key)
	}
	return buf, nil
}

// Put ...
func (b *rocksBucket) Put(key, value []byte) error {
	if b.tx.readOnly {
		return db.ErrWriteNotAllowed
	}

	if len(value) == 0 {
		return db.ErrIllegalValue
	}
	key, err := b.innerKey(key, false)
	if err != nil {
		return err
	}
	return b.tx.tr.Put(key, value)
}

// Get ...
func (b *rocksBucket) Get(key []byte) ([]byte, error) {
	key, err := b.innerKey(key, false)
	if err != nil {
		// no need return error
		return nil, nil
	}
	v, err := b.tx.tr.Get(b.tx.rdb.ro, key)
	if err != nil {
		return nil, err
	}
	defer v.Free()
	if !v.Exists() {
		return nil, nil
	}
	return append([]byte{}, v.Data()...), nil
}

// Delete ...
func (b *rocksBucket) Delete(key []byte) error {
	if b.tx.readOnly {
		return db.ErrWriteNotAllowed
	}

	key, err := b.innerKey(key, false)
	if err != nil {
		// no need return error
		return nil
	}
	return b.tx.tr.Delete(key)
}

// Clear ...
func (b *rocksBucket) Clear() error {
	if b.tx.readOnly {
		return db.ErrWriteNotAllowed
	}
	prefix := []byte(joinBucketPath(b.path, ""))
	iter := b.tx.tr.NewIterator(b.tx.rdb.ro)
	defer iter.Close()
	iter.Seek(prefix)
	l := make([][]byte, 0)
	for ; iter.ValidForPrefix(prefix); iter.Next() {
		key, _ := readKeyValue(iter)
		l = append(l, key)
	}
	if err := iter.Err(); err != nil {
		return err
	}
	for _, key := range l {
		if err := b.tx.tr.Delete(key); err != nil {
			logging.CPrint(logging.ERROR, "clear bucket error",
				logging.LogFormat{
					"key":    string(key),
					"bucket": b.path,
					"err":    err,
				})
			return err
		}
	}
	logging.CPrint(logging.DEBUG, "clear bucket",
		logging.LogFormat{
			"bucket": b.path,
			"num":    len(l),
		})

	return nil
}

// GetByPrefix ...
func (b *rocksBucket) GetByPrefix(prefix []byte) ([]*db.Entry, error) {
	innerPrefix, err := b.innerKey(prefix, true)
	if err != nil {
		// no need return error
		return []*db.Entry{}, nil
	}
	iter := b.tx.tr.NewIterator(b.tx.rdb.ro)
	defer iter.Close()
	iter.Seek(innerPrefix)

	entries := make([]*db.Entry, 0)
	for ; iter.ValidForPrefix(innerPrefix); iter.Next() {
		key, value := readKeyValue(iter)
		entry := &db.Entry{
			Key:   key[b.pathLen+1:],
			Value: value,
		}
		entries = append(entries, entry)
	}
	if err := iter.Err(); err != nil {
		return []*db.Entry{}, err
	}
	return entries, nil
}

// GetBucketMeta ...
func (b *rocksBucket) GetBucketMeta() db.BucketMeta {
	return &rocksBucketMeta{
		paths: strings.Split(b.path, bucketPathSep),
	}
}
