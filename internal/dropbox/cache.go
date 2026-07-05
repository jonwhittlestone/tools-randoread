package dropbox

import (
	"fmt"
	"sync"
	"time"
)

// Lister is the subset of Client that CachedLister wraps.
type Lister interface {
	ListFolder(path string, recursive bool) ([]Entry, error)
}

// CachedLister wraps a Lister and caches successful ListFolder results per
// (path, recursive) key for TTL. A full recursive listing of a large
// Obsidian vault is the slow part of Rando/Clipped (many paginated round
// trips to Dropbox) — caching it means only the first request (or the first
// after the cache goes stale) pays that cost; everything else is just a
// single file download.
type CachedLister struct {
	Lister Lister
	TTL    time.Duration
	Now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedEntries
}

type cachedEntries struct {
	entries  []Entry
	cachedAt time.Time
}

// NewCachedLister builds a CachedLister wrapping lister, caching each
// distinct (path, recursive) query for ttl.
func NewCachedLister(lister Lister, ttl time.Duration) *CachedLister {
	return &CachedLister{
		Lister: lister,
		TTL:    ttl,
		Now:    time.Now,
		cache:  make(map[string]cachedEntries),
	}
}

func (c *CachedLister) ListFolder(path string, recursive bool) ([]Entry, error) {
	key := fmt.Sprintf("%s|%v", path, recursive)

	c.mu.Lock()
	if e, ok := c.cache[key]; ok && c.Now().Sub(e.cachedAt) < c.TTL {
		c.mu.Unlock()
		return e.entries, nil
	}
	c.mu.Unlock()

	entries, err := c.Lister.ListFolder(path, recursive)
	if err != nil {
		// Don't cache failures — a transient Dropbox error shouldn't lock
		// out retries until TTL expires.
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = cachedEntries{entries: entries, cachedAt: c.Now()}
	c.mu.Unlock()

	return entries, nil
}
