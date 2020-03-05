package blockchain

import (
	"time"

	"massnet.org/mass/blockchain/orphanpool"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

const (
	// maxOrphanBlocks is the maximum number of orphan blocks that can be
	// queued.
	maxOrphanBlocks = 100
)

// orphanBlock represents a block that we don't yet have the Parent for.  It
// is a normal block plus an expiration time to prevent caching the orphan
// forever.
type orphanBlock struct {
	block      *massutil.Block
	expiration time.Time
}

func (blk *orphanBlock) OrphanPoolID() string {
	return blk.block.Hash().String()
}

type OrphanBlockPool struct {
	prevOrphans  map[wire.Hash][]*orphanBlock
	pool         *orphanpool.AbstractOrphanPool
	oldestOrphan *orphanBlock
}

func newOrphanBlockPool() *OrphanBlockPool {
	return &OrphanBlockPool{
		prevOrphans:  make(map[wire.Hash][]*orphanBlock),
		pool:         orphanpool.NewAbstractOrphanPool(),
		oldestOrphan: nil,
	}
}

// removeOrphanBlock removes the passed orphan block from the orphan pool and
// previous orphan index.
func (ors *OrphanBlockPool) removeOrphanBlock(orphan *orphanBlock) {
	orphanHash := orphan.block.Hash()
	ors.pool.Fetch(orphanHash.String())
}

func (ors *OrphanBlockPool) addOrphanBlock(block *massutil.Block) {
	// Remove expired orphan blocks.
	for _, entry := range ors.pool.Items() {
		oBlock := entry.(*orphanBlock)

		if time.Now().After(oBlock.expiration) {
			ors.removeOrphanBlock(oBlock)
			continue
		}

		if ors.oldestOrphan == nil || oBlock.expiration.Before(ors.oldestOrphan.expiration) {
			ors.oldestOrphan = oBlock
		}
	}

	// Limit orphan blocks to prevent memory exhaustion.
	if ors.pool.Count()+1 > maxOrphanBlocks {
		ors.removeOrphanBlock(ors.oldestOrphan)
		ors.oldestOrphan = nil
	}

	// Insert the block into the orphan map with an expiration time
	// 1 hour from now.
	expiration := time.Now().Add(time.Hour)
	oBlock := &orphanBlock{
		block:      block,
		expiration: expiration,
	}
	ors.pool.Put(oBlock, []string{block.MsgBlock().Header.Previous.String()})

	return
}

func (ors *OrphanBlockPool) isOrphanInPool(hash *wire.Hash) bool {
	return ors.pool.Has(hash.String())
}

func (ors *OrphanBlockPool) getOrphansByPrevious(hash *wire.Hash) []*orphanBlock {
	subs := ors.pool.ReadSubs(hash.String())
	orphans := make([]*orphanBlock, len(subs))

	for i, sub := range subs {
		orphans[i] = sub.(*orphanBlock)
	}

	return orphans
}
