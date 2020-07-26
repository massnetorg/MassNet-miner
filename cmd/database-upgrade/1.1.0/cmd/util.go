package cmd

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
	"massnet.org/mass/logging"
)

var (
	ErrUnsupportedVersion = errors.New("unsupported database version")
	ErrNoNeed             = errors.New("no need to upgrade")
)

func loadDatabase(dbDir string, supportedVersions ...int32) (database.Db, string, int32, error) {
	verPath := filepath.Join(dbDir, ".ver")
	typ, ver, err := storage.ReadVersion(verPath)
	if err != nil {
		logging.CPrint(logging.ERROR, "ReadVersion failed", logging.LogFormat{"err": err, "path": verPath})
		return nil, "", 0, err
	}
	// check version
	supported := false
	for _, supportedVersion := range supportedVersions {
		if ver == supportedVersion {
			supported = true
			break
		}
	}
	if !supported {
		return nil, "", 0, ErrUnsupportedVersion
	}

	blksPath := filepath.Join(dbDir, "blocks.db")
	db, err := database.OpenDB(typ, blksPath)
	if err != nil {
		logging.CPrint(logging.ERROR, "OpenDB failed", logging.LogFormat{"err": err, "path": blksPath})
		return nil, "", 0, err
	}

	_, height, err := db.NewestSha()
	if err != nil {
		return nil, "", 0, err
	}

	if height == math.MaxUint64 {
		return nil, "", 0, ErrNoNeed
	}

	return db, typ, ver, nil
}

func confirm() bool {
	var s string
	fmt.Print("Already backed up your database?(y/n):")
	fmt.Scan(&s)
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "y" || s == "yes" {
		return true
	}
	fmt.Println("Please back up your database before upgrade")
	return false
}
