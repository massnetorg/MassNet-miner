package blockchain

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
	"massnet.org/mass/txscript"
)

func TestBlockchain_NewBlockTemplate(t *testing.T) {
	bc, _, err := newBlockChain()
	if err != nil {
		t.Fatal("err in newBlockChain", err)
	}

	templateCh := make(chan interface{}, 2)

	pk, _ := btcec.NewPrivateKey(btcec.S256())
	_, address, _ := newWitnessScriptAddress([]*btcec.PublicKey{pk.PubKey()},
		1, massutil.AddressClassWitnessV0, &config.ChainParams)

	err = bc.NewBlockTemplate(address, templateCh)
	if err != nil {
		t.Fatal(err)
	}

	var pocTemplate *PoCTemplate
	var ok bool
	template := <-templateCh
	if pocTemplate, ok = template.(*PoCTemplate); !ok {
		t.Fatal("err type of templateCh")
	}
	assert.Nil(t, pocTemplate.Err)
	assert.Equal(t, uint64(1), pocTemplate.Height)
	assert.Equal(t, *config.ChainParams.GenesisHash, pocTemplate.Previous)

	var blockTemplate *BlockTemplate
	template = <-templateCh
	if blockTemplate, ok = template.(*BlockTemplate); !ok {
		t.Fatal("err type of templateCh")
	}
	assert.Nil(t, blockTemplate.Err)
	assert.Equal(t, uint64(1), blockTemplate.Height)
	assert.Equal(t, *config.ChainParams.GenesisHash, blockTemplate.Block.Header.Previous)
	assert.True(t, blockTemplate.ValidPayAddress)

	scriptClass, pops := txscript.GetScriptInfo(blockTemplate.Block.Transactions[0].TxOut[0].PkScript)
	_, scriptHash, err := txscript.GetParsedOpcode(pops, scriptClass)
	assert.Nil(t, err)
	assert.Equal(t, address.ScriptAddress(), scriptHash[:])
}

func TestCalcRshNum(t *testing.T) {
	testdata := []struct {
		height uint64
		result uint
	}{
		{
			height: 1,
			result: 0,
		},
		{
			height: 13440,
			result: 0,
		},
		{
			height: 13441,
			result: 1,
		},
		{
			height: 40320,
			result: 1,
		},
		{
			height: 40321,
			result: 2,
		},
		{
			height: 94080,
			result: 2,
		},

		{
			height: 94081,
			result: 3,
		},
		{
			height: 201600,
			result: 3,
		},

		{
			height: 201601,
			result: 4,
		},
		{
			height: 416640,
			result: 4,
		},
		{
			height: 416641,
			result: 5,
		},
		{
			height: 846720,
			result: 5,
		},
		{
			height: 846721,
			result: 6,
		},
		{
			height: 1706880,
			result: 6,
		},
		{
			height: 1706881,
			result: 7,
		},
		{
			height: 3427200,
			result: 7,
		},
		{
			height: 3427201,
			result: 8,
		},
		{
			height: 6867840,
			result: 8,
		},
		{
			height: 6867841,
			result: 9,
		},
		{
			height: 13749120,
			result: 9,
		},
		{
			height: 13749121,
			result: 10,
		},
		{
			height: 27511680,
			result: 10,
		},
		{
			height: 27511681,
			result: 11,
		},
		{
			height: 55036800,
			result: 11,
		},
		{
			height: 55036801,
			result: 12,
		},
		{
			height: 110087040,
			result: 12,
		},
		{
			height: 110087041,
			result: 13,
		},
		{
			height: 220187520,
			result: 13,
		},
		{
			height: 220187521,
			result: 14,
		},
		{
			height: 440388480,
			result: 14,
		},
		{
			height: 440388481,
			result: 15,
		},
	}
	for _, data := range testdata {
		res := calcRshNum(data.height)
		if res != data.result {
			t.Errorf("not same, result: %v, expected: %v", res, data.result)
		}
	}
}

func TestSubsidy(t *testing.T) {
	amount102400000000, _ := massutil.NewAmountFromInt(102400000000)
	amount51200000000, _ := massutil.NewAmountFromInt(51200000000)
	amount25600000000, _ := massutil.NewAmountFromInt(25600000000)
	amount12800000000, _ := massutil.NewAmountFromInt(12800000000)
	amount6400000000, _ := massutil.NewAmountFromInt(6400000000)
	amount3200000000, _ := massutil.NewAmountFromInt(3200000000)
	amount1600000000, _ := massutil.NewAmountFromInt(1600000000)
	amount800000000, _ := massutil.NewAmountFromInt(800000000)
	amount400000000, _ := massutil.NewAmountFromInt(400000000)
	amount200000000, _ := massutil.NewAmountFromInt(200000000)
	amount100000000, _ := massutil.NewAmountFromInt(100000000)
	amount50000000, _ := massutil.NewAmountFromInt(50000000)
	amount25000000, _ := massutil.NewAmountFromInt(25000000)
	amount12500000, _ := massutil.NewAmountFromInt(12500000)
	amount6250000, _ := massutil.NewAmountFromInt(6250000)
	amount0 := massutil.ZeroAmount()
	testdata := []struct {
		height uint64
		result massutil.Amount
	}{
		{
			height: 1,
			result: amount102400000000,
		},
		{
			height: 13440,
			result: amount102400000000,
		},
		{
			height: 13441,
			result: amount51200000000,
		},
		{
			height: 40320,
			result: amount51200000000,
		},
		{
			height: 40321,
			result: amount25600000000,
		},
		{
			height: 94080,
			result: amount25600000000,
		},

		{
			height: 94081,
			result: amount12800000000,
		},
		{
			height: 201600,
			result: amount12800000000,
		},

		{
			height: 201601,
			result: amount6400000000,
		},
		{
			height: 416640,
			result: amount6400000000,
		},
		{
			height: 416641,
			result: amount3200000000,
		},
		{
			height: 846720,
			result: amount3200000000,
		},
		{
			height: 846721,
			result: amount1600000000,
		},
		{
			height: 1706880,
			result: amount1600000000,
		},
		{
			height: 1706881,
			result: amount800000000,
		},
		{
			height: 3427200,
			result: amount800000000,
		},
		{
			height: 3427201,
			result: amount400000000,
		},
		{
			height: 6867840,
			result: amount400000000,
		},
		{
			height: 6867841,
			result: amount200000000,
		},
		{
			height: 13749120,
			result: amount200000000,
		},
		{
			height: 13749121,
			result: amount100000000,
		},
		{
			height: 27511680,
			result: amount100000000,
		},
		{
			height: 27511681,
			result: amount50000000,
		},
		{
			height: 55036800,
			result: amount50000000,
		},
		{
			height: 55036801,
			result: amount25000000,
		},
		{
			height: 110087040,
			result: amount25000000,
		},
		{
			height: 110087041,
			result: amount12500000,
		},
		{
			height: 220187520,
			result: amount12500000,
		},
		{
			height: 220187521,
			result: amount6250000,
		},
		{
			height: 440388480,
			result: amount6250000,
		},
		{
			height: 440388481,
			result: amount0,
		},
	}
	for _, data := range testdata {
		//res := calcRshNum(data.height)
		subsidy := baseSubsidy.Rsh(calcRshNum(data.height))
		if subsidy.Lt(minHalvedSubsidy) {
			subsidy = safetype.NewUint128()
		}
		res, err := massutil.NewAmount(subsidy)
		if err != nil {
			t.Fatalf("failed to new amount from subsidy, %v", err)
		}
		if res.Cmp(data.result) != 0 {
			t.Errorf("not same, result: %v, expected: %v", res, data.result)
		}
	}
}

//func TestCheckBinding(t *testing.T) {
//	testdata := []struct {
//		bindingValueList []int64
//		isBinding        bool
//	}{
//		{
//			bindingValueList: []int64{614400},
//			isBinding:        true,
//		},
//		{
//			bindingValueList: []int64{10000, 20000, 30000},
//			isBinding:        false,
//		},
//	}
//}

func TestPayload(t *testing.T) {
	payload, _ := hex.DecodeString("ab7600000000000001000000")
	coinbasePayload := &CoinbasePayload{
		height:           0,
		numStakingReward: 0,
	}
	err := coinbasePayload.SetBytes(payload)
	if err != nil {
		t.Fatalf("failed, %v", err)
	}
	assert.Equal(t, uint32(1), coinbasePayload.NumStakingReward())
	assert.Equal(t, uint64(30379), coinbasePayload.height)
}
