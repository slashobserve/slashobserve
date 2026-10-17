package dashboards

import (
	"container/list"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

const panelCacheSize = 256

type panelKey struct {
	dashboard pgtype.UUID
	version   int
}

type panelEntry struct {
	key    panelKey
	panels map[string]struct{}
}

type panelCache struct {
	mu      sync.Mutex
	order   *list.List
	entries map[panelKey]*list.Element
}

func newPanelCache() *panelCache {
	return &panelCache{order: list.New(), entries: make(map[panelKey]*list.Element, panelCacheSize)}
}

func (pc *panelCache) get(key panelKey) (map[string]struct{}, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	el, ok := pc.entries[key]
	if !ok {
		return nil, false
	}
	pc.order.MoveToFront(el)
	return el.Value.(*panelEntry).panels, true
}

func (pc *panelCache) put(key panelKey, panels map[string]struct{}) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if el, ok := pc.entries[key]; ok {
		pc.order.MoveToFront(el)
		return
	}
	pc.entries[key] = pc.order.PushFront(&panelEntry{key: key, panels: panels})
	if pc.order.Len() > panelCacheSize {
		oldest := pc.order.Back()
		pc.order.Remove(oldest)
		delete(pc.entries, oldest.Value.(*panelEntry).key)
	}
}
