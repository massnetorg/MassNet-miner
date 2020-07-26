package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// StorageV1
	//		- initial
	StorageV1 int32 = 1 + iota
	// StorageV2
	//		- record bitlength associated with poc pk
	StorageV2
	// StorageV3
	//		- since 1.1.0
	//		- save blocks to disk
	StorageV3

	CurrentStorageVersion int32 = StorageV3
)

const (
	KiB = 1024
	MiB = KiB * 1024
	GiB = MiB * 1024
)

var (
	ErrDbUnknownType       = errors.New("non-existent database type")
	ErrInvalidKey          = errors.New("invalid key")
	ErrInvalidValue        = errors.New("invalid value")
	ErrInvalidBatch        = errors.New("invalid batch")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("not found")
	ErrIncompatibleStorage = errors.New("incompatible storage")
)

// Range is a key range.
type Range struct {
	// Start of the key range, include in the range.
	Start []byte

	// Limit of the key range, not include in the range.
	Limit []byte
}

func (r *Range) IsPrefix() bool {
	pr := BytesPrefix(r.Start)
	expect := pr.Limit
	actual := r.Limit
	switch {
	case len(expect) < len(actual):
		expect = make([]byte, len(actual))
		copy(expect, pr.Limit)
	case len(expect) > len(actual):
		actual = make([]byte, len(expect))
		copy(actual, r.Limit)
	}
	return bytes.Equal(expect, actual)
}

func BytesPrefix(prefix []byte) *Range {
	var limit []byte
	for i := len(prefix) - 1; i >= 0; i-- {
		c := prefix[i]
		if c < 0xff {
			limit = make([]byte, i+1)
			copy(limit, prefix)
			limit[i] = c + 1
			break
		}
	}
	return &Range{Start: prefix, Limit: limit}
}

type Iterator interface {
	Release()
	Error() error
	Seek(key []byte) bool
	Next() bool
	Key() []byte
	Value() []byte
}

type Batch interface {
	Release()
	Put(key, value []byte) error
	Delete(key []byte) error
	Reset()
}

type Storage interface {
	Close() error
	// Get returns ErrNotFound if key not exist
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Has(key []byte) (bool, error)
	Delete(key []byte) error
	Write(batch Batch) error
	NewBatch() Batch
	NewIterator(slice *Range) Iterator
}

type StorageDriver struct {
	DbType        string
	CreateStorage func(storPath string, args ...interface{}) (s Storage, err error)
	OpenStorage   func(storPath string, args ...interface{}) (s Storage, err error)
}

var drivers []StorageDriver

func RegisterDriver(instance StorageDriver) {
	for _, drv := range drivers {
		if drv.DbType == instance.DbType {
			return
		}
	}
	drivers = append(drivers, instance)
}

// CreateStorage intializes and opens a database.
func CreateStorage(dbtype, dbpath string, args ...interface{}) (s Storage, err error) {
	for _, drv := range drivers {
		if drv.DbType == dbtype {
			s, err = drv.CreateStorage(dbpath, args...)
			if err != nil {
				return nil, err
			}
			return
		}
	}
	return nil, ErrDbUnknownType
}

// CreateStorage opens an existing database.
func OpenStorage(dbtype, dbpath string, args ...interface{}) (s Storage, err error) {
	for _, drv := range drivers {
		if drv.DbType == dbtype {
			return drv.OpenStorage(dbpath, args...)
		}
	}
	return nil, ErrDbUnknownType
}

func RegisteredDbTypes() []string {
	var types []string
	for _, drv := range drivers {
		types = append(types, drv.DbType)
	}
	return types
}

type storageVersion struct {
	Dbtype  string `json:"dbtype,omitempty"`
	Version int32  `json:"version,omitempty"`
}

func WriteVersion(path, dbtype string, version int32) error {
	fo, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write ver file %s error: %v", path, err)
	}
	defer fo.Close()

	b, err := json.Marshal(storageVersion{
		Dbtype:  dbtype,
		Version: version,
	})
	if err != nil {
		return err
	}
	return binary.Write(fo, binary.LittleEndian, b)
}

func ReadVersion(path string) (string, int32, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	fs, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	buf := make([]byte, fs.Size())
	err = binary.Read(file, binary.LittleEndian, buf)
	if err != nil {
		return "", 0, err
	}

	var ver storageVersion
	err = json.Unmarshal(buf, &ver)
	if err != nil {
		return "", 0, err
	}
	return ver.Dbtype, ver.Version, nil
}

func CheckCompatibility(dbtype, storPath string) error {
	verFile := filepath.Join(storPath, ".ver")
	fs, err := os.Stat(verFile)
	if err != nil {
		if os.IsNotExist(err) {
			file, err := os.Create(verFile)
			if err != nil {
				return err
			}
			defer file.Close()

			data, err := json.Marshal(storageVersion{
				Dbtype:  dbtype,
				Version: CurrentStorageVersion,
			})
			if err != nil {
				return fmt.Errorf("marshal failed: %v", err)
			}
			return binary.Write(file, binary.LittleEndian, data)
		}
		return err
	}
	if fs.IsDir() {
		return fmt.Errorf("directory %s already exists", verFile)
	}

	// open
	file, err := os.Open(verFile)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, fs.Size())
	err = binary.Read(file, binary.LittleEndian, buf)
	if err != nil {
		return fmt.Errorf("read version file error: %v", err)
	}

	var ver storageVersion
	err = json.Unmarshal(buf, &ver)
	if err != nil {
		return fmt.Errorf("unmarshal failed: %v", err)
	}

	if ver.Version == CurrentStorageVersion && ver.Dbtype == dbtype {
		return nil
	}
	return ErrIncompatibleStorage
}
