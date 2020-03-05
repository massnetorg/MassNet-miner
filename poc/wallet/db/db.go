package db

import "errors"

// Error definition
var (
	ErrFileExist         = errors.New("file exist")
	ErrFileNotExist      = errors.New("file not exist")
	ErrBucketExist       = errors.New("bucket already exist")
	ErrBucketNotFound    = errors.New("bucket not found")
	ErrInvalidBucketName = errors.New("invalid bucket name")
	ErrIllegalKey        = errors.New("illegal key")
	ErrIllegalValue      = errors.New("illegal value")
	ErrNotSupported      = errors.New("not supported")
	ErrIllegalBucketPath = errors.New("illegal bucket path")
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrWriteNotAllowed   = errors.New("write not allowed")
	ErrDbUnknownType     = errors.New("unknownt db type")
	ErrOpenDBFailed      = errors.New("open db failed")
	ErrCreateDBFailed    = errors.New("create db failed")
)

// DB ...
type DB interface {
	Close() error
	BeginTx() (DBTransaction, error)
	BeginReadTx() (ReadTransaction, error)
}

type ReadTransaction interface {
	TopLevelBucket(name string) Bucket
	FetchBucket(meta BucketMeta) Bucket
	BucketNames() ([]string, error)
	Rollback() error
}

// DBTransaction ...
type DBTransaction interface {
	Commit() error
	Rollback() error
	TopLevelBucket(name string) Bucket
	BucketNames() ([]string, error)
	FetchBucket(meta BucketMeta) Bucket
	CreateTopLevelBucket(name string) (Bucket, error)
	DeleteTopLevelBucket(name string) error
}

// Bucket ...
type Bucket interface {
	NewBucket(name string) (Bucket, error)
	Bucket(name string) Bucket
	BucketNames() ([]string, error)
	DeleteBucket(name string) error
	Put(key, value []byte) error
	Delete(key []byte) error
	Get(key []byte) ([]byte, error)
	Clear() error
	GetByPrefix([]byte) ([]*Entry, error)
	GetBucketMeta() BucketMeta
}

// BucketMeta ...
type BucketMeta interface {
	Paths() []string
	Name() string
	Depth() int
}

// Entry ...
type Entry struct {
	Key   []byte
	Value []byte
}

// View ...
func View(db DB, f func(tx ReadTransaction) error) error {
	tx, err := db.BeginReadTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return f(tx)
}

// Update ...
func Update(db DB, f func(tx DBTransaction) error) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	err = f(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

var drivers []DBDriver

type DBDriver struct {
	Type     string
	OpenDB   func(args ...interface{}) (DB, error)
	CreateDB func(args ...interface{}) (DB, error)
}

func RegisterDriver(ins DBDriver) {
	for _, driver := range drivers {
		if driver.Type == ins.Type {
			return
		}
	}
	drivers = append(drivers, ins)
}

func RegisteredDbTypes() []string {
	var types []string
	for _, drv := range drivers {
		types = append(types, drv.Type)
	}
	return types
}

func CreateDB(dbtype string, args ...interface{}) (DB, error) {
	for _, driver := range drivers {
		if driver.Type == dbtype {
			return driver.CreateDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

func OpenDB(dbtype string, args ...interface{}) (DB, error) {
	for _, driver := range drivers {
		if driver.Type == dbtype {
			return driver.OpenDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}
