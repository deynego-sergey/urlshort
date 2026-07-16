package cache

import (
	"sync"
	"time"
)

type cacheItem struct {
	act time.Time
	val string
}
type ICache interface {
	Get(key int64) (string, bool)
	Put(key int64, value string)
	Drop(key int64)
}

type cache struct {
	d  map[int64]*cacheItem
	mu sync.RWMutex
}

func NewCache() ICache {
	return &cache{d: make(map[int64]*cacheItem, 10)}
}

/*
*
*
*
 */
func (c *cache) Get(key int64) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, o := c.d[key]; o {
		v.act = time.Now()
		return v.val, true
	}
	return "", false
}

func (c *cache) Put(key int64, value string) {
	v := &cacheItem{time.Now(), value}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.d[key] = v
	return
}
func (c *cache) Drop(key int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.d, key)
}
