package blockchain

import (
	"sort"
	"time"

	"massnet.org/mass/consensus"
)

var (
	coinbaseMaturity     = consensus.BaseSubsidy
	maxMedianTimeEntries int
)

// TstSetCoinbaseMaturity makes the ability to set the coinbase maturity
// available to the test package.
func TstSetCoinbaseMaturity(maturity uint64) {
	coinbaseMaturity = maturity
}

// TstTimeSorter makes the internal timeSorter type available to the test
// package.
func TstTimeSorter(times []time.Time) sort.Interface {
	return timeSorter(times)
}

// TstCheckSerializedHeight makes the internal checkSerializedHeight function
// available to the test package.
var TstCheckSerializedHeight = checkSerializedHeight

// TstSetMaxMedianTimeEntries makes the ability to set the maximum number of
// median time entries available to the test package.
func TstSetMaxMedianTimeEntries(val int) {
	maxMedianTimeEntries = val
}

// TstCheckBlockScripts makes the internal checkBlockScripts function available
// to the test package.
var TstCheckBlockScripts = checkBlockScripts
