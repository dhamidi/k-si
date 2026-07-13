package skills

import (
	"github.com/dhamidi/k-si/runtime"
	"github.com/dhamidi/k-si/skills/msg"
)

// "unregister-skill" — drop a skill from the registry by name; sent by the owner's /skills Remove control (Flow D Ask 2)

func registerUnregisterSkill(mod *runtime.Module) {
	runtime.HandleMsg(mod, msg.UnregisterSkill, handleUnregisterSkill)
}

func handleUnregisterSkill(v runtime.View, s Model, p msg.UnregisterSkillPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	// Drop the entry whose name matches; an absent name is a no-op (idempotent),
	// so a double-submit or a replay after the store blob is already gone is
	// harmless. Copy-on-write, order-preserving so the prior model snapshot stays
	// immutable for lock-free readers and replay converges deterministically
	// (rules/no-inplace-model-mutation.yml). The tar blob is deleted separately at
	// the web edge (store.Content.DeleteSkill); this logged event is Home #2.
	i := s.findName(p.Name)
	if i < 0 {
		return s, nil
	}
	next := make([]Skill, 0, len(s.Skills)-1)
	next = append(next, s.Skills[:i]...)
	next = append(next, s.Skills[i+1:]...)
	s.Skills = next
	return s, nil
}
