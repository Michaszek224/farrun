package web

import (
	"net/http"
	"time"

	"github.com/michaszek224/farrun/internal/analysis"
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

type listPage struct {
	Rows          []listRow
	TotalDistance float64
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	activities, err := s.store.ListActivities(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	page := listPage{Rows: make([]listRow, 0, len(activities))}
	for _, a := range activities {
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

	s.render(w, r, "list", http.StatusOK, page)
}
