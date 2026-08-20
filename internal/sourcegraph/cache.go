package sourcegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type cacheStatus int

const (
	cacheMiss cacheStatus = iota
	cacheFresh
	cacheStaleYoung // past TTL but within staleMax — return + refresh in bg
	cacheStaleOld   // past staleMax — caller must synchronously refetch
)

const staleMax = 30 * time.Minute

type cacheEntry struct {
	res       FetchResult
	createdAt time.Time
}

type ttlCache struct {
	ttl      time.Duration
	mu       sync.Mutex
	entries  map[string]cacheEntry
	inFlight map[string]struct{} // refresh deduplication
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{
		ttl:      ttl,
		entries:  map[string]cacheEntry{},
		inFlight: map[string]struct{}{},
	}
}

func (c *ttlCache) put(key string, res FetchResult) {
	c.putAt(key, res, time.Now())
}

func (c *ttlCache) putAt(key string, res FetchResult, at time.Time) {
	c.mu.Lock()
	c.entries[key] = cacheEntry{res: res, createdAt: at}
	c.mu.Unlock()
}

// get returns the cached entry only when fresh. Use lookup for stale-aware reads.
func (c *ttlCache) get(key string, now time.Time) (FetchResult, bool) {
	res, status := c.lookup(key, now)
	return res, status == cacheFresh
}

func (c *ttlCache) lookup(key string, now time.Time) (FetchResult, cacheStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return FetchResult{}, cacheMiss
	}
	age := now.Sub(e.createdAt)
	switch {
	case age < c.ttl:
		return e.res, cacheFresh
	case age < staleMax:
		return e.res, cacheStaleYoung
	default:
		return e.res, cacheStaleOld
	}
}

func (c *ttlCache) Purge() {
	c.mu.Lock()
	c.entries = map[string]cacheEntry{}
	c.mu.Unlock()
}

// persisted is the on-disk JSON shape for one cache entry. Body is []byte
// (Go marshals it as base64), so arbitrary file content round-trips.
type persisted struct {
	Key       string `json:"key"`
	Body      []byte `json:"body"`
	SHA       string `json:"sha"`
	FetchedAt int64  `json:"fetched_at"`
}

func keyFile(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}

// loadDir reads every *.json entry under dir into the cache, preserving age.
func (c *ttlCache) loadDir(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return // missing dir is fine — empty cache
	}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var p persisted
		if json.Unmarshal(b, &p) != nil || p.Key == "" {
			continue
		}
		c.putAt(p.Key, FetchResult{Body: p.Body, SHA: p.SHA}, time.Unix(p.FetchedAt, 0))
	}
}

// writeFile persists one entry atomically (temp + rename).
func writeFile(dir, key string, res FetchResult, at time.Time) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	p := persisted{Key: key, Body: res.Body, SHA: res.SHA, FetchedAt: at.Unix()}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	tmp := keyFile(dir, key) + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, keyFile(dir, key))
}

// expireAll backdates every entry past any plausible TTL (test helper).
func (c *ttlCache) expireAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		e.createdAt = time.Unix(0, 0)
		c.entries[k] = e
	}
}

// scheduleRefresh runs fn in a goroutine if no refresh for key is already in
// flight. The result replaces the cached value on success.
func (c *ttlCache) scheduleRefresh(key string, fn func() (FetchResult, error)) {
	c.mu.Lock()
	if _, busy := c.inFlight[key]; busy {
		c.mu.Unlock()
		return
	}
	c.inFlight[key] = struct{}{}
	c.mu.Unlock()
	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.inFlight, key)
			c.mu.Unlock()
		}()
		res, err := fn()
		if err != nil {
			return
		}
		c.put(key, res)
	}()
}
