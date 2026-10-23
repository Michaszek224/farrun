package query

import (
	"strings"
	"unicode"
)

type tokKind int

const (
	tEOF tokKind = iota
	tWord
	tString // a "quoted string": never a keyword or a field name
	tOp
	tLParen
	tRParen
)

type token struct {
	kind tokKind
	text string
	pos  int
}

func describe(t token) string {
	switch t.kind {
	case tEOF:
		return "the end of the query"
	case tString:
		return `"` + t.text + `"`
	default:
		return `"` + t.text + `"`
	}
}

// isOpRune are the characters that end a word, because they start an operator.
func isOpRune(r rune) bool { return strings.ContainsRune("<>=!~", r) }

func lex(input string) ([]token, error) {
	var toks []token
	rs := []rune(input)
	// pos is reported as a rune index; for the messages here that is close enough
	// to a byte offset, and exact for ASCII, which queries nearly always are.
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++

		case r == '(':
			toks = append(toks, token{tLParen, "(", i})
			i++

		case r == ')':
			toks = append(toks, token{tRParen, ")", i})
			i++

		case r == '"':
			start := i
			i++
			var b strings.Builder
			for i < len(rs) && rs[i] != '"' {
				b.WriteRune(rs[i])
				i++
			}
			if i >= len(rs) {
				return nil, &Error{Pos: start, Msg: `a quote was opened but never closed`}
			}
			i++ // closing quote
			toks = append(toks, token{tString, b.String(), start})

		case isOpRune(r):
			start := i
			op := string(r)
			if i+1 < len(rs) {
				switch two := string(rs[i : i+2]); two {
				case "<=", ">=", "<>", "!=", "==":
					op = two
				}
			}
			if op == "!" {
				return nil, &Error{Pos: start, Msg: `"!" must be written "!="`}
			}
			i += len([]rune(op))
			toks = append(toks, token{tOp, op, start})

		default:
			start := i
			for i < len(rs) && !unicode.IsSpace(rs[i]) && rs[i] != '(' && rs[i] != ')' && rs[i] != '"' && !isOpRune(rs[i]) {
				i++
			}
			toks = append(toks, token{tWord, string(rs[start:i]), start})
		}
	}
	toks = append(toks, token{tEOF, "", len(rs)})
	return toks, nil
}
