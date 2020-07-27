package ldb

import (
	"bytes"
	"testing"

	"massnet.org/mass/database/storage"
)

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
