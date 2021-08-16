package server

import (
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"massnet.org/mass/api"
	"massnet.org/mass/config"
	"massnet.org/mass/fractal"
	"massnet.org/mass/mining"
	pocminer_v2 "massnet.org/mass/poc/engine.v2/pocminer"
	miner_v2 "massnet.org/mass/poc/engine.v2/pocminer/miner"
	spacekeeper_v2 "massnet.org/mass/poc/engine.v2/spacekeeper"
	pocminer_v1 "massnet.org/mass/poc/engine/pocminer"
	spacekeeper_v1 "massnet.org/mass/poc/engine/spacekeeper"
	"massnet.org/mass/version"
)

type MinerServerV1 struct {
	srv         *Server
	api         *api.Server
	spaceKeeper spacekeeper_v1.SpaceKeeper
	pocMiner    pocminer_v1.PoCMiner
}

func NewMinerServerV1(cfg *config.Config, srv *Server, payoutAddresses []massutil.Address) (*MinerServerV1, error) {
	var err error
	var pocWallet mining.PoCWallet
	var pocMiner pocminer_v1.PoCMiner
	var spaceKeeper spacekeeper_v1.SpaceKeeper

	pocWallet, err = NewOrOpenPoCWallet(cfg)
	if err != nil {
		logging.CPrint(logging.ERROR, "unable to open poc wallet", logging.LogFormat{"err": err})
		return nil, err
	}
	if spaceKeeper, err = spacekeeper_v1.NewSpaceKeeper(cfg.Miner.SpacekeeperBackend, cfg, pocWallet); err != nil {
		logging.CPrint(logging.ERROR, "fail on NewSpaceKeeper",
			logging.LogFormat{"err": err, "backend": cfg.Miner.SpacekeeperBackend})
		return nil, err
	}

	// Create PoCMiner according to MinerBackend
	if pocMiner, err = pocminer_v1.NewPoCMiner(cfg.Miner.PocminerBackend, cfg.Miner.AllowSolo, srv.chain,
		srv.syncManager, spaceKeeper, srv.newBlockCh, payoutAddresses); err != nil {
		logging.CPrint(logging.ERROR, "fail on NewPoCMiner", logging.LogFormat{"err": err, "backend": cfg.Miner.PocminerBackend})
		return nil, err
	}

	// Create API Server
	apiServer, err := api.NewServer(cfg.API, srv.db, srv.chain, srv.chain.GetTxPool(), srv.syncManager, pocMiner, pocWallet,
		mining.NewConfigurableSpaceKeeperV1(spaceKeeper), mining.NewMockedSpaceKeeperV2(), version.ModeMinerV1, func() { go srv.Stop() })
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on new server (miner_v1)", logging.LogFormat{"err": err})
		return nil, err
	}

	minerServerV1 := &MinerServerV1{
		srv:         srv,
		api:         apiServer,
		spaceKeeper: spaceKeeper,
		pocMiner:    pocMiner,
	}
	srv.mode = version.ModeMinerV1

	return minerServerV1, nil
}

func (miner *MinerServerV1) Start() error {
	if err := miner.srv.Start(); err != nil {
		return err
	}

	if miner.srv.cfg.Miner.Plot {
		miner.spaceKeeper.Start()
	}

	if miner.srv.cfg.Miner.Generate {
		miner.pocMiner.Start()
	}

	miner.api.Start()
	miner.api.RunGateway()

	return nil
}

func (miner *MinerServerV1) Stop() error {
	miner.api.Stop()

	if miner.pocMiner.Started() {
		miner.pocMiner.Stop()
	}

	if miner.spaceKeeper.Started() {
		miner.spaceKeeper.Stop()
	}

	return miner.srv.Stop()
}

type MinerServerV2 struct {
	srv         *Server
	api         *api.Server
	spaceKeeper spacekeeper_v2.SpaceKeeper
	pocMiner    pocminer_v2.PoCMiner
}

func NewMinerServerV2(cfg *config.Config, srv *Server, payoutAddresses []massutil.Address, spaceKeeper spacekeeper_v2.SpaceKeeper, superior *fractal.LocalSuperior) (*MinerServerV2, error) {
	keystore, err := chiawallet.NewKeystoreFromFile(cfg.Miner.ChiaMinerKeystore)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on NewKeystoreFromFile", logging.LogFormat{"err": err, "keystore": cfg.Miner.ChiaMinerKeystore})
		return nil, err
	}

	pocMiner, err := pocminer_v2.NewPoCMiner(miner_v2.TypeChiaPosMiner, cfg.Miner.AllowSolo, srv.chain, srv.syncManager, spaceKeeper, srv.newBlockCh, payoutAddresses, keystore, superior)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on NewPoCMiner", logging.LogFormat{"err": err, "backend": cfg.Miner.PocminerBackend})
		return nil, err
	}

	// Create API Server
	apiServer, err := api.NewServer(cfg.API, srv.db, srv.chain, srv.chain.GetTxPool(), srv.syncManager, pocMiner, mining.NewMockedPoCWallet(),
		mining.NewMockedSpaceKeeperV1(), mining.NewConfigurableSpaceKeeperV2(spaceKeeper), version.ModeMinerV2, func() { go srv.Stop() })
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on new server (miner_v2)", logging.LogFormat{"err": err})
		return nil, err
	}

	minerServerV2 := &MinerServerV2{
		srv:         srv,
		api:         apiServer,
		spaceKeeper: spaceKeeper,
		pocMiner:    pocMiner,
	}
	srv.mode = version.ModeMinerV2

	return minerServerV2, nil
}

func (miner *MinerServerV2) Start() error {
	if err := miner.srv.Start(); err != nil {
		return err
	}

	if miner.srv.cfg.Miner.Plot {
		miner.spaceKeeper.Start()
	}

	if miner.srv.cfg.Miner.Generate {
		miner.pocMiner.Start()
	}

	miner.api.Start()
	miner.api.RunGateway()

	return nil
}

func (miner *MinerServerV2) Stop() error {
	miner.api.Stop()

	if miner.pocMiner.Started() {
		miner.pocMiner.Stop()
	}

	if miner.spaceKeeper.Started() {
		miner.spaceKeeper.Stop()
	}

	return miner.srv.Stop()
}
