package blockchain

import (
	"crypto/rand"
	"encoding/binary"

	"massnet.org/mass/blockchain/orphanpool"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

const (
	// defaultMaxOrphanTransactions is the default maximum number of orphan transactions
	// that can be queued.
	defaultMaxOrphanTransactions = 1000
)

type OrphanTxPool struct {
	maxOrphanTransactions int
	pool                  *orphanpool.AbstractOrphanPool
}

func newOrphanTxPool() *OrphanTxPool {
	return &OrphanTxPool{
		maxOrphanTransactions: defaultMaxOrphanTransactions,
		pool:                  orphanpool.NewAbstractOrphanPool(),
	}
}

type orphanTx massutil.Tx

func newOrphanTx(tx *massutil.Tx) *orphanTx {
	return (*orphanTx)(tx)
}

func (tx *orphanTx) OrphanPoolID() string {
	return (*massutil.Tx)(tx).Hash().String()
}

func (tx *orphanTx) Tx() *massutil.Tx {
	return (*massutil.Tx)(tx)
}

func (otp *OrphanTxPool) orphans() []*massutil.Tx {
	entries := otp.pool.Items()
	orphans := make([]*massutil.Tx, 0, len(entries))
	for _, e := range entries {
		orphans = append(orphans, e.(*orphanTx).Tx())
	}
	return orphans
}

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
//
// This function MUST be called with the txPool lock held (for writes).
func (otp *OrphanTxPool) removeOrphan(txHash *wire.Hash) {
	otp.pool.Fetch(txHash.String())
}

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the txPool lock held (for writes).
func (otp *OrphanTxPool) limitNumOrphans() error {
	if otp.pool.Count()+1 > otp.maxOrphanTransactions {
		ids := otp.pool.IDs()

		var randNum [4]byte
		if _, err := rand.Read(randNum[:]); err != nil {
			return err
		}

		index := int(binary.LittleEndian.Uint32(randNum[:])) % len(ids)

		removeHash, err := wire.NewHashFromStr(ids[index])
		if err != nil {
			return err
		}
		otp.removeOrphan(removeHash)
	}

	return nil
}

func (otp *OrphanTxPool) addOrphan(tx *massutil.Tx) {
	if err := otp.limitNumOrphans(); err != nil {
		logging.CPrint(logging.ERROR, "fail on limitNumOrphans", logging.LogFormat{"err": err, "txid": tx.Hash()})
	}

	parents := make([]string, 0)
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		parents = append(parents, originTxHash.String())
	}

	otp.pool.Put(newOrphanTx(tx), parents)

	logging.CPrint(logging.DEBUG, "stored orphan transaction", logging.LogFormat{
		"orphan transaction":     tx.Hash(),
		"total orphan tx number": otp.pool.Count(),
	})
}

func (otp *OrphanTxPool) maybeAddOrphan(tx *massutil.Tx) error {
	serializedLen := tx.MsgTx().PlainSize()
	if serializedLen > maxOrphanTxSize {
		logging.CPrint(logging.ERROR, "orphan transaction size is larger than max allowed size",
			logging.LogFormat{"txSize": serializedLen, "maxOrphanTxSize": maxOrphanTxSize})
		return ErrTxTooBig
	}

	otp.addOrphan(tx)

	return nil
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the txPool RLock held (for reads).
func (otp *OrphanTxPool) isOrphanInPool(hash *wire.Hash) bool {
	return otp.pool.Has(hash.String())
}

func (otp *OrphanTxPool) getOrphansByPrevious(hash *wire.Hash) []*massutil.Tx {
	subs := otp.pool.ReadSubs(hash.String())
	orphans := make([]*massutil.Tx, len(subs))

	for i, sub := range subs {
		orphans[i] = sub.(*orphanTx).Tx()
	}

	return orphans
}
