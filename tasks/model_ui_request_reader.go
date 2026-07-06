package tasks

import "github.com/dhamidi/k-si/runtime"

// RequestByRunID returns the UI request keyed by the raising run's id, if
// present — the exported point read the web edge uses to render and answer a
// request link (Flow C, decision-003). It mirrors Get: a typed read funnelled
// through the tasks slice, never a raw model reach.
func RequestByRunID(v runtime.View, runID int64) (UIRequest, bool) {
	m := slice(v)
	if i := m.findRequest(runID); i >= 0 {
		return m.Requests[i], true
	}
	return UIRequest{}, false
}
