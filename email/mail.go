package email

import "context"

// Mail is the mail-provider edge (docs/04): käsi's one seam to the outside mail
// world. Submitting transmits an assembled RFC 5322 message. The real twin
// speaks JMAP to Fastmail (jmap.go); the simulated twin (mail_sim.go) records
// what was sent so scenarios can observe it (docs/12).
type Mail interface {
	Submit(ctx context.Context, raw []byte) error
}
