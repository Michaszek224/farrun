package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/michaszek224/farrun/internal/analysis"
	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/store"
)

const (
	defaultSeriesPoints = 500
	maxSeriesPoints     = 2000
)

type lapRow struct {
	Number   int
	Distance float64
	Duration float64
	Pace     string
	AvgHR    string
	MaxHR    string
	Calories string
}

type detailPage struct {
	ID            int64
	Start         time.Time
	Sport         string
	Device        string
	Distance      float64
	Duration      float64
	Pace          string
	AvgHR         string
	MaxHR         string
	ElevationGain string
	Laps          []lapRow
	SeriesURL     string
	Label         string
}

// parseID reads the {id} path value. It reports false for anything that is not
// a positive integer, so callers can answer 400 instead of looking it up.
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *server) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		s.errorPage(w, r, http.StatusBadRequest, "That is not a valid activity id.")
		return
	}

	a, err := s.store.Activity(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, r, http.StatusNotFound, "There is no activity with that id.")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.render(w, r, "detail", http.StatusOK, s.newDetailPage(a))
}

func (s *server) newDetailPage(a model.Activity) detailPage {
	p := detailPage{
		ID:            a.ID,
		Start:         a.StartTime,
		Sport:         a.Sport,
		Device:        a.Device,
		Distance:      a.DistanceMeters,
		Duration:      a.DurationSeconds,
		Pace:          paceOrDash(a.DistanceMeters, a.DurationSeconds),
		AvgHR:         "–",
		MaxHR:         "–",
		ElevationGain: fmt.Sprintf("%.0f", analysis.ElevationGain(a.Points)),
		SeriesURL:     fmt.Sprintf("/api/activities/%d/series", a.ID),
		Label:         fmt.Sprintf("%s · %s km", a.StartTime.In(s.loc).Format("2006-01-02"), formatKm(a.DistanceMeters)),
	}
	if avg, ok := analysis.AvgHR(a.Points); ok {
		p.AvgHR = fmt.Sprintf("%.0f", avg)
	}
	if maxHR, ok := analysis.MaxHR(a.Points); ok {
		p.MaxHR = strconv.Itoa(maxHR)
	}

	for i, l := range a.Laps {
		p.Laps = append(p.Laps, lapRow{
			Number:   i + 1,
			Distance: l.DistanceMeters,
			Duration: l.DurationSeconds,
			Pace:     paceOrDash(l.DistanceMeters, l.DurationSeconds),
			AvgHR:    intOrDash(l.AvgHR),
			MaxHR:    intOrDash(l.MaxHR),
			Calories: intOrDash(l.Calories),
		})
	}
	return p
}

func paceOrDash(meters, seconds float64) string {
	if pace, ok := analysis.Pace(meters, seconds); ok {
		return formatPace(pace)
	}
	return "–"
}

func intOrDash(v *int) string {
	if v == nil {
		return "–"
	}
	return strconv.Itoa(*v)
}

type seriesResponse struct {
	ID     int64                  `json:"id"`
	Points []analysis.SeriesPoint `json:"points"`
}

// handleSeries serves the chart data of one activity as JSON. The optional
// "points" query parameter caps the number of samples.
func (s *server) handleSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid activity id")
		return
	}

	maxPoints := defaultSeriesPoints
	if v := r.URL.Query().Get("points"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "points must be a positive integer")
			return
		}
		maxPoints = min(n, maxSeriesPoints)
	}

	a, err := s.store.Activity(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "activity not found")
		return
	}
	if err != nil {
		slogError(r, err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	points := analysis.Series(a.Points, maxPoints)
	if points == nil {
		points = []analysis.SeriesPoint{} // "[]", not "null"
	}
	writeJSON(w, http.StatusOK, seriesResponse{ID: a.ID, Points: points})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
