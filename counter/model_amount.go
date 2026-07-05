package counter

import (
	"fmt"
	"strconv"
)

// Amount — how much to move the counter by. A rich value implementing the
// stdlib flag.Value contract: Set parses and validates (its error text is
// the form field's error message), String renders back (docs/15).
type Amount int64

func (a *Amount) Set(raw string) error {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return fmt.Errorf("must be a whole number, at least 1")
	}

	*a = Amount(n)
	return nil
}

func (a Amount) String() string { return strconv.FormatInt(int64(a), 10) }
