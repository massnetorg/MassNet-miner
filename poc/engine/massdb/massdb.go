package massdb

import (
	"encoding/hex"
	"errors"

	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/pocec"
)

type MassDB interface {
	// Get type of MassDB
	Type() string

	// Close MassDB
	Close() error

	// Execute plotting on MassDB
	Plot() chan error

	// Stop plotting on MassDB
	StopPlot() chan error

	// Is MassDB loaded and plotted
	Ready() bool

	// Get bitLength of MassDB
	BitLength() int

	// Get PubKeyHash of MassDB
	PubKeyHash() pocutil.Hash

	// Get PubKey of MassDB
	PubKey() *pocec.PublicKey

	// Get assembled proof by challenge
	GetProof(challenge pocutil.Hash, filter bool) (proof *poc.DefaultProof, err error)

	// Get plot progress
	Progress() (prePlotted, plotted bool, progress float64)

	// Delete all data of MassDB
	Delete() chan error
}

var (
	ErrInvalidDBType        = errors.New("invalid massdb type")
	ErrInvalidDBArgs        = errors.New("invalid massdb args")
	ErrDBDoesNotExist       = errors.New("non-existent massdb")
	ErrDBAlreadyExists      = errors.New("massdb already exists")
	ErrDBCorrupted          = errors.New("massdb corrupted")
	ErrUnimplemented        = errors.New("unimplemented massdb interface")
	ErrUnsupportedBitLength = errors.New("unsupported bit length")
)

// DoubleSHA256([]byte("MASSDB"))
const DBFileCodeStr = "52A7AD74C4929DEC7B5C8D46CC3BAFA81FC96129283B3A6923CD12F41A30B3AC"

var (
	DBFileCode    []byte
	DBBackendList []DBBackend
)

type DBBackend struct {
	Typ      string
	OpenDB   func(args ...interface{}) (MassDB, error)
	CreateDB func(args ...interface{}) (MassDB, error)
}

func AddDBBackend(ins DBBackend) {
	for _, dbb := range DBBackendList {
		if dbb.Typ == ins.Typ {
			return
		}
	}
	DBBackendList = append(DBBackendList, ins)
}

func OpenDB(dbType string, args ...interface{}) (MassDB, error) {
	for _, dbb := range DBBackendList {
		if dbb.Typ == dbType {
			return dbb.OpenDB(args...)
		}
	}
	return nil, ErrInvalidDBType
}

func CreateDB(dbType string, args ...interface{}) (MassDB, error) {
	for _, dbb := range DBBackendList {
		if dbb.Typ == dbType {
			return dbb.CreateDB(args...)
		}
	}
	return nil, ErrInvalidDBType
}

func init() {
	var err error
	DBFileCode, err = hex.DecodeString(DBFileCodeStr)
	if err != nil || len(DBFileCode) != 32 {
		panic(err) // should never happen
	}
}
