package posture

import (
	"fmt"
	"strings"
	"unicode"
)

// parseExpr parses one condition string into a node.
//
//	expr    := orExpr
//	orExpr  := andExpr ( "||" andExpr )*
//	andExpr := term ( "&&" term )*
//	term    := "(" expr ")" | comparison | membership
//	comparison := ATTR OP STRING
//	membership := ATTR "in" "[" STRING ( "," STRING )* "]"
func parseExpr(src string) (node, error) {
	p := &parser{toks: lex(src)}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected %q in %q", p.peek().val, src)
	}
	return n, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tAnd {
		p.next()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
	return left, nil
}

func (p *parser) parseTerm() (node, error) {
	if p.peek().kind == tLParen {
		p.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRParen {
			return nil, fmt.Errorf("missing )")
		}
		return n, nil
	}

	attrTok := p.next()
	if attrTok.kind != tIdent {
		return nil, fmt.Errorf("expected an attribute, got %q", attrTok.val)
	}
	attr := attrTok.val

	op := p.next()
	if op.kind == tIn {
		if p.next().kind != tLBracket {
			return nil, fmt.Errorf("expected [ after 'in'")
		}
		var vals []string
		for {
			s := p.next()
			if s.kind != tString {
				return nil, fmt.Errorf("expected a string in list, got %q", s.val)
			}
			vals = append(vals, s.val)
			sep := p.next()
			if sep.kind == tRBracket {
				break
			}
			if sep.kind != tComma {
				return nil, fmt.Errorf("expected , or ] in list, got %q", sep.val)
			}
		}
		return &inNode{attr: attr, vals: vals}, nil
	}

	if op.kind != tOp {
		return nil, fmt.Errorf("expected an operator after %q, got %q", attr, op.val)
	}
	val := p.next()
	if val.kind != tString {
		return nil, fmt.Errorf("expected a quoted value after %q, got %q", op.val, val.val)
	}
	return &cmpNode{attr: attr, op: op.val, val: val.val}, nil
}

// --- lexer ------------------------------------------------------------------

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tOp // == != > >= < <=
	tAnd
	tOr
	tIn
	tLParen
	tRParen
	tLBracket
	tRBracket
	tComma
	tErr
)

type token struct {
	kind tokKind
	val  string
}

func lex(src string) []token {
	var toks []token
	rs := []rune(src)
	i := 0
	for i < len(rs) {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != c {
				j++
			}
			toks = append(toks, token{tString, string(rs[i+1 : min(j, len(rs))])})
			i = j + 1
		case c == '(':
			toks = append(toks, token{tLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tRParen, ")"})
			i++
		case c == '[':
			toks = append(toks, token{tLBracket, "["})
			i++
		case c == ']':
			toks = append(toks, token{tRBracket, "]"})
			i++
		case c == ',':
			toks = append(toks, token{tComma, ","})
			i++
		case c == '&' && i+1 < len(rs) && rs[i+1] == '&':
			toks = append(toks, token{tAnd, "&&"})
			i += 2
		case c == '|' && i+1 < len(rs) && rs[i+1] == '|':
			toks = append(toks, token{tOr, "||"})
			i += 2
		case c == '=' || c == '!' || c == '<' || c == '>':
			if i+1 < len(rs) && rs[i+1] == '=' {
				toks = append(toks, token{tOp, string(c) + "="})
				i += 2
			} else if c == '<' || c == '>' {
				toks = append(toks, token{tOp, string(c)})
				i++
			} else {
				toks = append(toks, token{tErr, string(c)})
				i++
			}
		default:
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == ':' || rs[j] == '_' || rs[j] == '-' || rs[j] == '.') {
				j++
			}
			if j == i {
				toks = append(toks, token{tErr, string(c)})
				i++
				continue
			}
			word := string(rs[i:j])
			switch strings.ToLower(word) {
			case "in":
				toks = append(toks, token{tIn, "in"})
			case "and":
				toks = append(toks, token{tAnd, "&&"})
			case "or":
				toks = append(toks, token{tOr, "||"})
			default:
				toks = append(toks, token{tIdent, word})
			}
			i = j
		}
	}
	toks = append(toks, token{tEOF, ""})
	return toks
}
