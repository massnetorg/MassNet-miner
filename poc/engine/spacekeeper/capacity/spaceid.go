package capacity

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/massnetorg/mass-core/pocec"
)

type SpaceID struct {
	pubKey     *pocec.PublicKey
	pubKeyHash pocutil.Hash
	bitLength  int
	ordinal    int64
	str        string
}

func NewSpaceID(ordinal int64, pk *pocec.PublicKey, bl int) *SpaceID {
	return &SpaceID{
		pubKeyHash: pocutil.PubKeyHash(pk),
		bitLength:  bl,
		pubKey:     pk,
		ordinal:    ordinal,
	}
}

func (sid *SpaceID) PubKey() *pocec.PublicKey {
	return sid.pubKey
}

func (sid *SpaceID) PubKeyHash() pocutil.Hash {
	return sid.pubKeyHash
}

func (sid *SpaceID) BitLength() int {
	return sid.bitLength
}

func (sid *SpaceID) Ordinal() int64 {
	return sid.ordinal
}

func (sid *SpaceID) String() string {
	if sid.str == "" {
		sid.str = strings.Join([]string{hex.EncodeToString(sid.pubKey.SerializeCompressed()), "-", strconv.Itoa(sid.bitLength)}, "")
	}
	return sid.str
}
