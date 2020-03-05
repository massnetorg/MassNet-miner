package api

import (
	"encoding/hex"
	"errors"
	"reflect"
	"sort"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc/status"
	pb "massnet.org/mass/api/proto"
	"massnet.org/mass/blockchain"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

func (s *Server) GetCoinbase(ctx context.Context, in *pb.GetCoinbaseRequest) (*pb.GetCoinbaseResponse, error) {
	block, err := s.chain.GetBlockByHeight(in.Height)
	if err != nil {
		return nil, err
	}

	currentHeight := s.chain.BestBlockHeight()

	txs := block.Transactions()
	if len(txs) <= 0 {
		return nil, errors.New("cannot find transaction in block")
	}
	coinbase := txs[0]
	msgtx := coinbase.MsgTx()

	vins, bindingValue, err := s.showCoinbaseInputDetails(msgtx)
	if err != nil {
		return nil, err
	}
	bindingValueStr, err := AmountToString(bindingValue)
	if err != nil {
		return nil, err
	}

	vouts, totalFees, err := s.showCoinbaseOutputDetails(msgtx, &config.ChainParams, block.Height(), bindingValue, block.MsgBlock().Header.Proof.BitLength)
	if err != nil {
		return nil, err
	}

	confirmations := currentHeight - block.Height()
	var status int32
	if confirmations >= consensus.CoinbaseMaturity {
		status = 4
	} else {
		status = 3
	}

	feesStr, err := AmountToString(totalFees)
	if err != nil {
		return nil, err
	}

	return &pb.GetCoinbaseResponse{
		Txid:     msgtx.TxHash().String(),
		Version:  msgtx.Version,
		LockTime: msgtx.LockTime,
		Block: &pb.BlockInfoForTx{
			Height:    block.Height(),
			BlockHash: block.Hash().String(),
			Timestamp: block.MsgBlock().Header.Timestamp.Unix(),
		},
		BindingValue:  bindingValueStr,
		Vin:           vins,
		Vout:          vouts,
		Payload:       hex.EncodeToString(msgtx.Payload),
		Confirmations: confirmations,
		Size:          uint32(msgtx.PlainSize()),
		TotalFees:     feesStr,
		Status:        status,
	}, nil
}

func (s *Server) GetTxPool(ctx context.Context, in *empty.Empty) (*pb.GetTxPoolResponse, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetTxPool called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetTxPool responded", logging.LogFormat{"req_id": reqID})

	resp := &pb.GetTxPoolResponse{}
	if err := s.marshalGetTxPoolResponse(reflect.ValueOf(resp), -1); err != nil {
		logging.CPrint(logging.ERROR, "GetTxPool fail on marshalGetTxPoolResponse", logging.LogFormat{"err": err})
		return nil, err
	}
	return resp, nil
}

func (s *Server) GetTxPoolVerbose0(ctx context.Context, in *empty.Empty) (*pb.GetTxPoolVerbose0Response, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetTxPoolVerbose0 called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetTxPoolVerbose0 responded", logging.LogFormat{"req_id": reqID})

	resp := &pb.GetTxPoolVerbose0Response{}
	if err := s.marshalGetTxPoolResponse(reflect.ValueOf(resp), 0); err != nil {
		logging.CPrint(logging.ERROR, "GetTxPoolVerbose0 fail on marshalGetTxPoolResponse", logging.LogFormat{"err": err})
		return nil, err
	}
	return resp, nil
}

func (s *Server) GetTxPoolVerbose1(ctx context.Context, in *empty.Empty) (*pb.GetTxPoolVerbose1Response, error) {
	var reqID = generateReqID()
	logging.CPrint(logging.INFO, "GetTxPoolVerbose1 called", logging.LogFormat{"req_id": reqID})
	defer logging.CPrint(logging.INFO, "GetTxPoolVerbose1 responded", logging.LogFormat{"req_id": reqID})

	resp := &pb.GetTxPoolVerbose1Response{}
	if err := s.marshalGetTxPoolResponse(reflect.ValueOf(resp), 1); err != nil {
		logging.CPrint(logging.ERROR, "GetTxPoolVerbose1 fail on marshalGetTxPoolResponse", logging.LogFormat{"err": err})
		return nil, err
	}
	return resp, nil
}

func (s *Server) marshalGetTxPoolResponse(resp reflect.Value, verbose int) error {
	resp = reflect.Indirect(resp)
	txs := s.txMemPool.TxDescs()
	sort.Sort(sort.Reverse(txDescList(txs)))
	orphans := s.txMemPool.OrphanTxs()
	sort.Sort(sort.Reverse(orphanTxDescList(orphans)))
	var txsPlainSize, txsPacketSize, orphansPlainSize, orphansPacketSize int
	txIDs, orphanIDs := make([]string, 0, len(txs)), make([]string, 0, len(orphans))

	for _, tx := range txs {
		txsPlainSize += tx.Tx.PlainSize()
		txsPacketSize += tx.Tx.PacketSize()
		txIDs = append(txIDs, tx.Tx.Hash().String())
	}
	for _, orphan := range orphans {
		orphansPlainSize += orphan.PlainSize()
		orphansPacketSize += orphan.PacketSize()
		orphanIDs = append(orphanIDs, orphan.Hash().String())
	}

	// write common parts of GetTxPoolResponse, GetTxPoolResponseV0 and GetTxPoolResponseV1
	resp.FieldByName("TxCount").SetUint(uint64(len(txs)))
	resp.FieldByName("OrphanCount").SetUint(uint64(len(orphans)))
	resp.FieldByName("TxPlainSize").SetUint(uint64(txsPlainSize))
	resp.FieldByName("TxPacketSize").SetUint(uint64(txsPacketSize))
	resp.FieldByName("OrphanPlainSize").SetUint(uint64(orphansPlainSize))
	resp.FieldByName("OrphanPacketSize").SetUint(uint64(orphansPacketSize))
	resp.FieldByName("Txs").Set(reflect.ValueOf(txIDs))
	resp.FieldByName("Orphans").Set(reflect.ValueOf(orphanIDs))

	// write differential parts
	var err error
	switch verbose {
	case 0:
		txDescsV0 := make([]*pb.GetTxDescVerbose0Response, 0, len(txs))
		for _, tx := range txs {
			txResp := &pb.GetTxDescVerbose0Response{}
			if err = s.marshalGetTxDescResponse(reflect.ValueOf(txResp), tx, verbose); err != nil {
				return err
			}
			txDescsV0 = append(txDescsV0, txResp)
		}
		orphanDescs := make([]*pb.GetOrphanTxDescResponse, 0, len(orphans))
		for _, orphan := range orphans {
			orphanResp := &pb.GetOrphanTxDescResponse{}
			if err = s.marshalGetOrphanTxDescResponse(reflect.ValueOf(orphanResp), orphan); err != nil {
				return err
			}
			orphanDescs = append(orphanDescs, orphanResp)
		}
		resp.FieldByName("TxDescs").Set(reflect.ValueOf(txDescsV0))
		resp.FieldByName("OrphanDescs").Set(reflect.ValueOf(orphanDescs))

	case 1:
		txDescsV1 := make([]*pb.GetTxDescVerbose1Response, 0, len(txs))
		for _, tx := range txs {
			txResp := &pb.GetTxDescVerbose1Response{}
			if err = s.marshalGetTxDescResponse(reflect.ValueOf(txResp), tx, verbose); err != nil {
				return err
			}
			txDescsV1 = append(txDescsV1, txResp)
		}
		orphanDescs := make([]*pb.GetOrphanTxDescResponse, 0, len(orphans))
		for _, orphan := range orphans {
			orphanResp := &pb.GetOrphanTxDescResponse{}
			if err = s.marshalGetOrphanTxDescResponse(reflect.ValueOf(orphanResp), orphan); err != nil {
				return err
			}
			orphanDescs = append(orphanDescs, orphanResp)
		}
		resp.FieldByName("TxDescs").Set(reflect.ValueOf(txDescsV1))
		resp.FieldByName("OrphanDescs").Set(reflect.ValueOf(orphanDescs))
	}

	return nil
}

func (s *Server) marshalGetTxDescResponse(resp reflect.Value, txD *blockchain.TxDesc, verbose int) error {
	resp = reflect.Indirect(resp)
	startingPriority, _ := txD.StartingPriority()
	totalInputAge, _ := txD.TotalInputAge()

	// write common parts of GetTxDescV0Response, GetTxDescV1Response
	resp.FieldByName("Txid").SetString(txD.Tx.Hash().String())
	resp.FieldByName("PlainSize").SetUint(uint64(txD.Tx.PlainSize()))
	resp.FieldByName("PacketSize").SetUint(uint64(txD.Tx.PacketSize()))
	resp.FieldByName("Time").SetInt(txD.Added.UnixNano())
	resp.FieldByName("Height").SetUint(txD.Height)
	resp.FieldByName("Fee").SetString(txD.Fee.String())
	resp.FieldByName("StartingPriority").SetFloat(startingPriority)
	resp.FieldByName("TotalInputAge").SetInt(totalInputAge.IntValue())

	// write differential parts
	switch verbose {
	case 1:
		txStore := s.chain.FetchTransactionStore(txD.Tx, false)
		priority, err := txD.CurrentPriority(txStore, s.chain.BestBlockHeight())
		if err != nil {
			logging.CPrint(logging.ERROR, "error on api get txD current priority", logging.LogFormat{"err": err, "txid": txD.Tx.Hash()})
			return err
		}
		txIns := txD.Tx.MsgTx().TxIn
		depends := make([]*pb.TxOutPoint, 0, len(txIns))
		for _, txIn := range txIns {
			depends = append(depends, &pb.TxOutPoint{Txid: txIn.PreviousOutPoint.Hash.String(), Index: txIn.PreviousOutPoint.Index})
		}
		resp.FieldByName("CurrentPriority").SetFloat(priority)
		resp.FieldByName("Depends").Set(reflect.ValueOf(depends))
	}

	return nil
}

func (s *Server) marshalGetOrphanTxDescResponse(resp reflect.Value, orphan *massutil.Tx) error {
	resp = reflect.Indirect(resp)
	resp.FieldByName("Txid").SetString(orphan.Hash().String())
	resp.FieldByName("PlainSize").SetUint(uint64(orphan.PlainSize()))
	resp.FieldByName("PacketSize").SetUint(uint64(orphan.PacketSize()))
	txIns := orphan.MsgTx().TxIn
	depends := make([]*pb.TxOutPoint, 0, len(txIns))
	for _, txIn := range txIns {
		depends = append(depends, &pb.TxOutPoint{Txid: txIn.PreviousOutPoint.Hash.String(), Index: txIn.PreviousOutPoint.Index})
	}
	resp.FieldByName("Depends").Set(reflect.ValueOf(depends))

	return nil
}

func (s *Server) showCoinbaseInputDetails(mtx *wire.MsgTx) ([]*pb.Vin, int64, error) {
	vinList := make([]*pb.Vin, len(mtx.TxIn))
	var bindingValue int64
	if blockchain.IsCoinBaseTx(mtx) {
		txIn := mtx.TxIn[0]
		vinTemp := &pb.Vin{
			Txid:     txIn.PreviousOutPoint.Hash.String(),
			Sequence: txIn.Sequence,
			Witness:  txWitnessToHex(txIn.Witness),
		}
		vinList[0] = vinTemp

		for i, txIn := range mtx.TxIn[1:] {
			vinTemp := &pb.Vin{
				Txid:     txIn.PreviousOutPoint.Hash.String(),
				Vout:     txIn.PreviousOutPoint.Index,
				Sequence: txIn.Sequence,
			}

			originTx, err := s.chain.GetTransaction(&txIn.PreviousOutPoint.Hash)
			if err != nil {
				return nil, 0, err
			}
			bindingValue += originTx.TxOut[txIn.PreviousOutPoint.Index].Value
			vinList[i+1] = vinTemp
		}

		return vinList, bindingValue, nil
	} else {
		return nil, 0, errors.New("")
	}
}

func (s *Server) showCoinbaseOutputDetails(mtx *wire.MsgTx, chainParams *config.Params, height uint64, bindingValue int64, bitlength int) ([]*pb.CoinbaseVout, int64, error) {
	voutList := make([]*pb.CoinbaseVout, 0, len(mtx.TxOut))

	g, err := massutil.NewAmountFromInt(bindingValue)
	if err != nil {
		return nil, -1, err
	}

	coinbasePayload := blockchain.NewCoinbasePayload()
	err = coinbasePayload.SetBytes(mtx.Payload)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to deserialize coinbase payload", logging.LogFormat{"error": err})
		return nil, -1, err
	}
	numStaking := coinbasePayload.NumStakingReward()

	baseMiner, superNode, err := blockchain.CalcBlockSubsidy(height, chainParams, g, int(numStaking), bitlength)
	if err != nil {
		return nil, -1, err
	}

	blockSubsidy, err := baseMiner.Add(superNode)
	if err != nil {
		return nil, -1, err
	}

	var (
		outputType string
		totalOut   = massutil.ZeroAmount()
	)
	for i, v := range mtx.TxOut {
		// The disassembled string will contain [error] inline if the
		// script doesn't fully parse, so ignore the error here.
		disbuf, err := txscript.DisasmString(v.PkScript)
		if err != nil {
			logging.CPrint(logging.WARN, "decode pkscript to asm exists err", logging.LogFormat{"err": err})
			st := status.New(ErrAPIDisasmScript, ErrCode[ErrAPIDisasmScript])
			return nil, -1, st.Err()
		}

		// Ignore the error here since an error means the script
		// couldn't parse and there is no additional information about
		// it anyways.
		scriptClass, addrs, _, reqSigs, err := txscript.ExtractPkScriptAddrs(
			v.PkScript, chainParams)
		if err != nil {
			st := status.New(ErrAPIExtractPKScript, ErrCode[ErrAPIExtractPKScript])
			return nil, -1, st.Err()
		}

		encodedAddrs := make([]string, len(addrs))
		for j, addr := range addrs {
			encodedAddrs[j] = addr.EncodeAddress()
		}

		if uint32(i) < numStaking {
			outputType = "staking reward"
		} else {
			outputType = "miner"
		}

		valueStr, err := AmountToString(v.Value)
		if err != nil {
			return nil, -1, err
		}

		totalOut, err = totalOut.AddInt(v.Value)
		if err != nil {
			return nil, -1, err
		}

		vout := &pb.CoinbaseVout{
			N:     uint32(i),
			Value: valueStr,
			ScriptPublicKey: &pb.ScriptPubKeyResult{
				Asm:       disbuf,
				Hex:       hex.EncodeToString(v.PkScript),
				ReqSigs:   uint32(reqSigs),
				Type:      scriptClass.String(),
				Addresses: encodedAddrs,
			},
			Type: outputType,
		}

		voutList = append(voutList, vout)
	}
	totalFees, err := totalOut.Sub(blockSubsidy)
	if err != nil {
		return nil, -1, err
	}

	return voutList, totalFees.IntValue(), nil
}

func (s *Server) createTxRawResult(chainParams *config.Params, mtx *wire.MsgTx, txHash string,
	blkHeader *wire.BlockHeader, blkHash string, blkHeight uint64, chainHeight uint64) (*pb.TxRawResult, error) {

	voutList, totalOutValue, err := createVoutList(mtx, chainParams, nil)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to create vout list", logging.LogFormat{"error": err})
		return nil, err
	}
	to := make([]*pb.ToAddressForTx, 0)
	for _, voutR := range voutList {
		to = append(to, &pb.ToAddressForTx{
			Address: voutR.ScriptPublicKey.Addresses,
			Value:   voutR.Value,
		})
	}

	if blockchain.IsCoinBaseTx(mtx) {
		totalOutValue = 0
	}

	if mtx.Payload == nil {
		mtx.Payload = make([]byte, 0)
	}

	vins, fromAddrs, inputs, totalInValue, err := s.createVinList(mtx, chainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to create vin list", logging.LogFormat{"error": err})
		return nil, err
	}

	txid, err := wire.NewHashFromStr(txHash)
	if err != nil {
		st := status.New(ErrAPIShaHashFromStr, ErrCode[ErrAPIShaHashFromStr])
		return nil, st.Err()
	}
	code, err := s.getTxStatus(txid)
	if err != nil {
		return nil, err
	}
	txType, err := s.getTxType(mtx)
	if err != nil {
		return nil, err
	}

	fee, err := AmountToString(totalInValue - totalOutValue)
	if err != nil {
		return nil, err
	}

	txReply := &pb.TxRawResult{
		Txid:        txHash,
		Version:     mtx.Version,
		LockTime:    mtx.LockTime,
		Vin:         vins,
		Vout:        voutList,
		FromAddress: fromAddrs,
		To:          to,
		Inputs:      inputs,
		Payload:     hex.EncodeToString(mtx.Payload),
		Size:        uint32(mtx.PlainSize()),
		Fee:         fee,
		Status:      code,
		Type:        txType,
	}

	if blkHeader != nil {
		// This is not a typo, they are identical in massd as well.
		txReply.Block = &pb.BlockInfoForTx{Height: uint64(blkHeight), BlockHash: blkHash, Timestamp: blkHeader.Timestamp.Unix()}
		txReply.Confirmations = uint64(1 + chainHeight - blkHeight)
	}

	return txReply, nil
}

// Tx type codes are shown below:
//  -----------------------------------------------------
// |  Tx Type  | Staking | Binding | Ordinary | Coinbase |
// |-----------------------------------------------------|
// | Type Code |    1    |    2    |     3    |     4    |
//   ----------------------------------------------------
func (s *Server) getTxType(tx *wire.MsgTx) (int32, error) {
	if blockchain.IsCoinBaseTx(tx) {
		return 4, nil
	}
	for _, txOut := range tx.TxOut {
		if txscript.IsPayToStakingScriptHash(txOut.PkScript) {
			return 1, nil
		}
		if txscript.IsPayToBindingScriptHash(txOut.PkScript) {
			return 2, nil
		}
	}
	for _, txIn := range tx.TxIn {
		hash := txIn.PreviousOutPoint.Hash
		index := txIn.PreviousOutPoint.Index
		tx, err := s.chain.GetTransaction(&hash)
		if err != nil {
			logging.CPrint(logging.ERROR, "No information available about transaction in db", logging.LogFormat{"err": err, "txid": hash.String()})
			st := status.New(ErrAPINoTxInfo, ErrCode[ErrAPINoTxInfo])
			return -1, st.Err()
		}
		if txscript.IsPayToStakingScriptHash(tx.TxOut[index].PkScript) {
			return 1, nil
		}
		if txscript.IsPayToBindingScriptHash(tx.TxOut[index].PkScript) {
			return 2, nil
		}
	}
	return 3, nil
}

func createVoutList(mtx *wire.MsgTx, chainParams *config.Params, filterAddrMap map[string]struct{}) ([]*pb.Vout, int64, error) {
	voutList := make([]*pb.Vout, 0, len(mtx.TxOut))
	var totalOutValue int64
	for i, v := range mtx.TxOut {
		// reset filter flag for each.
		passesFilter := len(filterAddrMap) == 0

		// The disassembled string will contain [error] inline if the
		// script doesn't fully parse, so ignore the error here.
		disbuf, err := txscript.DisasmString(v.PkScript)
		if err != nil {
			logging.CPrint(logging.WARN, "decode pkscript to asm exists err", logging.LogFormat{"err": err})
			st := status.New(ErrAPIDisasmScript, ErrCode[ErrAPIDisasmScript])
			return nil, -1, st.Err()
		}

		// Ignore the error here since an error means the script
		// couldn't parse and there is no additional information about
		// it anyways.
		scriptClass, addrs, _, reqSigs, err := txscript.ExtractPkScriptAddrs(
			v.PkScript, chainParams)
		if err != nil {
			st := status.New(ErrAPIExtractPKScript, ErrCode[ErrAPIExtractPKScript])
			return nil, -1, st.Err()
		}
		var frozenPeriod uint64
		var rewardAddress string
		if scriptClass == txscript.StakingScriptHashTy {
			_, pops := txscript.GetScriptInfo(v.PkScript)
			frozenPeriod, _, err = txscript.GetParsedOpcode(pops, scriptClass)
			if err != nil {
				return nil, -1, err
			}

			normalAddress, err := massutil.NewAddressWitnessScriptHash(addrs[0].ScriptAddress(), chainParams)
			if err != nil {
				return nil, -1, err
			}
			rewardAddress = normalAddress.String()
		}

		encodedAddrs := make([]string, len(addrs))
		for j, addr := range addrs {
			encodedAddrs[j] = addr.EncodeAddress()

			if len(filterAddrMap) > 0 {
				if _, exists := filterAddrMap[encodedAddrs[j]]; exists {
					passesFilter = true
				}
			}
		}

		if !passesFilter {
			continue
		}

		totalOutValue += v.Value

		valueStr, err := AmountToString(v.Value)
		if err != nil {
			logging.CPrint(logging.ERROR, "")
			return nil, -1, err
		}

		vout := &pb.Vout{
			N:     uint32(i),
			Value: valueStr,
			ScriptPublicKey: &pb.ScriptPubKeyResult{
				Asm:           disbuf,
				Hex:           hex.EncodeToString(v.PkScript),
				ReqSigs:       uint32(reqSigs),
				Type:          scriptClass.String(),
				FrozenPeriod:  uint32(frozenPeriod),
				RewardAddress: rewardAddress,
				Addresses:     encodedAddrs,
			},
		}

		voutList = append(voutList, vout)
	}

	return voutList, totalOutValue, nil
}

func (s *Server) createVinList(mtx *wire.MsgTx, chainParams *config.Params) ([]*pb.Vin, []string, []*pb.InputsInTx, int64, error) {
	// Coinbase transactions only have a single txin by definition.
	vinList := make([]*pb.Vin, len(mtx.TxIn))
	addrs := make([]string, 0)
	inputs := make([]*pb.InputsInTx, 0)
	var totalInValue int64
	if blockchain.IsCoinBaseTx(mtx) {
		txIn := mtx.TxIn[0]
		vinTemp := &pb.Vin{
			Txid:     txIn.PreviousOutPoint.Hash.String(),
			Sequence: txIn.Sequence,
			Witness:  txWitnessToHex(txIn.Witness),
		}
		vinList[0] = vinTemp

		for i, txIn := range mtx.TxIn[1:] {
			vinTemp := &pb.Vin{
				Txid:     txIn.PreviousOutPoint.Hash.String(),
				Vout:     txIn.PreviousOutPoint.Index,
				Sequence: txIn.Sequence,
			}
			vinList[i+1] = vinTemp
		}

		return vinList, addrs, inputs, totalInValue, nil
	}

	for i, txIn := range mtx.TxIn {
		vinEntry := &pb.Vin{
			Txid:     txIn.PreviousOutPoint.Hash.String(),
			Vout:     txIn.PreviousOutPoint.Index,
			Sequence: txIn.Sequence,
			Witness:  txWitnessToHex(txIn.Witness),
		}
		vinList[i] = vinEntry

		addrs, inValue, err := s.getTxInAddr(&txIn.PreviousOutPoint.Hash, txIn.PreviousOutPoint.Index, chainParams)
		if err != nil {
			logging.CPrint(logging.ERROR, "No information available about transaction in db", logging.LogFormat{"err": err.Error(), "txid": txIn.PreviousOutPoint.Hash.String()})
			st := status.New(ErrAPINoTxInfo, ErrCode[ErrAPINoTxInfo])
			return nil, nil, nil, -1, st.Err()
		}
		totalInValue = totalInValue + inValue
		val, err := AmountToString(inValue)
		if err != nil {
			logging.CPrint(logging.ERROR, "")
			return nil, nil, nil, -1, err
		}
		inputs = append(inputs, &pb.InputsInTx{Txid: txIn.PreviousOutPoint.Hash.String(), Index: txIn.PreviousOutPoint.Index, Address: addrs, Value: val})
	}

	return vinList, addrs, inputs, totalInValue, nil
}

func (s *Server) getTxInAddr(txid *wire.Hash, index uint32, chainParams *config.Params) ([]string, int64, error) {
	addrStrs := make([]string, 0)
	var inValue int64
	tx, err := s.txMemPool.FetchTransaction(txid)
	var inmtx *wire.MsgTx
	if err != nil {
		txReply, err := s.chain.GetTransactionInDB(txid)
		if err != nil || len(txReply) == 0 {
			logging.CPrint(logging.ERROR, "No information available about transaction in db", logging.LogFormat{"err": err, "txid": txid.String()})
			st := status.New(ErrAPINoTxInfo, ErrCode[ErrAPINoTxInfo])
			return addrStrs, inValue, st.Err()
		}
		lastTx := txReply[len(txReply)-1]
		inmtx = lastTx.Tx
	} else {
		inmtx = tx.MsgTx()
	}

	_, addrs, _, _, err := txscript.ExtractPkScriptAddrs(inmtx.TxOut[int(index)].PkScript, chainParams)

	for _, addr := range addrs {
		addrStrs = append(addrStrs, addr.EncodeAddress())
	}
	inValue = inmtx.TxOut[int(index)].Value
	return addrStrs, inValue, nil
}

func (s *Server) getTxStatus(txHash *wire.Hash) (code int32, err error) {
	txList, err := s.chain.GetTransactionInDB(txHash)
	if err != nil || len(txList) == 0 {
		_, err := s.txMemPool.FetchTransaction(txHash)
		if err != nil {
			code = -1
			//stats = "failed"
			return code, nil
		} else {
			code = 2
			//stats = "packing"
			return code, nil
		}
	}

	lastTx := txList[len(txList)-1]
	txHeight := lastTx.Height
	bestHeight := s.chain.BestBlockHeight()
	confirmations := 1 + bestHeight - txHeight
	if confirmations < 0 {
		code = -1
		//stats = "failed"
		return code, nil
	}
	if blockchain.IsCoinBaseTx(lastTx.Tx) {
		if confirmations >= consensus.CoinbaseMaturity {
			code = 4
			//stats = "succeed"
			return code, nil
		} else {
			code = 3
			//stats = "confirming"
			return code, nil
		}
	}
	if confirmations >= consensus.TransactionMaturity {
		code = 4
		//stats = "succeed"
		return code, nil
	} else {
		code = 3
		//stats = "confirming"
		return code, nil
	}
}

func txWitnessToHex(witness wire.TxWitness) []string {
	// Ensure nil is returned when there are no entries versus an empty
	// slice so it can properly be omitted as necessary.
	if len(witness) == 0 {
		return nil
	}

	result := make([]string, 0, len(witness))
	for _, wit := range witness {
		result = append(result, hex.EncodeToString(wit))
	}

	return result
}
