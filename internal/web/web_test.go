package web

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/store"
)

func newTestHandler(t *testing.T, activities ...model.Activity) http.Handler {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if len(activities) > 0 {
		if _, err := st.SaveActivities(t.Context(), activities); err != nil {
			t.Fatalf("SaveActivities() error = %v", err)
		}
	}

	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(st, loc)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return h
}

func get(t *testing.T, h http.Handler, target string) (*http.Response, string) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	res := rec.Result()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(body)
}

func sampleRun(externalID string, start time.Time, meters, seconds float64) model.Activity {
	return model.Activity{
		Source:          "test",
		ExternalID:      externalID,
		Sport:           "Running",
		Device:          "Forerunner",
		StartTime:       start,
		DistanceMeters:  meters,
		DurationSeconds: seconds,
	}
}

func TestRootRedirectsToActivities(t *testing.T) {
	res, _ := get(t, newTestHandler(t), "/")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/activities" {
		t.Errorf("GET / = %d to %q, want 302 to /activities", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestListEmpty(t *testing.T) {
	res, body := get(t, newTestHandler(t), "/activities")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, "No activities yet") {
		t.Errorf("empty database should show the empty state, body:\n%s", body)
	}
	if strings.Contains(body, "<table") {
		t.Error("empty database should not render a table")
	}
}

func TestList(t *testing.T) {
	// 05:30 UTC is 07:30 in Warsaw (CEST), so this checks the time zone too.
	h := newTestHandler(t,
		sampleRun("older", time.Date(2026, 10, 18, 5, 30, 0, 0, time.UTC), 5000, 1500),
		sampleRun("newer", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 10000, 3725),
		sampleRun("no-distance", time.Date(2026, 10, 19, 5, 30, 0, 0, time.UTC), 0, 60),
	)

	res, body := get(t, h, "/activities")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	for _, want := range []string{
		"3 runs",
		"15.00 km in total",
		"2026-10-20 07:30",
		"10.00",   // distance
		"1:02:05", // duration over an hour
		"6:13",    // pace: 3725 s / 10 km = 372.5 s/km, rounded to 373
		"5:00",    // 1500 s / 5 km
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	// Newest first.
	if strings.Index(body, "2026-10-20") > strings.Index(body, "2026-10-19") ||
		strings.Index(body, "2026-10-19") > strings.Index(body, "2026-10-18") {
		t.Error("runs are not listed newest first")
	}

	// A run with no distance has no pace, shown as a dash rather than "0:00" or a crash.
	if !strings.Contains(body, "–") {
		t.Error("run without distance should show a dash for pace")
	}
}

func TestListEscapesHTML(t *testing.T) {
	a := sampleRun("x", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 1000, 300)
	a.Sport = `<script>alert(1)</script>`

	_, body := get(t, newTestHandler(t, a), "/activities")
	if strings.Contains(body, "<script>alert(1)") {
		t.Error("sport name was not HTML-escaped")
	}
}

func TestSecurityHeaders(t *testing.T) {
	res, _ := get(t, newTestHandler(t), "/activities")
	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
	} {
		if got := res.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestStaticAndNotFound(t *testing.T) {
	h := newTestHandler(t)

	res, body := get(t, h, "/static/style.css")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "--accent") {
		t.Errorf("GET /static/style.css = %d, want the stylesheet", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("stylesheet Content-Type = %q", ct)
	}

	if res, _ := get(t, h, "/static/nope.css"); res.StatusCode != http.StatusNotFound {
		t.Errorf("missing static file = %d, want 404", res.StatusCode)
	}
	if res, _ := get(t, h, "/nope"); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path = %d, want 404", res.StatusCode)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/activities", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /activities = %d, want 405", rec.Code)
	}
}

func TestFormatting(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{formatKm(5000), "5.00"},
		{formatKm(786.03), "0.79"},
		{formatDuration(324.392), "5:24"},
		{formatDuration(3599.6), "1:00:00"},
		{formatDuration(3725), "1:02:05"},
		{formatDuration(0), "0:00"},
		{formatPace(342), "5:42"},
		{formatPace(359.6), "6:00"},
		{formatPace(300), "5:00"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestListIsSortable(t *testing.T) {
	h := newTestHandler(t,
		sampleRun("normal", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 10000, 3725),
		sampleRun("no-distance", time.Date(2026, 10, 19, 5, 30, 0, 0, time.UTC), 0, 60),
		sampleRun("long", time.Date(2026, 10, 18, 5, 30, 0, 0, time.UTC), 1500000, 5400000),
	)

	_, body := get(t, h, "/activities")

	for _, key := range []string{"date", "sport", "distance", "time", "pace"} {
		if !strings.Contains(body, `data-sort-key="`+key+`"`) {
			t.Errorf("no sortable header for %q", key)
		}
	}

	for _, want := range []string{
		fmt.Sprintf(`data-sort="%d"`, time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC).Unix()), // date: unix seconds
		`data-sort="Running"`,
		`data-sort="10000"`,   // distance in meters
		`data-sort="3725"`,    // time in seconds
		`data-sort="372.5"`,   // pace in seconds per km
		`data-sort="1500000"`, // large values stay plain decimals...
		`data-sort="5400000"`, // ...never 1.5e+06, which would still parse but is fragile
		`<script src="/static/sort.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
	if strings.Contains(body, "e+0") {
		t.Error("a sort value is in exponent notation")
	}

	// A run without distance has no pace: its pace cell has an empty sort value,
	// which the script sorts last in either direction.
	if !strings.Contains(body, `data-sort="">–</td>`) {
		t.Error("run without distance should have an empty pace sort value")
	}
}

func TestListSortAsset(t *testing.T) {
	res, js := get(t, newTestHandler(t), "/static/sort.js")
	if res.StatusCode != http.StatusOK || !strings.Contains(js, "data-sort-key") {
		t.Errorf("GET /static/sort.js = %d, want the script", res.StatusCode)
	}
}

func TestListWithoutRunsHasNoSortScript(t *testing.T) {
	_, body := get(t, newTestHandler(t), "/activities")
	if strings.Contains(body, "sort.js") {
		t.Error("the empty state should not load the sort script")
	}
}

func TestFormatNum(t *testing.T) {
	for in, want := range map[float64]string{
		0:          "0",
		786.03:     "786.03",
		372.5:      "372.5",
		10000:      "10000",
		1500000:    "1500000",
		0.000001:   "0.000001",
		1234567.25: "1234567.25",
	} {
		if got := formatNum(in); got != want {
			t.Errorf("formatNum(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFontAssets(t *testing.T) {
	h := newTestHandler(t)

	for _, name := range []string{"JetBrainsMono-Regular.woff2", "JetBrainsMono-Bold.woff2"} {
		res, body := get(t, h, "/static/fonts/"+name)
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", name, res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); ct != "font/woff2" {
			t.Errorf("%s Content-Type = %q, want font/woff2", name, ct)
		}
		if !strings.HasPrefix(body, "wOF2") || len(body) < 50_000 {
			t.Errorf("%s is not a woff2 font (%d bytes)", name, len(body))
		}
	}

	// The font's licence has to ship with it.
	res, body := get(t, h, "/static/fonts/OFL.txt")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "SIL OPEN FONT LICENSE Version 1.1") {
		t.Errorf("GET /static/fonts/OFL.txt = %d, want the SIL Open Font License", res.StatusCode)
	}
}

// Every url(...) in the stylesheet must point at a file that is really served,
// and nothing may point at another site.
func TestStylesheetReferencesResolve(t *testing.T) {
	h := newTestHandler(t)
	_, css := get(t, h, "/static/style.css")

	urls := regexp.MustCompile(`url\(["']?([^"')]+)["']?\)`).FindAllStringSubmatch(css, -1)
	if len(urls) == 0 {
		t.Fatal("stylesheet has no url(...) references; the @font-face rules are gone")
	}
	for _, m := range urls {
		u := m[1]
		if !strings.HasPrefix(u, "/static/") {
			t.Errorf("stylesheet references %q, which is not served from /static/", u)
			continue
		}
		if res, _ := get(t, h, u); res.StatusCode != http.StatusOK {
			t.Errorf("stylesheet references %s, which returns %d", u, res.StatusCode)
		}
	}
	if strings.Contains(css, "@import") || strings.Contains(css, "http://") || strings.Contains(css, "https://") {
		t.Error("stylesheet loads something from another site")
	}
}

func TestLayoutHasTagline(t *testing.T) {
	_, body := get(t, newTestHandler(t), "/activities")
	if !strings.Contains(body, `class="tagline"`) {
		t.Error("layout is missing the header tagline")
	}
}

func TestDetailStatLabelsFitTheirFrames(t *testing.T) {
	_, body := get(t, newTestHandler(t, detailedRun("a", 3)), "/activities/1")
	for _, label := range []string{"Distance", "Time", "Avg pace", "Avg HR", "Max HR", "Climb"} {
		if !strings.Contains(body, "<dt>"+label+"</dt>") {
			t.Errorf("detail page is missing the %q stat", label)
		}
	}
}
