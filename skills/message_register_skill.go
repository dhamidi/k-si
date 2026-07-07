package skills

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/skills/msg"
	"github.com/dhamidi/k-si/skilltree"
)

// "register-skill" — sent by tasks/store-skill after it writes the skill's tree
// to the store and provisions it into the workspace (Flow D, decision-009). It
// records the light registry entry; the tar tree never enters the log or model.

func registerRegisterSkill(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.RegisterSkill, handleRegisterSkill)
}

func handleRegisterSkill(v runtime.View, s Model, p msg.RegisterSkillPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Derive the description from the raw SKILL.md HERE, in the pure reducer, so
	// every replay re-parses it with the current parser — a parser fix or a new
	// derived field is free on the next replay, no migration (the log holds the raw,
	// the model holds the derivation). Messages logged before SkillMD carry no raw;
	// fall back to their already-derived Description.
	description := p.Description
	if len(p.SkillMD) > 0 {
		_, description = skilltree.Frontmatter(p.SkillMD)
	}

	entry := Skill{
		ID:          p.SkillID,
		OriginTask:  p.OriginTask,
		Name:        p.Name,
		Description: description,
		Origin:      p.Origin,
		Version:     p.Version,
	}

	// A skill's name is UNIQUE (decision-010): a re-authored skill updates its
	// registry entry in place rather than appending a duplicate. Copy-on-write so
	// the prior model stays untouched for replay.
	skillsCopy := append([]Skill(nil), s.Skills...)
	if i := s.findName(p.Name); i >= 0 {
		skillsCopy[i] = entry
	} else {
		skillsCopy = append(skillsCopy, entry)
	}
	s.Skills = skillsCopy
	return s, nil
}
