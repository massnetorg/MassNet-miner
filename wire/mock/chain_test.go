package mock

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/consensus"
	"massnet.org/mass/poc"
	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

func TestMain(m *testing.M) {
	consensus.CoinbaseMaturity = 20
	consensus.MinStakingValue = 100 * consensus.MaxwellPerMass
	consensus.MinFrozenPeriod = 4
	consensus.StakingTxRewardStart = 2
	DefaultFrozenPeriodRange = [2]uint64{consensus.MinFrozenPeriod, consensus.MinFrozenPeriod + 20}

	if consensus.CoinbaseMaturity > challengeInterval {
		threshold = consensus.CoinbaseMaturity + 1
	}
	fmt.Println("threshold:", threshold)
	fmt.Println("coinbase maturity:", consensus.CoinbaseMaturity)
	fmt.Println("minimum frozen period:", consensus.MinFrozenPeriod)
	fmt.Println("minimum staking value:", consensus.MinStakingValue)

	os.Exit(m.Run())
}

func TestNewMockedChain(t *testing.T) {
	opt := &Option{
		Mode:                Auto,
		TotalHeight:         30,
		TxPerBlock:          10,
		MinNormalTxPerBlock: 1,
	}
	chain, err := NewMockedChain(opt)
	if err != nil {
		t.Fatal(err)
	}

	blocks := chain.Blocks()
	type UTxO struct {
		height   uint64
		spent    bool
		maturity uint64
	}
	utxoMap := make(map[string]UTxO)

	// record utxos
	for _, blk := range blocks {
		height := blk.Header.Height
		if height == 0 {
			continue
		}
		for k, tx := range blk.Transactions {
			txID := tx.TxHash()
			for i, txout := range tx.TxOut {
				scriptClass, pops := txscript.GetScriptInfo(txout.PkScript)
				frozenPeriod, _, err := txscript.GetParsedOpcode(pops, scriptClass)
				if err != nil {
					t.Log(err, height, i)
					t.FailNow()
				}

				id := txID.String() + strconv.Itoa(i)
				uo := UTxO{
					height: height,
					spent:  false,
				}

				switch scriptClass {
				case txscript.BindingScriptHashTy:
					uo.maturity = consensus.TransactionMaturity
				case txscript.WitnessV0ScriptHashTy:
					if k == 0 && i == len(tx.TxOut)-1 {
						uo.maturity = consensus.CoinbaseMaturity
						break
					}
					uo.maturity = 0
				case txscript.StakingScriptHashTy:
					uo.maturity = frozenPeriod + 1
				default:
					panic("unsupported script class")
				}
				utxoMap[id] = uo
			}
		}
	}

	var findUTxO = func(txid wire.Hash, index int, endHeight uint64) bool {
		id := txid.String() + strconv.Itoa(index)
		utxo, ok := utxoMap[id]
		if !ok {
			t.Error("UTxO not exists")
			return false
		}
		if utxo.height+utxo.maturity > endHeight {
			t.Error("UTxO not mature")
			return false
		}
		if utxo.spent {
			t.Error("double spent UTxO")
			return false
		}
		utxo.spent = true
		return true
	}

	for _, blk := range blocks {
		height := blk.Header.Height
		// t.Log("height", height, "hash", blk.BlockHash(), "tx num", len(blk.Transactions))
		for j, tx := range blk.Transactions {
			if j == 0 {
				continue
			}
			for _, in := range tx.TxIn {
				if !findUTxO(in.PreviousOutPoint.Hash, int(in.PreviousOutPoint.Index), height) {
					t.Fatal(blk.Header.Height, in.PreviousOutPoint)
				}
			}
		}
	}

}

type Proof struct {
	X         string `json:"X"`
	XPrime    string `json:"XPrime"`
	BitLength int    `json:"BitLength"`
}

type PoolData struct {
	PublicKey   string `json:"PublicKey"`
	PrivateKey  string `json:"PrivateKey"`
	ChainCode   string `json:"ChainCode"`
	Proof       Proof  `json:"Proof"`
	Target      string `json:"Target"`
	Quality     string `json:"Quality"`
	QualityData string `json:"QualityData"`
	Challenge   string `json:"Challenge"`
	Time        uint64 `json:"Time"`
	Height      uint64 `json:"Height"`
}

type AllResult struct {
	RootChainCode  string     `json:"RootChainCode"`
	RootPrivateKey string     `json:"RootPrivateKey"`
	PoolDataArr    []PoolData `json:"PoolDataArr"`
}

func TestExportData(t *testing.T) {
	opt := &Option{
		Mode:        Auto,
		TotalHeight: 199,
		TxPerBlock:  1,
	}
	chain, err := NewMockedChain(opt)
	if err != nil {
		t.Fatal(err)
	}
	poolArr := make([]PoolData, chain.Height()-10, chain.Height()-10)
	for i, blk := range chain.Blocks() {
		if i < 10 {
			continue
		}
		pk := blk.Header.PubKey
		sk, exists := pocKeys[pkStr(pk)]
		if !exists {
			t.FailNow()
		}
		// chaincode, exists := pocKeyChainCodes[pkStr(pk)]
		// if !exists {
		// 	t.Fatal("chaincode not exists")
		// }
		proof := blk.Header.Proof

		slot := uint64(blk.Header.Timestamp.Unix()) / poc.PoCSlot
		quality, err := proof.GetVerifiedQuality(pocutil.PubKeyHash(pk), pocutil.Hash(blk.Header.Challenge), slot, blk.Header.Height)
		if err != nil {
			t.Fatal(err)
		}

		poolArr[i-10] = PoolData{
			PublicKey:  pkStr(pk),
			PrivateKey: hex.EncodeToString(sk.Serialize()),
			// ChainCode:  chaincode,
			Proof: Proof{
				X:         hex.EncodeToString(blk.Header.Proof.X),
				XPrime:    hex.EncodeToString(blk.Header.Proof.XPrime),
				BitLength: blk.Header.Proof.BitLength,
			},
			Target:      hex.EncodeToString(blk.Header.Target.Bytes()),
			Quality:     hex.EncodeToString(quality.Bytes()),
			QualityData: proof.GetHashVal(uint64(blk.Header.Timestamp.Unix())/poc.PoCSlot, blk.Header.Height).String(),
			Challenge:   blk.Header.Challenge.String(),
			Time:        uint64(blk.Header.Timestamp.Unix()),
			Height:      blk.Header.Height,
		}
	}

	result := &AllResult{
		RootChainCode:  rawPoCRootKeyChainCode,
		RootPrivateKey: rawPoCRootKeyData,
		PoolDataArr:    poolArr,
	}

	jData, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create("pool_data.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write(jData)
}

func TestHex(t *testing.T) {
	x := uint64(1<<20 + 18)
	var xb [8]byte
	binary.LittleEndian.PutUint64(xb[:], x)
	fmt.Println("byteArray", xb)
	fmt.Println("byteHex", hex.EncodeToString(xb[:]))
	fmt.Println("numHex", fmt.Sprintf("%x", x))

	y := big.NewInt(1 << 62)

	fmt.Println(y.Mul(y, big.NewInt(4)).Uint64())
	fmt.Println(y.Add(y, big.NewInt(4)).Uint64())
}

func TestShowChainTxStat(t *testing.T) {
	opt := &Option{
		Mode:                Auto,
		TotalHeight:         100,
		TxPerBlock:          15,
		MinNormalTxPerBlock: 1,
	}
	chain, err := NewMockedChain(opt)
	assert.Nil(t, err)

	t.Log("N=NormalTx, B=BindingTx, L=StakingTx W=WithdrawalTx")
	t.Logf("TxNormal : TxBinding : TxStaking : TxWithdrawal = %d:%d:%d:%d",
		chain.opt.TxScale[0], chain.opt.TxScale[1],
		chain.opt.TxScale[2], chain.opt.TxScale[3])
	var totalN, totalB, totalL, totalW, totalNBro int
	for _, stat := range chain.stats {
		n := stat.stat[TxNormal]
		totalN += n
		b := stat.stat[TxBinding]
		totalB += b
		l := stat.stat[TxStaking]
		totalL += l
		w := stat.stat[Txwithdrawal]
		totalW += w
		nbro := stat.stat[TxNormalSpentBroUtxo]
		totalNBro += nbro
		t.Logf("Height=%d, N=%d(%d), B=%d, L=%d, W=%d", stat.height, n, nbro, b, l, w)
	}
	t.Logf("total N=%d(%d), total B=%d, total L=%d, total W=%d", totalN, totalNBro, totalB, totalL, totalW)
	// t.Log("owner ", len(chain.owner2utxos))
	// for owner, m := range chain.owner2utxos {
	// 	i := 0
	// 	for _, u := range m {
	// 		if u.spent {
	// 			i++
	// 		}
	// 	}
	// 	t.Log(owner, len(m), i)
	// }
}

func TestMockDataForBlockchainTests(t *testing.T) {
	fmt.Println(os.Args)
	switch os.Args[len(os.Args)-1] {
	case "1":
		testExportForkBeforeStaking(t)
	case "2":
		testExportForkBeforeStakingExpire(t)
	case "3":
		testExportForkBeforeStakingReward(t)
	case "4":
		testExportForkBeforeBinding(t)
	case "5":
		testExportForkForStdOnce(t)
	case "6":
		testHelperExportBlks(t)
	default:
	}
}

// this is used for tests in package blockchain
func testHelperExportBlks(t *testing.T) {
	opt := &Option{
		Mode:                Auto,
		TotalHeight:         200,
		TxPerBlock:          6,
		MinNormalTxPerBlock: 1,
	}
	chain, err := NewMockedChain(opt)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create("export/mockBlks.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	for _, blk := range chain.Blocks() {
		buf, err := blk.Bytes(wire.Packet)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
		if err != nil {
			t.Fatal(err)
		}
	}
}

// this is used for tests in package blockchain
func testExportForkBeforeStakingExpire(t *testing.T) {

	tests := []struct {
		name string
		path string
		opt  *Option
	}{
		{
			name: "mock FPStakingEnd",
			path: "./export/beforestakingexpire.dat",
			opt: &Option{
				Mode:                AutoFork,
				TotalHeight:         50,
				TxPerBlock:          6,
				MinNormalTxPerBlock: 1,
				TxScale:             [4]byte{20, 1, 0, 0},
				FrozenPeriodRange:   [2]uint64{consensus.MinFrozenPeriod, consensus.MinFrozenPeriod + 1},
				ForkOpt: &ForkOptions{
					maxForks:     1,
					maxBlkOnFork: 3,
					FP:           FPStakingEnd,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewMockedChain(test.opt)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println("total blks: ", len(chain.allBlks))
			for _, stat := range chain.stats {
				n := stat.stat[TxNormal]
				g := stat.stat[TxBinding]
				l := stat.stat[TxStaking]
				w := stat.stat[Txwithdrawal]
				nbro := stat.stat[TxNormalSpentBroUtxo]
				t.Logf("Height=%d, N=%d(%d), G=%d, L=%d, W=%d", stat.height, n, nbro, g, l, w)
			}

			f, err := os.Create(test.path)
			assert.Nil(t, err)
			defer f.Close()

			for _, blk := range chain.allBlks {
				buf, err := blk.Bytes(wire.Packet)
				assert.Nil(t, err)
				_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
				assert.Nil(t, err)
			}
		})
	}
}

// this is used for tests in package blockchain
func testExportForkBeforeStaking(t *testing.T) {

	tests := []struct {
		name string
		path string
		opt  *Option
	}{
		{
			name: "mock FPStakingBegin",
			path: "./export/beforestaking.dat",
			opt: &Option{
				Mode:                AutoFork,
				TotalHeight:         50,
				TxPerBlock:          6,
				MinNormalTxPerBlock: 1,
				TxScale:             [4]byte{30, 1, 0, 0},
				FrozenPeriodRange:   [2]uint64{consensus.MinFrozenPeriod, consensus.MinFrozenPeriod + 1},
				ForkOpt: &ForkOptions{
					maxForks:     1,
					maxBlkOnFork: 3,
					FP:           FPStakingBegin,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewMockedChain(test.opt)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println("total blks: ", len(chain.allBlks))
			for _, stat := range chain.stats {
				n := stat.stat[TxNormal]
				g := stat.stat[TxBinding]
				l := stat.stat[TxStaking]
				w := stat.stat[Txwithdrawal]
				nbro := stat.stat[TxNormalSpentBroUtxo]
				t.Logf("Height=%d, N=%d(%d), G=%d, L=%d, W=%d", stat.height, n, nbro, g, l, w)
			}

			f, err := os.Create(test.path)
			assert.Nil(t, err)
			defer f.Close()

			for _, blk := range chain.allBlks {
				buf, err := blk.Bytes(wire.Packet)
				assert.Nil(t, err)
				_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
				assert.Nil(t, err)
			}
		})
	}
}

// this is used for tests in package blockchain
func testExportForkBeforeBinding(t *testing.T) {

	tests := []struct {
		name string
		path string
		opt  *Option
	}{
		{
			name: "mock FPBindingBegin",
			path: "./export/beforebinding.dat",
			opt: &Option{
				Mode:                AutoFork,
				TotalHeight:         50,
				TxPerBlock:          6,
				MinNormalTxPerBlock: 1,
				TxScale:             [4]byte{30, 0, 1, 0},
				ForkOpt: &ForkOptions{
					maxForks:     1,
					maxBlkOnFork: 3,
					FP:           FPBindingBegin,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewMockedChain(test.opt)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println("total blks: ", len(chain.allBlks))
			for _, stat := range chain.stats {
				n := stat.stat[TxNormal]
				g := stat.stat[TxBinding]
				l := stat.stat[TxStaking]
				w := stat.stat[Txwithdrawal]
				nbro := stat.stat[TxNormalSpentBroUtxo]
				t.Logf("Height=%d, N=%d(%d), G=%d, L=%d, W=%d", stat.height, n, nbro, g, l, w)
			}

			f, err := os.Create(test.path)
			assert.Nil(t, err)
			defer f.Close()

			for _, blk := range chain.allBlks {
				buf, err := blk.Bytes(wire.Packet)
				assert.Nil(t, err)
				_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
				assert.Nil(t, err)
			}
		})
	}
}

// this is used for tests in package blockchain
func testExportForkBeforeStakingReward(t *testing.T) {

	tests := []struct {
		name string
		path string
		opt  *Option
	}{
		{
			name: "mock FPStakingRewardBegin",
			path: "./export/beforestakingreward.dat",
			opt: &Option{
				Mode:                AutoFork,
				TotalHeight:         50,
				TxPerBlock:          6,
				MinNormalTxPerBlock: 1,
				TxScale:             [4]byte{30, 1, 0, 0},
				ForkOpt: &ForkOptions{
					maxForks:     1,
					maxBlkOnFork: 3,
					FP:           FPStakingRewardBegin,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewMockedChain(test.opt)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println("total blks: ", len(chain.allBlks))
			for _, stat := range chain.stats {
				n := stat.stat[TxNormal]
				g := stat.stat[TxBinding]
				l := stat.stat[TxStaking]
				w := stat.stat[Txwithdrawal]
				nbro := stat.stat[TxNormalSpentBroUtxo]
				t.Logf("Height=%d, N=%d(%d), G=%d, L=%d, W=%d", stat.height, n, nbro, g, l, w)
			}

			f, err := os.Create(test.path)
			assert.Nil(t, err)
			defer f.Close()

			for _, blk := range chain.allBlks {
				buf, err := blk.Bytes(wire.Packet)
				assert.Nil(t, err)
				_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
				assert.Nil(t, err)
			}
		})
	}
}

// this is used for preparing dup txid test data
func testExportForkForStdOnce(t *testing.T) {

	tests := []struct {
		name string
		path string
		opt  *Option
	}{
		{
			name: "mock FPStandardOnce",
			path: "./export/stdonce.dat",
			opt: &Option{
				Mode:                AutoFork,
				TotalHeight:         50,
				TxPerBlock:          1,
				MinNormalTxPerBlock: 1,
				TxScale:             [4]byte{1, 0, 0, 0},
				ForkOpt: &ForkOptions{
					maxForks:     1,
					maxBlkOnFork: 3,
					FP:           FPStandardOnce,
				},
				NoNormalOutputSplit: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := NewMockedChain(test.opt)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println("total blks: ", len(chain.allBlks))
			for _, stat := range chain.stats {
				n := stat.stat[TxNormal]
				g := stat.stat[TxBinding]
				l := stat.stat[TxStaking]
				w := stat.stat[Txwithdrawal]
				nbro := stat.stat[TxNormalSpentBroUtxo]
				t.Logf("Height=%d, N=%d(%d), G=%d, L=%d, W=%d", stat.height, n, nbro, g, l, w)
			}

			f, err := os.Create(test.path)
			assert.Nil(t, err)
			defer f.Close()

			for _, blk := range chain.allBlks {
				buf, err := blk.Bytes(wire.Packet)
				assert.Nil(t, err)
				_, err = fmt.Fprintln(f, hex.EncodeToString(buf))
				assert.Nil(t, err)
				// for i := 1; i < len(blk.Transactions); i++ {
				// 	fmt.Println(blk.Header.Height, i, len(blk.Transactions[i].TxOut))
				// }
			}
		})
	}
}
