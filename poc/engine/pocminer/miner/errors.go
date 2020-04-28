package miner

import "errors"

var (
	errQuitSolveBlock    = errors.New("quit solve block")
	errNoValidProof      = errors.New("can not find valid proof")
	errWrongTemplateCh   = errors.New("unexpected element received from template channel")
	errAvoidDoubleMining = errors.New("sleep mining for 1 second to avoid double mining")
	errBestChainSwitched = errors.New("best chain has been switched")
	ErrNoPayoutAddresses = errors.New("can not mine without payout addresses")
)
