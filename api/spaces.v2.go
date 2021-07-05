package api

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
	engine_v2 "massnet.org/mass/poc/engine.v2"
)

func (s *Server) GetCapacitySpacesV2(ctx context.Context, in *empty.Empty) (*pb.WorkSpacesResponseV2, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpacesV2")

	resultList, err := s.getCapacitySpacesV2(engine_v2.SFAll)
	if err != nil {
		return nil, err
	}

	logging.CPrint(logging.INFO, "GetCapacitySpaces completed")
	return &pb.WorkSpacesResponseV2{SpaceCount: uint32(len(resultList)), Spaces: resultList}, nil
}

func (s *Server) GetCapacitySpaceV2(ctx context.Context, in *pb.WorkSpaceRequest) (*pb.WorkSpaceResponseV2, error) {
	logging.CPrint(logging.INFO, "Received a request for GetCapacitySpaceV2", logging.LogFormat{"in": in.String()})
	err := checkSpaceIDLen(in.SpaceId)
	if err != nil {
		return nil, err
	}

	result, err := s.getCapacitySpaceV2(in.SpaceId)
	if err != nil {
		return nil, err
	}
	resp := &pb.WorkSpaceResponseV2{
		Space: result,
	}

	logging.CPrint(logging.INFO, "GetCapacitySpace completed", logging.LogFormat{"resp": resp})
	return resp, nil
}

func (s *Server) getCapacitySpacesV2(flags engine_v2.WorkSpaceStateFlags) ([]*pb.WorkSpaceV2, error) {
	wsiList, err := s.getWorkSpaceInfosV2(flags)
	if err != nil {
		return nil, err
	}
	resultList := make([]*pb.WorkSpaceV2, len(wsiList))
	for i, wsi := range wsiList {
		result, err := workSpaceInfo2ProtoWorkSpaceV2(wsi)
		if err != nil {
			return nil, err
		}
		resultList[i] = result
	}
	return resultList, nil
}

func decodeAPISpaceIDV2(id string) (string, error) {
	data := strings.Split(id, "-")
	if len(data) != 2 {
		logging.CPrint(logging.ERROR, "invalid spaceIDV2", logging.LogFormat{"spaceID": id})
		return "", errors.New("invalid spaceID")
	} else {
		_, err := pocutil.DecodeStringToHash(data[0])
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid spaceIDV2", logging.LogFormat{"spaceID": id})
			return "", err
		}
		_, err = strconv.Atoi(data[1])
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid spaceIDV2", logging.LogFormat{"spaceID": id})
			return "", err
		}
		return id, nil
	}
}

func workSpaceInfo2ProtoWorkSpaceV2(wsi engine_v2.WorkSpaceInfo) (*pb.WorkSpaceV2, error) {
	pkStr := hex.EncodeToString(wsi.PublicKey.SerializeCompressed())
	target, err := getBindingTarget(wsi.PlotID.Bytes(), poc.ProofTypeChia, wsi.BitLength)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get binding target", logging.LogFormat{"err": err, "pubkey": pkStr})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return &pb.WorkSpaceV2{
		SpaceId:       wsi.SpaceID,
		PlotId:        wsi.PlotID.String(),
		PublicKey:     pkStr,
		BindingTarget: target,
		K:             uint32(wsi.BitLength),
	}, nil
}

func (s *Server) getWorkSpaceInfosV2(flags engine_v2.WorkSpaceStateFlags) ([]engine_v2.WorkSpaceInfo, error) {
	wsiList, err := s.spaceKeeperV2.WorkSpaceInfos(flags)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get spaceKeeperV2 WorkSpaceInfos", logging.LogFormat{"err": err, "flags": flags})
		return nil, status.New(ErrAPIMinerInternal, ErrCode[ErrAPIMinerInternal]).Err()
	}
	return wsiList, nil
}

func (s *Server) getWorkSpaceInfoV2(sid string) (engine_v2.WorkSpaceInfo, error) {
	sid, err := decodeAPISpaceIDV2(sid)
	if err != nil {
		logging.CPrint(logging.ERROR, "invalid api spaceIDV2", logging.LogFormat{"err": err, "sid": sid})
		return engine_v2.WorkSpaceInfo{}, status.New(ErrAPIMinerInvalidSpaceID, ErrCode[ErrAPIMinerInvalidSpaceID]).Err()
	}
	wsiList, err := s.getWorkSpaceInfosV2(engine_v2.SFAll)
	if err != nil {
		return engine_v2.WorkSpaceInfo{}, err
	}
	for _, wsi := range wsiList {
		if sid == wsi.SpaceID {
			return wsi, nil
		}
	}
	logging.CPrint(logging.ERROR, "cannot find space by spaceIDV2", logging.LogFormat{"sid": sid})
	return engine_v2.WorkSpaceInfo{}, status.New(ErrAPIMinerSpaceNotFound, ErrCode[ErrAPIMinerSpaceNotFound]).Err()
}

func (s *Server) getCapacitySpaceV2(sid string) (*pb.WorkSpaceV2, error) {
	wsi, err := s.getWorkSpaceInfoV2(sid)
	if err != nil {
		return nil, err
	}
	return workSpaceInfo2ProtoWorkSpaceV2(wsi)
}
