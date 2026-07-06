package msg

import "github.com/dhamidi/k-si/runtime"

// "register-skill" — sent by tasks/store-skill; records a stored skill in the registry
const RegisterSkill = "register-skill"

type RegisterSkillPayload struct {
	SkillID     int64  `json:"skill_id"`
	OriginTask  int64  `json:"origin_task"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Origin      string `json:"origin"`
	Version     int    `json:"version"`
}

func NewRegisterSkill(p RegisterSkillPayload) runtime.Msg {
	return runtime.NewMsg(RegisterSkill, p)
}
