package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/model"
)

func TestSaveActivities(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	altitude := 123.4
	distance := 987.6
	speed := 3.21
	heartRate := 150
	cadence := 82
	calories := 300
	avgHR := 148
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)

	activity := model.Activity{
		Source:          "Garmin TCX",
		ExternalID:      "2026-10-20T06:30:00Z",
		Sport:           "Running",
		Device:          "Garmin Forerunner",
		StartTime:       start,
		DistanceMeters:  5000,
		DurationSeconds: 1800,
		Laps: []model.Lap{
			{StartTime: start, DurationSeconds: 900, DistanceMeters: 2500, Calories: &calories, AvgHR: &avgHR},
			{DurationSeconds: 900, DistanceMeters: 2500}, // no start time, no optional data
		},
		Points: []model.Point{
			{
				Time:           start,
				Position:       &model.Position{Lat: 52.2297, Lon: 21.0122},
				AltitudeMeters: &altitude,
				DistanceMeters: &distance,
				SpeedMPS:       &speed,
				HR:             &heartRate,
				Cadence:        &cadence,
			},
			{
				Time: start.Add(time.Second),
			},
		},
	}

	inserted, err := st.SaveActivities(ctx, []model.Activity{activity})
	if err != nil {
		t.Fatalf("SaveActivities() error = %v", err)
	}
	if inserted != 1 {
		t.Fatalf("SaveActivities() inserted = %d, want 1", inserted)
	}

	// Importing the same activity again must not duplicate it.
	inserted, err = st.SaveActivities(ctx, []model.Activity{activity})
	if err != nil {
		t.Fatalf("second SaveActivities() error = %v", err)
	}
	if inserted != 0 {
		t.Fatalf("second SaveActivities() inserted = %d, want 0", inserted)
	}

	count, err := st.CountActivities(ctx)
	if err != nil {
		t.Fatalf("CountActivities() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountActivities() = %d, want 1", count)
	}

	// Reopen the database to make sure the data is really persisted and the
	// schema can be applied to an existing database.
	if err := st.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	st, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	activities, err := st.ListActivities(ctx)
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("ListActivities() returned %d activities, want 1", len(activities))
	}
	if got := activities[0]; got.Source != activity.Source ||
		got.ExternalID != activity.ExternalID ||
		got.Sport != activity.Sport ||
		got.Device != activity.Device ||
		!got.StartTime.Equal(start) {
		t.Fatalf("ListActivities()[0] = %+v, want %+v", got, activity)
	}

	got, err := st.Activity(ctx, activities[0].ID)
	if err != nil {
		t.Fatalf("Activity() error = %v", err)
	}
	if len(got.Points) != 2 {
		t.Fatalf("Activity() returned %d points, want 2", len(got.Points))
	}

	if len(got.Laps) != 2 {
		t.Fatalf("Activity() returned %d laps, want 2", len(got.Laps))
	}
	lap := got.Laps[0]
	if !lap.StartTime.Equal(start) || lap.DurationSeconds != 900 || lap.DistanceMeters != 2500 ||
		lap.Calories == nil || *lap.Calories != calories ||
		lap.AvgHR == nil || *lap.AvgHR != avgHR || lap.MaxHR != nil {
		t.Errorf("first lap = %+v", lap)
	}
	if lap := got.Laps[1]; !lap.StartTime.IsZero() || lap.Calories != nil || lap.AvgHR != nil || lap.MaxHR != nil {
		t.Errorf("second lap = %+v, want no start time and no optional data", lap)
	}

	first := got.Points[0]
	if first.Position == nil || *first.Position != *activity.Points[0].Position {
		t.Errorf("first point position = %+v, want %+v", first.Position, activity.Points[0].Position)
	}
	if first.AltitudeMeters == nil || *first.AltitudeMeters != altitude {
		t.Errorf("first point altitude = %v, want %v", first.AltitudeMeters, altitude)
	}
	if first.DistanceMeters == nil || *first.DistanceMeters != distance {
		t.Errorf("first point distance = %v, want %v", first.DistanceMeters, distance)
	}
	if first.SpeedMPS == nil || *first.SpeedMPS != speed {
		t.Errorf("first point speed = %v, want %v", first.SpeedMPS, speed)
	}
	if first.HR == nil || *first.HR != heartRate {
		t.Errorf("first point heart rate = %v, want %v", first.HR, heartRate)
	}
	if first.Cadence == nil || *first.Cadence != cadence {
		t.Errorf("first point cadence = %v, want %v", first.Cadence, cadence)
	}
	if !first.Time.Equal(activity.Points[0].Time) {
		t.Errorf("first point time = %v, want %v", first.Time, activity.Points[0].Time)
	}

	second := got.Points[1]
	if second.Position != nil || second.AltitudeMeters != nil || second.DistanceMeters != nil ||
		second.SpeedMPS != nil || second.HR != nil || second.Cadence != nil {
		t.Errorf("second point optional fields = %+v, want all nil", second)
	}
}

func TestActivityNotFound(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	_, err = st.Activity(context.Background(), 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Activity() error = %v, want ErrNotFound", err)
	}
}

func TestEmptyStore(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	count, err := st.CountActivities(ctx)
	if err != nil {
		t.Fatalf("CountActivities() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountActivities() = %d, want 0", count)
	}

	activities, err := st.ListActivities(ctx)
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if len(activities) != 0 {
		t.Errorf("ListActivities() returned %d activities, want 0", len(activities))
	}
}

func TestListActivitiesOrder(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	older := model.Activity{
		Source:          "test",
		ExternalID:      "older",
		Sport:           "Running",
		StartTime:       time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC),
		DistanceMeters:  1000,
		DurationSeconds: 300,
	}
	newer := model.Activity{
		Source:          "test",
		ExternalID:      "newer",
		Sport:           "Running",
		StartTime:       time.Date(2026, 10, 21, 6, 0, 0, 0, time.UTC),
		DistanceMeters:  2000,
		DurationSeconds: 600,
	}

	if _, err := st.SaveActivities(context.Background(), []model.Activity{older, newer}); err != nil {
		t.Fatalf("SaveActivities() error = %v", err)
	}

	activities, err := st.ListActivities(context.Background())
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("ListActivities() returned %d activities, want 2", len(activities))
	}
	if activities[0].ExternalID != "newer" || activities[1].ExternalID != "older" {
		t.Errorf("activities order = [%s, %s], want [newer, older]",
			activities[0].ExternalID, activities[1].ExternalID)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func tableCount(t *testing.T, st *Store, table string) int {
	t.Helper()

	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func sampleActivity(externalID string) model.Activity {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	return model.Activity{
		Source:          "test",
		ExternalID:      externalID,
		Sport:           "Running",
		StartTime:       start,
		DistanceMeters:  1000,
		DurationSeconds: 300,
		Laps:            []model.Lap{{StartTime: start, DurationSeconds: 300, DistanceMeters: 1000}},
		Points:          []model.Point{{Time: start}, {Time: start.Add(time.Second)}},
	}
}

func TestSaveActivitiesRejectsInvalidActivities(t *testing.T) {
	good := sampleActivity("good")

	noSource := sampleActivity("no-source")
	noSource.Source = ""

	noID := sampleActivity("")

	noStart := sampleActivity("no-start")
	noStart.StartTime = time.Time{}

	noPointTime := sampleActivity("no-point-time")
	noPointTime.Points = append(noPointTime.Points, model.Point{})

	for name, bad := range map[string]model.Activity{
		"empty source":       noSource,
		"empty external id":  noID,
		"missing start time": noStart,
		"point without time": noPointTime,
	} {
		t.Run(name, func(t *testing.T) {
			st := openTestStore(t)

			// A valid activity in the same batch must not be saved either.
			if _, err := st.SaveActivities(context.Background(), []model.Activity{good, bad}); err == nil {
				t.Fatal("SaveActivities() error = nil, want an error")
			}
			if n := tableCount(t, st, "activities"); n != 0 {
				t.Errorf("activities stored = %d, want 0", n)
			}
		})
	}
}

func TestSaveActivitiesIsAtomic(t *testing.T) {
	st := openTestStore(t)

	// Break trackpoint inserts so the import fails after the activity row was
	// already written inside the transaction.
	if _, err := st.db.Exec(`DROP TABLE trackpoints`); err != nil {
		t.Fatalf("drop trackpoints: %v", err)
	}

	_, err := st.SaveActivities(context.Background(), []model.Activity{sampleActivity("a"), sampleActivity("b")})
	if err == nil {
		t.Fatal("SaveActivities() error = nil, want an error")
	}
	if n := tableCount(t, st, "activities"); n != 0 {
		t.Errorf("activities stored = %d, want 0 after a failed import", n)
	}
	if n := tableCount(t, st, "laps"); n != 0 {
		t.Errorf("laps stored = %d, want 0 after a failed import", n)
	}
}

func TestSaveActivitiesDuplicateWithinBatch(t *testing.T) {
	st := openTestStore(t)

	inserted, err := st.SaveActivities(context.Background(),
		[]model.Activity{sampleActivity("same"), sampleActivity("same")})
	if err != nil {
		t.Fatalf("SaveActivities() error = %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted = %d, want 1", inserted)
	}
	if n := tableCount(t, st, "trackpoints"); n != 2 {
		t.Errorf("trackpoints stored = %d, want 2 (the duplicate must not add more)", n)
	}
}

func TestDeletingActivityCascades(t *testing.T) {
	st := openTestStore(t)

	if _, err := st.SaveActivities(context.Background(), []model.Activity{sampleActivity("a")}); err != nil {
		t.Fatalf("SaveActivities() error = %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM activities`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := tableCount(t, st, "trackpoints") + tableCount(t, st, "laps"); n != 0 {
		t.Errorf("%d orphaned rows left after deleting the activity", n)
	}
}

func TestOpenAwkwardPaths(t *testing.T) {
	for _, name := range []string{"we?ird#na%me.db", "with space.db"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)

			st, err := Open(path)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if _, err := st.SaveActivities(context.Background(), []model.Activity{sampleActivity("a")}); err != nil {
				t.Fatalf("SaveActivities() error = %v", err)
			}
			if err := st.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			if _, err := os.Stat(path); err != nil {
				t.Fatalf("database not created at the requested path: %v", err)
			}
		})
	}
}

func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())

	st, err := Open("relative.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	st.Close()

	if _, err := os.Stat("relative.db"); err != nil {
		t.Fatalf("database not created in the working directory: %v", err)
	}
}

func TestOpenErrors(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("Open(\"\") error = nil, want an error")
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("Open(directory) error = nil, want an error")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing", "dir", "x.db")); err == nil {
		t.Error("Open(path in missing directory) error = nil, want an error")
	}
}

func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()

	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func TestMigrateFreshDatabase(t *testing.T) {
	st := openTestStore(t)

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaVersion(t, st.db); got != len(entries) {
		t.Errorf("user_version = %d, want %d", got, len(entries))
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 3; i++ {
		st, err := Open(path)
		if err != nil {
			t.Fatalf("Open() #%d error = %v", i, err)
		}
		st.Close()
	}
}

// A database created before migrations existed has the first schema but
// user_version 0, with real data in it. Opening it must keep the data and
// bring it up to date.
func TestMigrateAdoptsPreMigrationDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	first, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := dsn(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", conn)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		string(first),
		`INSERT INTO activities (source, external_id, sport, start_time_ms, distance_meters, duration_seconds)
		 VALUES ('Garmin TCX', 'old', 'Running', 1000, 5000, 1800)`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("seed old database: %v", err)
		}
	}
	if got := schemaVersion(t, raw); got != 0 {
		t.Fatalf("seeded user_version = %d, want 0", got)
	}
	raw.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	activities, err := st.ListActivities(context.Background())
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if len(activities) != 1 || activities[0].ExternalID != "old" || activities[0].Device != "" {
		t.Fatalf("activities after migration = %+v", activities)
	}

	// The new columns and tables work on the migrated database.
	if _, err := st.SaveActivities(context.Background(), []model.Activity{sampleActivity("new")}); err != nil {
		t.Fatalf("SaveActivities() error = %v", err)
	}
}

func TestMigrateRejectsNewerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	_, err = Open(path)
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("Open() error = %v, want a 'newer' schema error", err)
	}
}

func TestFailedMigrationLeavesVersionUntouched(t *testing.T) {
	st := openTestStore(t)
	before := schemaVersion(t, st.db)

	err := applyMigration(context.Background(), st.db, before+1, `CREATE TABLE ok_table (x); THIS IS NOT SQL;`)
	if err == nil {
		t.Fatal("applyMigration() error = nil, want an error")
	}
	if got := schemaVersion(t, st.db); got != before {
		t.Errorf("user_version = %d, want %d after a failed migration", got, before)
	}
	if _, err := st.db.Exec(`SELECT 1 FROM ok_table`); err == nil {
		t.Error("ok_table exists: failed migration was not rolled back")
	}
}
