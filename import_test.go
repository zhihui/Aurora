package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The 导入技能 modal opens from the hub's header, switches between the
// ways to import, and closes from 取消 and the ×.
func TestImportModal(t *testing.T) {
	a := &app{selected: "hub", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	if tt.HasText("从外部来源复制技能到") {
		t.Error("the modal shows before 导入技能 is clicked")
	}
	if err := tt.Click("导入技能…"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"从外部来源复制技能到", "GitHub", "命令行", "链接 URL", "本地", "粘贴 / 新建", "仓库或子目录链接", "分支"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}

	// The ways switch the form.
	if err := tt.Click("命令行"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("skills add 命令") {
		t.Errorf("no 命令行 form, texts %q", tt.Texts())
	}
	if err := tt.Click("粘贴 / 新建"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("SKILL.md 内容") {
		t.Errorf("no 粘贴 / 新建 form, texts %q", tt.Texts())
	}

	// The form takes input.
	if err := tt.Click("技能名称输入框"); err != nil {
		t.Fatal(err)
	}
	tt.Type("my-skill")
	if a.importName != "my-skill" {
		t.Errorf("importName %q after typing", a.importName)
	}

	// The 本地 way has no 解析 button: choosing a folder or an archive
	// parses right away. The real flows are in TestImportGitFlow,
	// TestImportCmdFlow, TestImportURLFlow and the localimport tests.
	if err := tt.Click("本地"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"选文件夹", "选压缩包"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q in the 本地 form, texts %q", s, tt.Texts())
		}
	}
	if err := tt.Click("解析"); err == nil {
		t.Error("the 本地 way still has a 解析 button")
	}

	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.importOpen {
		t.Error("the modal shows after 取消")
	}
	if err := tt.Click("导入技能…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if a.importOpen {
		t.Error("the modal shows after the ×")
	}
}
