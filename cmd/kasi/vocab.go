package main

// The domain test vocabulary (docs/14): stimuli (deliver, agent, click, fail)
// and reads (outbound, outbox, task, tasks, archive), plus fixture. Each command
// drives or observes the sim world assembled in simworld.go — the mail edge, the
// harness, the workspace, the content tables — exactly as production's real
// edges would be driven by the outside world. Stimuli settle the instance before
// returning, so the next script line sees a stable model (docs/13).

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhamidi/k-si/agents"
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/email"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/memory"
	memorymsg "github.com/dhamidi/k-si/memory/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/secrets"
	"github.com/dhamidi/k-si/store"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/testlang"
	"github.com/dhamidi/k-si/web"
	"github.com/dhamidi/k-si/workspace"
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

	v["answer"] = func(in *testlang.Interp, args []string) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("answer needs a request URL and a block { text|secret|file … }")
		}
		return "", answer(in, inst, args[0], args[1])
	}

	// store observes and seeds the agent's persistent store (Flow F,
	// decision-012), the sim twin the edge type wired onto simWorld. Two forms:
	//   store write <path> <content> — seed the store, simulating the agent's
	//     cache write through the ./store symlink.
	//   store <path>                 — read a store file back for is/matches.
	// The store is outside the log and not replayable model state, so scenarios
	// assert on its OBSERVABLE contents here, not on a rebuilt model.
	v["store"] = func(in *testlang.Interp, args []string) (string, error) {
		return storeVocab(inst, args)
	}

	// memory seeds and observes the durable memory collection (feature-memory.md),
	// the replayable model slice memory.All reads. Two forms:
	//   memory write <name> <content> — seed a memory, sending remember through the
	//     app exactly as the owner's /memory form or a harvested out/memory/ file would.
	//   memory <name> [description]   — read a stored memory's raw content (default)
	//     or its DERIVED description back, for is/matches.
	v["memory"] = func(in *testlang.Interp, args []string) (string, error) {
		return memoryVocab(inst, args)
	}

	// visit / post drive the REAL web.Server in-process (decision-008): visit
	// GETs a page and returns the rendered HTML for matches/is assertions; post
	// issues the Stop action's POST and returns the redirect Location, settling
	// the app so the emitted stop-agent-run has taken effect before the next line.
	v["visit"] = func(in *testlang.Interp, args []string) (string, error) {
		read, verb := splitVerb(args)
		if len(read) != 1 {
			return "", fmt.Errorf("visit needs a single path, e.g. `visit /tasks matches \"*awaiting-user*\"`")
		}
		body, err := inst.webGET(read[0])
		if err != nil {
			return "", err
		}
		return finishRead("visit "+read[0], body, verb)
	}

	v["post"] = func(in *testlang.Interp, args []string) (string, error) {
		read, verb := splitVerb(args)
		// Two forms: a bare POST (the Stop / forget action, no body), or a form POST
		// carrying a `{ field value ... }` body (the remember form). The optional
		// block rides as the second read word.
		if len(read) < 1 || len(read) > 2 {
			return "", fmt.Errorf("post needs a path and an optional form block, e.g. `post /tasks/1/runs/2/stop` or `post /memory { name x; content \"…\" }`")
		}
		var body url.Values
		if len(read) == 2 {
			lines, err := blockLines(in, read[1])
			if err != nil {
				return "", err
			}
			body = url.Values{}
			for _, words := range lines {
				if len(words) < 2 {
					return "", fmt.Errorf("post: form field needs a name and a value, got %q", strings.Join(words, " "))
				}
				body.Set(words[0], strings.Join(words[1:], " "))
			}
		}
		result, err := inst.webPOST(read[0], body)
		if err != nil {
			return "", err
		}
		return finishRead("post "+read[0], result, verb)
	}

	// seed-transcript seats a fixture transcript as a given task+run's in-progress
	// bytes so the transcript-render page has something to parse (decision-007/008).
	// A finished run's transcript is archived and read archive-first, so this is
	// used on a RUNNING run, whose bytes the view sources from the workspace.
	v["seed-transcript"] = func(in *testlang.Interp, args []string) (string, error) {
		return "", seedTranscript(inst, args)
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
		case "work", "workspace":
			// Fault-inject the sim workspace: `fail work harvest` fails the
			// ProvisionedMemory read the memory harvest depends on, and `fail work reply`
			// fails the out/ Harvest read the reply harvest depends on — each scoped to
			// one post-finish harvest so a scenario can crash it mid-flight and prove
			// HarvestPending reconciliation recovers it (decision-013).
			failer, ok := inst.world.work.(interface{ FailNext(string, int) })
			if !ok {
				return "", fmt.Errorf("fail: workspace %T does not support fault injection", inst.world.work)
			}
			failer.FailNext(args[1], n)
		case "content", "store":
			// Fault-inject the content store — `fail content skill` fails AddSkill, the
			// op unique to store-skill, so a scenario can crash the SKILL harvest
			// mid-store (its workspace reads are shared, so this is its only clean seam).
			inst.world.content.FailNext(args[1], n)
		default:
			return "", fmt.Errorf("fail: unknown edge %q (mail, work, content)", args[0])
		}
		return "", nil
	}
}

// --- store (the agent's persistent store edge, Flow F) -----------------------

// storeVocab implements the `store` read/write vocabulary against the sim store
// on inst.world.store — the datastore.Sim the edge type wired onto simWorld. It
// is an edge outside the event log (decision-012), so scenarios assert on its
// OBSERVABLE contents here, never on a rebuilt model.
//
//	store write <path> <content> — seed the store (simulating the agent's cache
//	                               write through the ./store symlink).
//	store <path>                 — read a store file back for is/matches.
func storeVocab(inst *instance, args []string) (string, error) {
	if len(args) >= 1 && args[0] == "write" {
		if len(args) != 3 {
			return "", fmt.Errorf("store write needs a path and content, e.g. `store write wise.db \"…\"`")
		}
		writer, ok := inst.world.store.(interface {
			Write(string, []byte) error
		})
		if !ok {
			return "", fmt.Errorf("store write: the store does not support seeding (only the sim store does)")
		}
		if err := writer.Write(args[1], []byte(args[2])); err != nil {
			return "", fmt.Errorf("store write %s: %w", args[1], err)
		}
		return "", nil
	}

	read, verb := splitVerb(args)
	if len(read) != 1 {
		return "", fmt.Errorf("store read needs a single path, e.g. `store wise.db is \"…\"`")
	}
	b, err := fs.ReadFile(inst.world.store, read[0])
	if err != nil {
		return "", fmt.Errorf("store %s: %w", read[0], err)
	}
	return finishRead("store "+read[0], string(b), verb)
}

// --- memory (the durable memory collection, feature-memory.md) ---------------

// memoryVocab implements the `memory` read/write vocabulary against the
// replayable memory model slice (memory.All).
//
//	memory write <name> <content> — seed a memory: send remember through the app,
//	                                the same directive the /memory form and the
//	                                out/memory/ harvest emit.
//	memory <name> [description]    — read a stored memory's raw content (default) or
//	                                its derived description back for is/matches.
func memoryVocab(inst *instance, args []string) (string, error) {
	if len(args) >= 1 && args[0] == "write" {
		if len(args) != 3 {
			return "", fmt.Errorf("memory write needs a name and content, e.g. `memory write reply-style \"…\"`")
		}
		inst.app.Send(memorymsg.NewRemember(memorymsg.RememberPayload{
			Name:    args[1],
			Content: []byte(args[2]),
		}))
		inst.app.Settle()
		return "", nil
	}

	read, verb := splitVerb(args)
	if len(read) < 1 || len(read) > 2 {
		return "", fmt.Errorf("memory read is `memory <name> [content|description]`")
	}
	name := read[0]
	field := "content"
	if len(read) == 2 {
		field = read[1]
	}
	for _, m := range memory.All(inst.app.View()) {
		if m.Name != name {
			continue
		}
		switch field {
		case "content":
			return finishRead("memory "+strings.Join(read, " "), string(m.Content), verb)
		case "description":
			return finishRead("memory "+strings.Join(read, " "), m.Description, verb)
		default:
			return "", fmt.Errorf("memory %s: unknown field %q (content|description)", name, field)
		}
	}
	return "", fmt.Errorf("memory %s: no such memory", name)
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
		// Deterministic in the sim ring — the real edge mints crypto/rand; scenarios
		// extract the token via `click`, so the value only needs to be stable.
		CompletionToken: fmt.Sprintf("tok-%d", inboxID),
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
		InboxID:         inboxID,
		Recipient:       msg.Header.Get("To"),
		Sender:          firstAddr(msg.Header.Get("From")),
		Cc:              mime.CcList(msg.Header.Get("Cc")),
		Subject:         msg.Header.Get("Subject"),
		MessageID:       msg.Header.Get("Message-ID"),
		InReplyTo:       msg.Header.Get("In-Reply-To"),
		References:      strings.Fields(msg.Header.Get("References")),
		CompletionToken: fmt.Sprintf("tok-%d", inboxID),
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
	var deletions []string
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
		case "del":
			// The agent forgets a memory by deleting the copy it was handed in in/
			// (feature-memory.md). The path is in/-box-relative; `del in/memory/x.md`
			// and `del memory/x.md` both name the same file, so strip a leading "in/".
			if len(words) != 2 {
				return fmt.Errorf("agent: del needs a single in/ path, e.g. `del in/memory/reply-style.md`")
			}
			rel := strings.TrimPrefix(words[1], "in/")
			deletions = append(deletions, rel)
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
		if err := inst.world.sim.DeliverTurn(taskID, out, deletions, exit); err != nil {
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

	// `task <n> inputs` / `task <n> input <file>` observe the files käsi laid into
	// the workspace in/ box — the parser's actual output, not the task model — so a
	// scenario can assert the body was extracted and every attachment was laid in.
	if len(read) >= 2 {
		switch read[1] {
		case "inputs":
			if len(read) != 2 {
				return "", fmt.Errorf("task %d inputs: takes no argument", n)
			}
			names, err := taskInputNames(inst, n)
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), strings.Join(names, " "), verb)
		case "input":
			if len(read) != 3 {
				return "", fmt.Errorf("task %d input: needs a filename, e.g. `task %d input body.txt`", n, n)
			}
			body, err := taskInputBytes(inst, n, read[2])
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), string(body), verb)
		case "outputs":
			if len(read) != 2 {
				return "", fmt.Errorf("task %d outputs: takes no argument", n)
			}
			names, err := taskOutputNames(inst, n)
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), strings.Join(names, " "), verb)
		case "provisioned":
			if len(read) != 2 {
				return "", fmt.Errorf("task %d provisioned: takes no argument", n)
			}
			names, err := taskProvisionedNames(inst, n)
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), strings.Join(names, " "), verb)
		case "request-link":
			if len(read) != 2 {
				return "", fmt.Errorf("task %d request-link: takes no argument", n)
			}
			req, err := taskRequest(inst, n)
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), req.Link, verb)
		case "request-secret":
			if len(read) != 3 {
				return "", fmt.Errorf("task %d request-secret: needs a field, e.g. `task %d request-secret bank-login`", n, n)
			}
			req, err := taskRequest(inst, n)
			if err != nil {
				return "", err
			}
			return finishRead("task "+strings.Join(read, " "), req.SecretRefs[read[2]], verb)
		case "running-run":
			// The id of the task's currently-running agent run — the run id the
			// browse UI's transcript/stop routes carry for a live run. A running run
			// is NOT yet in the task model's Runs slice (that holds finished runs),
			// so it is read from the agents model, the same place RunningRuns looks.
			if len(read) != 2 {
				return "", fmt.Errorf("task %d running-run: takes no argument", n)
			}
			id, err := nthTaskID(inst, n)
			if err != nil {
				return "", err
			}
			for _, r := range agents.RunningRuns(inst.app.View()) {
				if r.TaskID == id {
					return finishRead("task "+strings.Join(read, " "), strconv.FormatInt(int64(r.ID), 10), verb)
				}
			}
			return "", fmt.Errorf("task %d has no running run", n)
		case "run-env":
			// The resolved run environment the (sim) harness was handed — proves a
			// Flow C secret was Resolve'd into the agent's env at the edge (M1.5).
			if len(read) != 3 {
				return "", fmt.Errorf("task %d run-env: needs a var, e.g. `task %d run-env bank-login`", n, n)
			}
			id, err := nthTaskID(inst, n)
			if err != nil {
				return "", err
			}
			h, ok := inst.world.harness.(interface {
				EnvFor(int64) map[string]string
			})
			if !ok {
				return "", fmt.Errorf("task %d run-env: only the sim harness records the run environment", n)
			}
			return finishRead("task "+strings.Join(read, " "), h.EnvFor(id)[read[2]], verb)
		}
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

// taskInputFiles returns the parts käsi laid under the nth task's in/ box, in the
// sorted, in/-stripped form the input reads expose. Workspace.Files already yields
// in/ first and sorted (see the Workspace doc); we keep the in/-prefixed entries
// and strip the prefix so a scenario names files as `body.txt`, not `in/body.txt`.
func taskInputFiles(inst *instance, n int) ([]mime.Part, error) {
	id, err := nthTaskID(inst, n)
	if err != nil {
		return nil, err
	}
	all, err := inst.world.work.Files(id)
	if err != nil {
		return nil, fmt.Errorf("task %d inputs: %w", n, err)
	}
	var ins []mime.Part
	for _, p := range all {
		if name, ok := strings.CutPrefix(p.Filename, "in/"); ok {
			p.Filename = name
			ins = append(ins, p)
		}
	}
	return ins, nil
}

func taskInputNames(inst *instance, n int) ([]string, error) {
	ins, err := taskInputFiles(inst, n)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ins))
	for _, p := range ins {
		names = append(names, p.Filename)
	}
	sort.Strings(names)
	return names, nil
}

// taskOutputNames returns the nth task's out/ box as sorted, out/-stripped
// relative paths — the run's artifact tree ("reply.txt",
// "skills/pay/SKILL.md"), mirroring taskInputNames but for out/ (decision-011).
func taskOutputNames(inst *instance, n int) ([]string, error) {
	id, err := nthTaskID(inst, n)
	if err != nil {
		return nil, err
	}
	all, err := inst.world.work.Files(id)
	if err != nil {
		return nil, fmt.Errorf("task %d outputs: %w", n, err)
	}
	var names []string
	for _, p := range all {
		if name, ok := strings.CutPrefix(p.Filename, "out/"); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// taskProvisionedNames returns the nth task's provisioned-skills box as sorted,
// box-stripped relative paths — the skill trees laid into the workspace (Flow D,
// decision-009). It mirrors taskOutputNames but filters the SkillsBox
// (.claude/skills/) prefix, so a scenario can assert a skill landed in the
// workspace as <name>/… ("pay-invoice/SKILL.md").
func taskProvisionedNames(inst *instance, n int) ([]string, error) {
	id, err := nthTaskID(inst, n)
	if err != nil {
		return nil, err
	}
	all, err := inst.world.work.Files(id)
	if err != nil {
		return nil, fmt.Errorf("task %d provisioned: %w", n, err)
	}
	var names []string
	for _, p := range all {
		if name, ok := strings.CutPrefix(p.Filename, workspace.SkillsBox+"/"); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func taskInputBytes(inst *instance, n int, file string) ([]byte, error) {
	ins, err := taskInputFiles(inst, n)
	if err != nil {
		return nil, err
	}
	for _, p := range ins {
		if p.Filename == file {
			return p.Bytes, nil
		}
	}
	return nil, fmt.Errorf("task %d input %s: no such file in in/", n, file)
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

// taskRequest returns the UI request the nth task raised — the model's
// Model.Requests entry whose TaskID matches, the record `answer` acts on and the
// request-link / request-secret reads expose (Flow C, decision-003).
func taskRequest(inst *instance, n int) (uiRequest, error) {
	id, err := nthTaskID(inst, n)
	if err != nil {
		return uiRequest{}, err
	}
	raw, err := inst.app.ModelJSON("tasks")
	if err != nil {
		return uiRequest{}, err
	}
	var m struct {
		Requests []uiRequest `json:"requests"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return uiRequest{}, err
	}
	for _, r := range m.Requests {
		if r.TaskID == id {
			return r, nil
		}
	}
	return uiRequest{}, fmt.Errorf("task %d has no UI request", n)
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

// --- visit / post / seed-transcript (the real web edge, driven offline) ------

// webServer lazily builds ONE web.Server per App over the world's live edges
// (decision-008): SimSecrets satisfies web.SecretWriter, MemoryContent satisfies
// store.Content. The server reads the live model each request, so caching it
// across visits is safe; boot() nils it so a fresh App rebinds.
func (inst *instance) webServer() (*web.Server, error) {
	if inst.server == nil {
		s, err := web.NewServer(inst.app, inst.world.secrets, inst.world.content, inst.world.work)
		if err != nil {
			return nil, fmt.Errorf("visit: build server: %w", err)
		}
		inst.server = s
	}
	return inst.server, nil
}

// webGET renders a page through the real router+handlers and returns the body.
// A non-2xx status is a loud failure (a broken route must fail the scenario, not
// silently return an empty body) — the body is included for diagnosis.
func (inst *instance) webGET(path string) (string, error) {
	s, err := inst.webServer()
	if err != nil {
		return "", err
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body := rec.Body.String()
	if rec.Code >= 400 {
		return "", fmt.Errorf("visit %s: status %d\n%s", path, rec.Code, body)
	}
	return body, nil
}

// webPOST issues a POST through the real router, settles the app so the handler's
// emitted message has fully applied, and returns the redirect Location (a 303 See
// Other) or, absent one, the status code. A nil body is a bare POST (the Stop /
// forget action); a non-nil url.Values is form-urlencoded (the remember form).
func (inst *instance) webPOST(path string, body url.Values) (string, error) {
	s, err := inst.webServer()
	if err != nil {
		return "", err
	}
	rec := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(http.MethodPost, path, nil)
	}
	s.ServeHTTP(rec, req)
	inst.app.Settle()
	if loc := rec.Header().Get("Location"); loc != "" {
		return loc, nil
	}
	// A 422 is a form re-render on an invalid submit (decision-008) — an EXPECTED
	// outcome, not a broken route — so return its HTML body for matches/is, mirroring
	// visit. Any other >=400 is a genuine failure and fails the scenario loudly.
	if rec.Code == http.StatusUnprocessableEntity {
		return rec.Body.String(), nil
	}
	if rec.Code >= 400 {
		return "", fmt.Errorf("post %s: status %d", path, rec.Code)
	}
	return strconv.Itoa(rec.Code), nil
}

// seedTranscript writes a fixture transcript into a run's workspace slot so the
// transcript-render page has bytes to parse. The task id resolves via nthTaskID;
// the fixture defaults to transcript/sample.jsonl under t/fixtures/.
func seedTranscript(inst *instance, args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("seed-transcript needs `<task-ordinal> <run> [fixture-path]`")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("seed-transcript: task ordinal must be a number, got %q", args[0])
	}
	runID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("seed-transcript: run id must be a number, got %q", args[1])
	}
	id, err := nthTaskID(inst, n)
	if err != nil {
		return err
	}
	fixture := filepath.Join("transcript", "sample.jsonl")
	if len(args) == 3 {
		fixture = args[2]
	}
	b, err := os.ReadFile(filepath.Join("t", "fixtures", fixture))
	if err != nil {
		return fmt.Errorf("seed-transcript: fixture %s: %w", fixture, err)
	}
	if err := inst.world.work.WriteTranscript(id, runID, b); err != nil {
		return fmt.Errorf("seed-transcript: %w", err)
	}
	return nil
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

// --- answer (the web edge's UI-request submission) ---------------------------

// uiRequest is the sliver of the tasks Model.Requests entry the answer vocab
// needs: enough to locate the pending request by run id, capability-check the
// token, and read back its link / secret references for assertions.
type uiRequest struct {
	RunID      int64             `json:"run_id"`
	TaskID     int64             `json:"task_id"`
	Token      string            `json:"token"`
	Link       string            `json:"link"`
	Status     string            `json:"status"`
	SecretRefs map[string]string `json:"secret_refs"`
}

// requestByRunID reads the pending UIRequest keyed by run id from the tasks
// model — the record the web edge locates from a request link (decision-003).
func requestByRunID(inst *instance, runID int64) (uiRequest, error) {
	raw, err := inst.app.ModelJSON("tasks")
	if err != nil {
		return uiRequest{}, err
	}
	var m struct {
		Requests []uiRequest `json:"requests"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return uiRequest{}, err
	}
	for _, r := range m.Requests {
		if r.RunID == runID {
			return r, nil
		}
	}
	return uiRequest{}, fmt.Errorf("no UI request for run %d", runID)
}

// answer performs the web edge's UI-request submission offline, mirroring click
// (decision-003): parse the request link, locate the pending request by run id,
// constant-time token-check, do the web edge's I/O (write secrets and files to
// their stores as references), then emit answer-ui-request carrying ONLY those
// references plus the plaintext text values. Secret plaintext goes to
// SimSecrets.Set and nowhere else — never into the message, the log, or a file.
func answer(in *testlang.Interp, inst *instance, url, block string) error {
	runID, token, err := link.ParseRequest(url)
	if err != nil {
		return err
	}

	req, err := requestByRunID(inst, runID)
	if err != nil {
		return err
	}
	if req.Status != "pending" {
		return fmt.Errorf("answer: request for run %d is %q, not pending", runID, req.Status)
	}
	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(token)) != 1 {
		return fmt.Errorf("answer: token %q does not authorise the request for run %d", token, runID)
	}
	taskID := req.TaskID

	lines, err := blockLines(in, block)
	if err != nil {
		return err
	}

	values := map[string]string{}
	fileRefs := map[string]int64{}
	secretRefs := map[string]string{}

	for _, words := range lines {
		switch words[0] {
		case "text":
			if len(words) < 3 {
				return fmt.Errorf("answer: text needs a field and a value")
			}
			values[words[1]] = strings.Join(words[2:], " ")
		case "secret":
			if len(words) < 3 {
				return fmt.Errorf("answer: secret needs a field and a value")
			}
			field := words[1]
			plaintext := strings.Join(words[2:], " ")
			// Write at the web edge (decision-004): the plaintext goes ONLY to the
			// secrets store; the message carries a secret:// reference.
			u := secrets.URL(fmt.Sprintf("task/%d", taskID), field)
			if err := inst.world.secrets.Set(u, plaintext); err != nil {
				return fmt.Errorf("answer: secret %s: %w", field, err)
			}
			secretRefs[field] = u
		case "file":
			if len(words) != 3 {
				return fmt.Errorf("answer: file needs a field and a path")
			}
			field := words[1]
			path := words[2]
			b, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("answer: file %s: %w", field, err)
			}
			id, err := inst.world.content.AddArchive(store.ArchiveRow{
				TaskID:      taskID,
				Kind:        "attachment",
				Filename:    filepath.Base(path),
				ContentType: "application/octet-stream",
				Bytes:       b,
			})
			if err != nil {
				return fmt.Errorf("answer: file %s: %w", field, err)
			}
			fileRefs[field] = id
		default:
			return fmt.Errorf("answer: unknown field %q (text|secret|file)", words[0])
		}
	}

	inst.app.Send(taskmsg.NewAnswerUIRequest(taskmsg.AnswerUIRequestPayload{
		TaskID:     taskID,
		RunID:      runID,
		Values:     values,
		FileRefs:   fileRefs,
		SecretRefs: secretRefs,
	}))
	inst.app.Settle()
	return nil
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
