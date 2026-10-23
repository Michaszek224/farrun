package query

import (
	"fmt"
	"strings"
	"time"
)

type fieldKind int

const (
	kindDistance fieldKind = iota
	kindTime
	kindPace
	kindDate
	kindSport
	kindDevice
)

var fieldNames = map[string]fieldKind{
	"distance": kindDistance, "dist": kindDistance,
	"time": kindTime, "duration": kindTime,
	"pace":   kindPace,
	"date":   kindDate,
	"sport":  kindSport,
	"device": kindDevice,
}

func (k fieldKind) numeric() bool { return k == kindDistance || k == kindTime || k == kindPace }
func (k fieldKind) text() bool    { return k == kindSport || k == kindDevice }

// getters return what the list shows, rounded the way it shows it.
func (k fieldKind) number() number {
	switch k {
	case kindDistance:
		return func(s Subject) (float64, bool) { return shownKm(s.DistanceMeters), true }
	case kindTime:
		return func(s Subject) (float64, bool) { return shownSeconds(s.DurationSeconds), true }
	default: // kindPace
		return func(s Subject) (float64, bool) { return shownPace(s.DistanceMeters, s.DurationSeconds) }
	}
}

type parser struct {
	toks []token
	i    int
	loc  *time.Location
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func isWord(t token, word string) bool { return t.kind == tWord && strings.EqualFold(t.text, word) }

func isReserved(t token) bool {
	return isWord(t, "and") || isWord(t, "or") || isWord(t, "between") || isWord(t, "like")
}

// startsTerm says whether a token can begin a new term, which is how
// "distance > 5km pace < 6:20" is read as an "and" without the word.
func startsTerm(t token) bool {
	switch t.kind {
	case tString, tLParen:
		return true
	case tWord:
		return !isReserved(t)
	}
	return false
}

// or binds weakest, then and, then not: "a or b and c" is "a or (b and c)".
func (p *parser) parseOr(depth int) (node, error) {
	left, err := p.parseAnd(depth)
	if err != nil {
		return nil, err
	}
	for isWord(p.peek(), "or") {
		p.next()
		right, err := p.parseAnd(depth)
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
	return left, nil
}

func (p *parser) parseAnd(depth int) (node, error) {
	left, err := p.parseUnary(depth)
	if err != nil {
		return nil, err
	}
	for {
		switch t := p.peek(); {
		case isWord(t, "and"):
			p.next()
		case startsTerm(t):
		default:
			return left, nil
		}
		right, err := p.parseUnary(depth)
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
}

func (p *parser) parseUnary(depth int) (node, error) {
	if depth > maxDepth {
		return nil, &Error{Pos: p.peek().pos, Msg: "the query is nested too deeply"}
	}

	t := p.next()
	switch t.kind {
	case tEOF:
		return nil, &Error{Pos: t.pos, Msg: "the query ends too early: a term is missing"}

	case tLParen:
		inner, err := p.parseOr(depth + 1)
		if err != nil {
			return nil, err
		}
		if c := p.next(); c.kind != tRParen {
			return nil, &Error{Pos: t.pos, Msg: `a "(" was opened but never closed`}
		}
		return inner, nil

	case tRParen:
		return nil, &Error{Pos: t.pos, Msg: `unexpected ")"`}

	case tOp:
		return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf(`"%s" needs a field before it, for example: distance %s 5km`, t.text, t.text)}

	case tString:
		return textNode{t.text}, nil
	}

	// A word.
	switch {
	case isWord(t, "not"):
		inner, err := p.parseUnary(depth + 1)
		if err != nil {
			return nil, err
		}
		return notNode{inner}, nil

	case isReserved(t):
		return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf(`unexpected "%s" (put it in quotes to search for the word)`, t.text)}
	}

	if kind, ok := fieldNames[strings.ToLower(t.text)]; ok {
		return p.parseComparison(kind, t)
	}
	return textNode{t.text}, nil
}

func (p *parser) parseComparison(kind fieldKind, field token) (node, error) {
	t := p.peek()

	switch {
	case t.kind == tOp:
		p.next()
		op := normalizeOp(t.text)

		if op == "~" {
			op = "like"
		}
		return p.finishComparison(kind, field, op, t)

	case isWord(t, "like"):
		p.next()
		return p.finishComparison(kind, field, "like", t)

	case isWord(t, "between"):
		p.next()
		return p.parseBetween(kind, field, t)
	}

	return nil, &Error{Pos: field.pos, Msg: fmt.Sprintf(`"%s" needs a comparison after it, for example: %s`, field.text, example(kind))}
}

func normalizeOp(op string) string {
	switch op {
	case "==":
		return "="
	case "<>":
		return "!="
	}
	return op
}

func example(kind fieldKind) string {
	switch kind {
	case kindDistance:
		return "distance > 5km"
	case kindTime:
		return "time >= 30:00"
	case kindPace:
		return "pace < 6:20"
	case kindDate:
		return "date >= 2026-03"
	case kindSport:
		return "sport = running"
	default:
		return "device like forerunner"
	}
}

func (p *parser) finishComparison(kind fieldKind, field token, op string, opTok token) (node, error) {
	if kind.text() {
		if op != "=" && op != "!=" && op != "like" {
			return nil, &Error{Pos: opTok.pos, Msg: fmt.Sprintf(`"%s" can only be compared with =, != or like`, field.text)}
		}
		v := p.next()
		if v.kind != tWord && v.kind != tString {
			return nil, &Error{Pos: v.pos, Msg: fmt.Sprintf(`expected a name after "%s %s"`, field.text, opTok.text)}
		}
		get := func(s Subject) string { return s.Sport }
		if kind == kindDevice {
			get = func(s Subject) string { return s.Device }
		}
		return textCmp{get: get, op: op, value: v.text}, nil
	}

	if op == "like" {
		return nil, &Error{Pos: opTok.pos, Msg: fmt.Sprintf(`"like" only works on sport and device; for "%s" use > < >= <= = or between`, field.text)}
	}

	if kind == kindDate {
		start, end, pos, err := p.dateOperand()
		if err != nil {
			return nil, &Error{Pos: pos, Msg: err.Error()}
		}
		return dateCmp{op: op, start: start, end: end}, nil
	}

	value, pos, err := p.numberOperand(kind)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	return numCmp{get: kind.number(), op: op, value: value}, nil
}

func (p *parser) parseBetween(kind fieldKind, field token, betweenTok token) (node, error) {
	if kind.text() {
		return nil, &Error{Pos: betweenTok.pos, Msg: fmt.Sprintf(`"between" does not work on %s`, field.text)}
	}

	if kind == kindDate {
		firstStart, firstEnd, pos, err := p.dateOperand()
		if err != nil {
			return nil, &Error{Pos: pos, Msg: err.Error()}
		}
		if and := p.next(); !isWord(and, "and") {
			return nil, &Error{Pos: and.pos, Msg: `"between" needs "and": date between 2026-01 and 2026-03`}
		}
		secondStart, secondEnd, pos, err := p.dateOperand()
		if err != nil {
			return nil, &Error{Pos: pos, Msg: err.Error()}
		}
		// Both periods are included: from the start of the earlier one to the end of
		// the later one, whichever order they were written in.
		if secondStart.Before(firstStart) {
			return dateBetween{start: secondStart, end: firstEnd}, nil
		}
		return dateBetween{start: firstStart, end: secondEnd}, nil
	}

	lo, pos, err := p.numberOperand(kind)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	if and := p.next(); !isWord(and, "and") {
		return nil, &Error{Pos: and.pos, Msg: fmt.Sprintf(`"between" needs "and": %s between 5 and 10`, field.text)}
	}
	hi, pos, err := p.numberOperand(kind)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return numBetween{get: kind.number(), lo: lo, hi: hi}, nil
}

// operandToken reads the next token as a value. "5 km" is accepted as well as "5km".
func (p *parser) operandToken(joiners ...string) (text string, pos int, err error) {
	t := p.next()
	if t.kind != tWord && t.kind != tString {
		return "", t.pos, fmt.Errorf("expected a value here, but found %s", describe(t))
	}
	text = t.text

	if n := p.peek(); n.kind == tWord && numberRe.MatchString(text) {
		for _, j := range joiners {
			if strings.EqualFold(n.text, j) {
				p.next()
				text += j
				break
			}
		}
	}
	return text, t.pos, nil
}

func (p *parser) numberOperand(kind fieldKind) (value float64, pos int, err error) {
	switch kind {
	case kindDistance:
		text, pos, err := p.operandToken("km", "k", "m", "mi")
		if err != nil {
			return 0, pos, err
		}
		v, err := parseDistance(text)
		return v, pos, err

	case kindPace:
		text, pos, err := p.operandToken()
		if err != nil {
			return 0, pos, err
		}
		// "6:20 /km" is written with a space: accept the unit as a separate word.
		if n := p.peek(); n.kind == tWord && (strings.EqualFold(n.text, "/km") || strings.EqualFold(n.text, "min/km")) {
			p.next()
		}
		v, err := parsePace(text)
		return v, pos, err

	default: // kindTime
		text, pos, err := p.operandToken()
		if err != nil {
			return 0, pos, err
		}
		v, err := parseDuration(text)
		return v, pos, err
	}
}

func (p *parser) dateOperand() (start, end time.Time, pos int, err error) {
	text, pos, err := p.operandToken()
	if err != nil {
		return time.Time{}, time.Time{}, pos, err
	}
	start, end, err = parseDate(text, p.loc)
	return start, end, pos, err
}
