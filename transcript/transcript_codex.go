package transcript

import (
	"encoding/json"
	"strings"
)

// ParseCodex parses the Codex CLI's `codex exec --json` stream (JSONL) into the
// SAME ordered []Turn / Kind constants the Claude parser (Parse) produces, so
// the transcript view stays harness-agnostic (decision-024): it switches on Kind
// alone and never learns which worker wrote the run.
//
// The Codex event shapes are live-verified against codex-cli (the harvest path
// that also announces the session on thread.started):
//
//	{"type":"thread.started","thread_id":"…"}                          — skipped (session announce)
//	{"type":"turn.started"}                                            — skipped
//	{"type":"item.completed","item":{"type":"agent_message","text":…}} — KindAssistant
//	{"type":"item.completed","item":{"type":"reasoning","text":…}}     — KindThinking
//	{"type":"item.completed","item":{"type":"command_execution", …}}   — KindToolUse + KindToolResult
//	{"type":"item.completed","item":{"type":"error","message":…}}      — KindResult (error)
//	{"type":"turn.completed", …}                                       — KindResult (success footer)
//	{"type":"turn.failed","error":{"message":…}}                       — KindResult (error footer)
//
// Like Parse it is PURE and follows the skip-unknown-never-error discipline: the
// format is an open set we do not own, so blank lines, malformed lines, and
// unknown event/item types are SKIPPED, never errored — a Codex format bump
// degrades to "rendered a bit less," never a crashed page. Transient top-level
// {"type":"error",…} reconnect notices are skipped as noise.
func ParseCodex(b []byte) []Turn {
	var turns []Turn
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // not ours to police — skip, never error
		}
		switch ev.Type {
		case "item.completed":
			turns = append(turns, codexItemTurns(ev.Item)...)
		case "turn.completed":
			turns = append(turns, Turn{Kind: KindResult, Text: "success"})
		case "turn.failed":
			turns = append(turns, Turn{Kind: KindResult, Text: ev.Error.Message, IsError: true})
		default:
			// thread.started, turn.started, top-level error/reconnect noise, and any
			// future event type: skip.
		}
	}
	return turns
}

// codexItemTurns renders one completed item. An agent_message is assistant prose,
// reasoning is dimmed thinking, a command_execution expands into a tool call plus
// its captured output (error-flagged on a non-zero exit), and an error item is a
// failed-run footer.
func codexItemTurns(item codexItem) []Turn {
	switch item.Type {
	case "agent_message":
		if item.Text == "" {
			return nil
		}
		return []Turn{{Kind: KindAssistant, Text: item.Text}}
	case "reasoning":
		if item.Text == "" {
			return nil
		}
		return []Turn{{Kind: KindThinking, Text: item.Text}}
	case "command_execution":
		return []Turn{
			{Kind: KindToolUse, Tool: "Shell", Text: oneLine(item.Command)},
			{Kind: KindToolResult, Text: item.AggregatedOutput, IsError: item.commandFailed()},
		}
	case "error":
		return []Turn{{Kind: KindResult, Text: item.Message, IsError: true}}
	default:
		// file_change, mcp_tool_call, and any future item type: skip.
		return nil
	}
}

// codexEvent is the loose envelope every `codex exec --json` line unmarshals
// into. Only the fields we render are named; everything else is ignored. A line
// that fails to unmarshal is skipped (the format is not ours to police).
type codexEvent struct {
	Type  string    `json:"type"`
	Item  codexItem `json:"item"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// codexItem is the "item" object of an item.completed event. The fields are a
// union over the item types we render; each type reads only its own.
type codexItem struct {
	Type             string `json:"type"`
	Text             string `json:"text"`              // agent_message, reasoning
	Message          string `json:"message"`           // error
	Command          string `json:"command"`           // command_execution
	AggregatedOutput string `json:"aggregated_output"` // command_execution
	ExitCode         *int   `json:"exit_code"`         // command_execution
	Status           string `json:"status"`            // command_execution
}

// commandFailed reports whether a command_execution ended in failure. A present
// non-zero exit code marks the error; absent the code, a non-"completed" status
// (e.g. "failed") does. A clean run (exit 0 / "completed") is not an error.
func (i codexItem) commandFailed() bool {
	if i.ExitCode != nil {
		return *i.ExitCode != 0
	}
	return i.Status != "" && i.Status != "completed"
}

// IsCodexStream reports whether b looks like a `codex exec --json` transcript
// rather than Claude's stream-json — it inspects the first non-blank JSON line's
// event type. The web edge uses it to pick ParseCodex vs Parse when it has not
// otherwise pinned the run's harness; the two formats' leading events are
// disjoint (Codex opens on thread.started, Claude on system/init), so one line
// decides it.
func IsCodexStream(b []byte) bool {
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		switch ev.Type {
		case "thread.started", "turn.started", "turn.completed", "turn.failed", "item.completed":
			return true
		default:
			return false
		}
	}
	return false
}
