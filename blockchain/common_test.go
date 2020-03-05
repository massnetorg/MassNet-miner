package blockchain

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/database"
	"massnet.org/mass/database/ldb"
	"massnet.org/mass/database/memdb"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

var (
	dataDir = "testdata"
	dbpath  = "./" + dataDir + "/testdb"
	logpath = "./" + dataDir + "/testlog"
	// dbType             = "leveldb"

	blks200 []*massutil.Block
)

// testDbRoot is the root directory used to create all test databases.
const testDbRoot = "testdbs"

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Returns a new memory chaindb which is initialized with genesis block.
func newTestChainDb() (database.Db, error) {
	db, err := memdb.NewMemDb()
	if err != nil {
		return nil, err
	}

	// Get the latest block height from the database.
	_, height, err := db.NewestSha()
	if err != nil {
		db.Close()
		return nil, err
	}

	// Insert the appropriate genesis block for the Mass network being
	// connected to if needed.
	if height == math.MaxUint64 {
		blk, err := loadNthBlk(1)
		if err != nil {
			return nil, err
		}
		err = db.InitByGenesisBlock(blk)
		if err != nil {
			db.Close()
			return nil, err
		}
		height = blk.Height()
		copy(config.ChainParams.GenesisHash[:], blk.Hash()[:])
	}
	fmt.Printf("newTestChainDb initialized with block height %v\n", height)
	return db, nil
}

// loadTestBlksIntoTestChainDb Loads fisrt N blocks (genesis not included) into db
func loadTestBlksIntoTestChainDb(db database.Db, numBlks int) error {
	blks, err := loadTopNBlk(numBlks)
	if err != nil {
		return err
	}

	for i, blk := range blks {
		if i == 0 {
			continue
		}

		err = db.SubmitBlock(blk)
		if err != nil {
			return err
		}
		cdb := db.(*ldb.ChainDb)
		cdb.Batch(1).Set(*blk.Hash())
		cdb.Batch(1).Done()
		err = db.Commit(*blk.Hash())
		if err != nil {
			return err
		}

		if i == numBlks-1 {
			// // used to print tx info
			// txs := blk.Transactions()
			// tx1 := txs[1]
			// for i, txIn := range tx1.MsgTx().TxIn {
			// 	h := txIn.PreviousOutPoint.Hash
			// 	fmt.Println(i, h, h[:], txIn.PreviousOutPoint.Index)
			// 	fmt.Println(txIn.Sequence)
			// 	fmt.Println(txIn.Witness)
			// }
			// fmt.Println(tx1.MsgTx().LockTime, tx1.MsgTx().Payload)
			// for i, txOut := range tx1.MsgTx().TxOut {
			// 	fmt.Println(i, txOut.Value, txOut.PkScript)
			// }
			break
		}
	}

	_, height, err := db.NewestSha()
	if err != nil {
		return err
	}
	if height != uint64(numBlks-1) {
		return errors.New("incorrect best height")
	}
	return nil
}

func newTestBlockchain(db database.Db, blkCachePath string) (*Blockchain, error) {
	chain, err := NewBlockchain(db, blkCachePath, nil)
	if err != nil {
		return nil, err
	}
	chain.GetTxPool().SetNewTxCh(make(chan *massutil.Tx, 2000)) // prevent deadlock
	return chain, nil
}

func init() {
	f, err := os.Open("./data/mockBlks.dat")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		buf, err := hex.DecodeString(scanner.Text())
		if err != nil {
			panic(err)
		}

		blk, err := massutil.NewBlockFromBytes(buf, wire.Packet)
		if err != nil {
			panic(err)
		}
		blks200 = append(blks200, blk)
	}
}

// the 1st block is genesis
func loadNthBlk(nth int) (*massutil.Block, error) {
	if nth <= 0 || nth > len(blks200) {
		return nil, errors.New("invalid nth")
	}
	return blks200[nth-1], nil
}

// the 1st block is genesis
func loadTopNBlk(n int) ([]*massutil.Block, error) {
	if n <= 0 || n > len(blks200) {
		return nil, errors.New("invalid n")
	}

	return blks200[:n], nil
}

func newWitnessScriptAddress(pubkeys []*btcec.PublicKey, nrequired int,
	addressClass uint16, net *config.Params) ([]byte, massutil.Address, error) {

	var addressPubKeyStructs []*massutil.AddressPubKey
	for i := 0; i < len(pubkeys); i++ {
		pubKeySerial := pubkeys[i].SerializeCompressed()
		addressPubKeyStruct, err := massutil.NewAddressPubKey(pubKeySerial, net)
		if err != nil {
			logging.CPrint(logging.ERROR, "create addressPubKey failed",
				logging.LogFormat{
					"err":       err,
					"version":   addressClass,
					"nrequired": nrequired,
				})
			return nil, nil, err
		}
		addressPubKeyStructs = append(addressPubKeyStructs, addressPubKeyStruct)
	}

	redeemScript, err := txscript.MultiSigScript(addressPubKeyStructs, nrequired)
	if err != nil {
		logging.CPrint(logging.ERROR, "create redeemScript failed",
			logging.LogFormat{
				"err":       err,
				"version":   addressClass,
				"nrequired": nrequired,
			})
		return nil, nil, err
	}
	var witAddress massutil.Address
	// scriptHash is witnessProgram
	scriptHash := sha256.Sum256(redeemScript)
	switch addressClass {
	case massutil.AddressClassWitnessStaking:
		witAddress, err = massutil.NewAddressStakingScriptHash(scriptHash[:], net)
	case massutil.AddressClassWitnessV0:
		witAddress, err = massutil.NewAddressWitnessScriptHash(scriptHash[:], net)
	default:
		return nil, nil, errors.New("invalid version")
	}

	if err != nil {
		logging.CPrint(logging.ERROR, "create witness address failed",
			logging.LogFormat{
				"err":       err,
				"version":   addressClass,
				"nrequired": nrequired,
			})
		return nil, nil, err
	}

	return redeemScript, witAddress, nil
}

func mkTmpDir(dir string) (func(), error) {
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0700)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return func() {
		os.RemoveAll(dir)
	}, nil
}

var (
	comTestCoinbaseMaturity     uint64 = 20
	comTestMinStakingValue             = 100 * consensus.MaxwellPerMass
	comTestMinFrozenPeriod      uint64 = 4
	comTestStakingTxRewardStart uint64 = 2
)

// make sure to be consistent with mocked blocks
func TestMain(m *testing.M) {
	consensus.CoinbaseMaturity = comTestCoinbaseMaturity
	consensus.MinStakingValue = comTestMinStakingValue
	consensus.MinFrozenPeriod = comTestMinFrozenPeriod
	consensus.StakingTxRewardStart = comTestStakingTxRewardStart

	fmt.Println("coinbase maturity:", consensus.CoinbaseMaturity)
	fmt.Println("mininum frozen period:", consensus.MinFrozenPeriod)
	fmt.Println("mininum staking value:", consensus.MinStakingValue)

	os.Exit(m.Run())
}
