// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/massutil"
)

// decodeHex decodes the passed hex string and returns the resulting bytes.  It
// panics if an error occurs.  This is only used in the tests as a helper since
// the only way it can fail is if there is an error in the test source code.
func decodeHex(hexStr string) []byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic("invalid hex string in test source: err " + err.Error() +
			", hex: " + hexStr)
	}

	return b
}

// mustParseShortForm parses the passed short form script and returns the
// resulting bytes.  It panics if an error occurs.  This is only used in the
// tests as a helper since the only way it can fail is if there is an error in
// the test source code.
func mustParseShortForm(script string) []byte {
	s, err := parseShortForm(script)
	if err != nil {
		panic("invalid short form script in test source: err " +
			err.Error() + ", script: " + script)
	}

	return s
}

func TestCalcScriptInfo(t *testing.T) {
	t.Parallel()
	//witness[0]=witnessSig, witness[1]=witnessRedeem
	tests := []struct {
		name          string
		SigScript     string
		witnessSig    string
		witnessRedeem string
		pkScript      string
		scriptInfo    ScriptInfo
		scriptInfoErr error
	}{
		{
			// Invented scripts, the hashes do not match
			// Truncated version of test below:
			name:       "p2wsh",
			SigScript:  "",
			witnessSig: "DATA_71 0x30440220349169b4fd8af5f942af9f5df88dc8da462e9d4c66ec23a206d3ddc8d8e08fba022061c23f0d3fc9bf39131c7f2ccc12e51cea5f24ff578e8a67c782ad3807d9e30101",
			witnessRedeem: "2 DATA_33 0x0102030405060708090a0b0c0d0e0f1011" +
				"12131415161718191a1b1c1d1e1f2021 DATA_33 " +
				"0x0102030405060708090a0b0c0d0e0f101112131415" +
				"161718191a1b1c1d1e1f2021 DATA_33 0x010203040" +
				"5060708090a0b0c0d0e0f101112131415161718191a1" +
				"b1c1d1e1f2021 3 CHECKMULTISIG",

			// pkScript: "OP_0 DATA_20 0xfe441065b6532231de2fac56" +
			// 	"3152205ec4f59caa",
			pkScript: "OP_0 DATA_32 0x010203040506070801020304050607080" +
				"1020304050607080102030405060708",
			scriptInfo: ScriptInfo{
				PkScriptClass:  WitnessV0ScriptHashTy,
				NumInputs:      2,
				ExpectedInputs: 3,
				SigOps:         3,
			},
		},
	}

	for _, test := range tests {
		witness := make([][]byte, 2)
		witness[0] = mustParseShortForm(test.witnessSig)
		witness[1] = mustParseShortForm(test.witnessRedeem)
		pkScript := mustParseShortForm(test.pkScript)
		si, err := CalcScriptInfo(pkScript, witness)
		if err != nil {
			if err != test.scriptInfoErr {
				t.Errorf("scriptinfo test \"%s\": got \"%v\""+
					"expected \"%v\"", test.name, err,
					test.scriptInfoErr)
			}
			continue
		}
		if test.scriptInfoErr != nil {
			t.Errorf("%s: succeeded when expecting \"%v\"",
				test.name, test.scriptInfoErr)
			continue
		}
		if *si != test.scriptInfo {
			t.Errorf("%s: scriptinfo doesn't match expected. "+
				"got: \"%v\" expected \"%v\"", test.name,
				*si, test.scriptInfo)
			continue
		}
	}
}

//// TestCalcMultiSigStats ensures the CalcMutliSigStats function returns the
//// expected errors.
func TestCalcMultiSigStats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		err    error
	}{
		{
			name: "short script",
			script: "0x046708afdb0fe5548271967f1a67130b7105cd6a828" +
				"e03909a67962e0ea1f61d",
			err: ErrStackShortScript,
		},
		{
			name: "stack underflow",
			script: "RETURN DATA_41 0x046708afdb0fe5548271967f1a" +
				"67130b7105cd6a828e03909a67962e0ea1f61deb649f6" +
				"bc3f4cef308",
			err: ErrStackUnderflow,
		},
		{
			name: "standard multisig script",
			script: "0 DATA_72 0x30450220106a3e4ef0b51b764a2887226" +
				"2ffef55846514dacbdcbbdd652c849d395b4384022100" +
				"e03ae554c3cbb40600d31dd46fc33f25e47bf8525b1fe" +
				"07282e3b6ecb5f3bb2801 CODESEPARATOR 1 DATA_33 " +
				"0x0232abdc893e7f0631364d7fd01cb33d24da45329a0" +
				"0357b3a7886211ab414d55a 1 CHECKMULTISIG",
			err: nil,
		},
	}

	for i, test := range tests {
		script := mustParseShortForm(test.script)
		if _, _, err := CalcMultiSigStats(script); err != test.err {
			t.Errorf("CalcMultiSigStats #%d (%s) unexpected "+
				"error\ngot: %v\nwant: %v", i, test.name, err,
				test.err)
		}
	}
}

// TestExtractPkScriptAddrs ensures that extracting the type, addresses, and
// number of required signatures from PkScripts works as intended.
func TestExtractPkScriptAddrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		script  []byte
		addrs   []massutil.Address
		reqSigs int
		class   ScriptClass
	}{

		{
			name: "standard p2wsh",
			script: mustParseShortForm("OP_0 DATA_32 0x010203040506070801020304050607080" +
				"1020304050607080102030405060708"),
			addrs: []massutil.Address{
				newAddressWitnessScriptHash(decodeHex("010203040506070801020304050607080" +
					"1020304050607080102030405060708")),
			},
			reqSigs: 1,
			class:   WitnessV0ScriptHashTy,
		},
		// from real tx 60a20bd93aa49ab4b28d514ec10b06e1829ce6818ec06cd3aabd013ebcdc4bb1, vout 0
		{
			name: "standard 1 of 2 multisig",
			script: mustParseShortForm("1 DATA_65 0x04cc71eb30d653c0c" +
				"3163990c47b976f3fb3f37cccdcbedb169a1" +
				"dfef58bbfbfaff7d8a473e7e2e6d317b87ba" +
				"fe8bde97e3cf8f065dec022b51d11fcdd0d3" +
				"48ac4 " +
				"DATA_65 0x0461cbdcc5409fb4b" +
				"4d42b51d33381354d80e550078cb532a34bf" +
				"a2fcfdeb7d76519aecc62770f5b0e4ef8551" +
				"946d8a540911abe3e7854a26f39f58b25c15" +
				"342af 2 CHECKMULTISIG"),
			addrs: []massutil.Address{
				newAddressPubKey(decodeHex("04cc71eb30d653c0c" +
					"3163990c47b976f3fb3f37cccdcbedb169a1" +
					"dfef58bbfbfaff7d8a473e7e2e6d317b87ba" +
					"fe8bde97e3cf8f065dec022b51d11fcdd0d3" +
					"48ac4")),
				newAddressPubKey(decodeHex("0461cbdcc5409fb4b" +
					"4d42b51d33381354d80e550078cb532a34bf" +
					"a2fcfdeb7d76519aecc62770f5b0e4ef8551" +
					"946d8a540911abe3e7854a26f39f58b25c15" +
					"342af")),
			},
			reqSigs: 1,
			class:   MultiSigTy,
		},
		// from real tx d646f82bd5fbdb94a36872ce460f97662b80c3050ad3209bef9d1e398ea277ab, vin 1
		{
			name: "standard 2 of 3 multisig",
			script: mustParseShortForm("2 DATA_65 0x04cb9c3c222c5f7a7" +
				"d3b9bd152f363a0b6d54c9eb312c4d4f9af1" +
				"e8551b6c421a6a4ab0e29105f24de20ff463" +
				"c1c91fcf3bf662cdde4783d4799f787cb7c0" +
				"8869b " +
				"DATA_65 0x04ccc588420deeebe" +
				"a22a7e900cc8b68620d2212c374604e3487c" +
				"a08f1ff3ae12bdc639514d0ec8612a2d3c51" +
				"9f084d9a00cbbe3b53d071e9b09e71e610b0" +
				"36aa2 " +
				"DATA_65 0x04ab47ad1939edcb3" +
				"db65f7fedea62bbf781c5410d3f22a7a3a56" +
				"ffefb2238af8627363bdf2ed97c1f89784a1" +
				"aecdb43384f11d2acc64443c7fc299cef040" +
				"0421a 3 CHECKMULTISIG"),
			addrs: []massutil.Address{
				newAddressPubKey(decodeHex("04cb9c3c222c5f7a7" +
					"d3b9bd152f363a0b6d54c9eb312c4d4f9af1" +
					"e8551b6c421a6a4ab0e29105f24de20ff463" +
					"c1c91fcf3bf662cdde4783d4799f787cb7c0" +
					"8869b")),
				newAddressPubKey(decodeHex("04ccc588420deeebe" +
					"a22a7e900cc8b68620d2212c374604e3487c" +
					"a08f1ff3ae12bdc639514d0ec8612a2d3c51" +
					"9f084d9a00cbbe3b53d071e9b09e71e610b0" +
					"36aa2")),
				newAddressPubKey(decodeHex("04ab47ad1939edcb3" +
					"db65f7fedea62bbf781c5410d3f22a7a3a56" +
					"ffefb2238af8627363bdf2ed97c1f89784a1" +
					"aecdb43384f11d2acc64443c7fc299cef040" +
					"0421a")),
			},
			reqSigs: 2,
			class:   MultiSigTy,
		},

		{
			name: "valid signature from a sigscript - no addresses",
			script: mustParseShortForm("DATA_71 0x304402204e45e16932b8af514961a1d3" +
				"a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220" +
				"181522ec8eca07de4860a4acdd12909d831cc56cbbac" +
				"4622082221a8768d1d0901"),
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		{
			name:    "empty script",
			script:  []byte{},
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
		{
			name:    "script that does not parse",
			script:  []byte{OP_DATA_45},
			addrs:   nil,
			reqSigs: 0,
			class:   NonStandardTy,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			class, addrs, _, reqSigs, err := ExtractPkScriptAddrs(
				test.script, &config.ChainParams)
			if err != nil {
			}

			if !reflect.DeepEqual(addrs, test.addrs) {
				t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
					"addresses\ngot  %v\nwant %v", i, test.name,
					addrs, test.addrs)
				return
			}

			if reqSigs != test.reqSigs {
				t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
					"number of required signatures - got %d, "+
					"want %d", i, test.name, reqSigs, test.reqSigs)
				return
			}

			if class != test.class {
				t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
					"script type - got %s, want %s", i, test.name,
					class, test.class)
				return
			}
		})
	}
}

// scriptClassTest houses a test used to ensure various scripts have the
// expected class.
type scriptClassTest struct {
	name   string
	script string
	class  ScriptClass
}

// scriptClassTests houses several test scripts used to ensure various class
// determination is working as expected.  It's defined as a test global versus
// inside a function scope since this spans both the standard tests and the
// consensus tests (pay-to-script-hash is part of consensus).

// TestScriptClass ensures all the scripts in scriptClassTests have the expected
// class.
func TestScriptClass(t *testing.T) {
	t.Parallel()
	var scriptClassTests = []scriptClassTest{
		{
			name: "multisig",
			script: "1 DATA_33 0x0232abdc893e7f0631364d7fd01cb33d24da4" +
				"5329a00357b3a7886211ab414d55a 1 CHECKMULTISIG",
			class: MultiSigTy,
		},
		//tx e5779b9e78f9650debc2893fd9636d827b26b4ddfa6a8172fe8708c924f5c39d
		{
			name: "P2WSH",
			script: "OP_0 DATA_32 0x010203040506070801020304050607080" +
				"1020304050607080102030405060708",
			class: WitnessV0ScriptHashTy,
		},
		{
			name: "P2WSH-staking",
			script: "OP_0 DATA_32 0x010203040506070801020304050607080" +
				"1020304050607080102030405060708 DATA_8 0x3412080934526719",
			class: StakingScriptHashTy,
		},
		{
			name: "P2WSH-binding",
			script: "OP_0 DATA_32 0x010203040506070801020304050607080" +
				"1020304050607080102030405060708 DATA_20 " +
				"0x433ec2ac1ffa1b7b7d027f564529c57197f9ae88",
			class: BindingScriptHashTy,
		},
		{
			// Nulldata with no data at all.
			name:   "nulldata",
			script: "RETURN",
			class:  NullDataTy,
		},
		{
			// Nulldata with small data.
			name:   "nulldata2",
			script: "RETURN DATA_8 0x046708afdb0fe554",
			class:  NullDataTy,
		},
		{
			// Nulldata with max allowed data.
			name: "nulldata3",
			script: "RETURN PUSHDATA1 0x50 0x046708afdb0fe5548271967f1a67" +
				"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
				"046708afdb0fe5548271967f1a67130b7105cd6a828e03909a67" +
				"962e0ea1f61deb649f6bc3f4cef3",
			class: NullDataTy,
		},
		{
			// Nulldata with more than max allowed data (so therefore
			// nonstandard)
			name: "nulldata4",
			script: "RETURN PUSHDATA1 0x51 0x046708afdb0fe5548271967f1a67" +
				"130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3" +
				"046708afdb0fe5548271967f1a67130b7105cd6a828e03909a67" +
				"962e0ea1f61deb649f6bc3f4cef308",
			class: NonStandardTy,
		},
		{
			// Almost nulldata, but add an additional opcode after the data
			// to make it nonstandard.
			name:   "nulldata5",
			script: "RETURN 4 TRUE",
			class:  NonStandardTy,
		},

		{
			name:   "doesn't parse",
			script: "DATA_5 0x01020304",
			class:  NonStandardTy,
		},
	}
	for _, test := range scriptClassTests {
		t.Run(test.name, func(t *testing.T) {
			script := mustParseShortForm(test.script)
			class := GetScriptClass(script)
			if class != test.class {
				t.Errorf("%s: expected %s got %s", test.name,
					test.class, class)
				return
			}
		})
	}
}

// TestStringifyClass ensures the script class string returns the expected
// string for each script class.
func TestStringifyClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		class    ScriptClass
		stringed string
	}{
		{
			name:     "nonstandardty",
			class:    NonStandardTy,
			stringed: "nonstandard",
		},
		{
			name:     "witness_v0_scripthashty",
			class:    WitnessV0ScriptHashTy,
			stringed: "witness_v0_scripthash",
		},
		{
			name:     "staking_scripthashty",
			class:    StakingScriptHashTy,
			stringed: "staking_scripthash",
		},
		{
			name:     "binding_scripthashty",
			class:    BindingScriptHashTy,
			stringed: "binding_scripthash",
		},
		{
			name:     "multisigty",
			class:    MultiSigTy,
			stringed: "multisig",
		},
		{
			name:     "nulldataty",
			class:    NullDataTy,
			stringed: "nulldata",
		},
		{
			name:     "broken",
			class:    ScriptClass(255),
			stringed: "Invalid",
		},
	}

	for _, test := range tests {
		typeString := test.class.String()
		if typeString != test.stringed {
			t.Errorf("%s: got %#q, want %#q", test.name,
				typeString, test.stringed)
		}
	}
}

// newAddressPubKey returns a new massutil.AddressPubKey from the provided
// serialized public key.  It panics if an error occurs.  This is only used in
// the tests as a helper since the only way it can fail is if there is an error
// in the test source code.
func newAddressPubKey(serializedPubKey []byte) massutil.Address {
	addr, err := massutil.NewAddressPubKey(serializedPubKey,
		&config.ChainParams)
	if err != nil {
		panic("invalid public key in test source")
	}

	return addr
}

func newAddressWitnessScriptHash(scriptHash []byte) massutil.Address {
	addr, err := massutil.NewAddressWitnessScriptHash(scriptHash, &config.ChainParams)
	if err != nil {
		panic("invalid script hash in test source")
	}

	return addr
}

func TestPayToStakingAddrScript(t *testing.T) {
	encodedAddr := "ms1qpqypqxpq9qcrssqgzqvzq2ps8pqqsyqcyq5rqwzqpqgpsgpgxquyqjyww3n"
	addr, err := massutil.DecodeAddress(encodedAddr,
		&config.ChainParams)
	if err != nil {
		t.Error(err)
	}

	if !massutil.IsWitnessStakingAddress(addr) {
		t.Error("not a staking address")
		t.FailNow()
	}

	pkScript, err := PayToStakingAddrScript(addr, consensus.MinFrozenPeriod)
	assert.Nil(t, err)
	t.Log(len(pkScript))
	class, pops := GetScriptInfo(pkScript)
	t.Log("class: ", class, len(pops))

	for i, val := range pops {
		t.Log(i, val)
		data, _ := val.bytes()
		t.Log(val.opcode, data, len(data))
	}
}

func ExampleWitnessScript() {
	var address massutil.Address
	var err error

	redeemScriptHash := []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}
	pocPubKeyHash := []byte{7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2}

	// witness v0
	address, err = massutil.NewAddressWitnessScriptHash(redeemScriptHash, &config.ChainParams)
	if err != nil {
		panic(err)
	}
	pkScript, err := PayToWitnessScriptHashScript(redeemScriptHash)
	if err != nil {
		panic(err)
	}
	fmt.Println("witV0", address.EncodeAddress(), len(pkScript))

	// witness staking
	address, err = massutil.NewAddressStakingScriptHash(redeemScriptHash, &config.ChainParams)
	if err != nil {
		panic(err)
	}
	pkScript, err = PayToStakingAddrScript(address, consensus.MinFrozenPeriod)
	if err != nil {
		panic(err)
	}
	fmt.Println("staking", address.EncodeAddress(), len(pkScript))

	// witness binding
	address, err = massutil.NewAddressPubKeyHash(pocPubKeyHash, &config.ChainParams)
	if err != nil {
		panic(err)
	}
	pkScript, err = PayToBindingScriptHashScript(redeemScriptHash, pocPubKeyHash)
	if err != nil {
		panic(err)
	}
	fmt.Println("binding", address.EncodeAddress(), len(pkScript))

	// Output:
	// witV0 ms1qqqypqxpq9qcrssqgzqvzq2ps8pqqsyqcyq5rqwzqpqgpsgpgxquyqd07tvd 34
	// staking ms1qpqypqxpq9qcrssqgzqvzq2ps8pqqsyqcyq5rqwzqpqgpsgpgxquyqjyww3n 43
	// binding 1eBKU2y19PUERmytBTyUUGiK7GeJDqq42 55
}
