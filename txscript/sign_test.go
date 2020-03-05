// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
package txscript_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

type addressDetail struct {
	PubKey *btcec.PublicKey
}

type wallet struct {
	WitnessMap      map[string][]byte            // witness address -> redeemscript
	PubKeyToPrivKey map[string]*btcec.PrivateKey // map key is string(pub key bytes)
}

func iniWallet() *wallet {
	return &wallet{
		WitnessMap:      make(map[string][]byte),
		PubKeyToPrivKey: make(map[string]*btcec.PrivateKey),
	}
}

func (w *wallet) getPrivKey(pk *btcec.PublicKey) *btcec.PrivateKey {
	return w.PubKeyToPrivKey[string(pk.SerializeCompressed())]
}

func newWitnessScriptAddress(pubkeys []*btcec.PublicKey, nrequired int,
	addressClass uint16, net *config.Params) ([]byte, massutil.Address, error) {

	var addressPubKeyStructs []*massutil.AddressPubKey
	for i := 0; i < len(pubkeys); i++ {
		pubKeySerial := pubkeys[i].SerializeCompressed()
		addressPubKeyStruct, err := massutil.NewAddressPubKey(pubKeySerial, net)
		if err != nil {
			logging.CPrint(logging.ERROR, "create addressPubKey failed",
				logging.LogFormat{
					"err":       err,
					"version":   addressClass,
					"nrequired": nrequired,
				})
			return nil, nil, err
		}
		addressPubKeyStructs = append(addressPubKeyStructs, addressPubKeyStruct)
	}

	redeemScript, err := txscript.MultiSigScript(addressPubKeyStructs, nrequired)
	if err != nil {
		logging.CPrint(logging.ERROR, "create redeemScript failed",
			logging.LogFormat{
				"err":       err,
				"version":   addressClass,
				"nrequired": nrequired,
			})
		return nil, nil, err
	}
	var witAddress massutil.Address
	// scriptHash is witnessProgram
	scriptHash := sha256.Sum256(redeemScript)
	switch addressClass {
	case massutil.AddressClassWitnessStaking:
		witAddress, err = massutil.NewAddressStakingScriptHash(scriptHash[:], net)
	case massutil.AddressClassWitnessV0:
		witAddress, err = massutil.NewAddressWitnessScriptHash(scriptHash[:], net)
	default:
		return nil, nil, errors.New("invalid version")
	}

	if err != nil {
		logging.CPrint(logging.ERROR, "create witness address failed",
			logging.LogFormat{
				"err":       err,
				"version":   addressClass,
				"nrequired": nrequired,
			})
		return nil, nil, err
	}

	return redeemScript, witAddress, nil
}

func generateAddress(w *wallet, n int) ([]*addressDetail, error) {
	var result []*addressDetail
	for n > 0 {
		privk, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			return nil, err
		}
		w.PubKeyToPrivKey[string(privk.PubKey().SerializeCompressed())] = privk
		result = append(result, &addressDetail{
			PubKey: privk.PubKey(),
		})
		n--
	}
	return result, nil
}

func createP2wshScript(w *wallet, nRequire, nTotal int) ([]byte, error) {

	addrs, err := generateAddress(w, nTotal)
	if err != nil {
		return nil, err
	}

	var pubKeys []*btcec.PublicKey
	for _, addr := range addrs {
		pubKeys = append(pubKeys, addr.PubKey)
	}
	redeemScript, witnessAddress, err := newWitnessScriptAddress(pubKeys, nRequire, massutil.AddressClassWitnessV0, &config.ChainParams)
	if err != nil {
		return nil, err
	}
	w.WitnessMap[witnessAddress.EncodeAddress()] = redeemScript
	pkScript, err := txscript.PayToAddrScript(witnessAddress)
	if err != nil {
		return nil, err
	}
	return pkScript, nil
}

func createP2wshStakingScript(w *wallet, nRequire, nTotal int, frozenPeriod uint64) ([]byte, error) {

	addrs, err := generateAddress(w, nTotal)
	if err != nil {
		return nil, err
	}

	var pubKeys []*btcec.PublicKey
	for _, addr := range addrs {
		pubKeys = append(pubKeys, addr.PubKey)
	}
	redeemScript, witnessAddress, err := newWitnessScriptAddress(pubKeys,
		nRequire, massutil.AddressClassWitnessStaking, &config.ChainParams)
	if err != nil {
		return nil, err
	}
	w.WitnessMap[witnessAddress.EncodeAddress()] = redeemScript
	pkScript, err := txscript.PayToStakingAddrScript(witnessAddress, frozenPeriod)
	if err != nil {
		return nil, err
	}
	return pkScript, nil
}

func createP2wshBindingScript(w *wallet, nRequire, nTotal int, pocPkHash []byte) ([]byte, error) {

	addrs, err := generateAddress(w, nTotal)
	if err != nil {
		return nil, err
	}

	var pubKeys []*btcec.PublicKey
	for _, addr := range addrs {
		pubKeys = append(pubKeys, addr.PubKey)
	}
	redeemScript, witnessAddress, err := newWitnessScriptAddress(pubKeys,
		nRequire, massutil.AddressClassWitnessV0, &config.ChainParams)
	if err != nil {
		return nil, err
	}
	w.WitnessMap[witnessAddress.EncodeAddress()] = redeemScript
	pkScript, err := txscript.PayToBindingScriptHashScript(witnessAddress.ScriptAddress(), pocPkHash)
	if err != nil {
		return nil, err
	}
	return pkScript, nil
}

func checkScripts(msg string, tx *wire.MsgTx, idx int, witness wire.TxWitness, pkScript []byte, value int64) error {
	tx.TxIn[idx].Witness = witness
	vm, err := txscript.NewEngine(pkScript, tx, idx,
		txscript.StandardVerifyFlags, nil, nil, value)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = vm.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

//sign and execute the script
func signAndCheck(msg string, tx *wire.MsgTx, idx int, pkScript []byte,
	hashType txscript.SigHashType, kdb txscript.GetSignDB, sdb txscript.ScriptDB,
	previousScript []byte, value int64) error {
	hashCache := txscript.NewTxSigHashes(tx)

	witness, err := txscript.SignTxOutputWit(&config.ChainParams, tx,
		idx, value, pkScript, hashCache, hashType, kdb, sdb)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}
	tx.TxIn[idx].Witness = witness

	return checkScripts(msg, tx, idx, witness, pkScript, value)
}

func TestAnyoneCanPay(t *testing.T) {
	wit0, _ := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddOp(txscript.OP_DROP).Script()
	wit1, _ := txscript.NewScriptBuilder().AddOp(txscript.OP_TRUE).Script()
	scriptHash := sha256.Sum256(wit1)
	pkscript, err := txscript.PayToWitnessScriptHashScript(scriptHash[:])
	if err != nil {
		t.FailNow()
	}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 0,
				},
				Witness:  wire.TxWitness{wit0, wit1},
				Sequence: wire.MaxTxInSequenceNum,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	hashCache := txscript.NewTxSigHashes(tx)
	vm, err := txscript.NewEngine(pkscript, tx, 0, txscript.StandardVerifyFlags, nil, hashCache, 3000000000)
	assert.Nil(t, err)
	if err == nil {
		err = vm.Execute()
		assert.Nil(t, err)
	}
}

//test the signTxOutputWit function
func TestSignTxOutputWit(t *testing.T) {
	// t.Parallel()
	w := iniWallet()
	pkScript, err := createP2wshScript(w, 1, 1)
	// find redeem script
	getScript := txscript.ScriptClosure(func(addr massutil.Address) ([]byte, error) {
		// If keys were provided then we can only use the
		// redeem scripts provided with our inputs, too.

		script, _ := w.WitnessMap[addr.EncodeAddress()]

		return script, nil
	})
	// find private key
	getSign := txscript.SignClosure(func(pub *btcec.PublicKey, hash []byte) (*btcec.Signature, error) {
		if len(hash) != 32 {
			return nil, errors.New("invalid data to sign")
		}
		privK := w.getPrivKey(pub)
		if privK == nil {
			return nil, errors.New("private key not found")
		}
		return privK.Sign(hash)
	})
	if err != nil {
		t.Errorf("create wallet error : %v", err)
	}
	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []txscript.SigHashType{
		txscript.SigHashAll,
		txscript.SigHashNone,
		txscript.SigHashSingle,
		txscript.SigHashAll | txscript.SigHashAnyOneCanPay,
		txscript.SigHashNone | txscript.SigHashAnyOneCanPay,
		txscript.SigHashSingle | txscript.SigHashAnyOneCanPay,
	}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 0,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 1,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 2,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	// p2wsh
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			//output value is 0
			var value = 3000000000
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				getSign, getScript, nil, int64(value)); err != nil {
				t.Error(err)
				break
			}
		}
		bs, err := tx.Bytes(wire.Packet)
		assert.Nil(t, err)
		fmt.Println(pkScript)
		fmt.Println(hex.EncodeToString(bs))
	}
}

//test the signTxOutputWit function
func TestSignStakingTxOutputWit(t *testing.T) {
	// t.Parallel()
	w := iniWallet()
	pkScript, err := createP2wshStakingScript(w, 1, 1, consensus.MinFrozenPeriod)
	// find redeem script
	getScript := txscript.ScriptClosure(func(addr massutil.Address) ([]byte, error) {
		script, _ := w.WitnessMap[addr.EncodeAddress()]
		return script, nil
	})
	// find private key
	getSign := txscript.SignClosure(func(pub *btcec.PublicKey, hash []byte) (*btcec.Signature, error) {
		if len(hash) != 32 {
			return nil, errors.New("invalid data to sign")
		}
		privK := w.getPrivKey(pub)
		if privK == nil {
			return nil, errors.New("private key not found")
		}
		return privK.Sign(hash)
	})
	if err != nil {
		t.Errorf("create wallet error : %v", err)
	}
	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []txscript.SigHashType{
		txscript.SigHashAll,
		txscript.SigHashNone,
		txscript.SigHashSingle,
		txscript.SigHashAll | txscript.SigHashAnyOneCanPay,
		txscript.SigHashNone | txscript.SigHashAnyOneCanPay,
		txscript.SigHashSingle | txscript.SigHashAnyOneCanPay,
	}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 0,
				},
				Sequence: consensus.MinFrozenPeriod + 1,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 1,
				},
				Sequence: consensus.MinFrozenPeriod + 1,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 2,
				},
				Sequence: consensus.MinFrozenPeriod + 1,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	// p2wsh
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			//output value is 0
			var value = 0
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				getSign, getScript, nil, int64(value)); err != nil {
				t.Error(err)
				break
			}
		}
	}
}

//test the signTxOutputWit function
func TestSignBindingTxOutputWit(t *testing.T) {
	// t.Parallel()
	w := iniWallet()

	pocPkHash := []byte{
		12, 13, 14, 15, 116,
		12, 13, 14, 15, 116,
		12, 13, 14, 15, 116,
		12, 13, 14, 15, 116,
	}

	pkScript, err := createP2wshBindingScript(w, 1, 1, pocPkHash)
	// find redeem script
	getScript := txscript.ScriptClosure(func(addr massutil.Address) ([]byte, error) {
		script, _ := w.WitnessMap[addr.EncodeAddress()]
		return script, nil
	})
	// find private key
	getSign := txscript.SignClosure(func(pub *btcec.PublicKey, hash []byte) (*btcec.Signature, error) {
		if len(hash) != 32 {
			return nil, errors.New("invalid data to sign")
		}
		privK := w.getPrivKey(pub)
		if privK == nil {
			return nil, errors.New("private key not found")
		}
		return privK.Sign(hash)
	})
	if err != nil {
		t.Errorf("create wallet error : %v", err)
	}
	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []txscript.SigHashType{
		txscript.SigHashAll,
		txscript.SigHashNone,
		txscript.SigHashSingle,
		txscript.SigHashAll | txscript.SigHashAnyOneCanPay,
		txscript.SigHashNone | txscript.SigHashAnyOneCanPay,
		txscript.SigHashSingle | txscript.SigHashAnyOneCanPay,
	}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 0,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 1,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: 2,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	// p2wsh
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			//output value is 0
			var value = 0
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				getSign, getScript, nil, int64(value)); err != nil {
				t.Error(err)
				break
			}
		}
	}
}
