// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/wire"
)

// parseScriptFlags parses the provided flags string from the format used in the
// reference tests into ScriptFlags suitable for use in the script engine.
func parseScriptFlags(flagStr string) (ScriptFlags, error) {
	var flags ScriptFlags

	sFlags := strings.Split(flagStr, ",")
	for _, flag := range sFlags {
		switch flag {
		case "":
			// Nothing.
		// case "CLEANSTACK":
		// 	flags |= ScriptVerifyCleanStack
		// case "DERSIG":
		// 	flags |= ScriptVerifyDERSignatures
		case "DISCOURAGE_UPGRADABLE_NOPS":
			flags |= ScriptDiscourageUpgradableNops
		// case "LOW_S":
		// 	flags |= ScriptVerifyLowS
		// case "MINIMALDATA":
		// 	flags |= ScriptVerifyMinimalData
		case "NONE":
			// Nothing.
		// case "NULLDUMMY":
		// 	flags |= ScriptStrictMultiSig
		// case "P2SH":
		// 	flags |= ScriptBip16
		// case "SIGPUSHONLY":
		// 	flags |= ScriptVerifySigPushOnly
		// case "STRICTENC":
		// 	flags |= ScriptVerifyStrictEncoding
		case "CLEANSTACK", "DERSIG", "LOW_S", "MINIMALDATA",
			"NULLDUMMY", "P2SH", "SIGPUSHONLY", "STRICTENC":
		default:
			return flags, fmt.Errorf("invalid flag: %s", flag)
		}
	}
	return flags, nil
}

// TestOpcodeDisabled tests the opcodeDisabled function manually because all
// disabled opcodes result in a script execution failure when executed normally,
// so the function is not called under normal circumstances.
func TestOpcodeDisabled(t *testing.T) {
	t.Parallel()

	tests := []byte{OP_CAT, OP_SUBSTR, OP_LEFT, OP_RIGHT, OP_INVERT,
		OP_AND, OP_OR, OP_2MUL, OP_2DIV, OP_MUL, OP_DIV, OP_MOD,
		OP_LSHIFT, OP_RSHIFT,
	}
	for _, opcodeVal := range tests {
		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: nil}
		if err := opcodeDisabled(&pop, nil); err != ErrStackOpDisabled {
			t.Errorf("opcodeDisabled: unexpected error - got %v, "+
				"want %v", err, ErrStackOpDisabled)
			return
		}
	}
}

// TestOpcodeDisasm tests the print function for all opcodes in both the oneline
// and full modes to ensure it provides the expected disassembly.
func TestOpcodeDisasm(t *testing.T) {
	t.Parallel()

	// First, test the oneline disassembly.

	// The expected strings for the data push opcodes are replaced in the
	// test loops below since they involve repeating bytes.  Also, the
	// OP_NOP# and OP_UNKNOWN# are replaced below too, since it's easier
	// than manually listing them here.
	oneBytes := []byte{0x01}
	oneStr := "01"
	expectedStrings := map[int]string{0x00: "0", 0x4f: "-1",
		0x50: "OP_RESERVED", 0x61: "OP_NOP", 0x62: "OP_VER",
		0x63: "OP_IF", 0x64: "OP_NOTIF", 0x65: "OP_VERIF",
		0x66: "OP_VERNOTIF", 0x67: "OP_ELSE", 0x68: "OP_ENDIF",
		0x69: "OP_VERIFY", 0x6a: "OP_RETURN", 0x6b: "OP_TOALTSTACK",
		0x6c: "OP_FROMALTSTACK", 0x6d: "OP_2DROP", 0x6e: "OP_2DUP",
		0x6f: "OP_3DUP", 0x70: "OP_2OVER", 0x71: "OP_2ROT",
		0x72: "OP_2SWAP", 0x73: "OP_IFDUP", 0x74: "OP_DEPTH",
		0x75: "OP_DROP", 0x76: "OP_DUP", 0x77: "OP_NIP",
		0x78: "OP_OVER", 0x79: "OP_PICK", 0x7a: "OP_ROLL",
		0x7b: "OP_ROT", 0x7c: "OP_SWAP", 0x7d: "OP_TUCK",
		0x7e: "OP_CAT", 0x7f: "OP_SUBSTR", 0x80: "OP_LEFT",
		0x81: "OP_RIGHT", 0x82: "OP_SIZE", 0x83: "OP_INVERT",
		0x84: "OP_AND", 0x85: "OP_OR", 0x86: "OP_XOR",
		0x87: "OP_EQUAL", 0x88: "OP_EQUALVERIFY", 0x89: "OP_RESERVED1",
		0x8a: "OP_RESERVED2", 0x8b: "OP_1ADD", 0x8c: "OP_1SUB",
		0x8d: "OP_2MUL", 0x8e: "OP_2DIV", 0x8f: "OP_NEGATE",
		0x90: "OP_ABS", 0x91: "OP_NOT", 0x92: "OP_0NOTEQUAL",
		0x93: "OP_ADD", 0x94: "OP_SUB", 0x95: "OP_MUL", 0x96: "OP_DIV",
		0x97: "OP_MOD", 0x98: "OP_LSHIFT", 0x99: "OP_RSHIFT",
		0x9a: "OP_BOOLAND", 0x9b: "OP_BOOLOR", 0x9c: "OP_NUMEQUAL",
		0x9d: "OP_NUMEQUALVERIFY", 0x9e: "OP_NUMNOTEQUAL",
		0x9f: "OP_LESSTHAN", 0xa0: "OP_GREATERTHAN",
		0xa1: "OP_LESSTHANOREQUAL", 0xa2: "OP_GREATERTHANOREQUAL",
		0xa3: "OP_MIN", 0xa4: "OP_MAX", 0xa5: "OP_WITHIN",
		0xa6: "OP_RIPEMD160", 0xa7: "OP_SHA1", 0xa8: "OP_SHA256",
		0xa9: "OP_HASH160", 0xaa: "OP_HASH256", 0xab: "OP_CODESEPARATOR",
		0xac: "OP_CHECKSIG", 0xad: "OP_CHECKSIGVERIFY",
		0xae: "OP_CHECKMULTISIG", 0xaf: "OP_CHECKMULTISIGVERIFY",
		0xf9: "OP_SMALLDATA", 0xfa: "OP_SMALLINTEGER",
		0xfb: "OP_PUBKEYS", 0xfd: "OP_PUBKEYHASH", 0xfe: "OP_PUBKEY",
		0xff: "OP_INVALIDOPCODE",
	}
	for opcodeVal, expectedStr := range expectedStrings {
		var data []byte
		switch {
		// OP_DATA_1 through OP_DATA_65 display the pushed data.
		case opcodeVal >= 0x01 && opcodeVal < 0x4c:
			data = bytes.Repeat(oneBytes, opcodeVal)
			expectedStr = strings.Repeat(oneStr, opcodeVal)

			// OP_PUSHDATA1.
		case opcodeVal == 0x4c:
			data = bytes.Repeat(oneBytes, 1)
			expectedStr = strings.Repeat(oneStr, 1)

			// OP_PUSHDATA2.
		case opcodeVal == 0x4d:
			data = bytes.Repeat(oneBytes, 2)
			expectedStr = strings.Repeat(oneStr, 2)

			// OP_PUSHDATA4.
		case opcodeVal == 0x4e:
			data = bytes.Repeat(oneBytes, 3)
			expectedStr = strings.Repeat(oneStr, 3)

			// OP_1 through OP_16 display the numbers themselves.
		case opcodeVal >= 0x51 && opcodeVal <= 0x60:
			val := byte(opcodeVal - (0x51 - 1))
			data = []byte{val}
			expectedStr = strconv.Itoa(int(val))

			// OP_NOP1 through OP_NOP10.
		case opcodeVal >= 0xb0 && opcodeVal <= 0xb9:
			// OP_NOP2 is an alias of OP_CHECKLOCKTIMEVERIFY
			if opcodeVal == 0xb1 {
				expectedStr = "OP_CHECKLOCKTIMEVERIFY"
			} else if opcodeVal == 0xb2 {
				expectedStr = "OP_CHECKSEQUENCEVERIFY"
			} else {
				val := byte(opcodeVal - (0xb0 - 1))
				expectedStr = "OP_NOP" + strconv.Itoa(int(val))
			}

			// OP_UNKNOWN#.
		case opcodeVal >= 0xba && opcodeVal <= 0xf8 || opcodeVal == 0xfc:
			expectedStr = "OP_UNKNOWN" + strconv.Itoa(int(opcodeVal))
		}

		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: data}
		gotStr := pop.print(true)
		if gotStr != expectedStr {
			t.Errorf("pop.print (opcode %x): Unexpected disasm "+
				"string - got %v, want %v", opcodeVal, gotStr,
				expectedStr)
			continue
		}
	}

	// Now, replace the relevant fields and test the full disassembly.
	expectedStrings[0x00] = "OP_0"
	expectedStrings[0x4f] = "OP_1NEGATE"
	for opcodeVal, expectedStr := range expectedStrings {
		var data []byte
		switch {
		// OP_DATA_1 through OP_DATA_65 display the opcode followed by
		// the pushed data.
		case opcodeVal >= 0x01 && opcodeVal < 0x4c:
			data = bytes.Repeat(oneBytes, opcodeVal)
			expectedStr = fmt.Sprintf("OP_DATA_%d 0x%s", opcodeVal,
				strings.Repeat(oneStr, opcodeVal))

			// OP_PUSHDATA1.
		case opcodeVal == 0x4c:
			data = bytes.Repeat(oneBytes, 1)
			expectedStr = fmt.Sprintf("OP_PUSHDATA1 0x%02x 0x%s",
				len(data), strings.Repeat(oneStr, 1))

			// OP_PUSHDATA2.
		case opcodeVal == 0x4d:
			data = bytes.Repeat(oneBytes, 2)
			expectedStr = fmt.Sprintf("OP_PUSHDATA2 0x%04x 0x%s",
				len(data), strings.Repeat(oneStr, 2))

			// OP_PUSHDATA4.
		case opcodeVal == 0x4e:
			data = bytes.Repeat(oneBytes, 3)
			expectedStr = fmt.Sprintf("OP_PUSHDATA4 0x%08x 0x%s",
				len(data), strings.Repeat(oneStr, 3))

			// OP_1 through OP_16.
		case opcodeVal >= 0x51 && opcodeVal <= 0x60:
			val := byte(opcodeVal - (0x51 - 1))
			data = []byte{val}
			expectedStr = "OP_" + strconv.Itoa(int(val))

			// OP_NOP1 through OP_NOP10.
		case opcodeVal >= 0xb0 && opcodeVal <= 0xb9:
			// OP_NOP2 is an alias of OP_CHECKLOCKTIMEVERIFY
			if opcodeVal == 0xb1 {
				expectedStr = "OP_CHECKLOCKTIMEVERIFY"
			} else if opcodeVal == 0xb2 {
				expectedStr = "OP_CHECKSEQUENCEVERIFY"
			} else {
				val := byte(opcodeVal - (0xb0 - 1))
				expectedStr = "OP_NOP" + strconv.Itoa(int(val))
			}

			// OP_UNKNOWN#.
		case opcodeVal >= 0xba && opcodeVal <= 0xf8 || opcodeVal == 0xfc:
			expectedStr = "OP_UNKNOWN" + strconv.Itoa(int(opcodeVal))
		}

		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: data}
		gotStr := pop.print(false)
		if gotStr != expectedStr {
			t.Errorf("pop.print (opcode %x): Unexpected disasm "+
				"string - got %v, want %v", opcodeVal, gotStr,
				expectedStr)
			continue
		}
	}
}

func TestOpcode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		scriptStr string
		err       string
	}{
		{
			"OP_NOP",
			"OP_NOP5",
			"OP_NOP5 reserved for soft-fork upgrades",
		},
		{
			"valid OP_IF",
			"0 IF 0 ELSE 1 ELSE 0 ENDIF",
			"",
		},
		{
			"invalid OP_NOTIF",
			"0 NOTIF ELSE 1 ENDIF",
			"stack is not clean",
		},
		{
			"valid OP_NOTIF",
			"0 NOTIF 1 ENDIF",
			"",
		},
		{
			"valid OP_VERIFY",
			"1 1 VERIFY",
			"",
		},
		{
			"valid OP_RETURN",
			"RETURN 2",
			"script returned early",
		},
		{
			"valid OP_TOALTSTACK and OP_FROMALTSTACK",
			"10 0 11 TOALTSTACK DROP FROMALTSTACK ADD 21 EQUAL",
			"",
		},
		{
			"valid OP_ROT",
			"22 21 20 ROT 22 EQUAL 2DROP",
			"",
		},
		{
			"valid OP_2ROT",
			"25 24 23 22 21 20 2ROT 2ROT 22 EQUAL 2DROP 2DROP DROP",
			"",
		},
		{
			"valid OP_TUCK",
			"0 1 TUCK DEPTH 3 EQUALVERIFY SWAP 2DROP",
			"",
		},
		{
			"vaild OP_2DUP",
			"13 14 2DUP ROT EQUALVERIFY EQUAL",
			"",
		},
		{
			"valid OP_3DUP",
			"-1 0 1 2 3DUP DEPTH 7 EQUALVERIFY ADD ADD 3 EQUALVERIFY 2DROP 0 EQUALVERIFY",
			"",
		},
		{
			"valid OP_OVER",
			"1 0 OVER DEPTH 3 EQUAL 2DROP DROP",
			"",
		},
		{
			"valid OP_2OVER",
			"1 2 3 5 2OVER ADD ADD 8 EQUALVERIFY ADD ADD 6 EQUAL",
			"",
		},
		{
			"valid OP_SWAP",
			"0 1 SWAP DROP",
			"",
		},
		{
			"valid OP_2SWAP",
			"1 3 5 7 2SWAP ADD 4 EQUALVERIFY ADD 12 EQUAL",
			"",
		},
		{
			"valid OP_IFDUP",
			"0 IFDUP DEPTH 1 EQUALVERIFY 0 EQUAL",
			"",
		},
		{
			"valid OP_NIP",
			"0 1 NIP",
			"",
		},
		{
			"valid OP_PICK",
			"1 0 20 0 PICK 20 EQUALVERIFY DEPTH 4 EQUAL 2DROP DROP",
			"",
		},
		{
			"valid OP_ROLL",
			"1 0 ROLL",
			"",
		},
		{
			"valid OP_ROT",
			"0 1 0 ROT 2DROP",
			"",
		},
		{
			"valid OP_SIZE",
			"1 0x00 SIZE 0 EQUALVERIFY DROP",
			"",
		},
		{
			"valid OP_1ADD",
			"2 1ADD 3 EQUAL",
			"",
		},
		{
			"valid OP_1SUB",
			"2 1SUB 1 EQUAL",
			"",
		},
		{
			"valid OP_NEGATE",
			"2 NEGATE -2 EQUAL",
			"",
		},
		{
			"valid OP_ABS",
			"-2 ABS 2 EQUAL",
			"",
		},
		{
			"valid OP_NOT",
			"0x02 0x0000 NOT DROP 1",
			"",
		},
		{
			"valid OP_0NOTEQUAL",
			"0x02 0x0000 0NOTEQUAL DROP 1",
			"",
		},
		{
			"valid OP_SUB",
			"2 1 SUB 1 EQUAL",
			"",
		},
		{
			"valid OP_BOOLAND",
			"0 0x02 0x0000 BOOLAND DROP 1",
			"",
		},
		{
			"valid OP_BOOLOR",
			"0x02 0x0000 0 BOOLOR DROP 1",
			"",
		},
		{
			"valid OP_NUMEQUAL",
			"0 0 NUMEQUAL",
			"",
		},
		{
			"valid OP_NUMEQUALVERIFY",
			"0 0 NUMEQUALVERIFY 1",
			"",
		},
		{
			"valid OP_NUMNOTEQUAL",
			"-1 0 NUMNOTEQUAL",
			"",
		},
		{
			"valid OP_LESSTHAN",
			"-1 0 LESSTHAN",
			"",
		},
		{
			"valid OP_GREATERTHAN",
			"1 0 GREATERTHAN",
			"",
		},
		{
			"valid OP_LESSTHANOREQUAL",
			"0 0 LESSTHANOREQUAL",
			"",
		},
		{
			"valid OP_GREATERTHANOREQUAL",
			"0 0 GREATERTHANOREQUAL",
			"",
		},
		{
			"valid OP_MIN",
			"-1 0 MIN",
			"",
		},
		{
			"valid OP_MAX",
			"1 0 MAX",
			"",
		},
		{
			"valid OP_WITHIN",
			"-1 -1 0 WITHIN",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := mustParseShortForm(tt.scriptStr)
			vm := Engine{}
			vm.flags = StandardVerifyFlags
			pops, err := TstParseScript(script)
			assert.Nil(t, err)

			TstAddScript(&vm, pops)

			err = vm.Execute()
			if err != nil {
				assert.Equal(t, tt.err, err.Error())
				//println(vm.dstack.Depth())
			}
			//println(vm.dstack.Depth())
		})
	}
}

// TestScriptValidTests ensures all of the tests in script_valid.json pass as
// expected.
func TestScriptValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/scriptTest_valid.json")
	if err != nil {
		t.Errorf("TestBitcoinValidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindValidTests couldn't Unmarshal: %v",
			err)
		return
	}
	for i, test := range tests {
		// Skip comments
		if len(test) == 1 {
			continue
		}
		name, err := testName(test)
		if err != nil {
			t.Errorf("TestBitcoindValidTests: invalid test #%d",
				i)
			continue
		}
		scriptSig, err := parseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}
		scriptPubKey, err := parseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}
		vm := Engine{}
		var scriptStr [][]byte
		if len(scriptSig) != 0 {
			scriptStr = [][]byte{scriptSig, scriptPubKey}
		} else {
			scriptStr = [][]byte{scriptPubKey}
		}
		vm.scripts = make([][]parsedOpcode, len(scriptStr))
		for i, scr := range scriptStr {
			if len(scr) > maxScriptSize {
				t.Error("maxScript error")
			}
			var err error
			vm.scripts[i], err = parseScript(scr)
			if err != nil {
				t.Errorf("error is %v", err)
			}
		}
		err = vm.Execute()
		if err != nil {
			t.Errorf("%s failed to execute: %v", name, err)
			continue
		}
	}

}

func TestScriptInValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/scriptTest_invalid.json")
	if err != nil {
		t.Errorf("TestBitcoinValidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindValidTests couldn't Unmarshal: %v",
			err)
		return
	}

	for i, test := range tests {
		// Skip comments
		if len(test) == 1 {
			continue
		}
		name, err := testName(test)
		if err != nil {
			t.Errorf("TestBitcoindValidTests: invalid test #%d",
				i)
			continue
		}
		scriptSig, err := parseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}
		scriptPubKey, err := parseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}
		flags, err := parseScriptFlags(test[2])
		if err != nil {
			t.Errorf("%s: can't parse flags; %v", name, err)
			continue
		}
		vm := Engine{
			tx: wire.MsgTx{
				TxIn: []*wire.TxIn{
					&wire.TxIn{
						Sequence: wire.MaxTxInSequenceNum,
					},
				},
			},
		}
		vm.flags = flags
		var scriptStr [][]byte
		if len(scriptSig) != 0 {
			scriptStr = [][]byte{scriptSig, scriptPubKey}
		} else {
			scriptStr = [][]byte{scriptPubKey}
		}
		vm.scripts = make([][]parsedOpcode, len(scriptStr))
		for i, scr := range scriptStr {
			vm.scripts[i], err = parseScript(scr)
		}
		if err == nil {
			if err := vm.Execute(); err == nil {
				t.Errorf("%s test succeeded when it "+
					"should have failed\n", name)
			}
			continue
		}
	}
}
