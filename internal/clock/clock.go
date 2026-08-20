// Package clock abstracts wall-clock time so the interlocking engine can be
// driven deterministically in tests and in the smoke-test. Delayed
// (approach-lock) unlock timers are advanced by an injected clock instead of
// real time, so a self-check can complete an approach-lock delay without
// sleeping.
package clock

import (
	"context"
	"sync"
	"time"
)

// Clock is the minimal interface the services depend on. Production uses Real;
// tests/self-check use Fake and advance it explicitly.
type Clock interface {
	// Now returns the current instant.
	Now() time.Time
	// After returns a channel that fires once after d has elapsed on this
	// clock. For the Fake clock it fires when the fake time is advanced past
	// now+d. The real clock uses time.After.
	After(d time.Duration) <-chan time.Time
}

// Real is the production clock backed by the wall clock.
type Real struct{}

// Now returns the wall-clock time.
func (Real) Now() time.Time { return time.Now() }

// After returns time.After(d).
func (Real) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Fake is a controllable clock. It is safe for concurrent use. Advance moves
// Now forward and fires any pending After channels whose deadline has passed.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	pending []fakeTimer
}

// fakeTimer records a pending After call.
type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	fired    bool
}

// NewFake creates a Fake clock anchored at t. Callers should pass a fixed base
// time (the self-check passes a constant) so timestamps are deterministic.
func NewFake(t time.Time) *Fake {
	return &Fake{now: t}
}

// Now returns the fake current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// After registers a timer that fires when the fake clock is advanced past
// now+d. Returns a buffered (size 1) channel so a late Advance still delivers.
func (f *Fake) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan time.Time, 1)
	f.pending = append(f.pending, fakeTimer{deadline: f.now.Add(d), ch: ch})
	return ch
}

// Advance moves the fake clock forward by d and fires every pending timer whose
// deadline is <= the new time, in deadline order. Fired timers are removed.
// Advance does not block waiting for receivers.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	// Fire in deadline order so earlier timers fire first.
	kept := f.pending[:0]
	var fired []fakeTimer
	for _, t := range f.pending {
		if !f.now.Before(t.deadline) {
			fired = append(fired, t)
		} else {
			kept = append(kept, t)
		}
	}
	f.pending = kept
	for _, t := range fired {
		if !t.fired {
			// Non-blocking send: channel is buffered size 1.
			select {
			case t.ch <- f.now:
			default:
			}
		}
	}
}

// HasPending reports whether any registered timer has not yet fired. Used by
// the self-check to assert timer state across a restart.
func (f *Fake) HasPending() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pending) > 0
}

// ctxKey is an unexported type for context-stored clocks so callers cannot
// collide on the key.
type ctxKey struct{}

// WithClock returns ctx with c attached. Service methods retrieve it via
// FromContext, falling back to Real when absent.
func WithClock(ctx context.Context, c Clock) context.Context {
	if c == nil {
		c = Real{}
	}
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the clock stored in ctx, or Real{} if none is present.
func FromContext(ctx context.Context) Clock {
	if c, ok := ctx.Value(ctxKey{}).(Clock); ok {
		return c
	}
	return Real{}
}
