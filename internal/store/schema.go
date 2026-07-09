package store

// schema is the SQLite DDL for the local cache. Every table has an explicit,
// NOT NULL primary key sourced from the provider's own id field — the sync bug
// we already paid for came from an id-less resource silently storing zero rows.
// The regulatory_actions table stores every agency (FDA, EMA, HealthCanada,
// TGA, PMDA) in one place, keyed by (agency, source_id).
const schema = `
CREATE TABLE IF NOT EXISTS devices (
    id                 TEXT PRIMARY KEY NOT NULL,
    name               TEXT,
    manufacturer       TEXT,
    fda_class          TEXT,
    regulatory_status  TEXT,
    raw                TEXT,
    fetched_at         TEXT
);

CREATE TABLE IF NOT EXISTS udis (
    udi_di          TEXT PRIMARY KEY NOT NULL,
    device_id       TEXT,
    manufacturer    TEXT,
    model           TEXT,
    commercial_name TEXT,
    raw             TEXT,
    fetched_at      TEXT
);

CREATE TABLE IF NOT EXISTS regulatory_actions (
    source_id     TEXT NOT NULL,
    agency        TEXT NOT NULL,
    jurisdiction  TEXT,
    device_id     TEXT,
    action_type   TEXT,
    status        TEXT,
    date          TEXT,
    url           TEXT,
    raw           TEXT,
    fetched_at    TEXT,
    PRIMARY KEY (agency, source_id)
);

CREATE TABLE IF NOT EXISTS adverse_events (
    maude_id      TEXT PRIMARY KEY NOT NULL,
    device_id     TEXT,
    event_count   INTEGER,
    serious_count INTEGER,
    death_count   INTEGER,
    raw           TEXT,
    fetched_at    TEXT
);

CREATE TABLE IF NOT EXISTS clinical_trials (
    trial_id         TEXT PRIMARY KEY NOT NULL,
    device_id        TEXT,
    phase            TEXT,
    status           TEXT,
    recruiter_count  INTEGER,
    raw              TEXT,
    fetched_at       TEXT
);

CREATE TABLE IF NOT EXISTS publications (
    id         TEXT PRIMARY KEY NOT NULL,
    device_id  TEXT,
    pubmed_id  TEXT,
    title      TEXT,
    pmid       TEXT,
    doi        TEXT,
    raw        TEXT,
    fetched_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_reg_agency  ON regulatory_actions(agency);
CREATE INDEX IF NOT EXISTS idx_reg_device  ON regulatory_actions(device_id);
CREATE INDEX IF NOT EXISTS idx_recalls_dev ON adverse_events(device_id);

-- records is the generic sync cache: one row per provider record, keyed by
-- (source, record_id) so ids from different providers can never collide —
-- the source-qualified uniqueness the per-entity tables above lack.
CREATE TABLE IF NOT EXISTS records (
    source     TEXT NOT NULL,
    record_id  TEXT NOT NULL,
    term       TEXT,
    date       TEXT,
    summary    TEXT,
    raw        TEXT,
    fetched_at TEXT,
    PRIMARY KEY (source, record_id)
);
CREATE INDEX IF NOT EXISTS idx_records_term ON records(term);

-- sync_runs is the bookkeeping trail: watch derives its "since last sync"
-- delta window from the newest row for a term.
CREATE TABLE IF NOT EXISTS sync_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    term        TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    new_records INTEGER,
    total_after INTEGER
);
CREATE INDEX IF NOT EXISTS idx_sync_runs_term ON sync_runs(term);
`
