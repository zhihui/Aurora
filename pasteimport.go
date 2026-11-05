package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// checkPastedSkill validates what the 粘贴/新建 way is given: a name
// that can hold a directory of the hub, and a SKILL.md whose front
// matter says a name and a description.
func checkPastedSkill(name, content string) error {
	if !validSkillName(strings.TrimSpace(name)) {
		return fmt.Errorf("请填写有效的技能名称")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("请填写 SKILL.md 内容")
	}
	sk := parseSkill([]byte(content), "")
	if sk.Name == "" {
		return fmt.Errorf("SKILL.md 的 front matter 缺少 name")
	}
	if sk.Desc == "" {
		return fmt.Errorf("SKILL.md 的 front matter 缺少 description")
	}
	return nil
}

// createPastedSkill writes the 粘贴/新建 way's skill into the hub — over
// what the hub has of the name — recording that it was made by hand.
func createPastedSkill(skillsDir, name, content string) error {
	dir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("覆盖「%s」: %w", name, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入「%s」: %w", name, err)
	}
	meta := importMeta{Source: "paste", ImportedAt: time.Now()}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, importMetaFile), append(data, '\n'), 0o644)
	}
	return nil
}

// importPasted is the footer button of the 粘贴/新建 way: it validates
// and writes what the user typed. False leaves the modal open, the
// reason in a.importErr.
func (a *app) importPasted() bool {
	a.importErr = ""
	name := strings.TrimSpace(a.importName)
	if err := checkPastedSkill(name, a.importContent); err != nil {
		a.importErr = err.Error()
		return false
	}
	if err := createPastedSkill(a.skillsDir, name, a.importContent); err != nil {
		a.importErr = err.Error()
		return false
	}
	a.loadSkills()
	a.closeImport()
	return true
}
