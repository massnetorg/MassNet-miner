package ldb

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database/storage"
)

func unpackTxIndex(data [16]byte) *txIndex {
	return &txIndex{
		blkHeight: binary.BigEndian.Uint64(data[:8]),
		txOffset:  binary.LittleEndian.Uint32(data[8:12]),
		txLen:     binary.LittleEndian.Uint32(data[12:]),
	}
}

func TestAddrIndexKeySerialization(t *testing.T) {
	var packedIndex [16]byte

	fakeIndex := txIndex{
		blkHeight: 1,
		txOffset:  5,
		txLen:     360,
	}

	serializedKey := txIndexToKey(&fakeIndex)
	copy(packedIndex[:], serializedKey[35:])
	unpackedIndex := unpackTxIndex(packedIndex)
	assert.Equal(t, unpackedIndex.blkHeight, fakeIndex.blkHeight)
	assert.Equal(t, unpackedIndex.txOffset, fakeIndex.txOffset)
	assert.Equal(t, unpackedIndex.txLen, fakeIndex.txLen)
}

func TestBytesPrefix(t *testing.T) {
	testKey := []byte("a")

	prefixRange := storage.BytesPrefix(testKey)
	if !bytes.Equal(prefixRange.Start, []byte("a")) {
		t.Errorf("Wrong prefix start, got %d, expected %d", prefixRange.Start,
			[]byte("a"))
	}

	if !bytes.Equal(prefixRange.Limit, []byte("b")) {
		t.Errorf("Wrong prefix end, got %d, expected %d", prefixRange.Limit,
			[]byte("b"))
	}
}
