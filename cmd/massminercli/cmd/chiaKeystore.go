package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"massnet.org/mass/poc/wallet/keystore"
	"massnet.org/mass/poc/wallet/keystore/zero"
)

var (
	importChiaKeystoreUseMnemonic bool
)

// importChiaKeystoreCmd represents the importChiaKeystoreCmd command
var importChiaKeystoreCmd = &cobra.Command{
	Use:   "importchiakeystore <filename>",
	Short: "Create Chia Plots Keystore",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "import-chia-keystore called")
		if err := importChiaKeystore(args[0], importChiaKeystoreUseMnemonic); err != nil {
			logging.CPrint(logging.ERROR, "fail on import-chia-keystore", logging.LogFormat{
				"err":      err,
				"filename": args[0],
			})
		}
	},
}

func importChiaKeystore(filename string, useMnemonic bool) error {
	store, err := chiawallet.NewKeystoreFromFile(filename)
	if os.IsNotExist(err) {
		store = chiawallet.NewEmptyKeystore()
	} else if err != nil {
		return err
	}

	var farmerPriv, poolPriv *chiapos.PrivateKey
	if useMnemonic {
		mnemonic, err := promptPass("mnemonic")
		if err != nil {
			return err
		}
		seed, err := keystore.NewSeedWithErrorChecking(string(mnemonic), "")
		if err != nil {
			return err
		}
		masterPriv, err := chiapos.NewAugSchemeMPL().KeyGen(seed)
		if err != nil {
			return err
		}
		zero.Bytes(seed)
		if farmerPriv, err = chiapos.MasterSkToFarmerSk(masterPriv); err != nil {
			return err
		}
		if poolPriv, err = chiapos.MasterSkToPoolSk(masterPriv); err != nil {
			return err
		}
	} else {
		if farmerPriv, err = readParsePrivateKey("farmer"); err != nil {
			return err
		}
		if poolPriv, err = readParsePrivateKey("pool"); err != nil {
			return err
		}
	}

	id, err := store.SetMinerKey(farmerPriv, poolPriv)
	if err != nil {
		return err
	}
	if err = chiawallet.WriteKeystoreToFile(store, filename, 0666); err != nil {
		return err
	}
	fmt.Println("key imported:", id.String())
	return nil
}

func readParsePrivateKey(name string) (*chiapos.PrivateKey, error) {
	privStr, err := promptPass(name)
	if err != nil {
		return nil, err
	}
	privBytes, err := hex.DecodeString(string(privStr))
	if err != nil {
		return nil, err
	}
	return chiapos.NewPrivateKeyFromBytes(privBytes)
}

func promptPass(prefix string) ([]byte, error) {
	for {
		fmt.Fprint(os.Stdout, fmt.Sprintf("Enter %s:", prefix))
		pwd, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return nil, err
		}
		password := strings.TrimSpace(string(pwd))
		if len(password) == 0 {
			fmt.Println("")
			PromptError("Empty input not allowed")
			continue
		}
		fmt.Println("")
		return []byte(password), nil
	}
}

func PromptError(msg string) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf("%c[1;;31m%s%s%c[0m", 0x1B, "Failed: ", msg, 0x1B))
}

// showChiaKeystoreCmd represents the showChiaKeystoreCmd command
var showChiaKeystoreCmd = &cobra.Command{
	Use:   "showchiakeystore <filename>",
	Short: "Show chia plots keystore",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "show-chia-keystore called")
		if err := showChiaKeystore(args[0]); err != nil {
			logging.CPrint(logging.ERROR, "fail on import-chia-keystore", logging.LogFormat{
				"err":      err,
				"filename": args[0],
			})
		}
	},
}

func showChiaKeystore(filename string) error {
	store, err := chiawallet.NewKeystoreFromFile(filename)
	if os.IsNotExist(err) {
		store = chiawallet.NewEmptyKeystore()
	} else if err != nil {
		return err
	}
	keys := store.GetAllMinerKeys()
	var results []interface{}
	for _, key := range keys {
		results = append(results, struct {
			KeyID           string `json:"key_id"`
			FarmerPublicKey string `json:"farmer_public_key"`
			PoolPublicKey   string `json:"pool_public_key"`
		}{
			KeyID:           chiawallet.NewKeyID(key.FarmerPublicKey, key.PoolPublicKey).String(),
			FarmerPublicKey: hex.EncodeToString(key.FarmerPublicKey.Bytes()),
			PoolPublicKey:   hex.EncodeToString(key.PoolPublicKey.Bytes()),
		})
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
