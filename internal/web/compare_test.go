package web

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	a := detailedRun("a", 20)
	b := detailedRun("b", 20)
	b.StartTime = b.StartTime.AddDate(0, 0, 3)
	b.DistanceMeters = 5000

	h := newTestHandler(t, a, b)

	res, body := get(t, h, "/compare?id=1&id=2")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	for _, want := range []string{
		"Compare 2 runs",
		"2026-10-20 07:30",
		"2026-10-23 07:30",
		`href="/activities/1"`,
		`href="/activities/2"`,
		`data-series-url="/api/activities/1/series"`,
		`data-series-url="/api/activities/2/series"`,
		`data-label="2026-10-23 · 5.00 km"`,
		`class="swatch s0"`,
		`class="swatch s1"`,
		"/static/charts.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestCompareKeepsOrderAndDropsDuplicates(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 3), detailedRun("b", 3), detailedRun("c", 3))

	_, body := get(t, h, "/compare?id=3&id=1&id=3&id=2")
	if !strings.Contains(body, "Compare 3 runs") {
		t.Error("duplicate id should be counted once")
	}

	first, second, third := strings.Index(body, "/api/activities/3/"), strings.Index(body, "/api/activities/1/"), strings.Index(body, "/api/activities/2/")
	if !(first >= 0 && first < second && second < third) {
		t.Errorf("runs are not in the requested order 3, 1, 2 (positions %d, %d, %d)", first, second, third)
	}
}

func TestCompareErrors(t *testing.T) {
	h := newTestHandler(t, detailedRun("a", 3), detailedRun("b", 3))

	tooMany := "/compare?"
	for i := 1; i <= maxCompare+1; i++ {
		tooMany += fmt.Sprintf("id=%d&", i)
	}

	for _, tc := range []struct {
		name, path string
		status     int
		message    string
	}{
		{"nothing selected", "/compare", http.StatusBadRequest, "at least two"},
		{"only one", "/compare?id=1", http.StatusBadRequest, "at least two"},
		{"same one twice", "/compare?id=1&id=1", http.StatusBadRequest, "at least two"},
		{"too many", tooMany, http.StatusBadRequest, "at most"},
		{"not a number", "/compare?id=1&id=abc", http.StatusBadRequest, "not valid"},
		{"zero", "/compare?id=1&id=0", http.StatusBadRequest, "not valid"},
		{"negative", "/compare?id=1&id=-2", http.StatusBadRequest, "not valid"},
		{"unknown activity", "/compare?id=1&id=99", http.StatusNotFound, "no longer exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, body := get(t, h, tc.path)
			if res.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.status)
			}
			if !strings.Contains(body, tc.message) {
				t.Errorf("body does not contain %q:\n%s", tc.message, body)
			}
		})
	}
}

func TestParseCompareIDs(t *testing.T) {
	ids, problem := parseCompareIDs([]string{"5", "2", "5", "9"})
	if problem != "" || !reflect.DeepEqual(ids, []int64{5, 2, 9}) {
		t.Errorf("parseCompareIDs() = %v, %q; want [5 2 9], no problem", ids, problem)
	}
}

func TestListHasCompareForm(t *testing.T) {
	h := newTestHandler(t,
		sampleRun("a", time.Date(2026, 10, 18, 5, 30, 0, 0, time.UTC), 5000, 1500),
		sampleRun("b", time.Date(2026, 10, 19, 5, 30, 0, 0, time.UTC), 6000, 1800),
	)

	_, body := get(t, h, "/activities")
	for _, want := range []string{
		`<form method="get" action="/compare"`,
		`name="id" value="1"`,
		`name="id" value="2"`,
		`data-min="2"`,
		fmt.Sprintf(`data-max="%d"`, maxCompare),
		"/static/list.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}

	res, js := get(t, h, "/static/list.js")
	if res.StatusCode != http.StatusOK || !strings.Contains(js, "data-compare-form") {
		t.Errorf("GET /static/list.js = %d, want the script", res.StatusCode)
	}
}

func TestListWithoutRunsHasNoCompareForm(t *testing.T) {
	_, body := get(t, newTestHandler(t), "/activities")
	if strings.Contains(body, "<form") || strings.Contains(body, "list.js") {
		t.Error("the empty state should not render the compare form or its script")
	}
}
