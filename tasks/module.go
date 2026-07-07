package tasks

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
	"github.com/dhamidi/k-si/workspace"
)

// Edges is everything tasks touches in the world. Real implementations are
// wired in cmd/kasi/main.go; simulated twins live in this package (docs/12).
type Edges struct {
	Clock   runtime.Clock
	Work    workspace.Workspace
	Content store.Content
}

// Module bundles task lifecycle and workspaces (docs/01).
func Module(e Edges) *runtime.Module {
	mod := runtime.NewModule("tasks", Model{}, e)

	registerCreateTask(mod)
	registerAppendToTask(mod)
	registerFinishTask(mod)
	registerAgentRunFinished(mod)
	registerRegisterUIRequest(mod)
	registerAnswerUIRequest(mod)
	registerLayInAnswers(mod)
	registerLayInFromInbox(mod)
	registerCaptureTranscript(mod)
	registerArchiveTask(mod)
	registerSetReplyFrom(mod)
	registerStoreSkill(mod)
	registerCaptureMemory(mod)
	registerRunHarvest(mod)
	registerMarkHarvested(mod)
	runtime.Subscribe(mod, harvestReconcileSubs)
	return mod
}

// SimEdges is the full simulated set — what `kasi test` assembles by
// default, and the simulated twin the twin rule demands (docs/12).
func SimEdges() Edges {
	return Edges{
		Clock:   runtime.SimClock(),
		Work:    workspace.NewMemory(),
		Content: store.NewMemoryContent(),
	}
}
