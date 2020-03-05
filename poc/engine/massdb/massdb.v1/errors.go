package massdb_v1

import (
	"errors"
)

var (
	ErrDBWrongFileSize   = errors.New("db file size is invalid")
	ErrDBWrongFileCode   = errors.New("db file code not matched")
	ErrDBWrongVersion    = errors.New("db version not allowed")
	ErrDBWrongType       = errors.New("db type not allowed")
	ErrDBWrongPubKeyHash = errors.New("db pubKey hash is not matched with pubKey")
	ErrDBWrongMapType    = errors.New("db mapType is not valid")

	ErrAlreadyPlotting = errors.New("db already been plotting")
	ErrStopPlotting    = errors.New("db stop plotting")
	ErrMemoryNotEnough = errors.New("memory is not enough")
)
