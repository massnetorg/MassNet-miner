package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/massnetorg/mass-core/limits"
	"github.com/massnetorg/mass-core/logging"
	"github.com/urfave/cli/v2"
	"massnet.org/mass/config"
	"massnet.org/mass/fractal"
	"massnet.org/mass/fractal/connection"
	spacekeeper_v2 "massnet.org/mass/poc/engine.v2/spacekeeper"
	"massnet.org/mass/poc/engine.v2/spacekeeper/skchia"
	"massnet.org/mass/version"
)

func fractalMain(cliContext *cli.Context) error {
	var (
		superiorAddress      = cliContext.String("superior")
		collectorPoolAddress = cliContext.String("pool")
		maxCollector         = uint32(cliContext.Uint64("max-collector"))
		noLocalCollector     = cliContext.Bool("no-local-collector")
	)
	// check args
	if collectorPoolAddress == "" && noLocalCollector {
		return fmt.Errorf("must specify either \"-p <collector_pool_listen_address>\" or \"--no-local-collector false\", or both")
	}

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

	// New Superior
	superior, stopSuperior, err := fractal.NewPersistentRemoteSuperior(context.Background(), connection.DialAddress(superiorAddress))
	if err != nil {
		return fmt.Errorf("fail on NewPersistentRemoteSuperior: %v", err)
	}

	// New CollectorPool
	var (
		pool     *fractal.CollectorPool
		stopPool context.CancelFunc
	)
	if collectorPoolAddress != "" {
		pool, stopPool, err = fractal.NewCollectorPool(context.Background(), superior,
			fractal.CollectorPoolListenAddress(collectorPoolAddress), fractal.CollectorPoolMaxCollector(maxCollector))
		if err != nil {
			return fmt.Errorf("fail on NewCollectorPool: %v", err)
		}
		go reportCollectorPool(context.Background(), pool)
	}

	var stopLocalCollector context.CancelFunc
	if !noLocalCollector {
		// New SpaceKeeper
		cfg.Miner.Generate = true
		spaceKeeper, err := spacekeeper_v2.NewSpaceKeeper(skchia.TypeSpaceKeeperChiaPoS, cfg)
		if err != nil {
			logging.CPrint(logging.ERROR, "fail on NewSpaceKeeper", logging.LogFormat{"err": err, "backend": skchia.TypeSpaceKeeperChiaPoS})
			return err
		}
		if err = spaceKeeper.Start(); err != nil {
			logging.CPrint(logging.ERROR, "fail to start SpaceKeeper", logging.LogFormat{"err": err, "backend": skchia.TypeSpaceKeeperChiaPoS})
			return err
		}

		// New LocalCollector
		lc, stopLc := fractal.NewLocalCollector(context.Background(), superior, spaceKeeper)
		logging.CPrint(logging.INFO, "new local_collector", logging.LogFormat{"collector_id": lc.ID()})

		stopLocalCollector = func() {
			stopLc()
			spaceKeeper.Stop()
		}
	}

	// wait
	interruptCh := make(chan os.Signal, 2)
	signal.Notify(interruptCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interruptCh)
	sig := <-interruptCh

	logging.CPrint(logging.INFO, "stopping fractal", logging.LogFormat{"sig": sig})
	stopSuperior()
	if stopPool != nil {
		stopPool()
	}
	if stopLocalCollector != nil {
		stopLocalCollector()
	}

	logging.CPrint(logging.INFO, "Shutdown complete", logging.LogFormat{"err": err})
	return err
}

func reportCollectorPool(ctx context.Context, pool *fractal.CollectorPool) {
	var (
		ticker = time.NewTicker(time.Minute)
		doneCh = ctx.Done()
	)
	for {
		select {
		case <-doneCh:
			return
		case <-ticker.C:
		}
		logging.CPrint(logging.INFO, "collector_pool current count", logging.LogFormat{"count": pool.Count()})
	}
}
