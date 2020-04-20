package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"massnet.org/mass/config"
	"massnet.org/mass/database"
	_ "massnet.org/mass/database/ldb"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
	"massnet.org/mass/limits"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	_ "massnet.org/mass/poc/engine/pocminer/miner"
	_ "massnet.org/mass/poc/wallet/db/ldb"
	"massnet.org/mass/version"
)

var (
	cfg             *config.Config
	closeDbChannel  = make(chan struct{})
	shutdownChannel = make(chan struct{})
)

// winServiceMain is only invoked on Windows.  It detects when mass is running
// as a service and reacts accordingly.
var winServiceMain func() (bool, error)

// massMain is the real main function for mass.  It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.  The
// optional serverChan parameter is mainly used by the service code to be
// notified with the server once it is setup so it can gracefully stop it when
// requested from the service control manager.
func massMain(serverChan chan<- *server) error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	tcfg, _, err := config.ParseConfig()
	if err != nil {
		return err
	}

	tcfg, err = config.LoadConfig(tcfg)
	if err != nil {
		return err
	}

	cfg, err = config.CheckConfig(tcfg)
	if err != nil {
		return err
	}

	logging.Init(cfg.Log.LogDir, config.DefaultLoggingFilename, cfg.Log.LogLevel, 1, cfg.Log.DisableCprint)

	// Show version at startup.
	logging.CPrint(logging.INFO, fmt.Sprintf("version %s", version.GetVersion()))

	// Init Miner Keystore
	//if cfg.Init {
	//	return InitPoCWallet(cfg)
	//}

	// Enable http profiling server if requested.
	if cfg.App.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.App.Profile)
			logging.CPrint(logging.INFO, fmt.Sprintf("profile server listening on %s", listenAddr))
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			logging.CPrint(logging.ERROR, fmt.Sprintf("%v", http.ListenAndServe(listenAddr, nil)))
		}()
	}

	// Write cpu profile if requested.
	if cfg.App.CPUProfile != "" {
		f, err := os.Create(cfg.App.CPUProfile)
		if err != nil {
			logging.CPrint(logging.ERROR, "unable to create cpu profile", logging.LogFormat{"err": err})
			return err
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	// Load PoC Wallet
	pocWallet, err := NewOrOpenPoCWallet(cfg)
	if err != nil {
		logging.CPrint(logging.ERROR, "unable to open existing wallet", logging.LogFormat{"error": err})
		return err
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		logging.CPrint(logging.ERROR, "loadBlockDB error", logging.LogFormat{"err": err})
		return err
	}

	MiningAddrList, err := massutil.NewAddressesFromStringList(cfg.Miner.MiningAddr, &config.ChainParams)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to decode mining address", logging.LogFormat{"err": err, "addrList": cfg.Miner.MiningAddr})
		return err
	}
	// Create server and start it.
	server, err := newServer(MiningAddrList, db, pocWallet)
	if err != nil {
		logging.CPrint(logging.ERROR, "unable to start server on address", logging.LogFormat{"addr": cfg.Network.P2P.ListenAddress, "err": err})
		return err
	}
	addInterruptHandler(func() {
		logging.CPrint(logging.INFO, "Stopping server...")
		server.Stop()
		server.WaitForShutdown()

		err = db.Close()
		logging.CPrint(logging.INFO, "Chain db closed", logging.LogFormat{
			"err": err,
		})
		closeDbChannel <- struct{}{}
	})

	server.Start()
	if serverChan != nil {
		serverChan <- server
	}

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done.  This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler.  This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		shutdownChannel <- (<-closeDbChannel)
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	logging.CPrint(logging.INFO, "Shutdown complete")
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	// Call serviceMain on Windows to handle running as a service.  When
	// the return isService flag is true, exit now since we ran as a
	// service.  Otherwise, just fall through to normal operation.
	if runtime.GOOS == "windows" {
		isService, err := winServiceMain()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if isService {
			os.Exit(0)
		}
	}

	// Work around defer not working after os.Exit()
	if err := massMain(nil); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// blockDbNamePrefix is the prefix for the block database name.  The
// database type is appended to this value to form the full block
// database name.
const blockDbNamePrefix = "blocks"

// dbPath returns the path to the block database given a database type.
func blockDbPath(dbType string) string {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + ".db"
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.Db.DataDir, dbName)
	return dbPath
}

// warnMultipeDBs shows a warning if multiple block database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipeDBs() {
	// This is intentionally not using the known db types which depend
	// on the database types compiled into the binary since we want to
	// detect legacy db types as well.
	dbTypes := []string{"leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == cfg.Db.DbType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		dbPath := blockDbPath(dbType)
		if FileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := blockDbPath(cfg.Db.DbType)
		str := fmt.Sprintf("WARNING: There are multiple block chain databases "+
			"using different database types.\nYou probably don't "+
			"want to waste disk space by having more than one.\n"+
			"Your current database is located at [%v].\nThe "+
			"additional database is located at %v", selectedDbPath,
			duplicateDbPaths)
		logging.CPrint(logging.WARN, str)
	}
}

// setupBlockDB loads (or creates when needed) the block database taking into
// account the selected database backend.  It also contains additional logic
// such warning the user if there are multiple databases which consume space on
// the file system and ensuring the regression test database is clean when in
// regression test mode.
func setupBlockDB() (database.Db, bool, error) {
	// The memdb backend does not have a file path associated with it, so
	// handle it uniquely.  We also don't want to worry about the multiple
	// database type warnings when running with the memory database.
	if cfg.Db.DbType == "memdb" {
		logging.CPrint(logging.INFO, "creating block database in memory")
		db, err := database.CreateDB(cfg.Db.DbType)
		if err != nil {
			return nil, false, err
		}
		return db, false, nil
	}

	warnMultipeDBs()

	// The database name is based on the database type.
	dbPath := blockDbPath(cfg.Db.DbType)

	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	//removeRegressionDB(dbPath)

	needUpgrade, err := checkVersion(cfg.Db.DbType, cfg.Db.DataDir)
	if err != nil {
		logging.CPrint(logging.ERROR, "check db version failed", logging.LogFormat{"err": err, "path": dbPath})
		return nil, false, err
	}

	db, err := database.OpenDB(cfg.Db.DbType, dbPath)
	if err != nil {
		logging.CPrint(logging.WARN, "open db failed", logging.LogFormat{"err": err, "path": dbPath})
		db, err = database.CreateDB(cfg.Db.DbType, dbPath)
		if err != nil {
			logging.CPrint(logging.ERROR, "create db failed", logging.LogFormat{"err": err, "path": dbPath})
			return nil, false, err
		}
	}

	return db, needUpgrade, nil
}

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (database.Db, error) {
	db, needUpgrade, err := setupBlockDB()
	if err != nil {
		return nil, err
	}

	// Get the latest block height from the database.
	_, height, err := db.NewestSha()
	if err != nil {
		db.Close()
		return nil, err
	}

	// Insert the appropriate genesis block for the Mass network being
	// connected to if needed.
	if height == math.MaxUint64 {
		genesis := massutil.NewBlock(config.ChainParams.GenesisBlock)
		if err := db.InitByGenesisBlock(genesis); err != nil {
			db.Close()
			return nil, err
		}
		logging.CPrint(logging.INFO, "inserted genesis block", logging.LogFormat{"hash": config.ChainParams.GenesisHash})
		height = 0
	}

	if needUpgrade {
		err = db.IndexPubkbl(false)
		if err != nil {
			db.Close()
			logging.CPrint(logging.ERROR, "IndexPubkbl error", logging.LogFormat{"err": err})
			return nil, err
		}
		err = storage.WriteVersion(filepath.Join(cfg.Db.DataDir, ".ver"), cfg.Db.DbType, storage.CurrentStorageVersion)
		if err != nil {
			db.Close()
			return nil, err
		}
	}

	logging.CPrint(logging.INFO, "block database loaded with block height", logging.LogFormat{"height": height})
	return db, nil
}

// filesExists reports whether the named file or directory exists.
func FileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func checkVersion(dbtype, dir string) (needUpgrade bool, err error) {
	create := false
	_, err = os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(dir, 0700)
		} else {
			return false, err
		}
		create = true
	} else {
		verFile := filepath.Join(dir, ".ver")
		_, err = os.Stat(verFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
			create, err = transV1VerFile(dir, verFile)
			if err != nil {
				return false, err
			}
		}
	}
	err = storage.CheckVersion(dbtype, dir, create)
	if err == storage.ErrUpgradeRequired {
		return true, nil
	}
	return false, err
}

// transition code, it will be removed soon
func transV1VerFile(dir, newPath string) (notExistV1 bool, err error) {
	path := filepath.Join(dir, "blocks.db.ver")
	tp, ver, err := storage.ReadVersion(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, err
	}
	err = storage.WriteVersion(newPath, tp, ver)
	if err != nil {
		return true, err
	}
	return false, nil
}
