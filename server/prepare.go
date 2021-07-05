package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/massnetorg/mass-core/logging"
	"golang.org/x/crypto/ssh/terminal"
	"massnet.org/mass/config"
	"massnet.org/mass/poc/wallet"
)

var (
	ErrWalletNotExist = errors.New("wallet has not been initialized")
)

func promptPass(reader *bufio.Reader, prefix string, public bool, confirm bool) ([]byte, error) {
	// Prompt the user until they enter a passphrase
	prompt := fmt.Sprintf("%s: ", prefix)
	for {
		fmt.Print(prompt)
		pass, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return nil, err
		}
		fmt.Print("\n")
		pass = bytes.TrimSpace(pass)
		if len(pass) == 0 {
			continue
		}

		if !confirm {
			return pass, nil
		}
		if public {
			fmt.Print("Confirm public passphrase: ")
		} else {
			fmt.Print("Confirm private passphrase: ")
		}

		confirm, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return nil, err
		}
		fmt.Print("\n")
		confirm = bytes.TrimSpace(confirm)
		if !bytes.Equal(pass, confirm) {
			fmt.Println("The entered passphrases do not match")
			continue
		}
		for i := range confirm {
			confirm[i] = 0
		}

		return pass, nil
	}
}

func PrivatePass(reader *bufio.Reader, exist bool) ([]byte, error) {
	// When there is not an existing wallet, simply prompt the user
	// for a new private passphase and return it.
	if !exist {
		return promptPass(reader, "Enter the private passphrase for your new wallet", false, true)
	}
	// At this point, there is an existing legacy wallet, so prompt the user
	// for the existing private passphrase and ensure it properly unlocks
	// the legacy wallet so all of the addresses can later be imported.
	fmt.Println("You have an existing wallet.  All addresses from " +
		"your existing wallet will be imported into the new " +
		"wallet format.")
	for {
		privPass, err := promptPass(reader, "Enter the private "+
			"passphrase for your existing wallet", false, false)
		if err != nil {
			return nil, err
		}
		return privPass, nil
	}
}

func PublicPass(reader *bufio.Reader, privPass []byte) (pubPass []byte, err error) {
	for {
		var exist bool
		if len(privPass) == 0 {
			exist = true
		}
		pubPass, err = promptPass(reader, "Enter the public passphrase for your new wallet", true, !exist)
		if err != nil {
			return nil, err
		}
		if len(privPass) != 0 && bytes.Equal(pubPass, privPass) {
			fmt.Println("public passphrase cannot be the same as private passphrase")
			continue
		}
		break
	}

	return pubPass, nil
}

// checkCreateDir checks that the path exists and is a directory.
// If path does not exist, it is created.
func checkCreateDir(path string) error {
	if fi, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Attempt data directory creation
			if err = os.MkdirAll(path, 0700); err != nil {
				return fmt.Errorf("cannot create directory: %s", err)
			}
		} else {
			return fmt.Errorf("error checking directory: %s", err)
		}
	} else {
		if !fi.IsDir() {
			return fmt.Errorf("path '%s' is not a directory", path)
		}
	}

	return nil
}

func NewOrOpenPoCWallet(cfg *config.Config) (*wallet.PoCWallet, error) {
	pub := []byte(cfg.Miner.PublicPassword)
	if len(pub) == 0 {
		return nil, errors.New("missing pubpass")
	}

	manager, err := wallet.NewPoCWallet(cfg, pub)
	if err != nil {
		return nil, err
	}

	accountIDs := manager.ListKeystoreNames()
	if len(accountIDs) == 0 {
		priv := []byte(cfg.Miner.PrivatePassword)
		if len(priv) == 0 {
			return nil, errors.New("missing privpass")
		}
		logging.CPrint(logging.INFO, "initialize wallet", logging.LogFormat{"path": cfg.Miner.MinerDir})
		accountID, err := manager.NewKeystore(priv, nil, "", config.ChainParams, nil)
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to create new keystore", logging.LogFormat{"path": cfg.Miner.MinerDir, "err": err})
			return nil, err
		}
		logging.CPrint(logging.INFO, "wallet succeffully initialized", logging.LogFormat{"path": cfg.Miner.MinerDir, "accountID": accountID})
	}
	return manager, nil
}
