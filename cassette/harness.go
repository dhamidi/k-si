package cassette

// The second cassette kind is the harness run: the recorded worker turns of a
// real live run (docs/13), replayed against the current build so the recorded
// ring can drive tasks end to end without spawning a subprocess. Unlike the
// message log (one JSONL file), a harness run is a directory — a turn carries
// verbatim in/, out/, and transcript bytes, so it is stored as files rather
// than squeezed into one line.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// harnessKind is the provenance Kind that marks a directory cassette as a
// recorded harness run.
const harnessKind = "harness-run"

// HarnessTurn is one recorded worker turn: the inputs the system laid into in/,
// the verbatim transcript, the files the agent left in out/, and how it exited.
type HarnessTurn struct {
	TaskID      int64
	RunID       int64
	Session     string
	Exit        int
	Stopped     bool
	OutManifest []string
	In          map[string][]byte // filename (no "in/" prefix) -> bytes
	Out         map[string][]byte // filename (no "out/" prefix) -> bytes
	Transcript  []byte
}

// HarnessCassette is a loaded harness-run cassette: its provenance and the
// worker turns it recorded, in play order.
type HarnessCassette struct {
	Provenance Provenance
	Turns      []HarnessTurn
}

// turnMeta is the per-turn meta.json header (everything but the file bytes).
type turnMeta struct {
	TaskID      int64    `json:"task_id"`
	RunID       int64    `json:"run_id"`
	Session     string   `json:"session"`
	Exit        int      `json:"exit"`
	Stopped     bool     `json:"stopped"`
	OutManifest []string `json:"out_manifest"`
}

// SaveHarness writes a harness-run cassette as a directory under dir: an
// indented provenance.json header, then one turn-N/ subdir per turn holding its
// meta.json, in/ and out/ files, and verbatim transcript.jsonl. It refuses to
// save without valid provenance — a cassette is captured, never authored
// (docs/13). Map iteration is sorted so the on-disk output is deterministic.
func SaveHarness(dir string, c HarnessCassette) error {
	c.Provenance.Kind = harnessKind
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

	for i, turn := range c.Turns {
		turnDir := filepath.Join(dir, fmt.Sprintf("turn-%d", i))
		if err := os.MkdirAll(turnDir, 0o755); err != nil {
			return err
		}

		meta := turnMeta{
			TaskID:      turn.TaskID,
			RunID:       turn.RunID,
			Session:     turn.Session,
			Exit:        turn.Exit,
			Stopped:     turn.Stopped,
			OutManifest: turn.OutManifest,
		}
		metaBytes, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(turnDir, "meta.json"), metaBytes, 0o644); err != nil {
			return err
		}

		if err := writeFileMap(filepath.Join(turnDir, "in"), turn.In); err != nil {
			return err
		}
		if err := writeFileMap(filepath.Join(turnDir, "out"), turn.Out); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(turnDir, "transcript.jsonl"), turn.Transcript, 0o644); err != nil {
			return err
		}
	}

	return nil
}

// writeFileMap writes each map entry as a file under sub, creating sub even when
// the map is empty. Keys are sorted for deterministic output.
func writeFileMap(sub string, files map[string][]byte) error {
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return err
	}
	for _, name := range sortedKeys(files) {
		if err := os.WriteFile(filepath.Join(sub, name), files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// LoadHarness reads a harness-run cassette directory: provenance.json (validated
// and required to be Kind "harness-run"), then the turn-N/ subdirs in numeric
// order. A directory without valid provenance is refused — re-record it through
// the live ring rather than editing it (docs/13).
func LoadHarness(dir string) (HarnessCassette, error) {
	provPath := filepath.Join(dir, "provenance.json")
	provBytes, err := os.ReadFile(provPath)
	if err != nil {
		return HarnessCassette{}, refusal(provPath, err)
	}

	var prov Provenance
	if err := json.Unmarshal(provBytes, &prov); err != nil {
		return HarnessCassette{}, refusal(provPath, err)
	}
	if err := prov.validate(); err != nil {
		return HarnessCassette{}, refusal(provPath, err)
	}
	if prov.Kind != harnessKind {
		return HarnessCassette{}, refusal(provPath, fmt.Errorf("kind %q is not %s", prov.Kind, harnessKind))
	}

	c := HarnessCassette{Provenance: prov}
	for i := 0; ; i++ {
		turnDir := filepath.Join(dir, fmt.Sprintf("turn-%d", i))
		if _, err := os.Stat(turnDir); os.IsNotExist(err) {
			break
		} else if err != nil {
			return HarnessCassette{}, err
		}

		turn, err := loadTurn(turnDir)
		if err != nil {
			return HarnessCassette{}, err
		}
		c.Turns = append(c.Turns, turn)
	}

	return c, nil
}

func loadTurn(turnDir string) (HarnessTurn, error) {
	metaBytes, err := os.ReadFile(filepath.Join(turnDir, "meta.json"))
	if err != nil {
		return HarnessTurn{}, err
	}
	var meta turnMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return HarnessTurn{}, fmt.Errorf("%s: %w", filepath.Join(turnDir, "meta.json"), err)
	}

	in, err := readFileMap(filepath.Join(turnDir, "in"))
	if err != nil {
		return HarnessTurn{}, err
	}
	out, err := readFileMap(filepath.Join(turnDir, "out"))
	if err != nil {
		return HarnessTurn{}, err
	}

	transcript, err := os.ReadFile(filepath.Join(turnDir, "transcript.jsonl"))
	if err != nil {
		return HarnessTurn{}, err
	}

	return HarnessTurn{
		TaskID:      meta.TaskID,
		RunID:       meta.RunID,
		Session:     meta.Session,
		Exit:        meta.Exit,
		Stopped:     meta.Stopped,
		OutManifest: meta.OutManifest,
		In:          in,
		Out:         out,
		Transcript:  transcript,
	}, nil
}

// readFileMap reads every regular file in sub into a map keyed by filename. A
// missing sub yields an empty map (a turn may have no in/ or out/ files).
func readFileMap(sub string) (map[string][]byte, error) {
	entries, err := os.ReadDir(sub)
	if os.IsNotExist(err) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(sub, e.Name()))
		if err != nil {
			return nil, err
		}
		files[e.Name()] = b
	}
	return files, nil
}
