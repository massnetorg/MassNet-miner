package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"massnet.org/mass/database"
	"massnet.org/mass/database/ldb"
	"massnet.org/mass/logging"
)

var checkCmd = &cobra.Command{
	Use:   "check [db_dir]",
	Short: "Checks data correctness.",
	Long: "Checks data correctness.\n" +
		"\nArguments:\n" +
		"  [db_dir]   optional, default './chain'.\n",
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "./chain"
		if len(args) > 0 {
			dir = args[0]
		}
		db, err := loadDatabase(dir)
		if err != nil {
			return err
		}
		defer db.Close()

		return process(db, false)
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [db_dir]",
	Short: "Upgrades or rebuild data.",
	Long: "Upgrades or rebuild data.\n" +
		"\nArguments:\n" +
		"  [db_dir]   optional, default './chain'.\n",
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "./chain"
		if len(args) > 0 {
			dir = args[0]
		}
		db, err := loadDatabase(dir)
		if err != nil {
			return err
		}
		defer db.Close()

		return process(db, true)
	},
}

func process(db database.Db, rebuild bool) error {

	if rebuild {
		// backup confirm
		if !confirm() {
			return nil
		}
	}

	// check or upgrade
	cdb := db.(*ldb.ChainDb)
	err := cdb.CheckStakingTxIndex(rebuild)
	if err != nil {
		logging.CPrint(logging.ERROR, "CheckStakingTxIndex failed", logging.LogFormat{"err": err})
		return err
	}
	logging.CPrint(logging.INFO, "[1/2]...ok", logging.LogFormat{})

	err = cdb.CheckBindingIndex(rebuild)
	if err != nil {
		logging.CPrint(logging.ERROR, "CheckBindingIndex failed", logging.LogFormat{"err": err})
		return err
	}
	logging.CPrint(logging.INFO, "[2/2]...ok", logging.LogFormat{})
	return nil
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
