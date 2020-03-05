package pocminer

import (
	"errors"

	"massnet.org/mass/massutil"
)

type PoCMiner interface {
	Start() error
	Stop() error
	Started() bool
	Type() string
	SetPayoutAddresses(addresses []massutil.Address) error
}

var (
	ErrInvalidMinerType = errors.New("invalid Miner type")
	ErrInvalidMinerArgs = errors.New("invalid Miner args")
	ErrUnimplemented    = errors.New("unimplemented Miner interface")
)

var (
	BackendList []Backend
)

type Backend struct {
	Typ         string
	NewPoCMiner func(args ...interface{}) (PoCMiner, error)
}

func AddPoCMinerBackend(ins Backend) {
	for _, kb := range BackendList {
		if kb.Typ == ins.Typ {
			return
		}
	}
	BackendList = append(BackendList, ins)
}

func NewPoCMiner(kbType string, args ...interface{}) (PoCMiner, error) {
	for _, kb := range BackendList {
		if kb.Typ == kbType {
			return kb.NewPoCMiner(args...)
		}
	}
	return nil, ErrInvalidMinerType
}
