// Package testlang is the parser and evaluator for käsi's test scripts
// (docs/14). It knows nothing about käsi: it parses words, blocks, and
// substitutions, and hands commands to a vocabulary — mirroring how runtime/
// knows nothing about email.
package testlang

import (
	"fmt"
	"strings"
)

// Command is one parsed command: its words, its line, and the narration
// comment nearest above it — which the runner quotes when the command fails
// (docs/14).
type Command struct {
	Line      int
	Narration string
	Words     []Word
}

// Word is one argument: a sequence of segments concatenated at eval time. A
// braced word is a single literal segment, substitutions deferred.
type Word struct {
	Braced bool
	Segs   []Seg
}

// Seg is a word fragment: a literal, a $variable, or a [command] whose
// result substitutes in.
type Seg struct {
	Lit string
	Var string
	Sub []Command
}

// Parse turns script source into commands. The grammar is all of docs/14:
// words, newline/; termination, # comments, $var, [cmd], {literal}, "interp".
func Parse(src string) ([]Command, error) {
	p := &parser{src: src, line: 1}
	cmds, err := p.commands(eofEnds)
	if err != nil {
		return nil, err
	}
	return cmds, nil
}

type parser struct {
	src  string
	pos  int
	line int
}

type terminator int

const (
	eofEnds terminator = iota
	bracketEnds
)

func (p *parser) commands(until terminator) ([]Command, error) {
	var cmds []Command
	narration := ""

	for {
		p.skipBlank(&narration)

		if p.done() {
			if until == bracketEnds {
				return nil, p.errorf("missing closing ]")
			}
			return cmds, nil
		}

		if until == bracketEnds && p.peek() == ']' {
			p.pos++
			return cmds, nil
		}

		if p.peek() == '#' {
			narration = p.comment()
			continue
		}

		cmd, err := p.command(until)
		if err != nil {
			return nil, err
		}

		if len(cmd.Words) > 0 {
			cmd.Narration = narration
			cmds = append(cmds, cmd)
		}
	}
}

func (p *parser) command(until terminator) (Command, error) {
	cmd := Command{Line: p.line}

	for {
		p.skipSpaces()

		if p.done() {
			return cmd, nil
		}

		switch c := p.peek(); {
		case c == '\n':
			p.pos++
			p.line++
			return cmd, nil
		case c == ';':
			p.pos++
			return cmd, nil
		case c == ']' && until == bracketEnds:
			return cmd, nil
		case c == '#':
			// A '#' at a word boundary starts a comment to end of line — whether
			// the command is empty (a narration line) or trailing an assertion
			// (docs/14 shows `task 1 status is done   # ...`). The outer loop
			// consumes the comment text; a '#' *inside* a word (a URL, say) has no
			// preceding space and is handled by word() as a literal.
			return cmd, nil
		default:
			w, err := p.word(until)
			if err != nil {
				return Command{}, err
			}
			cmd.Words = append(cmd.Words, w)
		}
	}
}

func (p *parser) word(until terminator) (Word, error) {
	if p.peek() == '{' {
		lit, err := p.braced()
		if err != nil {
			return Word{}, err
		}
		return Word{Braced: true, Segs: []Seg{{Lit: lit}}}, nil
	}

	if p.peek() == '"' {
		return p.quoted()
	}

	var segs []Seg
	var lit strings.Builder

	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, Seg{Lit: lit.String()})
			lit.Reset()
		}
	}

	for !p.done() {
		c := p.peek()

		if c == ' ' || c == '\t' || c == '\n' || c == ';' {
			break
		}
		if c == ']' && until == bracketEnds {
			break
		}

		switch c {
		case '$':
			flush()
			p.pos++
			segs = append(segs, Seg{Var: p.name()})
		case '[':
			flush()
			p.pos++
			sub, err := p.commands(bracketEnds)
			if err != nil {
				return Word{}, err
			}
			segs = append(segs, Seg{Sub: sub})
		default:
			lit.WriteByte(c)
			p.pos++
		}
	}

	flush()
	return Word{Segs: segs}, nil
}

func (p *parser) quoted() (Word, error) {
	p.pos++ // opening quote

	var segs []Seg
	var lit strings.Builder

	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, Seg{Lit: lit.String()})
			lit.Reset()
		}
	}

	for {
		if p.done() {
			return Word{}, p.errorf(`missing closing "`)
		}

		switch c := p.peek(); c {
		case '"':
			p.pos++
			flush()
			if len(segs) == 0 {
				segs = []Seg{{Lit: ""}}
			}
			return Word{Segs: segs}, nil
		case '$':
			flush()
			p.pos++
			segs = append(segs, Seg{Var: p.name()})
		case '[':
			flush()
			p.pos++
			sub, err := p.commands(bracketEnds)
			if err != nil {
				return Word{}, err
			}
			segs = append(segs, Seg{Sub: sub})
		case '\n':
			return Word{}, p.errorf(`missing closing "`)
		default:
			lit.WriteByte(c)
			p.pos++
		}
	}
}

func (p *parser) braced() (string, error) {
	p.pos++ // opening brace
	start := p.pos
	depth := 1

	for !p.done() {
		switch p.peek() {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				body := p.src[start:p.pos]
				p.pos++
				p.line += strings.Count(body, "\n")
				return body, nil
			}
		case '\n':
			// counted on close, so errors report the opening line
		}
		p.pos++
	}

	return "", p.errorf("missing closing }")
}

func (p *parser) comment() string {
	start := p.pos
	for !p.done() && p.peek() != '\n' {
		p.pos++
	}
	return strings.TrimSpace(strings.TrimPrefix(p.src[start:p.pos], "#"))
}

func (p *parser) name() string {
	start := p.pos
	for !p.done() {
		c := p.peek()
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			p.pos++
			continue
		}
		break
	}
	return p.src[start:p.pos]
}

func (p *parser) skipSpaces() {
	for !p.done() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

func (p *parser) skipBlank(narration *string) {
	for !p.done() {
		switch p.peek() {
		case ' ', '\t', ';':
			p.pos++
		case '\n':
			p.pos++
			p.line++
		default:
			return
		}
	}
	_ = narration
}

func (p *parser) done() bool { return p.pos >= len(p.src) }

func (p *parser) peek() byte { return p.src[p.pos] }

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.line, fmt.Sprintf(format, args...))
}
