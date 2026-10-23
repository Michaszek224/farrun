package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testTCX = `<?xml version="1.0" encoding="UTF-8"?>
<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="Running">
      <Id>2023-09-16T11:12:43.000+02:00</Id>
      <Lap StartTime="2023-09-16T11:12:43.000+02:00">
        <TotalTimeSeconds>100</TotalTimeSeconds>
        <DistanceMeters>500</DistanceMeters>
        <Calories>10</Calories>
        <AverageHeartRateBpm><Value>140</Value></AverageHeartRateBpm>
        <MaximumHeartRateBpm><Value>160</Value></MaximumHeartRateBpm>
        <Track>
          <Trackpoint>
            <Time>2023-09-16T11:12:43.000+02:00</Time>
            <Position>
              <LatitudeDegrees>52.1</LatitudeDegrees>
              <LongitudeDegrees>21.2</LongitudeDegrees>
            </Position>
            <AltitudeMeters>100.5</AltitudeMeters>
            <DistanceMeters>0.0</DistanceMeters>
            <HeartRateBpm><Value>100</Value></HeartRateBpm>
            <Extensions>
              <TPX>
                <Speed>2.5</Speed>
                <RunCadence>80</RunCadence>
              </TPX>
            </Extensions>
          </Trackpoint>
          <Trackpoint>
            <Time>2023-09-16T11:12:53.000+02:00</Time>
            <DistanceMeters>25.0</DistanceMeters>
            <HeartRateBpm><Value>105</Value></HeartRateBpm>
            <Extensions>
              <TPX>
                <Speed>2.6</Speed>
                <RunCadence>81</RunCadence>
              </TPX>
            </Extensions>
          </Trackpoint>
        </Track>
      </Lap>
      <Lap StartTime="2023-09-16T11:14:33.000+02:00">
        <TotalTimeSeconds>50</TotalTimeSeconds>
        <DistanceMeters>250</DistanceMeters>
        <Track>
          <Trackpoint>
            <Time>2023-09-16T11:14:33.000+02:00</Time>
            <Position>
              <LatitudeDegrees>52.2</LatitudeDegrees>
              <LongitudeDegrees>21.3</LongitudeDegrees>
            </Position>
            <AltitudeMeters>101.0</AltitudeMeters>
            <DistanceMeters>500.0</DistanceMeters>
            <HeartRateBpm><Value>110</Value></HeartRateBpm>
            <Extensions>
              <TPX>
                <Speed>2.7</Speed>
                <RunCadence>82</RunCadence>
              </TPX>
            </Extensions>
          </Trackpoint>
        </Track>
      </Lap>
      <Creator><Name>Garmin</Name></Creator>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func parseTestTCX(t *testing.T) *TCX {
	t.Helper()

	path := writeTestFile(t, t.TempDir(), "activity.tcx", testTCX)
	tcx, err := parseTCX(path)
	if err != nil {
		t.Fatalf("parseTCX() error = %v", err)
	}
	return tcx
}

func TestParseTCX(t *testing.T) {
	tcx := parseTestTCX(t)

	if len(tcx.Activities) != 1 {
		t.Fatalf("len(Activities) = %d, want 1", len(tcx.Activities))
	}

	activity := tcx.Activities[0]
	wantID := time.Date(2023, 9, 16, 9, 12, 43, 0, time.UTC)
	if activity.Sport != "Running" {
		t.Errorf("Sport = %q, want Running", activity.Sport)
	}
	if !activity.ID.Equal(wantID) {
		t.Errorf("ID = %v, want %v", activity.ID, wantID)
	}
	if activity.Device != "Garmin" {
		t.Errorf("Device = %q, want Garmin", activity.Device)
	}
	if len(activity.Laps) != 2 {
		t.Fatalf("len(Laps) = %d, want 2", len(activity.Laps))
	}

	lap := activity.Laps[0]
	if lap.TotalTimeSeconds != 100 {
		t.Errorf("TotalTimeSeconds = %v, want 100", lap.TotalTimeSeconds)
	}
	if lap.DistanceMeters != 500 {
		t.Errorf("DistanceMeters = %v, want 500", lap.DistanceMeters)
	}
	if lap.Calories == nil || *lap.Calories != 10 {
		t.Errorf("Calories = %v, want 10", lap.Calories)
	}
	if lap.AvgHR == nil || *lap.AvgHR != 140 {
		t.Errorf("AvgHR = %v, want 140", lap.AvgHR)
	}
	if lap.MaxHR == nil || *lap.MaxHR != 160 {
		t.Errorf("MaxHR = %v, want 160", lap.MaxHR)
	}
	if got := activity.Laps[1]; got.Calories != nil || got.AvgHR != nil || got.MaxHR != nil {
		t.Errorf("second lap optional fields = %+v, want all nil", got)
	}
	if len(lap.Trackpoints) != 2 {
		t.Fatalf("len(Trackpoints) = %d, want 2", len(lap.Trackpoints))
	}

	point := lap.Trackpoints[0]
	if point.Position == nil || point.Position.Lat != 52.1 || point.Position.Lon != 21.2 {
		t.Errorf("Position = %+v, want 52.1/21.2", point.Position)
	}
	if point.AltitudeMeters == nil || *point.AltitudeMeters != 100.5 {
		t.Errorf("AltitudeMeters = %v, want 100.5", point.AltitudeMeters)
	}
	if point.DistanceMeters == nil || *point.DistanceMeters != 0 {
		t.Errorf("DistanceMeters = %v, want 0", point.DistanceMeters)
	}
	if point.HR == nil || *point.HR != 100 {
		t.Errorf("HR = %v, want 100", point.HR)
	}
	if point.Speed == nil || *point.Speed != 2.5 {
		t.Errorf("Speed = %v, want 2.5", point.Speed)
	}
	if point.Cadence == nil || *point.Cadence != 80 {
		t.Errorf("Cadence = %v, want 80", point.Cadence)
	}
	if !point.Time.Equal(wantID) {
		t.Errorf("Time = %v, want %v", point.Time, wantID)
	}
}

func TestParseTCXErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := parseTCX(filepath.Join(t.TempDir(), "missing.tcx"))
		if err == nil {
			t.Fatal("parseTCX() error = nil, want an error")
		}
	})

	t.Run("invalid xml", func(t *testing.T) {
		path := writeTestFile(t, t.TempDir(), "bad.tcx", "<TrainingCenterDatabase>")
		_, err := parseTCX(path)
		if err == nil {
			t.Fatal("parseTCX() error = nil, want an error")
		}
	})
}

func TestParseAllFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.tcx", testTCX)
	writeTestFile(t, dir, "b.TCX", testTCX)
	writeTestFile(t, dir, "notes.txt", "not a tcx file")
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTestFile(t, dir, filepath.Join("nested", "c.tcx"), testTCX)

	tcxList, skipped, err := ParseAllFiles(dir)
	if err != nil {
		t.Fatalf("ParseAllFiles() error = %v", err)
	}
	if len(tcxList) != 2 {
		t.Fatalf("ParseAllFiles() returned %d files, want 2", len(tcxList))
	}
	if len(skipped) != 0 {
		t.Fatalf("ParseAllFiles() skipped %v, want none", skipped)
	}
}

func TestParseAllFilesMissingDirectory(t *testing.T) {
	_, _, err := ParseAllFiles(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("ParseAllFiles() error = nil, want an error")
	}
}

func TestParseAllFilesSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "good.tcx", testTCX)
	writeTestFile(t, dir, "bad.tcx", "not xml")

	tcxList, skipped, err := ParseAllFiles(dir)
	if err != nil {
		t.Fatalf("ParseAllFiles() error = %v", err)
	}
	if len(tcxList) != 1 {
		t.Fatalf("ParseAllFiles() returned %d files, want the 1 good one", len(tcxList))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Error(), "bad.tcx") {
		t.Fatalf("ParseAllFiles() skipped = %v, want only bad.tcx", skipped)
	}
}

func TestToActivities(t *testing.T) {
	activities := parseTestTCX(t).ToActivities()
	if len(activities) != 1 {
		t.Fatalf("ToActivities() returned %d activities, want 1", len(activities))
	}

	activity := activities[0]
	wantStart := time.Date(2023, 9, 16, 9, 12, 43, 0, time.UTC)
	if activity.Source != "Garmin TCX" {
		t.Errorf("Source = %q, want Garmin TCX", activity.Source)
	}
	if activity.ExternalID != "2023-09-16T09:12:43Z" {
		t.Errorf("ExternalID = %q, want 2023-09-16T09:12:43Z", activity.ExternalID)
	}
	if activity.Sport != "Running" {
		t.Errorf("Sport = %q, want Running", activity.Sport)
	}
	if !activity.StartTime.Equal(wantStart) {
		t.Errorf("StartTime = %v, want %v", activity.StartTime, wantStart)
	}
	if activity.DistanceMeters != 750 {
		t.Errorf("DistanceMeters = %v, want 750", activity.DistanceMeters)
	}
	if activity.DurationSeconds != 150 {
		t.Errorf("DurationSeconds = %v, want 150", activity.DurationSeconds)
	}
	if activity.Device != "Garmin" {
		t.Errorf("Device = %q, want Garmin", activity.Device)
	}
	if len(activity.Laps) != 2 {
		t.Fatalf("len(Laps) = %d, want 2", len(activity.Laps))
	}
	if lap := activity.Laps[0]; !lap.StartTime.Equal(wantStart) ||
		lap.DurationSeconds != 100 || lap.DistanceMeters != 500 ||
		lap.Calories == nil || *lap.Calories != 10 {
		t.Errorf("first lap = %+v", lap)
	}
	if len(activity.Points) != 3 {
		t.Fatalf("len(Points) = %d, want 3", len(activity.Points))
	}

	first := activity.Points[0]
	if first.Position == nil || first.Position.Lat != 52.1 || first.Position.Lon != 21.2 {
		t.Errorf("first Position = %+v, want 52.1/21.2", first.Position)
	}
	if first.AltitudeMeters == nil || *first.AltitudeMeters != 100.5 {
		t.Errorf("first AltitudeMeters = %v, want 100.5", first.AltitudeMeters)
	}
	if first.DistanceMeters == nil || *first.DistanceMeters != 0 {
		t.Errorf("first DistanceMeters = %v, want 0", first.DistanceMeters)
	}
	if first.SpeedMPS == nil || *first.SpeedMPS != 2.5 {
		t.Errorf("first SpeedMPS = %v, want 2.5", first.SpeedMPS)
	}
	if first.HR == nil || *first.HR != 100 {
		t.Errorf("first HR = %v, want 100", first.HR)
	}
	if first.Cadence == nil || *first.Cadence != 80 {
		t.Errorf("first Cadence = %v, want 80", first.Cadence)
	}
	if !first.Time.Equal(wantStart) || first.Time.Location() != time.UTC {
		t.Errorf("first Time = %v, want %v in UTC", first.Time, wantStart)
	}

	second := activity.Points[1]
	if second.Position != nil {
		t.Errorf("second Position = %+v, want nil", second.Position)
	}
	if second.AltitudeMeters != nil {
		t.Errorf("second AltitudeMeters = %v, want nil", second.AltitudeMeters)
	}
	if second.DistanceMeters == nil || *second.DistanceMeters != 25 {
		t.Errorf("second DistanceMeters = %v, want 25", second.DistanceMeters)
	}
	if second.SpeedMPS == nil || *second.SpeedMPS != 2.6 {
		t.Errorf("second SpeedMPS = %v, want 2.6", second.SpeedMPS)
	}
	if !second.Time.Equal(wantStart.Add(10 * time.Second)) {
		t.Errorf("second Time = %v, want %v", second.Time, wantStart.Add(10*time.Second))
	}
}

func TestToActivitiesEmpty(t *testing.T) {
	if got := (&TCX{}).ToActivities(); len(got) != 0 {
		t.Fatalf("ToActivities() returned %d activities, want 0", len(got))
	}
}

func TestTrackpointToPoint(t *testing.T) {
	zone := time.FixedZone("CEST", 2*60*60)
	localTime := time.Date(2023, 9, 16, 11, 12, 43, 0, zone)
	altitude := 100.5
	distance := 12.5
	speed := 3.1
	heartRate := 150
	cadence := 80

	point := Trackpoint{
		Time:           localTime,
		Position:       &Position{Lat: 52.1, Lon: 21.2},
		AltitudeMeters: &altitude,
		DistanceMeters: &distance,
		Speed:          &speed,
		HR:             &heartRate,
		Cadence:        &cadence,
	}.toPoint()

	if !point.Time.Equal(localTime) || point.Time.Location() != time.UTC {
		t.Errorf("Time = %v, want %v in UTC", point.Time, localTime)
	}
	if point.Position == nil || point.Position.Lat != 52.1 || point.Position.Lon != 21.2 {
		t.Errorf("Position = %+v, want 52.1/21.2", point.Position)
	}
	if point.AltitudeMeters == nil || *point.AltitudeMeters != altitude {
		t.Errorf("AltitudeMeters = %v, want %v", point.AltitudeMeters, altitude)
	}
	if point.DistanceMeters == nil || *point.DistanceMeters != distance {
		t.Errorf("DistanceMeters = %v, want %v", point.DistanceMeters, distance)
	}
	if point.SpeedMPS == nil || *point.SpeedMPS != speed {
		t.Errorf("SpeedMPS = %v, want %v", point.SpeedMPS, speed)
	}
	if point.HR == nil || *point.HR != heartRate {
		t.Errorf("HR = %v, want %v", point.HR, heartRate)
	}
	if point.Cadence == nil || *point.Cadence != cadence {
		t.Errorf("Cadence = %v, want %v", point.Cadence, cadence)
	}
}

func TestTrackpointToPointWithoutOptionalData(t *testing.T) {
	point := Trackpoint{Time: time.Date(2023, 9, 16, 9, 12, 43, 0, time.UTC)}.toPoint()

	if point.Position != nil || point.AltitudeMeters != nil || point.DistanceMeters != nil ||
		point.SpeedMPS != nil || point.HR != nil || point.Cadence != nil {
		t.Errorf("Point = %+v, want all optional fields nil", point)
	}
}

func TestToActivitiesDropsUnusableData(t *testing.T) {
	validTime := time.Date(2023, 9, 16, 9, 12, 43, 0, time.UTC)
	tcx := &TCX{Activities: []Activity{
		{Sport: "Running"}, // no Id: cannot be identified
		{
			Sport: "Running",
			ID:    validTime,
			Laps: []Lap{{
				Trackpoints: []Trackpoint{
					{}, // no time
					{Time: validTime},
				},
			}},
		},
	}}

	activities := tcx.ToActivities()
	if len(activities) != 1 {
		t.Fatalf("ToActivities() returned %d activities, want 1", len(activities))
	}
	if len(activities[0].Points) != 1 {
		t.Fatalf("len(Points) = %d, want 1", len(activities[0].Points))
	}
}
