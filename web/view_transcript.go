package web

import (
	"context"
	"html"
	"io"
	"log"
	"net/http"

	"github.com/dhamidi/htmlc"

	"github.com/dhamidi/k-si/transcript"
)

// TranscriptView is the data view_transcript.vue renders — one agent run's
// session, parsed into turns (docs/08, decision-007): assistant prose, dimmed
// thinking, tool calls with their output, and a status footer. Built from
// transcript.Parse over bytes the handler sources (archive for a finished run,
// workspace for a running one) — the same view either way.
//
// The turns are wrapped in a <turbo-frame id="run-transcript"> so a live run can
// refresh JUST that frame, not the whole page. The frame HTML is built once
// (transcript_turns.vue rendered, then wrapped) and either embedded in the full
// page (v-html) or written alone as the fragment — content-negotiated on the
// Turbo-Frame request header (docs/16), the same pattern the settings reshape set.
type TranscriptView struct {
	TaskID    int64
	RunNumber int64
	Turns     []TurnView
	// Active is true while the run is still writing; the page then layers a live
	// frame refresh over a no-JavaScript <meta http-equiv=refresh> fallback so new
	// turns appear either way (docs/08, decision-007).
	Active bool
	// BackPath returns to the task detail (reverse-routed) — the local secondary
	// crumb, kept beside the shared top-level nav.
	BackPath string
	// Nav is the shared top-level navbar (navView) — a transcript lights the Tasks
	// section.
	Nav NavView
	// FrameID is the <turbo-frame> id ("run-transcript") Turbo targets on a swap.
	FrameID string
	// FrameHTML is the pre-rendered <turbo-frame>…turns…</turbo-frame>, injected into
	// the page via v-html. htmlc cannot emit a hyphenated custom-element tag itself
	// (it reads <turbo-frame> as a component reference), so the frame wrapper is
	// applied around the htmlc-rendered turns here at the render edge (docs/16).
	FrameHTML string
	// TurboSrc is the reverse-routed assets.turbo URL, passed to base_styles so it
	// emits the one <script> include — the frame reloads through Turbo.
	TurboSrc string
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

// transcriptFrameHTML renders the transcript_turns component to a string and wraps
// it in its <turbo-frame> — the one component, rendered once, that both the full
// page and the bare fragment carry (docs/16). The frame id is HTML-escaped
// defensively, though it is a fixed slug.
func (s *Server) transcriptFrameHTML(ctx context.Context, view TranscriptView) (string, error) {
	turns, err := s.engine.RenderFragmentString(ctx, "transcript_turns", map[string]any{"transcript": view})
	if err != nil {
		return "", err
	}
	return `<turbo-frame id="` + html.EscapeString(view.FrameID) + `">` + turns + `</turbo-frame>`, nil
}

// renderTranscriptPage writes the full <html> page (docs/08): the frame-wrapped
// turns are built first and embedded via v-html, so the page and the frame
// fragment share the one transcript_turns template. The nav, header, back-link and
// live note sit OUTSIDE the frame — only the turns reload.
func (s *Server) renderTranscriptPage(w http.ResponseWriter, r *http.Request, status int, view TranscriptView) {
	fh, err := s.transcriptFrameHTML(r.Context(), view)
	if err != nil {
		log.Printf("web: render transcript frame: %v", err)
		http.Error(w, "could not render the transcript", http.StatusInternalServerError)
		return
	}
	view.FrameHTML = fh

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := RenderTranscript(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render transcript: %v", err)
	}
}

// renderTranscriptFragment writes the bare <turbo-frame> fragment Turbo swaps in
// place (docs/16) — the same turns component, no page chrome. Content-negotiated
// against the full page on the Turbo-Frame request header.
func (s *Server) renderTranscriptFragment(w http.ResponseWriter, r *http.Request, status int, view TranscriptView) {
	fh, err := s.transcriptFrameHTML(r.Context(), view)
	if err != nil {
		log.Printf("web: render transcript fragment: %v", err)
		http.Error(w, "could not render the transcript", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, fh); err != nil {
		log.Printf("web: write transcript fragment: %v", err)
	}
}

// RenderTranscript writes the full transcript page (docs/08).
func RenderTranscript(ctx context.Context, w io.Writer, engine *htmlc.Engine, view TranscriptView) error {
	return engine.RenderPage(ctx, w, "view_transcript", map[string]any{
		"transcript": view,
	})
}
