package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSplitShell(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"npx skills add o/r", []string{"npx", "skills", "add", "o/r"}},
		{"  npx   skills  add  o/r  ", []string{"npx", "skills", "add", "o/r"}},
		{`add --skill "Convex Best Practices"`, []string{"add", "--skill", "Convex Best Practices"}},
		{"add 'my dir'", []string{"add", "my dir"}},
		{`add my\ dir`, []string{"add", "my dir"}},
		{`add "say \"hi\""`, []string{"add", `say "hi"`}},
		{"add 'it''s'", []string{"add", "its"}},
	}
	for _, tc := range cases {
		got, err := splitShell(tc.raw)
		if err != nil {
			t.Errorf("splitShell(%q): %v", tc.raw, err)
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("splitShell(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
	if _, err := splitShell(`add "unterminated`); err == nil {
		t.Error("splitShell of an open quote = nil error")
	}
}

func TestParseSkillsCmd(t *testing.T) {
	cases := []struct {
		raw    string
		source string
		skills []string
	}{
		{"npx skills add vercel-labs/agent-skills", "vercel-labs/agent-skills", nil},
		{"skills add https://github.com/o/r/tree/main/skills/pdf", "https://github.com/o/r/tree/main/skills/pdf", nil},
		{"npx -y skills@latest add git@github.com:o/r.git", "git@github.com:o/r.git", nil},
		{"npx skills add ./my-local-skills", "./my-local-skills", nil},
		{"npx skills add o/r --skill web-design --skill pdf-tools", "o/r", []string{"web-design", "pdf-tools"}},
		{`npx skills add o/r --skill "Convex Best Practices"`, "o/r", []string{"Convex Best Practices"}},
		{"npx skills add o/r --skill=*", "o/r", []string{"*"}},
		{"npx skills add --skill web-design o/r", "o/r", []string{"web-design"}},
		// The installation options do not steer an import.
		{"npx skills add o/r -g -y --all --copy -l", "o/r", nil},
		{"npx skills add o/r -a claude-code --agent opencode --agent=cursor", "o/r", nil},
	}
	for _, tc := range cases {
		spec, err := parseSkillsCmd(tc.raw)
		if err != nil {
			t.Errorf("parseSkillsCmd(%q): %v", tc.raw, err)
			continue
		}
		if spec.source != tc.source || !slices.Equal(spec.skills, tc.skills) {
			t.Errorf("parseSkillsCmd(%q) = %+v; want source %q skills %q", tc.raw, spec, tc.source, tc.skills)
		}
	}
	for _, bad := range []string{
		"", "   ", "npx skills", "ls -la", "npx skills add",
		"npx skills add o/r extra",
		"npx skills add o/r --skill",
	} {
		if _, err := parseSkillsCmd(bad); err == nil {
			t.Errorf("parseSkillsCmd(%q) = nil error", bad)
		}
	}
}

func TestLocalSkillDir(t *testing.T) {
	dir := t.TempDir()
	if got, local, err := localSkillDir(dir); !local || err != nil || got != dir {
		t.Errorf("localSkillDir(%q) = %q, %v, %v", dir, got, local, err)
	}
	if _, local, err := localSkillDir("./no-such-dir-anywhere"); !local || err == nil {
		t.Errorf("localSkillDir of a missing local path = %v, %v", local, err)
	}
	// owner/repo shorthand is no local path: git tries it.
	if _, local, err := localSkillDir("owner/repo"); local || err != nil {
		t.Errorf("localSkillDir(owner/repo) = %v, %v", local, err)
	}
}

func TestFilterFound(t *testing.T) {
	skills := []foundSkill{
		{skill: skill{Name: "a"}},
		{skill: skill{Name: "b"}},
	}
	if got, err := filterFound(skills, nil); err != nil || len(got) != 2 {
		t.Errorf("filterFound(nil) = %v, %v", got, err)
	}
	if got, err := filterFound(skills, []string{"*"}); err != nil || len(got) != 2 {
		t.Errorf("filterFound(*) = %v, %v", got, err)
	}
	if got, err := filterFound(skills, []string{"b"}); err != nil || len(got) != 1 || got[0].Name != "b" {
		t.Errorf("filterFound(b) = %v, %v", got, err)
	}
	if _, err := filterFound(skills, []string{"a", "nope"}); err == nil {
		t.Error("filterFound of a missing name = nil error")
	}
}

// writeSkill makes a directory with a SKILL.md under root.
func writeSkill(t *testing.T, root, dir, md string) {
	t.Helper()
	sub := filepath.Join(root, dir)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFetchCmd(t *testing.T) {
	src := t.TempDir()
	writeSkill(t, src, "one", "---\nname: one\ndescription: 一。\n---\n")
	writeSkill(t, src, "two", "---\nname: two\n---\n")

	// A local source is scanned where it lies: no clone to remove, no
	// commit, the source recorded as local.
	res, err := fetchCmd(context.Background(), cmdSpec{source: src})
	if err != nil {
		t.Fatal(err)
	}
	if res.source != "local" || res.root != "" || res.commit != "" || len(res.skills) != 2 {
		t.Errorf("fetchCmd(local) = %+v", res)
	}
	// The --skill picks filter it.
	res, err = fetchCmd(context.Background(), cmdSpec{source: src, skills: []string{"two"}})
	if err != nil || len(res.skills) != 1 || res.skills[0].Name != "two" {
		t.Errorf("fetchCmd(--skill two) = %+v, %v", res, err)
	}
	if _, err = fetchCmd(context.Background(), cmdSpec{source: src, skills: []string{"nope"}}); err == nil {
		t.Error("fetchCmd of a missing --skill = nil error")
	}
	if _, err = fetchCmd(context.Background(), cmdSpec{source: t.TempDir()}); err == nil {
		t.Error("fetchCmd of a skill-less directory = nil error")
	}

	// Anything else goes to the GitHub way's git machinery.
	old := gitFetch
	t.Cleanup(func() { gitFetch = old })
	clone := t.TempDir()
	gitFetch = func(_ context.Context, rawURL, branch string) (*gitResult, error) {
		if rawURL != "o/r" || branch != "" {
			t.Errorf("gitFetch = %q, %q", rawURL, branch)
		}
		return &gitResult{root: clone, commit: "abc", skills: []foundSkill{{skill: skill{Name: "one"}}}}, nil
	}
	res, err = fetchCmd(context.Background(), cmdSpec{source: "o/r"})
	if err != nil {
		t.Fatal(err)
	}
	if res.source != "git" || res.root != clone || res.commit != "abc" || len(res.skills) != 1 {
		t.Errorf("fetchCmd(git) = %+v", res)
	}
	// A --skill nothing answers to fails and the clone goes away.
	if _, err = fetchCmd(context.Background(), cmdSpec{source: "o/r", skills: []string{"nope"}}); err == nil {
		t.Error("fetchCmd(git, missing --skill) = nil error")
	}
	if _, err := os.Lstat(clone); !os.IsNotExist(err) {
		t.Error("the clone stayed after the failed filter")
	}
}

// The 命令行 way's flow: 解析 reads the `skills add` command and lists
// the skills of its source — a local directory here, so no stub — the
// hub's own marked 已存在 and unchosen; 导入 copies the chosen ones,
// recording the source, and never touches the directory they came from.
func TestImportCmdFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "hub-skills")
	writeSkill(t, skillsDir, "web-design", "---\nname: web-design\ndescription: 旧版。\n---\n")
	src := filepath.Join(dir, "source")
	writeSkill(t, src, "design", "---\nname: web-design\ndescription: 新版。\n---\n")
	writeSkill(t, src, "art", "---\nname: algorithmic-art\ndescription: 艺术。\n---\n")

	// The background 解析 delivers synchronously.
	applied := make(chan struct{}, 1)
	old := runOnMain
	t.Cleanup(func() { runOnMain = old })
	runOnMain = func(f func()) { f(); applied <- struct{}{} }

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, importOpen: true, importMethod: 1}
	a.loadSkills()
	a.importCmd = `npx skills add "` + src + `"`
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	for _, s := range []string{"发现 2 个技能 · 已选 1 个", "web-design", "algorithmic-art", "已存在", "导入 1 个技能"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q after 解析, texts %q", s, tt.Texts())
		}
	}

	// 全选 and 取消选择 choose all of the list or none, as the GitHub
	// way's do.
	if err := tt.Click("取消选择"); err != nil {
		t.Fatal(err)
	}
	if a.importCount() != 0 || !tt.HasText("发现 2 个技能 · 已选 0 个") {
		t.Errorf("取消选择 = %d chosen, texts %q", a.importCount(), tt.Texts())
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatal(err)
	}
	if a.importCount() != 2 || !tt.HasText("导入 2 个技能") {
		t.Fatalf("全选 = %d chosen, texts %q", a.importCount(), tt.Texts())
	}

	if err := tt.Click("导入 2 个技能"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已导入 2 个技能") {
		t.Errorf("no toast after 导入, texts %q", tt.Texts())
	}
	if a.importOpen {
		t.Error("the modal stayed open after 导入")
	}
	// Both landed, the hub's own overwritten, the source recorded as
	// local with the path the command gave.
	for _, name := range []string{"web-design", "algorithmic-art"} {
		data, err := os.ReadFile(filepath.Join(skillsDir, name, importMetaFile))
		if err != nil {
			t.Fatalf("no import record of %s: %v", name, err)
		}
		var meta importMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatal(err)
		}
		if meta.Source != "local" || meta.URL != src || meta.Commit != "" {
			t.Errorf("importMeta of %s = %+v", name, meta)
		}
	}
	if data, err := os.ReadFile(filepath.Join(skillsDir, "web-design", "SKILL.md")); err != nil || string(data) != "---\nname: web-design\ndescription: 新版。\n---\n" {
		t.Errorf("SKILL.md after the overwrite: %q, %v", data, err)
	}
	// The directory the command named is the user's own: untouched.
	if _, err := os.Stat(filepath.Join(src, "design", "SKILL.md")); err != nil {
		t.Error("the local source was touched")
	}

	// A mistyped command fails right away, without a fetch.
	a.importOpen = true
	a.importCmd = "not a command"
	tt.Frame()
	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	if a.importErr == "" || !tt.HasText(a.importErr) {
		t.Errorf("no error in the modal, texts %q", tt.Texts())
	}
	if a.importParsing {
		t.Error("parsing after the mistyped command")
	}
}
