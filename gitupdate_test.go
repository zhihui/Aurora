package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestRepoLabel(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/owner/repo":         "owner/repo",
		"https://github.com/owner/repo.git":     "owner/repo",
		"git@github.com:owner/repo.git":         "owner/repo",
		"https://gitlab.com/group/sub/repo":     "sub/repo",
		"ssh://git@host.example.com/owner/repo": "owner/repo",
	} {
		if got := repoLabel(url); got != want {
			t.Errorf("repoLabel(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestSkillBranch(t *testing.T) {
	refs := &repoRefs{branches: map[string]string{"main": "h1", "dev": "h2", "feat/x": "h3"}, head: "h1"}
	// The branch the import tracked wins.
	if b := skillBranch(importMeta{Branch: "dev", URL: "https://github.com/o/r"}, refs); b != "dev" {
		t.Errorf("tracked branch = %q", b)
	}
	// The link's /tree/ tail names one, the longest matching prefix.
	if b := skillBranch(importMeta{URL: "https://github.com/o/r/tree/feat/x/skills/a"}, refs); b != "feat/x" {
		t.Errorf("tree branch = %q", b)
	}
	// A bare link takes the default branch, the one the remote's HEAD
	// names.
	if b := skillBranch(importMeta{URL: "https://github.com/o/r"}, refs); b != "main" {
		t.Errorf("default branch = %q", b)
	}
	// A remote that names no HEAD leaves the usual guesses.
	refs = &repoRefs{branches: map[string]string{"master": "h9"}}
	if b := skillBranch(importMeta{URL: "https://github.com/o/r"}, refs); b != "master" {
		t.Errorf("fallback branch = %q", b)
	}
}

// gitSkill is a hub skill as a git import left it, for the check's
// targets.
func gitSkill(dir, url, commit string) skill {
	return skill{
		Name: dir, Dir: dir, HasMeta: true,
		Meta: importMeta{Source: "git", URL: url, Commit: commit},
	}
}

// stubRefs swaps the remote calls for synchronous stubs: refs answers
// by repository, the returned map counting the calls.
func stubRefs(t *testing.T, refs map[string]*repoRefs) map[string]int {
	t.Helper()
	calls := map[string]int{}
	old := listRemoteRefs
	t.Cleanup(func() { listRemoteRefs = old })
	listRemoteRefs = func(_ context.Context, cloneURL string) (*repoRefs, error) {
		calls[cloneURL]++
		r, ok := refs[cloneURL]
		if !ok {
			return nil, fmt.Errorf("no such repository")
		}
		return r, nil
	}
	return calls
}

// The check groups the skills of a repository — whatever the links'
// shapes — into one clone's worth, one remote call; the up-to-date stay
// out, the unreachable are reported.
func TestCheckTargets(t *testing.T) {
	repo := "https://github.com/o/r"
	calls := stubRefs(t, map[string]*repoRefs{
		repo: {branches: map[string]string{"main": "new", "dev": "devnew"}, head: "new"},
	})
	targets := []skill{
		gitSkill("a", "https://github.com/o/r/tree/main/skills/a", "old"),
		gitSkill("b", "o/r", "old"),                    // the shorthand is the same repository
		gitSkill("c", "https://github.com/o/r", "new"), // up to date
		gitSkill("d", "https://github.com/o/r/tree/dev/skills/d", "old"),
		gitSkill("e", "https://github.com/o/gone", "old"),
	}
	avail, groups, errs := checkTargets(context.Background(), targets)
	if calls[repo] != 1 {
		t.Errorf("remote calls = %v, one repository is listed once", calls)
	}
	if !avail["a"] || !avail["b"] || !avail["d"] || avail["c"] || avail["e"] {
		t.Errorf("avail = %v", avail)
	}
	// a and b share the main group; d is another branch, another group.
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[0].Branch != "main" || len(groups[0].Items) != 2 ||
		groups[1].Branch != "dev" || len(groups[1].Items) != 1 {
		t.Errorf("groups = %+v", groups)
	}
	if groups[0].Items[0].NewCommit != "new" || !groups[0].Items[0].Checked {
		t.Errorf("item = %+v", groups[0].Items[0])
	}
	if len(errs) != 1 {
		t.Errorf("errs = %v, the unreachable repository is reported once", errs)
	}
}

// The check reports a branch the repository no longer has.
func TestCheckTargetsBranchGone(t *testing.T) {
	repo := "https://github.com/o/r"
	stubRefs(t, map[string]*repoRefs{
		repo: {branches: map[string]string{"main": "h"}, head: "h"},
	})
	targets := []skill{{
		Name: "a", Dir: "a", HasMeta: true,
		Meta: importMeta{Source: "git", URL: repo, Commit: "old", Branch: "dev"},
	}}
	avail, groups, errs := checkTargets(context.Background(), targets)
	if len(avail) != 0 || len(groups) != 0 || len(errs) != 1 {
		t.Errorf("avail = %v, groups = %v, errs = %v", avail, groups, errs)
	}
}

func TestLocateUpdate(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "skills/a", "---\nname: alpha\n---\n")
	writeSkill(t, root, "other/b", "---\nname: beta\n---\n")

	// The recorded path leads straight there.
	src, rel := locateUpdate(root, updateItem{Dir: "a", Name: "alpha", Meta: importMeta{Path: "skills/a"}})
	if rel != "skills/a" || filepath.Base(src) != "a" {
		t.Errorf("recorded path = %q, %q", src, rel)
	}
	// A path the upstream no longer has finds nothing.
	if src, _ = locateUpdate(root, updateItem{Dir: "a", Name: "alpha", Meta: importMeta{Path: "skills/gone"}}); src != "" {
		t.Errorf("gone path = %q", src)
	}
	// Without a path the directory's name matches.
	src, rel = locateUpdate(root, updateItem{Dir: "b", Name: "whatever"})
	if rel != "other/b" || filepath.Base(src) != "b" {
		t.Errorf("dir fallback = %q, %q", src, rel)
	}
	// And the declared name where exactly one matches.
	src, _ = locateUpdate(root, updateItem{Dir: "x", Name: "alpha"})
	if filepath.Base(src) != "a" {
		t.Errorf("name fallback = %q", src)
	}
	// Two of a name is no match.
	writeSkill(t, root, "elsewhere/a2", "---\nname: alpha\n---\n")
	if src, _ = locateUpdate(root, updateItem{Dir: "x", Name: "alpha"}); src != "" {
		t.Errorf("ambiguous = %q", src)
	}
}

// stubUpdate swaps the update's network and main-thread delivery for
// synchronous stubs, reporting through applied when a result has been
// delivered.
func stubUpdate(t *testing.T, clones map[string]string) (applied chan struct{}) {
	t.Helper()
	applied = make(chan struct{}, 1)
	oldClone, oldRun := updateClone, runOnMain
	t.Cleanup(func() { updateClone, runOnMain = oldClone, oldRun })
	updateClone = func(_ context.Context, cloneURL, branch string) (string, string, error) {
		root, ok := clones[cloneURL+"\n"+branch]
		if !ok {
			return "", "", fmt.Errorf("no clone of %s %s", cloneURL, branch)
		}
		return root, "newcommit", nil
	}
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	return applied
}

// The update copies the clone over the hub's skill and the record gains
// the new commit, and the branch and path an old record never noted.
func TestRunUpdates(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	clone := filepath.Join(dir, "clone")
	writeSkill(t, skillsDir, "a", "---\nname: alpha\ndescription: 旧。\n---\n")
	writeSkill(t, clone, "skills/a", "---\nname: alpha\ndescription: 新。\n---\n")
	// The old record: no branch, no path.
	meta := importMeta{Source: "git", URL: "https://github.com/o/r", Commit: "old", ImportedAt: time.Now()}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(skillsDir, "a", importMetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := "https://github.com/o/r"
	applied := stubUpdate(t, map[string]string{repo + "\nmain": clone})

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	a.updateAvail = map[string]bool{"a": true}
	a.updateGroups = []updateGroup{{
		CloneURL: repo, Branch: "main",
		Items: []updateItem{{Dir: "a", Name: "alpha", Meta: meta, NewCommit: "newcommit", Checked: true}},
	}}
	a.updateOpen = true
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("更新 1 个技能"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()

	it := a.updateGroups[0].Items[0]
	if !it.Done || it.Err != "" {
		t.Fatalf("item = %+v", it)
	}
	if a.updateAvail["a"] {
		t.Errorf("the updated skill keeps no 有更新 mark")
	}
	if !a.updateFinished {
		t.Errorf("the modal does not show the results")
	}
	// The hub's skill is the clone's, its record complete.
	md, err := os.ReadFile(filepath.Join(skillsDir, "a", "SKILL.md"))
	if err != nil || !strings.Contains(string(md), "新。") {
		t.Errorf("SKILL.md = %q, %v", md, err)
	}
	got, ok := loadImportMeta(filepath.Join(skillsDir, "a"))
	if !ok || got.Commit != "newcommit" || got.Branch != "main" || got.Path != "skills/a" {
		t.Errorf("meta = %+v, %v", got, ok)
	}
	if !tt.HasText("已更新") {
		t.Errorf("no 已更新 in %q", tt.Texts())
	}
}

// A skill the upstream no longer has stays as it is, its failure listed.
func TestRunUpdatesGone(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	clone := filepath.Join(dir, "clone")
	writeSkill(t, skillsDir, "a", "---\nname: alpha\ndescription: 旧。\n---\n")
	writeSkill(t, clone, "skills/other", "---\nname: beta\n---\n")
	repo := "https://github.com/o/r"
	applied := stubUpdate(t, map[string]string{repo + "\nmain": clone})
	meta := importMeta{Source: "git", URL: repo, Commit: "old", Path: "skills/a"}

	a := &app{skillsDir: skillsDir}
	a.loadSkills()
	a.updateGroups = []updateGroup{{
		CloneURL: repo, Branch: "main",
		Items: []updateItem{{Dir: "a", Name: "alpha", Meta: meta, NewCommit: "new", Checked: true}},
	}}
	a.runUpdates()
	<-applied

	it := a.updateGroups[0].Items[0]
	if it.Done || it.Err == "" {
		t.Fatalf("item = %+v", it)
	}
	md, _ := os.ReadFile(filepath.Join(skillsDir, "a", "SKILL.md"))
	if !strings.Contains(string(md), "旧。") {
		t.Errorf("the gone skill changed: %q", md)
	}
}

// A failed clone fails its group, not the others.
func TestRunUpdatesGroupErr(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	clone := filepath.Join(dir, "clone")
	writeSkill(t, skillsDir, "a", "---\nname: alpha\n---\n")
	writeSkill(t, skillsDir, "b", "---\nname: beta\n---\n")
	writeSkill(t, clone, "b", "---\nname: beta\ndescription: 新。\n---\n")
	good := "https://github.com/o/good"
	applied := stubUpdate(t, map[string]string{good + "\nmain": clone})

	a := &app{skillsDir: skillsDir}
	a.loadSkills()
	a.updateGroups = []updateGroup{
		{CloneURL: "https://github.com/o/bad", Branch: "main", Items: []updateItem{{Dir: "a", Name: "alpha", Checked: true}}},
		{CloneURL: good, Branch: "main", Items: []updateItem{{Dir: "b", Name: "beta", Meta: importMeta{Source: "git", URL: good}, Checked: true}}},
	}
	a.runUpdates()
	<-applied

	if a.updateGroups[0].Err == "" || !a.updateGroups[1].Items[0].Done {
		t.Errorf("groups = %+v", a.updateGroups)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "b", importMetaFile)); err != nil {
		t.Errorf("no record written: %v", err)
	}
}

// The header's 检查更新 opens the modal with the check's groups, and the
// cards of the skills waiting wear 有更新.
func TestCheckUpdatesFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeSkill(t, skillsDir, "a", "---\nname: alpha\n---\n")
	meta := importMeta{Source: "git", URL: "https://github.com/o/r", Commit: "old", ImportedAt: time.Now()}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(skillsDir, "a", importMetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := "https://github.com/o/r"
	stubRefs(t, map[string]*repoRefs{
		repo: {branches: map[string]string{"main": "new"}, head: "new"},
	})
	applied := make(chan struct{}, 1)
	oldRun := runOnMain
	t.Cleanup(func() { runOnMain = oldRun })
	runOnMain = func(f func()) { f(); applied <- struct{}{} }

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if !a.updateOpen || len(a.updateGroups) != 1 || !a.updateAvail["a"] {
		t.Fatalf("open = %v, groups = %+v, avail = %v", a.updateOpen, a.updateGroups, a.updateAvail)
	}
	for _, s := range []string{"更新技能", "o/r", "main", "更新 1 个技能", "有更新"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
}

// The hub orders by time when 时间 is chosen: newest first, the
// unrecorded last; 名称 puts them back in alphabetical order.
func TestHubSortByTime(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	write := func(dirName string, meta *importMeta) {
		writeSkill(t, skillsDir, dirName, "---\nname: "+dirName+"\n---\n")
		if meta == nil {
			return
		}
		data, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(skillsDir, dirName, importMetaFile), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	write("old-one", &importMeta{Source: "git", URL: "o/r", ImportedAt: now.Add(-48 * time.Hour)})
	write("new-one", &importMeta{Source: "git", URL: "o/r", ImportedAt: now})
	write("no-record", nil)

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, skillSort: 1}
	a.loadSkills()
	// One column, so the order is in the rows.
	tt := ui.NewTester(a.view, 800, 640)
	rNew, _ := tt.Find("new-one")
	rOld, _ := tt.Find("old-one")
	rNone, _ := tt.Find("no-record")
	if !(rNew.Y < rOld.Y && rOld.Y < rNone.Y) {
		t.Errorf("order by time: new %v, old %v, none %v", rNew, rOld, rNone)
	}
	if !tt.HasText("更新于 " + now.Format("2006-01-02")) {
		t.Errorf("no 更新于 in %q", tt.Texts())
	}
	if err := tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	rNew, _ = tt.Find("new-one")
	rOld, _ = tt.Find("old-one")
	rNone, _ = tt.Find("no-record")
	if !(rNew.Y < rNone.Y && rNone.Y < rOld.Y) {
		t.Errorf("order by name: new %v, none %v, old %v", rNew, rNone, rOld)
	}
}
