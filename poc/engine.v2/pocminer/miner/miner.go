package miner

import (
	"context"
	"encoding/hex"
	"math/big"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/massnetorg/mass-core/blockchain"
	"github.com/massnetorg/mass-core/consensus/forks"
	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil"
	"github.com/massnetorg/mass-core/massutil/service"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/chiawallet"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/poc/engine.v2"
	"massnet.org/mass/poc/engine.v2/spacekeeper"
)

const (
	pocSlot    = poc.PoCSlot
	allowAhead = 1
)

type Chain interface {
	BestBlockNode() *blockchain.BlockNode
	BestBlockHash() *wire.Hash
	BestBlockHeight() uint64
	ProcessBlock(*massutil.Block) (bool, error)
	ChainID() *wire.Hash
	BlockWaiter(height uint64) (<-chan *blockchain.BlockNode, error)
	NewBlockTemplate([]massutil.Address, chan interface{}) error
}

type SyncManager interface {
	IsCaughtUp() bool
	PeerCount() int
}

type ProofTemplate struct {
	chiaQuality *engine.WorkSpaceQuality
	proof       *engine.WorkSpaceProof
	time        time.Time
	quality     *big.Int
}

type PoCMiner struct {
	*service.BaseService
	quit            chan struct{}
	wg              sync.WaitGroup
	allowSolo       bool
	chain           Chain
	syncManager     SyncManager
	SpaceKeeper     spacekeeper.SpaceKeeper
	minedHeight     map[uint64]struct{}
	newBlockCh      chan *wire.Hash
	payoutAddresses []massutil.Address
	getBestProof    func(pocTemplate *blockchain.PoCTemplate, quit chan struct{}) (*ProofTemplate, error)
	keystore        *chiawallet.Keystore
}

func NewPoCMiner(name string, allowSolo bool, chain Chain, syncManager SyncManager, sk spacekeeper.SpaceKeeper,
	newBlockCh chan *wire.Hash, payoutAddresses []massutil.Address, keystore *chiawallet.Keystore) *PoCMiner {
	m := &PoCMiner{
		allowSolo:       allowSolo,
		chain:           chain,
		syncManager:     syncManager,
		SpaceKeeper:     sk,
		minedHeight:     make(map[uint64]struct{}),
		newBlockCh:      newBlockCh,
		payoutAddresses: payoutAddresses,
		keystore:        keystore,
	}
	m.BaseService = service.NewBaseService(m, name)
	return m
}

func (m *PoCMiner) OnStart() error {
	if len(m.payoutAddresses) == 0 {
		logging.CPrint(logging.ERROR, "can not start mining", logging.LogFormat{"err": ErrNoPayoutAddresses})
		return ErrNoPayoutAddresses
	}

	if !m.SpaceKeeper.Started() {
		if err := m.SpaceKeeper.Start(); err != nil {
			return err
		}
	}

	m.quit = make(chan struct{})
	go m.generateBlocks(m.quit)

	logging.CPrint(logging.INFO, "PoC miner started")
	return nil
}

func (m *PoCMiner) OnStop() error {
	close(m.quit)
	m.wg.Wait()

	logging.CPrint(logging.INFO, "PoC miner stopped")
	return nil
}

func (m *PoCMiner) Type() string {
	return m.Name()
}

func (m *PoCMiner) SetPayoutAddresses(addresses []massutil.Address) error {
	if len(addresses) == 0 {
		return ErrNoPayoutAddresses
	}
	m.payoutAddresses = addresses
	return nil
}

func (m *PoCMiner) generateBlocks(quit chan struct{}) {
	m.wg.Add(1)
	defer m.wg.Done()

out:
	for {
		// Quit when miner stopped
		select {
		case <-quit:
			break out
		default:
			time.Sleep(time.Second * pocSlot / 4)
		}

		if peerCount, isCaughtUp := m.syncManager.PeerCount(), m.syncManager.IsCaughtUp(); ((peerCount == 0) || (!isCaughtUp)) && !m.allowSolo {
			logging.CPrint(logging.INFO, "sleep mining for a while to get sync with network",
				logging.LogFormat{
					"peerCount":  peerCount,
					"isCaughtUp": isCaughtUp,
				})
			time.Sleep(time.Second * pocSlot)
			continue out
		}

		// Choose a payment address randomly.
		payoutAddresses := m.payoutAddresses
		if len(payoutAddresses) == 0 {
			logging.CPrint(logging.ERROR, "no valid mining payout addresses", logging.LogFormat{"err": ErrNoPayoutAddresses})
			break out
		}

		// start solve block
		if newBlock, minerReward, err := m.solveBlock(payoutAddresses, quit); err == nil {
			block := massutil.NewBlock(newBlock)
			logging.CPrint(logging.INFO, "submitting mined block",
				logging.LogFormat{
					"height":     block.MsgBlock().Header.Height,
					"hash":       block.Hash().String(),
					"public_key": hex.EncodeToString(block.MsgBlock().Header.PublicKey().SerializeCompressed()),
					"bit_length": block.MsgBlock().Header.Proof.BitLength(),
				})
			m.submitBlock(block, minerReward)
		} else if err != errQuitSolveBlock {
			logging.CPrint(logging.ERROR, "fail to solve block", logging.LogFormat{"err": err})
		}
	}

	logging.CPrint(logging.TRACE, "generate blocks worker done")
}

// submitBlock submits the passed block to network after ensuring it passes all
// of the consensus validation rules.
func (m *PoCMiner) submitBlock(block *massutil.Block, minerReward massutil.Amount) bool {
	// wait for proper time
	for {
		if time.Now().After(block.MsgBlock().Header.Timestamp) {
			break
		}
		time.Sleep(time.Second * pocSlot / 4)
	}

	// Process this block using the same rules as blocks coming from other
	// nodes.  This will in turn relay it to the network like normal.
	isOrphan, err := m.chain.ProcessBlock(block)
	if err != nil {
		logging.CPrint(logging.ERROR, "block submitted via PoC miner rejected",
			logging.LogFormat{"err": err, "hash": block.Hash(), "height": block.Height()})
		return false
	}
	if isOrphan {
		logging.CPrint(logging.ERROR, "block submitted via PoC miner is an orphan",
			logging.LogFormat{"hash": block.Hash(), "height": block.Height()})
		return false
	}

	// The block was accepted.
	logging.CPrint(logging.INFO, "block submitted via PoC miner accepted",
		logging.LogFormat{"hash": block.Hash(), "height": block.Height(), "amount": minerReward})

	// send block to NetSync to broadcast it
	m.newBlockCh <- block.Hash()

	// prevent double-mining
	m.minedHeight[block.Height()] = struct{}{}

	return true
}

func (m *PoCMiner) solveBlock(payoutAddresses []massutil.Address, quit chan struct{}) (*wire.MsgBlock, massutil.Amount, error) {
	var failure = func(err error) (*wire.MsgBlock, massutil.Amount, error) {
		logging.CPrint(logging.INFO, "quit solve block", logging.LogFormat{"err": err})
		return nil, massutil.ZeroAmount(), err
	}

	if !forks.EnforceMASSIP0002(m.chain.BestBlockHeight() + 1) {
		return nil, massutil.Amount{}, errNotMassip2Block
	}

	// Step 1: request for poc & body template
	logging.CPrint(logging.INFO, "Step 1: request for poc & body template")
	templateCh := make(chan interface{}, 2)
	if err := m.chain.NewBlockTemplate(payoutAddresses, templateCh); err != nil {
		return failure(err)
	}

	// Step 2: wait for poc template
	logging.CPrint(logging.INFO, "Step 2: wait for poc template")
	pocTemplateI, err := getTemplate(quit, templateCh, reflect.TypeOf(&blockchain.PoCTemplate{}))
	if err != nil {
		return failure(err)
	}
	pocTemplate := pocTemplateI.(*blockchain.PoCTemplate)

	// Step 3: check for double mining
	logging.CPrint(logging.INFO, "Step 3: check for double mining")
	if _, ok := m.minedHeight[pocTemplate.Height]; ok {
		time.Sleep(time.Second)
		logging.CPrint(logging.INFO, "sleep mining for 1 sec to avoid double mining", logging.LogFormat{"height": pocTemplate.Height})
		return failure(errAvoidDoubleMining)
	}

	// Step 4: get best proof
	logging.CPrint(logging.INFO, "Step 4: get best proof", logging.LogFormat{
		"height":    pocTemplate.Height,
		"previous":  pocTemplate.Previous,
		"timestamp": pocTemplate.Timestamp.Unix(),
		"challenge": pocTemplate.Challenge,
	})
	tProof, err := m.getBestProof(pocTemplate, quit)
	if err != nil {
		if err == errNoValidProof {
			time.Sleep(time.Second * pocSlot)
			logging.CPrint(logging.INFO, "sleep mining for 3 sec to wait for valid poofs", logging.LogFormat{"height": pocTemplate.Height})
		}
		return failure(err)
	}

	// Step 5: wait for chain template
	logging.CPrint(logging.INFO, "Step 5: wait for chain template")
	blockTemplateI, err := getTemplate(quit, templateCh, reflect.TypeOf(&blockchain.BlockTemplate{}))
	if err != nil {
		return failure(err)
	}
	blockTemplate := blockTemplateI.(*blockchain.BlockTemplate)

	// Step 6: assemble full block
	logging.CPrint(logging.INFO, "Step 6: assemble full block")
	block, minerReward, err := assembleFullBlock(blockTemplate, pocTemplate, tProof)
	if err != nil {
		return failure(err)
	}

	// Step 7: get plot signature for poc hash
	logging.CPrint(logging.INFO, "Step 7: get plot signature for poc hash")
	pocHash, err := block.Header.PoCHash()
	if err != nil {
		return failure(err)
	}
	plotSig, err := m.SpaceKeeper.SignHash(tProof.proof.SpaceID, wire.HashH(pocHash[:]))
	if err != nil {
		return failure(err)
	}

	// Step 8: get farmer signature for poc hash
	minerKey, err := m.keystore.GetMinerKeyByPoolPublicKey(tProof.proof.Proof.PoolPublicKey)
	if err != nil {
		return failure(err)
	}
	// verify plot_key
	// TODO: get plot_local_public_key
	//calcPlotPub, err := tProof.proof.Proof.PlotPublicKey.Copy().Add(minerKey.FarmerPublicKey)
	//if err != nil {
	//	return failure(err)
	//}
	//if !calcPlotPub.Equals(tProof.proof.Proof.PlotPublicKey) {
	//	return failure(errors.New("farmer key is not as expected"))
	//}
	signer := chiapos.NewAugSchemeMPL()
	logging.CPrint(logging.INFO, "Step 8: get farmer signature for poc hash")
	farmerSig, err := signer.SignPrepend(minerKey.FarmerPrivateKey, wire.HashB(pocHash[:]), tProof.proof.PublicKey)
	if err != nil {
		return failure(err)
	}
	block.Header.Signature, err = signer.Aggregate(plotSig, farmerSig)
	if err != nil {
		return failure(err)
	}

	logging.CPrint(logging.INFO, "Step 9: return")
	return block, minerReward, nil
}

func getTemplate(quit chan struct{}, ch chan interface{}, typ reflect.Type) (interface{}, error) {
	select {
	case <-quit:
		return nil, errQuitSolveBlock

	case v := <-ch:
		var err error
		switch template := v.(type) {
		case *blockchain.PoCTemplate:
			err = template.Err
			if reflect.TypeOf(v) != typ {
				err = errWrongTemplateCh
			}

		case *blockchain.BlockTemplate:
			err = template.Err
			if reflect.TypeOf(v) != typ {
				err = errWrongTemplateCh
			}

		default:
			err = errWrongTemplateCh
		}

		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

func assembleFullBlock(blockTemplate *blockchain.BlockTemplate, pocTemplate *blockchain.PoCTemplate, tProof *ProofTemplate) (*wire.MsgBlock, massutil.Amount, error) {
	var block = blockTemplate.Block
	var workProof, pocProof = tProof.proof, tProof.proof.Proof

	block.Header.Timestamp = tProof.time
	block.Header.Target = pocTemplate.GetTarget(tProof.time)
	block.Header.Challenge = pocTemplate.Challenge
	block.Header.PubKey = tProof.proof.PublicKey
	block.Header.Proof = poc.NewChiaProof(pocProof)

	coinbaseTx, err := pocTemplate.GetCoinbase(workProof, blockTemplate.TotalFee)
	if err != nil {
		logging.CPrint(logging.WARN, "failed to find binding tx for the pubkey",
			logging.LogFormat{"pubkey": workProof.PublicKey, "err": err})
		return nil, massutil.Amount{}, err
	}

	if coinbaseTx.MsgTx().TxHash() != *blockTemplate.MerkleCache[0] {
		block.Transactions[0] = coinbaseTx.MsgTx()
		block.Header.TransactionRoot = wire.GetMerkleRootFromCache(coinbaseTx.Hash(), blockTemplate.MerkleCache)
		block.Header.WitnessRoot = wire.GetMerkleRootFromCache(coinbaseTx.WitnessHash(), blockTemplate.WitnessMerkleCache)
	}
	n := len(block.Transactions[0].TxOut)

	minerReward, err := massutil.NewAmountFromInt(block.Transactions[0].TxOut[n-1].Value)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to calculate miner reward", logging.LogFormat{"error": err})
		return nil, massutil.Amount{}, err
	}

	return block, minerReward, nil
}

func runStaleMonitor(chain Chain, workSlot *uint64, previousHash *wire.Hash) (cancelFunc func(), staleFunc func() bool, err error) {
	node := chain.BestBlockNode()
	if !node.Hash.IsEqual(previousHash) {
		return nil, nil, errBestChainSwitched
	}
	ch, err := chain.BlockWaiter(node.Height)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on chain.BlockWaiter", logging.LogFormat{"height": node.Height, "err": err})
		return nil, nil, err
	}

	var isBetterNode = func(node *blockchain.BlockNode, newNode *blockchain.BlockNode) bool {
		compare := newNode.CapSum.Cmp(node.CapSum)
		if compare < 0 {
			return false
		} else if compare == 0 {
			if newNode.Timestamp.Before(node.Timestamp) {
				return true
			} else if newNode.Timestamp.Equal(node.Timestamp) {
				return newNode.Quality.Cmp(node.Quality) > 0
			} else {
				return false
			}
		} else {
			return true
		}
	}

	var runMonitor = func(ctx context.Context, wg *sync.WaitGroup, node *blockchain.BlockNode, ch <-chan *blockchain.BlockNode, stale *int32) {
		wg.Add(1)
		var err error
	out:
		for {
			select {
			case <-ctx.Done():
				break out

			case newNode := <-ch:
				if isBetterNode(node, newNode) {
					atomic.StoreInt32(stale, 1)
					logging.CPrint(logging.INFO, "update miner stale slot",
						logging.LogFormat{
							"height":    newNode.Height,
							"staleSlot": node.Timestamp.Unix() / pocSlot,
							"workSlot":  atomic.LoadUint64(workSlot),
						})
					break out
				}
				ch, err = chain.BlockWaiter(node.Height)
				if err != nil {
					atomic.StoreInt32(stale, 1)
					logging.CPrint(logging.WARN, "update miner stale slot and break",
						logging.LogFormat{
							"err":       err,
							"height":    node.Height,
							"staleSlot": uint64(node.Timestamp.Unix()) / pocSlot,
							"workSlot":  atomic.LoadUint64(workSlot),
						})
					break out
				}
			}
		}
		wg.Done()
	}

	var stale int32
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	go runMonitor(ctx, &wg, node, ch, &stale)

	cancelFunc = func() {
		cancel()
		wg.Wait()
	}
	staleFunc = func() bool {
		return atomic.LoadInt32(&stale) != 0
	}
	return cancel, staleFunc, nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
