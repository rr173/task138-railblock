package service

import (
	"testing"

	"task138-railblock/internal/model"
)

// ev is a small helper to build an event for replayEvents tests.
func ev(kind model.EventKind, routeID, entityType, payload string) *model.Event {
	return &model.Event{Kind: kind, EntityType: entityType, RouteID: routeID, Payload: payload}
}

// TestReplayEventsReconstructsApproachLocked guards the restart-recovery
// consistency of the approach-lock: a route that was established and then
// approach-locked (lock_set route event with approach_locked=1) must rebuild
// to RouteApproachLocked, so a crash while approach-locked is not silently
// reverted to Established — which would let a manual cancel release at once
// instead of entering the timed delay.
func TestReplayEventsReconstructsApproachLocked(t *testing.T) {
	events := []*model.Event{
		ev(model.EventRouteEstablished, "R", "route", ""),
		ev(model.EventLockSet, "R", "route", `{"approach_locked":"1"}`),
	}
	rs := replayEvents(events)
	if got := rs.routeStates["R"]; got != model.RouteApproachLocked {
		t.Fatalf("replay state = %s, want approach_locked", got)
	}
}

// TestReplayEventsApproachLockOnlyAfterEstablished ensures a stray
// approach-locked marker does not resurrect a terminal route.
func TestReplayEventsApproachLockOnlyAfterEstablished(t *testing.T) {
	events := []*model.Event{
		ev(model.EventRouteEstablished, "R", "route", ""),
		ev(model.EventCancelRequested, "R", "route", ""),
		ev(model.EventDelayUnlockCompleted, "R", "route", ""),
		ev(model.EventLockSet, "R", "route", `{"approach_locked":"1"}`),
	}
	rs := replayEvents(events)
	if got := rs.routeStates["R"]; got != model.RouteCancelled {
		t.Fatalf("replay state = %s, want cancelled (terminal wins)", got)
	}
}

// TestReplayEventsAbandonReEnablesApproachLock ensures that after a timed
// cancel is abandoned (route back to Established) a fresh approach-lock marker
// rebuilds to ApproachLocked again.
func TestReplayEventsAbandonReEnablesApproachLock(t *testing.T) {
	events := []*model.Event{
		ev(model.EventRouteEstablished, "R", "route", ""),
		ev(model.EventCancelRequested, "R", "route", ""),
		ev(model.EventDelayUnlockAbandoned, "R", "route", ""),
		ev(model.EventLockSet, "R", "route", `{"approach_locked":"1"}`),
	}
	rs := replayEvents(events)
	if got := rs.routeStates["R"]; got != model.RouteApproachLocked {
		t.Fatalf("replay state = %s, want approach_locked after re-lock", got)
	}
}

// TestReplayEventsMalformedPayloadIsIgnored ensures recovery is tolerant of a
// malformed approach-locked payload (treated as not approach-locked).
func TestReplayEventsMalformedPayloadIsIgnored(t *testing.T) {
	events := []*model.Event{
		ev(model.EventRouteEstablished, "R", "route", ""),
		ev(model.EventLockSet, "R", "route", `{not-json`),
	}
	rs := replayEvents(events)
	if got := rs.routeStates["R"]; got != model.RouteEstablished {
		t.Fatalf("replay state = %s, want established (malformed ignored)", got)
	}
}

// TestHasApproachLockedPayload covers the payload helper directly.
func TestHasApproachLockedPayload(t *testing.T) {
	cases := []struct {
		payload string
		want     bool
	}{
		{`{"approach_locked":"1"}`, true},
		{`{"approach_locked":"0"}`, false},
		{`{}`, false},
		{``, false},
		{`{bad`, false},
		{`{"at":"x"}`, false},
	}
	for _, c := range cases {
		if got := hasApproachLockedPayload(c.payload); got != c.want {
			t.Errorf("hasApproachLockedPayload(%q) = %v, want %v", c.payload, got, c.want)
		}
	}
}
