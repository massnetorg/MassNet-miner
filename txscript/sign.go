// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

type GetSignDB interface {
	GetSign(*btcec.PublicKey, []byte) (*btcec.Signature, error)
}

type SignClosure func(*btcec.PublicKey, []byte) (*btcec.Signature, error)

func (kc SignClosure) GetSign(pubkey *btcec.PublicKey, hash []byte) (*btcec.Signature,
	error) {
	return kc(pubkey, hash)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(massutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(massutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address massutil.Address) ([]byte, error) {
	return sc(address)
}

// RawTxInWitnessSignature returns the serialized ECDA signature for the input
// idx of the given transaction, with the hashType appended to it. This
// function is identical to RawTxInSignature, however the signature generated
// signs a new sighash digest defined in BIP0143.
func RawTxInWitnessSignature(tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	amt int64, subScript []byte, hashType SigHashType,
	pubkey *btcec.PublicKey, kdb GetSignDB) ([]byte, error) {
	parsedScript, err := parseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}

	hash, err := calcWitnessSignatureHash(parsedScript, sigHashes, hashType, tx,
		idx, amt)
	if err != nil {
		return nil, err
	}

	//signature, err := key.Sign(hash)
	signature, err := kdb.GetSign(pubkey, hash)
	if err != nil {
		return nil, err
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

func signWitMultiSig(tx *wire.MsgTx, idx int, value int64, subScript []byte, sigHashes *TxSigHashes, hashType SigHashType,
	pubkey []*btcec.PublicKey, nRequired int, kdb GetSignDB) ([]byte, bool, error) {
	// We start with a single OP_FALSE to work around the (now standard)
	// but in the reference implementation that causes a spurious pop at
	// the end of OP_CHECKMULTISIG. (Already fixed in mass script vm)
	builder := NewScriptBuilder()
	//.AddOp(OP_FALSE)
	//sigHashes := NewTxSigHashes(tx)
	signed := 0
	for _, pk := range pubkey {
		sig, err := RawTxInWitnessSignature(tx, sigHashes, idx, value, subScript,
			hashType, pk, kdb)

		if err != nil {
			if len(pubkey) == 1 {
				return nil, false, err
			}
			continue
		}

		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}
	}
	script, err := builder.Script()
	if err != nil && len(pubkey) == 1 {
		return nil, false, err
	}
	return script, signed == nRequired, nil
}

func signwit(chainParams *config.Params, tx *wire.MsgTx, idx int, value int64,
	subScript []byte, sigHashes *TxSigHashes, hashType SigHashType, kdb GetSignDB, sdb ScriptDB) ([]byte,
	ScriptClass, []massutil.Address, int, error) {
	class, addresses, pubkey, nrequired, err := ExtractPkScriptAddrs(subScript,
		chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	switch class {

	case WitnessV0ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}
		return script, class, addresses, nrequired, nil
	case StakingScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}
		return script, class, addresses, nrequired, nil
	case BindingScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}
		return script, class, addresses, nrequired, nil
	case MultiSigTy:
		script, _, err := signWitMultiSig(tx, idx, value, subScript, sigHashes, hashType,
			pubkey, nrequired, kdb)
		return script, class, addresses, nrequired, err
	case NullDataTy:
		return nil, class, nil, 0,
			errors.New("can't sign NULLDATA transactions")

	default:
		return nil, class, nil, 0,
			errors.New("can't sign unknown transactions")
	}

}

func SignTxOutputWit(chainParams *config.Params, tx *wire.MsgTx, idx int, value int64, pkScript []byte, sigHashes *TxSigHashes, hashType SigHashType, kdb GetSignDB, sdb ScriptDB) (wire.TxWitness, error) {
	sigScript, _, _, _, err := signwit(chainParams, tx,
		idx, value, pkScript, sigHashes, hashType, kdb, sdb)
	if err != nil {
		return nil, err
	}
	// TODO keep the sub addressed and pass down to merge.
	realSigScript, _, _, _, err := signwit(chainParams, tx, idx, value,
		sigScript, sigHashes, hashType, kdb, sdb)

	if err != nil {
		return nil, err
	}

	return wire.TxWitness{realSigScript, sigScript}, nil

}
