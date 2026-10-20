package ui

import "applauncher/internal/win"

// iconCache keeps the HICONs of the rows the user has actually seen.
//
// Icons are loaded per visible row, never for the whole index: 96 applications
// would cost hundreds of shell calls at startup for icons nobody looked at.
type iconCache struct {
	icons    map[string]win.Icon
	order    []string
	max      int
	fallback win.Icon
}

func newIconCache(max int) *iconCache {
	return &iconCache{
		icons:    make(map[string]win.Icon),
		max:      max,
		fallback: win.DefaultAppIcon(),
	}
}

func (c *iconCache) get(path string) win.Icon {
	if path == "" {
		return c.fallback
	}
	if icon, ok := c.icons[path]; ok {
		return icon
	}

	icon, err := win.LoadShellIcon(path)
	if err != nil {
		icon = c.fallback
	}

	c.icons[path] = icon
	c.order = append(c.order, path)
	if len(c.order) > c.max {
		oldest := c.order[0]
		c.order = c.order[1:]
		if cached, ok := c.icons[oldest]; ok && cached != c.fallback {
			cached.Destroy()
		}
		delete(c.icons, oldest)
	}
	return icon
}

func (c *iconCache) close() {
	for _, icon := range c.icons {
		if icon != c.fallback {
			icon.Destroy()
		}
	}
	c.fallback.Destroy()
	c.icons = map[string]win.Icon{}
	c.order = nil
}
