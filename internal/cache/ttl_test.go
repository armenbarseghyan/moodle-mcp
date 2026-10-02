package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"moodle-mcp/internal/cache"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// counter returns a loader that yields 1, 2, 3, ... and counts calls.
func counter() (func(context.Context) (int, error), *atomic.Int32) {
	var n atomic.Int32
	return func(context.Context) (int, error) { return int(n.Add(1)), nil }, &n
}

func TestTTL(t *testing.T) {
	t.Parallel()
	type step struct {
		advance   time.Duration
		refresh   bool
		wantValue int
		wantAge   time.Duration
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"miss then hit", []step{
			{0, false, 1, 0},
			{time.Minute, false, 1, time.Minute},
		}},
		{"hit just before expiry", []step{
			{0, false, 1, 0},
			{5*time.Minute - time.Second, false, 1, 5*time.Minute - time.Second},
		}},
		{"expires exactly at ttl", []step{
			{0, false, 1, 0},
			{5 * time.Minute, false, 2, 0},
		}},
		{"refresh bypasses and overwrites", []step{
			{0, false, 1, 0},
			{time.Minute, true, 2, 0},
			{time.Minute, false, 2, time.Minute},
		}},
		{"refresh on empty cache", []step{
			{0, true, 1, 0},
			{0, false, 1, 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			c := cache.New[int](5*time.Minute, clk.Now)
			load, _ := counter()
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				e, err := c.Get(context.Background(), "k", s.refresh, load)
				if err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
				if e.Value != s.wantValue {
					t.Errorf("step %d: value = %d, want %d", i, e.Value, s.wantValue)
				}
				if age := clk.Now().Sub(e.FetchedAt); age != s.wantAge {
					t.Errorf("step %d: age = %v, want %v", i, age, s.wantAge)
				}
			}
		})
	}
}

func TestTTL_KeysAreIndependent(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Minute, newClock().Now)
	load, n := counter()
	a, _ := c.Get(context.Background(), "contents:1", false, load)
	b, _ := c.Get(context.Background(), "contents:2", false, load)
	a2, _ := c.Get(context.Background(), "contents:1", false, load)
	if a.Value != 1 || b.Value != 2 || a2.Value != 1 || n.Load() != 2 {
		t.Fatalf("a=%d b=%d a2=%d loads=%d", a.Value, b.Value, a2.Value, n.Load())
	}
}

func TestTTL_ErrorsAreNotCached(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Minute, newClock().Now)
	boom := errors.New("boom")
	calls := 0
	load := func(context.Context) (int, error) {
		calls++
		if calls == 1 {
			return 0, boom
		}
		return 42, nil
	}
	if _, err := c.Get(context.Background(), "k", false, load); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if c.Len() != 0 {
		t.Fatal("error must not be stored")
	}
	e, err := c.Get(context.Background(), "k", false, load)
	if err != nil || e.Value != 42 || calls != 2 {
		t.Fatalf("e=%v err=%v calls=%d", e, err, calls)
	}
}

func TestTTL_FailedRefreshKeepsOldEntry(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Minute, newClock().Now)
	ctx := context.Background()
	if _, err := c.Get(ctx, "k", false, func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "k", true, func(context.Context) (int, error) { return 0, errors.New("down") }); err == nil {
		t.Fatal("refresh error must be returned")
	}
	e, err := c.Get(ctx, "k", false, func(context.Context) (int, error) { t.Error("must not reload"); return 0, nil })
	if err != nil || e.Value != 1 {
		t.Fatalf("e=%v err=%v", e, err)
	}
}

func TestTTL_Invalidate(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Hour, newClock().Now)
	load, n := counter()
	_, _ = c.Get(context.Background(), "k", false, load)
	c.Invalidate("k")
	e, _ := c.Get(context.Background(), "k", false, load)
	if e.Value != 2 || n.Load() != 2 {
		t.Fatalf("value=%d loads=%d", e.Value, n.Load())
	}
}

func TestTTL_ConcurrentLoadsAreCoalesced(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Minute, newClock().Now)
	release := make(chan struct{})
	var loads atomic.Int32
	load := func(context.Context) (int, error) {
		loads.Add(1)
		<-release
		return 7, nil
	}
	const n = 10
	var wg sync.WaitGroup
	results := make([]int, n)
	for i := range n {
		wg.Go(func() {
			e, err := c.Get(context.Background(), "k", false, load)
			if err != nil {
				t.Error(err)
			}
			results[i] = e.Value
		})
	}
	// Wait until the single load is in flight, then let it finish.
	for loads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond) // let the other goroutines join the flight
	close(release)
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	for i, v := range results {
		if v != 7 {
			t.Errorf("result[%d] = %d", i, v)
		}
	}
}

func TestTTL_CallerCancellationDoesNotBreakSharedLoad(t *testing.T) {
	t.Parallel()
	c := cache.New[int](time.Minute, newClock().Now)
	release := make(chan struct{})
	started := make(chan struct{})
	load := func(ctx context.Context) (int, error) {
		close(started)
		<-release
		return 1, ctx.Err() // must be nil: load is detached from caller cancellation
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	errA := make(chan error, 1)
	go func() {
		_, err := c.Get(ctx1, "k", false, load)
		errA <- err
	}()
	<-started

	resB := make(chan cache.Entry[int], 1)
	go func() {
		e, err := c.Get(context.Background(), "k", false, load)
		if err != nil {
			t.Error(err)
		}
		resB <- e
	}()
	time.Sleep(10 * time.Millisecond)

	cancel1()
	if err := <-errA; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller: err = %v", err)
	}
	close(release)
	if e := <-resB; e.Value != 1 {
		t.Fatalf("other caller got %v", e)
	}
	if c.Len() != 1 {
		t.Fatal("value must be cached after the shared load completes")
	}
}
