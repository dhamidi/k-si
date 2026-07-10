package web

import (
	"log"
	"net/http"
	"net/url"

	"github.com/dhamidi/dispatch"

	"github.com/dhamidi/k-si/skills"
	"github.com/dhamidi/k-si/skilltree"
)

// showSkills renders the skills registry (docs/08, Flow D): every authored skill,
// newest-first, each a link to its detail. Host-gated, no token (decision-006).
// The list reads the content-free registry (skills.All); a skill's tree is read
// only on the detail page (decision-010).
func (s *Server) showSkills(w http.ResponseWriter, r *http.Request) {
	all := skills.All(s.app.View())

	view := SkillsView{
		Skills: skillRows(all, s.skillShowPath),
		Nav:    s.navView("skills.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderSkills(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render skills: %v", err)
	}
}

// showSkill renders one skill's detail (docs/08, Flow D): its metadata, its file
// tree (each entry linking to the file view), and the SKILL.md body inline. The
// tree is read from the content store's tar by name (decision-010). 404 on an
// unknown skill.
func (s *Server) showSkill(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	name := params["name"]

	row, found, err := s.content.SkillByName(name)
	if err != nil {
		log.Printf("web: skill %q: %v", name, err)
		http.Error(w, "could not read the skill", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	entries, err := skilltree.List(row.Content)
	if err != nil {
		log.Printf("web: skill %q: list tree: %v", name, err)
		http.Error(w, "could not read the skill", http.StatusInternalServerError)
		return
	}
	files := make([]SkillFileLink, 0, len(entries))
	for _, path := range entries {
		files = append(files, SkillFileLink{Path: path, FilePath: s.skillFilePath(name, path)})
	}

	// The SKILL.md body is shown inline (structure first). A missing SKILL.md
	// degrades to empty prose rather than a failed page.
	md, _, err := skilltree.Read(row.Content, "SKILL.md")
	if err != nil {
		log.Printf("web: skill %q: read SKILL.md: %v", name, err)
	}

	view := SkillView{
		Name:        row.Name,
		Description: row.Description,
		Origin:      row.Origin,
		Version:     row.Version,
		Files:       files,
		SkillMD:     string(md),
		Nav:         s.navView("skills.index"),
	}
	// An agent-origin skill links back to the task that authored it (docs/08).
	if row.Origin == "agent" && row.OriginTask != 0 {
		view.HasOriginTask = true
		view.OriginTask = row.OriginTask
		view.OriginTaskPath = s.taskShowPath(row.OriginTask)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderSkill(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render skill %q: %v", name, err)
	}
}

// showSkillFile renders one entry of a skill's tree (docs/08, Flow D): its raw
// text in a <pre>, read from the tar via skilltree.Read. The catch-all {+path}
// param carries a multi-segment path (scripts/extract.sh) percent-encoded, so it
// is unescaped before the tar lookup. 404 on an unknown skill or a missing entry.
func (s *Server) showSkillFile(w http.ResponseWriter, r *http.Request) {
	params, _ := dispatch.ParamsFromContext(r.Context())
	name := params["name"]
	path, err := url.PathUnescape(params["path"])
	if err != nil {
		path = params["path"]
	}

	row, found, err := s.content.SkillByName(name)
	if err != nil {
		log.Printf("web: skill %q: %v", name, err)
		http.Error(w, "could not read the skill", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	b, ok, err := skilltree.Read(row.Content, path)
	if err != nil {
		log.Printf("web: skill %q file %q: %v", name, path, err)
		http.Error(w, "could not read the file", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	view := SkillFileView{
		SkillName: name,
		Path:      path,
		Content:   string(b),
		BackPath:  s.skillShowPath(name),
		Nav:       s.navView("skills.index"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderSkillFile(r.Context(), w, s.engine, view); err != nil {
		log.Printf("web: render skill %q file %q: %v", name, path, err)
	}
}

// skillShowPath reverse-routes a skill's detail path for its name.
func (s *Server) skillShowPath(name string) string {
	p, _ := s.router.Path("skills.show", dispatch.Params{"name": name})
	return p
}

// skillFilePath reverse-routes a skill file's path — the catch-all {+path}
// preserves the multi-segment relative path (scripts/extract.sh).
func (s *Server) skillFilePath(name, path string) string {
	p, _ := s.router.Path("skills.file", dispatch.Params{"name": name, "path": path})
	return p
}
