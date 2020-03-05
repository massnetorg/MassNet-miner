package ccache

import (
	"sync"

	"github.com/golang/groupcache/lru"
)

// concurrent safe lru cache
type CCache struct {
	l     sync.Mutex
	cache *lru.Cache
}

func NewCCache(maxEntries int) *CCache {
	return &CCache{
		cache: lru.New(maxEntries),
	}
}

// read only
func (c *CCache) Get(key lru.Key) (value interface{}, ok bool) {
	c.l.Lock()
	defer c.l.Unlock()
	return c.cache.Get(key)
}

func (c *CCache) Add(key lru.Key, value interface{}) {
	c.l.Lock()
	c.cache.Add(key, value)
	c.l.Unlock()
}

func (c *CCache) Remove(key lru.Key) {
	c.l.Lock()
	c.cache.Remove(key)
	c.l.Unlock()
}

func (c *CCache) RemoveOldest() {
	c.l.Lock()
	c.cache.RemoveOldest()
	c.l.Unlock()
}

// read and remove
func (c *CCache) GetRemove(key lru.Key) (value interface{}, ok bool) {
	c.l.Lock()
	defer c.l.Unlock()
	value, ok = c.cache.Get(key)
	if ok {
		c.cache.Remove(key)
	}
	return value, ok
}

func (c *CCache) Clear() {
	c.l.Lock()
	c.cache.Clear()
	c.l.Unlock()
}

func (c *CCache) Len() int {
	c.l.Lock()
	defer c.l.Unlock()
	return c.cache.Len()
}

func (c *CCache) SetMaxEntries(maxEntries int) {
	c.l.Lock()
	c.cache.MaxEntries = maxEntries
	c.l.Unlock()
}

func (c *CCache) SetOnEvicted(onEvicted func(key lru.Key, value interface{})) {
	c.l.Lock()
	c.cache.OnEvicted = onEvicted
	c.l.Unlock()
}
