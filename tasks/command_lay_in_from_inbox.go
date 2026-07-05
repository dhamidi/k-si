package tasks

import (
	"context"

	"github.com/dhamidi/k-si/runtime"
)

// "lay-in-from-inbox" — parse the stored inbound MIME and write its parts into the workspace in/
const LayInFromInbox = "lay-in-from-inbox"

type LayInFromInboxPayload struct {
	TaskID  int64 `json:"task_id"`
	InboxID int64 `json:"inbox_id"`
}

func NewLayInFromInbox(p LayInFromInboxPayload) runtime.Cmd {
	return runtime.NewCmd(LayInFromInbox, p)
}

func registerLayInFromInbox(mod *runtime.Module) {
	runtime.HandleCmd(mod, LayInFromInbox, layInFromInboxEffect)
}

func layInFromInboxEffect(ctx context.Context, e Edges, p LayInFromInboxPayload,
	emit runtime.Emit) error {
	return nil
}
