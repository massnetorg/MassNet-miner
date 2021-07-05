package cmd

import (
	"github.com/massnetorg/mass-core/logging"
	"github.com/spf13/cobra"
	rpcprotobuf "massnet.org/mass/api/proto"
)

// getClientStatusCmd represents the getClientStatus command
var getClientStatusCmd = &cobra.Command{
	Use:   "getclientstatus",
	Short: "Get client status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logging.CPrint(logging.INFO, "get-client-status called")

		resp := &rpcprotobuf.GetClientStatusResponse{}
		ClientCall("/v1/client/status", GET, nil, resp)
		printJSON(resp)
	},
}
