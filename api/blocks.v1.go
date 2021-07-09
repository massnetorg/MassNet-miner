package api

import (
	"encoding/hex"
	"sort"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/interfaces"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/pocec"
	"github.com/massnetorg/mass-core/wire"
	wirepb "github.com/massnetorg/mass-core/wire/pb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
)

func (s *Server) GetBestBlock(ctx context.Context, msg *empty.Empty) (*pb.GetBestBlockResponse, error) {
	logging.CPrint(logging.INFO, "api get the best block height and hash")
	node := s.chain.BestBlockNode()
	sha, height := node.Hash, node.Height
	logging.CPrint(logging.INFO, "api get the best block height and hash succeed", logging.LogFormat{"height": height, "hash": sha.String()})
	return &pb.GetBestBlockResponse{Hash: sha.String(), Height: height}, nil
}

func (s *Server) GetBlock(ctx context.Context, in *pb.GetBlockRequest) (*pb.GetBlockResponse, error) {
	logging.CPrint(logging.INFO, "a request is received to query the block according to the block hash", logging.LogFormat{"hash": in.Hash})
	err := checkHashLen(in.Hash)
	if err != nil {
		return nil, err
	}
	sha, err := wire.NewHashFromStr(in.Hash)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to decode the input string into hash", logging.LogFormat{"input string": in.Hash, "error": err})
		st := status.New(ErrAPIShaHashFromStr, ErrCode[ErrAPIShaHashFromStr])
		return nil, st.Err()
	}
	blk, err := s.chain.GetBlockByHash(sha)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to query the block according to the block hash", logging.LogFormat{"hash": sha.String(), "error": err})
		st := status.New(ErrAPIBlockNotFound, ErrCode[ErrAPIBlockNotFound])
		return nil, st.Err()
	}

	blockReply, err := s.marshalGetBlockResponse(blk)
	if err == nil {
		logging.CPrint(logging.INFO, "the request to query the block according to the block hash was successfully answered", logging.LogFormat{"height": blk.Height()})
	}
	return blockReply, nil
}

func (s *Server) GetBlockHashByHeight(ctx context.Context, in *pb.GetBlockHashByHeightRequest) (*pb.GetBlockHashByHeightResponse, error) {
	logging.CPrint(logging.INFO, "api get block hash by height", logging.LogFormat{"height": in.Height})
	sha, err := s.chain.GetBlockHashByHeight(in.Height)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to get block hash by height", logging.LogFormat{"height": in.Height, "error": err})
		st := status.New(ErrAPIBlockHashByHeight, ErrCode[ErrAPIBlockHashByHeight])
		return nil, st.Err()
	}
	logging.CPrint(logging.INFO, "api get the block hash by height succeed", logging.LogFormat{"height": in.Height, "hash": sha.String()})
	return &pb.GetBlockHashByHeightResponse{Hash: sha.String()}, nil
}

func (s *Server) GetBlockByHeight(ctx context.Context, in *pb.GetBlockByHeightRequest) (*pb.GetBlockResponse, error) {
	logging.CPrint(logging.INFO, "api get block by height", logging.LogFormat{"height": in.Height})
	blk, err := s.chain.GetBlockByHeight(in.Height)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to query the block according to the block height", logging.LogFormat{"height": in.Height, "error": err})
		st := status.New(ErrAPIBlockNotFound, ErrCode[ErrAPIBlockNotFound])
		return nil, st.Err()
	}

	blockReply, err := s.marshalGetBlockResponse(blk)
	if err == nil {
		logging.CPrint(logging.INFO, "the request to query the block according to the block height was successfully answered", logging.LogFormat{"height": blk.Height()})
	}
	return blockReply, err
}

func (s *Server) GetBlockHeader(ctx context.Context, in *pb.GetBlockHeaderRequest) (*pb.GetBlockHeaderResponse, error) {
	logging.CPrint(logging.INFO, "a request is received to query the block header according to the block hash", logging.LogFormat{"hash": in.Hash})
	err := checkHashLen(in.Hash)
	if err != nil {
		return nil, err
	}
	sha, err := wire.NewHashFromStr(in.Hash)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to decode the input string into hash", logging.LogFormat{"input string": in.Hash})
		st := status.New(ErrAPIShaHashFromStr, ErrCode[ErrAPIShaHashFromStr])
		return nil, st.Err()
	}

	blk, err := s.chain.GetBlockByHash(sha)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to query the block according to the block hash", logging.LogFormat{"hash": sha.String(), "error": err})
		st := status.New(ErrAPIBlockNotFound, ErrCode[ErrAPIBlockNotFound])
		return nil, st.Err()
	}

	blockHeaderReply, err := s.marshalGetBlockHeaderResponse(blk)
	if err == nil {
		logging.CPrint(logging.INFO, "the request to query the block header according to the block hash was successfully answered", logging.LogFormat{"hash": in.Hash})
	}

	return blockHeaderReply, nil
}

func (s *Server) GetBlockHeightByPubKey(ctx context.Context, in *pb.GetBlockHeightByPubKeyRequest) (*pb.GetBlockHeightByPubKeyResponse, error) {
	logging.CPrint(logging.INFO, "a request is received to query all blocks submitted by provided public key",
		logging.LogFormat{"public key": in.PublicKey})
	err := checkPkStringLen(in.PublicKey)
	if err != nil {
		return nil, err
	}
	pubKeyBytes, err := hex.DecodeString(in.PublicKey)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to decode public key string", logging.LogFormat{"error": err})
		// TODO: change into ErrDecodePubKey
		return nil, err
	}
	pubKey, err := pocec.ParsePubKey(pubKeyBytes, pocec.S256())
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to parse public key", logging.LogFormat{"error": err})
		// TODO: change into ErrParsePubKey
		return nil, err
	}
	// TODO: add it in interface
	heights, err := s.chain.FetchMinedBlocks(pubKey)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to query all blocks submitted by provided public key",
			logging.LogFormat{"error": err})
		// TODO: change into ErrFetchMinedBLocks
		return nil, err
	}

	sort.Sort(sort.Reverse(heightList(heights)))
	logging.CPrint(logging.INFO,
		"the request to query all blocks submitted by provided public key was successfully answered",
		logging.LogFormat{"count": len(heights)})
	return &pb.GetBlockHeightByPubKeyResponse{
		Heights: heights,
	}, nil
}

func createNormalProposalResult(proposals []*wire.NormalProposal) []*pb.NormalProposal {
	result := make([]*pb.NormalProposal, 0, len(proposals))
	for _, p := range proposals {
		np := &pb.NormalProposal{
			Version:      p.Version(),
			ProposalType: uint32(p.Type()),
			Data:         hex.EncodeToString(p.Content()),
		}
		result = append(result, np)
	}
	return result
}

func createFaultPubKeyResult(proposals []*wire.FaultPubKey) []*pb.FaultPubKey {
	result := make([]*pb.FaultPubKey, 0, len(proposals))
	for _, p := range proposals {
		t := make([]*pb.Header, 0, wire.HeadersPerProposal)
		for _, h := range p.Testimony {
			ban := make([]string, 0, len(h.BanList))
			for _, pk := range h.BanList {
				ban = append(ban, hex.EncodeToString(pk.SerializeCompressed()))
			}

			proof := wirepb.ProofToProto(h.Proof)

			th := &pb.Header{
				Hash:            h.BlockHash().String(),
				ChainId:         h.ChainID.String(),
				Version:         h.Version,
				Height:          h.Height,
				Time:            h.Timestamp.Unix(),
				PreviousHash:    h.Previous.String(),
				TransactionRoot: h.TransactionRoot.String(),
				WitnessRoot:     h.WitnessRoot.String(),
				ProposalRoot:    h.ProposalRoot.String(),
				Target:          h.Target.Text(16),
				Challenge:       hex.EncodeToString(h.Challenge.Bytes()),
				PublicKey:       hex.EncodeToString(h.PubKey.SerializeCompressed()),
				Proof:           &pb.Proof{X: hex.EncodeToString(proof.X), XPrime: hex.EncodeToString(proof.XPrime), BitLength: uint32(proof.BitLength)},
				BlockSignature:  createPoCSignatureResult(h.Signature),
				BanList:         ban,
				BindingRoot:     h.BindingRoot.String(),
			}
			t = append(t, th)
		}

		fpk := &pb.FaultPubKey{
			Version:      p.Version(),
			ProposalType: uint32(p.Type()),
			PublicKey:    hex.EncodeToString(p.PubKey.SerializeCompressed()),
			Testimony:    t,
		}
		result = append(result, fpk)
	}
	return result
}

// TODO: show more types
func createPoCSignatureResult(sigI interfaces.Signature) *pb.PoCSignature {
	if sig, ok := sigI.(*pocec.Signature); ok {
		return &pb.PoCSignature{R: hex.EncodeToString(sig.R.Bytes()), S: hex.EncodeToString(sig.S.Bytes())}
	}
	return &pb.PoCSignature{}
}
