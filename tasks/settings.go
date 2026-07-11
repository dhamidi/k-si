package tasks

import (
	"fmt"
	"net/mail"
	"strconv"

	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/settings"
	"github.com/dhamidi/k-si/tasks/msg"
)

// FromAddress is the deliverable reply-from identity as a setting value — a
// flag.Value validating one email address, formed by the default former (one
// text field). The state stays in tasks.Model.ReplyFrom; this wraps it so the
// settings surface can read and write it (docs/16).
type FromAddress string

func (f *FromAddress) Set(raw string) error {
	if _, err := mail.ParseAddress(raw); err != nil {
		return fmt.Errorf("must be a deliverable email address, e.g. kasi@decode.ee")
	}
	*f = FromAddress(raw)
	return nil
}

func (f FromAddress) String() string        { return string(f) }
func (f FromAddress) ToForm() settings.Form { return settings.FormOf(&f) }

// LoopGuard is the per-task run cap as a setting value — a non-negative int
// (0 disables the breaker), rendered as a number by the default former. Distinct
// from the Model.LoopGuard int field it wraps.
type LoopGuard int

func (l *LoopGuard) Set(raw string) error {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fmt.Errorf("must be a whole number ≥ 0 (0 disables the cap)")
	}
	*l = LoopGuard(n)
	return nil
}

func (l LoopGuard) String() string        { return strconv.Itoa(int(l)) }
func (l LoopGuard) ToForm() settings.Form { return settings.FormOf(&l) }

// LoopGuardOf reads the per-task run cap out of the model — the -Of suffix leaves
// the bare name to the type.
func LoopGuardOf(v runtime.View) int { return slice(v).LoopGuard }

// Settings is tasks' contribution to the settings surface (docs/16): the
// reply-from identity and the per-task run cap. Both flat leaves — no explicit
// ToForm shape, just the default former.
func Settings() []settings.Setting {
	return []settings.Setting{
		{
			Key:   "reply_from",
			Short: "Reply-from address",
			Long:  "The From address käsi sends replies as. It must be an address you can actually send mail from.",
			Owner: "tasks",
			Read:  func(v runtime.View) settings.Value { return FromAddress(ReplyFrom(v)) },
			Write: func(val settings.Value) []runtime.Msg {
				return []runtime.Msg{msg.NewSetReplyFrom(msg.SetReplyFromPayload{Address: string(val.(FromAddress))})}
			},
		},
		{
			Key:   "max_task_runs",
			Short: "Max agent runs per task",
			Long:  "If a task runs this many times without finishing, käsi pauses it so it can't loop forever. 0 turns this limit off.",
			Owner: "tasks",
			Read:  func(v runtime.View) settings.Value { return LoopGuard(LoopGuardOf(v)) },
			Write: func(val settings.Value) []runtime.Msg {
				return []runtime.Msg{msg.NewSetLoopGuard(msg.SetLoopGuardPayload{Max: int(val.(LoopGuard))})}
			},
		},
	}
}
