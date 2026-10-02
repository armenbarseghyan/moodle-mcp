// Package cache provides an in-memory TTL cache with request coalescing.
package cache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Entry is a cached value with the time it was fetched.
type Entry[V any] struct {
	Value     V
	FetchedAt time.Time
}

// TTL caches values per string key for a fixed duration.
//
// Concurrent loads of the same key are coalesced into one call (singleflight).
// Errors are never cached. A refresh request bypasses a fresh entry, loads the
// value again and overwrites the entry.
type TTL[V any] struct {
	ttl   time.Duration
	now   func() time.Time
	mu    sync.Mutex
	items map[string]Entry[V]
	group singleflight.Group
}

// New returns a cache whose entries live for ttl. now defaults to time.Now.
func New[V any](ttl time.Duration, now func() time.Time) *TTL[V] {
	if now == nil {
		now = time.Now
	}
	return &TTL[V]{ttl: ttl, now: now, items: map[string]Entry[V]{}}
}

// Get returns the cached entry for key, or calls load and caches its result.
//
// load runs detached from ctx's cancellation (but keeps its values), so one
// caller giving up does not fail other callers waiting for the same key.
// Each caller still stops waiting when its own ctx is done.
func (c *TTL[V]) Get(ctx context.Context, key string, refresh bool, load func(context.Context) (V, error)) (Entry[V], error) {
	if !refresh {
		if e, ok := c.lookup(key); ok {
			return e, nil
		}
	}
	ch := c.group.DoChan(key, func() (any, error) {
		v, err := load(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		e := Entry[V]{Value: v, FetchedAt: c.now()}
		c.mu.Lock()
		c.items[key] = e
		c.mu.Unlock()
		return e, nil
	})
	select {
	case res := <-ch:
		if res.Err != nil {
			var zero Entry[V]
			return zero, res.Err
		}
		return res.Val.(Entry[V]), nil
	case <-ctx.Done():
		var zero Entry[V]
		return zero, ctx.Err()
	}
}

// Invalidate drops key.
func (c *TTL[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Len returns the number of stored (possibly expired) entries.
func (c *TTL[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *TTL[V]) lookup(key string) (Entry[V], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return e, false
	}
	if c.ttl > 0 && c.now().Sub(e.FetchedAt) >= c.ttl {
		delete(c.items, key)
		return e, false
	}
	return e, true
}
