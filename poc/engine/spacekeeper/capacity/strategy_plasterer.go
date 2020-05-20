// For more details about plasterer, see https://github.com/massnetorg/plasterer

package capacity

import (
	"github.com/panjf2000/ants"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil/service"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/engine/spacekeeper"
)

const TypeSpaceKeeperPlasterer = "spacekeeper.plasterer"

func NewSpaceKeeperPlasterer(args ...interface{}) (spacekeeper.SpaceKeeper, error) {
	cfg, poCWallet, err := parseArgs(args...)
	if err != nil {
		return nil, err
	}
	workerPool, err := ants.NewPoolPreMalloc(maxPoolWorker)
	if err != nil {
		return nil, err
	}
	sk := &SpaceKeeper{
		allowGenerateNewSpace: false,
		dbDirs:                cfg.Miner.ProofDir,
		dbType:                typeMassDBV1,
		wallet:                poCWallet,
		workSpaceIndex:        make([]*WorkSpaceMap, 0),
		workSpaceList:         make([]*WorkSpace, 0),
		queue:                 newPlotterQueue(),
		newQueuedWorkSpaceCh:  make(chan *queuedWorkSpace, plotterMaxChanSize),
		workerPool:            workerPool,
		fileWatcher:           func() {},
	}
	sk.BaseService = service.NewBaseService(sk, TypeSpaceKeeperV1)
	sk.generateInitialIndex = func() error { return generateInitialIndex(sk, typeMassDBV1, regMassDBV1, suffixMassDBV1) }

	if err = sk.generateInitialIndex(); err != nil {
		return nil, err
	}

	if cfg.Miner.PrivatePassword != "" {
		if err = poCWallet.Unlock([]byte(cfg.Miner.PrivatePassword)); err != nil {
			return nil, err
		}
		var wsiList []engine.WorkSpaceInfo
		var configureMethod = "ConfigureByFlags"
		wsiList, err = sk.ConfigureByFlags(engine.SFAll, cfg.Miner.Plot, cfg.Miner.Generate)
		if err != nil {
			return nil, err
		}
		logging.CPrint(logging.DEBUG, "try configure spaceKeeper", logging.LogFormat{"content": wsiList, "err": err, "method": configureMethod})
	}

	return sk, nil
}

func init() {
	spacekeeper.AddSpaceKeeperBackend(spacekeeper.SKBackend{
		Typ:            TypeSpaceKeeperPlasterer,
		NewSpaceKeeper: NewSpaceKeeperPlasterer,
	})
}
