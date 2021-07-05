package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/pocec"
	"golang.org/x/net/context"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/config"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/wallet/keystore"
)

func (s *Server) ConfigureCapacity(ctx context.Context, in *pb.ConfigureSpaceKeeperRequest) (*pb.WorkSpacesResponse, error) {
	logging.CPrint(logging.INFO, "ConfigureCapacity called", logging.LogFormat{"capacity": in.Capacity, "payout_addresses": in.PayoutAddresses})
	defer logging.CPrint(logging.INFO, "ConfigureCapacity responded")

	err := checkPassLen(in.Passphrase)
	if err != nil {
		return nil, err
	}

	if len(in.PayoutAddresses) == 0 {
		logging.CPrint(logging.ERROR, "payout_addresses is empty")
		return nil, status.New(ErrAPIMinerNoAddress, ErrCode[ErrAPIMinerNoAddress]).Err()
	}
	for _, addr := range in.PayoutAddresses {
		err := checkAddressLen(addr)
		if err != nil {
			return nil, err
		}
	}
	payoutAddresses, err := massutil.NewAddressesFromStringList(in.PayoutAddresses, config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to decode coinbase_address", logging.LogFormat{"addr_list": in.PayoutAddresses})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}
	if len(payoutAddresses) > config.MaxMiningPayoutAddresses {
		logging.CPrint(logging.ERROR, "coinbase_address is more than allowed",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}

	var diskSize = in.Capacity * poc.MiB
	if err = checkMinerDiskSize(s.spaceKeeperV1, in.Capacity); err != nil {
		logging.CPrint(logging.ERROR, "invalid capacity size", logging.LogFormat{"err": err, "capacity": in.Capacity})
		return nil, err
	}

	if s.spaceKeeperV1.Started() {
		logging.CPrint(logging.ERROR, "cannot configure while capacity is running")
		return nil, status.New(ErrAPIMinerNotStopped, ErrCode[ErrAPIMinerNotStopped]).Err()
	}

	if err := s.pocMiner.SetPayoutAddresses(payoutAddresses); err != nil {
		logging.CPrint(logging.ERROR, "missing miner payout address",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	if err := s.pocWallet.Unlock([]byte(in.Passphrase)); err != nil {
		logging.CPrint(logging.ERROR, "fail to unlock poc wallet",
			logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerWrongPassphrase, ErrCode[ErrAPIMinerWrongPassphrase]).Err()
	}
	_, err = s.spaceKeeperV1.ConfigureBySize(diskSize, false, false)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to configure spaceKeeperV1", logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerInternal, err.Error()).Err()
	}
	resultList, err := s.getCapacitySpaces(engine.SFAll)
	if err != nil {
		return nil, err
	}

	return &pb.WorkSpacesResponse{SpaceCount: uint32(len(resultList)), Spaces: resultList}, nil
}

func (s *Server) GetCapacitySpaces(ctx context.Context, in *empty.Empty) (*pb.WorkSpacesResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpaces")

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	resultList, err := s.getCapacitySpaces(engine.SFAll)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "GetCapacitySpaces completed")
	return &pb.WorkSpacesResponse{SpaceCount: uint32(len(resultList)), Spaces: resultList}, nil
}

func (s *Server) ConfigureCapacityByDirs(ctx context.Context, in *pb.ConfigureSpaceKeeperByDirsRequest) (*pb.WorkSpacesByDirsResponse, error) {
	logging.CPrint(logging.INFO, "ConfigureCapacityByDirs called", logging.LogFormat{"capacity": in.Allocations, "payout_addresses": in.PayoutAddresses})
	defer logging.CPrint(logging.INFO, "ConfigureCapacityByDirs responded")

	err := checkPassLen(in.Passphrase)
	if err != nil {
		return nil, err
	}

	if len(in.PayoutAddresses) == 0 {
		logging.CPrint(logging.ERROR, "payout_addresses is empty")
		return nil, status.New(ErrAPIMinerNoAddress, ErrCode[ErrAPIMinerNoAddress]).Err()
	}
	for _, addr := range in.PayoutAddresses {
		err := checkAddressLen(addr)
		if err != nil {
			return nil, err
		}
	}
	payoutAddresses, err := massutil.NewAddressesFromStringList(in.PayoutAddresses, config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to decode coinbase_address", logging.LogFormat{"addr_list": in.PayoutAddresses})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}
	if len(payoutAddresses) > config.MaxMiningPayoutAddresses {
		logging.CPrint(logging.ERROR, "coinbase_address is more than allowed",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerInvalidAddress, ErrCode[ErrAPIMinerInvalidAddress]).Err()
	}

	dirs, capacities := make([]string, len(in.Allocations)), make([]uint64, len(in.Allocations))
	for i, alloc := range in.Allocations {
		if alloc == nil {
			logging.CPrint(logging.ERROR, "miner allocation is nil", logging.LogFormat{"index": i})
			return nil, status.New(ErrAPIMinerInvalidAllocation, ErrCode[ErrAPIMinerInvalidAllocation]).Err()
		}
		absDir, err := filepath.Abs(alloc.Directory)
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to get abs path", logging.LogFormat{"dir": alloc.Directory, "err": err})
			return nil, status.New(ErrAPIMinerInvalidAllocation, ErrCode[ErrAPIMinerInvalidAllocation]).Err()
		}
		if fi, err := os.Stat(absDir); err != nil {
			if !os.IsNotExist(err) {
				logging.CPrint(logging.ERROR, "fail to get file stat", logging.LogFormat{"err": err})
				return nil, status.New(ErrAPIMinerInvalidAllocation, ErrCode[ErrAPIMinerInvalidAllocation]).Err()
			}
			if err = os.MkdirAll(absDir, 0700); err != nil {
				logging.CPrint(logging.ERROR, "mkdir failed", logging.LogFormat{"dir": absDir, "err": err})
				return nil, err
			}
		} else if !fi.IsDir() {
			logging.CPrint(logging.ERROR, "not directory", logging.LogFormat{"dir": absDir})
			return nil, status.New(ErrAPIMinerInvalidAllocation, ErrCode[ErrAPIMinerInvalidAllocation]).Err()
		}
		if err = checkMinerPathCapacity(s.spaceKeeperV1, absDir, alloc.Capacity); err != nil {
			logging.CPrint(logging.ERROR, "invalid capacity size",
				logging.LogFormat{"err": err, "capacity": alloc.Capacity, "directory": absDir})
			return nil, err
		}
		dirs[i] = absDir
		capacities[i] = alloc.Capacity * poc.MiB
	}

	if s.spaceKeeperV1.Started() {
		logging.CPrint(logging.ERROR, "cannot configure while capacity is running")
		return nil, status.New(ErrAPIMinerNotStopped, ErrCode[ErrAPIMinerNotStopped]).Err()
	}

	if err := s.pocMiner.SetPayoutAddresses(payoutAddresses); err != nil {
		logging.CPrint(logging.ERROR, "missing miner payout address",
			logging.LogFormat{"allowed": config.MaxMiningPayoutAddresses, "count": len(payoutAddresses)})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	if err := s.pocWallet.Unlock([]byte(in.Passphrase)); err != nil {
		logging.CPrint(logging.ERROR, "fail to unlock poc wallet",
			logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerWrongPassphrase, ErrCode[ErrAPIMinerWrongPassphrase]).Err()
	}
	_, err = s.spaceKeeperV1.ConfigureByPath(dirs, capacities, false, false)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to configure spaceKeeperV1", logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerInternal, err.Error()).Err()
	}
	alloctions, err := s.getCapacitySpacesByDirs()
	if err != nil {
		return nil, err
	}
	return &pb.WorkSpacesByDirsResponse{DirectoryCount: uint32(len(alloctions)), Allocations: alloctions}, nil
}

func (s *Server) GetCapacitySpacesByDirs(ctx context.Context, in *empty.Empty) (*pb.WorkSpacesByDirsResponse, error) {
	logging.CPrint(logging.INFO, "GetCapacitySpacesByDirs called")
	defer logging.CPrint(logging.INFO, "GetCapacitySpaces responded")

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	allocations, err := s.getCapacitySpacesByDirs()
	if err != nil {
		return nil, err
	}

	return &pb.WorkSpacesByDirsResponse{DirectoryCount: uint32(len(allocations)), Allocations: allocations}, nil
}

func (s *Server) GetCapacitySpace(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.WorkSpaceResponse, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpace", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
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

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.spaceKeeperV1.Started() {
		s.spaceKeeperV1.Start()
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

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
		return nil, status.New(ErrAPIMinerNoConfig, ErrCode[ErrAPIMinerNoConfig]).Err()
	}
	if !s.spaceKeeperV1.Started() {
		s.spaceKeeperV1.Start()
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

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
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

	if !s.spaceKeeperV1.Configured() {
		logging.CPrint(logging.ERROR, "spaceKeeperV1 is not configured")
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

	if s.spaceKeeperV1.Started() {
		s.spaceKeeperV1.Stop()
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
	pkBytes := wsi.PublicKey.SerializeCompressed()
	pkStr := hex.EncodeToString(pkBytes)
	_, addr, err := keystore.NewPoCAddress(wsi.PublicKey, config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get poc address", logging.LogFormat{"err": err, "pubkey": pkStr})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	target, err := getBindingTarget(pkBytes, poc.ProofTypeDefault, wsi.BitLength)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get binding target", logging.LogFormat{"err": err, "pubkey": pkStr})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return &pb.WorkSpace{
		SpaceId:       wsi.SpaceID,
		Ordinal:       wsi.Ordinal,
		PublicKey:     pkStr,
		Address:       addr.EncodeAddress(),
		BitLength:     uint32(wsi.BitLength),
		State:         wsi.State.String(),
		Progress:      wsi.Progress,
		BindingTarget: target,
	}, nil
}

func (s *Server) getWorkSpaceInfos(flags engine.WorkSpaceStateFlags) ([]engine.WorkSpaceInfo, error) {
	wsiList, err := s.spaceKeeperV1.WorkSpaceInfos(flags)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get spaceKeeperV1 WorkSpaceInfos", logging.LogFormat{"err": err, "flags": flags})
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

func workSpaceInfos2ProtoAllocation(dir string, wsiList []engine.WorkSpaceInfo) (*pb.WorkSpacesByDirsResponse_Allocation, error) {
	spaceList := make([]*pb.WorkSpace, len(wsiList))
	totalMiB := big.NewInt(0)
	for i, wsi := range wsiList {
		result, err := workSpaceInfo2ProtoWorkSpace(wsi)
		if err != nil {
			return nil, err
		}
		spaceList[i] = result
		totalMiB.Add(totalMiB, big.NewInt(int64(poc.ProofTypeDefault.PlotSize(wsi.BitLength))/poc.MiB))
	}
	alloc := &pb.WorkSpacesByDirsResponse_Allocation{
		Directory:  dir,
		Capacity:   totalMiB.Text(10),
		SpaceCount: uint32(len(spaceList)),
		Spaces:     spaceList,
	}
	return alloc, nil
}

func (s *Server) getCapacitySpacesByDirs() ([]*pb.WorkSpacesByDirsResponse_Allocation, error) {
	dirs, infos, err := s.spaceKeeperV1.WorkSpaceInfosByDirs()
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get spaceKeeperV1 WorkSpaceInfos by dirs", logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	allocs := make([]*pb.WorkSpacesByDirsResponse_Allocation, len(dirs))
	for i := range dirs {
		allocs[i], err = workSpaceInfos2ProtoAllocation(dirs[i], infos[i])
		if err != nil {
			return nil, err
		}
	}
	return allocs, nil
}

func (s *Server) getCapacitySpace(sid string) (*pb.WorkSpace, error) {
	wsi, err := s.getWorkSpaceInfo(sid)
	if err != nil {
		return nil, err
	}
	return workSpaceInfo2ProtoWorkSpace(wsi)
}

func (s *Server) actOnWorkSpaces(flags engine.WorkSpaceStateFlags, action engine.ActionType) ([]*pb.WorkSpace, error) {
	errs, err := s.spaceKeeperV1.ActOnWorkSpaces(flags, action)
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
	if err = s.spaceKeeperV1.ActOnWorkSpace(wsi.SpaceID, action); err != nil {
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
