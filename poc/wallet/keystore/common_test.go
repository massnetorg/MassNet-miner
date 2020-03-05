package keystore

import (
	"fmt"
	"os"
	"path/filepath"

	//"github.com/syndtr/goleveldb/leveldb"
	//"github.com/syndtr/goleveldb/leveldb/storage"
	//"massnet.org/mass/poc/wallet/db"
	walletdb "massnet.org/mass/poc/wallet/db"
	_ "massnet.org/mass/poc/wallet/db/ldb"
)

var (
	dbType = "leveldb"
)

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func GetDb(dbName string) (walletdb.DB, func(), error) {
	// Create the root directory for test databases.
	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			err := fmt.Errorf("unable to create test db "+
				"root: %v", err)
			return nil, nil, err
		}
	}
	dbPath := filepath.Join(testDbRoot, dbName)
	if err := os.RemoveAll(dbPath); err != nil {
		err := fmt.Errorf("cannot remove old db: %v", err)
		return nil, nil, err
	}
	db, err := walletdb.CreateDB(dbType, dbPath)
	if err != nil {
		fmt.Println("create db error: ", err)
		return nil, nil, err
	}
	tearDown := func() {
		dbVersionPath := filepath.Join(testDbRoot, dbName+".ver")
		db.Close()
		os.RemoveAll(dbPath)
		os.Remove(dbVersionPath)
		os.RemoveAll(testDbRoot)
	}
	return db, tearDown, nil
}
