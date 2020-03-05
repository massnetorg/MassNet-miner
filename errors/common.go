package errors

import "errors"

var (
	// Blockchain
	ErrTxAlreadyExists = errors.New("transaction already exists")
)
