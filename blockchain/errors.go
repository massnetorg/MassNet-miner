package blockchain

import "errors"

var (
	// BlockChain
	errIndexAlreadyInitialized = errors.New("the block index can only be initialized before it has been modified")
	errConnectMainChain        = errors.New("connectBlock must be called with a block that extends the main chain")
	errDisconnectMainChain     = errors.New("disconnectBlock must be called with the block at the end of the main chain")
	errWaitForOldBlockHeight   = errors.New("blockWaiter wait for old block height")

	// BlockTree
	errExpandOrphanRootBlockNode = errors.New("can not expand orphan block on root of blockTree")
	errExpandChildRootBlockNode  = errors.New("can not expand child block on root of blockTree")
	errRemoveAllRootBlockNode    = errors.New("can not remove all nodes on blockTree")
	errAttachNonLeafBlockNode    = errors.New("can not attach non-leaf block to blockTree")
	errDetachParentBlockNode     = errors.New("can not detach parent block from blockTree")
	errRootNodeAlreadyExists     = errors.New("can not set root node multiple times")

	// DoubleMiningDetector
	errInvalidDmdTree = errors.New("invalid double-mining-detector tree")

	// ProposalPool
	errFaultPubKeyGetBlockHeader = errors.New("fail to create new faultPubKey due to header data lost")

	// Block Validate
	errUnexpectedHeight          = errors.New("illegal block for AddrIndexer, unexpected block height")
	errIncompleteCoinbasePayload = errors.New("size of coinbase payload less than block height need")
	errBlockNoTransactions       = errors.New("block does not contain any transactions")
	errBlockCacheNotExists       = errors.New("required block cache not exists")
	ErrInvalidTime               = errors.New("block timestamp has a higher precsion the one second")
	ErrTimeTooOld                = errors.New("block timestamp is not after expected prevNode")
	ErrTimeTooNew                = errors.New("block timestamp of uinx is too far in the future")
	ErrUnexpectedDifficulty      = errors.New("block difficulty is not the expected value")
	ErrInvalidMerkleRoot         = errors.New("block transaction merkle root is invalid")
	ErrInvalidProposalRoot       = errors.New("block proposal merkle root is invalid")
	ErrTooManyTransactions       = errors.New("block contains too many transactions")
	ErrNoTxOutputs               = errors.New("transaction has no outputs")
	ErrDuplicateTxInputs         = errors.New("transaction contains duplicate inputs")
	ErrConnectGenesis            = errors.New("genesis block should not be connected")
	ErrOverwriteTx               = errors.New("tried to overwrite not fully spent transaction")
	ErrTooManySigOps             = errors.New("too many signature operations")
	ErrFirstTxNotCoinbase        = errors.New("first transaction in block is not a coinbase")
	ErrMultipleCoinbases         = errors.New("block contains other coinbase")
	ErrLowQuality                = errors.New("block's proof quality is lower than expected min target")
	ErrTimestampFormat           = errors.New("wrong timestamp format")
	ErrBadBlockHeight            = errors.New("block height does not match the expected height")
	ErrChainID                   = errors.New("block's chainID is not equal to expected chainID")
	ErrBlockSIG                  = errors.New("block signature verify failed")
	ErrInvalidBlockVersion       = errors.New("block's version is not valid")
	ErrInvalidBitLength          = errors.New("block's bitLength is smaller than the last submission of this public key")

	// BanList
	ErrBannedPk      = errors.New("block builder puKey has been banned")
	ErrBanSelfPk     = errors.New("block's pubKey is banned in header banList")
	ErrBanList       = errors.New("invalid banList")
	ErrCheckBannedPk = errors.New("invalid faultPk")

	// TxPool
	ErrTxPoolNil           = errors.New("the txpool is nil")
	ErrTxExsit             = errors.New("transaction is not in the pool")
	ErrFindTxByAddr        = errors.New("address does not have any transactions in the pool")
	ErrCoinbaseTx          = errors.New("transaction is an individual coinbase")
	ErrProhibitionOrphanTx = errors.New("Do not accept orphan transactions")
	ErrInvalidTxVersion    = errors.New("transaction version is invalid")

	// Coinbase
	ErrCoinbaseTxInWitness = errors.New("coinbaseTx txIn`s witness size must be 0")
	ErrBadCoinbaseValue    = errors.New("coinbase transaction for block pays is more than expected value")
	ErrBadCoinbaseHeight   = errors.New("the coinbase payload serialized block height does not equal expected height")
	ErrCoinbaseOutputValue = errors.New("the coinbaseTx output value is not correct")
	ErrCoinbaseOutputNum   = errors.New("incorrect coinbase output number")

	// StakingTx & BindingTx
	ErrStandardBindingTx     = errors.New("input and output of the transaction are all binding transactions")
	ErrInvalidStakingTxValue = errors.New("invalid staking tx value")
	ErrInvalidFrozenPeriod   = errors.New("invalid frozen period")
	ErrStakingRewardNum      = errors.New("incorrect staking reward number")
	ErrBindingPubKey         = errors.New("binding pubkey does not match miner pubkey")
	ErrBindingInputMissing   = errors.New("input of binding missing")
	ErrDuplicateStaking      = errors.New("duplicate staking")

	// TxIn
	ErrFindReferenceInput = errors.New("unable find reference transaction ")
	ErrBadTxInput         = errors.New("transaction input refers to previous output is invalid")
	ErrMissingTx          = errors.New("unable to find input transaction")
	ErrNoTxInputs         = errors.New("transaction has no inputs")

	// LockTime
	ErrSequenceNotSatisfied = errors.New("transaction's sequence locks on inputs not met")
	ErrImmatureSpend        = errors.New("try to spend immature coins")
	ErrUnfinalizedTx        = errors.New("transaction is not finalized")
	errUnFinalizedTx        = errors.New("block contains unFinalized transaction")

	// Value
	ErrDoubleSpend   = errors.New("the transaction has been spent")
	ErrDuplicateTx   = errors.New("the transaction is duplicate")
	ErrBadTxOutValue = errors.New("transaction output value is invalid")
	ErrSpendTooHigh  = errors.New("total value of all transaction inputs  is less than the spent value of output")

	// Fee
	ErrBadFees              = errors.New("total fees for block overflows accumulator")
	ErrInsufficientFee      = errors.New("transaction`s fees is under the required amount")
	ErrInsufficientPriority = errors.New("transaction`s Priority is under the required amount")
	ErrDust                 = errors.New("transaction output payment is dust")

	// Size
	ErrNonStandardTxSize = errors.New("transaction size is larger than max allowed size")
	ErrWitnessSize       = errors.New("transaction input witness size is large than max allowed size")
	ErrTxTooBig          = errors.New("transaction size is too big")
	ErrBlockTooBig       = errors.New("block size more than MaxBlockPayload")
	ErrTxMsgPayloadSize  = errors.New("transaction payload size is large than max allowed size")

	// TxScript
	ErrSignaturePushOnly   = errors.New("transaction input witness is not push only")
	ErrNuLLDataScript      = errors.New("more than one transaction output in a nulldata script")
	ErrNonStandardType     = errors.New("non-standard script form")
	ErrParseInputScript    = errors.New("failed to parse transaction input script")
	ErrExpectedSignInput   = errors.New("transaction expected input signature not satisfied")
	ErrMultiSigScriptPKNum = errors.New("multi-signature script with wrong pubkey number ")
	ErrMultiSigNumSign     = errors.New("multi-signature script with wrong signature number ")
	ErrScriptMalformed     = errors.New("failed to construct vm engine")
	ErrScriptValidation    = errors.New("failed to validate signature")
	ErrWitnessLength       = errors.New("invalid witness length")

	// Listener
	errNilArgument = errors.New("nil argument")
)
