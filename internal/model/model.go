// Package model defines the domain types for the railway interlocking engine.
// It is a pure data layer: structs, enums, JSON tags and a few value-methods.
// No persistence, no HTTP, no business rules live here — those are the
// responsibilities of interlocking, signaling, store and service.
//
// The model mirrors a real Computer-Based Interlocking (CBI) yard: a station
// owns switches (points), signals, track sections and track bays; a route
// (train or shunting) is defined as an ordered set of sections plus the
// required positions of the switches it traverses; the engine enforces the
// interlocking safety rules (conflicting-route exclusion, switch locking,
// approach locking, three-point sectional release) on top of these types.
package model

import "time"

// StationID is the identifier of a station.
type StationID = string

// SwitchID identifies a switch (point) within a station.
type SwitchID = string

// SignalID identifies a signal within a station.
type SignalID = string

// SectionID identifies a track section within a station.
type SectionID = string

// TrackID identifies a track bay (a stopping track) within a station.
type TrackID = string

// RouteID identifies a route within a station.
type RouteID = string

// TrainID identifies a train currently in or approaching the station.
type TrainID = string

// SwitchPosition is the two-state position of a switch.
//
// "normal" (the default, facing position, used by the straight route through
// the switch) is represented as PositionNormal; "reverse" (the diverging
// position) as PositionReverse. The numeric value is chosen so that 0 is the
// rest/default position, matching the SQLite integer column.
type SwitchPosition int

const (
	// PositionNormal is the straight, default switch position.
	PositionNormal SwitchPosition = 0
	// PositionReverse is the diverging switch position.
	PositionReverse SwitchPosition = 1
)

// String returns "normal" or "reverse".
func (p SwitchPosition) String() string {
	switch p {
	case PositionNormal:
		return "normal"
	case PositionReverse:
		return "reverse"
	}
	return "unknown"
}

// Opposite returns the other switch position.
func (p SwitchPosition) Opposite() SwitchPosition {
	if p == PositionNormal {
		return PositionReverse
	}
	return PositionNormal
}

// SignalDirection classifies a signal by the movement it protects.
type SignalDirection string

const (
	// DirectionArrival protects an inbound (receiving) train movement.
	DirectionArrival SignalDirection = "arrival"
	// DirectionDeparture protects an outbound (dispatching) train movement.
	DirectionDeparture SignalDirection = "departure"
	// DirectionShunt protects a shunting movement.
	DirectionShunt SignalDirection = "shunt"
)

// SignalAspect is the displayed colour/state of a signal.
//
// The model keeps the four real aspects that matter for interlocking logic:
// R (red — stop), Y (yellow — caution, next signal at R), YY (double yellow —
// medium speed, next two signals diverging), G (green — clear). The engine
// never lets a caller set an aspect directly; ComputeAspect derives it from
// the route and the sections ahead.
type SignalAspect string

const (
	// AspectRed means STOP. A signal shows red unless an established route
	// with all preconditions met allows a permissive aspect.
	AspectRed SignalAspect = "R"
	// AspectYellow means CAUTION; the next signal in the route is red.
	AspectYellow SignalAspect = "Y"
	// AspectDoubleYellow means MEDIUM SPEED; the route diverges at the next
	// switch and the aspect ahead is red/yellow.
	AspectDoubleYellow SignalAspect = "YY"
	// AspectGreen means CLEAR; the route is straight and the next signal is
	// permissive.
	AspectGreen SignalAspect = "G"
)

// SectionKind classifies a track section by its role in the yard.
type SectionKind string

const (
	// SectionSwitch is a section containing one or more switches; occupying it
	// blocks moving those switches.
	SectionSwitch SectionKind = "switch"
	// SectionPlain is an unswitched running section inside the station.
	SectionPlain SectionKind = "plain"
	// SectionApproach is the section immediately before the home signal on the
	// open line; a train occupying it triggers approach locking.
	SectionApproach SectionKind = "approach"
	// SectionDeparture is the section immediately beyond the terminal signal
	// on the open line.
	SectionDeparture SectionKind = "departure"
	// SectionBay is a track-bay section where trains may stand.
	SectionBay SectionKind = "bay"
)

// RouteKind classifies a route as a train movement or a shunting movement.
type RouteKind string

const (
	// RouteKindTrain is a train route (arrival/departure/through). Its approach
	// lock delay, when an occupied approach forces a manual cancel, is 180s.
	RouteKindTrain RouteKind = "train"
	// RouteKindShunt is a shunting route. Its approach-lock delay is 30s.
	RouteKindShunt RouteKind = "shunt"
)

// RouteState is the lifecycle state of a route.
type RouteState string

const (
	// RoutePending: defined in the route table but not yet requested.
	RoutePending RouteState = "pending"
	// RouteRequested: a route establishment has been requested and is being
	// checked/switched. Transient; resolves to Established or Failed.
	RouteRequested RouteState = "requested"
	// RouteEstablished: switches locked to their required positions, sections
	// locked, home signal open to a permissive aspect.
	RouteEstablished RouteState = "established"
	// RouteApproachLocked: an established route whose approach section is now
	// occupied by a train. Manual cancel now requires the timed delay.
	RouteApproachLocked RouteState = "approach_locked"
	// RouteCancelling: a manual cancel has been requested while approach-locked;
	// a timed unlock is counting down. If the train passes the home signal
	// before the timer, the cancel is abandoned and the route returns to
	// Established; otherwise it releases.
	RouteCancelling RouteState = "cancelling"
	// RouteUnlocked: the route has released normally after the train passed
	// (three-point sectional release completed). Terminal state.
	RouteUnlocked RouteState = "unlocked"
	// RouteCancelled: the route was manually cancelled and the timed release
	// completed without the train having passed. Terminal state.
	RouteCancelled RouteState = "cancelled"
	// RouteFailed: establishment failed (occupied section, lost switch
	// indication, or conflicting route already established). No locks held.
	RouteFailed RouteState = "failed"
)

// IsTerminal reports whether the route state admits no further transitions
// except a fresh establishment.
func (s RouteState) IsTerminal() bool {
	switch s {
	case RouteUnlocked, RouteCancelled, RouteFailed:
		return true
	}
	return false
}

// IsLocked reports whether the route currently holds switch/section locks
// (i.e. its conflicting routes may not be established).
func (s RouteState) IsLocked() bool {
	switch s {
	case RouteEstablished, RouteApproachLocked, RouteCancelling:
		return true
	}
	return false
}

// Station is a railway yard owning its switches, signals, sections and tracks.
type Station struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Switch (point) is a movable track element with two positions.
type Switch struct {
	ID             string    `json:"id"`
	StationID      string    `json:"station_id"`
	Name           string    `json:"name"`
	NormalPosition SwitchPosition `json:"normal_position"`
	// CurrentPosition is the position the switch is physically in. On a
	// freshly created switch this equals NormalPosition.
	CurrentPosition SwitchPosition `json:"current_position"`
	// SectionID is the switch section the switch belongs to. A switch may
	// only move when its section is unoccupied and unlocked.
	SectionID string `json:"section_id"`
	// Locked is true while a route holds this switch.
	Locked bool `json:"locked"`
	// LockedByRoute is the route currently holding the switch lock, or "".
	LockedByRoute string `json:"locked_by_route,omitempty"`
	// HasIndication is false when the switch has lost its position
	// indication (a track-circuit/relay fault). A switch without indication
	// can neither move nor be part of an established route.
	HasIndication bool `json:"has_indication"`
	CreatedAt     time.Time `json:"created_at"`
}

// Signal protects a movement and displays an aspect derived by the engine.
type Signal struct {
	ID             string         `json:"id"`
	StationID      string         `json:"station_id"`
	Name           string         `json:"name"`
	Direction      SignalDirection `json:"direction"`
	// Aspect is the currently displayed aspect. It is a recompute of the
	// interlocking state, never a free input.
	Aspect         SignalAspect   `json:"aspect"`
	// Home is true for a home signal that originates a route (the signal at
	// the foot of a route whose permissive aspect opens the route).
	Home bool `json:"home"`
	// ProtectsRouteID is the route this signal currently guards, or "".
	ProtectsRouteID string `json:"protects_route_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Section is a track circuit — the atomic occupancy/locking unit.
type Section struct {
	ID        string      `json:"id"`
	StationID string      `json:"station_id"`
	Name      string      `json:"name"`
	Kind      SectionKind `json:"kind"`
	// Occupied is true while a train (or a block-section hand-off) reports the
	// section occupied.
	Occupied         bool   `json:"occupied"`
	OccupiedByTrain  string `json:"occupied_by_train,omitempty"`
	// Locked is true while a route holds this section.
	Locked           bool   `json:"locked"`
	LockedByRoute    string `json:"locked_by_route,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// TrackBay is a named stopping track. It references the underlying section.
type TrackBay struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	Name      string `json:"name"`
	SectionID string `json:"section_id"`
}

// RouteSwitchPosition is one switch requirement of a route.
type RouteSwitchPosition struct {
	SwitchID        string         `json:"switch_id"`
	RequiredPosition SwitchPosition `json:"required_position"`
}

// RouteSection is one ordered section of a route. Seq orders the sections from
// the approach section (seq 0) to the terminal section (highest seq).
type RouteSection struct {
	SectionID string `json:"section_id"`
	Seq      int    `json:"seq"`
}

// Route is the core interlocking unit.
type Route struct {
	ID            string     `json:"id"`
	StationID     string     `json:"station_id"`
	Code          string     `json:"code"`
	Kind          RouteKind  `json:"kind"`
	// SourceSignalID is the home signal that originates the route.
	SourceSignalID string    `json:"source_signal_id"`
	// Terminal is either a signal ID (train route) or a section ID (shunting
	// route ending in a bay). Stored as a free string; the engine validates it
	// against the yard on definition.
	Terminal        string    `json:"terminal"`
	// SwitchPositions are the switch requirements, in insertion order.
	SwitchPositions  []RouteSwitchPosition `json:"switch_positions"`
	// Sections are the ordered sections the route traverses, seq ascending.
	Sections        []RouteSection `json:"sections"`
	// ApproachSectionID is the section before the home signal whose occupancy
	// triggers approach locking. May be "" for routes with no approach section
	// (a shunt originating inside the yard).
	ApproachSectionID string   `json:"approach_section_id,omitempty"`
	State           RouteState `json:"state"`
	TrainID         string     `json:"train_id,omitempty"`
	EstablishedAt   *time.Time `json:"established_at,omitempty"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty"`
	// UnlockTimerStartedAt is set when a timed (approach-lock) cancel begins.
	UnlockTimerStartedAt *time.Time `json:"unlock_timer_started_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SectionIDs returns the ordered section IDs of the route.
func (r *Route) SectionIDs() []string {
	out := make([]string, len(r.Sections))
	for i, s := range r.Sections {
		out[i] = s.SectionID
	}
	return out
}

// HasSection reports whether the route traverses the given section.
func (r *Route) HasSection(id string) bool {
	for _, s := range r.Sections {
		if s.SectionID == id {
			return true
		}
	}
	return false
}

// SwitchIDs returns the switch IDs required by the route.
func (r *Route) SwitchIDs() []string {
	out := make([]string, len(r.SwitchPositions))
	for i, s := range r.SwitchPositions {
		out[i] = s.SwitchID
	}
	return out
}

// RequiredPositionOf returns the required position of a switch on this route,
// and whether the route cares about that switch.
func (r *Route) RequiredPositionOf(swID string) (SwitchPosition, bool) {
	for _, s := range r.SwitchPositions {
		if s.SwitchID == swID {
			return s.RequiredPosition, true
		}
	}
	return PositionNormal, false
}

// ApproachLockDelay returns the timed-cancel delay for the route kind: 180s
// for train routes, 30s for shunting routes.
func (k RouteKind) ApproachLockDelay() time.Duration {
	if k == RouteKindShunt {
		return 30 * time.Second
	}
	return 180 * time.Second
}

// Train is a train known to the station (approaching or occupying sections).
type Train struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	StationID       string `json:"station_id"`
	PositionSection string `json:"position_section,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// EventKind enumerates the persisted event types. The event log is the source
// of truth for restart recovery: every state change is recorded here, and
// ReconcileAll replays it to rebuild the in-memory yard state.
type EventKind string

const (
	EventSwitchReversed        EventKind = "switch_reversed"
	EventRouteRequested        EventKind = "route_requested"
	EventRouteEstablished      EventKind = "route_established"
	EventRouteFailed           EventKind = "route_failed"
	EventSignalOpened          EventKind = "signal_opened"
	EventSignalClosed          EventKind = "signal_closed"
	EventSectionOccupied       EventKind = "section_occupied"
	EventSectionCleared        EventKind = "section_cleared"
	EventRouteSectionUnlocked  EventKind = "route_section_unlocked"
	EventRouteUnlocked         EventKind = "route_unlocked"
	EventCancelRequested       EventKind = "cancel_requested"
	EventDelayUnlockStarted    EventKind = "delay_unlock_started"
	EventDelayUnlockCompleted  EventKind = "delay_unlock_completed"
	EventDelayUnlockAbandoned  EventKind = "delay_unlock_abandoned"
	EventLockSet               EventKind = "lock_set"
	EventLockReleased          EventKind = "lock_released"
	EventIndicationLost        EventKind = "indication_lost"
	EventIndicationRestored    EventKind = "indication_restored"
	EventSwitchLocked          EventKind = "switch_locked"
	EventSwitchReleased        EventKind = "switch_released"
	EventBlockSectionOccupied  EventKind = "block_section_occupied"
	EventBlockSectionCleared   EventKind = "block_section_cleared"
)

// Event is one row of the append-only interlocking event log.
type Event struct {
	ID         int64     `json:"id"`
	TS         time.Time `json:"ts"`
	StationID  string    `json:"station_id"`
	Kind       EventKind `json:"kind"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	RouteID    string    `json:"route_id,omitempty"`
	// Payload is a JSON blob with kind-specific detail (e.g. the target
	// position of a switch_reversed event).
	Payload    string    `json:"payload,omitempty"`
}

// BlockSection is an open-line block partition between two stations; occupying
// one drives the adjacent approach/departure section state.
type BlockSection struct {
	ID             string `json:"id"`
	StationID      string `json:"station_id"`
	Name           string `json:"name"`
	AdjacentSignal string `json:"adjacent_signal"`
	Occupied       bool   `json:"occupied"`
}
