package cassette

// The third cassette kind is the mail exchange: the verbatim HTTP round-trips
// käsi's real JMAP Submit made to Fastmail on a live run (docs/13), replayed
// back offline so the same send code runs in the recorded ring with no network.
// Like the message log it is a small, ordered record, so it lives as one
// provenance.json header plus an interactions.jsonl of round-trips in send
// order. The Bearer token is never among the recorded fields, so it can never
// land in a cassette.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// mailKind is the provenance Kind that marks a cassette as a recorded mail
// exchange.
const mailKind = "mail-exchange"

// MailInteraction is one recorded HTTP round-trip to the mail provider: the
// request line and body käsi sent, and the status and body the provider
// returned. Headers are deliberately not recorded — the Authorization Bearer
// token lives in one, and a cassette must never carry it.
type MailInteraction struct {
	Method   string
	URL      string
	ReqBody  []byte
	Status   int
	RespBody []byte
}

// MailCassette is a loaded mail-exchange cassette: its provenance and the
// round-trips it recorded, in send order.
type MailCassette struct {
	Provenance   Provenance
	Interactions []MailInteraction
}

// mailLine is the on-disk form of one interaction. The bodies are []byte, so
// encoding/json base64-encodes them by default — MIME and JSON payloads are
// binary and survive a round trip unharmed.
type mailLine struct {
	Seq      int    `json:"seq"`
	Method   string `json:"method"`
	URL      string `json:"url"`
	ReqBody  []byte `json:"req_body"`
	Status   int    `json:"status"`
	RespBody []byte `json:"resp_body"`
}

// SaveMail writes a mail-exchange cassette under dir: an indented
// provenance.json header and an interactions.jsonl of one round-trip per line
// in recorded order. It refuses to save without valid provenance — a cassette
// is captured, never authored (docs/13).
func SaveMail(dir string, c MailCassette) error {
	c.Provenance.Kind = mailKind
	if err := c.Provenance.validate(); err != nil {
		return fmt.Errorf("%s: refusing to save (%v) — a cassette without provenance is refused (docs/13)", dir, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	header, err := json.MarshalIndent(c.Provenance, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "provenance.json"), header, 0o644); err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(dir, "interactions.jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for i, it := range c.Interactions {
		line, err := json.Marshal(mailLine{
			Seq:      i,
			Method:   it.Method,
			URL:      it.URL,
			ReqBody:  it.ReqBody,
			Status:   it.Status,
			RespBody: it.RespBody,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(line))
	}

	return w.Flush()
}

// LoadMail reads a mail-exchange cassette directory: provenance.json (validated
// and required to be Kind "mail-exchange"), then interactions.jsonl in order. A
// directory without valid provenance is refused — re-record it through the live
// ring rather than editing it (docs/13).
func LoadMail(dir string) (MailCassette, error) {
	provPath := filepath.Join(dir, "provenance.json")
	provBytes, err := os.ReadFile(provPath)
	if err != nil {
		return MailCassette{}, refusal(provPath, err)
	}

	var prov Provenance
	if err := json.Unmarshal(provBytes, &prov); err != nil {
		return MailCassette{}, refusal(provPath, err)
	}
	if err := prov.validate(); err != nil {
		return MailCassette{}, refusal(provPath, err)
	}
	if prov.Kind != mailKind {
		return MailCassette{}, refusal(provPath, fmt.Errorf("kind %q is not %s", prov.Kind, mailKind))
	}

	interPath := filepath.Join(dir, "interactions.jsonl")
	file, err := os.Open(interPath)
	if err != nil {
		return MailCassette{}, refusal(interPath, err)
	}
	defer file.Close()

	c := MailCassette{Provenance: prov}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var ml mailLine
		if err := json.Unmarshal(scanner.Bytes(), &ml); err != nil {
			return MailCassette{}, fmt.Errorf("%s:%d: %w", interPath, line, err)
		}
		c.Interactions = append(c.Interactions, MailInteraction{
			Method:   ml.Method,
			URL:      ml.URL,
			ReqBody:  ml.ReqBody,
			Status:   ml.Status,
			RespBody: ml.RespBody,
		})
	}
	if err := scanner.Err(); err != nil {
		return MailCassette{}, err
	}

	return c, nil
}
