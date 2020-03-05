package main

import (
	"os"
	"sync"
	"sync/atomic"

	"massnet.org/mass/api"
	"massnet.org/mass/blockchain"
	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/mining"
	"massnet.org/mass/netsync"
	"massnet.org/mass/poc/engine/pocminer"
	"massnet.org/mass/poc/engine/spacekeeper"
	"massnet.org/mass/poc/wallet"
	"massnet.org/mass/wire"
)

type server struct {
	started     int32 // atomic
	shutdown    int32 // atomic
	apiServer   *api.Server
	db          database.Db
	chain       *blockchain.Blockchain
	syncManager *netsync.SyncManager
	pocWallet   *wallet.PoCWallet
	pocMiner    pocminer.PoCMiner
	spaceKeeper spacekeeper.SpaceKeeper
	wg          sync.WaitGroup
	quit        chan struct{}
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		logging.CPrint(logging.INFO, "started exit", logging.LogFormat{"started": s.started})
		return
	}

	logging.CPrint(logging.TRACE, "starting server")

	// srvrLog.Trace("Starting server")
	logging.CPrint(logging.INFO, "starting any com")

	// Start SyncManager
	s.syncManager.Start()

	// Start SpaceKeeper
	if cfg.Miner.Plot {
		s.spaceKeeper.Start()
	}

	// Start the CPU miner if generation is enabled.
	if cfg.Miner.Generate {
		s.pocMiner.Start()
	}

	s.apiServer.Start()

	s.apiServer.RunGateway()

	s.wg.Add(1)
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		logging.CPrint(logging.INFO, "server is already in the process of shutting down")
		return nil
	}

	// Shutdown apiServer
	s.apiServer.Stop()

	// Stop the CPU miner if needed
	s.pocMiner.Stop()

	if s.spaceKeeper.Started() {
		s.spaceKeeper.Stop()
	}

	if err := s.pocWallet.Close(); err != nil {
		logging.CPrint(logging.ERROR, "fail to quit wallet", logging.LogFormat{"err": err})
	}

	s.syncManager.Stop()

	// Signal the remaining goroutines to quit.
	close(s.quit)

	s.wg.Done()

	return nil
}

// WaitForShutdown blocks until the main listener and peer handlers are stopped.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
}

// newServer returns a new mass server configured to listen on addr for the
// Mass network.
func newServer(miningAddrs []massutil.Address, db database.Db, pocWallet *wallet.PoCWallet) (*server, error) {
	s := &server{
		quit:      make(chan struct{}),
		db:        db,
		pocWallet: pocWallet,
	}

	var err error
	// Create Blockchain
	s.chain, err = blockchain.NewBlockchain(db, cfg.Db.DataDir, s)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on new BlockChain", logging.LogFormat{"err": err})
		return nil, err
	}

	// New SyncManager
	newBlockCh := make(chan *wire.Hash, consensus.MaxNewBlockChSize)
	syncManager, err := netsync.NewSyncManager(cfg, s.chain, s.chain.GetTxPool(), newBlockCh)
	if err != nil {
		return nil, err
	}
	s.syncManager = syncManager

	// Create SpaceKeeper according to SpaceKeeperBackend
	switch cfg.Miner.SpacekeeperBackend {
	case "poolmanager":
		s.spaceKeeper, err = spacekeeper.NewSpaceKeeper(cfg.Miner.SpacekeeperBackend, cfg, s.chain)
	default:
		s.spaceKeeper, err = spacekeeper.NewSpaceKeeper(cfg.Miner.SpacekeeperBackend, cfg, pocWallet)
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on NewSpaceKeeper", logging.LogFormat{"err": err, "backend": cfg.Miner.SpacekeeperBackend})
		return nil, err
	}

	// Create PoCMiner according to MinerBackend
	s.pocMiner, err = pocminer.NewPoCMiner(cfg.Miner.PocminerBackend, cfg.Miner.AllowSolo, s.chain, s.syncManager, s.spaceKeeper, newBlockCh, miningAddrs)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on NewPoCMiner", logging.LogFormat{"err": err, "backend": cfg.Miner.PocminerBackend})
		return nil, err
	}

	// Create API Server
	s.apiServer, err = api.NewServer(s.db, s.pocMiner, mining.NewConfigurableSpaceKeeper(s.spaceKeeper), s.chain, s.chain.GetTxPool(), s.syncManager, pocWallet, func() { interruptChannel <- os.Interrupt }, cfg)
	if err != nil {
		logging.CPrint(logging.ERROR, "new server", logging.LogFormat{"err": err})
		return nil, err
	}

	return s, nil
}
