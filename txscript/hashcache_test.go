package txscript

import (
	"testing"

	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

var msgtx = wire.NewMsgTx()

func TestHashCache_AddSigHashes(t *testing.T) {
	hashCache := NewHashCache(10)
	if len(hashCache.sigHashes) != 0 {
		t.Errorf("new hashCache error")
	}
	hashCache.AddSigHashes(msgtx)
	if len(hashCache.sigHashes) != 1 {
		t.Errorf("add hashCache error")
	}
}

func TestHashCache_ContainsHashes(t *testing.T) {
	hashCache := NewHashCache(10)
	if len(hashCache.sigHashes) != 0 {
		t.Errorf("new hashCache error")
	}
	msgtx.SetPayload([]byte{0x1})
	hashCache.AddSigHashes(msgtx)
	tx := massutil.NewTx(msgtx)
	if !hashCache.ContainsHashes(tx.Hash()) {
		t.Errorf("hashCache containsHashes error")
	}
}

func TestHashCache_GetSigHashes(t *testing.T) {
	hashCache := NewHashCache(10)
	if len(hashCache.sigHashes) != 0 {
		t.Errorf("new hashCache error")
	}
	msgtx.SetPayload([]byte{0x1})
	hashCache.AddSigHashes(msgtx)
	tx := massutil.NewTx(msgtx)
	_, ok := hashCache.GetSigHashes(tx.Hash())
	if !ok {
		t.Errorf("get hashCache error")
	}
}

func TestHashCache_PurgeSigHashes(t *testing.T) {
	hashCache := NewHashCache(10)
	if len(hashCache.sigHashes) != 0 {
		t.Errorf("new hashCache error")
	}
	msgtx.SetPayload([]byte{0x1})
	hashCache.AddSigHashes(msgtx)
	tx := massutil.NewTx(msgtx)
	_, ok1 := hashCache.GetSigHashes(tx.Hash())
	if !ok1 {
		t.Errorf("get hashCache error")
	}
	hashCache.PurgeSigHashes(tx.Hash())
	_, ok2 := hashCache.GetSigHashes(tx.Hash())
	if ok2 {
		t.Errorf("purge hashCache error")
	}
}
