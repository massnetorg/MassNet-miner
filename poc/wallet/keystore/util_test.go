package keystore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/massnetorg/mass-core/massutil/base58"
)

func TestBase58(t *testing.T) {
	testBytes := []byte("abcd")
	base58String := base58.Encode(testBytes)
	testString := string(testBytes)
	if strings.Compare(base58String, testString) != 0 {
		fmt.Println("base58 string: ", base58String)
		fmt.Println("string: ", testString)
		fmt.Println("not same")
	} else {
		fmt.Println("same")
	}
}

func TestJson(t *testing.T) {
	k := &Keystore{
		Crypto: cryptoJSON{
			Cipher: "a",
		},
		HDpath: hdPath{
			Purpose:          44,
			Coin:             1,
			Account:          1,
			ExternalChildNum: 2,
			InternalChildNum: 0,
		},
	}
	kbytes := k.Bytes()
	GetKeystoreFromJson(kbytes)
}
