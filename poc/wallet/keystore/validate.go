package keystore

import (
	"regexp"
)

var (
	passRe *regexp.Regexp
	seedRe *regexp.Regexp
)

func init() {
	passRe = regexp.MustCompile(`^[0-9a-zA-Z@#$%^&]{6,40}$`)
}

func ValidatePassphrase(pass []byte) bool {
	return passRe.Match(pass)
}

func ValidateSeed(seed []byte) bool {
	return len(seed) == 32
}

// func ValidateRemarks(remarks string) bool {
// 	return len(remarks) <= 32
// }
