package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/michaszek224/farrun/internal/query"
)

// queryRuns: three runs, all Running unless noted. Times are UTC; the page shows Warsaw.
//
//	2026-10-20 07:30  5.00 km  25:00  pace 5:00
//	2026-01-01 00:30  10.00 km 1:02:05 pace 6:13  (23:30 UTC on Dec 31: a time-zone trap)
//	2026-06-01 08:00  0 km     1:00    no pace
func queryHandler(t *testing.T) http.Handler {
	t.Helper()

	a := sampleRun("easy", time.Date(2026, 10, 20, 5, 30, 0, 0, time.UTC), 5000, 1500)
	b := sampleRun("long", time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC), 10000, 3725)
	c := sampleRun("empty", time.Date(2026, 6, 1, 6, 0, 0, 0, time.UTC), 0, 60)
	c.Sport = "Walking"
	return newTestHandler(t, a, b, c)
}

func listFor(t *testing.T, h http.Handler, q string) (*http.Response, string) {
	t.Helper()
	return get(t, h, "/activities?"+url.Values{"q": {q}}.Encode())
}

// shownDates are the dates of the rows in the table, in the order they appear on the page.
func shownDates(body string) []string {
	type hit struct {
		at   int
		date string
	}
	var hits []hit
	for _, d := range []string{"2026-10-20 07:30", "2026-01-01 00:30", "2026-06-01 08:00"} {
		if i := strings.Index(body, ">"+d+"</a>"); i >= 0 {
			hits = append(hits, hit{i, d})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })

	var dates []string
	for _, h := range hits {
		dates = append(dates, h.date)
	}
	return dates
}

func TestListFilter(t *testing.T) {
	h := queryHandler(t)
	const (
		easy   = "2026-10-20 07:30"
		long   = "2026-01-01 00:30"
		nodist = "2026-06-01 08:00"
	)

	for _, tc := range []struct {
		query string
		want  []string // dates shown, newest first
		count string   // the status line
	}{
		{"", []string{easy, nodist, long}, "3 runs"},
		{"distance > 5km", []string{long}, "1 of 3 runs match"},
		{"distance >= 5km", []string{easy, long}, "2 of 3 runs match"},
		{"pace < 6:20", []string{easy, long}, "2 of 3 runs match"},
		{"pace < 6:00", []string{easy}, "1 of 3 runs match"},
		{"distance > 5km and pace < 6:00", nil, ""},
		{"distance > 5km or pace < 6:00", []string{easy, long}, "2 of 3 runs match"},
		{"2026", []string{easy, nodist, long}, "3 runs"}, // all 2026 in Warsaw, including the New Year's one
		{"2026-10", []string{easy}, "1 of 3 runs match"},
		{"walking", []string{nodist}, "1 of 3 runs match"},
		{"sport = running", []string{easy, long}, "2 of 3 runs match"},
		{"date < 2026-06", []string{long}, "1 of 3 runs match"},
		{"time > 1h", []string{long}, "1 of 3 runs match"},
		{"distance between 5km and 10km", []string{easy, long}, "2 of 3 runs match"},
		{"not walking", []string{easy, long}, "2 of 3 runs match"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			res, body := listFor(t, h, tc.query)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}

			got := shownDates(body)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("shows %v, want %v", got, tc.want)
			}
			if tc.count != "" && !strings.Contains(body, tc.count) {
				t.Errorf("status line does not contain %q", tc.count)
			}
			if strings.Contains(body, "query-error") {
				t.Error("a valid query shows an error")
			}
		})
	}
}

func TestListFilterTotalsFollowTheFilter(t *testing.T) {
	_, body := listFor(t, queryHandler(t), "distance >= 5km")
	if !strings.Contains(body, "15.00 km") {
		t.Errorf("the total should be the 15.00 km of the two matching runs:\n%s", body)
	}
}

func TestListFilterNoMatches(t *testing.T) {
	res, body := listFor(t, queryHandler(t), "distance > 100km")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, "No runs match <code>distance &gt; 100km</code>") {
		t.Errorf("no 'no runs match' message:\n%s", body)
	}
	if strings.Contains(body, "data-compare-form") || strings.Contains(body, "Compare selected") || strings.Contains(body, `data-sort-key`) {
		t.Error("the runs table and compare bar should be hidden when nothing matches")
	}
	// The box must stay, with the query in it, so it can be edited.
	if !strings.Contains(body, `value="distance &gt; 100km"`) || !strings.Contains(body, `class="clear"`) {
		t.Error("the filter box (with its text and a clear link) must stay when nothing matches")
	}
	if !strings.Contains(body, "/static/query.js") {
		t.Error("query.js must still load so / works")
	}
}

func TestListFilterInvalidQuery(t *testing.T) {
	h := queryHandler(t)

	res, body := listFor(t, h, "distance > abc")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a bad query is not a server error)", res.StatusCode)
	}
	for _, want := range []string{
		`class="query-error"`,
		`can&#39;t read &#34;abc&#34; as a distance`,
		"The query was not applied",
		`value="distance &gt; abc"`, // kept, so it can be fixed
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// All runs are shown, so the page does not look empty.
	if got := shownDates(body); len(got) != 3 {
		t.Errorf("an invalid query should show all 3 runs, shows %v", got)
	}

	// A caret points at the spot: "distance > abc" has the value at column 11.
	if !strings.Contains(body, "  distance &gt; abc\n  "+strings.Repeat(" ", 11)+"^") {
		t.Errorf("the caret is not under the bad value:\n%s", body)
	}
}

func TestListFilterEscapesTheQuery(t *testing.T) {
	h := queryHandler(t)
	evil := `"><script>alert(1)</script>`

	for name, q := range map[string]string{"in a bad query": evil, "in no matches": "walking and " + evil + "x"} {
		_, body := listFor(t, h, q)
		if strings.Contains(body, "<script>alert(1)") {
			t.Errorf("%s: the query was not HTML-escaped", name)
		}
	}
	// And a plain word that matches nothing is escaped in the "no runs match" message.
	_, body := listFor(t, h, "<b>nothing</b>")
	if strings.Contains(body, "<b>nothing</b>") {
		t.Error("the 'no runs match' message does not escape the query")
	}
}

func TestListFilterBox(t *testing.T) {
	h := queryHandler(t)

	_, body := get(t, h, "/activities")
	for _, want := range []string{
		`<form class="query" method="get" action="/activities" role="search">`,
		`name="q"`,
		"query help",
		"/static/query.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
	if strings.Contains(body, `class="clear"`) {
		t.Error("the clear link should only show while a query is applied")
	}

	// The box is its own form: the compare form must not be nested inside it.
	if i, j := strings.Index(body, `<form class="query"`), strings.Index(body, `data-compare-form`); i < 0 || j < 0 || i > j {
		t.Error("the filter form must come before, and not inside, the compare form")
	}
	if strings.Count(body, "<form") != 2 {
		t.Errorf("want 2 forms (filter, compare), got %d", strings.Count(body, "<form"))
	}
}

func TestListFilterBoxNeedsRuns(t *testing.T) {
	_, body := get(t, newTestHandler(t), "/activities")
	if strings.Contains(body, "form") && strings.Contains(body, `class="query"`) {
		t.Error("an empty database should not show a filter box")
	}
	if strings.Contains(body, "query.js") {
		t.Error("an empty database should not load query.js")
	}
	if !strings.Contains(body, "No activities yet") {
		t.Error("the empty state is gone")
	}
}

// Every example under the box must parse, link to itself, and give a result without errors.
func TestExampleQueriesWork(t *testing.T) {
	h := queryHandler(t)
	_, body := get(t, h, "/activities")

	// Attribute values are HTML-escaped ("+" becomes "&#43;"); the browser decodes them.
	var hrefs []string
	for _, m := range regexp.MustCompile(`<li><a href="([^"]+)">`).FindAllStringSubmatch(body, -1) {
		hrefs = append(hrefs, html.UnescapeString(m[1]))
	}
	if len(hrefs) != len(exampleQueries) {
		t.Fatalf("found %d example links, want %d", len(hrefs), len(exampleQueries))
	}

	for i, example := range exampleQueries {
		if _, err := query.Parse(example, time.UTC); err != nil {
			t.Errorf("example %q does not parse: %v", example, err)
		}

		want := "/activities?" + url.Values{"q": {example}}.Encode()
		if hrefs[i] != want {
			t.Errorf("link %d = %q, want %q", i, hrefs[i], want)
		}

		// Following the link must give the same query back in the box.
		u, err := url.Parse(hrefs[i])
		if err != nil || u.Query().Get("q") != example {
			t.Errorf("link %q does not carry the query %q", hrefs[i], example)
		}
		res, page := get(t, h, hrefs[i])
		if res.StatusCode != http.StatusOK || strings.Contains(page, "query-error") {
			t.Errorf("example %q gives status %d or an error", example, res.StatusCode)
		}
	}
}

func TestQueryScriptAndStyles(t *testing.T) {
	h := queryHandler(t)

	res, js := get(t, h, "/static/query.js")
	if res.StatusCode != http.StatusOK || !strings.Contains(js, "form.query") {
		t.Errorf("GET /static/query.js = %d, want the script", res.StatusCode)
	}

	_, css := get(t, h, "/static/style.css")
	for _, sel := range []string{"form.query", ".query-error", ".list-head", ".help-body"} {
		if !strings.Contains(css, sel) {
			t.Errorf("stylesheet has no %q rules", sel)
		}
	}
}
