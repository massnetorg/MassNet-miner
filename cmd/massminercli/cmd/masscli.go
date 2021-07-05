package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/massnetorg/mass-core/limits"
	"github.com/massnetorg/mass-core/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   filepath.Base(os.Args[0]),
	Short: `Command line client for MASS Miner`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	if err := RootCmd.Execute(); err != nil {
		logging.CPrint(logging.FATAL, "fail on RootCmd.Execute", logging.LogFormat{"err": err})
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	cobra.OnInitialize(initLogger)
	cobra.OnInitialize(logBasicInfo)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./.masscli.json)")
	RootCmd.PersistentFlags().StringVar(&flagAPIURL, "api_url", defaultAPIURL, "API URL")
	RootCmd.PersistentFlags().StringVar(&flagLogDir, "log_dir", defaultLogDir, "directory for log files")
	RootCmd.PersistentFlags().StringVar(&flagLogLevel, "log_level", defaultLogLevel, "level of logs (debug, info, warn, error, fatal, panic)")

	viper.BindPFlag("api_url", RootCmd.PersistentFlags().Lookup("api_url"))
	viper.BindPFlag("log_dir", RootCmd.PersistentFlags().Lookup("log_dir"))
	viper.BindPFlag("log_level", RootCmd.PersistentFlags().Lookup("log_level"))

	RootCmd.AddCommand(getBlockCmd)
	RootCmd.AddCommand(getBestBlockCmd)
	RootCmd.AddCommand(getBlockHashCmd)
	RootCmd.AddCommand(getBlockHeaderCmd)

	RootCmd.AddCommand(configureMinerCmd)
	RootCmd.AddCommand(listSpacesCmd)
	RootCmd.AddCommand(getSpaceCmd)
	RootCmd.AddCommand(plotAllSpacesCmd)
	//RootCmd.AddCommand(plotSpaceCmd)
	RootCmd.AddCommand(mineAllSpacesCmd)
	//RootCmd.AddCommand(mineSpaceCmd)
	RootCmd.AddCommand(stopAllSpacesCmd)
	//RootCmd.AddCommand(stopSpaceCmd)

	RootCmd.AddCommand(getClientStatusCmd)

	RootCmd.AddCommand(exportChainCmd)

	importChainCmd.Flags().BoolVarP(&NoExpensiveValidation, "fast-validation", "f", false, "disable expensive validations")
	RootCmd.AddCommand(importChainCmd)

	RootCmd.AddCommand(importChiaKeystoreCmd)
	importChiaKeystoreCmd.Flags().BoolVarP(&importChiaKeystoreUseMnemonic, "mnemonic", "m", false, "use mnemonic instead of private keys")
	RootCmd.AddCommand(showChiaKeystoreCmd)

	getBindingListCmd.Flags().BoolVarP(&getBindingListFlagOverwrite, "overwrite", "o", false, "overwrite existed file")
	getBindingListCmd.Flags().BoolVarP(&getBindingListFlagListAll, "all", "a", false, "list all plotted files instead of only plotted files")
	RootCmd.AddCommand(getBindingListCmd)
}
