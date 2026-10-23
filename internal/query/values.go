package query

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const metersPerMile = 1609.344

var (
	distanceRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)(km|k|m|mi)?$`)
	paceRe     = regexp.MustCompile(`^(\d+):(\d{2})$`)
	clockRe    = regexp.MustCompile(`^(\d+):(\d{2})(?::(\d{2}))?$`)
	unitsRe    = regexp.MustCompile(`^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m(?:in)?)?(?:(\d+(?:\.\d+)?)s)?$`)
	numberRe   = regexp.MustCompile(`^\d+(?:\.\d+)?$`)
	dateRe     = regexp.MustCompile(`^(\d{4})(?:-(\d{1,2})(?:-(\d{1,2}))?)?$`)
)

// parseDistance returns kilometers. A bare number is kilometers.
func parseDistance(s string) (float64, error) {
	m := distanceRe.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return 0, fmt.Errorf(`can't read "%s" as a distance (try 5km, 800m or 3.1mi)`, s)
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	switch m[2] {
	case "m":
		return n / 1000, nil
	case "mi":
		return n * metersPerMile / 1000, nil
	default:
		return n, nil
	}
}

// parsePace returns seconds per kilometer. "6:20", "6:20/km" and "6:20 min/km"
// are the same, and a bare "6" means 6:00.
func parsePace(s string) (float64, error) {
	t := strings.ToLower(s)
	t = strings.TrimSuffix(t, "/km")
	t = strings.TrimSuffix(t, "min")
	t = strings.TrimSuffix(t, "/")

	if m := paceRe.FindStringSubmatch(t); m != nil {
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		if sec >= 60 {
			return 0, fmt.Errorf(`"%s" is not a pace: seconds must be below 60`, s)
		}
		return float64(min*60 + sec), nil
	}
	if numberRe.MatchString(t) {
		min, _ := strconv.ParseFloat(t, 64)
		return min * 60, nil
	}
	return 0, fmt.Errorf(`can't read "%s" as a pace (try 6:20 or 6:20/km)`, s)
}

// parseDuration returns seconds. It reads 1:02:05, 25:30 (minutes and seconds),
// 1h30m, 45m, 90s and a bare number of minutes.
func parseDuration(s string) (float64, error) {
	t := strings.ToLower(s)

	if m := clockRe.FindStringSubmatch(t); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if b >= 60 {
			return 0, fmt.Errorf(`"%s" is not a time: minutes and seconds must be below 60`, s)
		}
		if m[3] == "" { // m:ss
			return float64(a*60 + b), nil
		}
		c, _ := strconv.Atoi(m[3])
		if c >= 60 {
			return 0, fmt.Errorf(`"%s" is not a time: seconds must be below 60`, s)
		}
		return float64(a*3600 + b*60 + c), nil // h:mm:ss
	}

	if numberRe.MatchString(t) {
		min, _ := strconv.ParseFloat(t, 64)
		return min * 60, nil
	}

	if m := unitsRe.FindStringSubmatch(t); m != nil && (m[1] != "" || m[2] != "" || m[3] != "") {
		var total float64
		for i, mult := range []float64{3600, 60, 1} {
			if m[i+1] != "" {
				n, _ := strconv.ParseFloat(m[i+1], 64)
				total += n * mult
			}
		}
		return total, nil
	}

	return 0, fmt.Errorf(`can't read "%s" as a time (try 30:00, 1:15:00, 45m or 1h30m)`, s)
}

// parseDate reads 2026, 2026-03 or 2026-03-09 as the period it names: from its
// first instant up to, not including, the start of the next period.
func parseDate(s string, loc *time.Location) (start, end time.Time, err error) {
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, time.Time{}, fmt.Errorf(`can't read "%s" as a date (try 2026, 2026-03 or 2026-03-09)`, s)
	}
	year, _ := strconv.Atoi(m[1])

	switch {
	case m[2] == "": // a year
		start = time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
		end = start.AddDate(1, 0, 0)

	case m[3] == "": // a month
		month, _ := strconv.Atoi(m[2])
		if month < 1 || month > 12 {
			return time.Time{}, time.Time{}, fmt.Errorf(`"%s" is not a date: there is no month %d`, s, month)
		}
		start = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)

	default: // a day
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		start = time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
		// time.Date rolls 2026-02-30 over to March: reject that instead.
		if month < 1 || month > 12 || start.Day() != day || int(start.Month()) != month {
			return time.Time{}, time.Time{}, fmt.Errorf(`"%s" is not a date: that day does not exist`, s)
		}
		end = start.AddDate(0, 0, 1)
	}
	return start, end, nil
}

// What a run shows is rounded: 4996 m is "5.00 km", 372.5 s/km is "6:13". The
// query compares what you see, so a run listed as 5.00 km matches "distance >= 5km".

func shownKm(meters float64) float64 { return math.Round(meters/10) / 100 }

func shownSeconds(seconds float64) float64 { return math.Round(seconds) }

// shownPace is seconds per km, rounded like the list does. ok is false for a run
// with no distance or no time: it has no pace.
func shownPace(meters, seconds float64) (pace float64, ok bool) {
	if meters <= 0 || seconds <= 0 {
		return 0, false
	}
	return math.Round(seconds / (meters / 1000)), true
}
