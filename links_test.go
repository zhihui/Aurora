package main

import (
	"os"
	"path/filepath"
	"testing"
)

// linkFixture makes a skills directory with one skill in it and an
// agent's skills directory, in a temporary home.
func linkFixture(t *testing.T) (agentDir, target string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "skills", "web-design")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	agentDir = filepath.Join(dir, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return agentDir, target
}

func TestSkillLink(t *testing.T) {
	agentDir, target := linkFixture(t)
	link := filepath.Join(agentDir, "web-design")

	if got := skillLink(agentDir, target); got != linkNone {
		t.Errorf("nothing there, skillLink = %d", got)
	}
	// A symbolic link to the skill is the hub's own.
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, target); got != linkOurs {
		t.Errorf("a link to the skill, skillLink = %d", got)
	}
	// A symbolic link to somewhere else is not.
	other := filepath.Join(t.TempDir(), "other-skill")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, target); got != linkTaken {
		t.Errorf("a link elsewhere, skillLink = %d", got)
	}
	// A broken link is taken too: the name is not free.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(agentDir, "gone"), link); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, target); got != linkTaken {
		t.Errorf("a broken link, skillLink = %d", got)
	}
	// A real directory of the skill's name is taken.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := skillLink(agentDir, target); got != linkTaken {
		t.Errorf("a real directory, skillLink = %d", got)
	}
}

func TestToggleSkillLink(t *testing.T) {
	agentDir, target := linkFixture(t)
	link := filepath.Join(agentDir, "web-design")

	// Nothing there: the toggle links the skill.
	msg, err := toggleSkillLink(agentDir, target, "Agent")
	if err != nil || msg != "" {
		t.Fatalf("toggle = %q, %v", msg, err)
	}
	if got := skillLink(agentDir, target); got != linkOurs {
		t.Fatalf("after the toggle, skillLink = %d", got)
	}
	// The hub's own link: the toggle removes it.
	msg, err = toggleSkillLink(agentDir, target, "Agent")
	if err != nil || msg != "" {
		t.Fatalf("toggle = %q, %v", msg, err)
	}
	if got := skillLink(agentDir, target); got != linkNone {
		t.Fatalf("after the second toggle, skillLink = %d", got)
	}
	// The agent's skills directory is made for the link where missing.
	if err := os.RemoveAll(agentDir); err != nil {
		t.Fatal(err)
	}
	if _, err := toggleSkillLink(agentDir, target, "Agent"); err != nil {
		t.Fatalf("toggle = %v", err)
	}
	if got := skillLink(agentDir, target); got != linkOurs {
		t.Fatalf("in a directory made for it, skillLink = %d", got)
	}

	// A real directory of the skill's name: nothing changes, the
	// message says why.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err = toggleSkillLink(agentDir, target, "Agent")
	if err != nil {
		t.Fatalf("toggle = %v", err)
	}
	if msg == "" {
		t.Error("no message for a real directory in the way")
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the real directory changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); !os.IsNotExist(err) {
		t.Error("the real directory gained content")
	}
}

func TestRemoveSkill(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skills", "web-design")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("---\nname: web-design\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The record of where the skill was imported from lives and dies
	// with the skill.
	if err := os.WriteFile(filepath.Join(target, importMetaFile), []byte(`{"source":"git"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Agent A links to the skill, agent B has a link of the name pointing
	// elsewhere, agent C a real directory of the name.
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(dir, "b")
	cDir := filepath.Join(dir, "c")
	for _, d := range []string{aDir, bDir, cDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(target, filepath.Join(aDir, "web-design")); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(bDir, "web-design")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cDir, "web-design"), 0o755); err != nil {
		t.Fatal(err)
	}

	agents := []agent{{Name: "A", Dir: aDir}, {Name: "B", Dir: bDir}, {Name: "C", Dir: cDir}}
	if err := removeSkill(agents, filepath.Dir(target), "web-design"); err != nil {
		t.Fatal(err)
	}
	// The skill and the hub's own link are gone, the import record with
	// them.
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Error("the skill's directory stayed")
	}
	if _, err := os.Lstat(filepath.Join(target, importMetaFile)); !os.IsNotExist(err) {
		t.Error("the import record stayed")
	}
	if _, err := os.Lstat(filepath.Join(aDir, "web-design")); !os.IsNotExist(err) {
		t.Error("the hub's link in A stayed")
	}
	// What is not the hub's is untouched.
	if dst, err := os.Readlink(filepath.Join(bDir, "web-design")); err != nil || dst != other {
		t.Errorf("B's link is %q, %v", dst, err)
	}
	if fi, err := os.Lstat(filepath.Join(cDir, "web-design")); err != nil || !fi.IsDir() {
		t.Errorf("C's real directory changed: %v", err)
	}
}
