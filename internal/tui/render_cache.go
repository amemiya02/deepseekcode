package tui

type RenderCache struct {
	max   int
	order []string
	items map[string]string
}

func NewRenderCache(max int) *RenderCache {
	if max <= 0 {
		max = 64
	}
	return &RenderCache{max: max, items: map[string]string{}}
}

func (c *RenderCache) Get(key string) (string, bool) {
	v, ok := c.items[key]
	if ok {
		c.promote(key)
	}
	return v, ok
}

func (c *RenderCache) Put(key, value string) {
	if _, ok := c.items[key]; !ok {
		c.order = append(c.order, key)
	}
	c.items[key] = value
	c.promote(key)
	for len(c.order) > c.max {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

func (c *RenderCache) promote(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(append(c.order[:i], c.order[i+1:]...), key)
			return
		}
	}
}
