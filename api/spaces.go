package api

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/config"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/poc"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/wallet/keystore"
	"massnet.org/mass/pocec"
)

func (s *Server) ConfigureCapacity(ctx context.Context, in *pb.ConfigureSpaceKeeperRequest) (*pb.WorkSpacesResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for ConfigureCapacity", logging.LogFormat{"capacity": in.Capacity, "payout_addresses": in.PayoutAddresses})

	if len(in.PayoutAddresses) == 0 {
		logging.CPrint(logging.ERROR, "coinbase_address is empty")
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	for _, addr := range in.PayoutAddresses {
		err := checkAddressLen(addr)
		if err != nil {
			return nil, err
		}
	}
	err := checkPassLen(in.Passphrase)
	if err != nil {
		return nil, err
	}

	var diskSize = in.Capacity * poc.MiB
	if int(diskSize) < poc.MinDiskSize {
		logging.CPrint(logging.ERROR, "Capacity lower than MinDiskSize", logging.LogFormat{"capacity": in.Capacity})
		return nil, status.New(ErrAPIMinerInvalidCapacity, ErrCode[ErrAPIMinerInvalidCapacity]).Err()
	}
	// TODO: check disk size (SpaceKeeper would check size, still do it here?)
	if s.spaceKeeper.Started() {
		logging.CPrint(logging.ERROR, "cannot configure while capacity is running")
		return nil, status.New(ErrAPIMinerNotStopped, ErrCode[ErrAPIMinerNotStopped]).Err()
	}

	payoutAddresses, err := massutil.NewAddressesFromStringList(in.PayoutAddresses, &config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to decode coinbase_address", logging.LogFormat{"addr_list": in.PayoutAddresses})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}
	if len(payoutAddresses) > config.MaxMiningPayoutAddresses {
		logging.CPrint(logging.ERROR, "coinbase_address is more than allowed",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}
	if err := s.pocMiner.SetPayoutAddresses(payoutAddresses); err != nil {
		logging.CPrint(logging.ERROR, "missing miner payout address",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerNoAddress, ErrCode[ErrAPIMinerNoAddress]).Err()
	}
	_, err = s.spaceKeeper.ConfigureBySize(int(diskSize), in.Passphrase)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to configure spaceKeeper", logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerInternal, err.Error()).Err()
	}
	resultList, err := s.getCapacitySpaces(engine.SFAll)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "ConfigureCapacity completed")
	return &pb.WorkSpacesResponse{SpaceCount: uint32(len(resultList)), Spaces: resultList}, nil
}

func (s *Server) GetCapacitySpaces(ctx context.Context, in *empty.Empty) (*pb.WorkSpacesResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpaces")

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	resultList, err := s.getCapacitySpaces(engine.SFAll)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "GetCapacitySpaces completed")
	return &pb.WorkSpacesResponse{SpaceCount: uint32(len(resultList)), Spaces: resultList}, nil
}

func (s *Server) GetCapacitySpace(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.WorkSpaceResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpace", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}

	result, err := s.getCapacitySpace(in.SpaceId)
	if err != nil {
		return nil, err
	}
	resp := &pb.WorkSpaceResponse{
		Space: result,
	}

	logging.CPrint(logging.INFO, "GetCapacitySpace completed", logging.LogFormat{"resp": resp})
	return resp, nil
}

func (s *Server) PlotCapacitySpaces(ctx context.Context, in *empty.Empty) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for PlotCapacitySpaces", logging.LogFormat{"in": in.String()})

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.spaceKeeper.Started() {
		s.spaceKeeper.Start()
	}

	_, err := s.actOnWorkSpaces(engine.SFAll, engine.Plot)
	if err != nil {
		return nil, err
	}

	resp := &pb.ActOnSpaceKeeperResponse{}
	logging.CPrint(logging.INFO, "PlotCapacitySpaces completed", logging.LogFormat{"resp": resp})
	return resp, nil
}

func (s *Server) PlotCapacitySpace(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for PlotCapacitySpace", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.spaceKeeper.Started() {
		s.spaceKeeper.Start()
	}

	_, err = s.actOnWorkSpace(in.SpaceId, engine.Plot)
	if err != nil {
		return nil, err
	}

	resp := &pb.ActOnSpaceKeeperResponse{}
	logging.CPrint(logging.INFO, "PlotCapacitySpaces completed", logging.LogFormat{"resp": resp})
	return resp, nil
}

func (s *Server) MineCapacitySpaces(ctx context.Context, in *empty.Empty) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for MineCapacitySpaces", logging.LogFormat{"in": in.String()})

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.pocMiner.Started() {
		s.pocMiner.Start()
	}

	_, err := s.actOnWorkSpaces(engine.SFAll, engine.Mine)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "MineCapacitySpaces completed")
	return &pb.ActOnSpaceKeeperResponse{}, nil
}

func (s *Server) MineCapacitySpace(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for MineCapacitySpace", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	if !s.spaceKeeper.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeper is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.pocMiner.Started() {
		s.pocMiner.Start()
	}

	_, err = s.actOnWorkSpace(in.SpaceId, engine.Mine)
	if err != nil {
		return nil, err
	}

	resp := &pb.ActOnSpaceKeeperResponse{}
	logging.CPrint(logging.INFO, "MineCapacitySpace completed")
	return resp, nil
}

func (s *Server) StopCapacitySpaces(ctx context.Context, in *empty.Empty) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for StopCapacitySpaces", logging.LogFormat{"in": in.String()})

	if s.spaceKeeper.Started() {
		s.spaceKeeper.Stop()
	}
	if s.pocMiner.Started() {
		s.pocMiner.Stop()
	}

	_, err := s.actOnWorkSpaces(engine.SFAll, engine.Stop)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "StopCapacitySpaces completed")
	return &pb.ActOnSpaceKeeperResponse{}, nil
}

func (s *Server) StopCapacitySpace(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.ActOnSpaceKeeperResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for StopCapacitySpace", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	_, err = s.actOnWorkSpace(in.SpaceId, engine.Stop)
	if err != nil {
		return nil, err
	}
	if wsiList, err := s.getWorkSpaceInfos(engine.SFMining); err == nil && len(wsiList) == 0 {
		s.pocMiner.Stop()
	}

	resp := &pb.ActOnSpaceKeeperResponse{}
	logging.CPrint(logging.INFO, "StopCapacitySpace completed")
	return resp, nil
}

func decodeAPISpaceID(id string) (string, error) {
	data := strings.Split(id, "-")
	if len(data) != 2 {
		logging.CPrint(logging.ERROR, "invalid spaceID", logging.LogFormat{"spaceID": id})
		return "", errors.New("invalid spaceID")
	} else {
		bytePk, err := hex.DecodeString(data[0])
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid spaceID", logging.LogFormat{"spaceID": id})
			return "", err
		}
		_, err = strconv.Atoi(data[1])
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid spaceID", logging.LogFormat{"spaceID": id})
			return "", err
		}
		_, err = pocec.ParsePubKey(bytePk, pocec.S256())
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid spaceID", logging.LogFormat{"spaceID": id})
			return "", err
		}
		return id, nil
	}
}

func workSpaceInfo2ProtoWorkSpace(wsi engine.WorkSpaceInfo) (*pb.WorkSpace, error) {
	pkStr := hex.EncodeToString(wsi.PublicKey.SerializeCompressed())
	_, addr, err := keystore.NewPoCAddress(wsi.PublicKey, &config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get poc address", logging.LogFormat{"err": err, "pubkey": pkStr})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return &pb.WorkSpace{
		Ordinal:   wsi.Ordinal,
		PublicKey: pkStr,
		Address:   addr.EncodeAddress(),
		BitLength: uint32(wsi.BitLength),
		State:     wsi.State.String(),
		Progress:  wsi.Progress,
	}, nil
}

func (s *Server) getWorkSpaceInfos(flags engine.WorkSpaceStateFlags) ([]engine.WorkSpaceInfo, error) {
	wsiList, err := s.spaceKeeper.WorkSpaceInfos(flags)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get spaceKeeper WorkSpaceInfos", logging.LogFormat{"err": err, "flags": flags})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return wsiList, nil
}

func (s *Server) getCapacitySpaces(flags engine.WorkSpaceStateFlags) ([]*pb.WorkSpace, error) {
	wsiList, err := s.getWorkSpaceInfos(flags)
	if err != nil {
		return nil, err
	}
	resultList := make([]*pb.WorkSpace, len(wsiList))
	for i, wsi := range wsiList {
		result, err := workSpaceInfo2ProtoWorkSpace(wsi)
		if err != nil {
			return nil, err
		}
		resultList[i] = result
	}
	return resultList, nil
}

func (s *Server) getWorkSpaceInfo(sid string) (engine.WorkSpaceInfo, error) {
	sid, err := decodeAPISpaceID(sid)
	if err != nil {
		logging.CPrint(logging.ERROR, "invalid api spaceID", logging.LogFormat{"err": err, "sid": sid})
		return engine.WorkSpaceInfo{}, status.New(ErrAPIMinerInvalidSpaceID, ErrCode[ErrAPIMinerInvalidSpaceID]).Err()
	}
	wsiList, err := s.getWorkSpaceInfos(engine.SFAll)
	if err != nil {
		return engine.WorkSpaceInfo{}, err
	}
	for _, wsi := range wsiList {
		if sid == wsi.SpaceID {
			return wsi, nil
		}
	}
	logging.CPrint(logging.ERROR, "cannot find space by spaceID", logging.LogFormat{"sid": sid})
	return engine.WorkSpaceInfo{}, status.New(ErrAPIMinerSpaceNotFound, ErrCode[ErrAPIMinerSpaceNotFound]).Err()
}

func (s *Server) getCapacitySpace(sid string) (*pb.WorkSpace, error) {
	wsi, err := s.getWorkSpaceInfo(sid)
	if err != nil {
		return nil, err
	}
	return workSpaceInfo2ProtoWorkSpace(wsi)
}

func (s *Server) actOnWorkSpaces(flags engine.WorkSpaceStateFlags, action engine.ActionType) ([]*pb.WorkSpace, error) {
	errs, err := s.spaceKeeper.ActOnWorkSpaces(flags, action)
	if count, errMap := countErrors(errs); count != 0 || err != nil {
		logging.CPrint(logging.ERROR, "fail to act on WorkSpaces", logging.LogFormat{"errors": errMap, "err": err, "action": action})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return s.getCapacitySpaces(flags)
}

func (s *Server) actOnWorkSpace(sid string, action engine.ActionType) (*pb.WorkSpace, error) {
	wsi, err := s.getWorkSpaceInfo(sid)
	if err != nil {
		return nil, err
	}
	if err = s.spaceKeeper.ActOnWorkSpace(wsi.SpaceID, action); err != nil {
		logging.CPrint(logging.ERROR, "fail to act on WorkSpace", logging.LogFormat{"err": err, "action": action})
		return nil, err
	}
	return s.getCapacitySpace(sid)
}

func countErrors(errs map[string]error) (int, map[string]error) {
	var count int
	var resultMap = make(map[string]error)
	for desc, err := range errs {
		if err != nil {
			resultMap[desc] = err
			count++
		}
	}
	return count, resultMap
}
