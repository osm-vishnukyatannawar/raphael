package poller_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/poller"
)

// waitFor polls a condition instead of sleeping a fixed amount, so the tests
// stay fast locally and don't flake on a loaded CI runner.
func waitFor(t *testing.T, why string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", why)
}

func TestTicksOnInterval(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	p := poller.New(5*time.Millisecond, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	t.Cleanup(p.Stop)

	waitFor(t, "three ticks", func() bool { return runs.Load() >= 3 })
}

func TestZeroIntervalDisablesAutomaticTicks(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	p := poller.New(0, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	t.Cleanup(p.Stop)

	time.Sleep(50 * time.Millisecond)
	if got := runs.Load(); got != 0 {
		t.Errorf("ran %d times with the interval disabled, want 0", got)
	}

	// Manual refresh must still work while auto-refresh is off.
	p.Trigger()
	waitFor(t, "the manual trigger to run", func() bool { return runs.Load() == 1 })
}

func TestTriggerRunsImmediately(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	// An interval far longer than the test: any run must come from Trigger.
	p := poller.New(time.Hour, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	t.Cleanup(p.Stop)

	p.Trigger()
	waitFor(t, "the triggered run", func() bool { return runs.Load() == 1 })
}

func TestReloadTakesEffectWithoutWaitingOutTheOldInterval(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	p := poller.New(time.Hour, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	t.Cleanup(p.Stop)

	p.Reload(5 * time.Millisecond)
	waitFor(t, "ticks at the new interval", func() bool { return runs.Load() >= 3 })
}

func TestReloadToZeroStopsTicking(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	p := poller.New(5*time.Millisecond, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	t.Cleanup(p.Stop)

	waitFor(t, "the first tick", func() bool { return runs.Load() >= 1 })
	p.Reload(0)

	// Allow the in-flight cycle to settle, then confirm the count is frozen.
	time.Sleep(20 * time.Millisecond)
	settled := runs.Load()
	time.Sleep(50 * time.Millisecond)

	if got := runs.Load(); got != settled {
		t.Errorf("ran %d more times after being disabled", got-settled)
	}
}

func TestStopEndsTheLoop(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	p := poller.New(5*time.Millisecond, func(context.Context) { runs.Add(1) })

	p.Start(t.Context())
	waitFor(t, "the first tick", func() bool { return runs.Load() >= 1 })

	p.Stop()
	after := runs.Load()

	time.Sleep(50 * time.Millisecond)
	if got := runs.Load(); got != after {
		t.Errorf("ran %d more times after Stop", got-after)
	}

	// Stop is idempotent — shutdown paths should not have to track whether it
	// has already been called.
	p.Stop()
}

func TestStopOnAnUnstartedPollerDoesNotBlock(t *testing.T) {
	t.Parallel()

	// Startup can fail before the pollers are started; shutdown still runs.
	poller.New(time.Second, func(context.Context) {}).Stop()
}

func TestCancellingTheContextEndsTheLoop(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	ctx, cancel := context.WithCancel(t.Context())

	p := poller.New(5*time.Millisecond, func(context.Context) { runs.Add(1) })
	p.Start(ctx)
	t.Cleanup(p.Stop)

	waitFor(t, "the first tick", func() bool { return runs.Load() >= 1 })
	cancel()

	time.Sleep(20 * time.Millisecond)
	settled := runs.Load()
	time.Sleep(50 * time.Millisecond)

	if got := runs.Load(); got != settled {
		t.Errorf("ran %d more times after the context was cancelled", got-settled)
	}
}

// Triggers arriving during a slow run must not queue up into a burst.
func TestTriggersCoalesce(t *testing.T) {
	t.Parallel()

	var runs atomic.Int64
	release := make(chan struct{})

	p := poller.New(time.Hour, func(context.Context) {
		runs.Add(1)
		<-release
	})
	p.Start(t.Context())
	t.Cleanup(func() {
		close(release)
		p.Stop()
	})

	p.Trigger()
	waitFor(t, "the first run to start", func() bool { return runs.Load() == 1 })

	for range 10 {
		p.Trigger()
	}
	release <- struct{}{} // let the first run finish

	waitFor(t, "the coalesced follow-up", func() bool { return runs.Load() == 2 })
	time.Sleep(30 * time.Millisecond)

	if got := runs.Load(); got != 2 {
		t.Errorf("ran %d times, want 2 — ten triggers should collapse into one", got)
	}
}
