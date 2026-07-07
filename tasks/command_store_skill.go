package tasks

import (
	"context"
	"log"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dhamidi/k-si/mime"
	"github.com/dhamidi/k-si/runtime"
	skillsmsg "github.com/dhamidi/k-si/skills/msg"
	"github.com/dhamidi/k-si/skilltree"
	"github.com/dhamidi/k-si/store"
)

// "store-skill" — a run authored one or more Agent Skills directories under
// out/skills/<name>/ (Flow D, decision-009). This effect reads each tree from the
// workspace, stores it as a tar in the skill table, provisions it back into the
// workspace's skills/ box (so the next turn of the same task finds it), and emits
// register-skill so the registry gains a light entry. It is tasks-local: the
// finish-agent-run branch is its only caller.
const StoreSkill = "store-skill"

type StoreSkillPayload struct {
	TaskID int64 `json:"task_id"`
	RunID  int64 `json:"run_id"`
}

func NewStoreSkill(p StoreSkillPayload) runtime.Cmd {
	return runtime.NewCmd(StoreSkill, p)
}

func registerStoreSkill(mod *runtime.Module) {
	runtime.HandleCmd(mod, StoreSkill, storeSkillEffect)
}

// skillsPrefix is where the agent authors skills, relative to task-<id>: the
// out/ box's skills/ subtree (decision-009).
const skillsPrefix = "out/skills/"

func storeSkillEffect(ctx context.Context, e Edges, p StoreSkillPayload,
	emit runtime.Emit) error {

	files, err := e.Work.Files(p.TaskID)
	if err != nil {
		return err
	}

	// Group the authored files by skill folder <name>, keeping each part's path
	// relative to the skill root ("SKILL.md", "scripts/run.sh"). Deterministic
	// order so provisioning and the tar are reproducible.
	trees := map[string][]mime.Part{}
	for _, f := range files {
		rel, ok := strings.CutPrefix(f.Filename, skillsPrefix)
		if !ok {
			continue
		}
		name, inner, ok := strings.Cut(rel, "/")
		if !ok || name == "" || inner == "" {
			continue // a file directly under skills/, not inside a skill folder
		}
		trees[name] = append(trees[name], mime.Part{
			Filename:    inner,
			ContentType: f.ContentType,
			Bytes:       f.Bytes,
		})
	}

	names := make([]string, 0, len(trees))
	for name := range trees {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tree := trees[name]

		skillMD, ok := findFile(tree, "SKILL.md")
		if !ok {
			// A skill directory with no SKILL.md is not a valid Agent Skill —
			// skip it rather than failing the run (decision-009).
			log.Printf("tasks: store-skill: task %d skill %q has no SKILL.md, skipping", p.TaskID, name)
			continue
		}

		// The folder name is authoritative (it is the provisioning path). Derive the
		// description for the content table — a MUTABLE projection, so a derived
		// column there is fine (re-derivable in place). The LOG, by contrast, gets
		// the raw SKILL.md below and derives on replay (never a frozen parse result).
		_, description := skilltree.Frontmatter(skillMD)

		tar, err := skilltree.Pack(tree)
		if err != nil {
			return err
		}

		id, err := e.Content.AddSkill(store.SkillRow{
			Name:        name,
			Description: description,
			Content:     tar,
			Origin:      "agent",
			OriginTask:  p.TaskID,
			Version:     1,
			UpdatedAt:   e.Clock.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}

		// Read the stored row back for the authoritative version (AddSkill bumps it
		// on a re-author), so the registry entry matches the table.
		row, _, err := e.Content.SkillByID(id)
		if err != nil {
			return err
		}

		// Provision the same tree under skills/<name>/ so this task's next turn
		// finds ./skills/<name>/SKILL.md (decision-009).
		if err := e.Work.WriteSkills(p.TaskID, treeUnder(name, tree)); err != nil {
			return err
		}

		// Carry the RAW SKILL.md into the log; the skills reducer derives the
		// description from it on every replay (store raw, derive in the model).
		emit(skillsmsg.NewRegisterSkill(skillsmsg.RegisterSkillPayload{
			SkillID:    id,
			OriginTask: p.TaskID,
			Name:       name,
			SkillMD:    skillMD,
			Origin:     "agent",
			Version:    row.Version,
		}))
	}

	return nil
}

// findFile returns the bytes of the tree part whose path (relative to the skill
// root) equals want.
func findFile(tree []mime.Part, want string) ([]byte, bool) {
	for _, p := range tree {
		if p.Filename == want {
			return p.Bytes, true
		}
	}
	return nil, false
}

// treeUnder re-roots a skill tree under "<name>/" so WriteSkills lands it at
// skills/<name>/… — the layout the harness expects (decision-009).
func treeUnder(name string, tree []mime.Part) []mime.Part {
	out := make([]mime.Part, len(tree))
	for i, p := range tree {
		out[i] = mime.Part{
			Filename:    path.Join(name, p.Filename),
			ContentType: p.ContentType,
			Bytes:       p.Bytes,
		}
	}
	return out
}
