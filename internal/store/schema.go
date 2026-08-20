package store

// schema is applied once at Open. Tables use INTEGER PRIMARY KEY for stable
// ordering and WAL-friendly writes. Foreign keys are ON (see Open) so a
// station's elements cascade-delete with it.
const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS stations (
	id         TEXT PRIMARY KEY,
	code       TEXT NOT NULL,
	name       TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sections (
	id         TEXT PRIMARY KEY,
	station_id TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	kind       TEXT NOT NULL,
	occupied              INTEGER NOT NULL DEFAULT 0,
	occupied_by_train     TEXT NOT NULL DEFAULT '',
	locked                INTEGER NOT NULL DEFAULT 0,
	locked_by_route       TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sections_station ON sections(station_id);

CREATE TABLE IF NOT EXISTS switches (
	id              TEXT PRIMARY KEY,
	station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	name            TEXT NOT NULL,
	normal_position INTEGER NOT NULL,
	current_position INTEGER NOT NULL,
	section_id      TEXT NOT NULL REFERENCES sections(id),
	locked          INTEGER NOT NULL DEFAULT 0,
	locked_by_route TEXT NOT NULL DEFAULT '',
	has_indication  INTEGER NOT NULL DEFAULT 1,
	created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_switches_station ON switches(station_id);

CREATE TABLE IF NOT EXISTS signals (
	id                TEXT PRIMARY KEY,
	station_id        TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	name              TEXT NOT NULL,
	direction         TEXT NOT NULL,
	aspect            TEXT NOT NULL DEFAULT 'R',
	home              INTEGER NOT NULL DEFAULT 0,
	protects_route_id TEXT NOT NULL DEFAULT '',
	created_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_signals_station ON signals(station_id);

CREATE TABLE IF NOT EXISTS track_bays (
	id         TEXT PRIMARY KEY,
	station_id TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	section_id TEXT NOT NULL REFERENCES sections(id)
);

CREATE TABLE IF NOT EXISTS routes (
	id                       TEXT PRIMARY KEY,
	station_id               TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	code                     TEXT NOT NULL,
	kind                     TEXT NOT NULL,
	source_signal_id         TEXT NOT NULL REFERENCES signals(id),
	terminal                 TEXT NOT NULL,
	approach_section_id      TEXT NOT NULL DEFAULT '',
	state                    TEXT NOT NULL DEFAULT 'pending',
	train_id                 TEXT NOT NULL DEFAULT '',
	established_at           TEXT,
	cancelled_at             TEXT,
	unlock_timer_started_at  TEXT,
	created_at               TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_routes_station ON routes(station_id);
CREATE INDEX IF NOT EXISTS idx_routes_state ON routes(state);

CREATE TABLE IF NOT EXISTS route_switch_positions (
	route_id          TEXT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
	switch_id         TEXT NOT NULL REFERENCES switches(id) ON DELETE CASCADE,
	required_position INTEGER NOT NULL,
	PRIMARY KEY (route_id, switch_id)
);

CREATE TABLE IF NOT EXISTS route_sections (
	route_id   TEXT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
	section_id TEXT NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
	seq        INTEGER NOT NULL,
	PRIMARY KEY (route_id, section_id)
);

CREATE TABLE IF NOT EXISTS trains (
	id              TEXT PRIMARY KEY,
	code            TEXT NOT NULL,
	station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	position_section TEXT NOT NULL DEFAULT '',
	created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS block_sections (
	id              TEXT PRIMARY KEY,
	station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
	name            TEXT NOT NULL,
	adjacent_signal TEXT NOT NULL DEFAULT '',
	occupied        INTEGER NOT NULL DEFAULT 0
);

-- The append-only event log: the source of truth for restart recovery.
CREATE TABLE IF NOT EXISTS events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	ts          TEXT NOT NULL,
	station_id  TEXT NOT NULL,
	kind        TEXT NOT NULL,
	entity_type TEXT NOT NULL DEFAULT '',
	entity_id   TEXT NOT NULL DEFAULT '',
	route_id    TEXT NOT NULL DEFAULT '',
	payload     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_station_ts ON events(station_id, ts, id);

-- Approach-lock delay timers in flight, for restart resumption.
CREATE TABLE IF NOT EXISTS delay_unlock_timers (
	route_id   TEXT PRIMARY KEY REFERENCES routes(id) ON DELETE CASCADE,
	station_id TEXT NOT NULL,
	kind       TEXT NOT NULL,
	started_at TEXT NOT NULL,
	delay_secs INTEGER NOT NULL,
	completed  INTEGER NOT NULL DEFAULT 0
);

-- Per-route "section has been occupied at least once" bookkeeping, for the
-- three-point release. Rebuilt from events on restart; persisted so a restart
-- mid-train preserves the once-occupied flag without replaying occupancy
-- from the route's establishment.
CREATE TABLE IF NOT EXISTS route_section_occupied (
	route_id   TEXT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
	section_id TEXT NOT NULL,
	once       INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (route_id, section_id)
);
`
