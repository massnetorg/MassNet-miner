package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/poc"
	"github.com/spf13/cobra"
	rpcprotobuf "massnet.org/mass/api/proto"
)

var (
	getBindingListArgFilename   string
	getBindingListFlagOverwrite bool
	getBindingListFlagListAll   bool
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

		resp := &rpcprotobuf.GetClientStatusResponse{}
		ClientCall("/v1/client/status", GET, nil, resp)

		_, err := os.Stat(getBindingListArgFilename)
		if !os.IsNotExist(err) && !getBindingListFlagOverwrite {
			logging.CPrint(logging.ERROR, "cannot overwrite existed file, try again with --overwrite", logging.LogFormat{
				"filename": getBindingListArgFilename,
			})
			return
		}

		var plots []massutil.BindingPlot
		var defaultCount, chiaCount uint64
		switch resp.ServiceMode {
		case "miner_v1":
			plots, err = getBindingListV1(getBindingListFlagListAll)
			defaultCount = uint64(len(plots))
		case "miner_v2":
			plots, err = getBindingListV2(getBindingListFlagListAll)
			chiaCount = uint64(len(plots))
		default:
			logging.CPrint(logging.ERROR, "client does not support binding list", logging.LogFormat{"service_mode": resp.ServiceMode})
			return
		}
		if err != nil {
			logging.CPrint(logging.ERROR, "fail to get binding list", logging.LogFormat{"err": err})
			return
		}

		list := &massutil.BindingList{
			Plots:        plots,
			TotalCount:   defaultCount + chiaCount,
			DefaultCount: defaultCount,
			ChiaCount:    chiaCount,
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

func getBindingListV1(all bool) ([]massutil.BindingPlot, error) {
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

func getBindingListV2(all bool) ([]massutil.BindingPlot, error) {
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
