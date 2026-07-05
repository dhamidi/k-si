package email

import (
	"context"
	"fmt"
)

// Mail is the mail-provider edge (docs/04): käsi's one seam to the outside mail
// world. Submitting transmits an assembled RFC 5322 message. The real twin
// speaks JMAP to Fastmail; the simulated twin (mail_sim.go) records what was
// sent so scenarios can observe it (docs/12).
type Mail interface {
	Submit(ctx context.Context, raw []byte) error
}

// JMAP is the real mail edge (docs/04). It lands in stage 2 against a Fastmail
// test domain; until then it compiles and refuses, so `serve` builds and the
// simulation ring — which wires SimMail — carries all the logic (docs/12, BUILDING).
type JMAP struct {
	Endpoint string
	Token    string // a secret:// URL, resolved at the edge (docs/06)
}

func (JMAP) Submit(ctx context.Context, raw []byte) error {
	return fmt.Errorf("email: the JMAP mail edge lands in stage 2 (BUILDING.md)")
}
