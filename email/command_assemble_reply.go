package email

import (
	"context"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/runtime"
)

// "assemble-reply" — harvest out/ into a threaded MIME reply with a completion link; write a pending outbox row

func registerAssembleReply(mod *runtime.Module) {
	runtime.HandleCmd(mod, msg.AssembleReply, assembleReplyEffect)
}

func assembleReplyEffect(ctx context.Context, e Edges, p msg.AssembleReplyPayload,
	emit runtime.Emit) error {
	// On success, the result enters the model as a message (docs/01):
	// emit(NewMarkReplyQueued(…))
	return nil
}
