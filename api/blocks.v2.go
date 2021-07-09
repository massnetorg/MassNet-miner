package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/wire"
	wirepb "github.com/massnetorg/mass-core/wire/pb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/config"
)

const (
	blockIDHeight = iota
	blockIDHash
)

var ErrInvalidBlockID = errors.New("invalid id for block")

var (
	RegexpBlockHeightPattern = `^height-\d+$`
	RegexpBlockHashPattern   = `^hash-[a-fA-F0-9]{64}$`
	RegexpBlockHeight        *regexp.Regexp
	RegexpBlockHash          *regexp.Regexp
)

func init() {
	var err error
	RegexpBlockHeight, err = regexp.Compile(RegexpBlockHeightPattern)
	if err != nil {
		panic(err)
	}
	RegexpBlockHash, err = regexp.Compile(RegexpBlockHashPattern)
	if err != nil {
		panic(err)
	}
}

func (s *Server) GetBlockV2(ctx context.Context, in *pb.GetBlockRequestV2) (*pb.GetBlockResponseV2, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetBlockV2 called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetBlockV2 responded", logging.LogFormat{"req_id": reqID})

	if err := checkBlockIDLen(in.Id); err != nil {
		return nil, err
	}

	blk, err := s.getBlockByID(in.Id)
	if err != nil {
		return nil, err
	}

	resp, err := s.marshalGetBlockV2Response(blk)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *Server) GetBlockHeaderV2(ctx context.Context, in *pb.GetBlockRequestV2) (*pb.GetBlockHeaderResponse, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetBlockHeaderV2 called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetBlockHeaderV2 responded", logging.LogFormat{"req_id": reqID})

	if err := checkBlockIDLen(in.Id); err != nil {
		return nil, err
	}

	blk, err := s.getBlockByID(in.Id)
	if err != nil {
		return nil, err
	}

	resp, err := s.marshalGetBlockHeaderResponse(blk)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *Server) GetBlockVerbose1V2(ctx context.Context, in *pb.GetBlockRequestV2) (*pb.GetBlockResponse, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetBlockVerbose1V2 called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetBlockVerbose1V2 responded", logging.LogFormat{"req_id": reqID})

	if err := checkBlockIDLen(in.Id); err != nil {
		return nil, err
	}

	blk, err := s.getBlockByID(in.Id)
	if err != nil {
		return nil, err
	}

	resp, err := s.marshalGetBlockResponse(blk)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *Server) marshalGetBlockResponse(blk *massutil.Block) (*pb.GetBlockResponse, error) {
	idx := blk.Height()
	maxIdx := s.chain.BestBlockHeight()
	var shaNextStr string
	if idx < maxIdx {
		shaNext, err := s.chain.GetBlockHashByHeight(idx + 1)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to query next block hash according to the block height", logging.LogFormat{"height": idx, "error": err})
			st := status.New(ErrAPIBlockHashByHeight, ErrCode[ErrAPIBlockHashByHeight])
			return nil, st.Err()
		}
		shaNextStr = shaNext.String()
	}
	blockHeader := &blk.MsgBlock().Header
	blockHash := blk.Hash().String()

	banList := make([]string, 0, len(blockHeader.BanList))
	for _, pk := range blockHeader.BanList {
		banList = append(banList, hex.EncodeToString(pk.SerializeCompressed()))
	}

	var proof = wirepb.ProofToProto(blockHeader.Proof)
	blockReply := &pb.GetBlockResponse{
		Hash:            blockHash,
		ChainId:         blockHeader.ChainID.String(),
		Version:         blockHeader.Version,
		Height:          idx,
		Confirmations:   maxIdx + 1 - idx,
		Time:            blockHeader.Timestamp.Unix(),
		PreviousHash:    blockHeader.Previous.String(),
		NextHash:        shaNextStr,
		TransactionRoot: blockHeader.TransactionRoot.String(),
		WitnessRoot:     blockHeader.WitnessRoot.String(),
		ProposalRoot:    blockHeader.ProposalRoot.String(),
		Target:          blockHeader.Target.Text(16),
		Quality:         blk.MsgBlock().Header.Quality().Text(16),
		Challenge:       hex.EncodeToString(blockHeader.Challenge.Bytes()),
		PublicKey:       hex.EncodeToString(blockHeader.PubKey.SerializeCompressed()),
		Proof:           &pb.Proof{X: hex.EncodeToString(proof.X), XPrime: hex.EncodeToString(proof.XPrime), BitLength: uint32(proof.BitLength)},
		BlockSignature:  createPoCSignatureResult(blockHeader.Signature),
		BanList:         banList,
		Size:            uint32(blk.Size()),
		TimeUtc:         blockHeader.Timestamp.UTC().Format(time.RFC3339),
		TxCount:         uint32(len(blk.Transactions())),
		BindingRoot:     blockHeader.BindingRoot.String(),
	}
	proposalArea := blk.MsgBlock().Proposals
	punishments := createFaultPubKeyResult(proposalArea.PunishmentArea)
	others := createNormalProposalResult(proposalArea.OtherArea)

	blockReply.ProposalArea = &pb.ProposalArea{
		PunishmentArea: punishments,
		OtherArea:      others,
	}

	txns := blk.Transactions()
	rawTxns := make([]*pb.TxRawResult, len(txns))
	for i, tx := range txns {
		rawTxn, err := s.createTxRawResult(config.ChainParams,
			tx.MsgTx(), tx.Hash().String(), blockHeader,
			blockHash, idx, maxIdx)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to query transactions in the block", logging.LogFormat{
				"height":   idx,
				"error":    err,
				"function": "GetBlock",
			})
			return nil, err
		}
		rawTxns[i] = rawTxn
	}
	blockReply.RawTx = rawTxns

	return blockReply, nil
}

func (s *Server) marshalGetBlockV2Response(blk *massutil.Block) (*pb.GetBlockResponseV2, error) {
	idx := blk.Height()
	maxIdx := s.chain.BestBlockHeight()
	var shaNextStr string
	if idx < maxIdx {
		shaNext, err := s.chain.GetBlockHashByHeight(idx + 1)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to query next block hash according to the block height", logging.LogFormat{"height": idx, "error": err})
			st := status.New(ErrAPIBlockHashByHeight, ErrCode[ErrAPIBlockHashByHeight])
			return nil, st.Err()
		}
		shaNextStr = shaNext.String()
	}
	blockHeader := &blk.MsgBlock().Header
	blockHash := blk.Hash().String()

	banList := make([]string, 0, len(blockHeader.BanList))
	for _, pk := range blockHeader.BanList {
		banList = append(banList, hex.EncodeToString(pk.SerializeCompressed()))
	}

	txns := blk.Transactions()
	var proof = wirepb.ProofToProto(blockHeader.Proof)
	blockReply := &pb.GetBlockResponseV2{
		Hash:            blockHash,
		ChainId:         blockHeader.ChainID.String(),
		Version:         blockHeader.Version,
		Height:          idx,
		Confirmations:   maxIdx + 1 - idx,
		Timestamp:       blockHeader.Timestamp.Unix(),
		Previous:        blockHeader.Previous.String(),
		Next:            shaNextStr,
		TransactionRoot: blockHeader.TransactionRoot.String(),
		WitnessRoot:     blockHeader.WitnessRoot.String(),
		ProposalRoot:    blockHeader.ProposalRoot.String(),
		Target:          blockHeader.Target.Text(16),
		Quality:         blk.MsgBlock().Header.Quality().Text(16),
		Challenge:       hex.EncodeToString(blockHeader.Challenge.Bytes()),
		PublicKey:       hex.EncodeToString(blockHeader.PubKey.SerializeCompressed()),
		Proof:           &pb.Proof{X: hex.EncodeToString(proof.X), XPrime: hex.EncodeToString(proof.XPrime), BitLength: uint32(proof.BitLength)},
		Signature:       createPoCSignatureResult(blockHeader.Signature),
		BanList:         banList,
		PlainSize:       uint32(blk.Size()),
		PacketSize:      uint32(blk.PacketSize()),
		TimeUtc:         blockHeader.Timestamp.UTC().Format(time.RFC3339),
		TxCount:         uint32(len(txns)),
		BindingRoot:     blockHeader.BindingRoot.String(),
	}
	proposalArea := blk.MsgBlock().Proposals
	punishments := createFaultPubKeyResult(proposalArea.PunishmentArea)
	others := createNormalProposalResult(proposalArea.OtherArea)

	blockReply.Proposals = &pb.ProposalArea{
		PunishmentArea: punishments,
		OtherArea:      others,
	}

	txIDs := make([]string, len(txns))
	for i, tx := range txns {
		txIDs[i] = tx.Hash().String()
	}
	blockReply.Txids = txIDs

	return blockReply, nil
}

func (s *Server) marshalGetBlockHeaderResponse(blk *massutil.Block) (*pb.GetBlockHeaderResponse, error) {
	maxIdx := s.chain.BestBlockHeight()

	var shaNextStr string
	idx := blk.Height()
	if idx < maxIdx {
		shaNext, err := s.chain.GetBlockHashByHeight(idx + 1)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to query next block hash according to the block height", logging.LogFormat{"height": idx, "error": err})
			st := status.New(ErrAPIBlockHashByHeight, ErrCode[ErrAPIBlockHashByHeight])
			return nil, st.Err()
		}
		shaNextStr = shaNext.String()
	}

	var proof = wirepb.ProofToProto(blk.MsgBlock().Header.Proof)

	msgBlock := blk.MsgBlock()

	banList := make([]string, 0, len(msgBlock.Header.BanList))
	for _, pk := range msgBlock.Header.BanList {
		banList = append(banList, hex.EncodeToString(pk.SerializeCompressed()))
	}

	quality := msgBlock.Header.Quality().Text(10)

	blockHeaderReply := &pb.GetBlockHeaderResponse{
		Hash:            blk.Hash().String(),
		ChainId:         msgBlock.Header.ChainID.String(),
		Version:         msgBlock.Header.Version,
		Height:          uint64(blk.Height()),
		Confirmations:   uint64(1 + maxIdx - blk.Height()),
		Timestamp:       msgBlock.Header.Timestamp.Unix(),
		PreviousHash:    msgBlock.Header.Previous.String(),
		NextHash:        shaNextStr,
		TransactionRoot: msgBlock.Header.TransactionRoot.String(),
		ProposalRoot:    msgBlock.Header.ProposalRoot.String(),
		Target:          fmt.Sprintf("%x", msgBlock.Header.Target),
		Challenge:       hex.EncodeToString(msgBlock.Header.Challenge.Bytes()),
		PublicKey:       hex.EncodeToString(msgBlock.Header.PubKey.SerializeCompressed()),
		Proof:           &pb.Proof{X: hex.EncodeToString(proof.X), XPrime: hex.EncodeToString(proof.XPrime), BitLength: uint32(proof.BitLength)},
		BlockSignature:  createPoCSignatureResult(msgBlock.Header.Signature),
		BanList:         banList,
		Quality:         quality,
		TimeUtc:         msgBlock.Header.Timestamp.UTC().Format(time.RFC3339),
		BindingRoot:     msgBlock.Header.BindingRoot.String(),
	}
	return blockHeaderReply, nil
}

func (s *Server) getBlockByID(id string) (*massutil.Block, error) {
	if height, err := decodeBlockID(id, blockIDHeight); err == nil {
		return s.chain.GetBlockByHeight(height.(uint64))
	}

	if hash, err := decodeBlockID(id, blockIDHash); err == nil {
		return s.chain.GetBlockByHash(hash.(*wire.Hash))
	}

	return nil, ErrInvalidBlockID
}

func decodeBlockID(id string, typ int) (interface{}, error) {
	switch typ {
	case blockIDHeight:
		if !RegexpBlockHeight.MatchString(id) {
			return nil, ErrInvalidBlockID
		}
		height, _ := strconv.Atoi(id[7:]) // already make sure
		return uint64(height), nil

	case blockIDHash:
		if !RegexpBlockHash.MatchString(id) {
			return nil, ErrInvalidBlockID
		}
		return wire.NewHashFromStr(id[5:])

	default:
		return nil, ErrInvalidBlockID
	}
}
