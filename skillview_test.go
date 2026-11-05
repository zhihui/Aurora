package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// stubTranslate swaps the model call and the main-thread delivery for
// synchronous stubs, reporting through applied when a result lands.
func stubTranslate(t *testing.T, out string, err error) (applied chan struct{}) {
	t.Helper()
	applied = make(chan struct{}, 1)
	oldChat, oldRun := llmChat, runOnMain
	t.Cleanup(func() { llmChat, runOnMain = oldChat, oldRun })
	llmChat = func(context.Context, llmConfig, string, string) (string, error) { return out, err }
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	return applied
}

// The hub's card opens the viewer on its skill: the name, the path and
// the SKILL.md as 原文.
func TestSkillViewOpen(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\ndescription: 读取 PDF。\n---\n\n# PDF Tools\n")

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir,
		llm: llmConfig{Type: "openai", Endpoint: "https://x", Model: "m", APIKey: "k"}}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("查看 pdf-tools"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.viewOpen || a.viewDir != "pdf-tools" {
		t.Fatalf("open %v, dir %q", a.viewOpen, a.viewDir)
	}
	for _, s := range []string{"pdf-tools", "SKILL.md", "# PDF Tools", "原文", "中文"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
	// The 中文 tab offers 翻译 where there is no translation.
	if err := tt.Click("中文"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("还没有中文译文") || !tt.HasText("翻译") {
		t.Errorf("no 翻译 offer, texts %q", tt.Texts())
	}
}

// 翻译 calls the configured model, saves the translation beside the
// skill, and shows it; the next look needs no call.
func TestSkillViewTranslate(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\n---\n")
	applied := stubTranslate(t, "这是译文。", nil)

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir,
		llm: llmConfig{Type: "openai", Endpoint: "https://x", Model: "m", APIKey: "k"}}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)
	a.openSkillView(a.skills[0])
	a.viewTab = 1
	tt.Frame()
	if err := tt.Click("翻译"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if !tt.HasText("这是译文。") {
		t.Errorf("no translation shown, texts %q", tt.Texts())
	}
	tr, ok := loadTranslation(filepath.Join(skillsDir, "pdf-tools"))
	if !ok || tr.Content != "这是译文。" || tr.SourceHash != skillHash([]byte(a.viewContent)) {
		t.Fatalf("saved translation = %+v, %v", tr, ok)
	}

	// A change of the SKILL.md makes it stale, asking for a redo.
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\ndescription: 新版。\n---\n")
	a.closeSkillView()
	a.openSkillView(a.skills[0])
	a.viewTab = 1
	tt.Frame()
	if !tt.HasText("译文可能已过期") || !tt.HasText("重新翻译") {
		t.Errorf("no stale warning, texts %q", tt.Texts())
	}
}

// Without a configured model the 中文 tab says where to go, and offers
// no 翻译 button.
func TestSkillViewNoConfig(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\n---\n")

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)
	a.openSkillView(a.skills[0])
	a.viewTab = 1
	tt.Frame()
	if !tt.HasText("请先在「设置」中配置大语言模型 API") {
		t.Errorf("no config hint, texts %q", tt.Texts())
	}
	r, ok := tt.Find("翻译")
	if ok && r.W > 0 && strings.Contains(strings.Join(tt.Texts(), " "), "还没有中文译文") {
		t.Errorf("a 翻译 button shows without a configuration")
	}
}

// A failed translation shows in the viewer and writes nothing.
func TestSkillViewTranslateErr(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\n---\n")
	applied := stubTranslate(t, "", os.ErrPermission)

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir,
		llm: llmConfig{Type: "openai", Endpoint: "https://x", Model: "m", APIKey: "k"}}
	a.loadSkills()
	a.openSkillView(a.skills[0])
	a.viewTab = 1
	tt := ui.NewTester(a.view, 1000, 640)
	tt.Frame()
	a.startTranslate()
	<-applied
	tt.Frame()
	if a.translateErr == "" || !tt.HasText(a.translateErr) {
		t.Errorf("no error shown, texts %q", tt.Texts())
	}
	if _, ok := loadTranslation(filepath.Join(skillsDir, "pdf-tools")); ok {
		t.Error("a failed translation was saved")
	}
}
