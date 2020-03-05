package memdb

import "errors"

// Errors that the various database functions may return.
var (
	ErrDbClosed = errors.New("database is closed")
)
