// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

const (
	// LockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.  However, if the field is interpreted as a timestamp, given
	// the lock time is a uint32, the max is sometime around 2106.
	LockTimeThreshold uint64 = 5e8 // Tue Nov 5 00:53:20 1985 UTC
)

// ScriptFlags is a bitmask defining additional operations or tests that will be
// done when executing a script pair.
type ScriptFlags uint32

const (
	// ScriptDiscourageUpgradableNops defines whether to verify that
	// NOP1 through NOP10 are reserved for future soft-fork upgrades.  This
	// flag must not be used for consensus critical code nor applied to
	// blocks as this flag is only for stricter standard transaction
	// checks.  This flag is only applied when the above opcodes are
	// executed.
	ScriptDiscourageUpgradableNops ScriptFlags = 1 << iota
)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType uint32

// Hash type bits from the end of a signature.
const (
	// SigHashOld          SigHashType = 0x0
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
)

// These are the constants specified for maximums in individual scripts.
const (
	// maxStackSize is the maximum combined height of stack and alt stack
	// during execution.
	maxStackSize = 1000

	// maxScriptSize is the maximum allowed length of a raw script.
	maxScriptSize = 10000

	// // payToWitnessScriptHashDataSize is the size of the witness program's
	// // data push for a pay-to-witness-script-hash output.
	// payToWitnessScriptHashDataSize = 20

	MaxOpsPerScript       = 201 // Max number of non-push operations.
	MaxPubKeysPerMultiSig = 20  // Multisig can't have more sigs than this.
	MaxScriptElementSize  = 520 // Max bytes pushable to the stack.

	// defaultScriptAlloc is the default size used for the backing array
	// for a script being built by the ScriptBuilder.  The array will
	// dynamically grow as needed, but this figure is intended to provide
	// enough space for vast majority of scripts without needing to grow the
	// backing array multiple times.
	defaultScriptAlloc = 500

	maxInt32 = 1<<31 - 1
	minInt32 = -1 << 31

	// defaultScriptNumLen is the default number of bytes
	// data being interpreted as an integer may be.
	defaultScriptNumLen = 4

	// MaxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data to be considered a nulldata transaction
	MaxDataCarrierSize = 80
)
const (
	// StandardVerifyFlags are the script flags which are used when
	// executing transaction scripts to enforce additional checks which
	// are required for the script to be considered standard.  These checks
	// help reduce issues related to transaction malleability as well as
	// allow pay-to-script hash transactions.  Note these flags are
	// different than what is required for the consensus rules in that they
	// are more strict.
	//
	// TODO: This definition does not belong here.  It belongs in a policy
	StandardVerifyFlags = ScriptDiscourageUpgradableNops
)

// ScriptClass is an enumeration for the list of standard types of script.
type ScriptClass byte

// Classes of script payment known about in the blockchain.
const (
	NonStandardTy         ScriptClass = iota // None of the recognized forms.
	WitnessV0ScriptHashTy                    // Pay to witness script hash.
	StakingScriptHashTy                      // Pay to staking script hash.
	BindingScriptHashTy                      //pay to binding script hash
	MultiSigTy                               // Multi signature.
	NullDataTy                               // Empty data-only (provably prunable).
)

const (
	WitnessV0ScriptHashDataSize    = 32 // Length of witness script hash
	WitnessV0PoCPubKeyHashDataSize = 20 // Length of poc public key hash
	WitnessV0FrozenPeriodDataSize  = 8  // Length of byte slice of frozen period
)

// scriptClassToName houses the human-readable strings which describe each
// script class.
var scriptClassToName = []string{
	NonStandardTy:         "nonstandard",
	MultiSigTy:            "multisig",
	NullDataTy:            "nulldata",
	WitnessV0ScriptHashTy: "witness_v0_scripthash",
	StakingScriptHashTy:   "staking_scripthash",
	BindingScriptHashTy:   "binding_scripthash",
}
