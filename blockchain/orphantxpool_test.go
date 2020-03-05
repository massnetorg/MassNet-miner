package blockchain

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

func TestAddOrphan(t *testing.T) {
	pool := newOrphanTxPool()
	pool.maxOrphanTransactions = 2
	tests := []struct {
		tx wire.MsgTx
	}{
		{
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{},
				TxOut:    []*wire.TxOut{},
				LockTime: 0,
				Payload:  []byte{},
			},
		},
		{
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{},
				TxOut:    []*wire.TxOut{},
				LockTime: 10,
				Payload:  []byte{},
			},
		},
		{
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{},
				TxOut:    []*wire.TxOut{},
				LockTime: 20,
				Payload:  []byte{},
			},
		},
	}

	for i, test := range tests {
		pool.addOrphan(massutil.NewTx(&test.tx))
		if i+1 >= pool.maxOrphanTransactions {
			assert.Equal(t, pool.maxOrphanTransactions, pool.pool.Count())
		} else {
			assert.Equal(t, i+1, pool.pool.Count())
		}
	}
}

func TestMaybeAddOrphan(t *testing.T) {
	pool := newOrphanTxPool()
	msgtx := wire.MsgTx{
		Version: 1,
		TxIn:    []*wire.TxIn{},
		TxOut: []*wire.TxOut{{
			Value: 0,
			PkScript: bytes.Repeat([]byte{0x00},
				maxOrphanTxSize+1),
		}},
		LockTime: 0,
		Payload:  []byte{},
	}

	tx := massutil.NewTx(&msgtx)
	err := pool.maybeAddOrphan(tx)
	assert.Equal(t, ErrTxTooBig, err)

	pool.removeOrphan(tx.Hash())
}

func TestIsOrphanInPool(t *testing.T) {
	pool := newOrphanTxPool()
	msgtx := wire.MsgTx{
		Version: 1,
		TxIn:    []*wire.TxIn{},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: bytes.Repeat([]byte{0x00}, 10),
		}},
		LockTime: 0,
		Payload:  []byte{},
	}

	msgtx2 := wire.MsgTx{
		Version: 1,
		TxIn:    []*wire.TxIn{},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: bytes.Repeat([]byte{0x00}, 10),
		}},
		LockTime: 0,
		Payload:  []byte("not added to orphan pool"),
	}

	tx := massutil.NewTx(&msgtx)
	err := pool.maybeAddOrphan(tx)
	if err != nil {
		t.Fatal(err)
	}
	exists := pool.isOrphanInPool(tx.Hash())
	assert.True(t, exists)

	exists = pool.isOrphanInPool(massutil.NewTx(&msgtx2).Hash())
	assert.False(t, exists)
}
