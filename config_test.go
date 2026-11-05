package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The configuration lives in .aurora of the user's home, and displays as
// the platform's users write it.
func TestConfigDir(t *testing.T) {
	dir, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if dir != filepath.Join(home, ".aurora") {
		t.Errorf("configDir %q, want %q", dir, filepath.Join(home, ".aurora"))
	}
	s := displayHome(dir)
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(s, `%USERPROFILE%\.aurora`) {
			t.Errorf("displayHome %q does not start with %%USERPROFILE%%", s)
		}
	} else if s != "~/.aurora" {
		t.Errorf("displayHome %q, want ~/.aurora", s)
	}
	// A path outside the home shows as it is.
	if s := displayHome("/var/tmp/x"); s != "/var/tmp/x" {
		t.Errorf("displayHome %q, want /var/tmp/x", s)
	}
}

// The settings page shows the configuration directory, shortened.
func TestSettingsShowsConfigDir(t *testing.T) {
	dir, err := configDir()
	if err != nil {
		t.Skip(err)
	}
	a := &app{selected: "settings", focusSide: "sidebar", agentsPath: filepath.Join(dir, "agents.json")}
	a.agents = []agent{{Name: "Claude", Dir: "~/.claude/skills"}}
	tt := ui.NewTester(a.view, 1000, 640)
	if !tt.HasText("配置目录：" + displayHome(dir)) {
		t.Errorf("no config directory in %q", tt.Texts())
	}
}

// The first start creates the configuration: the config directory, the
// skills directory and an empty agents.json.
func TestEnsureConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir, err := ensureConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, filepath.Join(dir, "skills"), filepath.Join(dir, "agents.json")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("ensureConfig did not create %s", p)
		}
	}
	// It leaves what is there alone: a second run keeps the file.
	agents := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(agents, []byte(`[{"name":"Kimi"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureConfig(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(agents)
	if string(data) != `[{"name":"Kimi"}]` {
		t.Errorf("a second ensureConfig rewrote agents.json: %s", data)
	}
}

// expandHome expands the home prefix as the platform's users write it.
func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		"~":                home,
		"~/.claude/skills": filepath.Join(home, ".claude", "skills"),
		`~\x\y`:            filepath.Join(home, "x", "y"),
		"/var/tmp/x":       "/var/tmp/x",
		"relative/x":       "relative/x",
	} {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
