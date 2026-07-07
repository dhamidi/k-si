package msg

import "github.com/dhamidi/k-si/runtime"

// "register-skill" — sent by tasks/store-skill; records a stored skill in the registry
const RegisterSkill = "register-skill"

type RegisterSkillPayload struct {
	SkillID    int64  `json:"skill_id"`
	OriginTask int64  `json:"origin_task"`
	Name       string `json:"name"`
	// SkillMD is the RAW SKILL.md. The reducer derives the description from it on
	// every replay, so a parser fix (or a new frontmatter-derived field) corrects
	// the model on the next replay with no migration — the log stores the raw fact,
	// never a frozen parse result.
	SkillMD []byte `json:"skill_md,omitempty"`
	// Description is a LEGACY fallback: messages logged before SkillMD carried the
	// pre-derived description. The reducer uses it only when SkillMD is absent.
	Description string `json:"description,omitempty"`
	Origin      string `json:"origin"`
	Version     int    `json:"version"`
}

func NewRegisterSkill(p RegisterSkillPayload) runtime.Msg {
	return runtime.NewMsg(RegisterSkill, p)
}
