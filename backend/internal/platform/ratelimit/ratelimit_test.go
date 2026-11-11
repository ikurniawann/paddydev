package ratelimit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/ratelimit"
	"nuhabit/backend/internal/platform/testutil"
)

// key returns a fresh key whose rows are removed after the test.
func key(t *testing.T) string {
	k := "test:" + t.Name() + ":" + testutil.RandomHex(4)
	t.Cleanup(func() {
		_, _ = testutil.DB(t).Exec(context.Background(), `DELETE FROM platform.rate_limits WHERE key = $1`, k)
	})
	return k
}

// Two limiters stand in for two API replicas on one database.
func replicas(t *testing.T) (*ratelimit.Limiter, *ratelimit.Limiter) {
	db := testutil.DB(t)
	return ratelimit.New(db), ratelimit.New(db)
}

func TestFixedWindowIsSharedAcrossReplicas(t *testing.T) {
	a, b := replicas(t)
	ctx := context.Background()
	k := key(t)
	now := time.Now().Truncate(time.Microsecond)
	for i, l := range []*ratelimit.Limiter{a, b, a} {
		w, err := l.Fixed(ctx, k, 3, time.Minute, now)
		if err != nil || !w.Allowed || w.Count != i+1 {
			t.Fatalf("hit %d: %+v %v", i+1, w, err)
		}
	}
	w, err := b.Fixed(ctx, k, 3, time.Minute, now.Add(30*time.Second))
	if err != nil || w.Allowed || w.Count != 3 || !w.ResetAt.Equal(now.Add(time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("4th hit on the other replica: %+v %v", w, err)
	}
	peek, err := a.Peek(ctx, k)
	if err != nil || peek == nil || peek.Count != 3 {
		t.Fatalf("peek: %+v %v", peek, err)
	}
	// The window resets one minute after its first hit, like lib/rate-limit.ts.
	w, err = a.Fixed(ctx, k, 3, time.Minute, now.Add(61*time.Second))
	if err != nil || !w.Allowed || w.Count != 1 {
		t.Fatalf("after reset: %+v %v", w, err)
	}
	if peek, err := a.Peek(ctx, "test:never-hit:"+testutil.RandomHex(4)); err != nil || peek != nil {
		t.Fatalf("peek of an unknown key: %+v %v", peek, err)
	}
}

func TestSlidingWindowIsSharedAcrossReplicas(t *testing.T) {
	a, b := replicas(t)
	ctx := context.Background()
	k := key(t)
	start := time.Now().Truncate(time.Microsecond)
	if ok, _, err := a.Sliding(ctx, k, 2, time.Minute, start); err != nil || !ok {
		t.Fatalf("1st: %v %v", ok, err)
	}
	if ok, _, err := b.Sliding(ctx, k, 2, time.Minute, start.Add(20*time.Second)); err != nil || !ok {
		t.Fatalf("2nd: %v %v", ok, err)
	}
	ok, retry, err := a.Sliding(ctx, k, 2, time.Minute, start.Add(30*time.Second))
	if err != nil || ok || retry != 30*time.Second {
		t.Fatalf("3rd on the first replica: %v %v %v", ok, retry, err)
	}
	// The oldest hit leaves the window; the refused one was never recorded.
	if ok, _, err := b.Sliding(ctx, k, 2, time.Minute, start.Add(61*time.Second)); err != nil || !ok {
		t.Fatalf("after the first hit expired: %v %v", ok, err)
	}
	if ok, _, _ := a.Sliding(ctx, k, 2, time.Minute, start.Add(62*time.Second)); ok {
		t.Fatal("window holds the hits at 20s and 61s")
	}
}

func TestSlidingCountReadsWithoutRecording(t *testing.T) {
	a, b := replicas(t)
	ctx := context.Background()
	k := key(t)
	start := time.Now().Truncate(time.Microsecond)
	if n, err := a.SlidingCount(ctx, k, time.Minute, start); err != nil || n != 0 {
		t.Fatalf("unknown key: %d %v", n, err)
	}
	for i := range 2 {
		if ok, _, err := b.Sliding(ctx, k, 5, time.Minute, start.Add(time.Duration(i)*30*time.Second)); err != nil || !ok {
			t.Fatalf("hit %d: %v %v", i, ok, err)
		}
	}
	for range 2 { // reading twice records nothing
		if n, err := a.SlidingCount(ctx, k, time.Minute, start.Add(40*time.Second)); err != nil || n != 2 {
			t.Fatalf("count: %d %v", n, err)
		}
	}
	if n, _ := a.SlidingCount(ctx, k, time.Minute, start.Add(61*time.Second)); n != 1 {
		t.Fatalf("the first hit left the window: %d", n)
	}
}

func TestConcurrentHitsNeverExceedTheLimit(t *testing.T) {
	a, b := replicas(t)
	ctx := context.Background()
	fixed, sliding := key(t), key(t)
	now := time.Now().Truncate(time.Microsecond)
	var fixedOK, slidingOK atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		l := a
		if i%2 == 1 {
			l = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w, err := l.Fixed(ctx, fixed, 5, time.Minute, now); err == nil && w.Allowed {
				fixedOK.Add(1)
			}
			if ok, _, err := l.Sliding(ctx, sliding, 5, time.Minute, now); err == nil && ok {
				slidingOK.Add(1)
			}
		}()
	}
	wg.Wait()
	if fixedOK.Load() != 5 || slidingOK.Load() != 5 {
		t.Fatalf("allowed fixed=%d sliding=%d, want 5 each", fixedOK.Load(), slidingOK.Load())
	}
}

func TestPruneDropsExpiredKeys(t *testing.T) {
	l, _ := replicas(t)
	ctx := context.Background()
	old, live := key(t), key(t)
	past := time.Now().Add(-2 * time.Hour)
	if _, err := l.Fixed(ctx, old, 1, time.Minute, past); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Sliding(ctx, live, 1, time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Prune skips rows another replica's prune holds (SKIP LOCKED), and CI
	// runs packages in parallel on one database: retry briefly (EPIC-105 R6).
	gone := false
	for i := 0; i < 20 && !gone; i++ {
		if err := l.Prune(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
		if w, _ := l.Peek(ctx, old); w == nil {
			gone = true
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !gone {
		t.Fatal("expired key kept")
	}
	if ok, _, _ := l.Sliding(ctx, live, 1, time.Hour, time.Now()); ok {
		t.Fatal("live key pruned")
	}
}
