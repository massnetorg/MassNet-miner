package api

import (
	"fmt"
	"net"

	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/database"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/netsync"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/config"
	"massnet.org/mass/mining"
	"massnet.org/mass/version"
)

const (
	maxMsgSize        = 1e7
	GRPCListenAddress = "127.0.0.1"
)

type Server struct {
	rpcServer     *grpc.Server
	config        *config.API
	db            database.Db
	chain         *blockchain.Blockchain
	txMemPool     *blockchain.TxPool
	syncManager   *netsync.SyncManager
	pocMiner      mining.PoCMiner
	pocWallet     mining.PoCWallet
	spaceKeeperV1 mining.SpaceKeeperV1
	spaceKeeperV2 mining.SpaceKeeperV2
	serviceMode   version.ServiceMode
	quitClient    func()
}

func NewServer(cfg *config.API, db database.Db, chain *blockchain.Blockchain, txMemPool *blockchain.TxPool, sm *netsync.SyncManager,
	pocMiner mining.PoCMiner, pocWallet mining.PoCWallet, spaceKeeperV1 mining.SpaceKeeperV1, spaceKeeperV2 mining.SpaceKeeperV2,
	serviceMode version.ServiceMode, quitClient func()) (*Server, error) {
	// set the size for receive Msg
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	}
	s := grpc.NewServer(opts...)
	srv := &Server{
		rpcServer:     s,
		config:        cfg,
		db:            db,
		chain:         chain,
		txMemPool:     txMemPool,
		syncManager:   sm,
		pocMiner:      pocMiner,
		pocWallet:     pocWallet,
		spaceKeeperV1: spaceKeeperV1,
		spaceKeeperV2: spaceKeeperV2,
		serviceMode:   serviceMode,
		quitClient:    quitClient,
	}
	pb.RegisterApiServiceServer(s, srv)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	logging.CPrint(logging.INFO, "new api server")
	return srv, nil
}

func (s *Server) Start() error {
	address := fmt.Sprintf("%s:%d", GRPCListenAddress, s.config.PortGRPC)
	listen, err := net.Listen("tcp", address)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to start tcp listener", logging.LogFormat{"port": s.config.PortGRPC, "err": err})
		return err
	}
	go s.rpcServer.Serve(listen)
	logging.CPrint(logging.INFO, "gRPC server start", logging.LogFormat{"port": s.config.PortGRPC})
	return nil
}

func (s *Server) Stop() {
	s.rpcServer.Stop()
	logging.CPrint(logging.INFO, "API server stopped")
}

func (s *Server) RunGateway() {
	go func() {
		if err := Run(s.config); err != nil {
			logging.CPrint(logging.ERROR, "failed to start gateway", logging.LogFormat{"port": s.config.PortHttp, "err": err})
		}
	}()
	logging.CPrint(logging.INFO, "gRPC-gateway start", logging.LogFormat{"port": s.config.PortHttp})
}
