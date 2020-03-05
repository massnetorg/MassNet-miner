// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/wire"
)

func decodeHexStr(hexStr string) ([]byte, error) {
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

// pkScript & signedTx is created by sign_test.go:TestSignTxOutputWit
var (
	pkScript = []byte{0, 32, 250, 159, 128, 11, 240, 24, 136, 109, 216, 158, 124, 20, 7, 192, 37, 109, 8, 98, 23, 13, 5, 202, 56, 230, 158, 145, 22, 243, 112, 118, 176, 75}
	signedTx = "0801127e0a020a001248473044022028ea0411ca656ea9eea5a5e2950e50a3fe0ffd25f205ac0b0052dcf9b7335a5002206b9befeb8eb2427225a136c158953ababd75cc7bb9354ce2905f0ee6ea5ed2b501122551210379028390929ee7e71b4475b59c6c2ce20b6aad2b6a09ea2668de0237b4daf09551ae19ffffffffffffffff1281010a040a0010011249483045022100aff298a66483bcd99d5005734cddda55a40c565cfbdf77c6013ddc67ac4e7b47022019449d9fd6a4c466cd77c85e2a9a9491f194db7d268367a5ac5ae20a27d1335e01122551210379028390929ee7e71b4475b59c6c2ce20b6aad2b6a09ea2668de0237b4daf09551ae19ffffffffffffffff1281010a040a0010021249483045022100b6d5a5ba7afb56c842aadf1f1ba502167fe5e1ea881df6d733591e7c92d92918022030dc812eef8f2aa3a9d170d2504069c5b96fb289ca437b13271c8b40c3e4ad7b01122551210379028390929ee7e71b4475b59c6c2ce20b6aad2b6a09ea2668de0237b4daf09551ae19ffffffffffffffff1a0208011a0208021a020803"
)

func decodeHexTx(signedTx string) (*wire.MsgTx, error) {
	serializedTx, err := decodeHexStr(signedTx)
	if err != nil {
		return nil, err
	}
	var tx wire.MsgTx
	err = tx.SetBytes(serializedTx, wire.Packet)
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// TestBadPC sets the pc to a deliberately bad result then confirms that Step()
// and Disasm fail correctly.
func TestBadPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		script, off int
	}{
		{script: 5, off: 0},
		{script: 0, off: 5},
	}

	// tx with 2-2 redeem scripts.
	tx, err := decodeHexTx(signedTx)
	if err != nil {
		t.Errorf("decode Tx error:%v", err)
	}

	for _, test := range tests {
		vm, err := NewEngine(pkScript, tx, 0, 0, nil, nil, -1)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
		}

		// set to after all scripts
		vm.scriptIdx = test.script
		vm.scriptOff = test.off

		_, err = vm.Step()
		if err == nil {
			t.Errorf("Step with invalid pc (%v) succeeds!", test)
			continue
		}

		_, err = vm.DisasmPC()
		if err == nil {
			t.Errorf("DisasmPC with invalid pc (%v) succeeds!",
				test)
		}
	}
}

//TestEngine_Execute test the execute of vm
func TestEngine_Execute(t *testing.T) {
	anyoneRedeemableScript, _ := NewScriptBuilder().AddOp(OP_TRUE).Script()
	tests := []struct {
		name     string
		pkScript []byte
		err      error
		execErr  error
	}{
		{
			"nil witness program",
			nil,
			ErrWitnessUnexpected,
			nil,
		},
		{
			"anyone redeemable",
			anyoneRedeemableScript,
			nil,
			nil,
		},
		{
			"invalid script",
			[]byte{0x00},
			nil,
			ErrStackScriptFailed,
		},
		{
			"unspendable script",
			[]byte{0x01},
			ErrStackShortScript,
			nil,
		},
		{
			"op_return script",
			[]byte{0x6a},
			nil,
			ErrStackEarlyReturn,
		},
		{
			"mismatched witness program",
			[]byte{0x00, 0x14, 0x42, 0xbe, 0x17, 0x44, 0x6c, 0xd3, 0xa6, 0x45, 0xee, 0x02, 0x0f, 0x9c, 0x8a, 0xfd, 0x3c, 0xcc, 0x7e, 0x6d, 0xda, 0x30},
			nil,
			ErrWitnessProgramMismatch,
		},
		{
			"valid witness program",
			pkScript,
			nil,
			nil,
		},
	}
	//instantite the msgtx
	tx, err := decodeHexTx(signedTx)
	if err != nil {
		t.Errorf("decode Tx error:%v", err)
	}
	//hashCache is used for the sighash
	hashCache := NewTxSigHashes(tx)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm, err := NewEngine(tt.pkScript, tx, 0, StandardVerifyFlags, nil, hashCache, 3000000000)
			assert.Equal(t, tt.err, err)
			if err == nil {
				err = vm.Execute()
				assert.Equal(t, tt.execErr, err)
			}
		})
	}
}

// TestCheckPubKeyEncoding ensures the internal checkPubKeyEncoding function
// works as expected.
func TestCheckPubKeyEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     []byte
		isValid bool
	}{
		{
			name: "uncompressed ok",
			key: decodeHex("0411db93e1dcdb8a016b49840f8c53bc1eb68" +
				"a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf" +
				"9744464f82e160bfa9b8b64f9d4c03f999b8643f656b" +
				"412a3"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: decodeHex("02ce0b14fb842b1ba549fdd675c98075f12e9" +
				"c510f8ef52bd021a9a1f4809d3b4d"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: decodeHex("032689c7c2dab13309fb143e0e8fe39634252" +
				"1887e976690b6b47f5b2a4b7d448e"),
			isValid: true,
		},
		{
			name: "hybrid",
			key: decodeHex("0679be667ef9dcbbac55a06295ce870b07029" +
				"bfcdb2dce28d959f2815b16f81798483ada7726a3c46" +
				"55da4fbfc0e1108a8fd17b448a68554199c47d08ffb1" +
				"0d4b8"),
			isValid: false,
		},
		{
			name:    "empty",
			key:     nil,
			isValid: false,
		},
	}

	// flags := ScriptVerifyWitnessPubKeyType | ScriptVerifyStrictEncoding
	flags := StandardVerifyFlags
	for _, test := range tests {
		err := TstCheckPubKeyEncoding(test.key, flags)
		if err != nil && test.isValid {
			t.Errorf("checkPubkeyEncoding test '%s' failed "+
				"when it should have succeeded: %v", test.name,
				err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkPubkeyEncooding test '%s' succeeded "+
				"when it should have failed", test.name)
		}
	}

}

// TestCheckSignatureEncoding ensures the internal checkSignatureEncoding
// function works as expected.
func TestCheckSignatureEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sig     []byte
		isValid bool
	}{
		{
			name: "valid signature",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: true,
		},
		{
			name:    "empty.",
			sig:     nil,
			isValid: false,
		},
		{
			name: "bad magic",
			sig: decodeHex("314402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 1st int marker magic",
			sig: decodeHex("304403204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 2nd int marker",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41032018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short len",
			sig: decodeHex("304302204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long len",
			sig: decodeHex("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long X",
			sig: decodeHex("304402424e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long Y",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022118152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short Y",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41021918152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "trailing crap",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d0901"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: decodeHex("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364141022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: decodeHex("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364142022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "Y == N",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364141"),
			isValid: false,
		},
		{
			name: "Y > N",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364142"),
			isValid: false,
		},
		{
			name: "0 len X",
			sig: decodeHex("302402000220181522ec8eca07de4860a4acd" +
				"d12909d831cc56cbbac4622082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "0 len Y",
			sig: decodeHex("302402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410200"),
			isValid: false,
		},
		{
			name: "extra R padding",
			sig: decodeHex("30450221004e45e16932b8af514961a1d3a1a" +
				"25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "extra S padding",
			sig: decodeHex("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022100181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
	}

	// flags := ScriptVerifyStrictEncoding
	flags := StandardVerifyFlags
	for _, test := range tests {
		err := TstCheckSignatureEncoding(test.sig, flags)
		if err != nil && test.isValid {
			t.Errorf("checkSignatureEncoding test '%s' failed "+
				"when it should have succeeded: %v", test.name,
				err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkSignatureEncooding test '%s' succeeded "+
				"when it should have failed", test.name)
		}
	}
}

func TestEngine_Add(t *testing.T) {
	tests := []struct {
		name      string
		scriptStr string
		err       error
	}{
		{
			"add < 16",
			"1 2 OP_ADD 3 OP_EQUAL",
			nil,
		},
		{
			"",
			"1 16 OP_ADD 17 OP_EQUAL",
			nil,
		},
		{
			"",
			"17 18 OP_ADD 35 OP_EQUAL",
			nil,
		},
		{
			"",
			"17 18 OP_ADD 5 OP_EQUAL",
			ErrStackScriptFailed,
		},
		{
			"",
			"120 260 OP_ADD 380 OP_EQUAL",
			nil,
		},
		{
			"",
			"1073741824 1 OP_ADD 1073741825 OP_EQUAL",
			nil,
		},
		{
			"",
			"1073741824 1073741825 OP_ADD 2147483649 OP_EQUAL",
			nil,
		},
		{
			"",
			"1073741824 1073741824 OP_ADD 2147483648 OP_EQUAL",
			nil,
		},
		{
			"123",
			"2147483648 1 OP_ADD 2147483649 OP_EQUAL",
			ErrStackNumberTooBig,
		},
		{
			"32bit",
			"2147483647 1073741824 OP_ADD 3221225471 OP_EQUAL",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := mustParseShortForm(tt.scriptStr)
			vm := Engine{}
			pops, err := TstParseScript(script)
			assert.Nil(t, err)
			TstAddScript(&vm, pops)

			err = vm.Execute()
			assert.Equal(t, err, tt.err)
		})
	}
}

// func TestEngine_Execute2(t *testing.T) {
// 	tests := []struct {
// 		name           string
// 		witnessSig     string
// 		witnessRedeem  string
// 		pkScript       string
// 		witnessProgram []byte
// 		err            error
// 	}{
// 		{
// 			"invalid witness script",
// 			"1 1 OP_ADD 2 OP_EQUAL",
// 			"1",
// 			"1 1",
// 			[]byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
// 			ErrwitnessProgramFormat,
// 		},
// 		{
// 			"invalid version script",
// 			"1 2",
// 			"1",
// 			"1 1",
// 			[]byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
// 			ErrwitnessProgramFormat,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			witSig := mustParseShortForm(tt.witnessSig)
// 			witRedeem := mustParseShortForm(tt.witnessSig)
// 			pkScript := mustParseShortForm(tt.pkScript)

// 			parsedPkScript, err := TstParseScript(pkScript)
// 			assert.Nil(t, err)
// 			// parsedWitSig, err := TstParseScript(witSig)
// 			// assert.Nil(t, err)
// 			// parsedWitRedeem, err := TstParseScript(witRedeem)
// 			// assert.Nil(t, err)

// 			tx := &wire.MsgTx{
// 				Version: 1,
// 				TxIn: []*wire.TxIn{
// 					{
// 						PreviousOutPoint: wire.OutPoint{
// 							Hash:  wire.Hash{},
// 							Index: 0,
// 						},
// 						Sequence: wire.MaxTxInSequenceNum,
// 						Witness:  wire.TxWitness{witSig, witRedeem},
// 					},
// 				},
// 				TxOut: []*wire.TxOut{
// 					{
// 						Value: 1,
// 					},
// 				},
// 				LockTime: 0,
// 			}

// 			vm := Engine{
// 				tx:    *tx,
// 				txIdx: 0,
// 			}
// 			TstAddVersion(&vm, 0)
// 			TstAddProgram(&vm, tt.witnessProgram)
// 			TstAddScript(&vm, parsedPkScript)
// 			// TstAddScript(&vm, parsedWitSig)
// 			// TstAddScript(&vm, parsedWitRedeem)
// 			err = vm.Execute()
// 			assert.Equal(t, tt.err, err)
// 		})
// 	}

// }
