package web

import (
	"fmt"
	"html/template"
	"math"
	"time"
)

func (s *server) funcs() template.FuncMap {
	return template.FuncMap{
		"date":     func(t time.Time) string { return t.In(s.loc).Format("2006-01-02 15:04") },
		"km":       formatKm,
		"duration": formatDuration,
		"pace":     formatPace,
	}
}

func formatKm(meters float64) string {
	return fmt.Sprintf("%.2f", meters/1000)
}

// formatDuration renders seconds as m:ss, or h:mm:ss from one hour up.
func formatDuration(seconds float64) string {
	total := int(math.Round(seconds))
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// formatPace renders seconds per kilometer as m:ss, e.g. 5:42.
func formatPace(secPerKm float64) string {
	total := int(math.Round(secPerKm))
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
