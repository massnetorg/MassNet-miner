package mining

import (
	"context"
	"sync/atomic"

	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
	engine_v2 "massnet.org/mass/poc/engine.v2"
	spacekeeper_v2 "massnet.org/mass/poc/engine.v2/spacekeeper"
	_ "massnet.org/mass/poc/engine.v2/spacekeeper/skchia"
)

type SpaceKeeperV2 interface {
	spacekeeper_v2.SpaceKeeper
}

type ConfigurableSpaceKeeperV2 struct {
	spacekeeper_v2.SpaceKeeper
}

func NewConfigurableSpaceKeeperV2(sk spacekeeper_v2.SpaceKeeper) *ConfigurableSpaceKeeperV2 {
	return &ConfigurableSpaceKeeperV2{sk}
}

type MockedSpaceKeeperV2 struct {
	started int32 // atomic
}

func NewMockedSpaceKeeperV2() *MockedSpaceKeeperV2 {
	return &MockedSpaceKeeperV2{}
}

func (m *MockedSpaceKeeperV2) Start() error {
	atomic.StoreInt32(&m.started, 1)
	return nil
}

func (m *MockedSpaceKeeperV2) Stop() error {
	atomic.StoreInt32(&m.started, 0)
	return nil
}

func (m *MockedSpaceKeeperV2) Started() bool {
	return atomic.LoadInt32(&m.started) == 1
}

func (m *MockedSpaceKeeperV2) Type() string {
	return "spacekeeper.v2.mock"
}

func (m *MockedSpaceKeeperV2) WorkSpaceIDs(flags engine_v2.WorkSpaceStateFlags) ([]string, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) WorkSpaceInfos(flags engine_v2.WorkSpaceStateFlags) ([]engine_v2.WorkSpaceInfo, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetQuality(ctx context.Context, sid string, challenge pocutil.Hash) ([]*engine_v2.WorkSpaceQuality, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetQualities(ctx context.Context, flags engine_v2.WorkSpaceStateFlags, challenge pocutil.Hash) ([]*engine_v2.WorkSpaceQuality, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetQualityReader(ctx context.Context, sid string, challenge pocutil.Hash) (engine_v2.QualityReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetQualitiesReader(ctx context.Context, flags engine_v2.WorkSpaceStateFlags, challenge pocutil.Hash) (engine_v2.QualityReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetProof(ctx context.Context, sid string, challenge pocutil.Hash, index uint32) (*engine_v2.WorkSpaceProof, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetProofs(ctx context.Context, sids []string, challenge pocutil.Hash, indexes []uint32) ([]*engine_v2.WorkSpaceProof, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetProofReader(ctx context.Context, sid string, challenge pocutil.Hash, index uint32) (engine_v2.ProofReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetProofsReader(ctx context.Context, sids []string, challenge pocutil.Hash, indexes []uint32) (engine_v2.ProofReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) ActOnWorkSpace(sid string, action engine_v2.ActionType) error {
	return ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) ActOnWorkSpaces(flags engine_v2.WorkSpaceStateFlags, action engine_v2.ActionType) (map[string]error, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) SignHash(sid string, hash [32]byte) (*chiapos.G2Element, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV2) GetPrivateKey(sid string) (*chiapos.PrivateKey, error) {
	return nil, ErrNotImplemented
}
