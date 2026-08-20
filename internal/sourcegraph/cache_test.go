package sourcegraph

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_Hit(t *testing.T) {
	c := newTTLCache(5 * time.Minute)
	c.put("k", FetchResult{Body: []byte("v"), SHA: "abc"})
	got, hit := c.get("k", time.Now())
	if !hit {
		t.Fatal("cache.get: not hit")
	}
	if string(got.Body) != "v" || got.SHA != "abc" {
		t.Errorf("got = %+v, want body=v sha=abc", got)
	}
}

func TestCache_StaleYoung_ReturnsStaleSchedulesRefresh(t *testing.T) {
	c := newTTLCache(5 * time.Minute)
	c.putAt("k", FetchResult{Body: []byte("v")}, time.Now().Add(-10*time.Minute))
	// 10 minutes old: past 5-min TTL but inside 30-min stale window
	got, status := c.lookup("k", time.Now())
	if status != cacheStaleYoung {
		t.Fatalf("status = %v, want cacheStaleYoung", status)
	}
	if string(got.Body) != "v" {
		t.Errorf("got body = %q, want v", got.Body)
	}
}

func TestCache_StaleOld_NotReturned(t *testing.T) {
	c := newTTLCache(5 * time.Minute)
	c.putAt("k", FetchResult{Body: []byte("v")}, time.Now().Add(-45*time.Minute))
	_, status := c.lookup("k", time.Now())
	if status != cacheStaleOld {
		t.Fatalf("status = %v, want cacheStaleOld", status)
	}
}

func TestCache_Purge(t *testing.T) {
	c := newTTLCache(5 * time.Minute)
	c.put("k", FetchResult{Body: []byte("v")})
	c.Purge()
	_, hit := c.get("k", time.Now())
	if hit {
		t.Fatal("cache.get after Purge: still hit")
	}
}

// Confirm refresh deduplication: one background refresh in flight at a time.
func TestCache_RefreshDeduplication(t *testing.T) {
	c := newTTLCache(5 * time.Minute)
	c.putAt("k", FetchResult{Body: []byte("old")}, time.Now().Add(-10*time.Minute))
	var calls int64
	refresh := func() (FetchResult, error) {
		atomic.AddInt64(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return FetchResult{Body: []byte("new")}, nil
	}
	// Two simultaneous stale-young hits should issue exactly ONE refresh.
	c.scheduleRefresh("k", refresh)
	c.scheduleRefresh("k", refresh)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt64(&calls) != 1 {
		t.Errorf("refresh calls = %d, want 1 (dedup)", calls)
	}
}
