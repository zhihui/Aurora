package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// packFixture makes a skills directory with two skills in it and an
// agent's skills directory, in a temporary home.
func packFixture(t *testing.T) (skillsDir, agentDir string, targets []string) {
	t.Helper()
	dir := t.TempDir()
	skillsDir = filepath.Join(dir, "skills")
	for _, name := range []string{"web-design", "git-commit"} {
		sub := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + name + "\ndescription: 测试。\n---\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, sub)
	}
	agentDir = filepath.Join(dir, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return skillsDir, agentDir, targets
}

func TestTogglePack(t *testing.T) {
	_, agentDir, targets := packFixture(t)

	if packLinked(agentDir, targets) {
		t.Error("packLinked = true with nothing linked")
	}
	// Nothing linked: the toggle links the whole pack.
	msg, err := togglePack(agentDir, targets, "Agent")
	if err != nil || msg != "" {
		t.Fatalf("togglePack = %q, %v", msg, err)
	}
	if !packLinked(agentDir, targets) {
		t.Fatal("after the toggle, the pack is not linked")
	}
	// All linked: the toggle removes the links.
	if msg, err = togglePack(agentDir, targets, "Agent"); err != nil || msg != "" {
		t.Fatalf("togglePack = %q, %v", msg, err)
	}
	for _, target := range targets {
		if got := skillLink(agentDir, target); got != linkNone {
			t.Errorf("after the second toggle, skillLink(%s) = %d", target, got)
		}
	}
	// A partial state links the rest.
	if err := os.Symlink(targets[0], filepath.Join(agentDir, "web-design")); err != nil {
		t.Fatal(err)
	}
	if packLinked(agentDir, targets) {
		t.Error("packLinked = true with one of two linked")
	}
	if msg, err = togglePack(agentDir, targets, "Agent"); err != nil || msg != "" {
		t.Fatalf("togglePack = %q, %v", msg, err)
	}
	if !packLinked(agentDir, targets) {
		t.Fatal("after the toggle of a partial state, the pack is not linked")
	}
	// A name taken by a real directory is skipped and named in the
	// message; what is linked stays.
	if err := os.Remove(filepath.Join(agentDir, "git-commit")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "git-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err = togglePack(agentDir, targets, "Agent")
	if err != nil {
		t.Fatalf("togglePack = %v", err)
	}
	if msg == "" {
		t.Error("no message for a taken name")
	}
	if fi, err := os.Lstat(filepath.Join(agentDir, "git-commit")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the real directory changed: %v", err)
	}
	if got := skillLink(agentDir, targets[0]); got != linkOurs {
		t.Errorf("the linked skill changed, skillLink = %d", got)
	}
}

func TestPackPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	a := &app{packsPath: path, packs: []pack{
		{Name: "前端", Skills: []string{"web-design", "git-commit"}},
		{Name: "文档", Skills: []string{"web-design"}},
	}}
	a.savePacks()

	b := &app{packsPath: path}
	b.loadPacks()
	if len(b.packs) != 2 || b.packs[0].Name != "前端" || len(b.packs[0].Skills) != 2 {
		t.Fatalf("loadPacks = %+v", b.packs)
	}

	// dropSkill takes the skill out of every pack and saves.
	b.dropSkill("web-design")
	if len(b.packs[0].Skills) != 1 || b.packs[0].Skills[0] != "git-commit" {
		t.Errorf("after dropSkill, pack = %+v", b.packs[0])
	}
	if len(b.packs[1].Skills) != 0 {
		t.Errorf("after dropSkill, pack = %+v", b.packs[1])
	}
	c := &app{packsPath: path}
	c.loadPacks()
	if len(c.packs[1].Skills) != 0 {
		t.Error("dropSkill did not persist")
	}
}

func TestMatchPack(t *testing.T) {
	p := &pack{Name: "前端助手", Skills: []string{"web-design"}}
	for _, q := range []string{"", "前端", "WEB", "design"} {
		if !matchPack(p, q) {
			t.Errorf("matchPack(%q) = false", q)
		}
	}
	if matchPack(p, "后端") {
		t.Error("matchPack(后端) = true")
	}
}

// packTargets resolves the pack's skills by their directories — the
// identity on disk — not by the names the SKILL.md declares, which two
// directories can share.
func TestPackTargets(t *testing.T) {
	a := &app{skillsDir: "/hub/skills", skills: []skill{
		{Name: "academy-guide", Dir: "academy-guide"},
		{Name: "academy-guide", Dir: "academy-guide-2"},
	}}
	p := &pack{Name: "p", Skills: []string{"academy-guide-2", "gone"}}
	targets := a.packTargets(p)
	if len(targets) != 1 || targets[0] != "/hub/skills/academy-guide-2" {
		t.Errorf("packTargets = %v", targets)
	}
}

// The 技能包 pane: making a pack, adding skills to it, linking it to an
// agent and taking a skill out of it again.
func TestPacksPane(t *testing.T) {
	skillsDir, agentDir, targets := packFixture(t)
	a := &app{selected: "packs", focusSide: "sidebar", skillsDir: skillsDir, packEdit: -1}
	a.loadSkills()
	a.agents = []agent{{Name: "Agent A", Dir: agentDir}}
	tt := ui.NewTester(a.view, 1000, 640)

	if !tt.HasText("还没有技能包") {
		t.Errorf("no empty state, texts %q", tt.Texts())
	}

	// Make a pack in the modal.
	if err := tt.Click("新建技能包…"); err != nil {
		t.Fatal(err)
	}
	if !a.packOpen {
		t.Fatal("the modal did not open")
	}
	if err := tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("前端")
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}
	if a.packOpen {
		t.Error("the modal stayed open")
	}
	if len(a.packs) != 1 || a.packs[0].Name != "前端" {
		t.Fatalf("packs = %+v", a.packs)
	}
	if !tt.HasText("0 个技能") {
		t.Errorf("no size tag, texts %q", tt.Texts())
	}

	// A duplicate name shows in red and keeps the button still.
	if err := tt.Click("新建技能包…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("前端")
	if !tt.HasText("已存在同名技能包") {
		t.Errorf("no duplicate warning, texts %q", tt.Texts())
	}
	tt.Click("创建") // disabled: does nothing
	if len(a.packs) != 1 {
		t.Error("a duplicate pack was made")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}

	// An empty pack links nothing; the toast says why.
	if err := tt.Click("Agent A"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("技能包为空，请先添加技能") {
		t.Errorf("no toast for an empty pack, texts %q", tt.Texts())
	}

	// Add the hub's skills in the 添加技能 modal; it stays open.
	if err := tt.Click("添加技能到技能包"); err != nil {
		t.Fatal(err)
	}
	if !a.pickOpen {
		t.Fatal("the pick modal did not open")
	}
	if !tt.HasText("添加技能到 前端") {
		t.Errorf("no modal title, texts %q", tt.Texts())
	}
	if err := tt.Click("web-design"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("git-commit"); err != nil {
		t.Fatal(err)
	}
	if len(a.packs[0].Skills) != 2 {
		t.Fatalf("pack skills = %v", a.packs[0].Skills)
	}
	if !tt.HasText("已添加") {
		t.Errorf("no 已添加 tag, texts %q", tt.Texts())
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2 个技能") {
		t.Errorf("no size tag after adding, texts %q", tt.Texts())
	}

	// The agent's capsule links the whole pack, then unlinks it.
	if err := tt.Click("Agent A"); err != nil {
		t.Fatal(err)
	}
	if !packLinked(agentDir, targets) {
		t.Fatal("after a click on the capsule, the pack is not linked")
	}
	if err := tt.Click("Agent A"); err != nil {
		t.Fatal(err)
	}
	if packLinked(agentDir, targets) {
		t.Fatal("after a second click, the pack is still linked")
	}

	// Clicking the capsule itself — anywhere but the × — removes
	// nothing.
	r, ok := tt.Find("web-design")
	if !ok {
		t.Fatal("no web-design capsule")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if len(a.packs[0].Skills) != 2 {
		t.Fatalf("a click on the capsule removed a skill: %v", a.packs[0].Skills)
	}

	// Only the capsule's × takes the skill out of the pack; the skill
	// stays.
	if err := tt.Click("从技能包移除 web-design"); err != nil {
		t.Fatal(err)
	}
	if len(a.packs[0].Skills) != 1 || a.packs[0].Skills[0] != "git-commit" {
		t.Fatalf("after the removal, pack skills = %v", a.packs[0].Skills)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "web-design")); err != nil {
		t.Error("the skill went with the removal")
	}

	// A skill the hub no longer has shows as 已丢失; its click removes it.
	// A fresh tester rebuilds the view with the mutation.
	a.packs[0].Skills = append(a.packs[0].Skills, "gone-skill")
	tt = ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("gone-skill") {
		t.Fatal("the missing skill does not show")
	}
	if err := tt.Click("「gone-skill」已丢失，点击从技能包移除"); err != nil {
		t.Fatal(err)
	}
	if len(a.packs[0].Skills) != 1 {
		t.Fatalf("after the removal, pack skills = %v", a.packs[0].Skills)
	}

	// The search filters the packs.
	if err := tt.Click("搜索技能包"); err != nil {
		t.Fatal(err)
	}
	tt.Type("zzz")
	if !tt.HasText("没有匹配的技能包") {
		t.Errorf("no empty state after a search matching nothing, texts %q", tt.Texts())
	}
}

// The 删除 of a pack asks first, and only the pack goes.
func TestPackDelete(t *testing.T) {
	skillsDir, _, _ := packFixture(t)
	a := &app{selected: "packs", focusSide: "sidebar", skillsDir: skillsDir, packEdit: -1}
	a.loadSkills()
	a.packs = []pack{{Name: "前端", Skills: []string{"web-design"}}}
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("删除技能包"); err != nil {
		t.Fatal(err)
	}
	if !a.delPackOpen {
		t.Fatal("the confirmation did not open")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if len(a.packs) != 1 {
		t.Error("取消 deleted the pack")
	}
	if err := tt.Click("删除技能包"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if len(a.packs) != 0 {
		t.Fatal("the pack stayed")
	}
	if !tt.HasText("还没有技能包") {
		t.Errorf("no empty state after the deletion, texts %q", tt.Texts())
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "web-design")); err != nil {
		t.Error("the skill went with the pack")
	}
}

// The 重命名 of a pack opens the modal with the name in it.
func TestPackRename(t *testing.T) {
	a := &app{selected: "packs", focusSide: "sidebar", packEdit: -1}
	a.packs = []pack{{Name: "前端"}}
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("重命名技能包"); err != nil {
		t.Fatal(err)
	}
	if !a.packOpen || a.packEdit != 0 {
		t.Fatalf("the modal did not open for the rename: open=%v edit=%d", a.packOpen, a.packEdit)
	}
	if a.packName != "前端" {
		t.Errorf("packName = %q", a.packName)
	}
	a.packName = "后端"
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.packs[0].Name != "后端" {
		t.Errorf("after the rename, packs = %+v", a.packs)
	}
}
