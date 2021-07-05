package keystore

import (
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/pocec"
	"massnet.org/mass/config"
	"massnet.org/mass/poc/wallet/keystore/hdkeychain"
)

type ManagedAddress struct {
	pubKey         *pocec.PublicKey
	privKey        *pocec.PrivateKey
	scriptHash     []byte
	derivationPath DerivationPath
	address        string
	keystoreName   string
}

func newManagedAddressWithoutPrivKey(keystoreName string, derivationPath DerivationPath, pubKey *pocec.PublicKey, net *config.Params) (*ManagedAddress, error) {
	scriptHash := massutil.Hash160(pubKey.SerializeCompressed())
	addressPubKeyHash, err := massutil.NewAddressPubKeyHash(scriptHash, net)
	if err != nil {
		return nil, err
	}
	return &ManagedAddress{
		pubKey:         pubKey,
		scriptHash:     scriptHash,
		derivationPath: derivationPath,
		address:        addressPubKeyHash.EncodeAddress(),
		keystoreName:   keystoreName,
	}, nil
}

func newManagedAddress(keystoreName string, derivationPath DerivationPath, privKey *pocec.PrivateKey, net *config.Params) (*ManagedAddress, error) {
	ecPubKey := (*pocec.PublicKey)(&privKey.PublicKey)
	managedAddress, err := newManagedAddressWithoutPrivKey(keystoreName, derivationPath, ecPubKey, net)
	if err != nil {
		logging.CPrint(logging.ERROR, "create address failed", logging.LogFormat{"error": err})
		return nil, err
	}
	managedAddress.privKey = privKey
	return managedAddress, nil
}

func newManagedAddressFromExtKey(keystoreName string, derivationPath DerivationPath, extKey *hdkeychain.ExtendedKey, net *config.Params) (*ManagedAddress, error) {
	var managedAddr *ManagedAddress
	if extKey.IsPrivate() {
		privKey, err := extKey.ECPrivKey()
		if err != nil {
			return nil, err
		}

		managedAddr, err = newManagedAddress(
			keystoreName, derivationPath, privKey, net,
		)
		if err != nil {
			return nil, err
		}
	} else {
		pubKey, err := extKey.ECPubKey()
		if err != nil {
			return nil, err
		}

		managedAddr, err = newManagedAddressWithoutPrivKey(
			keystoreName, derivationPath, pubKey, net,
		)
		if err != nil {
			return nil, err
		}
	}
	return managedAddr, nil
}

func (mAddr *ManagedAddress) Account() string {
	return mAddr.keystoreName
}

func (mAddr *ManagedAddress) IsChangeAddr() bool {
	return mAddr.derivationPath.Branch == InternalBranch
}

func (mAddr *ManagedAddress) String() string {
	return mAddr.address
}

func (mAddr *ManagedAddress) ScriptAddress() []byte {
	return mAddr.scriptHash
}

func (mAddr *ManagedAddress) PubKey() *pocec.PublicKey {
	return mAddr.pubKey
}

func (mAddr *ManagedAddress) PrivKey() *pocec.PrivateKey {
	return mAddr.privKey
}
