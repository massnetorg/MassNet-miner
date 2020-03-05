package database

import (
	"crypto/sha256"
	"errors"

	"golang.org/x/crypto/ripemd160"

	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

// Errors that the various database functions may return.
var (
	ErrAddrIndexDoesNotNeedInit = errors.New("address index has been initialized")
	ErrAddrIndexDoesNotExist    = errors.New("address index hasn't been built or is an older version")
	ErrAddrIndexVersionNotFound = errors.New("address index version not found")
	ErrAddrIndexInvalidVersion  = errors.New("address index version is not allowed")
	ErrUnsupportedAddressType   = errors.New("address type is not supported by the address-index")
	ErrPrevShaMissing           = errors.New("previous sha missing from database")
	ErrTxShaMissing             = errors.New("requested transaction does not exist")
	ErrBlockShaMissing          = errors.New("requested block does not exist")
	ErrDuplicateSha             = errors.New("duplicate insert attempted")
	ErrDbDoesNotExist           = errors.New("non-existent database")
	ErrDbUnknownType            = errors.New("non-existent database type")
	ErrNotImplemented           = errors.New("method has not yet been implemented")
	ErrInvalidBlockStorageMeta  = errors.New("invalid block storage meta")
	ErrInvalidAddrIndexMeta     = errors.New("invalid addr index meta")
	ErrDeleteNonNewestBlock     = errors.New("delete block that is not newest")
)

// Db defines a generic interface that is used to request and insert data into
// the mass block chain.  This interface is intended to be agnostic to actual
// mechanism used for backend data storage.  The AddDBDriver function can be
// used to add a new backend data storage method.
type Db interface {
	// Close cleanly shuts down the database and syncs all data.
	Close() (err error)

	// RollbackClose discards the recent database changes to the previously
	// saved data at last Sync and closes the database.
	RollbackClose() (err error)

	// Sync verifies that the database is coherent on disk and no
	// outstanding transactions are in flight.
	Sync() (err error)

	// Commit commits batches in a single transaction
	Commit(hash wire.Hash) error

	// InitByGenesisBlock init database by setting genesis block
	InitByGenesisBlock(block *massutil.Block) (err error)

	// InsertBlock inserts raw block and transaction data from a block
	// into the database.  The first block inserted into the database
	// will be treated as the genesis block.  Every subsequent block insert
	// requires the referenced parent block to already exist.
	SubmitBlock(block *massutil.Block) (err error)

	// DeleteBlock will remove any blocks from the database after
	// the given block.  It terminates any existing transaction and performs
	// its operations in an atomic transaction which is committed before
	// the function returns.
	DeleteBlock(hash *wire.Hash) (err error)

	// ExistsSha returns whether or not the given block hash is present in
	// the database.
	ExistsSha(sha *wire.Hash) (exists bool, err error)

	// FetchBlockBySha returns a massutil Block.  The implementation may
	// cache the underlying data if desired.
	FetchBlockBySha(sha *wire.Hash) (blk *massutil.Block, err error)

	// FetchBlockHeightBySha returns the block height for the given hash.
	FetchBlockHeightBySha(sha *wire.Hash) (height uint64, err error)

	// FetchBlockHeaderBySha returns a wire.BlockHeader for the given
	// sha.  The implementation may cache the underlying data if desired.
	FetchBlockHeaderBySha(sha *wire.Hash) (bh *wire.BlockHeader, err error)

	// FetchBlockShaByHeight returns a block hash based on its height in the
	// block chain.
	FetchBlockShaByHeight(height uint64) (sha *wire.Hash, err error)

	// ExistsTxSha returns whether or not the given tx hash is present in
	// the database
	ExistsTxSha(sha *wire.Hash) (exists bool, err error)

	FetchTxByLoc(blkHeight uint64, txOff int, txLen int) (*wire.MsgTx, error)

	// FetchTxBySha returns some data for the given transaction hash. The
	// implementation may cache the underlying data if desired.
	FetchTxBySha(txsha *wire.Hash) ([]*TxReply, error)

	// GetTxData returns the block height, txOffset, txLen for the given transaction hash,
	// including unspent and fully spent transaction
	GetUnspentTxData(txsha *wire.Hash) (uint64, int, int, error)

	// FetchTxByShaList returns a TxReply given an array of transaction
	// hashes.  The implementation may cache the underlying data if desired.
	// This differs from FetchUnSpentTxByShaList in that it will return
	// the most recent known Tx, if it is fully spent or not.
	//
	// NOTE: This function does not return an error directly since it MUST
	// return at least one TxReply instance for each requested
	// transaction.  Each TxReply instance then contains an Err field
	// which can be used to detect errors.
	FetchTxByShaList(txShaList []*wire.Hash) []*TxReply

	// Returns database.ErrTxShaMissing if no transaction found
	FetchLastFullySpentTxBeforeHeight(txsha *wire.Hash, height uint64) (tx *wire.MsgTx, blkheight uint64, blksha *wire.Hash, err error)

	// FetchUnSpentTxByShaList returns a TxReply given an array of
	// transaction hashes.  The implementation may cache the underlying
	// data if desired. Fully spent transactions will not normally not
	// be returned in this operation.
	//
	// NOTE: This function does not return an error directly since it MUST
	// return at least one TxReply instance for each requested
	// transaction.  Each TxReply instance then contains an Err field
	// which can be used to detect errors.
	FetchUnSpentTxByShaList(txShaList []*wire.Hash) []*TxReply

	// fetch a rank of reward addresses
	FetchRankStakingTx(height uint64) ([]Rank, error)

	// fetch detail info of reward adresses
	FetchRewardStakingTx(height uint64) ([]Reward, uint32, error)

	// fetch detail info of all staking tx
	FetchInStakingTx(height uint64) ([]Reward, uint32, error)

	// fetch a map of all staking transactions in database
	FetchStakingTxMap() (StakingTxMapReply, error)

	FetchExpiredStakingTxListByHeight(height uint64) (StakingTxMapReply, error)

	FetchHeightRange(startHeight, endHeight uint64) ([]wire.Hash, error)

	// NewestSha returns the hash and block height of the most recent (end)
	// block of the block chain.  It will return the zero hash, -1 for
	// the block height, and no error (nil) if there are not any blocks in
	// the database yet.
	NewestSha() (sha *wire.Hash, height uint64, err error)

	// FetchFaultPkBySha returns the FaultPubKey by Hash of that PubKey.
	// Hash = DoubleHashB(PubKey.SerializeUnCompressed()).
	// It will return ErrNotFound if this PubKey is not banned.
	FetchFaultPkBySha(sha *wire.Hash) (fpk *wire.FaultPubKey, height uint64, err error)

	FetchAllFaultPks() ([]*wire.FaultPubKey, []uint64, error)

	// FetchFaultPkListByHeight returns the newly added FaultPubKey List
	// on given height. It will return ErrNotFound if this height has not
	// mined. And will return empty slice if there aren't any new banned Pks.
	FetchFaultPkListByHeight(blkHeight uint64) ([]*wire.FaultPubKey, error)

	// ExistsFaultPk returns whether or not the given FaultPubKey hash is present in
	// the database.
	ExistsFaultPk(sha *wire.Hash) (bool, error)

	// FetchAllPunishment returns all faultPubKey stored in db, with random order.
	FetchAllPunishment() ([]*wire.FaultPubKey, error)

	// ExistsPunishment returns whether or not the given PoC PublicKey is present in
	// the database.
	ExistsPunishment(pk *pocec.PublicKey) (bool, error)

	// InsertPunishment insert a fpk into punishment storage instantly.
	InsertPunishment(fpk *wire.FaultPubKey) error

	FetchMinedBlocks(pubKey *pocec.PublicKey) ([]uint64, error)

	// FetchAddrIndexTip returns the hash and block height of the most recent
	// block which has had its address index populated. It will return
	// ErrAddrIndexDoesNotExist along with a zero hash, and math.MaxUint64 if
	// it hasn't yet been built up.
	FetchAddrIndexTip() (sha *wire.Hash, height uint64, err error)

	SubmitAddrIndex(hash *wire.Hash, height uint64, addrIndexData *AddrIndexData) (err error)

	DeleteAddrIndex(hash *wire.Hash, height uint64) (err error)

	// FetchScriptHashRelatedTx  returns all relevant txhash mapped by block height
	FetchScriptHashRelatedTx(scriptHashes [][]byte, startBlock, stopBlock uint64, chainParams *config.Params) (map[uint64][]*wire.TxLoc, error)

	CheckScriptHashUsed(scriptHash []byte) (bool, error)

	FetchScriptHashRelatedBindingTx(scriptHash []byte, chainParams *config.Params) ([]*BindingTxReply, error)

	// For testing purpose
	TestExportDbEntries() map[string][]byte
}

// DriverDB defines a structure for backend drivers to use when they registered
// themselves as a backend which implements the Db interface.
type DriverDB struct {
	DbType   string
	CreateDB func(args ...interface{}) (pbdb Db, err error)
	OpenDB   func(args ...interface{}) (pbdb Db, err error)
}

// driverList holds all of the registered database backends.
var driverList []DriverDB

// AddDBDriver adds a back end database driver to available interfaces.
func AddDBDriver(instance DriverDB) {
	for _, drv := range driverList {
		if drv.DbType == instance.DbType {
			return
		}
	}
	driverList = append(driverList, instance)
}

// CreateDB intializes and opens a database.
func CreateDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.CreateDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// OpenDB opens an existing database.
func OpenDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.OpenDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// SupportedDBs returns a slice of strings that represent the database drivers
// that have been registered and are therefore supported.
func SupportedDBs() []string {
	var supportedDBs []string
	for _, drv := range driverList {
		supportedDBs = append(supportedDBs, drv.DbType)
	}
	return supportedDBs
}

// TxReply is used to return individual transaction information when
// data about multiple transactions is requested in a single call.
type TxReply struct {
	Sha     *wire.Hash
	Tx      *wire.MsgTx
	BlkSha  *wire.Hash
	Height  uint64
	TxSpent []bool
	Err     error
}

type UtxoReply struct {
	TxSha    *wire.Hash
	Height   uint64
	Coinbase bool
	Index    uint32
	Value    massutil.Amount
}

// AddrIndexKeySize is the number of bytes used by keys into the BlockAddrIndex.
// tianwei.cai: type + addr
const AddrIndexKeySize = ripemd160.Size + 1

type AddrIndexOutPoint struct {
	TxLoc *wire.TxLoc
	Index uint32
}

type BindingTxSpent struct {
	SpentTxLoc   *wire.TxLoc
	BTxBlkHeight uint64
	BTxLoc       *wire.TxLoc
	BTxIndex     uint32
}

type BindingTxReply struct {
	Height     uint64
	TxSha      *wire.Hash
	IsCoinbase bool
	Value      int64
	Index      uint32
}

// stakingTx
type StakingTxMapReply map[[sha256.Size]byte]map[wire.OutPoint]StakingTxInfo

// BlockAddrIndex represents the indexing structure for addresses.
// It maps a hash160 to a list of transaction locations within a block that
// either pays to or spends from the passed UTXO for the hash160.
type BlockAddrIndex map[[AddrIndexKeySize]byte][]*AddrIndexOutPoint

// BlockTxAddrIndex represents the indexing structure for txs
type TxAddrIndex map[[sha256.Size]byte][]*wire.TxLoc

type BindingTxAddrIndex map[[ripemd160.Size]byte][]*AddrIndexOutPoint

type BindingTxSpentAddrIndex map[[ripemd160.Size]byte][]*BindingTxSpent

type AddrIndexData struct {
	TxIndex             TxAddrIndex
	BindingTxIndex      BindingTxAddrIndex
	BindingTxSpentIndex BindingTxSpentAddrIndex
}
