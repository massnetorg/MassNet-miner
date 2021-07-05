package api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/logging"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/version"
)

func (s *Server) GetClientStatus(ctx context.Context, in *empty.Empty) (*pb.GetClientStatusResponse, error) {
	logging.CPrint(logging.INFO, "a request is received to query the status of client")

	resp := &pb.GetClientStatusResponse{
		Version:         version.GetVersion(),
		PeerListening:   s.syncManager.Switch().IsListening(),
		Syncing:         !s.syncManager.IsCaughtUp(),
		Mining:          s.pocMiner.Started(),
		SpaceKeeping:    s.isSpaceKeeping(),
		LocalBestHeight: s.chain.BestBlockHeight(),
		ChainId:         s.chain.ChainID().String(),
		P2PId:           s.syncManager.NodeInfo().PubKey.KeyString(),
		ServiceMode:     s.serviceMode.String(),
	}

	if bestPeer := s.syncManager.BestPeer(); bestPeer != nil {
		resp.KnownBestHeight = bestPeer.Height
	}
	if resp.LocalBestHeight > resp.KnownBestHeight {
		resp.KnownBestHeight = resp.LocalBestHeight
	}

	var outCount, inCount uint32
	resp.Peers = &pb.GetClientStatusResponsePeerList{
		Outbound: make([]*pb.GetClientStatusResponsePeerInfo, 0),
		Inbound:  make([]*pb.GetClientStatusResponsePeerInfo, 0),
		Other:    make([]*pb.GetClientStatusResponsePeerInfo, 0),
	}
	for _, info := range s.syncManager.GetPeerInfos() {
		peer := &pb.GetClientStatusResponsePeerInfo{
			Id:      info.ID,
			Address: info.RemoteAddr,
		}
		if info.IsOutbound {
			outCount++
			peer.Direction = "outbound"
			resp.Peers.Outbound = append(resp.Peers.Outbound, peer)
			continue
		}
		inCount++
		peer.Direction = "inbound"
		resp.Peers.Inbound = append(resp.Peers.Inbound, peer)
	}
	resp.PeerCount = &pb.GetClientStatusResponsePeerCountInfo{Total: outCount + inCount, Outbound: outCount, Inbound: inCount}

	logging.CPrint(logging.INFO, "GetClientStatus completed")
	return resp, nil
}

func (s *Server) QuitClient(ctx context.Context, in *empty.Empty) (*pb.QuitClientResponse, error) {
	defer func() {
		go s.quitClient()
	}()
	return &pb.QuitClientResponse{
		Msg: "wait for client quitting process",
	}, nil
}

func (s *Server) isSpaceKeeping() bool {
	if s.serviceMode.Is(version.ModeMinerV1) {
		return s.spaceKeeperV1.Started()
	}
	if s.serviceMode.Is(version.ModeMinerV2) {
		return s.spaceKeeperV2.Started()
	}
	return false
}
