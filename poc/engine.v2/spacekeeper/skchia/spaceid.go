package skchia

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"massnet.org/mass/poc/engine.v2"
)

type SpaceID struct {
	info      *chiapos.PlotInfo
	plotID    pocutil.Hash
	bitLength int
	ordinal   int64
	str       string
}

func NewSpaceID(info *chiapos.PlotInfo, bl int) *SpaceID {
	return &SpaceID{
		plotID:    chiapos.Hash256(bytes.Join([][]byte{info.PoolPublicKey.Bytes(), info.PlotPublicKey.Bytes()}, nil)),
		bitLength: bl,
		info:      info,
		ordinal:   engine.UnknownOrdinal,
	}
}

func (sid *SpaceID) PubKey() *chiapos.G1Element {
	return sid.info.PlotPublicKey
}

func (sid *SpaceID) PlotID() pocutil.Hash {
	return sid.plotID
}

func (sid *SpaceID) PlotInfo() *chiapos.PlotInfo {
	return sid.info
}

func (sid *SpaceID) BitLength() int {
	return sid.bitLength
}

func (sid *SpaceID) Ordinal() int64 {
	return sid.ordinal
}

func (sid *SpaceID) String() string {
	if sid.str == "" {
		sid.str = strings.Join([]string{hex.EncodeToString(sid.plotID[:]), "-", strconv.Itoa(sid.bitLength)}, "")
	}
	return sid.str
}
