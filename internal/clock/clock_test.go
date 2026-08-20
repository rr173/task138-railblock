package clock

import (
	"context"
	"testing"
	"time"
)

// TestFakeNowAndAdvance: Now returns the base time; Advance moves it.
func TestFakeNowAndAdvance(t *testing.T) {
	base := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	f := NewFake(base)
	if !f.Now().Equal(base) {
		t.Fatalf("Now = %v, want %v", f.Now(), base)
	}
	f.Advance(2 * time.Hour)
	want := base.Add(2 * time.Hour)
	if !f.Now().Equal(want) {
		t.Fatalf("after advance: Now = %v, want %v", f.Now(), want)
	}
}

// TestFakeAfterFires: a registered timer fires once the fake clock advances
// past its deadline.
func TestFakeAfterFires(t *testing.T) {
	base := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	f := NewFake(base)
	ch := f.After(30 * time.Second)
	// The timer is registered but unfired, so it is pending.
	if !f.HasPending() {
		t.Fatal("registered-but-unfired timer should be pending")
	}
	f.Advance(30 * time.Second)
	select {
	case <-ch:
		// ok
	default:
		t.Fatal("timer should have fired after advancing 30s")
	}
	if f.HasPending() {
		t.Fatal("no pending timers after firing")
	}
}

// TestFakeAfterNotFiredBeforeDeadline: advancing less than the deadline does not
// fire the timer.
func TestFakeAfterNotFiredBeforeDeadline(t *testing.T) {
	base := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	f := NewFake(base)
	ch := f.After(time.Minute)
	f.Advance(30 * time.Second)
	select {
	case <-ch:
		t.Fatal("timer must not fire before deadline")
	default:
	}
	if !f.HasPending() {
		t.Fatal("timer should still be pending")
	}
}

// TestWithClockFromContext: FromContext returns the stored clock or Real.
func TestWithClockFromContext(t *testing.T) {
	if _, ok := FromContext(context.Background()).(Real); !ok {
		t.Fatal("absent clock should default to Real")
	}
	f := NewFake(time.Unix(0, 0))
	ctx := WithClock(context.Background(), f)
	if FromContext(ctx) != f {
		t.Fatal("FromContext should return the injected fake")
	}
}
