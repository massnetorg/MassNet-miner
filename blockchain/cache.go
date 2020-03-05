package blockchain

import (
	"os"
	"path/filepath"
	"sync"

	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

const blockCacheFileName = "blocks.cache"

type blockCacheLoc struct {
	offset int64
	size   int
}

type blockCache struct {
	sync.RWMutex
	data  *os.File
	index map[wire.Hash]blockCacheLoc
}

func initBlockCache(path string) (*blockCache, error) {
	filePath := filepath.Join(path, blockCacheFileName)

	_, err := os.Stat(filePath)
	if err == nil || os.IsExist(err) {
		if err := os.Remove(filePath); err != nil {
			return nil, err
		}
	}

	f, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	return &blockCache{
		data:  f,
		index: make(map[wire.Hash]blockCacheLoc),
	}, nil
}

func (cache *blockCache) addBlock(block *massutil.Block) {
	cache.Lock()
	defer cache.Unlock()

	fi, err := cache.data.Stat()
	if err != nil {
		return
	}
	offset := fi.Size()

	bs, err := block.Bytes(wire.Packet)
	if err != nil {
		return
	}

	size, err := cache.data.Write(bs)
	if err != nil {
		return
	}

	cache.index[*block.Hash()] = blockCacheLoc{
		offset: offset,
		size:   size,
	}
}

func (cache *blockCache) getBlock(hash *wire.Hash) (*massutil.Block, error) {
	cache.RLock()
	defer cache.RUnlock()

	loc, exists := cache.index[*hash]
	if !exists {
		return nil, errBlockCacheNotExists
	}

	bs := make([]byte, loc.size)
	if _, err := cache.data.ReadAt(bs, loc.offset); err != nil {
		return nil, err
	}

	return massutil.NewBlockFromBytes(bs, wire.Packet)
}
