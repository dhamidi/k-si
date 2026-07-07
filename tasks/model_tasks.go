package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/tasks/msg"
)

// Model is the tasks slice of the application model (docs/15). It is a SLICE of
// tasks, never a map — a slice marshals in a deterministic order so refolding
// the log converges byte-for-byte on the live model (BRIEF replay-convergence).
type Model struct {
	Tasks []Task `json:"tasks"`
	// Requests are the UI requests agents raised (Flow C), keyed by the raising
	// run's id. A slice, never a map, for deterministic replay (decision-002).
	Requests []UIRequest `json:"requests"`
	// ReplyFrom is the deliverable From address replies are sent as — configured
	// once via set-reply-from (docs/04). Empty falls back to the routeAddr
	// placeholder, which is fine for the sim ring but not for real delivery.
	ReplyFrom string `json:"reply_from"`
	// HarvestPending is the set of post-finish jobs a run still owes — the
	// crash-safety marker (the sibling of email's pending outbox entry, docs/03).
	// agent-run-finished appends one job per KIND of durable post-finish work
	// (memory, skill, reply); the harvest-reconcile subscription drives the matching
	// effect for each; mark-harvested removes a job once its effect has emitted all
	// its logged directives. A SLICE, never a map, so it marshals in a deterministic
	// order and refolding the log converges byte-for-byte on the live model
	// (BRIEF replay-convergence).
	HarvestPending []HarvestJob `json:"harvest_pending"`
}

// The kinds of durable post-finish work a HarvestJob reconciles — aliases of the
// mark-harvested contract constants (tasks/msg), the single source of truth both
// this module and email share. Each is driven by run-harvest to a distinct effect
// and cleared by a matching mark-harvested (decision-013).
const (
	HarvestMemory     = msg.HarvestKindMemory     // capture-memory: the run's out/memory writes and forgets
	HarvestSkill      = msg.HarvestKindSkill      // store-skill: the run's authored Agent Skills trees
	HarvestReply      = msg.HarvestKindReply      // assemble-reply: the run's threaded email reply
	HarvestRequest    = msg.HarvestKindRequest    // mint-ui-request: the run's Flow C web request/secret mint
	HarvestTranscript = msg.HarvestKindTranscript // capture-transcript: the run's session transcript
)

// HarvestJob is one KIND of post-finish work a finished run still owes — the
// pending-work marker the reconcile subscription turns back into an effect until
// mark-harvested clears it. Identity is (RunID, Kind): a single run owes at most
// one job per kind, and the kinds are reconciled independently.
type HarvestJob struct {
	TaskID int64  `json:"task_id"`
	RunID  int64  `json:"run_id"`
	Kind   string `json:"kind"`
}

// withHarvestPending returns the pending set with job appended (copy-on-write),
// skipping a duplicate (RunID, Kind) so a re-fold of the same agent-run-finished —
// or a second finish for one run — never doubles the marker.
func withHarvestPending(pending []HarvestJob, job HarvestJob) []HarvestJob {
	for _, j := range pending {
		if j.RunID == job.RunID && j.Kind == job.Kind {
			return pending
		}
	}
	return append(append([]HarvestJob(nil), pending...), job)
}

// withoutHarvestPending returns the pending set with the (runID, kind) job removed
// (copy-on-write). An absent job is a no-op, so mark-harvested is idempotent. Only
// the matching kind clears, so a run's other pending kinds are left owed.
func withoutHarvestPending(pending []HarvestJob, runID int64, kind string) []HarvestJob {
	var out []HarvestJob
	for _, j := range pending {
		if j.RunID == runID && j.Kind == kind {
			continue
		}
		out = append(out, j)
	}
	return out
}

// slice reads the tasks Model out of a whole-model View — the typed accessor
// every exported read helper funnels through (docs/15).
func slice(v runtime.View) Model {
	return runtime.Slice[Model](v, "tasks")
}

// All returns a copy of the model's tasks in model order — the list read the
// browse UI groups and sorts (docs/08). It mirrors Get: a typed read funnelled
// through the tasks slice, never a raw model reach. The copy keeps the caller
// from aliasing model-owned memory; grouping/newest-first ordering is the
// view's job, not the model's.
func All(v runtime.View) []Task {
	m := slice(v)
	out := make([]Task, len(m.Tasks))
	copy(out, m.Tasks)
	return out
}

// find returns the index of the task with id in the slice, or -1.
func (m Model) find(id TaskID) int {
	for i := range m.Tasks {
		if m.Tasks[i].ID == id {
			return i
		}
	}
	return -1
}

// findRequest returns the index of the UI request keyed by runID, or -1.
func (m Model) findRequest(runID int64) int {
	for i := range m.Requests {
		if m.Requests[i].RunID == runID {
			return i
		}
	}
	return -1
}
