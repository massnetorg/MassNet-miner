package skchia

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil/service"
	"github.com/panjf2000/ants"
	"massnet.org/mass/config"
	"massnet.org/mass/poc/engine.v2"
	massdb_chiapos "massnet.org/mass/poc/engine.v2/massdb/massdb.chiapos"
	"massnet.org/mass/poc/engine.v2/spacekeeper"
)

const (
	typeMassDBChiaPoS      = massdb_chiapos.TypeMassDBChiaPoS
	regMassDBChiaPoS       = `^PLOT-K\d{2}-\d{4}(-\d{2}){4}-[A-F0-9]{64}\.PLOT$`
	suffixMassDBChiaPoS    = ".PLOT"
	TypeSpaceKeeperChiaPoS = "chiapos"
)

var (
	dbType2RegStr  = map[string]string{typeMassDBChiaPoS: regMassDBChiaPoS}
	dbType2SuffixB = map[string]string{typeMassDBChiaPoS: suffixMassDBChiaPoS}
)

// NewSpaceKeeperChiaPoS
func NewSpaceKeeperChiaPoS(args ...interface{}) (spacekeeper.SpaceKeeper, error) {
	cfg, err := parseArgs(args...)
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
		dbType:                typeMassDBChiaPoS,
		workSpaceIndex:        make([]*WorkSpaceMap, 0),
		workSpacePaths:        make(map[string]*WorkSpacePath),
		workSpaceList:         make([]*WorkSpace, 0),
		queue:                 newPlotterQueue(),
		newQueuedWorkSpaceCh:  make(chan *queuedWorkSpace, plotterMaxChanSize),
		workerPool:            workerPool,
		fileWatcher:           func() {},
	}
	sk.BaseService = service.NewBaseService(sk, TypeSpaceKeeperChiaPoS)
	sk.generateInitialIndex = func() error {
		return generateInitialIndex(sk, typeMassDBChiaPoS, regMassDBChiaPoS, suffixMassDBChiaPoS)
	}

	if err = sk.generateInitialIndex(); err != nil {
		return nil, err
	}

	var wsiList []engine.WorkSpaceInfo
	var configureMethod string

	wsiList, err = sk.ConfigureByFlags(engine.SFAll, cfg.Miner.Plot, cfg.Miner.Generate)
	if err != nil && err != ErrSpaceKeeperConfiguredNothing {
		return nil, err
	}
	configureMethod = "ConfigureByFlags"

	logging.CPrint(logging.DEBUG, "try configure spaceKeeper", logging.LogFormat{"content": wsiList, "err": err, "method": configureMethod})

	return sk, nil
}

func parseArgs(args ...interface{}) (*config.Config, error) {
	if len(args) != 1 {
		return nil, spacekeeper.ErrInvalidSKArgs
	}
	cfg, ok := args[0].(*config.Config)
	if !ok {
		return nil, spacekeeper.ErrInvalidSKArgs
	}

	return cfg, nil
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

			// NewWorkSpace
			ws, err := NewWorkSpace(dbDir, filePath)
			if err != nil {
				logging.CPrint(logging.WARN, "fail on NewWorkSpace",
					logging.LogFormat{"filepath": filePath, "err": err})
				continue
			}

			// prevent duplicate MassDB
			if _, ok := sk.workSpaceIndex[allState].Get(ws.SpaceID().String()); ok {
				logging.CPrint(logging.WARN, "duplicate massdb in root dirs",
					logging.LogFormat{"filepath": filePath, "err": ErrMassDBDuplicate})
				ws.Close()
				continue
			}

			// Add workSpace into index
			sk.addWorkSpaceToIndex(ws)
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
		if !strings.HasSuffix(strings.ToUpper(fileName), suffixB) || !regExpB.MatchString(strings.ToUpper(fileName)) {
			continue
		}

		filePath := filepath.Join(dbDir, fileName)

		// NewWorkSpace
		ws, err := NewWorkSpace(dbDir, filePath)
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
		Typ:            TypeSpaceKeeperChiaPoS,
		NewSpaceKeeper: NewSpaceKeeperChiaPoS,
	})
}
