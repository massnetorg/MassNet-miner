package cmd

import (
	"errors"
	"math"
	"path/filepath"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
	"massnet.org/mass/logging"
)

var (
	ErrUnsupportedVersion = errors.New("unsupported database version")
	ErrNoNeed             = errors.New("no need to upgrade")
)

func loadDatabase(dbDir string) (database.Db, error) {
	verPath := filepath.Join(dbDir, ".ver")
	typ, ver, err := storage.ReadVersion(verPath)
	if err != nil {
		logging.CPrint(logging.ERROR, "ReadVersion failed", logging.LogFormat{"err": err, "path": verPath})
		return nil, err
	}
	if ver > 2 {
		return nil, ErrUnsupportedVersion
	}

	blksPath := filepath.Join(dbDir, "blocks.db")
	db, err := database.OpenDB(typ, blksPath)
	if err != nil {
		logging.CPrint(logging.ERROR, "OpenDB failed", logging.LogFormat{"err": err, "path": blksPath})
		return nil, err
	}

	_, height, err := db.NewestSha()
	if err != nil {
		return nil, err
	}

	if height == math.MaxUint64 {
		return nil, ErrNoNeed
	}

	return db, nil
}
