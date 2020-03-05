package mock

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

var (
	DefaultFrozenPeriodRange = [2]uint64{}
	DefaultTxScale           = [4]byte{20, 4, 4, 2}
	Probability              = []TxType{}
	DefautlBitLength         = 24
)

const (
	// mockCount         = 1000
	keyCount          = 30
	challengeInterval = 10
)

var (
	threshold uint64 = challengeInterval + 1
)

type Option struct {
	// Basic config
	// Maturity uint64
	Mode    MockingMode
	ForkOpt *ForkOptions // in AutoFork mode
	// Auto mode options
	TotalHeight         int64
	TxPerBlock          int // Max Tx Count per Block (except for coinbase)
	MinNormalTxPerBlock int // Min Normal Tx Count per Bock (except for coinbase)
	// Other mode options
	TxScale           [4]byte   // Ratio of different types of txs per block (TxNormal:TxStaking:TxBinding:TxWithdrawal)
	FrozenPeriodRange [2]uint64 // Range of frozen period of staking tx, inclusive for min and exclusive for max value
	BitLength         int

	NoNormalOutputSplit bool
}

type BlockTxStat struct {
	height uint64
	stat   map[TxType]int
}

type Chain struct {
	// maturity       uint64
	// lastUtxoHeight uint64
	// utxoCount      int
	blocks      []*wire.MsgBlock
	opt         *Option
	utxos       map[uint64][]*utxo // utxos created at each height
	txIndex     map[wire.Hash]*txLoc
	gUtxoMgr    *gUtxoMgr
	lUtxoMgr    *lUtxoMgr
	owner2utxos map[string][]*utxo

	stats []*BlockTxStat

	constructHeight uint64
	forkStartHeight int64
	allBlks         []*wire.MsgBlock
	randEmitter     *randEmitter
}

//type witnessKey string
//
//func toWitnessKey(txHash wire.Hash, outIndex uint32) witnessKey {
//	return witnessKey(hex.EncodeToString(txHash[:]) + strconv.Itoa(int(outIndex)))
//}

type utxo struct {
	prePoint     *wire.OutPoint
	blockIndex   int
	txIndex      uint32
	redeemScript []byte
	privateKey   *btcec.PrivateKey
	value        int64
	spent        bool
	blockHeight  uint64
	maturity     uint64
	pocAddr      massutil.Address // it's a binding utxo
	isStaking    bool
}

type txLoc struct {
	blockHeight uint64
	blockIndex  int
	tx          *wire.MsgTx
}

type MockingMode int32

const (
	Auto MockingMode = iota
	AutoFork
	Other
)

type TxType byte

const (
	TxNormal TxType = iota
	TxStaking
	TxBinding
	Txwithdrawal
	TxNormalSpentBroUtxo
)

func checkOption(opt *Option) error {
	// if opt.Maturity < 1 {
	// 	return errors.New(fmt.Sprintf("invalid Maturity %d", opt.Maturity))
	// }

	switch opt.Mode {
	case Auto, AutoFork:
		if opt.TotalHeight < 0 {
			return errors.New(fmt.Sprintf("invalid totalHeight %d", opt.TotalHeight))
		}
		if opt.TxPerBlock < 1 {
			return errors.New(fmt.Sprintf("invalid TxPerBlock %d", opt.TxPerBlock))
		}

		if opt.MinNormalTxPerBlock < 0 {
			return errors.New(fmt.Sprintf("invalid MinNormalTxPerBlock %d", opt.MinNormalTxPerBlock))
		}

		if opt.MinNormalTxPerBlock > opt.TxPerBlock {
			return fmt.Errorf("MinNormalTxPerBlock %d is greater than TxPerBlock %d",
				opt.MinNormalTxPerBlock, opt.TxPerBlock)
		}

	case Other:
		return errors.New("invalid mock mode")
	default:
		return errors.New("invalid mock mode")
	}

	if opt.TxScale == [4]byte{} {
		opt.TxScale = DefaultTxScale
	}

	for i, n := range opt.TxScale {
		tp := TxType(i)
		for ; n > 0; n-- {
			Probability = append(Probability, tp)
		}
	}

	if opt.FrozenPeriodRange == [2]uint64{} {
		opt.FrozenPeriodRange = DefaultFrozenPeriodRange
	}
	if opt.FrozenPeriodRange[0] <= 0 || opt.FrozenPeriodRange[0] >= opt.FrozenPeriodRange[1] {
		return errors.New("invalid forzen period range")
	}

	if opt.BitLength == 0 {
		opt.BitLength = DefautlBitLength
	}

	fmt.Println("options:", opt)
	return nil
}

// Attention:
// if totalHeight < ChallengeInterval(=10), block content is fixed.
// if totalHeight >= ChallengeInterval(=10), blocks after ChallengeInterval is configurable.
func NewMockedChain(opt *Option) (*Chain, error) {
	if err := checkOption(opt); err != nil {
		return nil, err
	}
	initTemplateData(int(opt.TotalHeight))
	chain := &Chain{
		opt:     opt,
		utxos:   make(map[uint64][]*utxo),
		txIndex: make(map[wire.Hash]*txLoc),
		gUtxoMgr: &gUtxoMgr{
			pocAddr2utxos: make(map[string][]*bindingUtxo),
			op2gutxo:      make(map[wire.OutPoint]*bindingUtxo),
		},
		lUtxoMgr: &lUtxoMgr{
			addr2utxos: make(map[string][]*utxo),
			// addr2utxoSpent: make(map[string][]*lockUtxo),
			op2lutxo: make(map[wire.OutPoint]*utxo),
		},
		owner2utxos: make(map[string][]*utxo),
		stats:       make([]*BlockTxStat, 0),
		allBlks:     make([]*wire.MsgBlock, 0, opt.TotalHeight),
		randEmitter: &randEmitter{
			his: make(map[int]struct{}),
		},
	}
	chain.lUtxoMgr.c = chain
	chain.gUtxoMgr.c = chain
	if err := chain.mockChain(); err != nil {
		return nil, err
	}
	return chain, nil
}

func (c *Chain) mockChain() error {
	switch c.opt.Mode {
	case Auto:
		return c.autoMockChain()
	case AutoFork:
		return c.autoMockForkChain()
	case Other:
		return errors.New("invalid mock mode (other)")
	default:
		return errors.New("invalid mock mode")
	}
}

func (c *Chain) autoMockChain() error {
	totalHeight, txPerBlock := c.opt.TotalHeight, c.opt.TxPerBlock
	c.blocks = make([]*wire.MsgBlock, c.opt.TotalHeight, c.opt.TotalHeight)

	// fill in basic blocks
	for i := int64(0); i < totalHeight; i++ {
		blk, err := copyBlock(basicBlocks[i])
		if err != nil {
			return err
		}
		c.blocks[i] = blk
	}

	// else if totalHeight >= ChallengeInterval(=10), blocks after ChallengeInterval is configurable
	ensureNumberKeys(txPerBlock)
	for i := 1; i < int(totalHeight); i++ {
		if i < challengeInterval+1 {
			err := c.retrieveCoinbase(c.blocks[i], uint64(i))
			if err != nil {
				return err
			}
			continue
		}
		block, err := c.constructBlock(c.blocks[i], uint64(i))
		if err != nil {
			return err
		}
		c.blocks[i] = block

		if i%30 == 0 {
			c.clearUtxos()
		}
	}
	return nil
}

func (c *Chain) clearUtxos() {
	nm := make(map[string][]*utxo)
	for owner, utxos := range c.owner2utxos {
		nm[owner] = make([]*utxo, 0, len(utxos))
		for _, u := range utxos {
			if u.spent {
				continue
			}
			nm[owner] = append(nm[owner], u)
		}
	}
	c.owner2utxos = nm
}

func (c *Chain) Height() int64 {
	return int64(len(c.blocks))
}

func (c *Chain) Blocks() []*wire.MsgBlock {
	return c.blocks
}

func (c *Chain) ForkStartHeight() int64 {
	return c.forkStartHeight
}
