package msg

// NotifyMessageID is the Message-ID käsi puts on a mid-run notification mail for a
// given (task, seq) on the given domain. Unlike a reply — one per run — a run may
// send several notifications, so the sequence number (the log offset of the
// notify-user message) makes each one unique within the run and across the thread.
//
// Built by hand (no fmt) so this contract package stays a leaf that imports
// nothing but runtime (rules/contract-packages-are-leaves; docs/15). The output is
// identical to fmt.Sprintf("<notify-%d-%d@%s>", taskID, seq, domain).
func NotifyMessageID(taskID, seq int64, domain string) string {
	return "<notify-" + itoa(taskID) + "-" + itoa(seq) + "@" + domain + ">"
}
