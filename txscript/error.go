// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"fmt"
)

var (
	// ErrStackShortScript is returned if the script has an opcode that is
	// too long for the length of the script.
	ErrStackShortScript = errors.New("execute past end of script")

	// ErrStackLongScript is returned if the script has an opcode that is
	// too long for the length of the script.
	ErrStackLongScript = errors.New("script is longer than maximum allowed")

	// ErrStackUnderflow is returned if an opcode requires more items on the
	// stack than is present.f
	ErrStackUnderflow = errors.New("stack underflow")

	// ErrStackInvalidArgs is returned if the argument for an opcode is out
	// of acceptable range.
	ErrStackInvalidArgs = errors.New("invalid argument")

	// ErrStackOpDisabled is returned when a disabled opcode is encountered
	// in the script.
	ErrStackOpDisabled = errors.New("disabled opcode")

	// ErrStackVerifyFailed is returned when one of the OP_VERIFY or
	// OP_*VERIFY instructions is executed and the conditions fails.
	ErrStackVerifyFailed = errors.New("verify failed")

	// ErrStackNumberTooBig is returned when the argument for an opcode that
	// should be an offset is obviously far too large.
	ErrStackNumberTooBig = errors.New("number too big")

	// ErrStackInvalidOpcode is returned when an opcode marked as invalid or
	// a completely undefined opcode is encountered.
	ErrStackInvalidOpcode = errors.New("invalid opcode")

	// ErrStackReservedOpcode is returned when an opcode marked as reserved
	// is encountered.
	ErrStackReservedOpcode = errors.New("reserved opcode")

	// ErrTooManyOperations is returned if an OP_CHECKMULTISIG is
	// encountered with more than MaxOpsPerScript OpCodes present.
	ErrTooManyOperations = errors.New("too many operations")

	// ErrStackEarlyReturn is returned when OP_RETURN is executed in the
	// script.
	ErrStackEarlyReturn = errors.New("script returned early")

	// ErrStackNoIf is returned if an OP_ELSE or OP_ENDIF is encountered
	// without first having an OP_IF or OP_NOTIF in the script.
	ErrStackNoIf = errors.New("OP_ELSE or OP_ENDIF with no matching OP_IF")

	// ErrStackMissingEndif is returned if the end of a script is reached
	// without and OP_ENDIF to correspond to a conditional expression.
	ErrStackMissingEndif = fmt.Errorf("execute fail, in conditional execution")

	// ErrStackTooManyPubKeys is returned if an OP_CHECKMULTISIG is
	// encountered with more than MaxPubKeysPerMultiSig pubkeys present.
	ErrStackTooManyPubKeys = errors.New("invalid pubkey count in OP_CHECKMULTISIG")

	// ErrStackTooManyOperations is returned if a script has more than
	// MaxOpsPerScript opcodes that do not push data.
	ErrStackTooManyOperations = errors.New("too many operations in script")

	// ErrStackElementTooBig is returned if the size of an element to be
	// pushed to the stack is over MaxScriptElementSize.
	ErrStackElementTooBig = errors.New("element in script too large")

	// ErrStackUnknownAddress is returned when ScriptToAddrHash does not
	// recognise the pattern of the script and thus can not find the address
	// for payment.
	ErrStackUnknownAddress = errors.New("non-recognised address")

	// ErrStackScriptFailed is returned when at the end of a script the
	// boolean on top of the stack is false signifying that the script has
	// failed.
	ErrStackScriptFailed = errors.New("execute fail, fail on stack")

	// ErrStackScriptUnfinished is returned when CheckErrorCondition is
	// called on a script that has not finished executing.
	ErrStackScriptUnfinished = errors.New("error check when script unfinished")

	// ErrStackEmptyStack is returned when the stack is empty at the end of
	// execution. Normal operation requires that a boolean is on top of the
	// stack when the scripts have finished executing.
	ErrStackEmptyStack = errors.New("stack empty at end of execution")

	// ErrStackP2SHNonPushOnly is returned when a Pay-to-Script-Hash
	// transaction is encountered and the ScriptSig does operations other
	// than push data (in violation of bip16).
	ErrStackP2SHNonPushOnly = errors.New("pay to script hash with non " +
		"pushonly input")

	// ErrStackInvalidParseType is an internal error returned from
	// ScriptToAddrHash ony if the internal data tables are wrong.
	ErrStackInvalidParseType = errors.New("internal error: invalid parsetype found")

	// ErrStackInvalidAddrOffset is an internal error returned from
	// ScriptToAddrHash ony if the internal data tables are wrong.
	ErrStackInvalidAddrOffset = errors.New("internal error: invalid offset found")

	// ErrStackInvalidIndex is returned when an out-of-bounds index was
	// passed to a function.
	ErrStackInvalidIndex = errors.New("invalid script index")

	// ErrStackNonPushOnly is returned when ScriptInfo is called with a
	// pkScript that peforms operations other that pushing data to the stack.
	ErrStackNonPushOnly = errors.New("SigScript is non pushonly")

	// ErrStackOverflow is returned when stack and altstack combined depth
	// is over the limit.
	ErrStackOverflow = errors.New("stack overflow")

	// ErrStackInvalidLowSSignature is returned when the ScriptVerifyLowS
	// flag is set and the script contains any signatures whose S values
	// are higher than the half order.
	ErrStackInvalidLowSSignature = errors.New("invalid low s signature")

	// ErrStackInvalidPubKey is returned when the ScriptVerifyScriptEncoding
	// flag is set and the script contains invalid pubkeys.
	ErrStackInvalidPubKey = errors.New("invalid strict pubkey")

	// ErrStackCleanStack is returned when the ScriptVerifyCleanStack flag
	// is set and after evalution the stack does not contain only one element,
	// which also must be true if interpreted as a boolean.
	ErrStackCleanStack = errors.New("stack is not clean")

	// ErrStackMinimalData is returned when the ScriptVerifyMinimalData flag
	// is set and the script contains push operations that do not use
	// the minimal opcode required.
	ErrStackMinimalData = errors.New("non-minimally encoded script number")

	ErrWitnessProgramEmpty = errors.New("witness program is empty")

	ErrWitnessProgramMismatch = errors.New("witness program hash mismatch")

	ErrWitnessPubKeyType = errors.New("only uncompressed keys are accepted post-segwit")

	ErrWitnessProgramLength = errors.New("incorrect witness program length")

	ErrWitnessExtProgramLength = errors.New("incorrect witness extension program length")

	ErrDiscourageUpgradableWitnessProgram = errors.New("version of witness program is invalid")

	ErrInvalidFlags = errors.New("P2SH must be enabled to do witness verification")

	ErrWitnessUnexpected = errors.New("non-witness inputs cannot have a witness")

	ErrWitnessMalleated = errors.New("native witness program cannot also have a signature script")

	ErrWitnessLength = errors.New("invalid witness length")

	ErrwitnessProgramFormat = errors.New("script is not a witness program, unable to extract version or witness program")

	ErrUnsupportedAddress = errors.New("unsupport address")
	// ErrBadNumRequired is returned from MultiSigScript when nrequired is
	// larger than the number of provided public keys.
	ErrBadNumRequired = errors.New("more signatures required than keys present")

	ErrNullFail = errors.New("not all signatures empty on failed checkmultisig")

	ErrInvalidBindingScript = errors.New("input script is not a binding script")

	ErrBindingUnexpected = errors.New("null binding script")

	ErrFrozenPeriod = errors.New("invalid frozen period")

	ErrScriptTooBig = errors.New("script size is too large")

	ErrWitnessExtProgUnknown = errors.New("unknown witness extension program")
)
