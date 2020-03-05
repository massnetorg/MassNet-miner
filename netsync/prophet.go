package netsync

import (
	"time"

	"massnet.org/mass/errors"
	"massnet.org/mass/massutil"
)

// Reject block from far future (3 seconds for now)
func preventBlockFromFuture(block *massutil.Block) error {
	if time.Now().Add(3 * time.Second).Before(block.MsgBlock().Header.Timestamp) {
		return errors.Wrap(errPeerMisbehave, "preventBlockFromFuture")
	}
	return nil
}

// Reject blocks from far future (3 seconds for now)
func preventBlocksFromFuture(blocks []*massutil.Block) error {
	for _, block := range blocks {
		if preventBlockFromFuture(block) != nil {
			return errors.Wrap(errPeerMisbehave, "preventBlocksFromFuture")
		}
	}
	return nil
}
