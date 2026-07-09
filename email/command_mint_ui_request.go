package email

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/dhamidi/k-si/email/msg"
	"github.com/dhamidi/k-si/link"
	"github.com/dhamidi/k-si/runtime"
	taskmsg "github.com/dhamidi/k-si/tasks/msg"
)

// "mint-ui-request" — mint the capability token and build the request link, then
// emit register-ui-request back to tasks. Minting is a link/token concern that
// lives in email, mirroring how assemble-reply builds the completion link
// (decision-002). The form spec rides through as the raw out/request.json bytes.

func registerMintUIRequest(mod *runtime.Module) {
	runtime.HandleCmd(mod, msg.MintUIRequest, mintUIRequestEffect)
}

func mintUIRequestEffect(ctx context.Context, e Edges, p msg.MintUIRequestPayload,
	emit runtime.Emit) error {

	parts, err := e.Work.Harvest(p.TaskID)
	if err != nil {
		return fmt.Errorf("email: mint-ui-request: harvest: %w", err)
	}
	var spec []byte
	found := false
	for _, part := range parts {
		if part.Filename == "request.json" {
			spec = part.Bytes
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("email: mint-ui-request: task %d wrote no out/request.json", p.TaskID)
	}

	token := mintToken()
	requestURL, err := link.Request(p.BaseURL, p.RunID, token)
	if err != nil {
		return fmt.Errorf("email: mint-ui-request: request link: %w", err)
	}

	emit(taskmsg.NewRegisterUIRequest(taskmsg.RegisterUIRequestPayload{
		TaskID:   p.TaskID,
		RunID:    p.RunID,
		Token:    token,
		FormSpec: spec,
		Link:     requestURL,
	}))
	return nil
}

// mintToken mints an unguessable capability token — 128 bits of crypto/rand,
// URL-safe. Randomness enters here at the edge, never in a pure handler; the
// minted value rides register-ui-request into the log (docs/13), mirroring the
// completion token minted at the inbound edge in serve.go.
func mintToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("email: mint-ui-request: crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
