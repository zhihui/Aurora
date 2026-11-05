package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// pickFolder asks the user for a folder with the system's open dialog;
// a variable, so the tests can stub it. "" is a cancelation.
var pickFolder = func() (string, error) {
	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择技能文件夹", Directory: true})
	if err != nil || len(dirs) == 0 {
		return "", err
	}
	return dirs[0], nil
}

// pickArchive asks the user for a zip with the system's open dialog;
// a variable, so the tests can stub it. "" is a cancelation.
var pickArchive = func() (string, error) {
	files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:   "选择技能压缩包",
		Filters: []mygo.FileFilter{{Name: "Zip 压缩包", Extensions: []string{"zip"}}},
	})
	if err != nil || len(files) == 0 {
		return "", err
	}
	return files[0], nil
}

// pickLocalFolder asks for a folder and parses it right away — the 本地
// way has no 解析 button. The dialog blocks its caller, so it is asked
// off the main thread.
func (a *app) pickLocalFolder() {
	if a.importParsing {
		return
	}
	go func() {
		dir, err := pickFolder()
		runOnMain(func() {
			if err != nil {
				a.importErr = err.Error()
				return
			}
			if dir == "" {
				return
			}
			a.importLocalDir = dir
			a.parseLocalFolder(dir)
		})
	}()
}

// pickLocalArchive asks for a zip and parses it right away.
func (a *app) pickLocalArchive() {
	if a.importParsing {
		return
	}
	go func() {
		file, err := pickArchive()
		runOnMain(func() {
			if err != nil {
				a.importErr = err.Error()
				return
			}
			if file == "" {
				return
			}
			a.importLocalDir = "" // an archive is copied, never linked
			a.parseLocalArchive(file)
		})
	}()
}

// parseLocalFolder parses the folder the user picked: the skills stay
// where they are — the import copies them, or links them where the
// user chose 链接导入.
func (a *app) parseLocalFolder(dir string) {
	a.startParse(dir, "", func(context.Context) (string, string, string, []foundSkill, error) {
		skills, err := scanLocalFolder(dir)
		return "", "", "local", skills, err
	})
}

// parseLocalArchive parses the zip the user picked: it unpacks into a
// temporary directory, and the import copies.
func (a *app) parseLocalArchive(file string) {
	a.startParse(file, "", func(context.Context) (string, string, string, []foundSkill, error) {
		res, err := unpackLocalArchive(file)
		if err != nil {
			return "", "", "", nil, err
		}
		return res.root, "", "local", res.skills, nil
	})
}

// scanLocalFolder lists the skills of the folder the user picked.
func scanLocalFolder(dir string) ([]foundSkill, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("目录「%s」不存在或不是目录", dir)
	}
	skills := findGitSkills(dir)
	if len(skills) == 0 {
		return nil, fmt.Errorf("文件夹中没有含 SKILL.md 的技能")
	}
	return skills, nil
}

// unpackLocalArchive reads the zip the user picked and unpacks it as
// the URL way unpacks a download, under the same limits.
func unpackLocalArchive(file string) (*unpackResult, error) {
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		return nil, fmt.Errorf("文件「%s」不存在", filepath.Base(file))
	}
	if st.Size() > urlMaxDownload {
		return nil, fmt.Errorf("压缩包超过 10 MiB 上限")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) && !strings.HasSuffix(strings.ToLower(file), ".zip") {
		return nil, fmt.Errorf("「%s」不是 zip 压缩包", filepath.Base(file))
	}
	return unzipSkills(data)
}

// linkSkill links a skill of the hub to the directory it was parsed
// from — over what the hub has of the name — so the hub entry follows
// the source's updates. No import record goes into the user's own
// directory; loadSkills tells the link itself.
func linkSkill(src, skillsDir, name string) error {
	if !validSkillName(name) {
		return fmt.Errorf("技能名「%s」无效", name)
	}
	dst := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("覆盖「%s」: %w", name, err)
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if err := dirLink(abs, dst); err != nil {
		return fmt.Errorf("创建链接「%s」: %w", name, err)
	}
	return nil
}
