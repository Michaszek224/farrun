// Package query is a small SQL-like language for filtering the run list:
//
//	2026                              any run whose date, sport or device contains "2026"
//	distance > 5km                    longer than 5 km
//	pace < 6:20                       faster than 6:20 per km
//	date >= 2026-03 and time > 30:00  runs since March 2026 that lasted over 30 minutes
//	distance between 5km and 10km     5 to 10 km, both included
//	sport = running or not (pace > 7:00)
//
// It is pure: no database and no HTTP, so it is easy to test.
package query

import (
	"strings"
	"time"
)

// Subject is what a query is matched against: the facts the run list shows.
type Subject struct {
	Start           time.Time
	Sport           string
	Device          string
	DistanceMeters  float64
	DurationSeconds float64
}

// Query is a parsed query. The zero value matches everything.
type Query struct {
	root node
	loc  *time.Location
}

// Error says what is wrong with a query and where (a byte offset into it).
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// maxLength and maxDepth keep a hostile query from costing much: the query
// comes straight from the URL.
const (
	maxLength = 500
	maxDepth  = 20
)

// Parse reads a query. Dates in it, and the date text that bare words search,
// are in loc, the time zone the page shows times in. An empty query matches
// every run.
func Parse(input string, loc *time.Location) (*Query, error) {
	if len(input) > maxLength {
		return nil, &Error{Pos: maxLength, Msg: "the query is too long"}
	}
	if loc == nil {
		loc = time.UTC
	}

	toks, err := lex(input)
	if err != nil {
		return nil, err
	}

	p := &parser{toks: toks, loc: loc}
	if p.peek().kind == tEOF {
		return &Query{loc: loc}, nil
	}
	root, err := p.parseOr(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		if t.kind == tRParen {
			return nil, &Error{Pos: t.pos, Msg: `there is a ")" without a matching "("`}
		}
		return nil, &Error{Pos: t.pos, Msg: "unexpected " + describe(t)}
	}
	return &Query{root: root, loc: loc}, nil
}

// Match reports whether the run satisfies the query.
func (q *Query) Match(s Subject) bool {
	if q == nil || q.root == nil {
		return true
	}
	return q.root.match(s, q.loc)
}

// dateText is the date as the list shows it, and what bare words search.
func dateText(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02 15:04")
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
