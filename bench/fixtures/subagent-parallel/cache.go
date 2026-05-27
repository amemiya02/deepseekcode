package main

// CacheStore provides in-memory caching.
type CacheStore struct {
	data map[string]string
}

func NewCacheStore() *CacheStore {
	return &CacheStore{data: make(map[string]string)}
}

func (c *CacheStore) Get(key string) (string, bool) {
	v, ok := c.data[key]
	return v, ok
}

func (c *CacheStore) Set(key, value string) {
	c.data[key] = value
}
