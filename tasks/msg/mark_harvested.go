package msg

import "github.com/dhamidi/k-si/runtime"

// "mark-harvested" — clear one HarvestJob once its post-finish effect has emitted
// all its logged directives (decision-013). It is the harvest's mark-email-sent:
// the pending marker clears only here, at the END of the effect, so a crash before
// it leaves the job pending for reconciliation to re-drive.
//
// It lives in the tasks contract package (not tasks-local) because the reply
// harvest's effect — assemble-reply — runs in the EMAIL module and clears its job
// cross-module, exactly as mint-ui-request emits register-ui-request back into
// tasks. Kind names which job to clear so a run's other pending kinds stay owed.
const MarkHarvested = "mark-harvested"

// The kinds of durable post-finish work a run reconciles (decision-013). They are
// part of the mark-harvested contract because the reply kind is cleared by the
// email module, so both sides must agree on the string.
const (
	HarvestKindMemory  = "memory"  // capture-memory: out/memory writes and forgets
	HarvestKindSkill   = "skill"   // store-skill: authored Agent Skills trees
	HarvestKindReply   = "reply"   // assemble-reply: the threaded email reply
	HarvestKindRequest = "request" // mint-ui-request: the Flow C web request/secret mint
)

type MarkHarvestedPayload struct {
	RunID int64  `json:"run_id"`
	Kind  string `json:"kind"`
}

func NewMarkHarvested(p MarkHarvestedPayload) runtime.Msg {
	return runtime.NewMsg(MarkHarvested, p)
}
