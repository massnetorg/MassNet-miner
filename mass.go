package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/urfave/cli/v2"
	"massnet.org/mass/config"
	"massnet.org/mass/fractal"
	"massnet.org/mass/mining"
	spacekeeper_v2 "massnet.org/mass/poc/engine.v2/spacekeeper"
	"massnet.org/mass/poc/engine.v2/spacekeeper/skchia"
	_ "massnet.org/mass/poc/wallet/db/ldb"
	_ "massnet.org/mass/poc/wallet/db/rdb"
	"massnet.org/mass/server"
	"massnet.org/mass/version"
)

type Server interface {
	Start() error
	Stop() error
}

var configFilename = "config.json"

func massMain(cliContext *cli.Context, serverType version.ServiceMode) error {
	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		return fmt.Errorf("failed to set limits: %w", err)
	}

	// Load and check config
	cfg, err := config.LoadConfig(configFilename)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err = config.CheckConfig(cfg); err != nil {
		return fmt.Errorf("failed to check config: %w", err)
	}

	logging.Init(cfg.Log.LogDir, config.DefaultLoggingFilename, cfg.Log.LogLevel, 1, cfg.Log.DisableCPrint)

	// Show version at startup.
	logging.CPrint(logging.INFO, fmt.Sprintf("version %s", version.GetVersion()), logging.LogFormat{"config": configFilename})

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
	case version.ModeCore:
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			srv, err = server.NewCoreServer(cfg, s)
		} else {
			err = err1
		}
	case version.ModeMinerV1:
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			srv, err = server.NewMinerServerV1(cfg, s, payoutAddresses)
		} else {
			err = err1
		}
	case version.ModeMinerV2:
		if s, err1 := server.NewServer(cfg, db, state.NewDatabase(bindingDb), config.ChainParams); err1 == nil {
			superior := fractal.NewLocalSuperior()

			var pool *fractal.CollectorPool
			if poolAddress := cliContext.String("pool"); poolAddress != "" {
				var stopPool context.CancelFunc
				maxCollector := uint32(cliContext.Uint64("max-collector"))
				pool, stopPool, err = fractal.NewCollectorPool(context.Background(), superior,
					fractal.CollectorPoolListenAddress(poolAddress), fractal.CollectorPoolMaxCollector(maxCollector))
				if err != nil {
					logging.CPrint(logging.ERROR, "fail on NewCollectorPool", logging.LogFormat{
						"err":            err,
						"listen_address": poolAddress,
						"max_collector":  maxCollector,
					})
					return fmt.Errorf("fail on NewCollectorPool: %v", err)
				}
				defer stopPool()
				go reportCollectorPool(context.Background(), pool)
			}

			var spaceKeeper spacekeeper_v2.SpaceKeeper
			if cliContext.Bool("no-local-collector") {
				spaceKeeper = mining.NewMockedSpaceKeeperV2()
			} else {
				spaceKeeper, err = spacekeeper_v2.NewSpaceKeeper(skchia.TypeSpaceKeeperChiaPoS, cfg)
				if err != nil {
					logging.CPrint(logging.ERROR, "fail on NewSpaceKeeper", logging.LogFormat{"err": err, "backend": skchia.TypeSpaceKeeperChiaPoS})
					return fmt.Errorf("fail on NewSpaceKeeper: %v", err)
				}

				lc, stopLc := fractal.NewLocalCollector(context.Background(), superior, spaceKeeper)
				defer stopLc()
				logging.CPrint(logging.INFO, "new local_collector", logging.LogFormat{"collector_id": lc.ID()})
			}

			srv, err = server.NewMinerServerV2(cfg, s, payoutAddresses, spaceKeeper, superior)
		} else {
			err = err1
		}
	default:
		err = errors.New("unknown service mode, should be one of {core, m1, m2, fractal}")
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
	app := &cli.App{
		Name:  "massminer",
		Usage: "Miner Full Node for MassNet Blockchain.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"V"},
				Usage:   "show version",
				Value:   false,
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"C"},
				Usage:   "specify config filename",
				Value:   "config.json",
			},
		},
		Before: func(context *cli.Context) error {
			if name := context.String("config"); name != "" {
				configFilename = name
			}
			return nil
		},
		Action: func(context *cli.Context) error {
			if context.Bool("version") {
				fmt.Println("massminer", version.GetVersion())
				return nil
			}
			return cli.ShowAppHelp(context)
		},
		Commands: []*cli.Command{
			{
				Name:  "core",
				Usage: "Run massminer in core mode (sync with network but never mine blocks)",
				Action: func(context *cli.Context) error {
					return massMain(context, version.ModeCore)
				},
			},
			{
				Name:  "m1",
				Usage: "Run massminer in m1 mode (mine blocks with native MassDB)",
				Action: func(context *cli.Context) error {
					return massMain(context, version.ModeMinerV1)
				},
			},
			{
				Name:  "m2",
				Usage: "Run massminer in m2 mode (mine blocks with Chia DiskProver)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "pool",
						Aliases: []string{"p"},
						Usage:   "specify collector_pool listen address by <ip:port>",
					},
					&cli.Uint64Flag{
						Name:  "max-collector",
						Usage: "specify the max collector number in collector_pool",
						Value: 2000,
					},
					&cli.BoolFlag{
						Name:  "no-local-collector",
						Usage: "do not run local collector",
						Value: false,
					},
				},
				Action: func(context *cli.Context) error {
					return massMain(context, version.ModeMinerV2)
				},
			},
			{
				Name:  "fractal",
				Usage: "Run in fractal mode",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "superior",
						Aliases: []string{"s"},
						Usage:   "specify remote_superior listen address by <ip:port>",
					},
					&cli.StringFlag{
						Name:    "pool",
						Aliases: []string{"p"},
						Usage:   "specify collector_pool listen address by <ip:port>",
					},
					&cli.Uint64Flag{
						Name:  "max-collector",
						Usage: "specify the max collector number in collector_pool",
						Value: 2000,
					},
					&cli.BoolFlag{
						Name:  "no-local-collector",
						Usage: "do not run local collector",
						Value: false,
					},
				},
				Action: func(context *cli.Context) error {
					return fractalMain(context)
				},
			},
			{
				Name:  "version",
				Usage: "Print version",
				Action: func(context *cli.Context) error {
					fmt.Println("massminer", version.GetVersion())
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
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
