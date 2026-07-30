// Package poller runs a function on an interval that can change at runtime.
//
// The refresh loops live here, in Go, rather than as setInterval in the
// frontend. Both WebKitGTK and Chromium throttle timers in a hidden page —
// Chromium down to one wake per minute after five minutes — and Raphael's whole
// reason for polling is to notice a new review task while its window is *not*
// in front. A backend ticker is not subject to that.
package poller

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Poller calls run on an interval. The zero value is not usable; use New.
type Poller struct {
	run func(context.Context)

	// interval is atomic because Reload is called from bound frontend methods
	// while the loop goroutine is reading it.
	interval atomic.Int64

	trigger chan struct{}
	reload  chan struct{}
	stop    chan struct{}
	done    chan struct{}

	started   atomic.Bool
	startOnce sync.Once
	stopOnce  sync.Once
}

// New builds a Poller. An interval of zero or less means no automatic ticks;
// Trigger still works, which is how "auto-refresh off" behaves.
func New(interval time.Duration, run func(context.Context)) *Poller {
	p := &Poller{
		run: run,
		// Both signal channels are buffered and coalescing: several triggers
		// arriving during one slow run collapse into a single follow-up.
		trigger: make(chan struct{}, 1),
		reload:  make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	p.interval.Store(int64(interval))

	return p
}

// Start begins the loop. Calling it more than once is a no-op.
func (p *Poller) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		p.started.Store(true)
		go p.loop(ctx)
	})
}

// Trigger asks for an immediate run. It never blocks: if a run is already in
// flight, one follow-up is queued and further triggers are dropped.
func (p *Poller) Trigger() {
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

// Reload changes the cadence, taking effect from the next tick rather than
// waiting out the current one.
func (p *Poller) Reload(interval time.Duration) {
	p.interval.Store(int64(interval))

	select {
	case p.reload <- struct{}{}:
	default:
	}
}

// Stop ends the loop and waits for any in-flight run to finish. Safe to call
// more than once, and safe to call on a Poller that was never started.
func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })

	// Only wait if the loop is actually running; done is never closed otherwise.
	if p.started.Load() {
		<-p.done
	}
}

func (p *Poller) loop(ctx context.Context) {
	defer close(p.done)

	timer := time.NewTimer(time.Hour)
	p.disarm(timer)
	p.arm(timer)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case <-p.reload:
			p.arm(timer)
		case <-p.trigger:
			p.run(ctx)
			p.arm(timer)
		case <-timer.C:
			p.run(ctx)
			p.arm(timer)
		}
	}
}

// arm restarts the timer for the current interval, or leaves it stopped when
// automatic refresh is disabled.
func (p *Poller) arm(timer *time.Timer) {
	p.disarm(timer)

	if d := time.Duration(p.interval.Load()); d > 0 {
		timer.Reset(d)
	}
}

func (p *Poller) disarm(timer *time.Timer) {
	timer.Stop()
	// Drain a tick that fired between Stop and here, so the next Reset starts
	// from a clean channel rather than firing immediately.
	select {
	case <-timer.C:
	default:
	}
}
