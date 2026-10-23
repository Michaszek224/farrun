-- Timestamps are stored as Unix milliseconds in UTC.
-- IF NOT EXISTS lets this adopt databases created before migrations existed.

CREATE TABLE IF NOT EXISTS activities (
    id               INTEGER PRIMARY KEY,
    source           TEXT    NOT NULL,
    external_id      TEXT    NOT NULL,
    sport            TEXT    NOT NULL,
    start_time_ms    INTEGER NOT NULL,
    distance_meters  REAL    NOT NULL,
    duration_seconds REAL    NOT NULL,

    UNIQUE (source, external_id)
);

CREATE INDEX IF NOT EXISTS idx_activities_start_time
    ON activities (start_time_ms DESC);

CREATE TABLE IF NOT EXISTS trackpoints (
    id              INTEGER PRIMARY KEY,
    activity_id     INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    time_ms         INTEGER NOT NULL,
    latitude        REAL,
    longitude       REAL,
    altitude_meters REAL,
    distance_meters REAL,
    speed_mps       REAL,
    heart_rate      INTEGER,
    cadence         INTEGER
);

CREATE INDEX IF NOT EXISTS idx_trackpoints_activity_time
    ON trackpoints (activity_id, time_ms);
