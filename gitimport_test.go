package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestParseGitURL(t *testing.T) {
	cases := []struct {
		raw, clone, rest string
	}{
		{"owner/repo", "https://github.com/owner/repo", ""},
		{" owner/repo ", "https://github.com/owner/repo", ""},
		{"https://github.com/owner/repo", "https://github.com/owner/repo", ""},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo", ""},
		{"https://github.com/owner/repo/", "https://github.com/owner/repo", ""},
		{"https://github.com/owner/repo/tree/main", "https://github.com/owner/repo", "main"},
		{"https://github.com/owner/repo/tree/main/skills/pdf", "https://github.com/owner/repo", "main/skills/pdf"},
		{"https://github.com/owner/repo.git/tree/main/x", "https://github.com/owner/repo", "main/x"},
		{"https://gitlab.com/group/repo/-/tree/dev/x", "https://gitlab.com/group/repo", "dev/x"},
		{"https://example.com/git/r.git", "https://example.com/git/r", ""},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git", ""},
	}
	for _, tc := range cases {
		clone, rest, err := parseGitURL(tc.raw)
		if err != nil {
			t.Errorf("parseGitURL(%q): %v", tc.raw, err)
			continue
		}
		if clone != tc.clone || rest != tc.rest {
			t.Errorf("parseGitURL(%q) = %q, %q; want %q, %q", tc.raw, clone, rest, tc.clone, tc.rest)
		}
	}
	for _, bad := range []string{"", "  ", "not-a-url", "a/b/c"} {
		if _, _, err := parseGitURL(bad); err == nil {
			t.Errorf("parseGitURL(%q) = nil error", bad)
		}
	}
}

func TestResolveTreeRest(t *testing.T) {
	branches := []string{"main", "dev", "feature/x"}
	cases := []struct {
		rest, branch, wantBranch, wantSub string
	}{
		{"", "", "", ""},
		{"main/skills/pdf", "", "main", "skills/pdf"},
		{"feature/x/doc", "", "feature/x", "doc"},
		{"unknown/path", "", "unknown", "path"},
		{"main", "dev", "dev", "main"},       // the user's branch wins
		{"a/b", "release", "release", "a/b"}, // …over the whole tail
	}
	for _, tc := range cases {
		branch, sub := resolveTreeRest(branches, tc.rest, tc.branch)
		if branch != tc.wantBranch || sub != tc.wantSub {
			t.Errorf("resolveTreeRest(%q, %q) = %q, %q; want %q, %q",
				tc.rest, tc.branch, branch, sub, tc.wantBranch, tc.wantSub)
		}
	}
}

func TestFindGitSkills(t *testing.T) {
	root := t.TempDir()
	write := func(rel, md string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/pdf/SKILL.md", "---\nname: pdf-tools\ndescription: PDF。\n---\n")
	// A skill is not walked deeper: the nested SKILL.md is no new skill.
	write("skills/pdf/examples/nested/SKILL.md", "---\nname: nested\n---\n")
	write("single/SKILL.md", "# no front matter\n")
	write(".hidden/skill/SKILL.md", "---\nname: hidden\n---\n")
	write(".git/things/SKILL.md", "---\nname: git\n---\n")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	found := findGitSkills(root)
	if len(found) != 2 {
		t.Fatalf("findGitSkills = %v", found)
	}
	byName := map[string]foundSkill{}
	for _, f := range found {
		byName[f.Name] = f
	}
	if f, ok := byName["pdf-tools"]; !ok || f.Desc != "PDF。" {
		t.Errorf("pdf-tools = %+v, %v", f, ok)
	}
	// Without a name in the front matter the directory names the skill.
	if _, ok := byName["single"]; !ok {
		t.Errorf("no single in %v", found)
	}
}

// stubParse swaps the background machinery of the 解析 for synchronous
// stubs, and reports through applied when a result has been delivered.
func stubParse(t *testing.T, fetch func(context.Context, string, string) (*gitResult, error)) (applied chan struct{}) {
	t.Helper()
	applied = make(chan struct{}, 1)
	oldFetch, oldRun := gitFetch, runOnMain
	t.Cleanup(func() { gitFetch, runOnMain = oldFetch, oldRun })
	gitFetch = fetch
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	return applied
}

func TestImportSkill(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extra.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsDir := t.TempDir()
	meta := importMeta{Source: "git", URL: "https://github.com/o/r", Commit: "abc123"}
	if err := importSkill(src, skillsDir, "demo", meta); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"SKILL.md", "extra.txt", importMetaFile} {
		if _, err := os.Stat(filepath.Join(skillsDir, "demo", f)); err != nil {
			t.Errorf("no %s after the import: %v", f, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(skillsDir, "demo", importMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var got importMeta
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "git" || got.URL != meta.URL || got.Commit != "abc123" {
		t.Errorf("importMeta = %+v", got)
	}

	// Importing the name again overwrites: the new content lands, what
	// only the old skill had goes away.
	if err := os.WriteFile(filepath.Join(skillsDir, "demo", "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: demo\ndescription: 新版。\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(src, "extra.txt")); err != nil {
		t.Fatal(err)
	}
	if err := importSkill(src, skillsDir, "demo", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "demo", "stale.txt")); !os.IsNotExist(err) {
		t.Error("the overwrite kept a stale file")
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "demo", "extra.txt")); !os.IsNotExist(err) {
		t.Error("the overwrite kept extra.txt")
	}
	if data, err := os.ReadFile(filepath.Join(skillsDir, "demo", "SKILL.md")); err != nil || string(data) != "---\nname: demo\ndescription: 新版。\n---\n" {
		t.Errorf("SKILL.md after the overwrite: %q, %v", data, err)
	}

	// The source being the destination — a link to the hub's skill
	// imported again, the hub picked as a local folder — is refused,
	// and the skill stays.
	if err := importSkill(filepath.Join(skillsDir, "demo"), skillsDir, "demo", meta); err == nil {
		t.Error("importing the hub's own skill worked")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "demo", "SKILL.md")); err != nil {
		t.Error("the self-import deleted the skill:", err)
	}
	// Through a link too: the resolution follows it.
	linkDir := t.TempDir()
	if err := os.Symlink(filepath.Join(skillsDir, "demo"), filepath.Join(linkDir, "demo")); err != nil {
		t.Skip("no symbolic links:", err)
	}
	if err := importSkill(filepath.Join(linkDir, "demo"), skillsDir, "demo", meta); err == nil {
		t.Error("importing a link to the hub's skill worked")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "demo", "SKILL.md")); err != nil {
		t.Error("the import through the link deleted the skill:", err)
	}
}

func TestSourceLabel(t *testing.T) {
	cases := []struct {
		meta importMeta
		ok   bool
		want string
	}{
		{importMeta{}, false, "未知来源"},
		{importMeta{Source: "git", URL: "https://github.com/o/r"}, true, "github.com"},
		{importMeta{Source: "git", URL: "git@gitlab.com:o/r.git"}, true, "gitlab.com"},
		{importMeta{Source: "git", URL: "ssh://git@example.com:2222/o/r.git"}, true, "example.com"},
		{importMeta{Source: "git", URL: "weird"}, true, "Git"},
		{importMeta{Source: "url", URL: "https://example.com/x/SKILL.md"}, true, "example.com"},
		{importMeta{Source: "url", URL: "::bad"}, true, "链接"},
		{importMeta{Source: "local"}, true, "本地文件"},
		{importMeta{Source: "something"}, true, "未知来源"},
	}
	for _, tc := range cases {
		if got := sourceLabel(tc.meta, tc.ok); got != tc.want {
			t.Errorf("sourceLabel(%+v, %v) = %q; want %q", tc.meta, tc.ok, got, tc.want)
		}
	}
}

// The GitHub way's flow: 解析 lists the clone's skills in the background,
// the hub's own marked 已存在 and unchosen, and 导入 copies the chosen
// ones over what the hub has, recording the source.
func TestImportGitFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	// The hub already has one of the skills the clone offers, with a
	// stale file of its own.
	own := filepath.Join(skillsDir, "pdf-tools")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("---\nname: pdf-tools\ndescription: 旧版。\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The stubbed clone.
	clone := t.TempDir()
	for _, sk := range []struct{ dir, md string }{
		{"pdf", "---\nname: pdf-tools\ndescription: 新版。\n---\n"},
		{"art", "---\nname: algorithmic-art\ndescription: 艺术。\n---\n"},
	} {
		sub := filepath.Join(clone, sk.dir)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(sk.md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	applied := stubParse(t, func(_ context.Context, rawURL, branch string) (*gitResult, error) {
		if rawURL != "o/r" || branch != "dev" {
			t.Errorf("gitFetch = %q, %q", rawURL, branch)
		}
		return &gitResult{root: clone, commit: "abc123", skills: findGitSkills(clone)}, nil
	})

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.importOpen, a.importRepo, a.importBranch = true, "o/r", "dev"
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	// pdf-tools exists in the hub: 已存在 and unchosen; algorithmic-art
	// is new and chosen.
	for _, s := range []string{"发现 2 个技能 · 已选 1 个", "pdf-tools", "algorithmic-art", "已存在", "导入 1 个技能"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q after 解析, texts %q", s, tt.Texts())
		}
	}

	// Choosing the existing one imports it too, over what the hub has.
	// The card behind the modal is pdf-tools too, so the checkbox's
	// label is the target.
	if err := tt.Click("选择 pdf-tools"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("导入 2 个技能") {
		t.Fatalf("no 导入 2 个技能 after choosing pdf-tools, texts %q", tt.Texts())
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
	if a.importClone != "" {
		t.Error("the clone was not cleaned up")
	}
	// The new one is in the hub, with its source recorded.
	data, err := os.ReadFile(filepath.Join(skillsDir, "algorithmic-art", importMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta importMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Source != "git" || meta.URL != "o/r" || meta.Commit != "abc123" {
		t.Errorf("importMeta = %+v", meta)
	}
	// The existing one was overwritten: new content, the stale file
	// gone, the source recorded anew.
	if data, err := os.ReadFile(filepath.Join(skillsDir, "pdf-tools", "SKILL.md")); err != nil || string(data) != "---\nname: pdf-tools\ndescription: 新版。\n---\n" {
		t.Errorf("SKILL.md after the overwrite: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "pdf-tools", "stale.txt")); !os.IsNotExist(err) {
		t.Error("the overwrite kept the stale file")
	}
	if len(a.skills) != 2 || a.skills[1].Source != "github.com" {
		t.Errorf("loadSkills after 导入 = %+v", a.skills)
	}

	// A failed 解析 shows in the modal.
	applied = stubParse(t, func(context.Context, string, string) (*gitResult, error) {
		return nil, os.ErrNotExist
	})
	a.importOpen = true
	tt.Frame()
	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if a.importErr == "" || !tt.HasText(a.importErr) {
		t.Errorf("no error in the modal, texts %q", tt.Texts())
	}
}

// Closing the modal while the 解析 runs cancels it: no result lands, no
// error shows, and the clone is removed.
func TestParseGitCancel(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	applied := stubParse(t, func(ctx context.Context, _, _ string) (*gitResult, error) {
		close(started)
		defer close(finished)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	a := &app{selected: "hub", focusSide: "sidebar", importOpen: true}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-started
	if !a.importParsing {
		t.Fatal("not parsing after 解析")
	}
	if !tt.HasText("解析中…") {
		t.Errorf("no 解析中… while parsing, texts %q", tt.Texts())
	}
	a.closeImport()
	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		t.Fatal("the canceled 解析 never delivered")
	}
	<-finished
	if a.importErr != "" {
		t.Errorf("the canceled 解析 left an error: %q", a.importErr)
	}
	if a.importParsing || a.importCancel != nil {
		t.Error("the canceled 解析 still runs")
	}
	if len(a.importParsed) != 0 {
		t.Error("the canceled 解析 left skills")
	}
}

// The 全选 and 取消选择 of the parsed list choose all of it or none.
func TestImportSelectAll(t *testing.T) {
	clone := t.TempDir()
	for _, sk := range []string{"one", "two"} {
		sub := filepath.Join(clone, sk)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + sk + "\n---\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	applied := stubParse(t, func(context.Context, string, string) (*gitResult, error) {
		return &gitResult{root: clone, commit: "abc", skills: findGitSkills(clone)}, nil
	})
	a := &app{selected: "hub", focusSide: "sidebar", importOpen: true}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	// The clone's two are new, so all are chosen already: 全选 does not
	// apply, 取消选择 does.
	if !tt.HasText("发现 2 个技能 · 已选 2 个") {
		t.Fatalf("no status after 解析, texts %q", tt.Texts())
	}
	if err := tt.Click("取消选择"); err != nil {
		t.Fatal(err)
	}
	if a.importCount() != 0 || !tt.HasText("发现 2 个技能 · 已选 0 个") {
		t.Errorf("取消选择 = %d chosen, texts %q", a.importCount(), tt.Texts())
	}
	if !tt.HasText("导入") || tt.HasText("导入 1 个技能") {
		t.Errorf("the import button after 取消选择, texts %q", tt.Texts())
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatal(err)
	}
	if a.importCount() != 2 || !tt.HasText("导入 2 个技能") {
		t.Errorf("全选 = %d chosen, texts %q", a.importCount(), tt.Texts())
	}
}

// The 解析 button and the ways to import disable while the 解析 runs.
func TestParseGitDisablesUI(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stubParse(t, func(ctx context.Context, _, _ string) (*gitResult, error) {
		close(started)
		<-release
		return nil, fmt.Errorf("done")
	})
	a := &app{selected: "hub", focusSide: "sidebar", importOpen: true}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-started
	// The tester clicks what is disabled too; what matters is nothing
	// responds.
	gen := a.parseGen
	_ = tt.Click("解析中…")
	if a.parseGen != gen {
		t.Error("a second 解析 started while parsing")
	}
	_ = tt.Click("命令行")
	if a.importMethod != 0 {
		t.Error("the way switched while parsing")
	}
	close(release)
}
