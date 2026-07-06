package email

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/store"
)

// "assemble-reply" — harvest out/ into a threaded MIME reply with a completion link; write a pending outbox row

func registerAssembleReply(mod *runtime.Module) {
	runtime.HandleCmd(mod, msg.AssembleReply, assembleReplyEffect)
}

// assembleReplyEffect harvests the agent's out/ into a MIME reply — reply.txt is
// the body, everything else an attachment — threads it onto the conversation,
// appends the completion link, and writes a pending outbox row. It emits nothing
// but mark-reply-queued: the actual send is reconciliation's job (docs/03,
// docs/04), which is what makes delivery crash-safe.
func assembleReplyEffect(ctx context.Context, e Edges, p msg.AssembleReplyPayload,
	emit runtime.Emit) error {

	parts, err := e.Work.Harvest(p.TaskID)
	if err != nil {
		return fmt.Errorf("email: assemble-reply: harvest: %w", err)
	}

	body := ""
	var attachments []mime.Part
	for _, part := range parts {
		if part.Filename == "reply.txt" {
			body = string(part.Bytes)
			continue
		}
		attachments = append(attachments, part)
	}
	completionURL, err := link.Completion(e.BaseURL, p.TaskID, p.CompletionToken)
	if err != nil {
		return fmt.Errorf("email: assemble-reply: completion link: %w", err)
	}
	body += "\n\n— mark this task done: " + completionURL + "\n"

	// Same domain tasks used when it pre-recorded this into References — derived
	// from the reply-from address on both sides — or threading breaks.
	messageID := msg.ReplyMessageID(p.TaskID, p.RunID, mime.Domain(p.From))
	hdr := map[string][]string{
		"From":             {p.From},
		"To":               {strings.Join(p.To, ", ")},
		"Subject":          {p.Subject},
		"Message-ID":       {messageID},
		"In-Reply-To":      {p.InReplyTo},
		"References":       {strings.Join(p.References, " ")},
		"X-Kasi-Task":      {strconv.FormatInt(p.TaskID, 10)},
		"X-Kasi-Agent-Run": {strconv.FormatInt(p.RunID, 10)},
	}
	if p.CauseMessageID != "" {
		hdr["X-Kasi-Cause"] = []string{p.CauseMessageID}
	}

	raw, err := mime.Build(hdr, body, attachments)
	if err != nil {
		return fmt.Errorf("email: assemble-reply: build: %w", err)
	}

	id, err := e.Content.AddOutbox(store.OutboxRow{
		TaskID:    p.TaskID,
		MessageID: messageID,
		InReplyTo: p.InReplyTo,
		Raw:       raw,
		Status:    "pending",
		CreatedAt: e.Clock.Now(),
	})
	if err != nil {
		return fmt.Errorf("email: assemble-reply: queue outbox: %w", err)
	}

	emit(NewMarkReplyQueued(MarkReplyQueuedPayload{
		TaskID:    p.TaskID,
		OutboxID:  id,
		MessageID: messageID,
		InReplyTo: p.InReplyTo,
	}))
	return nil
}
