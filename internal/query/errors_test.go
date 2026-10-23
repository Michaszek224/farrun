package query

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct {
		query   string
		message string // a part of the message the user sees
		pos     int    // where it points, or -1 to not check
	}{
		// a value is missing or unreadable
		{"distance >", "expected a value", -1},
		{"distance <", "expected a value", -1},
		{"distance > abc", `can't read "abc" as a distance`, 11},
		{"distance > -5", `can't read "-5" as a distance`, -1},
		{"pace < 6:75", "seconds must be below 60", -1},
		{"pace < 6:2", `can't read "6:2" as a pace`, -1},
		{"pace < fast", `can't read "fast" as a pace`, -1},
		{"time > 99:99", "must be below 60", -1},
		{"time > 1:02:99", "seconds must be below 60", -1},
		{"time > x", `can't read "x" as a time`, -1},
		{"date = 2026-13", "there is no month 13", -1},
		{"date = 2026-00", "there is no month 0", -1},
		{"date = 2026-02-30", "that day does not exist", -1},
		{"date = 2026-04-31", "that day does not exist", -1},
		{"date = 202", `can't read "202" as a date`, -1},
		{"date = yesterday", `can't read "yesterday" as a date`, -1},

		// a field without a comparison
		{"distance", `"distance" needs a comparison`, 0},
		{"distance 5km", `"distance" needs a comparison`, 0},
		{"pace and distance > 5km", `"pace" needs a comparison`, 0},

		// the wrong operator for the field
		{"distance like 5", `"like" only works on sport and device`, -1},
		{"distance ~ 5", `"like" only works on sport and device`, -1},
		{"sport > running", "can only be compared with =, != or like", -1},
		{"device < x", "can only be compared with =, != or like", -1},
		{"sport between a and b", `"between" does not work on sport`, -1},
		{"sport =", "expected a name", -1},

		// between
		{"distance between 5km 10km", `"between" needs "and"`, -1},
		{"distance between 5km", `"between" needs "and"`, -1},
		{"date between 2026-01 2026-03", `"between" needs "and"`, -1},

		// structure
		{"and", `unexpected "and"`, 0},
		{"or running", `unexpected "or"`, 0},
		{"running and", "ends too early", -1},
		{"distance > 5km and", "ends too early", -1},
		{"not", "ends too early", -1},
		{"(running", `a "(" was opened but never closed`, 0},
		{"running)", `without a matching "("`, 7},
		{"()", `unexpected ")"`, 1},
		{"= 5", `"=" needs a field before it`, 0},
		{"> 5km", `">" needs a field before it`, 0},
		{"!", `must be written "!="`, 0},
		{`"unterminated`, "never closed", 0},
		{"like", `unexpected "like"`, 0},
		{"between", `unexpected "between"`, 0},

		// limits
		{strings.Repeat("a", maxLength+1), "too long", -1},
		{strings.Repeat("(", maxDepth+5) + "x" + strings.Repeat(")", maxDepth+5), "nested too deeply", -1},
		{strings.Repeat("not ", maxDepth+5) + "x", "nested too deeply", -1},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := Parse(tc.query, warsaw)
			if err == nil {
				t.Fatalf("Parse(%q) = %+v, want an error containing %q", tc.query, q, tc.message)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Parse(%q) error = %q, want it to contain %q", tc.query, err, tc.message)
			}

			var qe *Error
			if !errors.As(err, &qe) {
				t.Fatalf("error is %T, want *query.Error", err)
			}
			if tc.pos >= 0 && qe.Pos != tc.pos {
				t.Errorf("Parse(%q) points at %d, want %d", tc.query, qe.Pos, tc.pos)
			}
			if qe.Pos < 0 || qe.Pos > utf8.RuneCountInString(tc.query) {
				t.Errorf("position %d is outside the query", qe.Pos)
			}
		})
	}
}

func TestEmptyAndNilQueriesMatchEverything(t *testing.T) {
	s := runs["easy5k"]

	var nilQuery *Query
	if !nilQuery.Match(s) {
		t.Error("a nil *Query should match everything")
	}
	if !(&Query{}).Match(s) {
		t.Error("the zero Query should match everything")
	}
	q, err := Parse("", nil)
	if err != nil || !q.Match(s) {
		t.Errorf("Parse(\"\", nil) = %v, %v; want a query that matches everything", q, err)
	}
}

// With no time zone given, dates are read in UTC, so 23:30 UTC on Dec 31 is 2025.
func TestNilLocationMeansUTC(t *testing.T) {
	q, err := Parse("date = 2025", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Match(runs["newyear10k"]) {
		t.Error("23:30 UTC on 2025-12-31 should be 2025 in UTC")
	}
}

// The same instant is a different date in different zones; the query follows the
// zone it was parsed with, because that is the zone the page shows.
func TestDateFollowsTheTimeZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 02:30 UTC on Jan 1 is still Dec 31, 21:30 in New York.
	s := Subject{Start: time.Date(2026, 1, 1, 2, 30, 0, 0, time.UTC)}

	for loc, want := range map[*time.Location]string{time.UTC: "2026", ny: "2025"} {
		q, _ := Parse("date = "+want, loc)
		if !q.Match(s) {
			t.Errorf("in %v the run should be in %s", loc, want)
		}
		other := map[string]string{"2026": "2025", "2025": "2026"}[want]
		if q, _ := Parse("date = "+other, loc); q.Match(s) {
			t.Errorf("in %v the run should not be in %s", loc, other)
		}
	}

	// Bare words search the date as shown in that zone too.
	if q, _ := Parse("2025-12-31", ny); !q.Match(s) {
		t.Error("the bare text 2025-12-31 should match the New York date")
	}
}

func TestTextSearchIsCaseAndAccentFriendly(t *testing.T) {
	s := Subject{Start: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Sport: "Bieganie", Device: "Zegarek ŁÓDŹ"}

	for _, q := range []string{"bieg", "BIEG", "łódź", "ŁÓDŹ", "sport = bieganie", "device like łódź"} {
		parsed, err := Parse(q, time.UTC)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", q, err)
			continue
		}
		if !parsed.Match(s) {
			t.Errorf("%q should match", q)
		}
	}
}

func TestOperatorsNeedNoSpaces(t *testing.T) {
	for _, q := range []string{"distance>5km", "distance>=5km", "pace<6:20", "pace<=6:20", "sport=running", "sport!=cycling", "(distance>5km)and(pace<6:00)"} {
		if _, err := Parse(q, warsaw); err != nil {
			t.Errorf("Parse(%q) error = %v", q, err)
		}
	}
}

// Whatever is typed into the box ends up here, so nothing may panic, however odd.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"", "2026", "distance > 5km", "pace < 6:20", "date between 2026-01 and 2026-03",
		"not (a or b) and c", `"quoted`, "((((", "))))", "= = =", "distance >", "between and",
		"sport like", "!", "<>", "~", "date = 2026-02-30", "time > 1h30m15s",
		"é ü 日本語", "\x00\xff", strings.Repeat("(", 50), strings.Repeat("not ", 50),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		q, err := Parse(input, warsaw)
		if err != nil {
			var qe *Error
			if !errors.As(err, &qe) || qe.Msg == "" {
				t.Fatalf("Parse(%q) returned %v, want a *query.Error with a message", input, err)
			}
			return
		}
		for _, s := range runs {
			q.Match(s) // must not panic
		}
	})
}
