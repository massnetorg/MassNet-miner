package mining

import (
	"context"
	"reflect"
	"sync/atomic"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/pocec"
	engine_v1 "massnet.org/mass/poc/engine"
	spacekeeper_v1 "massnet.org/mass/poc/engine/spacekeeper"
	"massnet.org/mass/poc/engine/spacekeeper/capacity"
)

type SpaceKeeperV1 interface {
	spacekeeper_v1.SpaceKeeper
	Configured() bool
	ConfigureByBitLength(BlCount map[int]int, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error)
	ConfigureBySize(targetSize uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error)
	ConfigureByPath(paths []string, sizes []uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error)
	AvailableDiskSize() (uint64, error)
	IsCapacityAvailable(path string, capacity uint64) error
	WorkSpaceInfosByDirs() (dirs []string, results [][]engine_v1.WorkSpaceInfo, err error)
}

type ConfigurableSpaceKeeperV1 struct {
	spacekeeper_v1.SpaceKeeper
}

func NewConfigurableSpaceKeeperV1(sk spacekeeper_v1.SpaceKeeper) *ConfigurableSpaceKeeperV1 {
	return &ConfigurableSpaceKeeperV1{sk}
}

func (csk *ConfigurableSpaceKeeperV1) Configured() bool {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		return true
	}
	return sk.Configured()
}

func (csk *ConfigurableSpaceKeeperV1) ConfigureByBitLength(BlCount map[int]int, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, err
	}
	return sk.ConfigureByBitLength(BlCount, execPlot, execMine)
}

func (csk *ConfigurableSpaceKeeperV1) ConfigureBySize(targetSize uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, err
	}
	return sk.ConfigureBySize(targetSize, execPlot, execMine)
}

func (csk *ConfigurableSpaceKeeperV1) ConfigureByPath(paths []string, sizes []uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, err
	}
	sizesInt := make([]int, len(sizes))
	for i := range sizes {
		sizesInt[i] = int(sizes[i])
	}
	return sk.ConfigureByPath(paths, sizesInt, execPlot, execMine)
}

func (csk *ConfigurableSpaceKeeperV1) AvailableDiskSize() (uint64, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return 0, err
	}
	return sk.AvailableDiskSize(), nil
}

func (csk *ConfigurableSpaceKeeperV1) IsCapacityAvailable(path string, capacity uint64) error {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return err
	}
	return sk.IsCapacityAvailable(path, capacity)
}

func (csk *ConfigurableSpaceKeeperV1) WorkSpaceInfosByDirs() (dirs []string, results [][]engine_v1.WorkSpaceInfo, err error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, nil, err
	}
	return sk.WorkSpaceInfosByDirs()
}

func getInstance(sk spacekeeper_v1.SpaceKeeper) (*capacity.SpaceKeeper, error) {
	ins, ok := sk.(*capacity.SpaceKeeper)
	if !ok {
		return nil, spacekeeper_v1.ErrUnimplemented
	}
	return ins, nil
}

type MockedSpaceKeeperV1 struct {
	started int32 // atomic
}

func NewMockedSpaceKeeperV1() *MockedSpaceKeeperV1 {
	return &MockedSpaceKeeperV1{}
}

func (m *MockedSpaceKeeperV1) Start() error {
	atomic.StoreInt32(&m.started, 1)
	return nil
}

func (m *MockedSpaceKeeperV1) Stop() error {
	atomic.StoreInt32(&m.started, 0)
	return nil
}

func (m *MockedSpaceKeeperV1) Started() bool {
	return atomic.LoadInt32(&m.started) == 1
}

func (m *MockedSpaceKeeperV1) Type() string {
	return "spacekeeper.v1.mock"
}

func (m *MockedSpaceKeeperV1) WorkSpaceIDs(flags engine_v1.WorkSpaceStateFlags) ([]string, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) WorkSpaceInfos(flags engine_v1.WorkSpaceStateFlags) ([]engine_v1.WorkSpaceInfo, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) GetProof(ctx context.Context, sid string, challenge pocutil.Hash, filter bool) (*engine_v1.WorkSpaceProof, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) GetProofs(ctx context.Context, flags engine_v1.WorkSpaceStateFlags, challenge pocutil.Hash, filter bool) ([]*engine_v1.WorkSpaceProof, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) GetProofReader(ctx context.Context, sid string, challenge pocutil.Hash, filter bool) (engine_v1.ProofReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) GetProofsReader(ctx context.Context, flags engine_v1.WorkSpaceStateFlags, challenge pocutil.Hash, filter bool) (engine_v1.ProofReader, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) ActOnWorkSpace(sid string, action engine_v1.ActionType) error {
	return ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) ActOnWorkSpaces(flags engine_v1.WorkSpaceStateFlags, action engine_v1.ActionType) (map[string]error, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) SignHash(sid string, hash [32]byte) (*pocec.Signature, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) Configured() bool {
	return false
}

func (m *MockedSpaceKeeperV1) ConfigureByBitLength(BlCount map[int]int, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) ConfigureBySize(targetSize uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) ConfigureByPath(paths []string, sizes []uint64, execPlot, execMine bool) ([]engine_v1.WorkSpaceInfo, error) {
	return nil, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) AvailableDiskSize() (uint64, error) {
	return 0, ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) IsCapacityAvailable(path string, capacity uint64) error {
	return ErrNotImplemented
}

func (m *MockedSpaceKeeperV1) WorkSpaceInfosByDirs() (dirs []string, results [][]engine_v1.WorkSpaceInfo, err error) {
	return nil, nil, ErrNotImplemented
}
