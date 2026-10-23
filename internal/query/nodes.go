package query

import (
	"math"
	"strings"
	"time"
)

// node is one piece of a parsed query.
type node interface {
	match(s Subject, loc *time.Location) bool
}

type andNode struct{ left, right node }
type orNode struct{ left, right node }
type notNode struct{ inner node }

func (n andNode) match(s Subject, l *time.Location) bool {
	return n.left.match(s, l) && n.right.match(s, l)
}
func (n orNode) match(s Subject, l *time.Location) bool {
	return n.left.match(s, l) || n.right.match(s, l)
}
func (n notNode) match(s Subject, l *time.Location) bool { return !n.inner.match(s, l) }

// textNode is a bare word: it matches when the date, sport or device contains it.
type textNode struct{ needle string }

func (n textNode) match(s Subject, loc *time.Location) bool {
	return containsFold(dateText(s.Start, loc), n.needle) ||
		containsFold(s.Sport, n.needle) ||
		containsFold(s.Device, n.needle)
}

// number reads one numeric fact off a run, as the list shows it. ok is false when
// the run has none (a run without distance has no pace), and then no comparison
// matches, not even "!=": like NULL in SQL, "unknown" is neither equal nor different.
type number func(Subject) (value float64, ok bool)

type numCmp struct {
	get   number
	op    string
	value float64
}

func (n numCmp) match(s Subject, _ *time.Location) bool {
	v, ok := n.get(s)
	return ok && compare(n.op, v, n.value)
}

type numBetween struct {
	get    number
	lo, hi float64
}

func (n numBetween) match(s Subject, _ *time.Location) bool {
	v, ok := n.get(s)
	return ok && v >= n.lo-epsilon && v <= n.hi+epsilon
}

const epsilon = 1e-9

func compare(op string, a, b float64) bool {
	switch op {
	case "=":
		return math.Abs(a-b) < epsilon
	case "!=":
		return math.Abs(a-b) >= epsilon
	case "<":
		return a < b-epsilon
	case "<=":
		return a <= b+epsilon
	case ">":
		return a > b+epsilon
	case ">=":
		return a >= b-epsilon
	}
	return false
}

// dateCmp compares against a whole period, so "date = 2026" means "in 2026" and
// "date > 2026-03" means "after March". The period is [start, end).
type dateCmp struct {
	op         string
	start, end time.Time
}

func (n dateCmp) match(s Subject, _ *time.Location) bool {
	t := s.Start
	switch n.op {
	case "=":
		return !t.Before(n.start) && t.Before(n.end)
	case "!=":
		return t.Before(n.start) || !t.Before(n.end)
	case "<":
		return t.Before(n.start)
	case "<=":
		return t.Before(n.end)
	case ">":
		return !t.Before(n.end)
	case ">=":
		return !t.Before(n.start)
	}
	return false
}

// dateBetween runs from the first instant of one period to the end of another.
type dateBetween struct{ start, end time.Time }

func (n dateBetween) match(s Subject, _ *time.Location) bool {
	return !s.Start.Before(n.start) && s.Start.Before(n.end)
}

// textCmp compares sport or device, ignoring case.
type textCmp struct {
	get   func(Subject) string
	op    string // "=", "!=" or "like" (contains)
	value string
}

func (n textCmp) match(s Subject, _ *time.Location) bool {
	got := n.get(s)
	switch n.op {
	case "=":
		return strings.EqualFold(got, n.value)
	case "!=":
		return !strings.EqualFold(got, n.value)
	case "like":
		return containsFold(got, n.value)
	}
	return false
}
