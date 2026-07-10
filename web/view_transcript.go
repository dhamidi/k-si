package web

import (
	"context"
	"io"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/transcript"
)

// TranscriptView is the data view_transcript.vue renders — one agent run's
// session, parsed into turns (docs/08, decision-007): assistant prose, dimmed
// thinking, tool calls with their output, and a status footer. Built from
// transcript.Parse over bytes the handler sources (archive for a finished run,
// workspace for a running one) — the same view either way.
type TranscriptView struct {
	TaskID    int64
	RunNumber int64
	Turns     []TurnView
	// Active is true while the run is still writing; the page then self-refreshes
	// (a <meta http-equiv=refresh>) so new turns appear, degrading to a manual
	// refresh with no JavaScript (docs/08, decision-007).
	Active bool
	// BackPath returns to the task detail (reverse-routed) — the local secondary
	// crumb, kept beside the shared top-level nav.
	BackPath string
	// Nav is the shared top-level navbar (navView) — a transcript lights the Tasks
	// section.
	Nav NavView
}

// TurnView is one rendered turn (decision-007). Kind selects the structural
// rendering in the template; Text is the prose/output/summary, Tool the tool
// name (tool calls only), IsError the failure flag (tool results and the footer).
type TurnView struct {
	Kind    string
	Text    string
	Tool    string
	IsError bool
}

// turnViews adapts parsed transcript turns to the view shape.
func turnViews(turns []transcript.Turn) []TurnView {
	views := make([]TurnView, 0, len(turns))
	for _, t := range turns {
		views = append(views, TurnView{Kind: t.Kind, Text: t.Text, Tool: t.Tool, IsError: t.IsError})
	}
	return views
}

// RenderTranscript writes the full transcript page (docs/08).
func RenderTranscript(ctx context.Context, w io.Writer, engine *htmlc.Engine, view TranscriptView) error {
	return engine.RenderPage(ctx, w, "view_transcript", map[string]any{
		"transcript": view,
	})
}
