package api

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/blockchain"
	"massnet.org/mass/config"
	"massnet.org/mass/database"
	"massnet.org/mass/logging"
	"massnet.org/mass/mining"
	"massnet.org/mass/netsync"
	"massnet.org/mass/poc/engine/pocminer"
	"massnet.org/mass/poc/wallet"
)

const (
	maxMsgSize        = 1e7
	GRPCListenAddress = "127.0.0.1"
)

type Server struct {
	rpcServer   *grpc.Server
	db          database.Db
	config      *config.Config
	pocMiner    pocminer.PoCMiner
	spaceKeeper mining.SpaceKeeper
	chain       *blockchain.Blockchain
	txMemPool   *blockchain.TxPool
	syncManager *netsync.SyncManager
	pocWallet   *wallet.PoCWallet
	quitClient  func()
}

func NewServer(db database.Db, pocMiner pocminer.PoCMiner, spaceKeeper mining.SpaceKeeper, chain *blockchain.Blockchain,
	txMemPool *blockchain.TxPool, sm *netsync.SyncManager, pocWallet *wallet.PoCWallet, quitClient func(), config *config.Config) (*Server, error) {
	// set the size for receive Msg
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	}
	s := grpc.NewServer(opts...)
	srv := &Server{
		rpcServer:   s,
		db:          db,
		config:      config,
		pocMiner:    pocMiner,
		spaceKeeper: spaceKeeper,
		chain:       chain,
		txMemPool:   txMemPool,
		syncManager: sm,
		pocWallet:   pocWallet,
		quitClient:  quitClient,
	}
	pb.RegisterApiServiceServer(s, srv)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	logging.CPrint(logging.INFO, "new gRPC server")
	return srv, nil
}

func (s *Server) Start() error {
	address := fmt.Sprintf("%s%s%s", GRPCListenAddress, ":", s.config.Network.API.APIPortGRPC)
	listen, err := net.Listen("tcp", address)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to start tcp listener", logging.LogFormat{"port": s.config.Network.API.APIPortGRPC, "error": err})
		return err
	}
	go s.rpcServer.Serve(listen)
	logging.CPrint(logging.INFO, "gRPC server start", logging.LogFormat{"port": s.config.Network.API.APIPortGRPC})
	return nil
}

func (s *Server) Stop() {
	s.rpcServer.Stop()
	logging.CPrint(logging.INFO, "API server stopped")
}

func (s *Server) RunGateway() {
	go func() {
		if err := Run(s.config); err != nil {
			logging.CPrint(logging.ERROR, "failed to start gateway", logging.LogFormat{"port": s.config.Network.API.APIPortHttp, "error": err})
		}
	}()
	logging.CPrint(logging.INFO, "gRPC-gateway start", logging.LogFormat{"port": s.config.Network.API.APIPortHttp})
}
