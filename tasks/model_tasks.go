package tasks

import "github.com/dhamidi/k-si/runtime"

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
	// HarvestPending is the set of finished runs whose memory harvest is still
	// owed — the crash-safety marker (the memory sibling of email's pending outbox
	// entry, docs/03). agent-run-finished appends a job; the harvest-reconcile
	// subscription drives capture-memory for each; mark-harvested removes it once
	// the effect has emitted all its remember/forget directives. A SLICE, never a
	// map, so it marshals in a deterministic order and refolding the log converges
	// byte-for-byte on the live model (BRIEF replay-convergence).
	HarvestPending []HarvestJob `json:"harvest_pending"`
}

// HarvestJob is one finished run whose out/memory harvest has not yet been
// captured — the pending-work marker the reconcile subscription turns back into a
// capture-memory effect until mark-harvested clears it.
type HarvestJob struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

// withHarvestPending returns the pending set with job appended (copy-on-write),
// skipping a duplicate RunID so a re-fold of the same agent-run-finished — or a
// second finish for one run — never doubles the marker.
func withHarvestPending(pending []HarvestJob, job HarvestJob) []HarvestJob {
	for _, j := range pending {
		if j.RunID == job.RunID {
			return pending
		}
	}
	return append(append([]HarvestJob(nil), pending...), job)
}

// withoutHarvestPending returns the pending set with the job for runID removed
// (copy-on-write). An absent runID is a no-op, so mark-harvested is idempotent.
func withoutHarvestPending(pending []HarvestJob, runID int64) []HarvestJob {
	var out []HarvestJob
	for _, j := range pending {
		if j.RunID == runID {
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
