package blockchain

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

// TestCalcMinRequiredTxRelayFee tests the CalcMinRequiredTxRelayFee API.
func TestCalcMinRequiredTxRelayFee(t *testing.T) {
	tests := []struct {
		name     string // test description.
		size     int64  // Transaction size in bytes.
		relayFee uint64 // minimum relay transaction fee.
		want     uint64 // Expected fee.
	}{
		{
			// Ensure combination of size and fee that are less than 1000
			// produce a non-zero fee.
			"250 bytes with relay fee of 3",
			250,
			3,
			3,
		},
		{
			"100 bytes with default minimum relay fee",
			100,
			consensus.MinRelayTxFee,
			1000,
		},
		{
			"max standard tx size with default minimum relay fee",
			maxStandardTxSize,
			consensus.MinRelayTxFee,
			1000000,
		},
		{
			"max standard tx size with max maxwell relay fee",
			maxStandardTxSize,
			massutil.MaxAmount().UintValue(),
			massutil.MaxAmount().UintValue(),
		},
		{
			"1500 bytes with 5000 relay fee",
			1500,
			5000,
			7500,
		},
		{
			"1500 bytes with 3000 relay fee",
			1500,
			3000,
			4500,
		},
		{
			"782 bytes with 5000 relay fee",
			782,
			5000,
			3910,
		},
		{
			"782 bytes with 3000 relay fee",
			782,
			3000,
			2346,
		},
		{
			"782 bytes with 2550 relay fee",
			782,
			2550,
			1994,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minFee, err := massutil.NewAmountFromUint(test.relayFee)
			assert.Nil(t, err)
			got, err := CalcMinRequiredTxRelayFee(test.size, minFee)
			assert.Nil(t, err)
			if got.UintValue() != test.want {
				t.Errorf("index：%d, TestCalcMinRequiredTxRelayFee test '%s' "+
					"failed: got %v want %v", i, test.name, got,
					test.want)
			}
		})
	}
}

// TestCheckPkScriptStandard tests the checkPkScriptStandard API.
func TestCheckPkScriptStandard(t *testing.T) {
	var pubKeys [][]byte
	var witnessV0Addr massutil.Address
	for i := 0; i < 4; i++ {
		pk, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard NewPrivateKey failed: %v",
				err)
			return
		}
		pubKeys = append(pubKeys, pk.PubKey().SerializeCompressed())
		if witnessV0Addr == nil {
			_, witnessV0Addr, err = newWitnessScriptAddress([]*btcec.PublicKey{pk.PubKey()},
				1, massutil.AddressClassWitnessV0, &config.ChainParams)
			assert.Nil(t, err)
		}
	}
	pubkey := []byte{75}
	tests := []struct {
		name       string // test description.
		script     *txscript.ScriptBuilder
		isStandard bool
	}{
		{
			"multisig",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"nonstd1",
			txscript.NewScriptBuilder().
				AddData(pubkey).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"nonstd2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]),
			false,
		},
		{
			"nonstd3",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(append(witnessV0Addr.ScriptAddress(), byte(0))),
			false,
		},
		{
			"nulldata",
			txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN),
			true,
		},
		{
			"nulldata with 0 bytes",
			txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).AddData([]byte{}),
			true,
		},
		{
			"nulldata with 80 bytes",
			txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
				AddData([]byte("12345678901234567890123456789012345678901234567890123456789012345678901234567890")),
			true,
		},
		{
			"too many(>80) nulldata bytes",
			txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
				AddData([]byte("123456789012345678901234567890123456789012345678901234567890123456789012345678901")),
			false,
		},
		{
			"witness v0",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()),
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := test.script.Script()
			if err != nil {
				t.Fatalf("TestCheckPkScriptStandard test '%s' "+
					"failed: %v", test.name, err)
				return
			}
			txOut := &wire.TxOut{
				PkScript: script,
				Value:    1000,
			}
			cache := make(map[txscript.ScriptClass]bool)
			_, err = checkPkScriptStandard(txOut, nil, cache, nil)
			if (test.isStandard && err != nil) ||
				(!test.isStandard && err == nil) {
				t.Fatalf("TestCheckPkScriptStandard test '%s' failed",
					test.name)
				return
			}
		})
	}
}

// TestCheckPkScriptStandard2 tests the witness staking script.
func TestCheckPkScriptStandard2(t *testing.T) {
	var witnessV0Addr massutil.Address
	pk, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatalf("TestCheckPkScriptStandard2 NewPrivateKey failed: %v",
			err)
		return
	}
	_, witnessV0Addr, err = newWitnessScriptAddress([]*btcec.PublicKey{pk.PubKey()},
		1, massutil.AddressClassWitnessV0, &config.ChainParams)
	assert.Nil(t, err)

	tests := []struct {
		name         string // test description.
		script       *txscript.ScriptBuilder
		stakingValue uint64
		isStandard   bool
		err          error
	}{
		{
			"MinFrozenPeriod",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{byte(consensus.MinFrozenPeriod), 0, 0, 0, 0, 0, 0, 0}),
			consensus.MinStakingValue,
			true,
			nil,
		},
		{
			"MinFrozenPeriod and value not enough",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{byte(consensus.MinFrozenPeriod), 0, 0, 0, 0, 0, 0, 0}),
			consensus.MinStakingValue - 1,
			false,
			ErrInvalidStakingTxValue,
		},
		{
			"uint32(MinFrozenPeriod)",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{byte(consensus.MinFrozenPeriod), 0, 0, 0}),
			consensus.MinStakingValue,
			false,
			ErrNonStandardType,
		},
		{
			"period less than MinFrozenPeriod",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{byte(consensus.MinFrozenPeriod) - 1, 0, 0, 0, 0, 0, 0, 0}),
			consensus.MinStakingValue,
			false,
			ErrInvalidFrozenPeriod,
		},
		{
			"period greater than SequenceLockTimeMask-1",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{255, 255, 255, 255, 0, 0, 0, 0}),
			consensus.MinStakingValue,
			false,
			ErrInvalidFrozenPeriod,
		},
		{
			"period equal SequenceLockTimeMask-1",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(witnessV0Addr.ScriptAddress()).AddData([]byte{254, 255, 255, 255, 0, 0, 0, 0}),
			consensus.MinStakingValue,
			true,
			nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := test.script.Script()
			if err != nil {
				t.Fatalf("failed: %v", err)
				return
			}
			txOut := &wire.TxOut{
				PkScript: script,
				Value:    int64(test.stakingValue),
			}
			cache := make(map[txscript.ScriptClass]bool)
			_, err = checkPkScriptStandard(txOut, nil, cache, nil)
			assert.Equal(t, err, test.err)
			if (test.isStandard && err != nil) ||
				(!test.isStandard && err == nil) {
				t.Fatalf("failed: %v", err)
				return
			}
		})
	}

}

// TestCheckPkScriptStandard2 tests the witness binding script.
func TestCheckPkScriptStandard3(t *testing.T) {
	var witnessV0Addr massutil.Address
	pk, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatalf("TestCheckPkScriptStandard3 NewPrivateKey failed: %v",
			err)
		return
	}
	_, witnessV0Addr, err = newWitnessScriptAddress([]*btcec.PublicKey{pk.PubKey()},
		1, massutil.AddressClassWitnessV0, &config.ChainParams)
	assert.Nil(t, err)
	fmt.Println(witnessV0Addr.ScriptAddress())

	pk, err = btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	pocpkh := massutil.Hash160(pk.PubKey().SerializeCompressed())
	fmt.Println(pocpkh)

	stdBindingPkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(witnessV0Addr.ScriptAddress()).AddData(pocpkh).Script()
	assert.Nil(t, err)

	stdPkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(witnessV0Addr.ScriptAddress()).Script()
	assert.Nil(t, err)

	nonStdPkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(witnessV0Addr.ScriptAddress()).AddData(pocpkh[1:]).Script()
	assert.Nil(t, err)

	// txCoinbase
	txCoinbase := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: math.MaxUint32,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    200000000,
				PkScript: stdPkScript,
			},
		},
		LockTime: 0,
	}
	// txCoinbaseBinding
	txCoinbaseBinding := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  wire.Hash{},
					Index: math.MaxUint32,
				},
				Sequence: wire.MaxTxInSequenceNum,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    100000000,
				PkScript: stdBindingPkScript,
			},
		},
		LockTime: 0,
	}

	tests := []struct {
		name       string // test description.
		tx         *wire.MsgTx
		txStore    TxStore
		isStandard bool
		err        error
	}{
		{
			"standard coinbase binding",
			txCoinbaseBinding,
			make(TxStore),
			true,
			nil,
		},
		{
			"standard binding",
			&wire.MsgTx{
				TxIn: []*wire.TxIn{
					{
						PreviousOutPoint: wire.OutPoint{
							// hash of txCoinbase
							Hash:  wire.Hash{57, 84, 137, 127, 100, 211, 173, 238, 194, 183, 134, 204, 209, 197, 43, 164, 103, 85, 168, 246, 212, 105, 29, 34, 234, 243, 195, 201, 242, 214, 214, 21},
							Index: 0,
						},
					},
				},
				TxOut: []*wire.TxOut{
					{
						PkScript: stdBindingPkScript,
					},
				},
			},
			TxStore{
				wire.Hash{57, 84, 137, 127, 100, 211, 173, 238, 194, 183, 134, 204, 209, 197, 43, 164, 103, 85, 168, 246, 212, 105, 29, 34, 234, 243, 195, 201, 242, 214, 214, 21}: &TxData{
					Tx: massutil.NewTx(txCoinbase),
				},
			},
			true,
			nil,
		},
		{
			"both input and output are binding",
			&wire.MsgTx{
				TxIn: []*wire.TxIn{
					{
						PreviousOutPoint: wire.OutPoint{
							// hash of txCoinbaseBinding
							Hash:  wire.Hash{220, 135, 137, 12, 160, 115, 62, 251, 6, 162, 111, 181, 149, 164, 14, 184, 28, 95, 47, 39, 186, 242, 48, 135, 50, 163, 6, 205, 179, 57, 115, 109},
							Index: 0,
						},
					},
				},
				TxOut: []*wire.TxOut{
					{
						PkScript: stdBindingPkScript,
					},
				},
			},
			TxStore{
				wire.Hash{220, 135, 137, 12, 160, 115, 62, 251, 6, 162, 111, 181, 149, 164, 14, 184, 28, 95, 47, 39, 186, 242, 48, 135, 50, 163, 6, 205, 179, 57, 115, 109}: &TxData{
					Tx: massutil.NewTx(txCoinbaseBinding),
				},
			},
			false,
			ErrStandardBindingTx,
		},
		{
			"not standard script",
			&wire.MsgTx{
				TxOut: []*wire.TxOut{
					{
						PkScript: nonStdPkScript,
					},
				},
			},
			nil,
			false,
			ErrNonStandardType,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := make(map[txscript.ScriptClass]bool)
			_, err = checkPkScriptStandard(test.tx.TxOut[0], test.tx, cache, test.txStore)
			assert.Equal(t, err, test.err)
			if (test.isStandard && err != nil) ||
				(!test.isStandard && err == nil) {
				t.Fatalf("failed: %v", err)
				return
			}
		})
	}
}

// // helper func
// func ExampleBuf() {
// 	stdPkScript, _ := txscript.NewScriptBuilder().AddOp(txscript.OP_0).
// 		AddData([]byte{17, 217, 8, 218, 81, 209, 203, 86, 12, 88, 214, 158, 218, 172, 76, 128, 236, 13, 63, 56, 27, 211, 233, 160, 11, 6, 221, 228, 209, 54, 233, 162}).Script()

// 	txCoinbase := &wire.MsgTx{
// 		Version: 1,
// 		TxIn: []*wire.TxIn{
// 			{
// 				PreviousOutPoint: wire.OutPoint{
// 					Hash:  wire.Hash{},
// 					Index: math.MaxUint32,
// 				},
// 				Sequence: wire.MaxTxInSequenceNum,
// 			},
// 		},
// 		TxOut: []*wire.TxOut{
// 			{
// 				Value:    100000000,
// 				PkScript: stdPkScript,
// 			},
// 		},
// 		LockTime: 0,
// 	}
// 	// Output:
// }

// TestDust tests the isDust API.
func TestDust(t *testing.T) {
	pkScript := []byte{
		0, 32, 17, 217, 8, 218,
		81, 209, 203, 86, 12, 88,
		214, 158, 218, 172, 76, 128,
		236, 13, 63, 56, 27, 211,
		233, 160, 11, 6, 221, 228,
		209, 54, 233, 162,
	}

	amt1, err := massutil.NewAmountFromUint(1)
	assert.Nil(t, err)
	amtMinRelay, err := massutil.NewAmountFromUint(consensus.MinRelayTxFee)
	assert.Nil(t, err)

	tests := []struct {
		name     string // test description
		txOut    wire.TxOut
		relayFee massutil.Amount // minimum relay transaction fee.
		isDust   bool
	}{
		{
			// Any value is allowed with a zero relay fee.
			"zero value with zero relay fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			massutil.ZeroAmount(),
			false,
		},
		{
			// Zero value is dust with any relay fee"
			"zero value with very small tx fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			amt1,
			true,
		},
		{
			"34 byte p2wsh script with value 587",
			wire.TxOut{Value: 5879, PkScript: pkScript},
			amtMinRelay,
			true,
		},
		{
			"34 byte p2wsh script with value 588",
			wire.TxOut{Value: 5880, PkScript: pkScript},
			amtMinRelay,
			false,
		},
		{
			// Maximum allowed value is never dust.
			"max maxwell amount is never dust",
			wire.TxOut{Value: massutil.MaxAmount().IntValue(), PkScript: pkScript},
			massutil.MaxAmount(),
			false,
		},
		{
			// Unspendable pkScript due to an invalid public key
			// script.
			"unspendable pkScript",
			wire.TxOut{Value: 5000, PkScript: []byte{0x01}},
			massutil.ZeroAmount(), // no relay fee
			true,
		},
	}
	for i, test := range tests {
		res, err := isDust(&test.txOut, test.relayFee)
		assert.Nil(t, err)
		if res != test.isDust {
			t.Fatalf("i:%d,Dust test '%s' failed: want %v got %v", i,
				test.name, test.isDust, res)
			continue
		}
	}
}

func TestCheckInputsStandard(t *testing.T) {
	txP, close, err := newTxPool(22)
	if err != nil {
		t.Error(err)
	}
	defer close()

	blk, err := loadNthBlk(23)
	assert.Equal(t, uint64(22), blk.Height())
	assert.Nil(t, err)
	tx := blk.Transactions()[1]
	prevHash := tx.MsgTx().TxIn[0].PreviousOutPoint.Hash
	prevOut := tx.MsgTx().TxIn[0].PreviousOutPoint.Index

	txStore := txP.FetchInputTransactions(tx, false)
	err = checkInputsStandard(tx, txStore)
	assert.Nil(t, err)

	oriPks := txStore[prevHash].Tx.MsgTx().TxOut[prevOut].PkScript
	txStore[prevHash].Tx.MsgTx().TxOut[prevOut].PkScript = []byte{75}
	err = checkInputsStandard(tx, txStore)
	assert.Equal(t, ErrParseInputScript, err)
	//reset
	txStore[prevHash].Tx.MsgTx().TxOut[prevOut].PkScript = oriPks

	tx.MsgTx().TxIn[0].Witness = tx.MsgTx().TxIn[0].Witness[1:]
	err = checkInputsStandard(tx, txStore)
	assert.Equal(t, ErrWitnessLength, err)
}

func TestCheckTransactionStandard(t *testing.T) {
	var witnessV0Addr massutil.Address
	pk, _ := btcec.NewPrivateKey(btcec.S256())
	_, witnessV0Addr, _ = newWitnessScriptAddress([]*btcec.PublicKey{pk.PubKey()},
		1, massutil.AddressClassWitnessV0, &config.ChainParams)

	// Create some dummy, but otherwise standard, data for transactions.
	prevOutHash, err := wire.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewHashFromStr: unexpected error: %v", err)
	}
	dummyPrevOut := wire.OutPoint{Hash: *prevOutHash, Index: 1}
	dummyTxIn := wire.TxIn{
		Witness:          testTx.TxIn[0].Witness,
		PreviousOutPoint: dummyPrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
	}

	addr, err := massutil.NewAddressWitnessScriptHash(witnessV0Addr.ScriptAddress(),
		&config.ChainParams)
	if err != nil {
		t.Fatalf("NewAddressPubKeyHash: unexpected error: %v", err)
	}
	dummyPkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("PayToAddrScript: unexpected error: %v", err)
	}
	dummyTxOut := wire.TxOut{
		Value:    100000000, // 1 MASS
		PkScript: dummyPkScript,
	}

	tests := []struct {
		name   string
		tx     wire.MsgTx
		height uint64
		err    error
	}{
		{
			name: "Typical pay-to-witness-script-hash transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
				Payload:  []byte{},
			},
			height: 300000,
		},
		{
			name: "Transaction version too high",
			tx: wire.MsgTx{
				Version:  wire.TxVersion + 1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrInvalidTxVersion,
		},
		{
			name: "Transaction is not finalized",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         0,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 300001,
			},
			height: 300000,
			err:    ErrUnfinalizedTx,
		},
		{
			name: "Transaction size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value: 0,
					PkScript: bytes.Repeat([]byte{0x00},
						maxStandardTxSize+1),
				}},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrNonStandardTxSize,
		},
		{
			name: "too less witness",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         wire.MaxTxInSequenceNum,
					Witness:          wire.TxWitness{[]byte{0x00}},
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrWitnessLength,
		},
		{
			name: "too many witness",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         wire.MaxTxInSequenceNum,
					Witness:          wire.TxWitness{[]byte{0x00}, []byte{0x01}, []byte{0x02}},
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrWitnessLength,
		},
		{
			name: "WitnessSize size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         wire.MaxTxInSequenceNum,
					Witness: wire.TxWitness{dummyTxIn.Witness[0],
						bytes.Repeat([]byte{0x02}, maxStandardWitnessSize-len(dummyTxIn.Witness[0])+1)},
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrWitnessSize,
		},
		{
			name: "WitnessSize size reaches the maximum",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         wire.MaxTxInSequenceNum,
					Witness: wire.TxWitness{dummyTxIn.Witness[0],
						bytes.Repeat([]byte{0x02}, maxStandardWitnessSize-len(dummyTxIn.Witness[0]))},
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    nil,
		},
		{
			name: "witness that does more than push data",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					Sequence:         wire.MaxTxInSequenceNum,
					Witness:          [][]byte{{100, 20, 63, 144, 140, 149, 87, 108, 214, 24, 129, 250, 177, 187, 82, 157, 39, 239, 170, 233, 125, 124}, {75}},
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrSignaturePushOnly,
		},
		{
			name: "Valid but non standard script",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    100000000,
					PkScript: []byte{txscript.OP_TRUE},
				}},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrNonStandardType,
		},
		{
			name: "More than one nulldata output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrNuLLDataScript,
		},
		{
			name: "Dust output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: dummyPkScript,
				}},
				LockTime: 0,
			},
			height: 300000,
			err:    ErrDust,
		},
		{
			name: "One nulldata output with 0 amount (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height: 300000,
		},
	}

	txStore := make(TxStore)
	// timeSource := NewMedianTime()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Ensure standardness is as expected.
			err := checkTransactionStandard(massutil.NewTx(&test.tx),
				test.height, massutil.MinRelayTxFee(), txStore)
			assert.Equal(t, test.err, err)
		})
	}
}
