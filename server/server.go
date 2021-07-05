package server

import (
	"errors"
	"path/filepath"
	"sync/atomic"

	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/blockchain/state"
	coreconfig "github.com/massnetorg/mass-core/config"
	"github.com/massnetorg/mass-core/consensus"
	"github.com/massnetorg/mass-core/database"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/netsync"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/api"
	"massnet.org/mass/config"
	"massnet.org/mass/mining"
	"massnet.org/mass/version"
)

type Server struct {
	started     int32 // atomic
	shutdown    int32 // atomic
	mode        version.ServiceMode
	cfg         *config.Config
	db          database.Db
	chain       *blockchain.Blockchain
	syncManager *netsync.SyncManager
	newBlockCh  chan *wire.Hash
}

func (s *Server) Start() error {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		logging.CPrint(logging.INFO, "started exit", logging.LogFormat{"started": s.started})
		return errors.New("server already started")
	}
	logging.CPrint(logging.TRACE, "starting server")
	s.syncManager.Start()
	return nil
}

func (s *Server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		logging.CPrint(logging.INFO, "server is already in the process of shutting down")
		return nil
	}
	s.syncManager.Stop()
	return nil
}

func (s *Server) Mode() version.ServiceMode {
	return s.mode
}

func NewServer(cfg *config.Config, db database.Db, bindingDb state.Database, chainParams *config.Params) (*Server, error) {
	s := &Server{
		mode: version.ModeCore,
		cfg:  cfg,
		db:   db,
	}

	var err error
	// Create Blockchain
	checkpoints, err := coreconfig.ParseCheckpoints(cfg.Chain.AddCheckpoints)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on ParseCheckpoints", logging.LogFormat{"err": err})
		return nil, err
	}
	checkpoints = coreconfig.MergeCheckpoints(chainParams.Checkpoints, checkpoints)
	chainCfg := &blockchain.Config{
		DB:             db,
		StateBindingDb: bindingDb,
		ChainParams:    chainParams,
		Checkpoints:    checkpoints,
		CachePath:      filepath.Join(cfg.Datastore.Dir, blockchain.BlockCacheFileName),
	}
	s.chain, err = blockchain.NewBlockchain(chainCfg)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on new BlockChain", logging.LogFormat{"err": err})
		return nil, err
	}

	// New SyncManager
	s.newBlockCh = make(chan *wire.Hash, consensus.MaxNewBlockChSize)
	syncManager, err := netsync.NewSyncManager(cfg.CoreConfig(), s.chain, s.chain.GetTxPool(), s.newBlockCh)
	if err != nil {
		return nil, err
	}
	s.syncManager = syncManager

	return s, nil
}

type CoreServer struct {
	srv *Server
	api *api.Server
}

func NewCoreServer(cfg *config.Config, srv *Server) (*CoreServer, error) {
	// Create API Server
	apiServer, err := api.NewServer(cfg.API, srv.db, srv.chain, srv.chain.GetTxPool(), srv.syncManager,
		mining.NewMockedPoCMiner(), mining.NewMockedPoCWallet(), mining.NewMockedSpaceKeeperV1(), mining.NewMockedSpaceKeeperV2(),
		version.ModeCore, func() { go srv.Stop() })
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on new server (miner_v2)", logging.LogFormat{"err": err})
		return nil, err
	}

	coreServer := &CoreServer{
		srv: srv,
		api: apiServer,
	}
	srv.mode = version.ModeCore

	return coreServer, nil
}

func (core *CoreServer) Start() error {
	if err := core.srv.Start(); err != nil {
		return err
	}

	core.api.Start()
	core.api.RunGateway()

	return nil
}

func (core *CoreServer) Stop() error {
	core.api.Stop()
	return core.srv.Stop()
}
