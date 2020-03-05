package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/consensus"
	"massnet.org/mass/errors"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/txscript"
	"massnet.org/mass/wire"
)

var testTx = &wire.MsgTx{
	Version: wire.TxVersion,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				//Hash:  wire.Hash{56, 150, 133, 8, 255, 91, 93, 180, 51, 149, 76, 28, 134, 170, 238, 66, 129, 222, 56, 197, 28, 165, 108, 215, 211, 11, 71, 103, 135, 155, 243, 243},
				Hash:  wire.Hash{243, 243, 155, 135, 103, 71, 11, 211, 215, 108, 165, 28, 197, 56, 222, 129, 66, 238, 170, 134, 28, 76, 149, 51, 180, 93, 91, 255, 8, 133, 150, 56},
				Index: 0,
			},
			Witness: [][]byte{{72, 48, 69, 2, 33, 0, 145, 156, 12, 214, 50, 139, 190, 150, 45, 194, 172, 214, 92, 27, 4, 246, 31, 102, 1, 66, 48, 168, 72, 118, 23, 250, 91, 170, 247, 215, 3, 223, 2,
				32, 7, 101, 137, 166, 210, 163, 248, 117, 148, 56, 35, 15, 151, 195, 166, 198, 243, 234, 175, 177, 164, 246, 4, 183, 210, 182, 157, 54, 97, 238, 148, 42, 1},
				{81, 33, 2, 197, 133, 128, 106, 27, 84, 252, 212, 53, 27, 254, 77, 232, 46, 242, 210, 99, 95, 66, 116, 19, 127, 229, 199, 62, 205, 129, 241, 191, 57, 97, 237, 81, 174}},
			Sequence: 4294967295,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 2100000000,
			//PkScript: []byte{0, 20, 219, 196, 166, 140, 204, 59, 218, 46, 167, 227, 186, 178, 29, 18, 173, 208, 207, 147, 118, 248},
			PkScript: []byte{0, 20, 63, 144, 140, 149, 87, 108, 214, 24, 129, 250, 177, 187, 82, 157, 39, 239, 170, 233, 125, 124},
		},
	},
	LockTime: 0,
	Payload:  make([]byte, 0, 15),
}

var testTxs map[string]*blk50Tx

type blk50Tx struct {
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty"`
	Hex  string `json:"hex,omitempty"`
	Txid string `json:"txid,omitempty"`
}

func init() {
	buf, err := ioutil.ReadFile("./data/txpool_test_data.json")
	if err != nil {
		panic(err)
	}

	var slc []*blk50Tx
	err = json.Unmarshal(buf, &slc)
	if err != nil {
		panic(err)
	}
	testTxs = make(map[string]*blk50Tx)
	for _, tx := range slc {
		testTxs[tx.Name] = tx
		// fmt.Println("load txpool_test_data.json:", tx.Name)
	}
}

func firstTx(txStr string) (*wire.MsgTx, error) {
	if len(txStr)%2 != 0 {
		txStr = "0" + txStr
	}
	serializedTx, err := hex.DecodeString(txStr)
	if err != nil {
		return nil, err
	}

	msgtx := wire.NewMsgTx()

	err = msgtx.SetBytes(serializedTx, wire.Packet)
	if err != nil {
		return nil, err
	}
	return msgtx, nil
}

func decodeHexStr(hexStr string) ([]byte, error) {
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func messageToHex(msg wire.Message) (string, error) {
	var buf bytes.Buffer
	if _, err := msg.Encode(&buf, wire.Packet); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

func witnessToHex(witness wire.TxWitness) []string {
	// Ensure nil is returned when there are no entries versus an empty
	// slice so it can properly be omitted as necessary.
	if len(witness) == 0 {
		return nil
	}

	result := make([]string, 0, len(witness))
	for _, wit := range witness {
		result = append(result, hex.EncodeToString(wit))
	}

	return result
}

func TestTx(t *testing.T) {
	sign, err := decodeHexStr("483045022100919c0cd6328bbe962dc2acd65c1b04f61f66014230a8487617fa5baaf7d703df0220076589a6d2a3f8759438230f97c3a6c6f3eaafb1a4f604b7d2b69d3661ee942a01")
	redeemScript, _ := decodeHexStr("512102c585806a1b54fcd4351bfe4de82ef2d2635f4274137fe5c73ecd81f1bf3961ed51ae")
	pkscript, _ := decodeHexStr("00143f908c95576cd61881fab1bb529d27efaae97d7c")

	if err != nil {
		t.Error(err)
	}
	t.Log(redeemScript)
	t.Log(sign)
	t.Log(testTx.TxHash())

	witness := witnessToHex(testTx.TxIn[0].Witness)
	fmt.Println(len(testTx.TxIn[0].Witness[0]), len(testTx.TxIn[0].Witness[1]), len(testTx.TxIn[0].PreviousOutPoint.Hash))
	t.Log(witness)
	t.Log(hex.EncodeToString(pkscript))
	t.Log(hex.EncodeToString(testTx.TxIn[0].PreviousOutPoint.Hash.Bytes()))
	t.Log(messageToHex(testTx))
}

func newTxPool(loadBlksNum int) (*TxPool, func(), error) {

	db, err := newTestChainDb()
	if err != nil {
		return nil, nil, err
	}

	err = loadTestBlksIntoTestChainDb(db, loadBlksNum)
	if err != nil {
		db.Close()
		return nil, nil, err
	}

	bc, err := newTestBlockchain(db, ".")
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	bc.GetTxPool().SetNewTxCh(make(chan *massutil.Tx, 50))
	return bc.GetTxPool(), func() { db.Close() }, err
}

func getTx(name string) (*wire.MsgTx, error) {
	tx, ok := testTxs[name]
	if !ok {
		return nil, errors.New("not found tx:" + name)
	}
	serializedTx, err := hex.DecodeString(tx.Hex)
	if err != nil {
		return nil, err
	}

	msgtx := wire.NewMsgTx()

	err = msgtx.SetBytes(serializedTx, wire.Packet)
	if err != nil {
		return nil, err
	}
	return msgtx, nil
}

func TestTxPool_MaybeAcceptTransaction_Orphan(t *testing.T) {

	txP, close, err := newTxPool(22)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("orphanTxStr")
	if err != nil {
		t.Error(err)
	}

	tx := massutil.NewTx(msgtx)
	fmt.Println(tx.Hash())

	missingParents, err := txP.MaybeAcceptTransaction(tx, true, true)
	assert.Nil(t, err)
	assert.NotZero(t, len(missingParents))
	assert.Equal(t, 0, txP.Count())

}

func TestTxPool_MaybeAcceptTransaction_haveTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("child1")
	if err != nil {
		t.Fatal(err)
	}

	tx := massutil.NewTx(msgtx)
	missingParents, err := txP.maybeAcceptTransaction(tx, true, true)
	assert.Nil(t, err)
	assert.Zero(t, len(missingParents))
	assert.Equal(t, 1, txP.Count())

	h := msgtx.TxHash()
	have := txP.haveTransaction(&h)
	assert.True(t, have)
}

func TestTxPool_MaybeAcceptTransaction_AlreadyExist(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("parent")
	if err != nil {
		t.Fatal(err)
	}

	tx := massutil.NewTx(msgtx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, errors.ErrTxAlreadyExists, err)
}

func TestTxPool_MaybeAcceptTransaction_checkInputsStandard(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgTx, err := getTx("child1")
	if err != nil {
		t.Fatal(err)
	}
	msgTx.TxIn[0].Witness[1] = []byte{82, 33, 2, 197, 133, 128, 106, 27, 84, 252, 212, 53, 27, 254, 77, 232, 46, 242, 210, 99, 95, 66, 116, 19, 127, 229, 199, 62, 205, 129, 241, 191, 57, 97, 237, 81, 174}

	tx := massutil.NewTx(msgTx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrExpectedSignInput, err)
}

func TestTxPool_MaybeAcceptTransaction_DoubleSepneInTx(t *testing.T) {

	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("doubleSpendInTx")
	if err != nil {
		t.Error(err)
	}
	tx := massutil.NewTx(msgtx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrDuplicateTxInputs, err)
}

// tx is partially spent
func TestTxPool_MaybeAcceptTransaction_DoubleSpendInDb(t *testing.T) {
	txP, close, err := newTxPool(28)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("doubleSpendInDb")
	if err != nil {
		t.Error(err)
	}
	tx := massutil.NewTx(msgtx)
	missing, err := txP.maybeAcceptTransaction(tx, true, true)
	assert.Zero(t, len(missing))
	assert.Equal(t, ErrDoubleSpend, err)
}

// multiple txs spend the same output in mempool
func TestTxPool_checkPoolDoubleSpend(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()
	childTx, err := getTx("child2")
	assert.Nil(t, err)
	txP.addTransaction(massutil.NewTx(childTx), 26, 100.0, massutil.ZeroAmount(), massutil.ZeroAmount())

	grandTx, err := getTx("grandChild")
	assert.Nil(t, err)

	tx := massutil.NewTx(grandTx)
	txP.addTransaction(massutil.NewTx(grandTx), 27, 100.0, massutil.ZeroAmount(), massutil.ZeroAmount())
	err = txP.checkPoolDoubleSpend(tx)
	assert.Equal(t, ErrDoubleSpend, err)

	grandTx, err = getTx("grandChild")
	assert.Nil(t, err)
	grandTx.TxIn = grandTx.TxIn[1:]
	tx2 := massutil.NewTx(grandTx)
	missing, err := txP.maybeAcceptTransaction(tx2, true, true)
	assert.Zero(t, len(missing))
	assert.Equal(t, ErrDoubleSpend, err)
}

// total input is less than total output
func TestTxPool_MaybeAcceptTransaction_totalMaxwell(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("child1")
	if err != nil {
		t.Error(err)
	}
	tx := massutil.NewTx(msgtx)
	tx.MsgTx().TxOut[len(tx.MsgTx().TxOut)-1].Value = 1000000000000 // 10000 mass

	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, safetype.ErrUint128Underflow, err)
}

func TestTxPool_MaybeAcceptTransaction_CoinbaseTx(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	blk, err := loadNthBlk(25)
	assert.Nil(t, err)

	coinbaseMsgTx := blk.MsgBlock().Transactions[0]
	tx := massutil.NewTx(coinbaseMsgTx)
	_, err = txP.ProcessTransaction(tx, true, true)
	assert.Equal(t, ErrCoinbaseTx, err)
}

func TestTxPool_MaybeAcceptTransaction_ValidateTransactionScripts(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	childTx1, err := getTx("child1")
	if err != nil {
		t.Fatal(err)
	}
	childTx2, err := getTx("child2")
	if err != nil {
		t.Fatal(err)
	}
	msgtx := childTx1

	msgtx.TxIn[0].Witness = childTx2.TxIn[0].Witness
	tx := massutil.NewTx(msgtx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrScriptValidation, err)
}

// make sure it never accept tx without tx input.
func TestTxPool_MaybeAcceptTransaction_NoTxIn(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	msgtx, err := getTx("child1")
	if err != nil {
		t.Fatal(err)
	}
	msgtx.TxIn = make([]*wire.TxIn, 0)
	tx := massutil.NewTx(msgtx)

	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrNoTxInputs, err)
}

func TestTxPool_checkInputsStandard(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	childTx, err := getTx("child1")
	if err != nil {
		t.Fatal(err)
	}

	tx := massutil.NewTx(childTx)

	txStore := txP.FetchInputTransactions(tx, false)

	err = checkInputsStandard(tx, txStore)
	assert.Nil(t, err)
}

func Test_isDust(t *testing.T) {
	txP, close, err := newTxPool(25)
	if err != nil {
		t.Error(err)
	}
	defer close()

	childTx, err := getTx("child2")
	if err != nil {
		t.Fatal(err)
	}

	childTx.TxOut[0].Value = 5879
	tx := massutil.NewTx(childTx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrDust, err)

	childTx.TxOut[0].Value = 5880
	tx = massutil.NewTx(childTx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrScriptValidation, err)
}

//  actual fee < required
func Test_calcMinRequiredTxRelayFee(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	tx, err := getTx("child1")
	assert.Nil(t, err)

	serializedSize := int64(tx.PlainSize())
	minFee, err := CalcMinRequiredTxRelayFee(serializedSize, massutil.MinRelayTxFee())
	assert.Nil(t, err)
	assert.Equal(t, minFee.IntValue(), serializedSize*int64(consensus.MinRelayTxFee/1000))

	// settings for test
	oldLimit := config.FreeTxRelayLimit
	oldFee := massutil.MinRelayTxFee()
	config.FreeTxRelayLimit = 0.0
	massutil.TstSetMinRelayTxFee(10000000000)
	defer func() {
		config.FreeTxRelayLimit = oldLimit
		massutil.TstSetMinRelayTxFee(oldFee.UintValue())
	}()

	_, err = txP.maybeAcceptTransaction(massutil.NewTx(tx), true, true)
	assert.Equal(t, ErrInsufficientFee, err)
}

func TestCountSigOps(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	tx := massutil.NewTx(msgtx)
	numSigOps := CountSigOps(tx)
	assert.Equal(t, numSigOps, len(tx.MsgTx().TxIn)) // child1 is not a coinbase

	_, err = txP.maybeAcceptTransaction(tx, true, true)
	if err != nil {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			tx.Hash(), numSigOps, maxSigOpsPerTx)
		if err.Error() == str {
			t.Logf("success to test numSigOps:%v > maxSigOpsPerTx:%v\n", numSigOps, maxSigOpsPerTx)
		} else {
			t.Error(err)
		}
	}

}

func TestTxPool_IsTransactionInPool(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	txSha := msgtx.TxHash()
	if err != nil {
		t.Error("the txid can`t convert to hash")
	}
	result := txP.IsTransactionInPool(&txSha)
	if result == false {
		_, err := txP.maybeAcceptTransaction(tx, true, true)
		assert.Nil(t, err)
		assert.True(t, txP.IsTransactionInPool(&txSha))
	} else {
		t.Error("err : the tx is not exist in txpool")
	}
}

func TestIsFinalizedTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	msgtx.LockTime = 100
	msgtx.TxIn[0].Sequence = math.MaxUint32 - 1

	tx := massutil.NewTx(msgtx)

	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrUnfinalizedTx, err)
}

func TestTxPool_addTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	tx := massutil.NewTx(msgtx)

	txP.addTransaction(tx, 22, 12, massutil.ZeroAmount(), massutil.ZeroAmount())
	assert.True(t, txP.haveTransaction(tx.Hash()))
}

// txpool.pool == nil
func TestTxPool_addTransaction2(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	txP.pool = nil
	tx := massutil.NewTx(msgtx)

	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrTxPoolNil, err)
}

func TestTxPool_RemoveTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	txP.addTransaction(tx, 22, 22, massutil.ZeroAmount(), massutil.ZeroAmount())
	assert.True(t, txP.haveTransaction(tx.Hash()))

	txP.RemoveTransaction(tx, true)
	assert.False(t, txP.haveTransaction(tx.Hash()))
}

func TestTxPool_RemoveDoubleSpends(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	txP.addTransaction(tx, 12, 12, massutil.ZeroAmount(), massutil.ZeroAmount())
	if txP.haveTransaction(tx.Hash()) != true {
		t.Error("failed to addtransaction")
	}

	txP.RemoveDoubleSpends(tx)
	if txP.haveTransaction(tx.Hash()) == false {
		t.Log("success to removetransaction")
	}
}

func Test_removeScriptFromAddrIndex(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)
	msgtx.TxOut[0].PkScript = []byte{txscript.OP_DATA_45}

	err = txP.removeScriptFromAddrIndex(msgtx.TxOut[0].PkScript, tx)
	assert.Equal(t, txscript.ErrStackShortScript, err)
}

func Test_indexScriptAddressToTx(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)
	msgtx.TxOut[0].PkScript = []byte{txscript.OP_DATA_45}
	err = txP.indexScriptAddressToTx(msgtx.TxOut[0].PkScript, tx)
	assert.Equal(t, txscript.ErrStackShortScript, err)
}

func Test_addTransactionToAddrIndex(t *testing.T) {
	txP, close, err := newTxPool(5)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("grandChild")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	assert.Zero(t, len(txP.addrindex))
	t.Log(len(txP.addrindex), len(msgtx.TxIn), len(msgtx.TxOut))
	err = txP.addTransactionToAddrIndex(tx)
	assert.Nil(t, err)
	assert.NotZero(t, len(txP.addrindex))
}

func TestTxPool_FetchTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	_, err = txP.FetchTransaction(tx.Hash())
	assert.Equal(t, ErrTxExsit, err)
	txP.addTransaction(tx, 22, 22, massutil.ZeroAmount(), massutil.ZeroAmount())
	descTx, err := txP.FetchTransaction(tx.Hash())
	assert.Nil(t, err)
	assert.NotNil(t, descTx)
}

func TestTxPool_FilterTransactionsByAddress(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	_, addresses, _, _, err := txscript.ExtractPkScriptAddrs(msgtx.TxOut[0].PkScript, &config.ChainParams)
	assert.Equal(t, 1, len(addresses))

	_, err = txP.FilterTransactionsByAddress(addresses[0])
	assert.Equal(t, ErrFindTxByAddr, err)
	err = txP.addTransaction(tx, 25, 25, massutil.ZeroAmount(), massutil.ZeroAmount())
	assert.Nil(t, err)
	descTx, err := txP.FilterTransactionsByAddress(addresses[0])
	assert.Nil(t, err)
	assert.Equal(t, 1, len(descTx))
}

func Test_FetchInputTransactions(t *testing.T) {
	txP, close, err := newTxPool(27)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("grandChild")
	assert.Nil(t, err)

	tx := massutil.NewTx(msgtx)
	txStore := txP.FetchInputTransactions(tx, false)
	assert.NotZero(t, len(txStore))
	for _, ts := range txStore {
		if ts.Err != nil && ts.Err.Error() != "requested transaction does not exist" {
			t.Error("failed to test fetchinputtransactions")
		}
	}
}

func TestCheckTransactionInputs(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	totalIn, totalOut := int64(0), int64(0)

	tx := massutil.NewTx(msgtx)
	txStore := txP.FetchInputTransactions(tx, true)

	for _, txIn := range tx.MsgTx().TxIn {
		// Ensure the input is available.
		txInHash := &txIn.PreviousOutPoint.Hash
		data, exists := txStore[*txInHash]
		assert.True(t, exists)

		prevOut := data.Tx.MsgTx().TxOut[txIn.PreviousOutPoint.Index]
		totalIn += prevOut.Value
	}
	for _, txOut := range tx.MsgTx().TxOut {
		totalOut += txOut.Value
	}

	msgtx.TxOut[0].Value += totalIn - totalOut + 1
	tx2 := massutil.NewTx(msgtx)
	_, err = CheckTransactionInputs(tx2, 22, txStore)
	assert.Equal(t, safetype.ErrUint128Underflow, err)
}

func TestTxDesc_StartingPriority(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	txP.addTransaction(tx, 22, 22, massutil.ZeroAmount(), massutil.ZeroAmount())

	// test FetchInputTransactions
	txStore := txP.FetchInputTransactions(tx, false)

	for _, desc := range txP.TxDescs() {
		// test StartingPriority
		_, err := desc.StartingPriority()
		assert.Nil(t, err)
		t.Log("startPriority", desc.startingPriority)

		// test CurrentPriority
		cty, err := desc.CurrentPriority(txStore, 10000)
		assert.Nil(t, err)
		t.Log("currentPriority", cty)

	}
	txP.TxDescs()[0].startingPriority = 100
	_, err = txP.TxDescs()[0].StartingPriority()
	assert.Nil(t, err)
	t.Log(txP.TxDescs()[0].startingPriority)

}

func TestTxPool_ProcessOrphans(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("orphanTxStr")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)
	txP.ProcessOrphans(tx.Hash())
}

func TestTxPool_RemoveOrphan(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("orphanTxStr")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)
	err = txP.orphanTxPool.maybeAddOrphan(tx)
	assert.Nil(t, err)
	assert.True(t, txP.IsOrphanInPool(tx.Hash()))
	txP.RemoveOrphan(tx.Hash())
	assert.False(t, txP.IsOrphanInPool(tx.Hash()))
}

func TestTxPool_ProcessTransaction(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("grandChild")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	msgtx3, err := getTx("child2")
	assert.Nil(t, err)
	tx3 := massutil.NewTx(msgtx3)

	msgtx2, err := getTx("child1")
	assert.Nil(t, err)
	tx2 := massutil.NewTx(msgtx2)

	_, err = txP.ProcessTransaction(tx, false, true)
	assert.Equal(t, ErrProhibitionOrphanTx, err)

	isorphan1, err := txP.ProcessTransaction(tx, true, true)
	assert.True(t, isorphan1)

	msgtx1, err := getTx("orphanTxStr")
	assert.Nil(t, err)
	tx1 := massutil.NewTx(msgtx1)

	_, err = txP.ProcessTransaction(tx1, true, true)
	assert.Nil(t, err)

	_, err = txP.ProcessTransaction(tx3, true, true)
	assert.Nil(t, err)

	isorphan2, err := txP.ProcessTransaction(tx2, true, true)
	assert.False(t, isorphan2)

}

func TestTxPool_TxShas(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Nil(t, err)

	hs := txP.TxShas()
	assert.Equal(t, 1, len(hs))
	assert.Equal(t, tx.Hash(), hs[0])
}

func TestTxPool_LastUpdated(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)
	tx := massutil.NewTx(msgtx)

	t1 := txP.LastUpdated()
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	if err != nil {
		t.Error(err)
	}

	assert.True(t, txP.LastUpdated().After(t1))
}

func TestTxPool_MaybeAcceptTransaction_LockTime(t *testing.T) {
	txP, close, err := newTxPool(25)
	assert.Nil(t, err)
	defer close()

	msgtx, err := getTx("child1")
	assert.Nil(t, err)

	msgtx.LockTime = math.MaxInt64 + 1
	tx := massutil.NewTx(msgtx)
	_, err = txP.maybeAcceptTransaction(tx, true, true)
	assert.Equal(t, ErrImmatureSpend, err)
}
