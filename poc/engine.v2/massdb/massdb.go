package massdb

import (
	"encoding/hex"
	"errors"

	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
)

type MassDB interface {
	// Get type of MassDB
	Type() string

	// Close MassDB
	Close() error

	// Is MassDB loaded and plotted
	Ready() bool

	// Get bitLength of MassDB
	BitLength() int

	// Get ID of MassDB
	ID() [32]byte

	// Get PubKey of MassDB
	PlotInfo() *chiapos.PlotInfo

	// Get qualities of MassDB
	GetQualities(challenge pocutil.Hash) (qualities [][]byte, err error)

	// Get assembled proof by challenge
	GetProof(challenge pocutil.Hash, index uint32) (*chiapos.ProofOfSpace, error)
}

var (
	ErrInvalidDBType        = errors.New("invalid massdb type")
	ErrInvalidDBArgs        = errors.New("invalid massdb args")
	ErrDBDoesNotExist       = errors.New("non-existent massdb")
	ErrDBAlreadyExists      = errors.New("massdb already exists")
	ErrDBCorrupted          = errors.New("massdb corrupted")
	ErrUnimplemented        = errors.New("unimplemented massdb interface")
	ErrUnsupportedBitLength = errors.New("unsupported bit length")
	ErrNotPassingPlotFilter = errors.New("not passing plot filter")
)

// DoubleSHA256([]byte("MASSDB"))
const DBFileCodeStr = "52A7AD74C4929DEC7B5C8D46CC3BAFA81FC96129283B3A6923CD12F41A30B3AC"

var (
	DBFileCode    []byte
	DBBackendList []DBBackend
)

type DBBackend struct {
	Typ    string
	OpenDB func(args ...interface{}) (MassDB, error)
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

func init() {
	var err error
	DBFileCode, err = hex.DecodeString(DBFileCodeStr)
	if err != nil || len(DBFileCode) != 32 {
		panic(err) // should never happen
	}
}
