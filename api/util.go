package api

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"google.golang.org/grpc/status"
	"massnet.org/mass/blockchain"
	"massnet.org/mass/consensus"
	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
	"massnet.org/mass/massutil/safetype"
)

const (
	LenHash     = 64
	LenAddress  = 63
	LenPkString = 66
	LenWalletId = 42
	LenPassMax  = 40
	LenPassMin  = 6
	// evaluate value
	LenSpaceIDMax = 80
)

type heightList []uint64

func (h heightList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h heightList) Len() int {
	return len(h)
}

func (h heightList) Less(i, j int) bool {
	return h[i] < h[j]
}

type txDescList []*blockchain.TxDesc

func (l txDescList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l txDescList) Len() int {
	return len(l)
}

func (l txDescList) Less(i, j int) bool {
	return l[i].Added.After(l[j].Added)
}

type orphanTxDescList []*massutil.Tx

func (l orphanTxDescList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l orphanTxDescList) Len() int {
	return len(l)
}

func (l orphanTxDescList) Less(i, j int) bool {
	return l[i].Hash().ToProto().S0 < l[j].Hash().ToProto().S0
}

// AmountToString converts m(in Maxwell) to the string representation(float, in Mass)
func AmountToString(m int64) (string, error) {
	if m > massutil.MaxAmount().IntValue() {
		return "", fmt.Errorf("amount is out of range: %d", m)
	}
	u, err := safetype.NewUint128FromInt(m)
	if err != nil {
		return "", err
	}
	u, err = u.AddUint(consensus.MaxwellPerMass)
	if err != nil {
		return "", err
	}
	s := u.String()
	sInt, sFrac := s[:len(s)-8], s[len(s)-8:]
	sFrac = strings.TrimRight(sFrac, "0")
	i, err := strconv.Atoi(sInt)
	if err != nil {
		return "", err
	}
	sInt = strconv.Itoa(i - 1)
	if len(sFrac) > 0 {
		return sInt + "." + sFrac, nil
	}
	return sInt, nil
}

// StringToAmount converts s(float, in Mass) to amount(in Maxwell)
func StringToAmount(s string) (massutil.Amount, error) {
	s1 := strings.Split(s, ".")
	if len(s1) > 2 {
		return massutil.ZeroAmount(), fmt.Errorf("illegal number format")
	}
	var sInt, sFrac string
	// preproccess integral part
	sInt = strings.TrimLeft(s1[0], "0")
	if len(sInt) == 0 {
		sInt = "0"
	}
	// preproccess fractional part
	if len(s1) == 2 {
		sFrac = strings.TrimRight(s1[1], "0")
		if len(sFrac) > 8 {
			return massutil.ZeroAmount(), fmt.Errorf("precision is too high")
		}
	}
	sFrac += strings.Repeat("0", 8-len(sFrac))

	// convert
	i, err := strconv.ParseInt(sInt, 10, 64)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	if i < 0 || uint64(i) > consensus.MaxMass {
		return massutil.ZeroAmount(), fmt.Errorf("integral part is out of range")
	}

	f, err := strconv.ParseInt(sFrac, 10, 64)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	if f < 0 {
		return massutil.ZeroAmount(), fmt.Errorf("illegal number format")
	}

	u := safetype.NewUint128FromUint(consensus.MaxwellPerMass)
	u, err = u.MulInt(i)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	u, err = u.AddInt(f)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	total, err := massutil.NewAmount(u)
	if err != nil {
		return massutil.ZeroAmount(), err
	}
	return total, nil
}

func checkHashLen(hash string) error {
	if len(hash) != LenHash {
		logging.CPrint(logging.ERROR, "The length of the hash is invalid", logging.LogFormat{
			"hash":            hash,
			"length":          len(hash),
			"expected length": LenHash,
		})
		st := status.New(ErrAPIInvalidHash, ErrCode[ErrAPIInvalidHash])
		return st.Err()
	}
	return nil
}

func checkPkStringLen(publicKey string) error {
	if len(publicKey) != LenPkString {
		logging.CPrint(logging.ERROR, "The length of the public key(string) is invalid", logging.LogFormat{
			"public key":      publicKey,
			"length":          len(publicKey),
			"expected length": LenPkString,
		})
		st := status.New(ErrAPIInvalidPublicKey, ErrCode[ErrAPIInvalidPublicKey])
		return st.Err()
	}
	return nil
}

func checkWalletIdLen(walletId string) error {
	if len(walletId) != LenWalletId {
		logging.CPrint(logging.ERROR, "The length of the wallet id is incorrect", logging.LogFormat{
			"wallet_id":       walletId,
			"length":          len(walletId),
			"expected length": LenWalletId,
		})
		st := status.New(ErrAPIInvalidWalletId, ErrCode[ErrAPIInvalidWalletId])
		return st.Err()
	}
	return nil
}

func checkPassLen(pass string) error {
	if len(pass) > LenPassMax || len(pass) < LenPassMin {
		logging.CPrint(logging.ERROR, "The length of the pass is out of range", logging.LogFormat{
			"length":          len(pass),
			"allowable range": fmt.Sprintf("[%v, %v]", LenPassMin, LenPassMax),
		})
		st := status.New(ErrAPIInvalidPassphrase, ErrCode[ErrAPIInvalidPassphrase])
		return st.Err()
	}
	return nil
}

func checkAddressLen(addr string) error {
	if len(addr) != LenAddress {
		logging.CPrint(logging.ERROR, "The length of the address is incorrect", logging.LogFormat{
			"address":         addr,
			"length":          len(addr),
			"expected length": LenAddress,
		})
		st := status.New(ErrAPIInvalidAddress, ErrCode[ErrAPIInvalidAddress])
		return st.Err()
	}
	return nil
}

func checkSpaceIDLen(spaceID string) error {
	if len(spaceID) > LenSpaceIDMax {
		logging.CPrint(logging.ERROR, "The length of the spaceID is out of range", logging.LogFormat{
			"space id":   spaceID,
			"length":     len(spaceID),
			"max length": LenSpaceIDMax,
		})
		st := status.New(ErrAPIInvalidSpaceID, ErrCode[ErrAPIInvalidSpaceID])
		return st.Err()
	}
	return nil
}

func checkBlockIDLen(id string) error {
	if expectedLength := LenHash + 5; len(id) > expectedLength {
		logging.CPrint(logging.ERROR, "The length of the block id is invalid", logging.LogFormat{
			"id":              id,
			"length":          len(id),
			"expected length": expectedLength + 5,
		})
		st := status.New(ErrAPIInvalidHash, ErrCode[ErrAPIInvalidHash])
		return st.Err()
	}
	return nil
}

func generateReqID() int {
	return rand.Intn(1000000)
}
