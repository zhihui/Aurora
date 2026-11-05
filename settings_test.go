package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The settings pane adds agents from the modal's presets and its custom
// form, lists them and deletes them. agentsPath is "" in tests, which
// stay in memory.
func TestSettings(t *testing.T) {
	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("还没有添加任何 agent") {
		t.Errorf("no empty state, texts %q", tt.Texts())
	}

	// A preset is chosen on the 预设 tab, then added with 添加.
	if err := tt.Click("添加 Agent…"); err != nil {
		t.Fatal(err)
	}
	if !a.addOpen {
		t.Fatal("the modal did not open")
	}
	for _, s := range []string{"Claude", "~/.claude/skills", "Deepseek Harness", "~/.dsh/skills", "Kimi Code", "Work Buddy", "Pi Agent", "~/.pi/agent/skills", "Qoder CN"} {
		if !tt.HasText(s) {
			t.Errorf("no preset %q, texts %q", s, tt.Texts())
		}
	}
	if err := tt.Click("Claude"); err != nil {
		t.Fatal(err)
	}
	if !a.addOpen || a.presetSel != "Claude" {
		t.Fatalf("open %v, presetSel %q after a click on Claude", a.addOpen, a.presetSel)
	}
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	if a.addOpen || len(a.agents) != 1 || a.agents[0] != (agent{"Claude", "~/.claude/skills"}) {
		t.Fatalf("open %v, agents %v after a preset", a.addOpen, a.agents)
	}
	if err := tt.Click("添加 Agent…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已添加") {
		t.Errorf("the preset added does not say so, texts %q", tt.Texts())
	}

	// The 自定义 tab adds a name and a directory, and empties its inputs.
	if err := tt.Click("自定义"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("My Agent")
	if err := tt.Click("配置目录"); err != nil {
		t.Fatal(err)
	}
	tt.Type(" ~/.my-agent/skills ")
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	if a.addOpen || len(a.agents) != 2 || a.agents[1] != (agent{"My Agent", "~/.my-agent/skills"}) {
		t.Fatalf("open %v, agents %v after the custom form", a.addOpen, a.agents)
	}
	if a.addName != "" || a.addDir != "" {
		t.Errorf("the form keeps %q %q after an add", a.addName, a.addDir)
	}

	// 删除 lives in the row's ⋯ menu. It takes Claude's row off the list.
	if err := tt.Click("Claude 的操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if len(a.agents) != 1 || a.agents[0].Name != "My Agent" {
		t.Fatalf("agents %v after 删除", a.agents)
	}

	// A row shows how many skills the agent's directory has now; it is
	// not a button — a click on it leads nowhere.
	if !tt.HasText("My Agent") || !tt.HasText("~/.my-agent/skills") || !tt.HasText("0 个技能") {
		t.Fatalf("the agents are not listed, texts %q", tt.Texts())
	}
	if err := tt.Click("My Agent"); err == nil {
		tt.Frame()
		if a.selected != "settings" {
			t.Errorf("a click on the row navigated to %q", a.selected)
		}
	}
}

// Adding an agent removes the broken symbolic links of its skills
// directory and keeps the valid ones and the regular files; a directory
// that is not there is not created. Deleting the agent leaves the
// directory untouched.
func TestAddAgentCleansBrokenLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(regular, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(dir, "valid-link")
	broken := filepath.Join(dir, "broken-link")
	if err := os.Symlink(target, valid); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone"), broken); err != nil {
		t.Fatal(err)
	}

	a := &app{}
	a.addAgent(agent{"Test", dir})
	for _, p := range []string{target, regular, valid} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("addAgent removed %s", p)
		}
	}
	if _, err := os.Lstat(broken); !os.IsNotExist(err) {
		t.Error("the broken link is still there after addAgent")
	}

	// A directory that is not there stays not there.
	missing := filepath.Join(dir, "missing")
	a.addAgent(agent{"Missing", missing})
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("addAgent created the missing directory")
	}

	// Deleting only drops the configuration: the directory is untouched.
	a.agents = nil
	a.saveAgents()
	for _, p := range []string{target, regular, valid} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("the delete touched %s", p)
		}
	}
}

// The built-in 通用 Agent is added at the head of the list once, and it
// has no 删除: the badge 默认 takes its place. Deleting another agent
// leaves it there.
func TestDefaultAgent(t *testing.T) {
	a := &app{}
	a.ensureDefaultAgent()
	if len(a.agents) != 1 || a.agents[0].Name != defaultAgentName {
		t.Fatalf("agents %v after ensureDefaultAgent", a.agents)
	}
	a.ensureDefaultAgent()
	if len(a.agents) != 1 {
		t.Fatalf("agents %v after a second ensureDefaultAgent", a.agents)
	}
	if dir := expandHome(a.agents[0].Dir); !strings.HasSuffix(dir, filepath.Join(".agents", "skills")) {
		t.Errorf("the built-in agent's directory expands to %q", dir)
	}

	// A saved directory that drifted — an update moved it — is restored.
	a.agents[0].Dir = "~/.agent/skills"
	a.ensureDefaultAgent()
	if a.agents[0].Dir != defaultAgent().Dir {
		t.Errorf("the drifted directory stayed %q", a.agents[0].Dir)
	}

	a.selected, a.focusSide = "settings", "sidebar"
	a.addAgent(agent{"X", "~/x/skills"})
	tt := ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("通用 Agent") || !tt.HasText("默认") {
		t.Errorf("no built-in agent or its badge, texts %q", tt.Texts())
	}
	// The built-in agent's menu has 打开 only, never 删除.
	if err := tt.Click("通用 Agent 的操作"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("打开") {
		t.Errorf("no 打开 in the menu, texts %q", tt.Texts())
	}
	if tt.HasText("删除") {
		t.Error("the built-in agent's menu has 删除")
	}
	tt.Key(0, ui.KeyEscape)
	// X's menu has 删除, and it deletes.
	if err := tt.Click("X 的操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if len(a.agents) != 1 || a.agents[0].Name != defaultAgentName {
		t.Fatalf("agents %v after 删除 on X", a.agents)
	}
}

// The menu's 打开 opens the agent's skills directory in the system's
// file manager, the stored form expanded first.
func TestAgentOpenDir(t *testing.T) {
	dir := t.TempDir()
	var opened []string
	old := openDir
	t.Cleanup(func() { openDir = old })
	openDir = func(dir string) error {
		opened = append(opened, dir)
		return nil
	}

	a := &app{selected: "settings", focusSide: "sidebar"}
	a.agents = []agent{{Name: "Claude", Dir: dir}}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("Claude 的操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("打开"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != dir {
		t.Errorf("opened = %v", opened)
	}
}

// Opening a directory that is not there is an error, not a creation.
func TestOpenDirMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	if err := openDir(missing); err == nil {
		t.Error("opening a missing directory worked")
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Error("openDir created the directory")
	}
}
