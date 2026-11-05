package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// agentSkillFixture makes a hub's skills directory with one skill and
// an agent's skills directory with four entries: a link to the hub's
// skill, a link to a skill outside both, a real directory and a broken
// link. It returns the two directories and the outside skill.
func agentSkillFixture(t *testing.T) (skillsDir, agentDir, outside string) {
	t.Helper()
	dir := t.TempDir()
	skillsDir = filepath.Join(dir, "hub-skills")
	agentDir = filepath.Join(dir, "agent-skills")
	outside = filepath.Join(dir, "outside", "ext-skill")
	write := func(dir, name string) {
		sub := filepath.Join(dir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + name + "\ndescription: 测试 " + name + "。\n---\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(skillsDir, "web-design")
	write(filepath.Dir(outside), "ext-skill")
	write(dir, "real-skill")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(skillsDir, "web-design"), filepath.Join(agentDir, "web-design")); err != nil {
		t.Skip("no symbolic links:", err)
	}
	if err := os.Symlink(outside, filepath.Join(agentDir, "ext-skill")); err != nil {
		t.Skip("no symbolic links:", err)
	}
	if err := os.Rename(filepath.Join(dir, "real-skill"), filepath.Join(agentDir, "real-skill")); err != nil {
		t.Fatal(err)
	}
	// A broken link and a directory without a SKILL.md are no skills.
	if err := os.Symlink(filepath.Join(agentDir, "gone"), filepath.Join(agentDir, "broken")); err != nil {
		t.Skip("no symbolic links:", err)
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "no-md"), 0o755); err != nil {
		t.Fatal(err)
	}
	return skillsDir, agentDir, outside
}

func TestScanAgentSkills(t *testing.T) {
	skillsDir, agentDir, outside := agentSkillFixture(t)
	skills := scanAgentSkills(agentDir, skillsDir)
	if len(skills) != 3 {
		t.Fatalf("scanAgentSkills = %d skills: %+v", len(skills), skills)
	}
	byDir := make(map[string]agentSkill, len(skills))
	for _, s := range skills {
		byDir[s.Dir] = s
	}
	if s := byDir["web-design"]; s.kind != agentSkillHub || !samePath(s.path, filepath.Join(skillsDir, "web-design")) {
		t.Errorf("the hub's link = kind %d, path %q", s.kind, s.path)
	}
	if s := byDir["ext-skill"]; s.kind != agentSkillLink || s.path != filepath.Join(agentDir, "ext-skill") {
		t.Errorf("the outside link = kind %d, path %q", s.kind, s.path)
	}
	if s := byDir["real-skill"]; s.kind != agentSkillReal || s.path != filepath.Join(agentDir, "real-skill") {
		t.Errorf("the real directory = kind %d, path %q", s.kind, s.path)
	}
	if _, ok := byDir["broken"]; ok {
		t.Error("the broken link is a skill")
	}
	if _, ok := byDir["no-md"]; ok {
		t.Error("the directory without a SKILL.md is a skill")
	}
	// Sorted by name: ext-skill, real-skill, web-design.
	if skills[0].Dir != "ext-skill" || skills[1].Dir != "real-skill" || skills[2].Dir != "web-design" {
		t.Errorf("order = %q, %q, %q", skills[0].Dir, skills[1].Dir, skills[2].Dir)
	}

	// The agent's directory being the hub's, a real directory is the
	// hub's own.
	own := scanAgentSkills(skillsDir, skillsDir)
	if len(own) != 1 || own[0].kind != agentSkillOwn {
		t.Fatalf("the hub scanned as an agent's = %+v", own)
	}
	_ = outside
}

func TestImportAgentSkill(t *testing.T) {
	skillsDir, agentDir, outside := agentSkillFixture(t)

	// A real directory: copied into the hub, the directory removed, a
	// link to the hub left in its place.
	if err := importAgentSkill(agentDir, skillsDir, "real-skill"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "real-skill", "SKILL.md")); err != nil {
		t.Error("the skill was not copied into the hub:", err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "real-skill")); got != linkOurs {
		t.Errorf("after the import, skillLink = %d", got)
	}
	if fi, err := os.Lstat(filepath.Join(agentDir, "real-skill")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the agent's entry is not a link: %v", err)
	}
	// The import record came along.
	if meta, ok := loadImportMeta(filepath.Join(skillsDir, "real-skill")); !ok || meta.Source != "local" {
		t.Errorf("the import record = %+v, %v", meta, ok)
	}

	// A link to elsewhere: the content copied, the link removed — its
	// target outside untouched — a link to the hub in its place.
	if err := importAgentSkill(agentDir, skillsDir, "ext-skill"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "ext-skill", "SKILL.md")); err != nil {
		t.Error("the skill was not copied into the hub:", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err != nil {
		t.Error("the link's target outside was touched:", err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "ext-skill")); got != linkOurs {
		t.Errorf("after the import, skillLink = %d", got)
	}

	// Importing the same entry again — a second click on a stale view
	// meets the link the first import made — is refused, and the hub's
	// copy and the link stay.
	if err := importAgentSkill(agentDir, skillsDir, "ext-skill"); err == nil {
		t.Error("importing what is imported worked")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "ext-skill", "SKILL.md")); err != nil {
		t.Error("the hub's copy is gone:", err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "ext-skill")); got != linkOurs {
		t.Errorf("after the refused import, skillLink = %d", got)
	}

	// The agent's directory being the hub's, there is nothing to import.
	if err := importAgentSkill(skillsDir, skillsDir, "web-design"); err == nil {
		t.Error("importing the hub's own directory worked")
	}
}

// The Agent 技能 pane shows the chosen agent's skills as cards: a link
// to the hub removable, a real directory importable.
func TestAgentSkillsPane(t *testing.T) {
	skillsDir, agentDir, _ := agentSkillFixture(t)
	otherDir := filepath.Join(t.TempDir(), "other-agent")

	a := &app{selected: "agent", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.agents = []agent{{Name: "通用 Agent", Dir: agentDir}, {Name: "Kimi", Dir: otherDir}}
	tt := ui.NewTester(a.view, 1000, 640)
	for _, s := range []string{"Agent 技能", "当前：", "通用 Agent", "web-design", "ext-skill", "real-skill", "技能中心", "真实目录", "外部软链接"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}

	// The search filters the cards.
	if err := tt.Click("在该 agent 中搜索技能"); err != nil {
		t.Fatal(err)
	}
	tt.Type("real")
	if !tt.HasText("real-skill") {
		t.Errorf("no real-skill after a search for real, texts %q", tt.Texts())
	}
	if tt.HasText("web-design") {
		t.Error("web-design after a search for real")
	}
	// Clearing the search is a state change, which a fresh tester picks
	// up.
	a.agentSkillSearch = ""
	tt = ui.NewTester(a.view, 1000, 640)

	// The other agent has no skills; picking it shows the empty state,
	// and the subtitle follows the choice.
	if err := tt.Click("查看 Kimi 的技能"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("该 agent 还没有技能") {
		t.Errorf("no empty state for Kimi, texts %q", tt.Texts())
	}
	if err := tt.Click("查看 通用 Agent 的技能"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("web-design") {
		t.Fatalf("no web-design back on 通用 Agent, texts %q", tt.Texts())
	}

	// 移除 deletes the agent's link to the hub's skill; the skill stays.
	if err := tt.Click("移除 web-design"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(agentDir, "web-design")); !os.IsNotExist(err) {
		t.Error("the link stayed after 移除")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "web-design", "SKILL.md")); err != nil {
		t.Error("移除 deleted the hub's skill:", err)
	}

	// 导入 moves a real directory into the hub and links it back.
	if err := tt.Click("导入 real-skill 到技能中心"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "real-skill", "SKILL.md")); err != nil {
		t.Error("the skill was not imported into the hub:", err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "real-skill")); got != linkOurs {
		t.Errorf("after 导入, skillLink = %d", got)
	}
	// The hub's list reloaded with it.
	found := false
	for _, sk := range a.skills {
		if sk.Dir == "real-skill" {
			found = true
		}
	}
	if !found {
		t.Error("the imported skill is missing from the hub's list")
	}
}

// The pane without agents points at the settings.
func TestAgentSkillsNoAgents(t *testing.T) {
	a := &app{selected: "agent", focusSide: "sidebar", skillsDir: t.TempDir()}
	tt := ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("还没有添加任何 agent") {
		t.Errorf("no empty state, texts %q", tt.Texts())
	}
}
