package blockchain

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"massnet.org/mass/logging"
	"massnet.org/mass/massutil"
)

// BlockProgressLogger provides periodic logging for other services in order
// to show users progress of certain "actions" involving some or all current
// blocks. Ex: syncing to best chain, indexing all blocks, etc.
type BlockProgressLogger struct {
	started  int32
	shutdown int32

	quit chan struct{}

	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time

	progressAction string
	sync.Mutex
}

// NewBlockProgressLogger returns a new block progress logger.
// The progress message is templated as follows:
//  {progressAction} {numProcessed} {blocks|block} in the last {timePeriod}
//  ({numTxs}, height {lastBlockHeight}, {lastBlockTimeStamp})
func NewBlockProgressLogger(progressMessage string) *BlockProgressLogger {
	return &BlockProgressLogger{
		lastBlockLogTime: time.Now(),
		progressAction:   progressMessage,
		quit:             make(chan struct{}),
	}
}

func (b *BlockProgressLogger) Start() {
	// Already started?
	if atomic.AddInt32(&b.started, 1) != 1 {
		return
	}
}

func (b *BlockProgressLogger) Stop() error {
	if atomic.AddInt32(&b.shutdown, 1) != 1 {
		logging.CPrint(logging.WARN, "Block logger is already in the process of shutting down")
		return nil
	}

	logging.CPrint(logging.INFO, "Block logger shutting down")
	close(b.quit)
	return nil
}

// LogBlockHeight logs a new block height as an information message to show
// progress to the user. In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *BlockProgressLogger) LogBlockHeight(block *massutil.Block) {
	b.Lock()
	defer b.Unlock()

	b.receivedLogBlocks++
	b.receivedLogTx += int64(len(block.MsgBlock().Transactions))

	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if duration < time.Second*10 {
		return
	}

	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Log information about new block height.
	blockStr := "blocks"
	if b.receivedLogBlocks == 1 {
		blockStr = "block"
	}
	txStr := "transactions"
	if b.receivedLogTx == 1 {
		txStr = "transaction"
	}
	msg := fmt.Sprintf("%s %d %s in the last %s (%d %s, height %d, %s)",
		b.progressAction, b.receivedLogBlocks, blockStr, tDuration, b.receivedLogTx,
		txStr, block.Height(), block.MsgBlock().Header.Timestamp)
	logging.CPrint(logging.INFO, msg)
	//b.subsystemLogger.Infof("%s %d %s in the last %s (%d %s, height %d, %s)",
	//	b.progressAction, b.receivedLogBlocks, blockStr, tDuration, b.receivedLogTx,
	//	txStr, block.Height(), block.MsgBlock().Header.Timestamp)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.lastBlockLogTime = now
}

func (b *BlockProgressLogger) SetLastLogTime(time time.Time) {
	b.lastBlockLogTime = time
}
