package netsync

import (
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
	"massnet.org/mass/logging"
	"massnet.org/mass/wire"
)

const (
	maxBlockDistance = 64
	maxMsgSetSize    = 128
	newBlockChSize   = 64
)

// blockFetcher is responsible for accumulating block announcements from various peers
// and scheduling them for retrieval.
type blockFetcher struct {
	chain Chain
	peers *peerSet

	newBlockCh chan *blockMsg
	queue      *prque.Prque
	msgSet     map[wire.Hash]*blockMsg
}

//NewBlockFetcher creates a block fetcher to retrieve blocks of the new mined.
func newBlockFetcher(chain Chain, peers *peerSet) *blockFetcher {
	f := &blockFetcher{
		chain:      chain,
		peers:      peers,
		newBlockCh: make(chan *blockMsg, newBlockChSize),
		queue:      prque.New(),
		msgSet:     make(map[wire.Hash]*blockMsg),
	}
	go f.blockProcessor()
	return f
}

func (f *blockFetcher) blockProcessor() {
	for {
		height := f.chain.BestBlockHeight()
		for !f.queue.Empty() {
			msg := f.queue.PopItem().(*blockMsg)
			if msg.block.MsgBlock().Header.Height > height+1 {
				f.queue.Push(msg, -float32(msg.block.MsgBlock().Header.Height))
				break
			}

			f.insert(msg)
			delete(f.msgSet, *msg.block.Hash())
		}
		f.add(<-f.newBlockCh)
	}
}

func (f *blockFetcher) add(msg *blockMsg) {
	bestHeight := f.chain.BestBlockHeight()
	if len(f.msgSet) > maxMsgSetSize || bestHeight > msg.block.MsgBlock().Header.Height || msg.block.MsgBlock().Header.Height-bestHeight > maxBlockDistance {
		return
	}

	blockHash := *msg.block.Hash()
	if _, ok := f.msgSet[blockHash]; !ok {
		f.msgSet[blockHash] = msg
		f.queue.Push(msg, -float32(msg.block.MsgBlock().Header.Height))
		logging.CPrint(logging.DEBUG, "fetcher receive mine block", logging.LogFormat{"height": msg.block.Height()})
	}
}

func (f *blockFetcher) insert(msg *blockMsg) {
	if _, err := f.chain.ProcessBlock(msg.block); err != nil {
		peer := f.peers.getPeer(msg.peerID)
		if peer == nil {
			return
		}

		f.peers.addBanScore(msg.peerID, 20, 0, err.Error())
		return
	}

	if err := f.peers.broadcastMinedBlock(msg.block); err != nil {
		logging.CPrint(logging.ERROR, "fail on fetcher broadcast new block", logging.LogFormat{"err": err})
		return
	}
}

func (f *blockFetcher) processNewBlock(msg *blockMsg) {
	f.newBlockCh <- msg
}
