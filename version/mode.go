package version

import coreversion "github.com/massnetorg/mass-core/version"

type ServiceMode uint8

const (
	ModeCore    ServiceMode = 0
	ModeMinerV1 ServiceMode = 1
	ModeMinerV2 ServiceMode = 2
)

func (mode ServiceMode) String() string {
	var s string
	switch mode {
	case ModeCore:
		s = "core"
	case ModeMinerV1:
		s = "miner_v1"
	case ModeMinerV2:
		s = "miner_v2"
	default:
		s = "invalid"
	}
	return s
}

func (mode ServiceMode) Is(target ServiceMode) bool {
	return mode == target
}

func GetVersion() string {
	return coreversion.GetVersion()
}
