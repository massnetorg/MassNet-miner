package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/massnetorg/mass-core/errors"
	"github.com/massnetorg/mass-core/logging"
	"github.com/spf13/cobra"
	rpcprotobuf "massnet.org/mass/api/proto"
)

// args for createAddressCmd
var (
	configureMinerArgCapacity          int64
	configureMinerArgCoinbaseAddresses []string
)

// configureMinerCmd represents the configureMiner command
var configureMinerCmd = &cobra.Command{
	Use:   "configureminer <capacity> <coinbase_address_1> <coinbase_address_2> ... <coinbase_address_N>",
	Short: "Configure PoC miner, capacity should be integer ranges in [20, 200]",
	Long: "Configure PoC miner by mining capacity and coinbase addresses.\n" +
		"Capacity means the largest disk space used by miner, represented in MiB.\n" +
		"Coinbase_address means the payout address for mining reward.",
	Example: "configure-miner 50 my_address",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.MinimumNArgs(2)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		capacity, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			logging.CPrint(logging.ERROR, "invalid capacity", logging.LogFormat{"capacity": args[0]})
			return err
		}
		if capacity < 20 || capacity > 200 {
			logging.CPrint(logging.ERROR, "invalid capacity, should be ranges in [20, 200]", logging.LogFormat{"capacity": args[0]})
			return errors.New("invalid capacity, should be ranges in [20, 200]")
		}

		configureMinerArgCapacity, configureMinerArgCoinbaseAddresses = int64(capacity), args[1:]
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		capacity, coinbaseAddresses := configureMinerArgCapacity, configureMinerArgCoinbaseAddresses
		logging.CPrint(logging.INFO, "configure-miner called", logging.LogFormat{"capacity": capacity, "coinbase_addresses": strings.Join(coinbaseAddresses, ",")})

		resp := &rpcprotobuf.ConfigureSpaceKeeperRequest{}
		ClientCall("/v1/spaces", POST, &rpcprotobuf.ConfigureSpaceKeeperRequest{Capacity: uint64(capacity), PayoutAddresses: coinbaseAddresses}, resp)
		printJSON(resp)
	},
}

// listSpacesCmd represents the listSpaces command
var listSpacesCmd = &cobra.Command{
	Use:   "listspaces",
	Short: "List the spaces of miner",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "list-spaces called")

		resp := &rpcprotobuf.WorkSpacesResponse{}
		ClientCall("/v1/spaces", GET, nil, resp)
		printJSON(resp)
	},
}

// getSpaceCmd represents the getSpace command
var getSpaceCmd = &cobra.Command{
	Use:   "getspace <space_id>",
	Short: "Get info of one space specified by 'pk-bitLength'",
	Long: "A mining space is uniquely determined by pubKey and bitLength,\n" +
		"join these two parts by short line as spaceID.",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		spaceID := args[0]
		logging.CPrint(logging.INFO, "get-space called", logging.LogFormat{"space_id": spaceID})

		resp := &rpcprotobuf.WorkSpaceResponse{}
		ClientCall(fmt.Sprintf("/v1/spaces/%s", spaceID), GET, nil, resp)
		printJSON(resp)
	},
}

// plotAllSpacesCmd represents the plotAllSpace command
var plotAllSpacesCmd = &cobra.Command{
	Use:   "plotallspaces",
	Short: "Plot all configured spaces, and convert to mining status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "plot-all-spaces called")

		resp := &rpcprotobuf.ActOnSpaceKeeperResponse{}
		ClientCall("/v1/spaces/plot", POST, &empty.Empty{}, resp)
		printJSON(resp)
	},
}

//// plotSpaceCmd represents the plotSpace command
//var plotSpaceCmd = &cobra.Command{
//	Use:   "plot-space <space_id>",
//	Short: "Plot one space specified by 'pk-bl'",
//	Args: func(cmd *cobra.Command, args []string) error {
//		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
//			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
//			return err
//		}
//		return nil
//	},
//	Run: func(cmd *cobra.Command, args []string) {
//		spaceID := args[0]
//		logging.CPrint(logging.INFO, "plot-space called", logging.LogFormat{"space_id": spaceID})
//
//		resp := &rpcprotobuf.PlotCapacitySpaceResponse{}
//		ClientCall(fmt.Sprintf("/v1/spaces/%s/plot", spaceID), POST, &rpcprotobuf.PlotCapacitySpaceRequest{}, resp)
//		printJSON(resp)
//	},
//}

// mineAllSpacesCmd represents the mineAllSpaces command
var mineAllSpacesCmd = &cobra.Command{
	Use:   "mineallspaces",
	Short: "Mine all configured spaces which already in ready status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "mine-all-spaces called")

		resp := &rpcprotobuf.ActOnSpaceKeeperResponse{}
		ClientCall("/v1/spaces/mine", POST, &empty.Empty{}, resp)
		printJSON(resp)
	},
}

//// mineSpaceCmd represents the mineSpace command
//var mineSpaceCmd = &cobra.Command{
//	Use:   "mine-space <space_id>",
//	Short: "Mine one space specified by 'pk-bl'",
//	Args: func(cmd *cobra.Command, args []string) error {
//		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
//			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
//			return err
//		}
//		return nil
//	},
//	Run: func(cmd *cobra.Command, args []string) {
//		spaceID := args[0]
//		logging.CPrint(logging.INFO, "mine-space called", logging.LogFormat{"space_id": spaceID})
//
//		resp := &rpcprotobuf.MineCapacitySpaceResponse{}
//		ClientCall(fmt.Sprintf("/v1/spaces/%s/mine", spaceID), POST, &rpcprotobuf.MineCapacitySpaceRequest{}, resp)
//		printJSON(resp)
//	},
//}

// stopAllSpacesCmd represents the stopAllSpaces command
var stopAllSpacesCmd = &cobra.Command{
	Use:   "stopallspaces",
	Short: "Stop all configured spaces",
	Long: "Stop all configured spaces, and convert their status:\n" +
		"Convert spaces in mining status to ready status,\n" +
		"Convert spaces in plotting status to registered status.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "stop-all-spaces called")

		resp := &rpcprotobuf.ActOnSpaceKeeperResponse{}
		ClientCall("/v1/spaces/stop", POST, &empty.Empty{}, resp)
		printJSON(resp)
	},
}

//// stopSpaceCmd represents the stopSpace command
//var stopSpaceCmd = &cobra.Command{
//	Use:   "stop-space <space_id>",
//	Short: "Stop one space specified by 'pk-bl'",
//	Args: func(cmd *cobra.Command, args []string) error {
//		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
//			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
//			return err
//		}
//		return nil
//	},
//	Run: func(cmd *cobra.Command, args []string) {
//		spaceID := args[0]
//		logging.CPrint(logging.INFO, "stop-space called", logging.LogFormat{"space_id": spaceID})
//
//		resp := &rpcprotobuf.StopCapacitySpaceResponse{}
//		ClientCall(fmt.Sprintf("/v1/spaces/%s/stop", spaceID), POST, &rpcprotobuf.StopCapacitySpaceRequest{}, resp)
//		printJSON(resp)
//	},
//}
