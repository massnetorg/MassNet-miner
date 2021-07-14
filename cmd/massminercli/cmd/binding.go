package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"github.com/spf13/cobra"
	rpcprotobuf "massnet.org/mass/api/proto"
)

var (
	getBindingListArgFilename     string
	getBindingListFlagOverwrite   bool
	getBindingListFlagListAll     bool
	getBindingListFlagKeystore    string
	getBindingListFlagOffline     bool
	getBindingListFlagPlotType    string
	getBindingListFlagDirectories []string
)

// getBindingListCmd represents the getBindingListCmd command
var getBindingListCmd = &cobra.Command{
	Use:   "getbindinglist <filename>",
	Short: "Get binding target list",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		abs, err := filepath.Abs(args[0])
		if err != nil {
			logging.CPrint(logging.ERROR, "wrong filename format", logging.LogFormat{"err": err, "filename": args[0]})
			return err
		}
		fi, err := os.Stat(abs)
		if err == nil && fi.IsDir() {
			logging.CPrint(logging.ERROR, "filename is a directory", logging.LogFormat{"filename": args[0]})
			return err
		}
		getBindingListArgFilename = abs
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "get-binding-list called")

		_, err := os.Stat(getBindingListArgFilename)
		if !os.IsNotExist(err) && !getBindingListFlagOverwrite {
			logging.CPrint(logging.ERROR, "cannot overwrite existed file, try again with --overwrite", logging.LogFormat{
				"filename": getBindingListArgFilename,
			})
			return
		}

		var list *massutil.BindingList
		if getBindingListFlagOffline {
			list, err = getOfflineBindingList()
		} else {
			list, err = getOnlineBindingList()
		}
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to get binding list", logging.LogFormat{"err": err})
			return
		}
		list = list.RemoveDuplicate()

		if len(list.Plots) == 0 {
			fmt.Println("saved nothing in the binding list")
			return
		}

		data, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to marshal json", logging.LogFormat{
				"err":         err,
				"total_count": list.TotalCount,
			})
			return
		}

		if err = ioutil.WriteFile(getBindingListArgFilename, data, 0666); err != nil {
			logging.CPrint(logging.ERROR, "fail to write into binding list file", logging.LogFormat{
				"err":         err,
				"total_count": list.TotalCount,
				"byte_size":   len(data),
			})
			return
		}
		fmt.Printf("collected %d plot files.\n", list.TotalCount)
	},
}

func getOfflineBindingList() (list *massutil.BindingList, err error) {
	var absDirectories []string
	for _, dir := range getBindingListFlagDirectories {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		absDirectories = append(absDirectories, absDir)
	}

	interruptCh := make(chan os.Signal, 2)
	signal.Notify(interruptCh, os.Interrupt, syscall.SIGTERM)

	logging.CPrint(logging.INFO, "searching for plot files from disk, this may take a while (enter CTRL+C to cancel running)")

	var plots []massutil.BindingPlot
	var defaultCount, chiaCount uint64
	switch getBindingListFlagPlotType {
	case "m1":
		plots, err = getOfflineBindingListV1(interruptCh, absDirectories, getBindingListFlagListAll)
		defaultCount = uint64(len(plots))
	case "m2":
		plots, err = getOfflineBindingListV2(interruptCh, absDirectories, getBindingListFlagListAll, getBindingListFlagKeystore)
		chiaCount = uint64(len(plots))
	default:
		err = errors.New("invalid --type flag, should be m1 (for native MassDB) or m2 (for Chia Plot)")
		return
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get offline binding list", logging.LogFormat{"err": err})
		return
	}

	list = &massutil.BindingList{
		Plots:        plots,
		TotalCount:   defaultCount + chiaCount,
		DefaultCount: defaultCount,
		ChiaCount:    chiaCount,
	}
	return list, nil
}

func getOfflineBindingListV1(interruptCh chan os.Signal, dirs []string, all bool) ([]massutil.BindingPlot, error) {
	regStrB, suffixB := `^\d+_[A-F0-9]{66}_\d{2}\.MASSDB$`, ".MASSDB"
	regExpB, err := regexp.Compile(regStrB)
	if err != nil {
		return nil, err
	}

	var plots []massutil.BindingPlot

	for _, dbDir := range dirs {
		dirFileInfos, err := ioutil.ReadDir(dbDir)
		if err != nil {
			return nil, err
		}

		logging.CPrint(logging.INFO, "searching for native MassDB files", logging.LogFormat{"dir": dbDir})

		for _, fi := range dirFileInfos {
			select {
			case <-interruptCh:
				logging.CPrint(logging.WARN, "cancel searching plot files")
				return nil, nil
			default:
			}

			fileName := fi.Name()
			// try match suffix and `ordinal_pubKey_bitLength.suffix`
			if !strings.HasSuffix(strings.ToUpper(fileName), suffixB) || !regExpB.MatchString(strings.ToUpper(fileName)) {
				continue
			}

			info, err := massutil.NewMassDBInfoV1FromFile(filepath.Join(dbDir, fileName))
			if err != nil {
				logging.CPrint(logging.WARN, "fail to read native massdb info", logging.LogFormat{"err": err})
				continue
			}

			if !info.Plotted && !all {
				continue
			} else {
				target, err := massutil.GetMassDBBindingTarget(info.PublicKey, info.BitLength)
				if err != nil {
					return nil, err
				}
				plots = append(plots, massutil.BindingPlot{
					Target: target,
					Type:   uint8(poc.ProofTypeDefault),
					Size:   uint8(info.BitLength),
				})
			}
		}
	}

	return plots, nil
}

func getOfflineBindingListV2(interruptCh chan os.Signal, dirs []string, all bool, keystoreFile string) ([]massutil.BindingPlot, error) {
	regStrB, suffixB := `^PLOT-K\d{2}-\d{4}(-\d{2}){4}-[A-F0-9]{64}\.PLOT$`, ".PLOT"
	regExpB, err := regexp.Compile(regStrB)
	if err != nil {
		return nil, err
	}

	var keystore *chiawallet.Keystore
	if keystoreFile != "" {
		if keystore, err = chiawallet.NewKeystoreFromFile(keystoreFile); err != nil {
			return nil, err
		}
	}

	var ownablePlot = func(info *massutil.MassDBInfoV2) bool {
		if keystore == nil {
			return true
		}
		if _, err := keystore.GetPoolPrivateKey(info.PoolPublicKey); err != nil {
			return false
		}
		if _, err := keystore.GetFarmerPrivateKey(info.FarmerPublicKey); err != nil {
			return false
		}
		return true
	}

	var plots []massutil.BindingPlot

	for _, dbDir := range dirs {
		dirFileInfos, err := ioutil.ReadDir(dbDir)
		if err != nil {
			return nil, err
		}

		logging.CPrint(logging.INFO, "searching for chia plot files", logging.LogFormat{"dir": dbDir})

		for _, fi := range dirFileInfos {
			select {
			case <-interruptCh:
				logging.CPrint(logging.WARN, "cancel searching plot files")
				return nil, nil
			default:
			}

			fileName := fi.Name()
			if !strings.HasSuffix(strings.ToUpper(fileName), suffixB) || !regExpB.MatchString(strings.ToUpper(fileName)) {
				continue
			}

			info, err := massutil.NewMassDBInfoV2FromFile(filepath.Join(dbDir, fileName))
			if err != nil {
				logging.CPrint(logging.WARN, "fail to read chia plot info", logging.LogFormat{"err": err})
				continue
			}

			if !ownablePlot(info) {
				continue
			} else {
				target, err := massutil.GetChiaPlotBindingTarget(info.PlotID, info.K)
				if err != nil {
					return nil, err
				}
				plots = append(plots, massutil.BindingPlot{
					Target: target,
					Type:   uint8(poc.ProofTypeChia),
					Size:   uint8(info.K),
				})
			}
		}
	}

	return plots, err
}

func getOnlineBindingList() (list *massutil.BindingList, err error) {
	resp := &rpcprotobuf.GetClientStatusResponse{}
	ClientCall("/v1/client/status", GET, nil, resp)

	var plots []massutil.BindingPlot
	var defaultCount, chiaCount uint64
	switch resp.ServiceMode {
	case "miner_v1":
		plots, err = getOnlineBindingListV1(getBindingListFlagListAll)
		defaultCount = uint64(len(plots))
	case "miner_v2":
		plots, err = getOnlineBindingListV2(getBindingListFlagListAll)
		chiaCount = uint64(len(plots))
	default:
		logging.CPrint(logging.ERROR, "client does not support binding list", logging.LogFormat{"service_mode": resp.ServiceMode})
		return
	}
	if err != nil {
		logging.CPrint(logging.ERROR, "fail to get online binding list", logging.LogFormat{"err": err})
		return
	}

	list = &massutil.BindingList{
		Plots:        plots,
		TotalCount:   defaultCount + chiaCount,
		DefaultCount: defaultCount,
		ChiaCount:    chiaCount,
	}
	return list, nil
}

func getOnlineBindingListV1(all bool) ([]massutil.BindingPlot, error) {
	resp := &rpcprotobuf.WorkSpacesResponse{}
	ClientCall("/v1/spaces", GET, nil, resp)
	if resp.ErrorCode != 0 {
		return nil, fmt.Errorf("%s (%d)", resp.ErrorMessage, resp.ErrorCode)
	}

	plots := make([]massutil.BindingPlot, 0, len(resp.Spaces))
	for _, space := range resp.Spaces {
		if all || space.State == "mining" || space.State == "ready" {
			plots = append(plots, massutil.BindingPlot{
				Target: space.BindingTarget,
				Type:   uint8(poc.ProofTypeDefault),
				Size:   uint8(space.BitLength),
			})
		}
	}
	return plots, nil
}

func getOnlineBindingListV2(all bool) ([]massutil.BindingPlot, error) {
	resp := &rpcprotobuf.WorkSpacesResponseV2{}
	ClientCall("/v2/spaces", GET, nil, resp)
	if resp.ErrorCode != 0 {
		return nil, fmt.Errorf("%s (%d)", resp.ErrorMessage, resp.ErrorCode)
	}

	plots := make([]massutil.BindingPlot, 0, len(resp.Spaces))
	for _, space := range resp.Spaces {
		plots = append(plots, massutil.BindingPlot{
			Target: space.BindingTarget,
			Type:   uint8(poc.ProofTypeChia),
			Size:   uint8(space.K),
		})
	}
	return plots, nil
}
