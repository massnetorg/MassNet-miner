package massdb_v1

import (
	"io"
	"runtime"

	"github.com/massnetorg/mass-core/poc"
)

// MemCache runs without mutex for better performance
type MemCache struct {
	size int
	data []byte
}

func NewMemCache(size int) *MemCache {
	cache := &MemCache{}
	if size > 0 {
		cache.data = make([]byte, size)
		cache.size = size
	}
	return cache
}

func (cache *MemCache) Len() int {
	return cache.size
}

func (cache *MemCache) Update(size uint64) {
	cache.size = 0
	cache.data = nil
	runtime.GC()
	cache.data = make([]byte, size)
	cache.size = len(cache.data)
}

func (cache *MemCache) Release() {
	cache.size = 0
	cache.data = nil
	runtime.GC() // free memory as soon as possible
}

func (cache *MemCache) WriteAt(buf []byte, offset int64) (n int, err error) {
	if int(offset)+len(buf) > cache.size {
		err = io.EOF
	}
	if int(offset) >= cache.size {
		return
	}
	n = copy(cache.data[offset:], buf)
	return
}

func (cache *MemCache) ReadAt(buf []byte, offset int64) (n int, err error) {
	if int(offset)+len(buf) > cache.size {
		err = io.EOF
	}
	if int(offset) >= cache.size {
		return
	}
	n = copy(buf, cache.data[offset:])
	return
}

type SeekerWriter interface {
	io.Writer
	io.WriterAt
	Seek(offset int64, whence int) (ret int64, err error)
}

const memCacheWriteBlockSize = 256 * poc.MiB

func (cache *MemCache) WriteToWriter(quit chan struct{}, w SeekerWriter, srcStart, dstStart, len int64) (n int, err error) {
	if int(srcStart+len) > cache.size || int(srcStart) >= cache.size {
		err = io.EOF
		return
	}

	var count int64
	var bufSize int64

	for count < len {
		if quit != nil {
			select {
			case <-quit:
				return int(count), ErrStopPlotting
			default:
			}
		}

		if remain := len - count; remain > memCacheWriteBlockSize {
			bufSize = memCacheWriteBlockSize
		} else {
			bufSize = remain
		}
		if n, err = w.WriteAt(cache.data[srcStart+count:srcStart+count+bufSize], dstStart+count); err != nil {
			return int(count) + n, err
		}
		count += int64(n)
	}

	return int(count), err
}

func (cache *MemCache) ReadFromReader(r io.ReaderAt, srcStart, dstStart, len int64) (n int, err error) {
	if int(dstStart+len) > cache.size {
		err = io.EOF
	}
	if int(dstStart) >= cache.size {
		return
	}
	return r.ReadAt(cache.data[dstStart:dstStart+len], srcStart)
}
