package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/legibet/sbxctl/internal/config"
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
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; strings.Join(got, ",") != "alpha,beta,zeta" {
		t.Fatalf("name order = %v", got)
	}
	w.sort = sortDelay
	items = w.rightItems()
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; strings.Join(got, ",") != "beta,zeta,alpha" {
		t.Fatalf("delay order = %v", got)
	}
	w.reverse = true
	items = w.rightItems()
	if got := []string{items[0].Tag, items[1].Tag, items[2].Tag}; strings.Join(got, ",") != "zeta,beta,alpha" {
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

func TestAppViewDimensions(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {140, 40}} {
		t.Run(strconv.Itoa(size.width), func(t *testing.T) {
			a := newApp(sbx.Endpoint{URL: "http://127.0.0.1:9090"}, "home", &config.File{Targets: map[string]config.Target{}}, nil)
			a.overlay = overlayNone
			a.info = sbx.ServerInfo{Version: sbx.Version{Version: "1.14.0", APIVersion: 4}, StartedAt: time.Now().Add(-time.Hour)}
			a.connState = sbx.StateConnected
			a.proxies.setGroups([]sbx.Group{{
				Tag: "proxy", Type: "selector", Selectable: true, Selected: "hk-01",
				Items: []sbx.Outbound{{Tag: "hk-01", Type: "shadowsocks", Delay: 142, TestedAt: time.Now().Add(-time.Minute)}},
			}})
			a.width, a.height = size.width, size.height
			a.resizeWorkspaces()
			lines := strings.Split(a.View().Content, "\n")
			if len(lines) != size.height {
				t.Fatalf("lines = %d, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if width := lipgloss.Width(line); width != size.width {
					t.Fatalf("line %d width = %d, want %d", i, width, size.width)
				}
			}
		})
	}
}
