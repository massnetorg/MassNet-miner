package wallet

import (
	"fmt"
	"os"
	"path/filepath"

	"massnet.org/mass/config/pb"

	"massnet.org/mass/config"

	"massnet.org/mass/poc/wallet/db"

	"massnet.org/mass/poc/wallet/keystore"
)

type PoCWallet struct {
	*keystore.KeystoreManagerForPoC
	store db.DB
}

func NewPoCWallet(cfg *configpb.Config, password []byte) (*PoCWallet, error) {
	dbPath := filepath.Join(cfg.Miner.MinerDir, "keystore")

	var store db.DB
	if fi, err := os.Stat(dbPath); err == nil {
		if !fi.IsDir() {
			return nil, fmt.Errorf("open %s: not a directory", dbPath)
		}
		if store, err = db.OpenDB(cfg.Db.DbType, dbPath); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		if store, err = db.CreateDB(cfg.Db.DbType, dbPath); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	manager, err := keystore.NewKeystoreManagerForPoC(store, password, &config.ChainParams)
	if err != nil {
		store.Close()
		return nil, err
	}
	return &PoCWallet{
		KeystoreManagerForPoC: manager,
		store:                 store,
	}, nil
}

func (wallet *PoCWallet) Close() error {
	return wallet.store.Close()
}
