package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/massnetorg/mass-core/poc"
)

// DecodeProofList decodes config proof list in format like this: "<BL>:<Count>,<BL>:<Count>".
// Example: 3 spaces of BL = 24, 4 spaces of BL= 26, 5 spaces of BL = 28 would be like "24:3, 26:4, 28:5".
// Duplicate BL would be added together, "24:1, 24:2, 26:5" equals to "24:3, 26:5".
func DecodeProofList(proofList string) (map[int]int, error) {
	conf := make(map[int]int)
	proofList = strings.Replace(proofList, " ", "", -1)
	for _, item := range strings.Split(proofList, ",") {
		desc := strings.Split(item, ":")
		if len(desc) != 2 {
			return nil, errors.New(fmt.Sprintln("invalid proof list item, wrong format", proofList, desc))
		}
		bl, err := strconv.Atoi(desc[0])
		if err != nil {
			return nil, errors.New(fmt.Sprintln("invalid proof list bitLength, wrong bitLength str", desc, desc[0]))
		}
		if !poc.ProofTypeDefault.EnsureBitLength(bl) {
			return nil, errors.New(fmt.Sprintln("invalid proof list bitLength", desc, bl))
		}
		count, err := strconv.Atoi(desc[1])
		if err != nil {
			return nil, errors.New(fmt.Sprintln("invalid proof list spaceCount, wrong spaceCount str", desc, desc[1]))
		}
		if count < 0 {
			return nil, errors.New(fmt.Sprintln("invalid proof list spaceCount", desc, count))
		}
		if _, exists := conf[bl]; exists {
			conf[bl] += count
		} else {
			conf[bl] = count
		}
	}
	return conf, nil
}
