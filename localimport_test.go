package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestScanLocalFolder(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "one", "---\nname: one\n---\n")
	writeSkill(t, dir, "two", "---\nname: two\n---\n")
	skills, err := scanLocalFolder(dir)
	if err != nil || len(skills) != 2 {
		t.Errorf("scanLocalFolder = %v, %v", skills, err)
	}
	if _, err := scanLocalFolder(t.TempDir()); err == nil {
		t.Error("scanLocalFolder of a skill-less folder = nil error")
	}
	if _, err := scanLocalFolder(filepath.Join(dir, "missing")); err == nil {
		t.Error("scanLocalFolder of a missing folder = nil error")
	}
}

func TestUnpackLocalArchive(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pack.zip")
	data := zipBytes(t, map[string]string{
		"pack/pdf-tools/SKILL.md":     "---\nname: pdf-tools\n---\n",
		"pack/canvas-design/SKILL.md": "---\nname: canvas-design\n---\n",
	})
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := unpackLocalArchive(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(res.root)
	if len(res.skills) != 2 {
		t.Errorf("unpackLocalArchive = %v skills", len(res.skills))
	}

	notZip := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notZip, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := unpackLocalArchive(notZip); err == nil {
		t.Error("unpackLocalArchive of a text file = nil error")
	}
	if _, err := unpackLocalArchive(filepath.Join(dir, "missing.zip")); err == nil {
		t.Error("unpackLocalArchive of a missing file = nil error")
	}
}

// linkSkill links the hub's entry to the source directory: loadSkills
// reads it as a skill tagged 本地链接, nothing is written into the user's
// own directory, and deleting the skill removes the link only.
func TestLinkSkill(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	writeSkill(t, src, "design", "---\nname: web-design\ndescription: 设计。\n---\n")
	skillsDir := filepath.Join(dir, "hub-skills")

	if err := linkSkill(filepath.Join(src, "design"), skillsDir, "web-design"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skillsDir, "web-design")
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the hub entry is no link: %v", err)
	}
	// Nothing landed in the user's own directory.
	if _, err := os.Lstat(filepath.Join(src, "design", importMetaFile)); !os.IsNotExist(err) {
		t.Error("an import record landed in the source directory")
	}

	a := &app{skillsDir: skillsDir}
	a.loadSkills()
	if len(a.skills) != 1 || a.skills[0].Name != "web-design" || a.skills[0].Desc != "设计。" || a.skills[0].Source != "本地链接" {
		t.Fatalf("loadSkills = %+v", a.skills)
	}

	// Overwriting a real directory with a link works, and the other way
	// around.
	if err := linkSkill(filepath.Join(src, "design"), skillsDir, "web-design"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the second linkSkill broke the link")
	}
	writeSkill(t, src, "other", "---\nname: other\n---\n")
	if err := importSkill(filepath.Join(src, "other"), skillsDir, "web-design", importMeta{Source: "local"}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink != 0 {
		t.Error("importSkill kept the link")
	}
	if err := linkSkill(filepath.Join(src, "design"), skillsDir, "web-design"); err != nil {
		t.Fatal(err)
	}

	// Deleting the skill removes the link; the source stays.
	if err := removeSkill(nil, skillsDir, "web-design"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("the link stayed after removeSkill")
	}
	if _, err := os.Stat(filepath.Join(src, "design", "SKILL.md")); err != nil {
		t.Error("removeSkill touched the source directory")
	}
}

// stubLocal swaps the dialogs and the background delivery of the 本地
// way for synchronous stubs.
func stubLocal(t *testing.T, folder, archive string) (applied chan struct{}) {
	t.Helper()
	applied = make(chan struct{}, 4)
	oldFolder, oldArchive, oldRun := pickFolder, pickArchive, runOnMain
	t.Cleanup(func() { pickFolder, pickArchive, runOnMain = oldFolder, oldArchive, oldRun })
	pickFolder = func() (string, error) { return folder, nil }
	pickArchive = func() (string, error) { return archive, nil }
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	return applied
}

// The 本地 way's folder flow: 选文件夹 parses right away — no 解析
// button — and shows the 链接导入 choice; importing with it on links the
// skills instead of copying them.
func TestImportLocalFolderFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "hub-skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\ndescription: 旧版。\n---\n")
	src := filepath.Join(dir, "source")
	writeSkill(t, src, "pdf", "---\nname: pdf-tools\ndescription: 新版。\n---\n")
	writeSkill(t, src, "canvas", "---\nname: canvas-design\ndescription: 画布。\n---\n")

	applied := stubLocal(t, src, "")
	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, importOpen: true, importMethod: 3}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)

	// No folder yet: no 链接导入 choice.
	if tt.HasText("链接导入：源目录更新自动同步，不复制文件") {
		t.Error("链接导入 shows before a folder is picked")
	}
	if err := tt.Click("选文件夹"); err != nil {
		t.Fatal(err)
	}
	<-applied // the pick delivered
	<-applied // the 解析 delivered
	tt.Frame()
	if a.importLocalDir != src {
		t.Fatalf("importLocalDir = %q", a.importLocalDir)
	}
	for _, s := range []string{"发现 2 个技能 · 已选 1 个", "pdf-tools", "canvas-design", "已存在", "导入 1 个技能", "链接导入：源目录更新自动同步，不复制文件"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q after picking the folder, texts %q", s, tt.Texts())
		}
	}

	// Link the import, choose both, import.
	if err := tt.Click("链接导入：源目录更新自动同步，不复制文件"); err != nil {
		t.Fatal(err)
	}
	if !a.importLink {
		t.Fatal("the checkbox did not turn 链接导入 on")
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("导入 2 个技能"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已导入 2 个技能") {
		t.Errorf("no toast after 导入, texts %q", tt.Texts())
	}
	// Both are links now, the source directories untouched.
	for _, name := range []string{"pdf-tools", "canvas-design"} {
		fi, err := os.Lstat(filepath.Join(skillsDir, name))
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no link: %v", name, err)
		}
	}
	for _, sub := range []string{"pdf", "canvas"} {
		if _, err := os.Lstat(filepath.Join(src, sub, importMetaFile)); !os.IsNotExist(err) {
			t.Errorf("an import record landed in %s", sub)
		}
	}
	if len(a.skills) != 2 || a.skills[0].Source != "本地链接" {
		t.Errorf("loadSkills after 导入 = %+v", a.skills)
	}
}

// The 本地 way's archive flow: 选压缩包 unpacks and parses right away;
// the 链接导入 choice does not show, and the import copies.
func TestImportLocalArchiveFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "hub-skills")
	zipPath := filepath.Join(dir, "pack.zip")
	data := zipBytes(t, map[string]string{
		"pack/pdf-tools/SKILL.md":     "---\nname: pdf-tools\ndescription: PDF。\n---\n",
		"pack/canvas-design/SKILL.md": "---\nname: canvas-design\ndescription: 画布。\n---\n",
	})
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, importOpen: true, importMethod: 3}
	a.importLink = true // sticky from a folder import: an archive ignores it
	applied := stubLocal(t, "", zipPath)
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("选压缩包"); err != nil {
		t.Fatal(err)
	}
	<-applied
	<-applied
	tt.Frame()
	if !tt.HasText("发现 2 个技能 · 已选 2 个") {
		t.Fatalf("no list after picking the archive, texts %q", tt.Texts())
	}
	if tt.HasText("链接导入：源目录更新自动同步，不复制文件") {
		t.Error("链接导入 shows for an archive")
	}
	if err := tt.Click("导入 2 个技能"); err != nil {
		t.Fatal(err)
	}
	// The import copied: real directories, the archive as the source.
	for _, name := range []string{"pdf-tools", "canvas-design"} {
		fi, err := os.Lstat(filepath.Join(skillsDir, name))
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s is no real directory: %v", name, err)
		}
		var meta importMeta
		md, err := os.ReadFile(filepath.Join(skillsDir, name, importMetaFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(md, &meta); err != nil {
			t.Fatal(err)
		}
		if meta.Source != "local" || meta.URL != zipPath {
			t.Errorf("importMeta of %s = %+v", name, meta)
		}
	}
}

// A canceled dialog changes nothing: no parse, no error.
func TestPickLocalCancel(t *testing.T) {
	applied := stubLocal(t, "", "")
	a := &app{selected: "hub", focusSide: "sidebar", importOpen: true, importMethod: 3}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("选文件夹"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if a.importErr != "" || a.importParsing || len(a.importParsed) != 0 || a.importLocalDir != "" {
		t.Errorf("the cancelation changed something: %+v", a.importErr)
	}
}
