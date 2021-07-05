package capacity

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil/service"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/pocec"
	"github.com/panjf2000/ants"
	"massnet.org/mass/config"
	"massnet.org/mass/poc/engine"
	massdb_v1 "massnet.org/mass/poc/engine/massdb/massdb.v1"
	"massnet.org/mass/poc/engine/spacekeeper"
)

const (
	typeMassDBV1      = massdb_v1.TypeMassDBV1
	regMassDBV1       = `^\d+_[A-F0-9]{66}_\d{2}\.MASSDB$`
	suffixMassDBV1    = ".MASSDB"
	TypeSpaceKeeperV1 = "spacekeeper.v1"
)

var (
	dbType2RegStr  = map[string]string{typeMassDBV1: regMassDBV1}
	dbType2SuffixB = map[string]string{typeMassDBV1: suffixMassDBV1}
)

// NewSpaceKeeperV1
func NewSpaceKeeperV1(args ...interface{}) (spacekeeper.SpaceKeeper, error) {
	cfg, poCWallet, err := parseArgs(args...)
	if err != nil {
		return nil, err
	}
	workerPool, err := ants.NewPoolPreMalloc(maxPoolWorker)
	if err != nil {
		return nil, err
	}
	sk := &SpaceKeeper{
		allowGenerateNewSpace: true,
		dbDirs:                cfg.Miner.ProofDir,
		dbType:                typeMassDBV1,
		wallet:                poCWallet,
		workSpaceIndex:        make([]*WorkSpaceMap, 0),
		workSpacePaths:        make(map[string]*WorkSpacePath),
		workSpaceList:         make([]*WorkSpace, 0),
		queue:                 newPlotterQueue(),
		newQueuedWorkSpaceCh:  make(chan *queuedWorkSpace, plotterMaxChanSize),
		workerPool:            workerPool,
		fileWatcher:           func() {},
	}
	sk.BaseService = service.NewBaseService(sk, TypeSpaceKeeperV1)
	sk.generateInitialIndex = func() error { return generateInitialIndex(sk, typeMassDBV1, regMassDBV1, suffixMassDBV1) }

	if err = upgradeMassDBFile(sk); err != nil {
		return nil, err
	}
	if err = sk.generateInitialIndex(); err != nil {
		return nil, err
	}

	if cfg.Miner.PrivatePassword != "" {
		if err = poCWallet.Unlock([]byte(cfg.Miner.PrivatePassword)); err != nil {
			return nil, err
		}
		var wsiList []engine.WorkSpaceInfo
		var configureMethod string
		if cfg.Miner.ProofList != "" {
			spaceConf, err := config.DecodeProofList(cfg.Miner.ProofList)
			if err != nil {
				return nil, err
			}
			wsiList, err = sk.ConfigureByBitLength(spaceConf, cfg.Miner.Plot, cfg.Miner.Generate)
			if err != nil {
				return nil, err
			}
			configureMethod = "ConfigureByBitLength"
		} else {
			wsiList, err = sk.ConfigureByFlags(engine.SFAll, cfg.Miner.Plot, cfg.Miner.Generate)
			if err != nil && err != ErrSpaceKeeperConfiguredNothing {
				return nil, err
			}
			configureMethod = "ConfigureByFlags"
		}
		logging.CPrint(logging.DEBUG, "try configure spaceKeeper", logging.LogFormat{"content": wsiList, "err": err, "method": configureMethod})
	}

	return sk, nil
}

func parseArgs(args ...interface{}) (*config.Config, PoCWallet, error) {
	if len(args) != 2 {
		return nil, nil, spacekeeper.ErrInvalidSKArgs
	}
	cfg, ok := args[0].(*config.Config)
	if !ok {
		return nil, nil, spacekeeper.ErrInvalidSKArgs
	}
	wallet, ok := args[1].(PoCWallet)
	if !ok {
		return nil, nil, spacekeeper.ErrInvalidSKArgs
	}

	return cfg, wallet, nil
}

func generateInitialIndex(sk *SpaceKeeper, dbType, regStrB, suffixB string) error {
	for s := engine.FirstState; s <= allState; s++ {
		sk.workSpaceIndex = append(sk.workSpaceIndex, NewWorkSpaceMap())
	}

	regExpB, err := regexp.Compile(regStrB)
	if err != nil {
		return err
	}

	dbDirs, dirFileInfos := prepareDirs(sk.dbDirs)
	sk.dbDirs = dbDirs

	for idx, dbDir := range dbDirs {
		for _, fi := range dirFileInfos[idx] {
			fileName := fi.Name()
			// try match suffix and `ordinal_pubKey_bitLength.suffix`
			if !strings.HasSuffix(strings.ToUpper(fileName), suffixB) || !regExpB.MatchString(strings.ToUpper(fileName)) {
				continue
			}

			filePath := filepath.Join(dbDir, fileName)
			// extract args
			args := strings.Split(fileName[:len(fileName)-len(suffixB)], "_")
			dbIndex, pubKey, bitLength, err := parseMassDBArgsFromString(args[0], args[1], args[2])
			if err != nil {
				logging.CPrint(logging.ERROR, "cannot parse MassDB args from filename", logging.LogFormat{"filepath": filePath, "err": err})
				continue
			}

			// verify db ordinal
			ordinal, exists := sk.wallet.GetPublicKeyOrdinal(pubKey)
			if !exists || dbIndex != int(ordinal) {
				logging.CPrint(logging.WARN, "spaceKeeper wallet does not contain pubKey",
					logging.LogFormat{"filePath": filePath, "err": ErrWalletDoesNotContainPubKey})
				continue
			}

			// prevent duplicate MassDB
			sid := NewSpaceID(int64(ordinal), pubKey, bitLength).String()
			if _, ok := sk.workSpaceIndex[allState].Get(sid); ok {
				logging.CPrint(logging.WARN, "duplicate massdb in root dirs",
					logging.LogFormat{"filepath": filePath, "err": ErrMassDBDuplicate})
				continue
			}

			// NewWorkSpace
			ws, err := NewWorkSpace(dbType, dbDir, int64(ordinal), pubKey, bitLength)
			if err != nil {
				logging.CPrint(logging.WARN, "fail on NewWorkSpace",
					logging.LogFormat{"filepath": filePath, "err": err, "db_index": dbIndex})
				continue
			}

			// Add workSpace into index
			sk.addWorkSpaceToIndex(ws)
		}
	}

	return nil
}

func upgradeMassDBFile(sk *SpaceKeeper) error {
	var oldRegStrB, oldRegStrA = `^[A-F0-9]{66}-\d{2}-B\.MASSDB$`, `^[A-F0-9]{66}-\d{2}-A\.MASSDB$`
	regExpB, err := regexp.Compile(oldRegStrB)
	if err != nil {
		return err
	}
	regExpA, err := regexp.Compile(oldRegStrA)
	if err != nil {
		return err
	}

	var rename = func(dir, filename, tagA string) {
		filePath := filepath.Join(dir, filename)
		// extract args
		args := strings.Split(filename[:len(filename)-len(".MASSDB")], "-")
		_, pubKey, bitLength, err := parseMassDBArgsFromString("0", args[0], args[1])
		if err != nil {
			logging.CPrint(logging.TRACE, "cannot parse MassDB args from filename",
				logging.LogFormat{"filepath": filePath, "err": err})
			return
		}
		// get ordinal
		ordinal, exists := sk.wallet.GetPublicKeyOrdinal(pubKey)
		if !exists {
			logging.CPrint(logging.TRACE, "spaceKeeper wallet does not contain pubKey",
				logging.LogFormat{"filepath": filePath, "err": ErrWalletDoesNotContainPubKey})
			return
		}
		// make new filename
		newFilename := fmt.Sprintf("%d_%s_%d%s.massdb", ordinal, args[0], bitLength, tagA)
		newFilepath := filepath.Join(dir, newFilename)
		if err = os.Rename(filePath, newFilepath); err != nil {
			logging.CPrint(logging.ERROR, "fail to rename massdb",
				logging.LogFormat{"dir": dir, "old_name": filename, "new_name": newFilename, "err": err})
		}
	}

	dbDirs, dirFileInfos := prepareDirs(sk.dbDirs)
	sk.dbDirs = dbDirs
	for idx, dbDir := range dbDirs {
		for _, fi := range dirFileInfos[idx] {
			filename := fi.Name()
			// try match `*.massdb`
			if !strings.HasSuffix(strings.ToUpper(filename), ".MASSDB") {
				continue
			}
			if regExpB.MatchString(strings.ToUpper(filename)) {
				rename(dbDir, filename, "")
			}
			if regExpA.MatchString(strings.ToUpper(filename)) {
				rename(dbDir, filename, "_a")
			}
		}
	}
	return nil
}

func prepareDirs(dirs []string) ([]string, [][]os.FileInfo) {
	resultDir := make([]string, 0)
	resultDirFileInfo := make([][]os.FileInfo, 0)
	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to get abs path", logging.LogFormat{"err": err, "dir": dir})
			continue
		}
		if fi, err := os.Stat(absDir); err != nil {
			if !os.IsNotExist(err) {
				logging.CPrint(logging.ERROR, "fail to get file stat", logging.LogFormat{"err": err})
				continue
			}
			if err = os.MkdirAll(absDir, 0700); err != nil {
				logging.CPrint(logging.ERROR, "mkdir failed", logging.LogFormat{"dir": absDir, "err": err})
				continue
			}
		} else if !fi.IsDir() {
			logging.CPrint(logging.ERROR, "not directory", logging.LogFormat{"dir": absDir})
			continue
		}
		if fis, err := ioutil.ReadDir(absDir); err != nil {
			logging.CPrint(logging.ERROR, "fail to read dir", logging.LogFormat{"err": err, "dir": absDir})
		} else {
			resultDir = append(resultDir, absDir)
			resultDirFileInfo = append(resultDirFileInfo, fis)
		}
	}
	return resultDir, resultDirFileInfo
}

func parseMassDBArgsFromString(ordinalStr, pkStr, blStr string) (ordinal int, pubKey *pocec.PublicKey, bitLength int, err error) {
	// parse ordinal
	ordinal, err = strconv.Atoi(ordinalStr)
	if err != nil {
		return
	}
	// parse public key
	pubKeyBytes, err := hex.DecodeString(pkStr)
	if err != nil {
		return
	}
	pubKey, err = pocec.ParsePubKey(pubKeyBytes, pocec.S256())
	if err != nil {
		return
	}
	// parse bit length
	bitLength, err = strconv.Atoi(blStr)
	if err != nil {
		return
	}
	if !poc.ProofTypeDefault.EnsureBitLength(bitLength) {
		err = poc.ErrProofInvalidBitLength
		return
	}

	return
}

func peekMassDBInfosByDir(dbDir, dbType string) ([]engine.WorkSpaceInfo, error) {
	regStrB, suffixB := dbType2RegStr[dbType], dbType2SuffixB[dbType]
	regExpB, err := regexp.Compile(regStrB)
	if err != nil {
		return nil, err
	}
	dirFileInfos, err := ioutil.ReadDir(dbDir)
	if err != nil {
		return nil, err
	}
	var wsiList []engine.WorkSpaceInfo
	for _, fi := range dirFileInfos {
		fileName := fi.Name()
		// try match suffix and `ordinal_pubKey_bitLength.suffix`
		if !strings.HasSuffix(strings.ToUpper(fileName), suffixB) || !regExpB.MatchString(strings.ToUpper(fileName)) {
			continue
		}

		// extract args
		args := strings.Split(fileName[:len(fileName)-len(suffixB)], "_")
		dbIndex, pubKey, bitLength, err := parseMassDBArgsFromString(args[0], args[1], args[2])
		if err != nil {
			continue
		}

		// NewWorkSpace
		ws, err := NewWorkSpace(dbType, dbDir, int64(dbIndex), pubKey, bitLength)
		if err != nil {
			continue
		}
		wsiList = append(wsiList, ws.Info())
		ws.Close()
	}
	return wsiList, nil
}

func init() {
	spacekeeper.AddSpaceKeeperBackend(spacekeeper.SKBackend{
		Typ:            TypeSpaceKeeperV1,
		NewSpaceKeeper: NewSpaceKeeperV1,
	})
}
