package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/logging"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
)

var (
	keystoreFileNamePrefix = "keystore"
)

func (s *Server) GetKeystore(ctx context.Context, msg *empty.Empty) (*pb.GetKeystoreResponse, error) {
	keystores := make([]*pb.WalletSummary, 0)
	for _, summary := range s.pocWallet.GetManagedAddrManager() {
		keystores = append(keystores, &pb.WalletSummary{
			WalletId: summary.Name(),
			Remark:   summary.Remarks(),
		})
	}
	return &pb.GetKeystoreResponse{
		Wallets: keystores,
	}, nil
}

func (s *Server) ExportKeystore(ctx context.Context, in *pb.ExportKeystoreRequest) (*pb.ExportKeystoreResponse, error) {
	logging.CPrint(logging.INFO, "a request is received to export keystore", logging.LogFormat{"export keystore id": in.WalletId})
	err := checkWalletIdLen(in.WalletId)
	if err != nil {
		return nil, err
	}
	err = checkPassLen(in.Passphrase)
	if err != nil {
		return nil, err
	}
	// get keystore json from wallet
	keystoreJSON, err := s.pocWallet.ExportKeystore(in.WalletId, []byte(in.Passphrase))
	if err != nil {
		logging.CPrint(logging.ERROR, ErrCode[ErrAPIExportWallet], logging.LogFormat{
			"err": err,
		})
		return nil, status.New(ErrAPIExportWallet, ErrCode[ErrAPIExportWallet]).Err()
	}

	// write keystore json file to disk
	exportFileName := fmt.Sprintf("%s/%s-%s.json", in.ExportPath, keystoreFileNamePrefix, in.WalletId)
	file, err := os.OpenFile(exportFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		logging.CPrint(logging.ERROR, ErrCode[ErrAPIOpenFile], logging.LogFormat{
			"error": err,
		})
		return nil, status.New(ErrAPIOpenFile, ErrCode[ErrAPIOpenFile]).Err()
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	_, err = writer.WriteString(string(keystoreJSON))
	if err != nil {
		logging.CPrint(logging.ERROR, ErrCode[ErrAPIWriteFile], logging.LogFormat{
			"error": err,
		})
		return nil, status.New(ErrAPIWriteFile, ErrCode[ErrAPIWriteFile]).Err()
	}
	err = writer.Flush()
	if err != nil {
		logging.CPrint(logging.ERROR, ErrCode[ErrAPIFlush], logging.LogFormat{
			"error": err,
		})
		return nil, status.New(ErrAPIFlush, ErrCode[ErrAPIFlush]).Err()
	}

	logging.CPrint(logging.INFO, "the request to export keystore was successfully answered", logging.LogFormat{"export keystore id": in.WalletId})
	return &pb.ExportKeystoreResponse{
		Keystore: string(keystoreJSON),
	}, nil
}

func (s *Server) ImportKeystore(ctx context.Context, in *pb.ImportKeystoreRequest) (*pb.ImportKeystoreResponse, error) {
	err := checkPassLen(in.OldPassphrase)
	if err != nil {
		return nil, err
	}
	if len(in.NewPassphrase) != 0 {
		err = checkPassLen(in.NewPassphrase)
		if err != nil {
			return nil, err
		}
	}

	file, err := os.Open(in.ImportPath)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to open file", logging.LogFormat{"file": in.ImportPath, "error": err})
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	fileBytes, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		logging.CPrint(logging.ERROR, "failed to read file", logging.LogFormat{"error": err})
		return nil, err
	}

	accountID, remark, err := s.pocWallet.ImportKeystore(fileBytes, []byte(in.OldPassphrase), []byte(in.NewPassphrase))
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to import keystore", logging.LogFormat{"error": err})
		return nil, err
	}
	return &pb.ImportKeystoreResponse{
		Status:   true,
		WalletId: accountID,
		Remark:   remark,
	}, nil
}

func (s *Server) UnlockWallet(ctx context.Context, in *pb.UnlockWalletRequest) (*pb.UnlockWalletResponse, error) {
	logging.CPrint(logging.INFO, "api unlock wallet called")
	err := checkPassLen(in.Passphrase)
	if err != nil {
		return nil, err
	}
	resp := &pb.UnlockWalletResponse{}

	if !s.pocWallet.IsLocked() {
		logging.CPrint(logging.INFO, "api unlock wallet succeed")
		resp.Success, resp.Error = true, ""
		return resp, nil
	}

	if err := s.pocWallet.Unlock([]byte(in.Passphrase)); err != nil {
		logging.CPrint(logging.ERROR, "api unlock wallet failed", logging.LogFormat{"err": err})
		return nil, status.New(ErrAPIWalletInternal, err.Error()).Err()
	}

	logging.CPrint(logging.INFO, "api unlock wallet succeed")
	resp.Success, resp.Error = true, ""
	return resp, nil
}

func (s *Server) LockWallet(ctx context.Context, msg *empty.Empty) (*pb.LockWalletResponse, error) {
	logging.CPrint(logging.INFO, "api lock wallet called")
	resp := &pb.LockWalletResponse{}

	if s.pocWallet.IsLocked() {
		logging.CPrint(logging.INFO, "api lock wallet succeed")
		resp.Success, resp.Error = true, ""
		return resp, nil
	}

	if s.pocMiner.Started() {
		logging.CPrint(logging.ERROR, "api lock wallet failed", logging.LogFormat{"err": ErrCode[ErrAPIWalletIsMining]})
		return nil, status.New(ErrAPIWalletIsMining, ErrCode[ErrAPIWalletIsMining]).Err()
	}

	s.pocWallet.Lock()
	logging.CPrint(logging.INFO, "api lock wallet succeed")
	resp.Success, resp.Error = true, ""
	return resp, nil
}

func (s *Server) ChangePrivatePass(ctx context.Context, in *pb.ChangePrivatePassRequest) (*pb.ChangePrivatePassResponse, error) {
	err := checkPassLen(in.OldPrivpass)
	if err != nil {
		return nil, err
	}
	err = checkPassLen(in.NewPrivpass)
	if err != nil {
		return nil, err
	}
	err = s.pocWallet.ChangePrivPassphrase([]byte(in.OldPrivpass), []byte(in.NewPrivpass), nil)
	if err != nil {
		return nil, err
	}
	return &pb.ChangePrivatePassResponse{
		Success: true,
	}, nil
}

func (s *Server) ChangePublicPass(ctx context.Context, in *pb.ChangePublicPassRequest) (*pb.ChangePublicPassResponse, error) {
	err := checkPassLen(in.OldPubpass)
	if err != nil {
		return nil, err
	}
	err = checkPassLen(in.NewPubpass)
	if err != nil {
		return nil, err
	}
	err = s.pocWallet.ChangePubPassphrase([]byte(in.OldPubpass), []byte(in.NewPubpass), nil)
	if err != nil {
		return nil, err
	}
	return &pb.ChangePublicPassResponse{
		Success: true,
	}, nil
}
