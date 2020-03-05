package massutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
)

func TestAddressWitnessScriptHash(t *testing.T) {
	tests := []struct {
		name          string
		address       string
		prog          []byte
		witnessVer    byte
		witnessExtVer byte
		encodeError   error
		decodeError   error
	}{
		{
			name:          "t1",
			address:       "ms1qq75qqxpq9qcrssqgzqvzq2ps8pqqqyqcyq5rqwzqpqgpsgpgxp58q7wksu6",
			prog:          []byte{245, 0, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 13, 14},
			witnessVer:    0x00,
			witnessExtVer: 0x00,
			encodeError:   nil,
			decodeError:   nil,
		},
		{
			name:          "t2",
			address:       "ms1qp75qqxpq9qcrssqgzqvzq2ps8pqqqyqcyq5rqwzqpqgpsgpgxp58qp9x4py",
			prog:          []byte{245, 0, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 13, 14},
			witnessVer:    0x00,
			witnessExtVer: 0x01,
			encodeError:   nil,
			decodeError:   nil,
		},
		{
			name:          "t3-invalid ext ver",
			address:       "ms1qz75qqxpq9qcrssqgzqvzq2ps8pqqqyqcyq5rqwzqpqgpsgpgxp58qfcl600",
			prog:          []byte{245, 0, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 13, 14},
			witnessVer:    0x00,
			witnessExtVer: 0x02,
			encodeError:   nil,
			decodeError:   UnsupportedWitnessExtVerError(2),
		},
		{
			name:          "t4-invalid prog length",
			address:       "",
			prog:          []byte{245, 0, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 13},
			witnessVer:    0x00,
			witnessExtVer: 0x00,
			encodeError:   errors.New("invalid segwit address: invalid data length for witness version 0: 31"),
			decodeError:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			str, err := encodeSegWitAddress("ms", test.witnessVer, test.witnessExtVer, test.prog)
			assert.Equal(t, test.encodeError, err)
			assert.Equal(t, test.address, str)
			if err != nil {
				return
			}

			addr, err := DecodeAddress(test.address, &config.ChainParams)
			assert.Equal(t, test.decodeError, err)
			if err != nil {
				return
			}

			witAddr, ok := addr.(*AddressWitnessScriptHash)
			assert.True(t, ok)
			assert.Equal(t, test.witnessExtVer, witAddr.WitnessExtendVersion())
			assert.Equal(t, test.witnessVer, witAddr.WitnessVersion())
			assert.Equal(t, test.prog, witAddr.ScriptAddress())
		})
	}
}

func TestAddressPubKeyHash(t *testing.T) {
	pkHash := []byte{245, 0, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 3, 4} // 1PLSZpPDFQCyBCp5ftxVDFBsaoz16h2fJZ
	addr, err := NewAddressPubKeyHash(pkHash, &config.ChainParams)
	assert.Nil(t, err)
	fmt.Println(addr.EncodeAddress())

	address, err := DecodeAddress("1PLSZpPDFQCyBCp5ftxVDFBsaoz16h2fJZ", &config.ChainParams)
	assert.Nil(t, nil)
	pkh, ok := address.(*AddressPubKeyHash)
	fmt.Println(ok, pkh.Hash160())
	assert.Equal(t, 20, len(pkh.ScriptAddress()))
}

func TestWitnessAddressAssert(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		isStaking bool
	}{
		{
			name:      "case 1",
			address:   "ms1qq75qqxpq9qcrssqgzqvzq2ps8pqqqyqcyq5rqwzqpqgpsgpgxp58q7wksu6",
			isStaking: false,
		},
		{
			name:      "case 2",
			address:   "ms1qp75qqxpq9qcrssqgzqvzq2ps8pqqqyqcyq5rqwzqpqgpsgpgxp58qp9x4py",
			isStaking: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addr, err := DecodeAddress(test.address, &config.ChainParams)
			assert.Nil(t, err)
			if test.isStaking {
				assert.True(t, IsWitnessStakingAddress(addr))
				assert.False(t, IsWitnessV0Address(addr))
			} else {
				assert.False(t, IsWitnessStakingAddress(addr))
				assert.True(t, IsWitnessV0Address(addr))
			}
		})
	}
}
