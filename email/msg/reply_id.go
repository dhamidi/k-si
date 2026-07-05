package msg

// ReplyMessageID is the Message-ID käsi puts on the reply for a given (task, run).
// Deterministic so tasks can pre-record it in the thread's References the moment it
// asks email to assemble the reply, and email stamps the same value when building it.
//
// Built by hand (no fmt) so this contract package stays a leaf that imports
// nothing but runtime (rules/contract-packages-are-leaves; docs/15). The output
// is identical to fmt.Sprintf("<reply-%d-%d@kasi.test>", taskID, runID).
func ReplyMessageID(taskID, runID int64) string {
	return "<reply-" + itoa(taskID) + "-" + itoa(runID) + "@kasi.test>"
}

// itoa renders a base-10 int64 without importing strconv/fmt.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
