package capacity

import "errors"

var (
	ErrWorkSpaceDoesNotExist    = errors.New("non-existent workSpace")
	ErrWorkSpaceIsNotRegistered = errors.New("non-registered workSpace")
	ErrWorkSpaceIsNotPlotting   = errors.New("non-plotting workSpace")
	ErrWorkSpaceIsNotReady      = errors.New("non-ready workSpace")
	ErrWorkSpaceIsNotMining     = errors.New("non-mining workSpace")
	ErrWorkSpaceIsNotStill      = errors.New("non-registered or non-ready workSpace")
	ErrWorkSpaceCannotGenerate  = errors.New("not allowed to generate new workSpace")

	ErrMassDBWrongFileName        = errors.New("db file name not standard")
	ErrMassDBDuplicate            = errors.New("db file duplicate in root dirs")
	ErrMassDBDoesNotMatchWithName = errors.New("db file content does not match with name")

	ErrWalletDoesNotContainPubKey = errors.New("wallet does not contain pubKey")
	ErrWalletIsLocked             = errors.New("wallet is locked")

	ErrSpaceKeeperIsRunning         = errors.New("spaceKeeper is running")
	ErrSpaceKeeperIsNotRunning      = errors.New("spaceKeeper is not running")
	ErrSpaceKeeperIsConfiguring     = errors.New("spaceKeeper is configuring")
	ErrSpaceKeeperConfiguredNothing = errors.New("configured nothing for spaceKeeper")
	ErrSpaceKeeperChangeDBDirs      = errors.New("cannot change dbDirs")

	ErrConfigUnderSizeTarget = errors.New("target disk size is smaller than lower bound")
	ErrOSDiskSizeNotEnough   = errors.New("os disk size is not enough")
	ErrInvalidRequiredBytes  = errors.New("required disk size in bytes is not valid")
	ErrConfigInvalidPathSize = errors.New("target path and size is not matched")
	ErrInvalidDir            = errors.New("invalid directory")
)
