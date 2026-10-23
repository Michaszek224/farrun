package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/michaszek224/farrun/internal/analysis"
	"github.com/michaszek224/farrun/internal/query"
)

type listRow struct {
	ID       int64
	Start    time.Time
	Sport    string
	Device   string
	Distance float64
	Duration float64
	Pace     float64
	HasPace  bool
}

// queryExample is a clickable sample query under the filter box.
type queryExample struct {
	Text string
	URL  string
}

var exampleQueries = []string{
	"2026",
	"distance > 5km",
	"pace < 6:20",
	"date >= 2026-03 and distance > 5km",
	"time > 30:00 and pace < 6:30",
	"distance between 5km and 10km",
	"sport = running and not (pace > 7:00)",
}

type listPage struct {
	Rows          []listRow
	TotalDistance float64
	MinCompare    int
	MaxCompare    int

	HasAny       bool // the database has runs at all, even if the filter hides them
	Total        int  // runs in the database
	Matched      int  // runs the query lets through
	Query        string
	Filtered     bool   // a valid, non-empty query is applied
	QueryError   string // what is wrong with the query, if anything
	QueryPointer string // spaces and a ^ that point at the problem under the query
	Examples     []queryExample
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	activities, err := s.store.ListActivities(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	page := listPage{
		Rows:       make([]listRow, 0, len(activities)),
		MinCompare: minCompare,
		MaxCompare: maxCompare,
		HasAny:     len(activities) > 0,
		Total:      len(activities),
		Query:      strings.TrimSpace(r.URL.Query().Get("q")),
	}
	for _, e := range exampleQueries {
		page.Examples = append(page.Examples, queryExample{Text: e, URL: "/activities?" + url.Values{"q": {e}}.Encode()})
	}

	// A query that does not parse is reported, and the list stays unfiltered:
	// an empty page would look as if there were no runs.
	q, err := query.Parse(page.Query, s.loc)
	if err != nil {
		page.QueryError = err.Error()
		var qe *query.Error
		if errors.As(err, &qe) {
			page.QueryPointer = strings.Repeat(" ", qe.Pos) + "^"
		}
		q = nil
	}
	page.Filtered = q != nil && page.Query != ""

	for _, a := range activities {
		if q != nil && !q.Match(query.Subject{
			Start:           a.StartTime,
			Sport:           a.Sport,
			Device:          a.Device,
			DistanceMeters:  a.DistanceMeters,
			DurationSeconds: a.DurationSeconds,
		}) {
			continue
		}

		row := listRow{
			ID:       a.ID,
			Start:    a.StartTime,
			Sport:    a.Sport,
			Device:   a.Device,
			Distance: a.DistanceMeters,
			Duration: a.DurationSeconds,
		}
		row.Pace, row.HasPace = analysis.Pace(a.DistanceMeters, a.DurationSeconds)
		page.Rows = append(page.Rows, row)
		page.TotalDistance += a.DistanceMeters
	}
	page.Matched = len(page.Rows)

	s.render(w, r, "list", http.StatusOK, page)
}
