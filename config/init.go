package config

import (
	coreconfig "github.com/massnetorg/mass-core/config"
)

type Params = coreconfig.Params

var ChainParams *Params

func init() {
	ChainParams = &coreconfig.ChainParams
}
