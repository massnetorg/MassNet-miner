package ldb

import (
	"testing"

	"golang.org/x/crypto/ripemd160"
)

var (
	scriptHash1 = [ripemd160.Size]byte{38, 68, 243, 255, 120, 146, 209, 113, 132, 94, 65, 159, 222, 221, 173, 12, 156, 202, 252, 158}
	scriptHash2 = [ripemd160.Size]byte{118, 202, 212, 91, 172, 11, 89, 237, 168, 147, 179, 82, 245, 130, 5, 88, 44, 75, 136, 191}
	scriptHash3 = [ripemd160.Size]byte{94, 225, 94, 209, 24, 231, 86, 36, 6, 246, 9, 54, 38, 100, 90, 63, 242, 215, 35, 82}
)

func TestBindingTxKey(t *testing.T) {
	testdata := &bindingTxIndex{
		scriptHash: scriptHash1,
		blkHeight:  uint64(100),
		txOffset:   uint32(100),
		txLen:      uint32(206),
		index:      uint32(0),
	}
	key := bindingTxIndexToKey(testdata)
	res, err := mustDecodeBindingTxIndexKey(key)
	if err != nil {
		t.Errorf("failed to decode binding tx")
	}
	showBindingTxIndex(t, res)
}

func showBindingTxIndex(t *testing.T, btxIndex *bindingTxIndex) {
	t.Logf("scriptHash: %v", btxIndex.scriptHash)
	t.Logf("block height: %v", btxIndex.blkHeight)
	t.Logf("tx offset: %v", btxIndex.txOffset)
	t.Logf("tx length: %v", btxIndex.txLen)
	t.Logf("tx out index: %v", btxIndex.index)
}

func TestBindingShIndexKey(t *testing.T) {
	testdata := &bindingShIndex{
		blkHeight:  uint64(100),
		scriptHash: scriptHash1,
	}
	key := bindingShIndexToKey(testdata)
	res, err := mustDecodeBindingShIndexKey(key)
	if err != nil {
		t.Errorf("failed to decode binding sh")
	}
	showBindingShIndex(t, res)
}

func showBindingShIndex(t *testing.T, gshIndex *bindingShIndex) {
	t.Logf("scriptHash: %v", gshIndex.scriptHash)
	t.Logf("block height: %v", gshIndex.blkHeight)
}

func TestBindingTxSpentIndexKey(t *testing.T) {
	testdata := &bindingTxSpentIndex{
		scriptHash:       scriptHash1,
		blkHeightSpent:   uint64(200),
		txOffsetSpent:    uint32(1400),
		txLenSpent:       uint32(206),
		blkHeightBinding: uint64(100),
		txOffsetBinding:  uint32(1600),
		txLenBinding:     uint32(300),
		indexBinding:     uint32(0),
	}
	key := bindingTxSpentIndexToKey(testdata)
	res, err := mustDecodeBindingTxSpentIndexKey(key)
	if err != nil {
		t.Errorf("failed to decode binding tx spent")
	}
	showBindingTxSpentIndex(t, res)
}

func showBindingTxSpentIndex(t *testing.T, btxSpentIndex *bindingTxSpentIndex) {
	t.Logf("scriptHash: %v", btxSpentIndex.scriptHash)
	t.Logf("spent tx block height: %v", btxSpentIndex.blkHeightSpent)
	t.Logf("spent tx offset: %v", btxSpentIndex.txOffsetSpent)
	t.Logf("spent tx length: %v", btxSpentIndex.txLenSpent)
	t.Logf("binding tx block height: %v", btxSpentIndex.blkHeightBinding)
	t.Logf("binding tx offset: %v", btxSpentIndex.txOffsetBinding)
	t.Logf("binding tx length: %v", btxSpentIndex.txLenBinding)
	t.Logf("binding tx out index: %v", btxSpentIndex.indexBinding)
}
