// Package transcript parses the Claude CLI's stream-json transcript (JSONL,
// docs/05) into an ordered slice of legible turns for the web transcript view
// (docs/08, decision-007). It is PURE — bytes in, turns out, no I/O and no
// imports beyond the standard library's encoding/json. The two sources a run
// can live in (the archive for a finished run, the workspace for a running one)
// are the web edge's concern; this package sees only bytes.
//
// The format is an open set we do not own: unknown event types and unknown
// content blocks are SKIPPED, never errored, so a Claude CLI format bump
// degrades to "rendered a bit less," never a crashed page.
package transcript

import (
	"encoding/json"
	"strings"
)

// Kind classifies a turn for the view, which renders each kind structurally
// (decision-007): assistant prose, dimmed thinking, a tool call, a tool result,
// user prose, and a trailing status footer.
const (
	// KindAssistant is an assistant text block — rendered as prose.
	KindAssistant = "assistant"
	// KindThinking is an assistant thinking block — rendered dimmed/secondary.
	KindThinking = "thinking"
	// KindToolUse is a tool call — Tool holds the name, Text a one-line input summary.
	KindToolUse = "tool_use"
	// KindToolResult is a tool result — Text holds the output, IsError flags a failure.
	KindToolResult = "tool_result"
	// KindUser is a user text block — rendered as prose.
	KindUser = "user"
	// KindResult is the trailing run status/footer — Text holds the subtype or
	// final message, IsError flags a failed run.
	KindResult = "result"
)

// Turn is one rendered unit of a transcript (decision-007). The shape is
// deliberately flat so the view can switch on Kind: Text carries the turn's
// prose/output/summary, Tool the tool name (KindToolUse only), and IsError the
// failure flag (tool results and the final result).
type Turn struct {
	Kind    string
	Text    string
	Tool    string
	IsError bool
}

// event is the loose envelope every stream-json line unmarshals into. Only the
// fields we render are named; everything else is ignored. A line that fails to
// unmarshal is skipped (the format is not ours to police).
type event struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Message struct {
		Content []json.RawMessage `json:"content"`
	} `json:"message"`
}

// block is one content block inside an assistant or user message. content is a
// json.RawMessage because a tool_result's content is either a plain string or
// an array of typed sub-blocks (see contentText).
type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	IsError  bool            `json:"is_error"`
	Content  json.RawMessage `json:"content"`
}

// Parse turns stream-json JSONL into an ordered slice of turns. It never errors:
// blank lines, malformed lines, and unknown event/block types are skipped.
func Parse(b []byte) []Turn {
	var turns []Turn
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // not ours to police — skip, never error
		}
		switch ev.Type {
		case "assistant":
			turns = append(turns, assistantTurns(ev)...)
		case "user":
			turns = append(turns, userTurns(ev)...)
		case "result":
			turns = append(turns, resultTurn(ev))
		default:
			// system/init, rate_limit_event, and any future type: skip.
		}
	}
	return turns
}

// assistantTurns renders an assistant message's content blocks: text as prose,
// thinking dimmed, tool_use as a name + one-line input summary.
func assistantTurns(ev event) []Turn {
	var out []Turn
	for _, raw := range ev.Message.Content {
		var blk block
		if err := json.Unmarshal(raw, &blk); err != nil {
			continue
		}
		switch blk.Type {
		case "text":
			if blk.Text != "" {
				out = append(out, Turn{Kind: KindAssistant, Text: blk.Text})
			}
		case "thinking":
			if blk.Thinking != "" {
				out = append(out, Turn{Kind: KindThinking, Text: blk.Thinking})
			}
		case "tool_use":
			out = append(out, Turn{Kind: KindToolUse, Tool: blk.Name, Text: summarizeInput(blk.Input)})
		}
	}
	return out
}

// userTurns renders a user message's content blocks: tool_result as its output
// (error-flagged), text as prose.
func userTurns(ev event) []Turn {
	var out []Turn
	for _, raw := range ev.Message.Content {
		var blk block
		if err := json.Unmarshal(raw, &blk); err != nil {
			continue
		}
		switch blk.Type {
		case "tool_result":
			out = append(out, Turn{Kind: KindToolResult, Text: contentText(blk.Content), IsError: blk.IsError})
		case "text":
			if blk.Text != "" {
				out = append(out, Turn{Kind: KindUser, Text: blk.Text})
			}
		}
	}
	return out
}

// resultTurn renders the trailing run status/footer. Text prefers the final
// result message, falling back to the subtype ("success", "error_max_turns", …).
func resultTurn(ev event) Turn {
	text := ev.Result
	if text == "" {
		text = ev.Subtype
	}
	return Turn{Kind: KindResult, Text: text, IsError: ev.IsError}
}

// summarizeInput reduces a tool_use input object to a single readable line: a
// Bash command, a file path, otherwise a compact one-line JSON. Newlines are
// collapsed so the summary stays one line.
func summarizeInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return oneLine(string(raw))
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description"} {
		if v, ok := m[key].(string); ok && v != "" {
			return oneLine(v)
		}
	}
	compact, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return oneLine(string(compact))
}

// contentText renders a tool_result's content, which is either a plain string
// or an array of typed sub-blocks (e.g. [{type:"text", text:"…"}]). Unknown
// sub-block shapes contribute nothing rather than erroring.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// oneLine collapses all runs of whitespace (including newlines) to single
// spaces so a summary renders on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
