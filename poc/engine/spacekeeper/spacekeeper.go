package spacekeeper

import (
	"context"
	"errors"

	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/pocec"
	"massnet.org/mass/poc/engine"
)

type SpaceKeeper interface {
	Start() error
	Stop() error
	Started() bool
	Type() string
	WorkSpaceIDs(flags engine.WorkSpaceStateFlags) ([]string, error)
	WorkSpaceInfos(flags engine.WorkSpaceStateFlags) ([]engine.WorkSpaceInfo, error)
	GetProof(ctx context.Context, sid string, challenge pocutil.Hash, filter bool) (*engine.WorkSpaceProof, error)
	GetProofs(ctx context.Context, flags engine.WorkSpaceStateFlags, challenge pocutil.Hash, filter bool) ([]*engine.WorkSpaceProof, error)
	GetProofReader(ctx context.Context, sid string, challenge pocutil.Hash, filter bool) (engine.ProofReader, error)
	GetProofsReader(ctx context.Context, flags engine.WorkSpaceStateFlags, challenge pocutil.Hash, filter bool) (engine.ProofReader, error)
	ActOnWorkSpace(sid string, action engine.ActionType) error
	ActOnWorkSpaces(flags engine.WorkSpaceStateFlags, action engine.ActionType) (map[string]error, error)
	SignHash(sid string, hash [32]byte) (*pocec.Signature, error)
}

var (
	ErrInvalidSKType = errors.New("invalid SpaceKeeper type")
	ErrInvalidSKArgs = errors.New("invalid SpaceKeeper args")
	ErrUnimplemented = errors.New("unimplemented SpaceKeeper interface")
)

var (
	KeeperBackendList []SKBackend
)

type SKBackend struct {
	Typ            string
	NewSpaceKeeper func(args ...interface{}) (SpaceKeeper, error)
}

func AddSpaceKeeperBackend(ins SKBackend) {
	for _, kb := range KeeperBackendList {
		if kb.Typ == ins.Typ {
			return
		}
	}
	KeeperBackendList = append(KeeperBackendList, ins)
}

func NewSpaceKeeper(kbType string, args ...interface{}) (SpaceKeeper, error) {
	for _, kb := range KeeperBackendList {
		if kb.Typ == kbType {
			return kb.NewSpaceKeeper(args...)
		}
	}
	return nil, ErrInvalidSKType
}
