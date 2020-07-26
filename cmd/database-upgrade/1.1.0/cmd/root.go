package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"massnet.org/mass/logging"
)

func init() {
	logging.Init(".", "upgrade-1.1.0", "info", 1, false)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(upgradeCmd)
}

var rootCmd = &cobra.Command{
	Use:   filepath.Base(os.Args[0]),
	Short: "Database Upgrade Tool for MASS Core 1.1.0",
	Long:  "The tool is only applicable for version 1.0.3 or newer.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logging.VPrint(logging.FATAL, "Command failed", logging.LogFormat{"err": err})
	}
}
