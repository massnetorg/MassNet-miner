package keystore

import (
	"massnet.org/mass/config"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/pocec"
)

func NewPoCAddress(pubKey *pocec.PublicKey, net *config.Params) ([]byte, massutil.Address, error) {

	//verify nil pointer,avoid panic error
	if pubKey == nil {
		return nil, nil, ErrNilPointer
	}
	scriptHash, pocAddress, err := newPoCAddress(pubKey, net)
	if err != nil {
		logging.CPrint(logging.ERROR, "newPoCAddress failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, nil, err
	}

	return scriptHash, pocAddress, nil
}

func newPoCAddress(pubKey *pocec.PublicKey, net *config.Params) ([]byte, massutil.Address, error) {

	scriptHash := massutil.Hash160(pubKey.SerializeCompressed())

	addressPubKeyHash, err := massutil.NewAddressPubKeyHash(scriptHash, net)
	if err != nil {
		return nil, nil, err
	}

	return scriptHash, addressPubKeyHash, nil
}
