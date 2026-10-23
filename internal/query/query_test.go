package query

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

var warsaw = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		panic(err)
	}
	return loc
}()

// The runs every test below filters, as the list shows them:
//
//	easy5k      2026-10-21 07:30  Running  5.00 km  25:00    5:00 /km
//	newyear10k  2026-01-01 00:30  Running  10.00 km 1:02:05  6:13 /km (23:30 UTC the evening before)
//	ride        2026-03-09 11:00  Cycling  25.00 km 50:00    2:00 /km
//	nodistance  2026-06-01 08:00  Running  0.00 km  1:00     no pace
//	almost5k    2024-05-05 08:00  Running  5.00 km  24:40    4:56 /km (really 4996 m)
var runs = map[string]Subject{
	"easy5k":     {Start: time.Date(2026, 10, 21, 5, 30, 0, 0, time.UTC), Sport: "Running", Device: "Forerunner 745", DistanceMeters: 5000, DurationSeconds: 1500},
	"newyear10k": {Start: time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC), Sport: "Running", Device: "Forerunner 745", DistanceMeters: 10000, DurationSeconds: 3725},
	"ride":       {Start: time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC), Sport: "Cycling", Device: "Edge 530", DistanceMeters: 25000, DurationSeconds: 3000},
	"nodistance": {Start: time.Date(2026, 6, 1, 6, 0, 0, 0, time.UTC), Sport: "Running", Device: "Forerunner 745", DistanceMeters: 0, DurationSeconds: 60},
	"almost5k":   {Start: time.Date(2024, 5, 5, 6, 0, 0, 0, time.UTC), Sport: "Running", Device: "Forerunner 745", DistanceMeters: 4996, DurationSeconds: 1480},
}

func matching(t *testing.T, input string) []string {
	t.Helper()

	q, err := Parse(input, warsaw)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", input, err)
	}
	var got []string
	for name, s := range runs {
		if q.Match(s) {
			got = append(got, name)
		}
	}
	sort.Strings(got)
	return got
}

func TestMatch(t *testing.T) {
	all := []string{"almost5k", "easy5k", "newyear10k", "nodistance", "ride"}
	running := []string{"almost5k", "easy5k", "newyear10k", "nodistance"}

	for _, tc := range []struct {
		query string
		want  []string
	}{
		// ---- nothing, or bare words (date text, sport and device)
		{"", all},
		{"   ", all},
		{"2026", []string{"easy5k", "newyear10k", "nodistance", "ride"}}, // newyear10k is 2026 in Warsaw
		{"2026-10", []string{"easy5k"}},
		{"2025", nil}, // 23:30 UTC on Dec 31 is not 2025 in Warsaw
		{"07:30", []string{"easy5k"}},
		{"running", running},
		{"RUNNING", running},
		{"forerunner", running},
		{"edge", []string{"ride"}},
		{"nothing-like-this", nil},
		{`"edge 530"`, []string{"ride"}},
		{`"or"`, running}, // quoted: the word "or" (inside "Forerunner"), not the keyword
		{`"pace"`, nil},   // quoted: the word "pace", not the field
		{"(running)", running},

		// ---- distance
		{"distance > 5km", []string{"newyear10k", "ride"}},
		{"distance >= 5km", []string{"almost5k", "easy5k", "newyear10k", "ride"}}, // almost5k is shown as 5.00
		{"distance = 5km", []string{"almost5k", "easy5k"}},
		{"distance == 5", []string{"almost5k", "easy5k"}},
		{"distance != 5km", []string{"newyear10k", "nodistance", "ride"}},
		{"distance <> 5km", []string{"newyear10k", "nodistance", "ride"}},
		{"distance < 5km", []string{"nodistance"}},
		{"distance <= 5km", []string{"almost5k", "easy5k", "nodistance"}},
		{"distance>5km", []string{"newyear10k", "ride"}}, // no spaces
		{"distance > 5 km", []string{"newyear10k", "ride"}},
		{"distance > 5000m", []string{"newyear10k", "ride"}},
		{"distance > 5000 m", []string{"newyear10k", "ride"}},
		{"distance > 3.1mi", []string{"almost5k", "easy5k", "newyear10k", "ride"}}, // 3.1 mi is 4.99 km
		{"distance > 3.2mi", []string{"newyear10k", "ride"}},                       // 3.2 mi is 5.15 km
		{"DISTANCE > 5KM", []string{"newyear10k", "ride"}},
		{"dist > 9", []string{"newyear10k", "ride"}},

		// ---- time
		{"time > 30:00", []string{"newyear10k", "ride"}},
		{"time >= 25:00", []string{"easy5k", "newyear10k", "ride"}},
		{"time < 25:00", []string{"almost5k", "nodistance"}},
		{"time > 1:00:00", []string{"newyear10k"}},
		{"time > 1h", []string{"newyear10k"}},
		{"time > 1h30m", nil},
		{"time >= 45m", []string{"newyear10k", "ride"}},
		{"time = 90s", nil},
		{"time <= 90s", []string{"nodistance"}},
		{"time > 30", []string{"newyear10k", "ride"}}, // a bare number is minutes
		{"duration > 1h", []string{"newyear10k"}},

		// ---- pace: less means faster
		{"pace < 6:20", []string{"almost5k", "easy5k", "newyear10k", "ride"}},
		{"pace < 6:20/km", []string{"almost5k", "easy5k", "newyear10k", "ride"}},
		{"pace < 6:20 /km", []string{"almost5k", "easy5k", "newyear10k", "ride"}},
		{"pace < 6:20 min/km", []string{"almost5k", "easy5k", "newyear10k", "ride"}},
		{"pace < 6:00", []string{"almost5k", "easy5k", "ride"}},
		{"pace > 6:00", []string{"newyear10k"}},
		{"pace = 5:00", []string{"easy5k"}},
		{"pace = 6:13", []string{"newyear10k"}}, // 372.5 s/km is shown as 6:13
		{"pace <= 5", []string{"almost5k", "easy5k", "ride"}},
		{"pace < 7", []string{"almost5k", "easy5k", "newyear10k", "ride"}},
		// A run without distance has no pace, so no pace comparison matches it, not even "!=".
		{"pace != 5:00", []string{"almost5k", "newyear10k", "ride"}},
		{"pace > 0:01", []string{"almost5k", "easy5k", "newyear10k", "ride"}},

		// ---- date: a whole period, in the time zone the page shows
		{"date = 2026", []string{"easy5k", "newyear10k", "nodistance", "ride"}},
		{"date = 2025", nil},
		{"date = 2026-01", []string{"newyear10k"}},
		{"date = 2026-01-01", []string{"newyear10k"}},
		{"date = 2026-1-1", []string{"newyear10k"}}, // no leading zeros needed
		{"date = 2026-10-21", []string{"easy5k"}},
		{"date != 2026", []string{"almost5k"}},
		{"date >= 2026-03", []string{"easy5k", "nodistance", "ride"}}, // from the 1st of March
		{"date > 2026-03", []string{"easy5k", "nodistance"}},          // after the end of March
		{"date < 2026-03", []string{"almost5k", "newyear10k"}},        // before the 1st of March
		{"date <= 2026-03", []string{"almost5k", "newyear10k", "ride"}},
		{"date > 2026", nil},
		{"date >= 2026", []string{"easy5k", "newyear10k", "nodistance", "ride"}},
		{"date < 2026", []string{"almost5k"}},

		// ---- sport and device
		{"sport = running", running},
		{"sport = RUNNING", running},
		{`sport = "cycling"`, []string{"ride"}},
		{"sport != running", []string{"ride"}},
		{"sport like run", running},
		{"sport ~ cyc", []string{"ride"}},
		{"sport = run", nil}, // = is exact
		{`device = "edge 530"`, []string{"ride"}},
		{"device like fore", running},
		{"device != forerunner", all}, // exact: "Forerunner 745" is not "forerunner"

		// ---- between: both ends included
		{"distance between 5km and 10km", []string{"almost5k", "easy5k", "newyear10k"}},
		{"distance between 10km and 5km", []string{"almost5k", "easy5k", "newyear10k"}}, // backwards is forgiven
		{"pace between 5:00 and 6:20", []string{"easy5k", "newyear10k"}},
		{"time between 25:00 and 1:02:05", []string{"easy5k", "newyear10k", "ride"}},
		{"date between 2026-01 and 2026-03", []string{"newyear10k", "ride"}},
		{"date between 2026-03 and 2026-01", []string{"newyear10k", "ride"}},
		{"date between 2024 and 2025", []string{"almost5k"}},
		{"date between 2026-10-21 and 2026-10-21", []string{"easy5k"}},
		{"DISTANCE BETWEEN 5 AND 10", []string{"almost5k", "easy5k", "newyear10k"}},

		// ---- combining
		{"distance > 5km and pace < 6:00", []string{"ride"}},
		{"distance > 5km pace < 6:00", []string{"ride"}}, // side by side means and
		{"2026 distance >= 10", []string{"newyear10k", "ride"}},
		{"distance > 20km or pace > 6:00", []string{"newyear10k", "ride"}},
		{"sport = running and distance >= 5km", []string{"almost5k", "easy5k", "newyear10k"}},
		{"not sport = running", []string{"ride"}},
		{"not running", []string{"ride"}},
		{"not not running", running},
		{"not (distance > 5km)", []string{"almost5k", "easy5k", "nodistance"}},
		{"((distance > 5km))", []string{"newyear10k", "ride"}},
		// and binds tighter than or:
		{"sport = cycling or distance >= 10 and pace > 6:00", []string{"newyear10k", "ride"}},
		{"(sport = cycling or distance >= 10) and pace > 6:00", []string{"newyear10k"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			if got := matching(t, tc.query); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%q matches %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}
