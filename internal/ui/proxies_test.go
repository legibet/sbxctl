package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/legibet/sbxctl/internal/sbx"
)

func TestProxiesSortingAndFiltering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	w := newProxies(newTheme(), newKeyMap(), nil)
	w.setSize(120, 20)
	w.setGroups([]sbx.Group{
		{Tag: "select", Type: "selector", Items: []sbx.Outbound{
			{Tag: "zeta", Type: "vmess", Delay: 80, TestedAt: now},
			{Tag: "alpha", Type: "direct"},
			{Tag: "beta", Type: "trojan", Delay: 20, TestedAt: now},
		}},
		{Tag: "fallback", Type: "urltest"},
	})

	w.sort = sortName
	items := w.rightItems()
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; !slices.Equal(got, []string{"alpha", "beta", "zeta"}) {
		t.Fatalf("name order = %v", got)
	}
	w.sort = sortDelay
	items = w.rightItems()
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; !slices.Equal(got, []string{"beta", "zeta", "alpha"}) {
		t.Fatalf("delay order = %v", got)
	}
	w.reverse = true
	items = w.rightItems()
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; !slices.Equal(got, []string{"zeta", "beta", "alpha"}) {
		t.Fatalf("reverse delay order = %v", got)
	}

	w.focus = 1
	w.setFilter("tro")
	items = w.rightItems()
	if len(items) != 1 || items[0].Tag != "beta" {
		t.Fatalf("right filter = %#v", items)
	}
	w.focus = 0
	w.setFilter("fall")
	rows := w.leftRows()
	if len(rows) != 1 || rows[0] != 1 {
		t.Fatalf("left filter rows = %v", rows)
	}
}
