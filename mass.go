package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/massnetorg/mass-core/blockchain/state"
	"github.com/massnetorg/mass-core/database"
	_ "github.com/massnetorg/mass-core/database/ldb"
	"github.com/massnetorg/mass-core/database/storage"
	_ "github.com/massnetorg/mass-core/database/storage/ldbstorage"
	_ "github.com/massnetorg/mass-core/database/storage/rdbstorage"
	"github.com/massnetorg/mass-core/limits"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/trie/massdb"
	"github.com/massnetorg/mass-core/trie/rawdb"
	"massnet.org/mass/config"
	_ "massnet.org/mass/poc/wallet/db/ldb"
	_ "massnet.org/mass/poc/wallet/db/rdb"
	"massnet.org/mass/server"
	"massnet.org/mass/version"
)

type Server interface {
	Start() error
	Stop() error
}

func massMain(cfg *config.Config, serverType string) error {
	logging.Init(cfg.Log.LogDir, config.DefaultLoggingFilename, cfg.Log.LogLevel, 1, cfg.Log.DisableCPrint)

	// Show version at startup.
	logging.CPrint(logging.INFO, fmt.Sprintf("version %s", version.GetVersion()))

	// Enable http profiling srv if requested.
	if cfg.Metrics.ProfilePort != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Metrics.ProfilePort)
			logging.CPrint(logging.INFO, fmt.Sprintf("profile server listening on %s", listenAddr))
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			logging.CPrint(logging.ERROR, fmt.Sprintf("%v", http.ListenAndServe(listenAddr, nil)))
		}()
	}

	bindingDb, err := openStateDatabase(cfg.Datastore.Dir, "bindingstate", 0, 0, "", false)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to load binding database", logging.LogFormat{"err": err})
		return err
	}
	defer bindingDb.Close()

	// Load the block database.
	db, err := setupBlockDB(cfg)
	if err != nil {
		logging.CPrint(logging.ERROR, "loadBlockDB error", logging.LogFormat{"err": err})
		return err
	}
	defer db.Close()

	// payout addresses
	payoutAddresses, err := massutil.NewAddressesFromStringList(cfg.Miner.PayoutAddresses, config.ChainParams)
	if err != nil {
		return err
	}

	// Create srv and start it.
	var srv Server
	switch serverType {
	case "core":
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			srv, err = server.NewCoreServer(cfg, s)
		} else {
			err = err1
		}
	case "m1":
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			srv, err = server.NewMinerServerV1(cfg, s, payoutAddresses)
		} else {
			err = err1
		}
	case "m2":
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			srv, err = server.NewMinerServerV2(cfg, s, payoutAddresses)
		} else {
			err = err1
		}
	}

	if err != nil {
		logging.CPrint(logging.ERROR, "unable to create server on address", logging.LogFormat{"addr": cfg.P2P.ListenAddress, "err": err})
		return err
	}

	if err = srv.Start(); err != nil {
		logging.CPrint(logging.ERROR, "fail to start server", logging.LogFormat{"err": err})
		return err
	}

	interruptCh := make(chan os.Signal, 2)
	signal.Notify(interruptCh, os.Interrupt, syscall.SIGTERM)
	sig := <-interruptCh

	logging.CPrint(logging.INFO, "stopping server", logging.LogFormat{"sig": sig})
	err = srv.Stop()
	logging.CPrint(logging.INFO, "Shutdown complete", logging.LogFormat{"err": err})
	return err
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err = config.CheckConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to check config: %v\n", err)
		os.Exit(1)
	}

	// Work around defer not working after os.Exit()
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "at least two args (core, m1, m2): %v\n", err)
		os.Exit(1)
	}

	if err = massMain(cfg, os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "error in main process: %v\n", err)
		os.Exit(1)
	}
}

// blockDbNamePrefix is the prefix for the block database name.  The
// database type is appended to this value to form the full block
// database name.
const blockDbNamePrefix = "blocks"

// dbPath returns the path to the block database given a database type.
func blockDbPath(dbType, dbDir string) string {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + ".db"
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(dbDir, dbName)
	return dbPath
}

// warnMultipeDBs shows a warning if multiple block database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipeDBs(cfg *config.Config) {
	// This is intentionally not using the known db types which depend
	// on the database types compiled into the binary since we want to
	// detect legacy db types as well.
	dbTypes := []string{"leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == cfg.Datastore.DBType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		dbPath := blockDbPath(dbType, cfg.Datastore.Dir)
		if FileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := blockDbPath(cfg.Datastore.DBType, cfg.Datastore.Dir)
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
func setupBlockDB(cfg *config.Config) (database.Db, error) {
	// The memdb backend does not have a file path associated with it, so
	// handle it uniquely.  We also don't want to worry about the multiple
	// database type warnings when running with the memory database.
	if cfg.Datastore.DBType == "memdb" {
		logging.CPrint(logging.INFO, "creating block database in memory")
		db, err := database.CreateDB(cfg.Datastore.DBType)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	warnMultipeDBs(cfg)

	// Create the new path if needed.
	err := os.MkdirAll(cfg.Datastore.Dir, 0700)
	if err != nil {
		return nil, err
	}
	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	//removeRegressionDB(dbPath)

	if err = storage.CheckCompatibility(cfg.Datastore.DBType, cfg.Datastore.Dir); err != nil {
		logging.CPrint(logging.ERROR, "check storage compatibility failed", logging.LogFormat{"err": err})
		return nil, err
	}

	// The database name is based on the database type.
	dbPath := blockDbPath(cfg.Datastore.DBType, cfg.Datastore.Dir)
	db, err := database.OpenDB(cfg.Datastore.DBType, dbPath, false)
	if err != nil {
		logging.CPrint(logging.WARN, "open db failed", logging.LogFormat{"err": err, "path": dbPath})
		db, err = database.CreateDB(cfg.Datastore.DBType, dbPath)
		if err != nil {
			logging.CPrint(logging.ERROR, "create db failed", logging.LogFormat{"err": err, "path": dbPath})
			return nil, err
		}
	}

	return db, nil
}

// // loadBlockDB opens the block database and returns a handle to it.
// func loadBlockDB(cfg *config.Config) (database.Db, error) {
// 	db, err := setupBlockDB(cfg)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Get the latest block height from the database.
// 	_, height, err := db.NewestSha()
// 	if err != nil {
// 		db.Close()
// 		return nil, err
// 	}

// 	// Insert the appropriate genesis block for the Mass network being
// 	// connected to if needed.
// 	if height == math.MaxUint64 {
// 		genesis := massutil.NewBlock(config2.ChainParams.GenesisBlock)
// 		if err := db.InitByGenesisBlock(genesis); err != nil {
// 			db.Close()
// 			return nil, err
// 		}
// 		logging.CPrint(logging.INFO, "inserted genesis block", logging.LogFormat{"hash": config2.ChainParams.GenesisHash})
// 		height = 0
// 	}

// 	logging.CPrint(logging.INFO, "block database loaded with block height", logging.LogFormat{"height": height})
// 	return db, nil
// }

// filesExists reports whether the named file or directory exists.
func FileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func openStateDatabase(dataDir, name string, cache, handles int, namespace string, readonly bool) (massdb.Database, error) {
	if dataDir == "" {
		return rawdb.NewMemoryDatabase(), nil
	} else {
		path := filepath.Join(dataDir, name)
		return rawdb.NewLevelDBDatabase(path, cache, handles, namespace, readonly)
	}
}
