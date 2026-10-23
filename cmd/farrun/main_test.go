package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/store"
)

const runTCX = `<?xml version="1.0" encoding="UTF-8"?>
<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="Running">
      <Id>2026-10-20T06:30:00Z</Id>
      <Lap StartTime="2026-10-20T06:30:00Z">
        <TotalTimeSeconds>60</TotalTimeSeconds>
        <DistanceMeters>1000</DistanceMeters>
        <Track>
          <Trackpoint>
            <Time>2026-10-20T06:30:00Z</Time>
            <Position>
              <LatitudeDegrees>52.2</LatitudeDegrees>
              <LongitudeDegrees>21.0</LongitudeDegrees>
            </Position>
            <AltitudeMeters>100</AltitudeMeters>
            <DistanceMeters>0</DistanceMeters>
            <HeartRateBpm><Value>120</Value></HeartRateBpm>
            <Extensions>
              <TPX>
                <Speed>3.0</Speed>
                <RunCadence>80</RunCadence>
              </TPX>
            </Extensions>
          </Trackpoint>
          <Trackpoint>
            <Time>2026-10-20T06:30:10Z</Time>
            <DistanceMeters>50</DistanceMeters>
            <HeartRateBpm><Value>125</Value></HeartRateBpm>
            <Extensions>
              <TPX>
                <Speed>3.1</Speed>
                <RunCadence>81</RunCadence>
              </TPX>
            </Extensions>
          </Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join("activities", "run.tcx"), runTCX)
	dbPath := filepath.Join(dir, "test.db")

	var out bytes.Buffer
	if err := run([]string{"import", "--db", dbPath}, &out, io.Discard); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	for _, want := range []string{
		"Imported 1 new activities (1 in database)",
		"Running 2026-10-20: 1.00km, 1:00m",
		"  number of points: 2",
		"  #0 08:30:00 | gps: 52.20000, 21.00000 | wys: 100.0 | hr: 120 | kad: 80",
		"  #1 08:30:10 | gps: No gps | wys: - | hr: 125 | kad: 81",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	activities, err := st.ListActivities(ctx)
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("ListActivities() returned %d activities, want 1", len(activities))
	}

	activity, err := st.Activity(ctx, activities[0].ID)
	if err != nil {
		t.Fatalf("Activity() error = %v", err)
	}
	if len(activity.Points) != 2 {
		t.Fatalf("Activity() returned %d points, want 2", len(activity.Points))
	}
	if activity.Points[0].DistanceMeters == nil || *activity.Points[0].DistanceMeters != 0 {
		t.Errorf("first point distance = %v, want 0", activity.Points[0].DistanceMeters)
	}
	if activity.Points[0].SpeedMPS == nil || *activity.Points[0].SpeedMPS != 3.0 {
		t.Errorf("first point speed = %v, want 3.0", activity.Points[0].SpeedMPS)
	}

	out.Reset()
	if err := run([]string{"import", "--db", dbPath}, &out, io.Discard); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if !strings.Contains(out.String(), "Imported 0 new activities (1 in database)") {
		t.Errorf("second run output = %q", out.String())
	}
}

func TestRunSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join("activities", "run.tcx"), runTCX)
	writeFile(t, filepath.Join("activities", "bad.tcx"), "not xml")

	var out, errOut bytes.Buffer
	if err := run([]string{"import", "--db", filepath.Join(dir, "test.db")}, &out, &errOut); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(out.String(), "Imported 1 new activities (1 in database, 1 files skipped)") {
		t.Errorf("output = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "bad.tcx") {
		t.Errorf("stderr = %q, want a warning about bad.tcx", errOut.String())
	}
}

func TestRunAnon(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join("anonActivities", "run.tcx"), runTCX)

	var out bytes.Buffer
	if err := run([]string{"import", "--anon", "--db", filepath.Join(dir, "test.db")}, &out, io.Discard); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(out.String(), "Imported 1 new activities") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunAnonUsesSeparateDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join("anonActivities", "run.tcx"), runTCX)
	writeFile(t, filepath.Join("activities", "run.tcx"), runTCX)

	if err := run([]string{"import", "--anon"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("anon run() error = %v", err)
	}
	if _, err := os.Stat(defaultAnonDB); err != nil {
		t.Errorf("anon database not created: %v", err)
	}
	if _, err := os.Stat(defaultDB); err == nil {
		t.Errorf("%s was created by an --anon run", defaultDB)
	}

	if err := run([]string{"import"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("real run() error = %v", err)
	}
	if _, err := os.Stat(defaultDB); err != nil {
		t.Errorf("%s not created: %v", defaultDB, err)
	}
}

func TestRunErrors(t *testing.T) {
	t.Run("missing activities directory", func(t *testing.T) {
		t.Chdir(t.TempDir())

		dbPath := filepath.Join(t.TempDir(), "test.db")

		err := run([]string{"import", "--db", dbPath}, &bytes.Buffer{}, io.Discard)
		if err == nil {
			t.Fatal("run() error = nil, want an error")
		}
		if _, statErr := os.Stat(dbPath); statErr == nil {
			t.Error("database was created even though the activities directory is missing")
		}
	})

	t.Run("invalid time zone", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeFile(t, filepath.Join("activities", "run.tcx"), runTCX)
		dbPath := filepath.Join(dir, "test.db")

		err := run([]string{"import", "--db", dbPath, "--tz", "Mars/Olympus"}, &bytes.Buffer{}, io.Discard)
		if err == nil {
			t.Fatal("run() error = nil, want an error")
		}
		if _, statErr := os.Stat(dbPath); statErr == nil {
			t.Error("database was created even though the time zone was invalid")
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		if err := run([]string{"import", "--nope"}, &bytes.Buffer{}, io.Discard); err == nil {
			t.Fatal("run() error = nil, want an error")
		}
	})

	t.Run("invalid database path", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeFile(t, filepath.Join("activities", "run.tcx"), runTCX)

		err := run([]string{"import", "--db", dir}, &bytes.Buffer{}, io.Discard)
		if err == nil {
			t.Fatal("run() error = nil, want an error")
		}
	})
}

func TestRunCommands(t *testing.T) {
	var out, errOut bytes.Buffer

	if err := run(nil, &out, &errOut); err == nil {
		t.Error("run() without a command: error = nil, want an error")
	}
	if !strings.Contains(errOut.String(), "usage: farrun") {
		t.Errorf("stderr = %q, want the usage text", errOut.String())
	}

	if err := run([]string{"nope"}, io.Discard, io.Discard); err == nil ||
		!strings.Contains(err.Error(), `unknown command "nope"`) {
		t.Errorf("run(nope) error = %v, want an unknown command error", err)
	}

	out.Reset()
	if err := run([]string{"help"}, &out, io.Discard); err != nil {
		t.Fatalf("run(help) error = %v", err)
	}
	if !strings.Contains(out.String(), "import") {
		t.Errorf("help output = %q, want it to list the import command", out.String())
	}
}

func TestDBPath(t *testing.T) {
	for _, tc := range []struct {
		flag string
		anon bool
		want string
	}{
		{"", false, defaultDB},
		{"", true, defaultAnonDB},
		{"custom.db", false, "custom.db"},
		{"custom.db", true, "custom.db"},
	} {
		if got := dbPath(tc.flag, tc.anon); got != tc.want {
			t.Errorf("dbPath(%q, %v) = %q, want %q", tc.flag, tc.anon, got, tc.want)
		}
	}
}

func TestPrintActivity(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	altitude := 100.0
	heartRate := 150
	cadence := 80

	points := make([]model.Point, 7)
	for i := range points {
		points[i] = model.Point{Time: start.Add(time.Duration(i) * time.Second)}
	}
	points[0].Position = &model.Position{Lat: 52.2297, Lon: 21.0122}
	points[0].AltitudeMeters = &altitude
	points[0].HR = &heartRate
	points[0].Cadence = &cadence

	activity := model.Activity{
		Sport:           "Running",
		StartTime:       start,
		DistanceMeters:  5000,
		DurationSeconds: 1800,
		Points:          points,
	}

	var out bytes.Buffer
	printActivity(&out, activity, time.UTC)
	got := out.String()

	for _, want := range []string{
		"Running 2026-10-20: 5.00km, 30:00m\n",
		"  number of points: 7\n",
		"  #0 06:30:00 | gps: 52.22970, 21.01220 | wys: 100.0 | hr: 150 | kad: 80\n",
		"  #1 06:30:01 | gps: No gps | wys: - | hr: - | kad: -\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "  #"); count != 5 {
		t.Errorf("output has %d point lines, want 5:\n%s", count, got)
	}
}
