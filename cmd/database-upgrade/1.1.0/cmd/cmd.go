package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"massnet.org/mass/database/ldb"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
)

var (
	defaultDbDir = "./chain"
)

var checkCmd = &cobra.Command{
	Use:   "check [db_dir]",
	Short: "Checks data correctness for version 1.1.0.",
	Long: "Checks data correctness for version 1.1.0.\n" +
		"\nArguments:\n" +
		"  [db_dir]   optional, default './chain'.\n",
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := defaultDbDir
		if len(args) > 0 {
			dir = args[0]
		}
		db, _, _, err := loadDatabase(dir, storage.StorageV3)
		if err != nil {
			return err
		}
		defer db.Close()

		cdb := db.(*ldb.ChainDb)
		return cdb.CheckTxIndex_1_1_0()
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [db_dir]",
	Short: "Upgrades database to version 1.1.0.",
	Long: "Upgrades databsae to version 1.1.0.\n" +
		"\nArguments:\n" +
		"  [db_dir]   optional, default './chain'.\n",
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := defaultDbDir
		if len(args) > 0 {
			dir = args[0]
		}
		db, typ, _, err := loadDatabase(dir, storage.StorageV2)
		if err != nil {
			return err
		}
		defer db.Close()

		if !confirm() {
			return nil
		}

		logging.CPrint(logging.INFO, "upgrade...start", logging.LogFormat{})

		cdb := db.(*ldb.ChainDb)
		err = cdb.Upgrade_1_1_0()
		if err != nil {
			logging.CPrint(logging.ERROR, "upgrade failed", logging.LogFormat{"err": err})
			return err
		}

		err = storage.WriteVersion(filepath.Join(dir, ".ver"), typ, storage.StorageV3)
		if err != nil {
			logging.CPrint(logging.ERROR, "write .ver failed", logging.LogFormat{"err": err})
			return err
		}

		logging.CPrint(logging.INFO, "upgrade...done", logging.LogFormat{})
		return nil
	},
}
