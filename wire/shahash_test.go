package wire

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// mainNetGenesisHash is the hash of the first block in the block chain for the
// main network (genesis block).
var mainNetGenesisHash = Hash([HashSize]byte{ // Make go vet happy.
	0x00, 0x05, 0xa4, 0x59, 0x20, 0x42, 0x2f, 0x9d,
	0x41, 0x7e, 0x48, 0x67, 0xef, 0xdc, 0x4f, 0xb8,
	0xa0, 0x4a, 0x1f, 0x3f, 0xff, 0x1f, 0xa0, 0x7e,
	0x99, 0x8e, 0x86, 0xf7, 0xf7, 0xa2, 0x7a, 0xe3,
})

// TestShaHash tests the Hash API.
func TestShaHash(t *testing.T) {

	// Hash of block 234439.
	blockHashStr := "b3a8e0e1f9ab1bfe3a36f231f676f78bb30a519d2b21e6c530c0eee8ebb4a5d0"
	blockHash, err := NewHashFromStr(blockHashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Hash of another block
	buf := []byte{
		0x35, 0xa9, 0xe3, 0x81, 0xb1, 0xa2, 0x75, 0x67,
		0x54, 0x9b, 0x5f, 0x8a, 0x6f, 0x78, 0x3c, 0x16,
		0x7e, 0xbf, 0x80, 0x9f, 0x1c, 0x4d, 0x6a, 0x9e,
		0x36, 0x72, 0x40, 0x48, 0x4d, 0x8c, 0xe2, 0x81,
	}

	hash, err := NewHash(buf)
	if err != nil {
		t.Errorf("NewHash: unexpected error %v", err)
	}

	// Ensure proper size.
	if len(hash) != HashSize {
		t.Errorf("NewHash: hash length mismatch - got: %v, want: %v",
			len(hash), HashSize)
	}

	// Ensure contents match.
	if !bytes.Equal(hash[:], buf) {
		t.Errorf("NewHash: hash contents mismatch - got: %v, want: %v",
			hash[:], buf)
	}

	// Ensure contents of hash of block 234440 don't match 234439.
	if hash.IsEqual(blockHash) {
		t.Errorf("IsEqual: hash contents should not match - got: %v, want: %v",
			hash, blockHash)
	}

	// Set hash from byte slice and ensure contents match.
	err = hash.SetBytes(blockHash.Bytes())
	if err != nil {
		t.Errorf("SetBytes: %v", err)
	}
	if !hash.IsEqual(blockHash) {
		t.Errorf("IsEqual: hash contents mismatch - got: %v, want: %v",
			hash, blockHash)
	}

	// Invalid size for SetBytes.
	err = hash.SetBytes([]byte{0x00})
	if err == nil {
		t.Errorf("SetBytes: failed to received expected err - got: nil")
	}

	// Invalid size for NewHash.
	invalidHash := make([]byte, HashSize+1)
	_, err = NewHash(invalidHash)
	if err == nil {
		t.Errorf("NewHash: failed to received expected err - got: nil")
	}
}

// TestShaHashString  tests the stringized output for sha hashes.
func TestShaHashString(t *testing.T) {
	// Block 100000 hash.
	wantStr := "000000000003ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	hash := Hash([HashSize]byte{ // Make go vet happy.
		0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0xba, 0x27,
		0xaa, 0x20, 0x0b, 0x1c, 0xec, 0xaa, 0xd4, 0x78,
		0xd2, 0xb0, 0x04, 0x32, 0x34, 0x6c, 0x3f, 0x1f,
		0x39, 0x86, 0xda, 0x1a, 0xfd, 0x33, 0xe5, 0x06,
	})

	hashStr := hash.String()
	if hashStr != wantStr {
		t.Errorf("String: wrong hash string - got %v, want %v",
			hashStr, wantStr)
	}
}

// TestNewShaHashFromStr executes tests against the NewHashFromStr function.
func TestNewShaHashFromStr(t *testing.T) {
	tests := []struct {
		in   string
		want Hash
		err  error
	}{
		// Genesis hash.
		{
			"0005a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
			mainNetGenesisHash,
			nil,
		},

		// Genesis hash with stripped leading zeros.
		{
			"5a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
			mainNetGenesisHash,
			nil,
		},

		// Empty string.
		{
			"",
			Hash{},
			nil,
		},

		// Single digit hash.
		{
			"1",
			Hash([HashSize]byte{ // Make go vet happy.
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
			}),
			nil,
		},

		// double leading zero
		{
			"0065a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
			Hash([HashSize]byte{ // Make go vet happy.
				0x00, 0x65, 0xa4, 0x59, 0x20, 0x42, 0x2f, 0x9d,
				0x41, 0x7e, 0x48, 0x67, 0xef, 0xdc, 0x4f, 0xb8,
				0xa0, 0x4a, 0x1f, 0x3f, 0xff, 0x1f, 0xa0, 0x7e,
				0x99, 0x8e, 0x86, 0xf7, 0xf7, 0xa2, 0x7a, 0xe3,
			}),
			nil,
		},

		// Hash string that is too long.
		{
			"01234567890123456789012345678901234567890123456789012345678912345",
			Hash{},
			ErrHashStrSize,
		},

		// Hash string that is contains non-hex chars.
		{
			"abcdefg",
			Hash{},
			hex.InvalidByteError('g'),
		},
	}

	unexpectedErrStr := "NewHashFromStr #%d failed to detect expected error - got: %v want: %v"
	unexpectedResultStr := "NewHashFromStr #%d got: %v want: %v"
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result, err := NewHashFromStr(test.in)
		if err != test.err {
			t.Errorf(unexpectedErrStr, i, err, test.err)
			continue
		} else if err != nil {
			// Got expected error. Move on to the next test.
			continue
		}
		if !test.want.IsEqual(result) {
			t.Errorf(unexpectedResultStr, i, result, &test.want)
			continue
		}
	}
}
