package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/model"
)

func ptr[T any](v T) *T { return &v }

// detailedRun is a 2 km run with two laps and trackpoints every 10 seconds.
func detailedRun(externalID string, points int) model.Activity {
	start := time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC)
	a := sampleRun(externalID, start, 2000, 600)
	a.Laps = []model.Lap{
		{StartTime: start, DurationSeconds: 310, DistanceMeters: 1000, Calories: ptr(70), AvgHR: ptr(150), MaxHR: ptr(160)},
		{StartTime: start.Add(310 * time.Second), DurationSeconds: 290, DistanceMeters: 1000}, // no optional data
	}
	for i := 0; i < points; i++ {
		p := model.Point{
			Time:           start.Add(time.Duration(i) * 10 * time.Second),
			DistanceMeters: ptr(float64(i) * 33),
			SpeedMPS:       ptr(3.3),
			AltitudeMeters: ptr(40 + float64(i%10)*2),
		}
		if i%2 == 0 {
			p.HR = ptr(140 + i%20)
		}
		a.Points = append(a.Points, p)
	}
	return a
}

func TestDetail(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 60))

	res, body := get(t, h, "/activities/1")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	for _, want := range []string{
		"2026-10-20 07:30", // title in Warsaw time
		"Forerunner",       // device
		"2.00",             // distance
		"10:00",            // time
		"5:00",             // average pace: 600 s / 2 km = 300 s/km
		"/api/activities/1/series",
		`data-label="2026-10-20 · 2.00 km"`,
		"<h2>Laps</h2>",
		"<td class=\"num\">70</td>", // lap calories
		"/static/chart.umd.js",
		"/static/charts.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	// The second lap has no optional data: three dashes (avg HR, max HR, calories).
	if got := strings.Count(body, `<td class="num">–</td>`); got != 3 {
		t.Errorf("dash cells = %d, want 3 for the lap without heart rate and calories", got)
	}

	// The CSP forbids inline scripts, so none may be emitted.
	for _, tag := range strings.Split(body, "<script")[1:] {
		if !strings.HasPrefix(strings.TrimSpace(tag), "src=") {
			t.Errorf("inline script found: <script%.40s", tag)
		}
	}
}

func TestDetailWithoutOptionalData(t *testing.T) {
	a := sampleRun("bare", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 0, 0)

	res, body := get(t, newTestHandler(t, a), "/activities/1")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if strings.Contains(body, "<h2>Laps</h2>") {
		t.Error("no laps: the laps section should not be rendered")
	}
	if strings.Contains(body, "NaN") || strings.Contains(body, "Inf") {
		t.Errorf("zero distance/time must not render NaN or Inf:\n%s", body)
	}
}

func TestDetailErrors(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 3))

	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/activities/999", http.StatusNotFound},
		{"/activities/abc", http.StatusBadRequest},
		{"/activities/0", http.StatusBadRequest},
		{"/activities/-3", http.StatusBadRequest},
		{"/activities/1.5", http.StatusBadRequest},
		{"/activities/99999999999999999999", http.StatusBadRequest},
	} {
		res, body := get(t, h, tc.path)
		if res.StatusCode != tc.status {
			t.Errorf("GET %s = %d, want %d", tc.path, res.StatusCode, tc.status)
		}
		if !strings.Contains(body, "Back to all activities") {
			t.Errorf("GET %s: error page missing the way back", tc.path)
		}
	}
}

func decodeSeries(t *testing.T, body string) (id int64, points []map[string]any) {
	t.Helper()

	var out struct {
		ID     int64            `json:"id"`
		Points []map[string]any `json:"points"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, body)
	}
	return out.ID, out.Points
}

func TestSeries(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 60))

	res, body := get(t, h, "/api/activities/1/series")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	id, points := decodeSeries(t, body)
	if id != 1 || len(points) != 60 {
		t.Fatalf("id = %d, %d points; want 1, 60 (fewer than the default cap, so no averaging)", id, len(points))
	}

	// Odd points have no heart rate and cadence was never recorded: both must be
	// JSON null, which the chart draws as a gap, not as 0.
	if points[0]["hr"] == nil {
		t.Error("point 0 should have a heart rate")
	}
	if points[1]["hr"] != nil {
		t.Errorf("point 1 hr = %v, want null", points[1]["hr"])
	}
	if points[0]["cadence"] != nil {
		t.Errorf("cadence = %v, want null", points[0]["cadence"])
	}
	if got := points[1]["paceSecPerKm"].(float64); got < 303 || got > 304 {
		t.Errorf("pace = %v, want about 303.03 (1000 / 3.3)", got)
	}
}

func TestSeriesPointsParameter(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 300))

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", 300}, // below the default cap of 500
		{"?points=50", 50},
		{"?points=1", 1},
		{"?points=100000", 300}, // clamped to the max, which is above the data size
	} {
		res, body := get(t, h, "/api/activities/1/series"+tc.query)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%q: status = %d, want 200", tc.query, res.StatusCode)
		}
		if _, points := decodeSeries(t, body); len(points) != tc.want {
			t.Errorf("%q: %d points, want %d", tc.query, len(points), tc.want)
		}
	}

	for _, query := range []string{"?points=0", "?points=-5", "?points=abc"} {
		if res, _ := get(t, h, "/api/activities/1/series"+query); res.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", query, res.StatusCode)
		}
	}
}

func TestSeriesCapsAtMax(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", maxSeriesPoints*2+10))

	_, body := get(t, h, "/api/activities/1/series?points=999999")
	if _, points := decodeSeries(t, body); len(points) == 0 || len(points) > maxSeriesPoints {
		t.Errorf("%d points, want 1..%d", len(points), maxSeriesPoints)
	}
}

func TestSeriesWithoutPoints(t *testing.T) {
	h := newTestHandler(t, sampleRun("bare", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 1000, 300))

	_, body := get(t, h, "/api/activities/1/series")
	if !strings.Contains(body, `"points":[]`) {
		t.Errorf("an activity without trackpoints should give an empty array, got %s", body)
	}
}

func TestSeriesErrors(t *testing.T) {
	h := newTestHandler(t)

	for path, status := range map[string]int{
		"/api/activities/1/series":   http.StatusNotFound,
		"/api/activities/abc/series": http.StatusBadRequest,
	} {
		res, body := get(t, h, path)
		if res.StatusCode != status {
			t.Errorf("GET %s = %d, want %d", path, res.StatusCode, status)
		}
		if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q, errors from the API must be JSON", path, ct)
		}
		if !strings.Contains(body, `"error"`) {
			t.Errorf("GET %s: body = %s, want an error field", path, body)
		}
	}
}

func TestStaticChartAssets(t *testing.T) {
	h := newTestHandler(t)

	for path, want := range map[string]string{
		"/static/charts.js":    "data-charts",
		"/static/chart.umd.js": "Chart.js v4.4.7",
	} {
		res, body := get(t, h, path)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("GET %s = %d, want 200 containing %q", path, res.StatusCode, want)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Errorf("GET %s: Content-Type = %q, want JavaScript", path, ct)
		}
	}
}
