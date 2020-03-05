package mock

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil"
	"massnet.org/mass/poc/wallet/keystore"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
	"math/rand"
	"os"
	"path/filepath"
)

type mockConfig struct {
	rewardAddrNum int
}

var (
	pocKeyChainCodes  map[string]string
	pocKeys           map[string]*pocec.PrivateKey
	pkStrToWalletKey  map[string]*btcec.PrivateKey
	scriptToWalletKey map[string]*btcec.PrivateKey
	basicBlocks       []*wire.MsgBlock
)

func initTemplateData(n int) {
	// initialize maps
	basicBlocks = make([]*wire.MsgBlock, 0, n)
	pocKeys = make(map[string]*pocec.PrivateKey)
	pocKeyChainCodes = make(map[string]string)
	pkStrToWalletKey = make(map[string]*btcec.PrivateKey)
	scriptToWalletKey = make(map[string]*btcec.PrivateKey)

	gopath := os.Getenv("GOPATH")
	root := filepath.Join(gopath, "src/github.com/massnetorg/MassNet-miner")
	file1, err := os.Open(filepath.Join(root, "wire/mock/template_data/block.dat"))
	if err != nil {
		panic(err)
	}
	defer file1.Close()

	scanner := bufio.NewScanner(file1)
	for i := 0; scanner.Scan(); i++ {
		if i >= n+5 {
			break
		}
		buf, err := hex.DecodeString(scanner.Text())
		if err != nil {
			panic(err)
		}
		if i < challengeInterval+1 {
			block, err := massutil.NewBlockFromBytes(buf, wire.Packet)
			if err != nil {
				panic(err)
			}
			basicBlocks = append(basicBlocks, block.MsgBlock())
		} else {
			block, err := massutil.NewBlockFromBytes(buf, wire.Packet)
			if err != nil {
				panic(err)
			}
			header := block.MsgBlock().Header
			if err != nil {
				panic(err)
			}
			blk := wire.NewEmptyMsgBlock()
			blk.Header = header
			basicBlocks = append(basicBlocks, blk)
		}
	}

	// fill pocKeys
	file2, err := os.Open(filepath.Join(root, "wire/mock/template_data/pockey.dat"))
	if err != nil {
		panic(err)
	}
	defer file2.Close()
	scanner = bufio.NewScanner(file2)
	for i := 0; scanner.Scan(); i++ {
		buf, err := hex.DecodeString(scanner.Text())
		if err != nil {
			panic(err)
		}
		priv, pub := pocec.PrivKeyFromBytes(pocec.S256(), buf)
		pubStr := pkStr(pub)
		pocKeys[pubStr] = priv
		// pocKeyChainCodes[pubStr] = rawPoCKeyChainCode[i]
	}

	// fill pkStrToWalletKey
	// for i := 0; i < keyCount; i++ {
	file3, err := os.Open(filepath.Join(root, "wire/mock/template_data/walletkey.dat"))
	if err != nil {
		panic(err)
	}
	defer file3.Close()
	scanner = bufio.NewScanner(file3)
	for scanner.Scan() {
		buf, err := hex.DecodeString(scanner.Text())
		if err != nil {
			panic(err)
		}
		priv, pub := btcec.PrivKeyFromBytes(btcec.S256(), buf)
		pkStrToWalletKey[pkStr(pub)] = priv

		_, witAddr, err := newWitnessScriptAddress([]*btcec.PublicKey{priv.PubKey()}, 1, &config.ChainParams)
		if err != nil {
			panic(err)
		}
		scriptToWalletKey[string(witAddr.ScriptAddress())] = priv
	}
	fmt.Printf("load %d template blocks, %d poc keys, %d wallet keys\n", len(basicBlocks), len(pocKeys), len(pkStrToWalletKey))
}

func generateKey() *btcec.PrivateKey {
	key, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		panic(err)
	}
	pkStrToWalletKey[pkStr(key.PubKey())] = key

	_, addr, err := newWitnessScriptAddress([]*btcec.PublicKey{key.PubKey()}, 1, &config.ChainParams)
	if err != nil {
		panic(err)
	}
	scriptToWalletKey[string(addr.ScriptAddress())] = key

	return key
}

func ensureNumberKeys(targetNum int) {
	currentNum := len(pkStrToWalletKey)
	delta := targetNum - currentNum
	for i := 0; i < delta; i++ {
		generateKey()
	}
}

func randomAddress() (massutil.Address, *btcec.PrivateKey, error) {
	// choose a key from pkStrToWalletKey
	round := 0
	index := rand.Intn(len(pkStrToWalletKey))
	for _, key := range pkStrToWalletKey {
		if round == index {
			// generate Address from key
			_, addr, err := newWitnessScriptAddress([]*btcec.PublicKey{key.PubKey()}, 1, &config.ChainParams)
			if err != nil {
				return nil, nil, err
			}
			return addr, key, nil
		}
		round++
	}
	panic("impossible")
}

func randomPoCAddress() (massutil.Address, *pocec.PrivateKey, error) {
	index := rand.Intn(len(pocKeys))
	round := 0
	for _, key := range pocKeys {
		if round == index {
			_, addr, err := keystore.NewPoCAddress(key.PubKey(), &config.ChainParams)
			if err != nil {
				return nil, nil, err
			}
			return addr, key, nil
		}
		round++
	}
	panic("impossible")
}

type pubKey interface {
	SerializeCompressed() []byte
}

func pkStr(pk pubKey) string {
	return hex.EncodeToString(pk.SerializeCompressed())
}
