package testlang

import (
	"fmt"
	"strings"
)

// CommandFunc is one vocabulary entry. Reads return their value; stimuli
// return "". The interpreter is deliberately dumb — no procedures, no loops,
// no conditionals; abstraction lives in the vocabulary (docs/14).
type CommandFunc func(in *Interp, args []string) (string, error)

// Interp evaluates parsed commands against a vocabulary and variables.
type Interp struct {
	Vocabulary map[string]CommandFunc
	Vars       map[string]string

	// Line and Narration track the command being evaluated, for failures.
	Line      int
	Narration string
}

// New builds an interpreter with the built-in `set` and an empty vocabulary
// for the runner to fill.
func New() *Interp {
	in := &Interp{Vocabulary: map[string]CommandFunc{}, Vars: map[string]string{}}

	in.Vocabulary["set"] = func(in *Interp, args []string) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("set needs a name and a value")
		}
		in.Vars[args[0]] = args[1]
		return args[1], nil
	}

	return in
}

// Run evaluates a whole script.
func (in *Interp) Run(cmds []Command) error {
	_, err := in.eval(cmds)
	return err
}

// eval runs commands in order, returning the last command's result — the
// value of a [ ... ] substitution.
func (in *Interp) eval(cmds []Command) (string, error) {
	result := ""

	for _, cmd := range cmds {
		words, err := in.words(cmd)
		if err != nil {
			return "", err
		}
		if len(words) == 0 {
			continue
		}

		fn, ok := in.Vocabulary[words[0]]
		if !ok {
			return "", &Failure{Line: cmd.Line, Narration: cmd.Narration,
				Message: fmt.Sprintf("unknown command %q", words[0])}
		}

		in.Line, in.Narration = cmd.Line, cmd.Narration

		result, err = fn(in, words[1:])
		if err != nil {
			if failure, ok := err.(*Failure); ok {
				return "", failure
			}
			return "", &Failure{Line: cmd.Line, Narration: cmd.Narration, Message: err.Error()}
		}
	}

	return result, nil
}

func (in *Interp) words(cmd Command) ([]string, error) {
	var out []string

	for _, w := range cmd.Words {
		var b strings.Builder

		for _, seg := range w.Segs {
			switch {
			case seg.Var != "":
				b.WriteString(in.Vars[seg.Var])
			case seg.Sub != nil:
				value, err := in.eval(seg.Sub)
				if err != nil {
					return nil, err
				}
				b.WriteString(value)
			default:
				b.WriteString(seg.Lit)
			}
		}

		out = append(out, b.String())
	}

	return out, nil
}

// EvalWord evaluates a single word — vocabulary commands that take blocks
// (send's payload builder) use it to re-evaluate block contents with
// substitution while still seeing which words were braced.
func (in *Interp) EvalWord(w Word) (string, error) {
	var b strings.Builder

	for _, seg := range w.Segs {
		switch {
		case seg.Var != "":
			b.WriteString(in.Vars[seg.Var])
		case seg.Sub != nil:
			value, err := in.eval(seg.Sub)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
		default:
			b.WriteString(seg.Lit)
		}
	}

	return b.String(), nil
}

// Failure is a script failure with its story context: the line, the
// narration comment it falls under, and the message (docs/14).
type Failure struct {
	Line      int
	Narration string
	Message   string
}

func (f *Failure) Error() string {
	if f.Narration == "" {
		return fmt.Sprintf("line %d: %s", f.Line, f.Message)
	}
	return fmt.Sprintf("line %d: in %q: %s", f.Line, f.Narration, f.Message)
}
