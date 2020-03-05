package ldb

import "massnet.org/mass/errors"

var (
	ErrWrongScriptHashLength = errors.New("length of script hash error")
	ErrBindingIndexBroken    = errors.New("binding transaction index was broken")

	// errors for submit.go
	ErrPreBatchNotReady    = errors.New("previous batch is not ready")
	ErrUnrelatedBatch      = errors.New("unrelated batch to be submitted")
	ErrCommitHashNotEqual  = errors.New("commit hash is not equal to batch hash")
	ErrCommitBatchNotReady = errors.New("commit batch is not ready")

	// errors for binding transaction index
	ErrWrongBindingTxIndexLen         = errors.New("length of binding tx index is invalid")
	ErrWrongBindingTxIndexPrefix      = errors.New("prefix of binding tx index is invalid")
	ErrWrongBindingShIndexLen         = errors.New("length of binding sh index is invalid")
	ErrWrongBindingShIndexPrefix      = errors.New("prefix of binding sh index is invalid")
	ErrWrongBindingTxSpentIndexLen    = errors.New("length of binding tx spent index is invalid")
	ErrWrongBindingTxSpentIndexPrefix = errors.New("prefix of binding tx spent index is invalid")
)
