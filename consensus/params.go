package consensus

const (
	MaxwellPerMass uint64 = 100000000
	// 1 mass, 1 day old, a tx of 250 bytes
	MinHighPriority          = 1e8 * 1920.0 / 250
	MaxNewBlockChSize        = 1024
	DefaultBlockPrioritySize = 50000

	// staking tx
	MaxStakingRewardNum                = 30
	defaultStakingTxRewardStart uint64 = 24

	defaultCoinbaseMaturity    uint64 = 1000
	defaultTransactionMaturity uint64 = 1

	defaultMaxMass       uint64 = 206438400
	defaultMinRelayTxFee uint64 = 10000

	defaultSubsidyHalvingInterval uint64 = 13440
	defaultBaseSubsidy            uint64 = 1024 * MaxwellPerMass
	defaultMinHalvedSubsidy       uint64 = 6250000

	defaultMinFrozenPeriod uint64 = 61440
	defaultMinStakingValue uint64 = 2048 * MaxwellPerMass

	Massip1Activation uint64 = 694000
	MaxValidPeriod           = defaultMinFrozenPeriod * 24 // 1474560
)

var (
	// CoinbaseMaturity is the number of blocks required before newly
	// mined coins can be spent
	CoinbaseMaturity = defaultCoinbaseMaturity

	// TransactionMaturity is the number of blocks required before newly
	// binding tx get reward
	TransactionMaturity = defaultTransactionMaturity

	// MaxMass the maximum MASS amount
	MaxMass = defaultMaxMass

	SubsidyHalvingInterval = defaultSubsidyHalvingInterval
	// BaseSubsidy is the original subsidy Maxwell for mined blocks.  This
	// value is halved every SubsidyHalvingInterval blocks.
	BaseSubsidy = defaultBaseSubsidy
	// MinHalvedSubsidy is the minimum subsidy Maxwell for mined blocks.
	MinHalvedSubsidy = defaultMinHalvedSubsidy

	// MinRelayTxFee minimum relay fee in Maxwell
	MinRelayTxFee = defaultMinRelayTxFee

	//Min Frozen Period in a StakingScriptHash output
	MinFrozenPeriod = defaultMinFrozenPeriod
	//MinStakingValue minimum StakingScriptHash output in Maxwell
	MinStakingValue = defaultMinStakingValue

	StakingTxRewardStart = defaultStakingTxRewardStart
)
