package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/massnetorg/mass-core/cmdutils"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/wire"
	"github.com/spf13/cobra"
	rpcprotobuf "massnet.org/mass/api/proto"
	minercfg "massnet.org/mass/config"
)

// getBlockCmd represents the getBlock command
var getBlockCmd = &cobra.Command{
	Use:   "getblock <hash|height>",
	Short: "Get block by hash or height",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		path := "/v1/blocks"
		if height, err := strconv.Atoi(id); err != nil {
			// id may be a block hash
			if _, err := wire.NewHashFromStr(id); err != nil {
				logging.CPrint(logging.FATAL, "invalid block hash", logging.LogFormat{"hash|height": id})
			}
			path = strings.Join([]string{path, id}, "/")
			logging.CPrint(logging.INFO, "get-block called", logging.LogFormat{"hash|height": id})
		} else {
			// id is a block height
			path = strings.Join([]string{path, "height", id}, "/")
			logging.CPrint(logging.INFO, "get-block called", logging.LogFormat{"hash|height": height})
		}

		resp := &rpcprotobuf.GetBlockResponse{}
		ClientCall(path, GET, nil, resp)
		printJSON(resp)
	},
}

// getBestBlockCmd represents the getBestBlock command
var getBestBlockCmd = &cobra.Command{
	Use:   "getbestblock",
	Short: "Get best block",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "get-best-block called")

		resp := &rpcprotobuf.GetBestBlockResponse{}
		ClientCall("/v1/blocks/best", GET, nil, resp)
		printJSON(resp)
	},
}

// getBlockHashCmd represents the getBlockHash command
var getBlockHashCmd = &cobra.Command{
	Use:   "getblockhash <height>",
	Short: "Get block hash by height",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		if height, err := strconv.Atoi(id); err != nil {
			logging.CPrint(logging.FATAL, "invalid block height", logging.LogFormat{"height": height})
		} else {
			logging.CPrint(logging.INFO, "get-block-hash called", logging.LogFormat{"height": height})
		}

		resp := &rpcprotobuf.GetBlockHashByHeightResponse{}
		ClientCall(fmt.Sprintf("v1/blocks/hash/%s", id), GET, nil, resp)
		printJSON(resp)
	},
}

// getBlockHeaderCmd represents the getBlockHeader command
var getBlockHeaderCmd = &cobra.Command{
	Use:   "getblockheader <hash>",
	Short: "Get block header by hash",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		if _, err := wire.NewHashFromStr(id); err != nil {
			logging.CPrint(logging.FATAL, "invalid block hash", logging.LogFormat{"hash": id})
		} else {
			logging.CPrint(logging.INFO, "get-block-header called", logging.LogFormat{"hash": id})
		}

		resp := &rpcprotobuf.GetBlockHeaderResponse{}
		ClientCall(fmt.Sprintf("v1/blocks/%s/header", id), GET, nil, resp)
		printJSON(resp)
	},
}

var exportChainCmd = &cobra.Command{
	Use:   "exportchain <datastorePath> <filename> [lastHeight]",
	Short: "Export blockchain into file, *.gz for compression",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		bc, close, err := cmdutils.MakeChain(args[0], true, minercfg.ChainParams)
		if err != nil {
			return err
		}
		defer close()

		start := time.Now()

		height := uint64(0)
		if len(args) == 3 {
			if height, err = strconv.ParseUint(args[2], 10, 64); err != nil {
				return err
			}
		}

		err = cmdutils.ExportChain(bc, args[1], height)
		if err != nil {
			return err
		}
		fmt.Printf("Export done in %v\n", time.Since(start))
		return nil
	},
}

var NoExpensiveValidation bool

var importChainCmd = &cobra.Command{
	Use:   "importchain <filename> <datastorePath>",
	Short: "Import a blockchain file",

	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(2)(cmd, args); err != nil {
			logging.CPrint(logging.ERROR, "wrong argument count", logging.LogFormat{"count": len(args)})
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		bc, close, err := cmdutils.MakeChain(args[1], false, minercfg.ChainParams)
		if err != nil {
			return err
		}
		defer close()

		start := time.Now()

		logging.CPrint(logging.INFO, "Import start", logging.LogFormat{
			"head":   bc.BestBlockHash(),
			"height": bc.BestBlockHeight(),
		})

		err = cmdutils.ImportChain(bc, args[0], NoExpensiveValidation)
		if err != nil {
			logging.CPrint(logging.ERROR, "Import abort", logging.LogFormat{
				"head":   bc.BestBlockHash(),
				"height": bc.BestBlockHeight(),
				"err":    err,
			})
			return err
		}
		fmt.Printf("Import done in %v\n", time.Since(start))
		return nil
	},
}
