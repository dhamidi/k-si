package agents

import (
	"fmt"
	"strconv"

	"github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
)

// MaxConcurrent is the concurrent-run cap as a setting value — a non-negative
// int (0 = unlimited), rendered as a number by the default former. It wraps the
// Model.MaxConcurrent int field (docs/16, decision-016 the OOM breaker).
type MaxConcurrent int

func (m *MaxConcurrent) Set(raw string) error {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fmt.Errorf("must be a whole number ≥ 0 (0 = unlimited)")
	}
	*m = MaxConcurrent(n)
	return nil
}

func (m MaxConcurrent) String() string        { return strconv.Itoa(int(m)) }
func (m MaxConcurrent) ToForm() settings.Form { return settings.FormOf(&m) }

// MaxConcurrentOf reads the concurrent-run cap out of the model.
func MaxConcurrentOf(v runtime.View) int { return slice(v).MaxConcurrent }

// Settings is agents' contribution to the settings surface (docs/16): the
// concurrent-run cap. A flat leaf — the default former gives it a number field.
func Settings() []settings.Setting {
	return []settings.Setting{
		{
			Key:   "max_concurrent_runs",
			Short: "Max concurrent agent runs",
			Long:  "How many tasks käsi works on at once; the rest wait their turn. Keeping this low protects the machine's memory. 0 means no limit.",
			Owner: "agents",
			Read:  func(v runtime.View) settings.Value { return MaxConcurrent(MaxConcurrentOf(v)) },
			Write: func(val settings.Value) []runtime.Msg {
				return []runtime.Msg{msg.NewSetMaxConcurrentRuns(msg.SetMaxConcurrentRunsPayload{Max: int(val.(MaxConcurrent))})}
			},
		},
	}
}
