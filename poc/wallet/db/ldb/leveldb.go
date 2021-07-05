package db

import (
	"strconv"
	"strings"

	"massnet.org/mass/poc/wallet/db"

	"github.com/massnetorg/mass-core/logging"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
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
)

// LevelDB ...
type LevelDB struct {
	LDb *leveldb.DB
	//TODO: thread safe
}

func joinBucketPath(arr ...string) string {
	return strings.Join(arr, bucketPathSep)
}

// Close
// It is not safe to close a DB until all outstanding iterators are released.
func (l *LevelDB) Close() error {
	return l.LDb.Close()
}

// BeginTx ...
func (l *LevelDB) BeginTx() (db.DBTransaction, error) {
	tr, err := l.LDb.OpenTransaction()
	if err != nil {
		return nil, err
	}

	return &LDBTransaction{
		tr: tr,
	}, nil
}

// LDBTransaction ...
type LDBTransaction struct {
	tr *leveldb.Transaction
}

// TopLevelBucket ...
func (tx *LDBTransaction) TopLevelBucket(name string) db.Bucket {
	bucketPath := joinBucketPath(topLevelBucketDepth, name)

	key := []byte(joinBucketPath(bucketNameBucket, bucketPath))
	_, err := tx.tr.Get(key, nil) // value == name
	if err != nil {
		return nil
	}
	//TOCONFIRM: check value == name

	bucket := &LDBBucket{
		tx:    tx,
		name:  name,
		path:  bucketPath,
		depth: 1,
	}
	bucket.pathLen = len(bucket.path)

	return bucket
}

// BucketNames ...
func (tx *LDBTransaction) BucketNames() (names []string, err error) {
	names = make([]string, 0)
	prefix := []byte(joinBucketPath(bucketNameBucket, topLevelBucketDepth, ""))
	iter := tx.tr.NewIterator(util.BytesPrefix(prefix), nil)

	defer func() {
		iter.Release()
		if err != nil {
			names = nil
			return
		}
		err = iter.Error()
		if err != nil {
			names = nil
		}
	}()

	for iter.Next() {
		key := string(iter.Key())
		value := string(iter.Value())
		ss := strings.Split(key, bucketPathSep)
		if len(ss) != 3 || ss[2] != value {
			err = db.ErrIllegalValue
			return
		}
		names = append(names, value)
	}
	return names, nil
}

// FetchBucket ...
func (tx *LDBTransaction) FetchBucket(meta db.BucketMeta) db.Bucket {
	if meta == nil {
		return nil
	}
	path := joinBucketPath(meta.Paths()...)
	key := []byte(joinBucketPath(bucketNameBucket, path))
	_, err := tx.tr.Get(key, nil) // value == name
	if err != nil {
		return nil
	}
	//TOCONFIRM: check value == name

	bucket := &LDBBucket{
		tx:    tx,
		name:  meta.Name(),
		path:  path,
		depth: meta.Depth(),
	}
	bucket.pathLen = len(bucket.path)

	return bucket
}

// CreateTopLevelBucket ...
func (tx *LDBTransaction) CreateTopLevelBucket(name string) (db.Bucket, error) {

	if !isValidBucketName(name) {
		return nil, db.ErrInvalidBucketName
	}

	bucketPath := joinBucketPath(topLevelBucketDepth, name)

	// check exist
	key := []byte(joinBucketPath(bucketNameBucket, bucketPath))
	_, err := tx.tr.Get(key, nil) // value == name
	// if string(value) == name {
	// 	return nil, ErrBucketExist
	// }
	if err != nil && err != leveldb.ErrNotFound {
		return nil, err
	}

	bucket := &LDBBucket{
		tx:    tx,
		name:  name,
		path:  bucketPath,
		depth: 1,
	}
	bucket.pathLen = len(bucket.path)

	// write to db
	if err = tx.tr.Put(key, []byte(name), nil); err != nil {
		return nil, err
	}
	logging.VPrint(logging.DEBUG, "new top level bucket",
		logging.LogFormat{
			"bucket": string(key),
		})

	return bucket, nil
}

// DeleteTopLevelBucket ...
func (tx *LDBTransaction) DeleteTopLevelBucket(name string) error {
	return db.ErrNotSupported
}

// Rollback ...
func (tx *LDBTransaction) Rollback() error {
	tx.tr.Discard()
	return nil
}

// Commit ...
func (tx *LDBTransaction) Commit() error {
	return tx.tr.Commit()
}

// LDBBucket ...
type LDBBucket struct {
	tx      *LDBTransaction
	name    string
	path    string
	pathLen int
	depth   int
}

// NewBucket create sub bucket
func (b *LDBBucket) NewBucket(name string) (db.Bucket, error) {
	sub, err := b.subBucket(name)
	if err != nil {
		return nil, err
	}

	key := []byte(joinBucketPath(bucketNameBucket, sub.path))
	_, err = b.tx.tr.Get(key, nil) // value == name
	if err == nil {
		return nil, db.ErrBucketExist
	}
	if err != leveldb.ErrNotFound {
		return nil, err
	}
	// if string(v) == name {
	// 	return nil, ErrBucketExist
	// }

	if err = b.tx.tr.Put(key, []byte(name), nil); err != nil {
		return nil, err
	}
	logging.VPrint(logging.DEBUG, "new sub bucket",
		logging.LogFormat{
			"bucket": string(key),
		})
	return sub, nil
}

// Bucket ...
func (b *LDBBucket) Bucket(name string) db.Bucket {
	sub, err := b.subBucket(name)
	if err != nil {
		return nil
	}

	key := []byte(joinBucketPath(bucketNameBucket, sub.path))
	value, err := b.tx.tr.Get(key, nil) // value == name
	if err != nil || string(value) != name {
		return nil
	}

	return sub
}

func (b *LDBBucket) subBucket(name string) (*LDBBucket, error) {
	if !isValidBucketName(name) {
		return nil, db.ErrInvalidBucketName
	}
	sub := &LDBBucket{
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

// BucketNames ...
func (b *LDBBucket) BucketNames() (names []string, err error) {
	ss := strings.Split(b.path, bucketPathSep)
	if len(ss) < 2 {
		return nil, db.ErrIllegalBucketPath
	}

	names = make([]string, 0)

	ss[0] = strconv.Itoa(b.depth + 1)
	ss = append(ss, "")
	prefix := []byte(joinBucketPath(bucketNameBucket, joinBucketPath(ss...)))
	iter := b.tx.tr.NewIterator(util.BytesPrefix(prefix), nil)

	defer func() {
		iter.Release()
		if err != nil {
			names = nil
			return
		}
		err = iter.Error()
		if err != nil {
			names = nil
		}
	}()

	for iter.Next() {
		key := string(iter.Key())
		value := string(iter.Value())
		ss := strings.Split(key, bucketPathSep)
		if len(ss) != b.depth+3 || ss[b.depth+2] != value {
			err = db.ErrIllegalValue
			return
		}
		names = append(names, value)
	}
	return
}

// DeleteBucket ...
func (b *LDBBucket) DeleteBucket(name string) error {
	sub := b.Bucket(name)
	if sub == nil {
		return nil
	}
	batch := new(leveldb.Batch)
	err := deleteBucket(sub.(*LDBBucket), batch)
	if err != nil {
		return err
	}
	return b.tx.tr.Write(batch, nil)
}

func deleteBucket(b *LDBBucket, batch *leveldb.Batch) error {
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
		err = deleteBucket(sub.(*LDBBucket), batch)
		if err != nil {
			return err
		}
	}

	// delete k/v in bucket
	prefix := []byte(joinBucketPath(b.path, ""))
	iter := b.tx.tr.NewIterator(util.BytesPrefix(prefix), nil)
	for iter.Next() {
		batch.Delete(iter.Key())
		logging.VPrint(logging.DEBUG, "deleting bucket item",
			logging.LogFormat{
				"key": string(iter.Key()),
			})
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return err
	}

	// delete bucket name
	key := joinBucketPath(bucketNameBucket, b.path)
	logging.VPrint(logging.DEBUG, "deleting bucket",
		logging.LogFormat{
			"bucket": key,
		})
	return b.tx.tr.Delete([]byte(key), nil)
}

// Close ...
func (b *LDBBucket) Close() error {
	// noting to do in sub bucket instance
	return nil
}

// Put ...
func (b *LDBBucket) Put(key, value []byte) error {
	if len(value) == 0 {
		return db.ErrIllegalValue
	}
	key, err := b.innerKey(key, false)
	if err != nil {
		return err
	}
	return b.tx.tr.Put(key, value, nil)
}

// Get ...
func (b *LDBBucket) Get(key []byte) ([]byte, error) {
	key, err := b.innerKey(key, false)
	if err != nil {
		return nil, nil
	}
	value, err := b.tx.tr.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return value, nil
}

// Delete ...
func (b *LDBBucket) Delete(key []byte) error {
	key, err := b.innerKey(key, false)
	if err != nil {
		return nil
	}
	return b.tx.tr.Delete(key, nil)
}

// Clear ...
func (b *LDBBucket) Clear() error {
	batch := new(leveldb.Batch)
	prefix := []byte(joinBucketPath(b.path, ""))
	iter := b.tx.tr.NewIterator(util.BytesPrefix(prefix), nil)
	for iter.Next() {
		batch.Delete(iter.Key())
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		return err
	}
	logging.VPrint(logging.DEBUG, "clear bucket item",
		logging.LogFormat{
			"key":    string(iter.Key()),
			"bucket": b.path,
			"num":    batch.Len(),
		})

	return b.tx.tr.Write(batch, nil)
}

// GetByPrefix ...
func (b *LDBBucket) GetByPrefix(prefix []byte) ([]*db.Entry, error) {
	innerPrefix, err := b.innerKey(prefix, true)
	if err != nil {
		return nil, nil
	}
	iter := b.tx.tr.NewIterator(util.BytesPrefix(innerPrefix), nil)
	entries := make([]*db.Entry, 0)
	for iter.Next() {
		entry := &db.Entry{
			Key:   make([]byte, len(iter.Key()[b.pathLen+1:])),
			Value: make([]byte, len(iter.Value()[:])),
		}
		copy(entry.Key, iter.Key()[b.pathLen+1:])
		copy(entry.Value, iter.Value()[:])
		entries = append(entries, entry)
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// only read
// BeginReadTx ...
func (l *LevelDB) BeginReadTx() (db.ReadTransaction, error) {
	return &LDBReadTransaction{
		ldb: l.LDb,
	}, nil
}

type LDBReadTransaction struct {
	ldb *leveldb.DB
}

func (tx *LDBReadTransaction) BucketNames() (names []string, err error) {
	names = make([]string, 0)
	prefix := []byte(joinBucketPath(bucketNameBucket, topLevelBucketDepth, ""))
	iter := tx.ldb.NewIterator(util.BytesPrefix(prefix), nil)

	defer func() {
		iter.Release()
		if err != nil {
			names = nil
			return
		}
		err = iter.Error()
		if err != nil {
			names = nil
		}
	}()

	for iter.Next() {
		key := string(iter.Key())
		value := string(iter.Value())
		ss := strings.Split(key, bucketPathSep)
		if len(ss) != 3 || ss[2] != value {
			err = db.ErrIllegalValue
			return
		}
		names = append(names, value)
	}
	return names, nil
}

func (tx *LDBReadTransaction) FetchBucket(meta db.BucketMeta) db.Bucket {
	if meta == nil {
		return nil
	}
	path := joinBucketPath(meta.Paths()...)
	key := []byte(joinBucketPath(bucketNameBucket, path))
	_, err := tx.ldb.Get(key, nil) // value == name
	if err != nil {
		return nil
	}
	//TOCONFIRM: check value == name

	bucket := &LDBReadBucket{
		ldb:   tx.ldb,
		name:  meta.Name(),
		path:  path,
		depth: meta.Depth(),
	}
	bucket.pathLen = len(bucket.path)
	return bucket
}

func (tx *LDBReadTransaction) TopLevelBucket(name string) db.Bucket {
	bucketPath := joinBucketPath(topLevelBucketDepth, name)

	key := []byte(joinBucketPath(bucketNameBucket, bucketPath))
	_, err := tx.ldb.Get(key, nil) // value == name
	if err != nil {
		return nil
	}
	//TOCONFIRM: check value == name

	bucket := &LDBReadBucket{
		ldb:   tx.ldb,
		name:  name,
		path:  bucketPath,
		depth: 1,
	}
	bucket.pathLen = len(bucket.path)

	return bucket
}

// Rollback ...
func (tx *LDBReadTransaction) Rollback() error {
	return nil
}

type LDBReadBucket struct {
	ldb     *leveldb.DB
	name    string
	path    string
	pathLen int
	depth   int
}

func (b *LDBReadBucket) Bucket(name string) db.Bucket {
	sub, err := b.subBucket(name)
	if err != nil {
		return nil
	}

	key := []byte(joinBucketPath(bucketNameBucket, sub.path))
	value, err := b.ldb.Get(key, nil) // value == name
	if err != nil || string(value) != name {
		return nil
	}

	return sub
}

func (b *LDBReadBucket) subBucket(name string) (*LDBReadBucket, error) {
	if !isValidBucketName(name) {
		return nil, db.ErrInvalidBucketName
	}
	sub := &LDBReadBucket{
		ldb:   b.ldb,
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

func (b *LDBReadBucket) Get(key []byte) ([]byte, error) {
	key, err := b.innerKey(key, false)
	if err != nil {
		return nil, nil
	}
	value, err := b.ldb.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return value, nil
}

func (b *LDBReadBucket) GetByPrefix(prefix []byte) ([]*db.Entry, error) {
	innerPrefix, err := b.innerKey(prefix, true)
	if err != nil {
		return nil, nil
	}
	iter := b.ldb.NewIterator(util.BytesPrefix(innerPrefix), nil)
	entries := make([]*db.Entry, 0)
	for iter.Next() {
		entry := &db.Entry{
			Key:   make([]byte, len(iter.Key()[b.pathLen+1:])),
			Value: make([]byte, len(iter.Value()[:])),
		}
		copy(entry.Key, iter.Key()[b.pathLen+1:])
		copy(entry.Value, iter.Value()[:])
		entries = append(entries, entry)
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (b *LDBReadBucket) innerKey(key []byte, asPrefix bool) ([]byte, error) {
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

func (b *LDBReadBucket) GetBucketMeta() db.BucketMeta {
	return &LDBBucketMeta{
		paths: strings.Split(b.path, bucketPathSep),
	}
}

func (b *LDBReadBucket) BucketNames() (names []string, err error) {
	ss := strings.Split(b.path, bucketPathSep)
	if len(ss) < 2 {
		return nil, db.ErrIllegalBucketPath
	}

	names = make([]string, 0)

	ss[0] = strconv.Itoa(b.depth + 1)
	ss = append(ss, "")
	prefix := []byte(joinBucketPath(bucketNameBucket, joinBucketPath(ss...)))
	iter := b.ldb.NewIterator(util.BytesPrefix(prefix), nil)

	defer func() {
		iter.Release()
		if err != nil {
			names = nil
			return
		}
		err = iter.Error()
		if err != nil {
			names = nil
		}
	}()

	for iter.Next() {
		key := string(iter.Key())
		value := string(iter.Value())
		ss := strings.Split(key, bucketPathSep)
		if len(ss) != b.depth+3 || ss[b.depth+2] != value {
			err = db.ErrIllegalValue
			return
		}
		names = append(names, value)
	}
	return
}

func (b *LDBReadBucket) NewBucket(name string) (db.Bucket, error) {
	return nil, db.ErrNotSupported
}

func (b *LDBReadBucket) Clear() error {
	return db.ErrNotSupported
}

func (b *LDBReadBucket) Delete(key []byte) error {
	return db.ErrNotSupported
}

func (b *LDBReadBucket) Put(key, value []byte) error {
	return db.ErrNotSupported
}

func (b *LDBReadBucket) DeleteBucket(name string) error {
	return db.ErrNotSupported
}

// GetBucketMeta ...
func (b *LDBBucket) GetBucketMeta() db.BucketMeta {
	return &LDBBucketMeta{
		paths: strings.Split(b.path, bucketPathSep),
	}
}

// LDBBucketMeta ...
type LDBBucketMeta struct {
	paths []string
}

// Paths ...
func (m *LDBBucketMeta) Paths() []string {
	return m.paths
}

// Name ...
func (m *LDBBucketMeta) Name() string {
	return m.paths[m.Depth()]
}

// Depth ...
func (m *LDBBucketMeta) Depth() int {
	depth, _ := strconv.Atoi(m.paths[0])
	return depth
}

func (b *LDBBucket) innerKey(key []byte, asPrefix bool) ([]byte, error) {
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

// is valid bucket name
func isValidBucketName(name string) bool {
	return len(name) > 0 && len(name) <= maxBucketNameLen && strings.Index(name, bucketPathSep) < 0
}

func init() {
	db.RegisterDriver(db.DBDriver{
		Type:     "leveldb",
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
	return newLevelDB(path, true)
}

func OpenDB(args ...interface{}) (db.DB, error) {
	path, err := parseDbPath(args...)
	if err != nil {
		return nil, err
	}
	return newLevelDB(path, false)
}

func newLevelDB(path string, create bool) (db.DB, error) {
	var ldb *leveldb.DB

	opts := &opt.Options{
		Filter:             filter.NewBloomFilter(10),
		WriteBuffer:        64 * opt.MiB,
		BlockSize:          16 * opt.KiB,
		BlockCacheCapacity: 512 * opt.MiB,
		BlockCacher:        opt.DefaultBlockCacher,
		OpenFilesCacher:    opt.DefaultOpenFilesCacher,
		Compression:        opt.DefaultCompression,
		ErrorIfMissing:     !create,
		ErrorIfExist:       create,
	}

	ldb, err := leveldb.OpenFile(path, opts)
	if err != nil {
		logging.CPrint(logging.ERROR, "newLevelDB failed",
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
	return &LevelDB{LDb: ldb}, nil
}
