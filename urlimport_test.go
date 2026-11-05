package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// zipBytes packs files — name to content — into a zip in memory.
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A SKILL.md link makes one skill; its name comes from the front
// matter, or from the directory the link's path has it in.
func TestFetchURLMD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/skills/pdf/SKILL.md":
			fmt.Fprint(w, "---\nname: pdf-tools\ndescription: PDF。\n---\n")
		case "/skills/canvas-design/SKILL.md":
			fmt.Fprint(w, "# no front matter\n")
		case "/SKILL.md":
			fmt.Fprint(w, "# no front matter\n")
		case "/page":
			fmt.Fprint(w, "<!DOCTYPE html><html>…</html>")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := fetchURL(context.Background(), srv.URL+"/skills/pdf/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(res.root)
	if len(res.skills) != 1 || res.skills[0].Name != "pdf-tools" || res.skills[0].Desc != "PDF。" {
		t.Errorf("fetchURL(SKILL.md) = %+v", res.skills)
	}
	// The downloaded file landed under the skill's name.
	if _, err := os.Stat(filepath.Join(res.root, "pdf-tools", "SKILL.md")); err != nil {
		t.Errorf("no SKILL.md in the temporary directory: %v", err)
	}

	// Without a name in the front matter the link's directory names the
	// skill.
	res, err = fetchURL(context.Background(), srv.URL+"/skills/canvas-design/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(res.root)
	if len(res.skills) != 1 || res.skills[0].Name != "canvas-design" {
		t.Errorf("fetchURL(nameless) = %+v", res.skills)
	}

	for _, bad := range []string{
		srv.URL + "/SKILL.md", // no name anywhere
		srv.URL + "/page",     // a web page, not a SKILL.md
		srv.URL + "/missing",  // 404
		"ftp://example.com/SKILL.md",
		"not a url",
	} {
		if _, err := fetchURL(context.Background(), bad); err == nil {
			t.Errorf("fetchURL(%q) = nil error", bad)
		}
	}
}

// A zip link unpacks into a temporary directory and its skills are
// listed; an evil path or a skill-less zip is an error.
func TestFetchURLZip(t *testing.T) {
	good := zipBytes(t, map[string]string{
		"pack/pdf-tools/SKILL.md":       "---\nname: pdf-tools\ndescription: PDF。\n---\n",
		"pack/canvas-design/SKILL.md":   "---\nname: canvas-design\n---\n",
		"pack/canvas-design/helper.txt": "x",
		"pack/README.md":                "# pack\n",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(good)
	}))
	defer srv.Close()

	res, err := fetchURL(context.Background(), srv.URL+"/pack.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(res.root)
	if len(res.skills) != 2 {
		t.Fatalf("fetchURL(zip) = %+v", res.skills)
	}
	// The zip's extra files came along with their skill.
	if _, err := os.Stat(filepath.Join(res.root, "pack", "canvas-design", "helper.txt")); err != nil {
		t.Errorf("no helper.txt in the unpack: %v", err)
	}

	empty := zipBytes(t, map[string]string{"README.md": "# nothing\n"})
	if _, err := unzipSkills(empty); err == nil {
		t.Error("unzipSkills of a skill-less zip = nil error")
	}
	evil := zipBytes(t, map[string]string{"../evil/SKILL.md": "---\nname: evil\n---\n"})
	if _, err := unzipSkills(evil); err == nil {
		t.Error("unzipSkills of a zip-slip = nil error")
	}
	if _, err := unzipSkills([]byte("not a zip")); err == nil {
		t.Error("unzipSkills of garbage = nil error")
	}
}

// The 链接 URL way's flow: 解析 downloads the link in the background and
// lists the skills found, the hub's own marked 已存在 and unchosen; 导入
// copies the chosen ones, recording the source.
func TestImportURLFlow(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "hub-skills")
	writeSkill(t, skillsDir, "pdf-tools", "---\nname: pdf-tools\ndescription: 旧版。\n---\n")
	src := filepath.Join(dir, "download")
	writeSkill(t, src, "pdf", "---\nname: pdf-tools\ndescription: 新版。\n---\n")
	writeSkill(t, src, "canvas", "---\nname: canvas-design\ndescription: 画布。\n---\n")

	link := "https://example.com/pack.zip"
	var fail error
	oldURL, oldRun := urlFetch, runOnMain
	t.Cleanup(func() { urlFetch, runOnMain = oldURL, oldRun })
	applied := make(chan struct{}, 1)
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	urlFetch = func(_ context.Context, raw string) (*unpackResult, error) {
		if fail != nil {
			return nil, fail
		}
		if raw != link {
			t.Errorf("urlFetch = %q", raw)
		}
		return &unpackResult{root: src, skills: findGitSkills(src)}, nil
	}

	a := &app{selected: "hub", focusSide: "sidebar", skillsDir: skillsDir, importOpen: true, importMethod: 2, importURL: link}
	a.loadSkills()
	tt := ui.NewTester(a.view, 1000, 640)

	if err := tt.Click("解析"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	for _, s := range []string{"发现 2 个技能 · 已选 1 个", "pdf-tools", "canvas-design", "已存在", "导入 1 个技能"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q after 解析, texts %q", s, tt.Texts())
		}
	}

	// 全选 and 取消选择 choose all of the list or none.
	if err := tt.Click("取消选择"); err != nil {
		t.Fatal(err)
	}
	if a.importCount() != 0 || !tt.HasText("发现 2 个技能 · 已选 0 个") {
		t.Errorf("取消选择 = %d chosen, texts %q", a.importCount(), tt.Texts())
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("导入 2 个技能") {
		t.Fatalf("全选 did not count, texts %q", tt.Texts())
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
	for _, name := range []string{"pdf-tools", "canvas-design"} {
		data, err := os.ReadFile(filepath.Join(skillsDir, name, importMetaFile))
		if err != nil {
			t.Fatalf("no import record of %s: %v", name, err)
		}
		var meta importMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatal(err)
		}
		if meta.Source != "url" || meta.URL != link || meta.Commit != "" {
			t.Errorf("importMeta of %s = %+v", name, meta)
		}
	}
	if len(a.skills) != 2 || a.skills[1].Source != "example.com" {
		t.Errorf("loadSkills after 导入 = %+v", a.skills)
	}

	// A failed 解析 shows in the modal.
	fail = os.ErrNotExist
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
