package main

import (
	"sync"
	"time"
)

type cacheItem struct {
	value    interface{}
	expireMS int64
}

type Cache struct {
	mutex sync.RWMutex
	items map[string]cacheItem
	
	duration time.Duration
}

func NewCache(ExpireTime time.Duration) *Cache {
	c := &Cache{
		items: make(map[string]cacheItem),
		duration: ExpireTime,
	}
	
	return c
}

func (c *Cache) Set(Key string, Value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.SetNoMutex(Key, Value)
}
func (c *Cache) SetNoMutex(Key string, Value interface{}) {
	c.items[Key] = cacheItem{
		value: Value,
		expireMS: time.Now().Add(c.duration).UnixMilli(),
	}
}

func (c *Cache) Delete(Key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	_, ok := c.items[Key]
	if !ok { return } // Item doesn't exist.
	
	delete(c.items, Key)
}

func (c *Cache) Get(Key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	return c.GetNoMutex(Key)
}
func (c *Cache) GetNoMutex(Key string) (interface{}, bool) {
	item, ok := c.items[Key]
	if !ok || time.Now().UnixMilli() > item.expireMS {
		// Item doesn't exist or has expired.
		
		return nil, false
	}
	
	return item.value, true
}

func (c *Cache) CleanUp() {
	nowMS := time.Now().UnixMilli()
	
	c.mutex.Lock()
	for key, item := range c.items {
		if nowMS > item.expireMS {
			// Item expired!!!
			delete(c.items, key)
		}
	}
	c.mutex.Unlock()
}
