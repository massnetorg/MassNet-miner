package disk

import (
	"encoding/binary"
	"os"
	"sync"
	"time"

	"massnet.org/mass/logging"
)

const (
	MaxFileIdleTime = 10 * time.Minute
)

type BlockFile struct {
	mu sync.RWMutex

	fileNo      uint32
	numBlocks   uint32
	size        uint64
	heightFirst uint64
	heightLast  uint64
	timeFirst   uint64
	timeLast    uint64

	beforeAdd    *BlockFile
	lastAccessAt time.Time
	file         *os.File
	readonly     bool
}

func NewBlockFile(fileNo uint32, readonly bool) *BlockFile {
	return &BlockFile{fileNo: fileNo, readonly: readonly}
}

func NewBlockFileFromBytes(data []byte, readonly bool) *BlockFile {
	if len(data) != 48 {
		return nil
	}

	return &BlockFile{
		fileNo:      binary.LittleEndian.Uint32(data[0:4]),
		numBlocks:   binary.LittleEndian.Uint32(data[4:8]),
		size:        binary.LittleEndian.Uint64(data[8:16]),
		heightFirst: binary.LittleEndian.Uint64(data[16:24]),
		heightLast:  binary.LittleEndian.Uint64(data[24:32]),
		timeFirst:   binary.LittleEndian.Uint64(data[32:40]),
		timeLast:    binary.LittleEndian.Uint64(data[40:48]),
		readonly:    readonly,
	}
}

func (b *BlockFile) Bytes() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	buf := make([]byte, 48)
	binary.LittleEndian.PutUint32(buf[0:4], b.fileNo)
	binary.LittleEndian.PutUint32(buf[4:8], b.numBlocks)
	binary.LittleEndian.PutUint64(buf[8:16], b.size)
	binary.LittleEndian.PutUint64(buf[16:24], b.heightFirst)
	binary.LittleEndian.PutUint64(buf[24:32], b.heightLast)
	binary.LittleEndian.PutUint64(buf[32:40], b.timeFirst)
	binary.LittleEndian.PutUint64(buf[40:48], b.timeLast)
	return buf
}

func (b *BlockFile) Size() uint64 {
	return b.size
}

func (b *BlockFile) Number() uint32 {
	return b.fileNo
}

func (b *BlockFile) AddBlock(height, size uint64, timestamp uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.beforeAdd = &BlockFile{
		fileNo:      b.fileNo,
		numBlocks:   b.numBlocks,
		size:        b.size,
		heightFirst: b.heightFirst,
		heightLast:  b.heightLast,
		timeFirst:   b.timeFirst,
		timeLast:    b.timeLast,
	}

	if b.numBlocks == 0 || b.heightFirst > height {
		b.heightFirst = height
	}
	if b.numBlocks == 0 || b.timeFirst > timestamp {
		b.timeFirst = timestamp
	}
	if height > b.heightLast {
		b.heightLast = height
	}
	if timestamp > b.timeLast {
		b.timeLast = timestamp
	}
	b.numBlocks++
	b.size += size
}

func (b *BlockFile) discardRecentChange() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.beforeAdd != nil {
		b.numBlocks = b.beforeAdd.numBlocks
		b.size = b.beforeAdd.size
		b.heightFirst = b.beforeAdd.heightFirst
		b.heightFirst = b.beforeAdd.heightLast
		b.timeFirst = b.beforeAdd.timeFirst
		b.timeLast = b.beforeAdd.timeLast
		b.beforeAdd = nil
	}
}

func (b *BlockFile) commitRecentChange() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.beforeAdd = nil
}

// write raw block
func (b *BlockFile) WriteRawBlock(flatFileSeq *FlatFileSeq, offset int64, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	file, err := b.openFile(flatFileSeq, offset)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err != nil {
		logging.CPrint(logging.WARN, "write block file error", logging.LogFormat{"filename": b.file.Name(), "err": err})
		b.file.Close()
		b.file = nil
	}
	return err
}

// read raw data
func (b *BlockFile) ReadRawData(flatFileSeq *FlatFileSeq, offset int64, size int) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	file, err := b.openFile(flatFileSeq, offset)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	_, err = file.ReadAt(buf, offset)
	return buf, err
}

// get or open file
func (b *BlockFile) openFile(flatFileSeq *FlatFileSeq, offset int64) (*os.File, error) {

	b.lastAccessAt = time.Now()

	var err error
	if b.file == nil {
		pos := NewFlatFilePos(b.fileNo, offset)
		b.file, err = flatFileSeq.Open(pos, b.readonly)
		time.AfterFunc(MaxFileIdleTime, b.closeFileWhenIdle)
		logging.CPrint(logging.DEBUG, "open block file", logging.LogFormat{"filename": b.file.Name()})
	} else {
		_, err = b.file.Seek(offset, os.SEEK_SET)
		if err != nil {
			logging.CPrint(logging.WARN, "seek block file error", logging.LogFormat{"filename": b.file.Name(), "err": err})
			b.file.Close()
			b.file = nil
		}
	}
	return b.file, err
}

// close idle file
func (b *BlockFile) closeFileWhenIdle() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.file == nil {
		return
	}

	if time.Now().After(b.lastAccessAt.Add(MaxFileIdleTime)) {
		logging.CPrint(logging.INFO, "close idle block file", logging.LogFormat{
			"filename":     b.file.Name(),
			"lastAccessAt": b.lastAccessAt.String(),
		})
		b.file.Close()
		b.file = nil
		return
	}
	time.AfterFunc(MaxFileIdleTime, b.closeFileWhenIdle)
}

func (b *BlockFile) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.file != nil {
		logging.CPrint(logging.DEBUG, "close block file", logging.LogFormat{"filename": b.file.Name()})
		b.file.Close()
		b.file = nil
	}
}

func (b *BlockFile) Flush(size int64, finalize bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.readonly {
		return
	}
	b.readonly = finalize
	if b.file == nil {
		return
	}

	if finalize {
		if err := TruncateFile(b.file, size); err != nil {
			logging.CPrint(logging.ERROR, "truncate block file error", logging.LogFormat{
				"filename": b.file.Name(),
				"err":      err,
			})
		}
	}
	if err := b.file.Sync(); err != nil {
		logging.CPrint(logging.ERROR, "block file sync error", logging.LogFormat{
			"filename": b.file.Name(),
			"err":      err,
		})
	}
	logging.CPrint(logging.DEBUG, "flush block file", logging.LogFormat{
		"filename": b.file.Name(),
		"finalize": finalize,
	})
	b.file.Close() // close Read-Write mode file
	b.file = nil
}
