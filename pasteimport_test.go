package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCheckPastedSkill(t *testing.T) {
	good := "---\nname: my-skill\ndescription: 做什么的。\n---\n"
	if err := checkPastedSkill("my-skill", good); err != nil {
		t.Errorf("checkPastedSkill(good) = %v", err)
	}
	// A description in a YAML block scalar counts too.
	block := "---\nname: my-skill\ndescription: >\n  做什么的，\n  还有别的。\n---\n"
	if err := checkPastedSkill("my-skill", block); err != nil {
		t.Errorf("checkPastedSkill(block) = %v", err)
	}
	for _, tc := range []struct{ name, content string }{
		{"", good},
		{"  ", good},
		{"a/b", good},
		{"..", good},
		{"my-skill", ""},
		{"my-skill", "   \n "},
		{"my-skill", "# no front matter\n"},
		{"my-skill", "---\ndescription: 没有名字。\n---\n"},
		{"my-skill", "---\nname: my-skill\n---\n"},
	} {
		if err := checkPastedSkill(tc.name, tc.content); err == nil {
			t.Errorf("checkPastedSkill(%q, %q) = nil error", tc.name, tc.content)
		}
	}
}

func TestCreatePastedSkill(t *testing.T) {
	skillsDir := t.TempDir()
	content := "---\nname: my-skill\ndescription: 做什么的。\n---\n# 正文\n"
	if err := createPastedSkill(skillsDir, "my-skill", content); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(skillsDir, "my-skill", "SKILL.md")); err != nil || string(data) != content {
		t.Errorf("SKILL.md = %q, %v", data, err)
	}
	data, err := os.ReadFile(filepath.Join(skillsDir, "my-skill", importMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta importMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Source != "paste" || meta.ImportedAt.IsZero() {
		t.Errorf("importMeta = %+v", meta)
	}

	// Creating the name again overwrites: what only the old skill had
	// goes away.
	if err := os.WriteFile(filepath.Join(skillsDir, "my-skill", "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := createPastedSkill(skillsDir, "my-skill", content); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "my-skill", "stale.txt")); !os.IsNotExist(err) {
		t.Error("the overwrite kept a stale file")
	}
}

// The 粘贴/新建 way's flow: the footer's button is 创建; it validates
// the name and the SKILL.md's name and description, and writes the
// skill into the hub.
func TestImportPasteFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "hub-skills")
	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, importOpen: true, importMethod: 4}
	tt := ui.NewTester(a.view, 1000, 640)

	if !tt.HasText("创建") {
		t.Fatalf("no 创建 button, texts %q", tt.Texts())
	}
	// With nothing the button is disabled; the validation answers a
	// direct call as it would a click.
	if a.importPasted() {
		t.Fatal("importPasted with nothing = true")
	}
	tt.Frame()
	if a.importErr == "" || !tt.HasText(a.importErr) {
		t.Fatalf("no error after 创建 with nothing, texts %q", tt.Texts())
	}

	// The form takes input.
	if err := tt.Click("技能名称输入框"); err != nil {
		t.Fatal(err)
	}
	tt.Type("my-skill")
	if a.importName != "my-skill" {
		t.Fatalf("importName %q after typing", a.importName)
	}

	// A SKILL.md without a description fails.
	a.importContent = "---\nname: my-skill\n---\n"
	tt.Frame()
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}
	if a.importErr == "" || !tt.HasText("SKILL.md 的 front matter 缺少 description") {
		t.Fatalf("no description error, texts %q", tt.Texts())
	}

	// A complete one is created.
	a.importContent = "---\nname: my-skill\ndescription: 做什么的。\n---\n# 正文\n"
	tt.Frame()
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已创建技能") {
		t.Errorf("no toast after 创建, texts %q", tt.Texts())
	}
	if a.importOpen {
		t.Error("the modal stayed open after 创建")
	}
	if data, err := os.ReadFile(filepath.Join(skillsDir, "my-skill", "SKILL.md")); err != nil || string(data) != a.importContent {
		t.Errorf("SKILL.md = %q, %v", data, err)
	}
	if len(a.skills) != 1 || a.skills[0].Name != "my-skill" || a.skills[0].Source != "手动创建" {
		t.Errorf("loadSkills after 创建 = %+v", a.skills)
	}
}
