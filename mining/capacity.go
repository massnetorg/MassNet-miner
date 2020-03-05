package mining

import (
	"reflect"

	"massnet.org/mass/logging"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/engine/spacekeeper"
	"massnet.org/mass/poc/engine/spacekeeper/capacity"
)

type SpaceKeeper interface {
	spacekeeper.SpaceKeeper
	Configured() bool
	ConfigureByBitLength(BlCount map[int]int, execPlot, execMine bool) ([]engine.WorkSpaceInfo, error)
	ConfigureBySize(targetSize int, password string) ([]engine.WorkSpaceInfo, error)
}

type ConfigurableSpaceKeeper struct {
	spacekeeper.SpaceKeeper
}

func NewConfigurableSpaceKeeper(sk spacekeeper.SpaceKeeper) *ConfigurableSpaceKeeper {
	return &ConfigurableSpaceKeeper{sk}
}

func (csk *ConfigurableSpaceKeeper) Configured() bool {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		return true
	}
	return sk.Configured()
}

func (csk *ConfigurableSpaceKeeper) ConfigureByBitLength(BlCount map[int]int, execPlot, execMine bool) ([]engine.WorkSpaceInfo, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, err
	}
	return sk.ConfigureByBitLength(BlCount, execPlot, execMine)
}

func (csk *ConfigurableSpaceKeeper) ConfigureBySize(targetSize int, password string) ([]engine.WorkSpaceInfo, error) {
	sk, err := getInstance(csk.SpaceKeeper)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to assert SpaceKeeper type", logging.LogFormat{"actual": reflect.TypeOf(sk)})
		return nil, err
	}
	return sk.ConfigureBySize(targetSize, password)
}

func getInstance(sk spacekeeper.SpaceKeeper) (*capacity.SpaceKeeper, error) {
	ins, ok := sk.(*capacity.SpaceKeeper)
	if !ok {
		return nil, spacekeeper.ErrUnimplemented
	}
	return ins, nil
}
