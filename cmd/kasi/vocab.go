package main

// The domain test vocabulary (docs/14): stimuli (deliver, agent, click, fail)
// and reads (outbound, outbox, task, tasks, archive), plus fixture. Each command
// drives or observes the sim world assembled in simworld.go — the mail edge, the
// harness, the workspace, the content tables — exactly as production's real
// edges would be driven by the outside world. Stimuli settle the instance before
// returning, so the next script line sees a stable model (docs/13).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhamidi/k-si/agents"
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/mime"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/testlang"
)

func registerDomainVocabulary(in *testlang.Interp, inst *instance) {
	v := in.Vocabulary

	v["fixture"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("fixture needs a path under t/fixtures/")
		}
		b, err := os.ReadFile(filepath.Join("t", "fixtures", args[0]))
		if err != nil {
			return "", fmt.Errorf("fixture %s: %w", args[0], err)
		}
		return string(b), nil
	}

	v["deliver"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("deliver needs a block { from … to … }")
		}
		return "", deliver(in, inst, args[0])
	}

	v["agent"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("agent needs a block { out <file> <content> … }")
		}
		return "", agentTurn(in, inst, args[0])
	}

	v["stop"] = func(in *testlang.Interp, args []string) (string, error) {
		return "", stopAgent(inst)
	}

	v["outbound"] = func(in *testlang.Interp, args []string) (string, error) {
		return outbound(inst, args)
	}

	v["outbox"] = func(in *testlang.Interp, args []string) (string, error) {
		return outbox(inst, args)
	}

	v["task"] = func(in *testlang.Interp, args []string) (string, error) {
		return taskRead(inst, args)
	}

	v["tasks"] = func(in *testlang.Interp, args []string) (string, error) {
		read, verb := splitVerb(args)
		if len(read) == 1 && read[0] == "count" {
			ts, err := taskList(inst)
			if err != nil {
				return "", err
			}
			return finishRead("tasks count", strconv.Itoa(len(ts)), verb)
		}
		return "", fmt.Errorf("tasks: only `tasks count` is supported (use `task <n> …`)")
	}

	v["archive"] = func(in *testlang.Interp, args []string) (string, error) {
		return archiveRead(inst, args)
	}

	v["click"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("click needs a capability URL")
		}
		return "", click(inst, args[0])
	}

	v["fail"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) < 2 {
			return "", fmt.Errorf("fail needs an edge and an op, e.g. `fail mail send`")
		}
		n := 1
		if len(args) >= 3 {
			parsed, err := strconv.Atoi(args[2])
			if err != nil {
				return "", fmt.Errorf("fail: N must be a number, got %q", args[2])
			}
			n = parsed
		}
		switch args[0] {
		case "mail":
			inst.world.mail.FailNext(args[1], n)
		default:
			return "", fmt.Errorf("fail: unknown edge %q (mail)", args[0])
		}
		return "", nil
	}
}

// --- deliver -----------------------------------------------------------------

type inboundMail struct {
	from        string
	to          string
	cc          []string
	subject     string
	body        string
	attachments []mime.Part
	replyToLast bool
	inReplyTo   string
	references  []string

	// rawPath, when set by a `raw <file>` line, injects verbatim RFC-5322 bytes
	// (a captured real message) instead of building MIME from the fields above.
	// It is mutually exclusive with every structured field.
	rawPath    string
	structured bool
}

func deliver(in *testlang.Interp, inst *instance, block string) error {
	m, err := parseDeliverBlock(in, block)
	if err != nil {
		return err
	}

	if m.rawPath != "" {
		return deliverRaw(inst, m.rawPath)
	}

	if m.replyToLast {
		if err := applyReplyToLast(inst, &m); err != nil {
			return err
		}
	}

	inst.world.inboundSeq++
	messageID := fmt.Sprintf("<in-%d@example.test>", inst.world.inboundSeq)

	hdr := map[string][]string{
		"From":       {m.from},
		"To":         {m.to},
		"Subject":    {m.subject},
		"Message-ID": {messageID},
	}
	if len(m.cc) > 0 {
		hdr["Cc"] = []string{strings.Join(m.cc, ", ")}
	}
	if m.inReplyTo != "" {
		hdr["In-Reply-To"] = []string{m.inReplyTo}
	}
	if len(m.references) > 0 {
		hdr["References"] = []string{strings.Join(m.references, " ")}
	}

	raw, err := mime.Build(hdr, m.body, m.attachments)
	if err != nil {
		return fmt.Errorf("deliver: build MIME: %w", err)
	}

	inboxID, err := inst.world.mail.Deliver(raw)
	if err != nil {
		return fmt.Errorf("deliver: %w", err)
	}

	inst.app.Send(email.NewRouteEmail(email.RouteEmailPayload{
		InboxID:    inboxID,
		Recipient:  m.to,
		Sender:     m.from,
		Cc:         m.cc,
		Subject:    m.subject,
		MessageID:  messageID,
		InReplyTo:  m.inReplyTo,
		References: m.references,
	}))
	inst.app.Settle()
	return nil
}

// deliverRaw injects verbatim RFC-5322 bytes from a file, mirroring serve.route():
// store the raw message as an inbox row, then route-email it with the headers read
// straight off the wire (docs/04). This lets a scenario replay a captured real
// message rather than one built from `deliver` fields.
func deliverRaw(inst *instance, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("deliver: raw %s: %w", path, err)
	}
	msg, err := mime.Parse(raw)
	if err != nil {
		return fmt.Errorf("deliver: raw: parse: %w", err)
	}
	inboxID, err := inst.world.mail.Deliver(raw)
	if err != nil {
		return fmt.Errorf("deliver: %w", err)
	}
	inst.app.Send(email.NewRouteEmail(email.RouteEmailPayload{
		InboxID:    inboxID,
		Recipient:  msg.Header.Get("To"),
		Sender:     firstAddr(msg.Header.Get("From")),
		Cc:         mime.CcList(msg.Header.Get("Cc")),
		Subject:    msg.Header.Get("Subject"),
		MessageID:  msg.Header.Get("Message-ID"),
		InReplyTo:  msg.Header.Get("In-Reply-To"),
		References: strings.Fields(msg.Header.Get("References")),
	}))
	inst.app.Settle()
	return nil
}

func parseDeliverBlock(in *testlang.Interp, block string) (inboundMail, error) {
	var m inboundMail
	lines, err := blockLines(in, block)
	if err != nil {
		return m, err
	}
	for _, words := range lines {
		switch words[0] {
		case "raw":
			if len(words) != 2 {
				return m, fmt.Errorf("deliver: raw needs a single file path")
			}
			m.rawPath = words[1]
		case "from":
			m.structured = true
			m.from = arg(words)
		case "to":
			m.structured = true
			m.to = arg(words)
		case "cc":
			m.structured = true
			m.cc = append(m.cc, strings.Fields(strings.Join(words[1:], " "))...)
		case "subject":
			m.structured = true
			m.subject = arg(words)
		case "body":
			m.structured = true
			m.body = arg(words)
		case "attach":
			m.structured = true
			if len(words) != 3 {
				return m, fmt.Errorf("deliver: attach needs a filename and content")
			}
			m.attachments = append(m.attachments, mime.Part{
				Filename:    words[1],
				ContentType: contentTypeFor(words[1]),
				Bytes:       []byte(words[2]),
			})
		case "reply-to-last":
			m.structured = true
			m.replyToLast = true
		default:
			return m, fmt.Errorf("deliver: unknown field %q", words[0])
		}
	}
	if m.rawPath != "" && m.structured {
		return m, fmt.Errorf("deliver: raw is mutually exclusive with from/to/cc/subject/body/attach/reply-to-last")
	}
	return m, nil
}

// applyReplyToLast threads a reply onto the most recent outbound message, the
// way a mail client's reply would: To becomes our route address, In-Reply-To and
// References carry the outbound's identity so route-email matches the task.
func applyReplyToLast(inst *instance, m *inboundMail) error {
	// Read the messages the module actually sent — the outbound edge, which is
	// SimMail in the sim ring but RecordedMail/RecordingMail in the recorded and
	// live rings (where world.mail stays the inbound-injection twin).
	sent := inst.world.outbound.(sentMailer).Sent()
	if len(sent) == 0 {
		return fmt.Errorf("deliver: reply-to-last, but nothing has been sent")
	}
	last, err := mime.Parse(sent[len(sent)-1])
	if err != nil {
		return fmt.Errorf("deliver: reply-to-last: parse last outbound: %w", err)
	}
	m.to = last.Header.Get("From")
	m.inReplyTo = last.Header.Get("Message-ID")
	m.references = strings.Fields(last.Header.Get("References"))
	if m.subject == "" {
		m.subject = last.Header.Get("Subject")
	}
	return nil
}

// --- agent -------------------------------------------------------------------

func agentTurn(in *testlang.Interp, inst *instance, block string) error {
	lines, err := blockLines(in, block)
	if err != nil {
		return err
	}

	var out []mime.Part
	exit := 0
	for _, words := range lines {
		switch words[0] {
		case "out":
			if len(words) != 3 {
				return fmt.Errorf("agent: out needs a filename and content")
			}
			out = append(out, mime.Part{
				Filename:    words[1],
				ContentType: contentTypeFor(words[1]),
				Bytes:       []byte(words[2]),
			})
		case "exit":
			exit, err = strconv.Atoi(arg(words))
			if err != nil {
				return fmt.Errorf("agent: exit needs a code, got %q", arg(words))
			}
		default:
			return fmt.Errorf("agent: unknown field %q", words[0])
		}
	}

	running := agents.RunningRuns(inst.app.View())
	if len(running) == 0 {
		return fmt.Errorf("agent: no agent run is currently running")
	}
	taskID := running[0].TaskID

	// The dispatch branches per ring (docs/13, docs/14). In sim the block's
	// out/exit ARE the turn. In recorded and live the block is parsed only to
	// error uniformly on malformed input — the cassette (recorded) or the real
	// agent (live) is authoritative, so out/exit go unused.
	switch inst.world.ring {
	case "sim":
		if err := inst.world.sim.DeliverTurn(taskID, out, exit); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
	case "recorded":
		if err := inst.world.recorded.DeliverRecorded(taskID); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
	case "live":
		deadline := time.Now().Add(180 * time.Second)
		for {
			still := false
			for _, r := range agents.RunningRuns(inst.app.View()) {
				if r.TaskID == taskID {
					still = true
				}
			}
			if !still {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("agent: live run for task %d did not finish within 180s", taskID)
			}
			time.Sleep(100 * time.Millisecond)
		}
	default:
		return fmt.Errorf("agent: unknown ring %q", inst.world.ring)
	}

	inst.app.Settle()
	return nil
}

// stopAgent signals the currently-running agent run to stop — the web Stop button
// / supervisor path (docs/05). One stimulus, the same across rings: sim cancels
// the blocked SimHarness, live SIGTERMs the real process, recorded returns Stopped
// from the cassette. A stopped run yields no reply; the task returns to the human.
func stopAgent(inst *instance) error {
	running := agents.RunningRuns(inst.app.View())
	if len(running) == 0 {
		return fmt.Errorf("stop: no agent run is currently running")
	}
	r := running[0]
	inst.app.Send(agentmsg.NewStopAgentRun(agentmsg.StopAgentRunPayload{
		TaskID: r.TaskID,
		RunID:  int64(r.ID),
	}))
	inst.app.Settle()
	return nil
}

// --- outbound (mail the sim edge has sent) -----------------------------------

// sentMailer is the sliver of the Mail edge the `outbound` read needs: the log of
// transmitted messages. SimMail, RecordedMail and RecordingMail all satisfy it, so
// the read works against whichever edge the ring wired into email.Edges (docs/13).
type sentMailer interface{ Sent() [][]byte }

func outbound(inst *instance, args []string) (string, error) {
	read, verb := splitVerb(args)
	if len(read) == 0 {
		return "", fmt.Errorf("outbound needs last|N|count [field]")
	}
	sent := inst.world.outbound.(sentMailer).Sent()

	if read[0] == "count" {
		return finishRead("outbound count", strconv.Itoa(len(sent)), verb)
	}

	idx, err := selectorIndex(read[0], len(sent))
	if err != nil {
		return "", fmt.Errorf("outbound %s: %w", read[0], err)
	}
	msg, err := mime.Parse(sent[idx])
	if err != nil {
		return "", fmt.Errorf("outbound: parse: %w", err)
	}

	field := ""
	if len(read) > 1 {
		field = read[1]
	}

	value, err := outboundField(msg, field)
	if err != nil {
		return "", err
	}
	return finishRead("outbound "+strings.Join(read, " "), value, verb)
}

func outboundField(msg mime.Message, field string) (string, error) {
	switch field {
	case "", "raw":
		return string(msg.Raw), nil
	case "from":
		return strings.Join(mime.CcList(msg.Header.Get("From")), " "), nil
	case "to":
		return strings.Join(mime.CcList(msg.Header.Get("To")), " "), nil
	case "cc":
		return strings.Join(mime.CcList(msg.Header.Get("Cc")), " "), nil
	case "subject":
		return msg.Header.Get("Subject"), nil
	case "body":
		return msg.Text, nil
	case "attachments":
		names := make([]string, 0, len(msg.Parts))
		for _, p := range msg.Parts {
			names = append(names, p.Filename)
		}
		return strings.Join(names, " "), nil
	case "completion-link":
		return completionLink(msg.Text), nil
	default:
		return "", fmt.Errorf("outbound: unknown field %q", field)
	}
}

// completionLink pulls the capability URL out of a reply body (docs/04).
func completionLink(body string) string {
	for _, tok := range strings.FieldsFunc(body, func(r rune) bool { return r == ' ' || r == '\n' || r == '\t' || r == '\r' }) {
		if strings.HasPrefix(tok, "https://") || strings.HasPrefix(tok, "http://") {
			return strings.TrimRight(tok, ".,)")
		}
	}
	return ""
}

// --- outbox (email's model of the send queue) --------------------------------

func outbox(inst *instance, args []string) (string, error) {
	read, verb := splitVerb(args)
	if len(read) < 2 {
		return "", fmt.Errorf("outbox needs last|N and a field")
	}
	raw, err := inst.app.ModelJSON("email")
	if err != nil {
		return "", err
	}
	var m struct {
		Outbox []map[string]any `json:"outbox"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	idx, err := selectorIndex(read[0], len(m.Outbox))
	if err != nil {
		return "", fmt.Errorf("outbox %s: %w", read[0], err)
	}
	value, ok := m.Outbox[idx][read[1]]
	if !ok {
		return "", fmt.Errorf("outbox: no field %q", read[1])
	}
	return finishRead("outbox "+strings.Join(read, " "), render(value), verb)
}

// --- task / tasks ------------------------------------------------------------

func taskRead(inst *instance, args []string) (string, error) {
	read, verb := splitVerb(args)
	if len(read) < 1 {
		return "", fmt.Errorf("task needs an ordinal, e.g. `task 1 status`")
	}
	n, err := strconv.Atoi(read[0])
	if err != nil {
		return "", fmt.Errorf("task: first word is the task ordinal, got %q", read[0])
	}
	obj, err := nthTask(inst, n)
	if err != nil {
		return "", err
	}
	value, err := walkJSON(obj, read[1:])
	if err != nil {
		return "", fmt.Errorf("task %s: %w", strings.Join(read, " "), err)
	}
	return finishRead("task "+strings.Join(read, " "), value, verb)
}

func taskList(inst *instance) ([]json.RawMessage, error) {
	raw, err := inst.app.ModelJSON("tasks")
	if err != nil {
		return nil, err
	}
	var m struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m.Tasks, nil
}

func nthTask(inst *instance, n int) ([]byte, error) {
	ts, err := taskList(inst)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(ts) {
		return nil, fmt.Errorf("task %d: there are %d task(s)", n, len(ts))
	}
	return ts[n-1], nil
}

func nthTaskID(inst *instance, n int) (int64, error) {
	obj, err := nthTask(inst, n)
	if err != nil {
		return 0, err
	}
	var t struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(obj, &t); err != nil {
		return 0, err
	}
	return t.ID, nil
}

// --- archive -----------------------------------------------------------------

// archive count task <n> <kind> — how many archive rows of a kind a task has.
func archiveRead(inst *instance, args []string) (string, error) {
	read, verb := splitVerb(args)
	if len(read) != 4 || read[0] != "count" || read[1] != "task" {
		return "", fmt.Errorf("archive read is `archive count task <n> <kind>`")
	}
	n, err := strconv.Atoi(read[2])
	if err != nil {
		return "", fmt.Errorf("archive: task ordinal must be a number, got %q", read[2])
	}
	id, err := nthTaskID(inst, n)
	if err != nil {
		return "", err
	}
	count, err := inst.world.content.ArchiveCount(id, read[3])
	if err != nil {
		return "", err
	}
	return finishRead("archive "+strings.Join(read, " "), strconv.Itoa(count), verb)
}

// --- click -------------------------------------------------------------------

func click(inst *instance, url string) error {
	id, token, err := link.ParseCompletion(url)
	if err != nil {
		return err
	}
	// Validate the token against the task's minted one — the capability check the
	// web edge performs (docs/04, docs/08).
	obj, err := taskByID(inst, id)
	if err != nil {
		return err
	}
	var t struct {
		CompletionToken string `json:"completion_token"`
	}
	if err := json.Unmarshal(obj, &t); err != nil {
		return err
	}
	if t.CompletionToken == "" || t.CompletionToken != token {
		return fmt.Errorf("click: token %q does not authorise task %d", token, id)
	}

	inst.app.Send(taskmsg.NewFinishTask(taskmsg.FinishTaskPayload{TaskID: id}))
	inst.app.Settle()
	return nil
}

func taskByID(inst *instance, id int64) ([]byte, error) {
	ts, err := taskList(inst)
	if err != nil {
		return nil, err
	}
	for _, raw := range ts {
		var t struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, err
		}
		if t.ID == id {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("no task with id %d", id)
}

// --- shared helpers ----------------------------------------------------------

// blockLines parses a stimulus block into evaluated word lists, so `$var` and
// `[cmd]` (e.g. [fixture …]) substitute while braces still group.
func blockLines(in *testlang.Interp, block string) ([][]string, error) {
	cmds, err := testlang.Parse(block)
	if err != nil {
		return nil, err
	}
	var lines [][]string
	for _, cmd := range cmds {
		var words []string
		for _, w := range cmd.Words {
			s, err := in.EvalWord(w)
			if err != nil {
				return nil, err
			}
			words = append(words, s)
		}
		if len(words) > 0 {
			lines = append(lines, words)
		}
	}
	return lines, nil
}

// arg joins everything after the field name — a value that had spaces was one
// quoted/braced word, so this is usually just words[1].
func arg(words []string) string {
	return strings.Join(words[1:], " ")
}

// selectorIndex resolves `last` or a 1-based N against a collection length.
func selectorIndex(sel string, n int) (int, error) {
	if n == 0 {
		return 0, fmt.Errorf("nothing to select from")
	}
	if sel == "last" {
		return n - 1, nil
	}
	i, err := strconv.Atoi(sel)
	if err != nil {
		return 0, fmt.Errorf("selector must be `last` or a number, got %q", sel)
	}
	if i < 1 || i > n {
		return 0, fmt.Errorf("index %d out of range 1..%d", i, n)
	}
	return i - 1, nil
}

func contentTypeFor(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
