ALTER TABLE activities ADD COLUMN device TEXT NOT NULL DEFAULT '';

CREATE TABLE laps (
    id               INTEGER PRIMARY KEY,
    activity_id      INTEGER NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    idx              INTEGER NOT NULL,
    start_time_ms    INTEGER,
    duration_seconds REAL    NOT NULL,
    distance_meters  REAL    NOT NULL,
    calories         INTEGER,
    avg_heart_rate   INTEGER,
    max_heart_rate   INTEGER,

    UNIQUE (activity_id, idx)
);
