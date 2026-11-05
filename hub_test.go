package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseSkill(t *testing.T) {
	sk := parseSkill([]byte("---\nname: web-design\ndescription: 生成高保真网页界面。\n---\n\n# Web Design\n"), "dir-name")
	if sk.Name != "web-design" || sk.Desc != "生成高保真网页界面。" {
		t.Errorf("parseSkill = %+v", sk)
	}
	// The directory's name where the front matter says none, and quotes
	// come off.
	sk = parseSkill([]byte("---\ndescription: 'quoted desc'\n---\n"), "dir-name")
	if sk.Name != "dir-name" || sk.Desc != "quoted desc" {
		t.Errorf("parseSkill = %+v", sk)
	}
	// No front matter at all.
	sk = parseSkill([]byte("# Just a title\n"), "dir-name")
	if sk.Name != "dir-name" || sk.Desc != "" {
		t.Errorf("parseSkill = %+v", sk)
	}
	// A folded block scalar joins its lines with spaces.
	sk = parseSkill([]byte("---\nname: x\ndescription: >\n  First line\n  second line.\n  \n  New paragraph.\n---\n"), "d")
	if sk.Desc != "First line second line. New paragraph." {
		t.Errorf("parseSkill folded = %q", sk.Desc)
	}
	// A literal one keeps the line breaks.
	sk = parseSkill([]byte("---\ndescription: |-\n  one\n  two\n---\n"), "d")
	if sk.Desc != "one\ntwo" {
		t.Errorf("parseSkill literal = %q", sk.Desc)
	}
}

func TestLoadSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill := func(name, md string) {
		sub := filepath.Join(dir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill("b-skill", "---\nname: bee\ndescription: 第二个。\n---\n")
	writeSkill("a-skill", "---\nname: ant\ndescription: 第一个。\n---\n")
	// The record of where a skill came from makes its source tag.
	meta := `{"source":"git","url":"https://github.com/o/r"}`
	if err := os.WriteFile(filepath.Join(dir, "a-skill", importMetaFile), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory without a SKILL.md and a hidden one are no skills.
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "no-md"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app{skillsDir: dir}
	a.loadSkills()
	if len(a.skills) != 2 {
		t.Fatalf("loadSkills = %d skills", len(a.skills))
	}
	// Sorted by name.
	if a.skills[0].Name != "ant" || a.skills[1].Name != "bee" {
		t.Errorf("loadSkills = %+v", a.skills)
	}
	if a.skills[0].Dir != "a-skill" || a.skills[1].Dir != "b-skill" {
		t.Errorf("dirs = %q, %q", a.skills[0].Dir, a.skills[1].Dir)
	}
	if a.skills[0].Source != "github.com" || a.skills[1].Source != "未知来源" {
		t.Errorf("sources = %q, %q", a.skills[0].Source, a.skills[1].Source)
	}
}

func TestMatchSkill(t *testing.T) {
	sk := skill{Name: "pdf-tools", Desc: "读取、提取与转换 PDF 文档的内容。"}
	agents := []agent{{Name: "Kimi"}, {Name: "Work Buddy"}}
	for _, q := range []string{"", "pdf", "PDF", "读取", "kimi", "work"} {
		if !matchSkill(sk, q, agents) {
			t.Errorf("matchSkill(%q) = false", q)
		}
	}
	for _, q := range []string{"claude", "图片"} {
		if matchSkill(sk, q, agents) {
			t.Errorf("matchSkill(%q) = true", q)
		}
	}
}

// The hub shows the skills as cards and the search field filters them,
// down to the empty state when nothing matches.
func TestHub(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	agentDir := filepath.Join(dir, "agent-skills")
	writeSkill := func(name, desc string) {
		sub := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + name + "\ndescription: " + desc + "\n---\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill("web-design", "生成高保真网页界面。")
	writeSkill("git-commit", "撰写规范的提交信息。")
	writeSkill("pdf-tools", "读取、提取与转换 PDF 文档的内容。")

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.agents = []agent{{Name: "通用 Agent", Dir: agentDir}}
	tt := ui.NewTester(a.view, 1000, 640)
	for _, s := range []string{"技能中心", "导入技能…", "点击技能下方 agent 徽标", "web-design", "git-commit", "通用 Agent"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
	if err := tt.Click("搜索技能"); err != nil {
		t.Fatal(err)
	}
	// The capsule's ring is the background's color until the input takes
	// the focus, which turns it light blue; it is as wide in both
	// states, so the focus shifts nothing.
	r, ok := tt.Find("搜索技能")
	if !ok {
		t.Fatal("no 搜索技能")
	}
	edge := func() [4]byte {
		img := tt.Image()
		off := img.PixOffset(int(r.X)+1, int(r.Y+r.H/2))
		return [4]byte{img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]}
	}
	focused := edge()
	tt.ClickAt(500, 400) // the pane takes the focus back
	if blurred := edge(); blurred == focused {
		t.Errorf("the ring of the search capsule is %v without the focus, was %v", blurred, focused)
	}
	if r2, _ := tt.Find("搜索技能"); r2 != r {
		t.Errorf("the search capsule moved with the focus: %+v, was %+v", r2, r)
	}
	if err := tt.Click("搜索技能"); err != nil {
		t.Fatal(err)
	}
	tt.Type("pdf")
	if !tt.HasText("pdf-tools") {
		t.Errorf("no pdf-tools after a search for pdf, texts %q", tt.Texts())
	}
	if tt.HasText("web-design") {
		t.Error("web-design after a search for pdf")
	}

	// The capsule tag of the agent toggles the link of the skill.
	link := filepath.Join(agentDir, "pdf-tools")
	if err := tt.Click("通用 Agent"); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "pdf-tools")); got != linkOurs {
		t.Errorf("after a click on the capsule, skillLink = %d", got)
	}
	if dst, err := os.Readlink(link); err != nil || dst != filepath.Join(skillsDir, "pdf-tools") {
		t.Errorf("the link is %q, %v", dst, err)
	}
	if err := tt.Click("通用 Agent"); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, filepath.Join(skillsDir, "pdf-tools")); got != linkNone {
		t.Errorf("after a second click, skillLink = %d", got)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the link is still there: %v", err)
	}

	if err := tt.Click("搜索技能"); err != nil { // the capsule took the focus
		t.Fatal(err)
	}
	tt.Type("zzz")
	if !tt.HasText("没有匹配的技能") {
		t.Errorf("no empty state after a search matching nothing, texts %q", tt.Texts())
	}
}

// The hub's empty state while the skills directory has no skills.
func TestHubEmpty(t *testing.T) {
	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: t.TempDir()}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("还没有技能") {
		t.Errorf("no empty state, texts %q", tt.Texts())
	}
}

// A skill whose directory is not its name — two directories can declare
// the same name — shows its directory beside the name, and everything it
// does goes by the directory: the link points at it, the deletion
// removes it.
func TestHubDirIdentity(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	agentDir := filepath.Join(dir, "agent-skills")
	sub := filepath.Join(skillsDir, "academy-guide-2")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: academy-guide\ndescription: 测试。\n---\n"
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.agents = []agent{{Name: "通用 Agent", Dir: agentDir}}
	tt := ui.NewTester(a.view, 1000, 640)

	if !tt.HasText("(academy-guide-2)") {
		t.Errorf("the directory does not show beside the name, texts %q", tt.Texts())
	}
	// The capsule links the directory, not the skill's name.
	if err := tt.Click("通用 Agent"); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, sub); got != linkOurs {
		t.Fatalf("after a click on the capsule, skillLink = %d", got)
	}
	if dst, err := os.Readlink(filepath.Join(agentDir, "academy-guide-2")); err != nil || dst != sub {
		t.Errorf("the link is %q, %v", dst, err)
	}

	// The deletion removes the directory, and nothing else of the name.
	r, ok := tt.Find("academy-guide")
	if !ok {
		t.Fatal("no card")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	if err := tt.Click("删除 academy-guide"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(sub); !os.IsNotExist(err) {
		t.Error("the skill's directory stayed")
	}
	if _, err := os.Lstat(filepath.Join(agentDir, "academy-guide-2")); !os.IsNotExist(err) {
		t.Error("the agent's link stayed")
	}
}

// The × of a card rests at its top right — dimmed until the pointer is
// near — and asks before it deletes the skill: the agents' links go
// first, then the skill's directory.
func TestHubDelete(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	for _, name := range []string{"web-design", "git-commit"} {
		sub := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + name + "\ndescription: 测试。\n---\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	agentDir := filepath.Join(dir, "agent-skills")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(skillsDir, "web-design"), filepath.Join(agentDir, "web-design")); err != nil {
		t.Fatal(err)
	}
	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.agents = []agent{{Name: "通用 Agent", Dir: agentDir}}
	tt := ui.NewTester(a.view, 1000, 640)

	// The × shows without the pointer on the card, dimmed.
	if !tt.HasText("删除 web-design") {
		t.Error("no × on the card without the pointer")
	}
	if _, ok := tt.Find("web-design"); !ok {
		t.Fatal("no web-design card")
	}
	if err := tt.Click("删除 web-design"); err != nil {
		t.Fatal(err)
	}
	if !a.delOpen {
		t.Fatal("the confirmation did not open")
	}
	if !tt.HasText("此操作无法撤销") {
		t.Errorf("no confirmation, texts %q", tt.Texts())
	}
	// Canceling keeps everything.
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "web-design")); err != nil {
		t.Error("取消 deleted the skill")
	}

	// Confirming deletes the link, then the skill.
	if err := tt.Click("删除 web-design"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(agentDir, "web-design")); !os.IsNotExist(err) {
		t.Error("the agent's link stayed")
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "web-design")); !os.IsNotExist(err) {
		t.Error("the skill's directory stayed")
	}
	if tt.HasText("web-design") {
		t.Error("the deleted skill still shows")
	}
	if !tt.HasText("git-commit") {
		t.Error("the other skill went too")
	}
}

// loadSkills reads only the head of a SKILL.md: a front matter beyond
// the cap falls back to the directory's name instead of reading on.
func TestLoadSkillsHeadCap(t *testing.T) {
	dir := t.TempDir()
	// A huge comment pushes the front matter's end past the cap.
	big := "---\nname: huge\n# " + strings.Repeat("x", skillHeadLen) + "\n---\n"
	sub := filepath.Join(dir, "big-skill")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{skillsDir: dir}
	a.loadSkills()
	if len(a.skills) != 1 || a.skills[0].Name != "big-skill" {
		t.Errorf("loadSkills = %+v", a.skills)
	}
}
