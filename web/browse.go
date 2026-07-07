package web

import (
	"log"
	"net/http"
	"strconv"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/agents"
	agentmsg "github.com/dhamidi/k-si/agents/msg"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/tasks"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
	"github.com/dhamidi/k-si/transcript"
)

// showTasks renders the task list (docs/08): every task grouped by status,
// newest-first within each group. Host-gated, no token (decision-006). The view
// does the grouping/sorting; the model read returns tasks in model order.
func (s *Server) showTasks(w http.ResponseWriter, r *http.Request) {
	all := tasks.All(s.app.View())

	skillsPath, _ := s.router.Path("skills.index", nil)
	memoryPath, _ := s.router.Path("memory.index", nil)
	view := TasksView{Groups: groupTasks(all, s.taskShowPath, s.taskMarkDonePath), SkillsPath: skillsPath, MemoryPath: memoryPath}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderTasks(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render tasks: %v", err)
	}
}

// showTask renders one task's detail (docs/08): status/subject/participants, the
// agent runs (with a Stop form on the active run), any open UI request, and the
// archived artifacts. 404 on a bad id or a missing task.
func (s *Server) showTask(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathInt(w, r, "id")
	if !ok {
		return
	}

	task, found := tasks.Get(s.app.View(), tasks.TaskID(id))
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	view := TaskView{
		ID:           id,
		Status:       string(task.Status),
		Subject:      mime.DecodeSubject(task.Subject),
		Route:        task.Route,
		Participants: task.Participants,
		Runs:         s.runRows(id, task),
		Request:      s.openRequest(task),
		Artifacts:    s.artifactNames(id),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderTask(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render task %d: %v", id, err)
	}
}

// showTranscript renders one run's session (docs/08, decision-007). It sources
// bytes from two places: the archived transcript in content for a finished run,
// else the workspace's in-progress transcript for a running one — the same
// parser and view either way. 404 on a bad id or a missing task.
func (s *Server) showTranscript(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathInt(w, r, "id")
	if !ok {
		return
	}
	runID, ok := s.pathInt(w, r, "run")
	if !ok {
		return
	}

	task, found := tasks.Get(s.app.View(), tasks.TaskID(id))
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	b := s.transcriptBytes(id, runID)

	back, _ := s.router.Path("tasks.show", dispatch.Params{"id": strconv.FormatInt(id, 10)})
	view := TranscriptView{
		TaskID:    id,
		RunNumber: runOrdinal(task, runID),
		Turns:     turnViews(transcript.Parse(b)),
		Active:    runID == s.activeRunID(id),
		BackPath:  back,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderTranscript(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render transcript task %d run %d: %v", id, runID, err)
	}
}

// stopRun emits stop-agent-run for a running agent and redirects back to the
// task detail (docs/08, "Stopping an agent"). App.Send blocks until applied, so
// the redirected GET already reflects the stop. Host-gated, no token.
func (s *Server) stopRun(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathInt(w, r, "id")
	if !ok {
		return
	}
	runID, ok := s.pathInt(w, r, "run")
	if !ok {
		return
	}

	if _, found := tasks.Get(s.app.View(), tasks.TaskID(id)); !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	s.app.Send(agentmsg.NewStopAgentRun(agentmsg.StopAgentRunPayload{TaskID: id, RunID: runID}))

	show, _ := s.router.Path("tasks.show", dispatch.Params{"id": strconv.FormatInt(id, 10)})
	http.Redirect(w, r, show, http.StatusSeeOther)
}

// runRows builds the detail view's run list. Each run gets its transcript link;
// the active run (the last run of an awaiting-agent task) additionally gets the
// Stop form's POST target. Number is the 1-based ordinal; the reverse-routed
// paths carry the run's real id.
func (s *Server) runRows(taskID int64, task tasks.Task) []RunRow {
	rows := make([]RunRow, 0, len(task.Runs)+1)
	// Finished runs live in task.Runs (appended on agent-run-finished); each gets a
	// transcript link, none is active.
	for i, runID := range task.Runs {
		rows = append(rows, RunRow{
			Number:         int64(i + 1),
			TranscriptPath: s.transcriptPath(taskID, runID),
		})
	}
	// The currently-running run is NOT in task.Runs — it lives in the agents model.
	// It is the active run: the one with a live transcript and the only one that can
	// be Stopped, so its Stop form must target its real id (docs/08).
	if runID := s.activeRunID(taskID); runID != 0 {
		rows = append(rows, RunRow{
			Number:         int64(len(task.Runs) + 1),
			TranscriptPath: s.transcriptPath(taskID, runID),
			Active:         true,
			StopPath:       s.stopPath(taskID, runID),
		})
	}
	return rows
}

// activeRunID returns the id of the task's currently-running agent run, read from
// the agents model (a running run is not yet in task.Runs), or 0 if none is
// running — the run the Stop button targets and the transcript view auto-refreshes.
func (s *Server) activeRunID(taskID int64) int64 {
	for _, run := range agents.RunningRuns(s.app.View()) {
		if run.TaskID == taskID {
			return int64(run.ID)
		}
	}
	return 0
}

// openRequest finds a pending UI request raised by one of the task's runs and
// returns a link to answer it (the tokened request route). None open → absent.
func (s *Server) openRequest(task tasks.Task) RequestLink {
	for _, runID := range task.Runs {
		req, ok := tasks.RequestByRunID(s.app.View(), runID)
		if ok && req.Status == tasks.RequestPending {
			return RequestLink{Present: true, URL: s.requestAction(req.RunID, req.Token)}
		}
	}
	return RequestLink{}
}

// artifactNames lists the filenames archived for a task (docs/08). A store error
// degrades to an empty list — a browse page never fails on a missing archive.
func (s *Server) artifactNames(taskID int64) []string {
	rows, err := s.content.ArchiveByTask(taskID)
	if err != nil {
		log.Printf("web: archive for task %d: %v", taskID, err)
		return nil
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Filename)
	}
	return names
}

// transcriptBytes sources a run's transcript: the archived transcript row in
// content for a finished run, else the workspace's in-progress bytes for a
// running one (decision-007). Either miss degrades to empty (parsed to no
// turns) rather than a failed page.
func (s *Server) transcriptBytes(taskID, runID int64) []byte {
	rows, err := s.content.ArchiveByTask(taskID)
	if err == nil {
		for _, row := range rows {
			if row.Kind == "transcript" && row.AgentRun == runID {
				return row.Bytes
			}
		}
	}
	b, err := s.work.ReadTranscript(taskID, runID)
	if err != nil {
		return nil
	}
	return b
}

// pathInt parses a positive int route param, 404-ing on a bad value.
func (s *Server) pathInt(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	v, err := strconv.ParseInt(params[name], 10, 64)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return 0, false
	}
	return v, true
}

// taskShowPath reverse-routes the detail path for a task id (rule
// no-url-string-building).
func (s *Server) taskShowPath(id int64) string {
	p, _ := s.router.Path("tasks.show", dispatch.Params{"id": strconv.FormatInt(id, 10)})
	return p
}

// taskMarkDonePath reverse-routes the list's "Done" POST target for a task id —
// the completion pattern, driven host-gated from the UI (rule no-url-string-building).
func (s *Server) taskMarkDonePath(id int64) string {
	p, _ := s.router.Path("tasks.markdone", dispatch.Params{"id": strconv.FormatInt(id, 10)})
	return p
}

// markDone finishes a task straight from the list — the host-gated UI counterpart
// of the emailed completion link (decision-006, no token). It emits finish-task and
// redirects back to the list; App.Send blocks until applied, so the redirected GET
// shows the task moved to done. Idempotent: a done task is a no-op.
func (s *Server) markDone(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathInt(w, r, "id")
	if !ok {
		return
	}
	task, found := tasks.Get(s.app.View(), tasks.TaskID(id))
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if task.Status != tasks.Done {
		s.app.Send(taskmsg.NewFinishTask(taskmsg.FinishTaskPayload{TaskID: id}))
	}
	index, _ := s.router.Path("tasks.index", nil)
	http.Redirect(w, r, index, http.StatusSeeOther)
}

func (s *Server) transcriptPath(taskID, runID int64) string {
	p, _ := s.router.Path("runs.transcript", dispatch.Params{
		"id":  strconv.FormatInt(taskID, 10),
		"run": strconv.FormatInt(runID, 10),
	})
	return p
}

func (s *Server) stopPath(taskID, runID int64) string {
	p, _ := s.router.Path("runs.stop", dispatch.Params{
		"id":  strconv.FormatInt(taskID, 10),
		"run": strconv.FormatInt(runID, 10),
	})
	return p
}

// runOrdinal returns the 1-based position of runID among the task's runs — its
// index in the finished list, or one past the end for the currently-running run,
// which is not yet in task.Runs.
func runOrdinal(task tasks.Task, runID int64) int64 {
	for i, id := range task.Runs {
		if id == runID {
			return int64(i + 1)
		}
	}
	return int64(len(task.Runs) + 1)
}
