package msg

import "github.com/dhamidi/k-si/runtime"

// "assemble-reply" — harvest out/ into a threaded MIME reply with a completion link; write a pending outbox row
const AssembleReply = "assemble-reply"

type AssembleReplyPayload struct {
	TaskID          int64    `json:"task_id"`
	RunID           int64    `json:"run_id"`
	From            string   `json:"from"`
	To              []string `json:"to"`
	Subject         string   `json:"subject"`
	InReplyTo       string   `json:"in_reply_to"`
	References      []string `json:"references"`
	CompletionToken string   `json:"completion_token"`
	OutManifest     []string `json:"out_manifest"`
	CauseMessageID  string   `json:"cause_message_id"`
}

func NewAssembleReply(p AssembleReplyPayload) runtime.Cmd {
	return runtime.NewCmd(AssembleReply, p)
}
