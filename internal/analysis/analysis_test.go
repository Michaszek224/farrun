package analysis

import (
	"math"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/model"
)

func ptr[T any](v T) *T { return &v }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPace(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dist, du float64
		want     float64
		ok       bool
	}{
		{"5k in 25 minutes", 5000, 1500, 300, true},
		{"1 km in 6:30", 1000, 390, 390, true},
		{"zero distance", 0, 60, 0, false},
		{"zero duration", 1000, 0, 0, false},
		{"negative distance", -5, 60, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Pace(tc.dist, tc.du)
			if ok != tc.ok || !near(got, tc.want) {
				t.Errorf("Pace(%v, %v) = %v, %v; want %v, %v", tc.dist, tc.du, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAvgAndMaxHR(t *testing.T) {
	points := []model.Point{{HR: ptr(100)}, {}, {HR: ptr(150)}, {HR: ptr(140)}}

	avg, ok := AvgHR(points)
	if !ok || !near(avg, 130) {
		t.Errorf("AvgHR() = %v, %v; want 130, true (points without HR ignored)", avg, ok)
	}
	maxHR, ok := MaxHR(points)
	if !ok || maxHR != 150 {
		t.Errorf("MaxHR() = %v, %v; want 150, true", maxHR, ok)
	}

	if _, ok := AvgHR([]model.Point{{}, {}}); ok {
		t.Error("AvgHR() without any heart rate: ok = true, want false")
	}
	if _, ok := MaxHR(nil); ok {
		t.Error("MaxHR(nil): ok = true, want false")
	}
}

func TestElevationGain(t *testing.T) {
	alts := func(values ...float64) []model.Point {
		var pts []model.Point
		for _, v := range values {
			pts = append(pts, model.Point{AltitudeMeters: ptr(v)})
		}
		return pts
	}

	for _, tc := range []struct {
		name   string
		points []model.Point
		want   float64
	}{
		{"empty", nil, 0},
		{"flat", alts(10, 10, 10), 0},
		{"one climb", alts(10, 12, 15), 5},
		{"descent only", alts(15, 12, 10), 0},
		{"up, down, up", alts(10, 15, 11, 14), 5 + 3},
		{"jitter below threshold is ignored", alts(10, 10.4, 9.8, 10.3, 9.9, 10.2), 0},
		{"gain counts across small steps", alts(10, 11, 12, 13), 3},
		{"missing altitude skipped", []model.Point{{AltitudeMeters: ptr(10.0)}, {}, {AltitudeMeters: ptr(14.0)}}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ElevationGain(tc.points); !near(got, tc.want) {
				t.Errorf("ElevationGain() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSeriesEmpty(t *testing.T) {
	if got := Series(nil, 100); got != nil {
		t.Errorf("Series(nil) = %v, want nil", got)
	}
}

func TestSeriesWithoutDownsampling(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	points := []model.Point{
		{Time: start, DistanceMeters: ptr(0.0), SpeedMPS: ptr(0.0), HR: ptr(90)},
		{Time: start.Add(10 * time.Second), DistanceMeters: ptr(30.0), SpeedMPS: ptr(3.0), HR: ptr(120),
			AltitudeMeters: ptr(40.0), Cadence: ptr(160)},
	}

	got := Series(points, 100)
	if len(got) != 2 {
		t.Fatalf("len(Series) = %d, want 2", len(got))
	}

	if got[0].PaceSecPerKm != nil {
		t.Errorf("standing still: pace = %v, want nil", *got[0].PaceSecPerKm)
	}
	if got[0].AltitudeMeters != nil || got[0].Cadence != nil {
		t.Errorf("first point has no altitude/cadence, got %+v", got[0])
	}

	second := got[1]
	if !near(second.DistanceMeters, 30) || !near(second.ElapsedSeconds, 10) {
		t.Errorf("second position = %v m / %v s, want 30 m / 10 s", second.DistanceMeters, second.ElapsedSeconds)
	}
	if second.PaceSecPerKm == nil || !near(*second.PaceSecPerKm, 1000.0/3) {
		t.Errorf("second pace = %v, want %v", second.PaceSecPerKm, 1000.0/3)
	}
	if second.HR == nil || !near(*second.HR, 120) ||
		second.AltitudeMeters == nil || !near(*second.AltitudeMeters, 40) ||
		second.Cadence == nil || !near(*second.Cadence, 160) {
		t.Errorf("second point = %+v", second)
	}
}

func TestSeriesCarriesDistanceForward(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	points := []model.Point{
		{Time: start, DistanceMeters: ptr(10.0)},
		{Time: start.Add(time.Second)}, // no distance recorded
	}

	got := Series(points, 10)
	if !near(got[1].DistanceMeters, 10) {
		t.Errorf("distance without data = %v, want the previous 10", got[1].DistanceMeters)
	}
}

func TestSeriesDownsamples(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	var points []model.Point
	for i := 0; i < 1000; i++ {
		points = append(points, model.Point{
			Time:           start.Add(time.Duration(i) * time.Second),
			DistanceMeters: ptr(float64(i) * 3),
			SpeedMPS:       ptr(3.0),
			HR:             ptr(100 + i/10),
		})
	}

	for _, maxPoints := range []int{1, 7, 100, 333, 999, 1000, 5000} {
		got := Series(points, maxPoints)
		if len(got) == 0 || len(got) > maxPoints {
			t.Fatalf("maxPoints %d: got %d samples", maxPoints, len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i].DistanceMeters < got[i-1].DistanceMeters || got[i].ElapsedSeconds < got[i-1].ElapsedSeconds {
				t.Fatalf("maxPoints %d: sample %d goes backwards", maxPoints, i)
			}
		}
	}

	got := Series(points, 100)
	if len(got) != 100 {
		t.Fatalf("len = %d, want 100", len(got))
	}
	// Constant speed 3 m/s stays 3 m/s after averaging: pace 333.3 s/km.
	for i, s := range got {
		if s.PaceSecPerKm == nil || !near(*s.PaceSecPerKm, 1000.0/3) {
			t.Fatalf("sample %d pace = %v, want %v", i, s.PaceSecPerKm, 1000.0/3)
		}
	}
	// Heart rate rises over the run, so the last bucket is above the first.
	if *got[len(got)-1].HR <= *got[0].HR {
		t.Errorf("hr did not rise: first %v, last %v", *got[0].HR, *got[len(got)-1].HR)
	}
}

func TestSeriesInvalidMax(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	points := []model.Point{{Time: start}, {Time: start.Add(time.Second)}}

	if got := Series(points, 0); len(got) != 1 {
		t.Errorf("Series(maxPoints=0) returned %d samples, want 1", len(got))
	}
}

func TestSeriesPaceNeedsMovement(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)

	for _, tc := range []struct {
		name    string
		speed   float64
		wantNil bool
		want    float64
	}{
		{"standing still", 0, true, 0},
		{"drifting at 0.9 m/s", 0.9, true, 0},
		{"just moving, 1 m/s", 1.0, false, 1000},
		{"slow walk, 1.4 m/s", 1.4, false, 1000 / 1.4},
		{"running, 3 m/s", 3.0, false, 1000.0 / 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Series([]model.Point{{Time: start, SpeedMPS: ptr(tc.speed)}}, 10)
			if tc.wantNil {
				if got[0].PaceSecPerKm != nil {
					t.Errorf("pace = %v, want nil", *got[0].PaceSecPerKm)
				}
				return
			}
			if got[0].PaceSecPerKm == nil || !near(*got[0].PaceSecPerKm, tc.want) {
				t.Errorf("pace = %v, want %v", got[0].PaceSecPerKm, tc.want)
			}
		})
	}
}

func TestSeriesCadenceZeroMeansNoData(t *testing.T) {
	start := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	points := []model.Point{
		{Time: start, Cadence: ptr(0)},
		{Time: start.Add(time.Second), Cadence: ptr(80)},
		{Time: start.Add(2 * time.Second), Cadence: ptr(84)},
	}

	got := Series(points, 10)
	if got[0].Cadence != nil {
		t.Errorf("cadence 0 = %v, want nil", *got[0].Cadence)
	}
	if got[1].Cadence == nil || !near(*got[1].Cadence, 80) {
		t.Errorf("cadence 80 = %v", got[1].Cadence)
	}

	// Averaged into one bucket, the zero must not pull the mean down to 54.7.
	one := Series(points, 1)
	if one[0].Cadence == nil || !near(*one[0].Cadence, 82) {
		t.Errorf("bucket cadence = %v, want 82 (the zero is ignored)", one[0].Cadence)
	}
}
