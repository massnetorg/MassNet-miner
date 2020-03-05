package testutil

import (
	"os"
	"testing"
)

const envUseCI = "MASS_CI"

func SkipCI(t *testing.T) {
	if os.Getenv(envUseCI) == "" {
		t.Skip("Skip MASS CI")
	}
}
