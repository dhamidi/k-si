package msg

import "github.com/dhamidi/k-si/runtime"

// "unregister-skill" — drop a skill from the registry by name; sent by the owner's /skills Remove control (Flow D Ask 2)
const UnregisterSkill = "unregister-skill"

type UnregisterSkillPayload struct {
	Name string `json:"name"`
}

func NewUnregisterSkill(p UnregisterSkillPayload) runtime.Msg {
	return runtime.NewMsg(UnregisterSkill, p)
}
