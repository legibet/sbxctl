package ui

import "testing"

func TestCursorMovementPagingAndGG(t *testing.T) {
	var c cursor
	c.setHeight(4)
	c.setCount(12)
	for _, key := range []string{"j", "j", "ctrl+d"} {
		if !c.handleKey(key) {
			t.Fatalf("%q was not consumed", key)
		}
	}
	if c.index != 4 {
		t.Fatalf("index = %d, want 4", c.index)
	}
	c.handleKey("ctrl+f")
	if c.index != 8 {
		t.Fatalf("page index = %d, want 8", c.index)
	}
	c.handleKey("G")
	if c.index != 11 {
		t.Fatalf("bottom index = %d, want 11", c.index)
	}
	c.handleKey("g")
	c.handleKey("g")
	if c.index != 0 {
		t.Fatalf("gg index = %d, want 0", c.index)
	}
	c.handleKey("g")
	c.handleKey("j")
	c.handleKey("g")
	if !c.pendingG {
		t.Fatal("new g chord did not start after another key")
	}
}

func TestCursorClampsWhenCountShrinks(t *testing.T) {
	c := cursor{index: 9, offset: 6, height: 4, count: 10}
	c.setCount(3)
	if c.index != 2 || c.offset != 0 {
		t.Fatalf("cursor = index %d offset %d, want 2 and 0", c.index, c.offset)
	}
	start, end := c.visible()
	if start != 0 || end != 3 {
		t.Fatalf("visible = %d:%d, want 0:3", start, end)
	}
	c.setCount(0)
	if c.index != 0 || c.offset != 0 {
		t.Fatalf("empty cursor = index %d offset %d", c.index, c.offset)
	}
}
