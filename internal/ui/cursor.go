package ui

type cursor struct {
	index, offset, height, count int
	pendingG                     bool
}

func (c *cursor) setCount(n int) {
	if n < 0 {
		n = 0
	}
	c.count = n
	if n == 0 {
		c.index = 0
		c.offset = 0
		return
	}
	if c.index >= n {
		c.index = n - 1
	}
	c.clampOffset()
	c.visible()
}

func (c *cursor) setHeight(h int) {
	if h < 0 {
		h = 0
	}
	c.height = h
	c.clampOffset()
	c.visible()
}

func (c *cursor) handleKey(k string) bool {
	if k == "g" {
		if c.pendingG {
			c.index = 0
			c.offset = 0
			c.pendingG = false
		} else {
			c.pendingG = true
		}
		return true
	}
	c.pendingG = false

	step := max(1, c.height/2)
	switch k {
	case "j", "down":
		c.index++
	case "k", "up":
		c.index--
	case "ctrl+d":
		c.index += step
	case "ctrl+u":
		c.index -= step
	case "ctrl+f", "pgdown":
		c.index += max(1, c.height)
	case "ctrl+b", "pgup":
		c.index -= max(1, c.height)
	case "G", "end":
		c.index = c.count - 1
	case "home":
		c.index = 0
	default:
		return false
	}
	if c.count == 0 {
		c.index = 0
	} else {
		c.index = max(0, min(c.index, c.count-1))
	}
	c.clampOffset()
	c.visible()
	return true
}

func (c *cursor) visible() (start, end int) {
	if c.count == 0 || c.height == 0 {
		return 0, 0
	}
	if c.index < c.offset {
		c.offset = c.index
	}
	if c.index >= c.offset+c.height {
		c.offset = c.index - c.height + 1
	}
	c.clampOffset()
	return c.offset, min(c.count, c.offset+c.height)
}

func (c *cursor) clampOffset() {
	maxOffset := max(0, c.count-c.height)
	c.offset = max(0, min(c.offset, maxOffset, c.index))
}
