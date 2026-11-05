package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// What an agent's skills directory holds under a skill's name.
const (
	linkNone  = iota // nothing
	linkOurs         // a symbolic link to the skill in the hub
	linkTaken        // something else: a real directory, a file, another link
)

// skillLink reports what the agent's skills directory holds under the
// skill's name: nothing, a symbolic link to the skill in the hub, or
// something else — which the hub never touches. target is the skill's
// directory in the hub; the link takes its name.
func skillLink(agentDir, target string) int {
	link := filepath.Join(agentDir, filepath.Base(target))
	fi, err := os.Lstat(link)
	if err != nil {
		return linkNone
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return linkTaken
	}
	dst, err := os.Readlink(link)
	if err != nil {
		return linkTaken
	}
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(agentDir, dst)
	}
	if samePath(dst, target) {
		return linkOurs
	}
	return linkTaken
}

// samePath reports whether two paths name the same file, resolving the
// symbolic links of what exists — /tmp is one on macOS, for example.
func samePath(a, b string) bool {
	if ea, err := filepath.EvalSymlinks(a); err == nil {
		a = ea
	}
	if eb, err := filepath.EvalSymlinks(b); err == nil {
		b = eb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// toggleSkillLink switches the agent's link to the skill: it removes a
// link of the hub's, and makes one where the name is free — the agent's
// skills directory is created for that. Where the name is taken, nothing
// changes and the message tells the user why. target is the skill's
// directory in the hub; the message is "" when all went well.
func toggleSkillLink(agentDir, target, agentName string) (string, error) {
	name := filepath.Base(target)
	link := filepath.Join(agentDir, name)
	switch skillLink(agentDir, target) {
	case linkOurs:
		if err := os.Remove(link); err != nil {
			return "", fmt.Errorf("删除链接: %w", err)
		}
		return "", nil
	case linkNone:
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			return "", fmt.Errorf("创建目录: %w", err)
		}
		if err := os.Symlink(target, link); err != nil {
			return "", fmt.Errorf("创建链接: %w", err)
		}
		return "", nil
	default:
		if fi, err := os.Lstat(link); err == nil && fi.IsDir() {
			return fmt.Sprintf("「%s」在 %s 中是真实目录，未做更改", name, agentName), nil
		}
		return fmt.Sprintf("「%s」在 %s 中已存在，未做更改", name, agentName), nil
	}
}

// removeSkill deletes the skill of the hub: first every symbolic link to
// it in the agents' skills directories — only the hub's own links, as a
// toggle makes them; anything else of the name stays — then the skill's
// directory itself, which takes the record of where the skill was
// imported from (importMetaFile) with it.
func removeSkill(agents []agent, skillsDir, name string) error {
	target := filepath.Join(skillsDir, name)
	for _, ag := range agents {
		dir := expandHome(ag.Dir)
		if skillLink(dir, target) != linkOurs {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("删除 %s 中的链接: %w", ag.Name, err)
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("删除技能目录: %w", err)
	}
	return nil
}

// packLinked reports whether the agent's skills directory links to every
// one of the pack's targets; an empty pack links to nothing.
func packLinked(agentDir string, targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if skillLink(agentDir, target) != linkOurs {
			return false
		}
	}
	return true
}

// togglePack switches the agent's links to a whole pack: linked to every
// target, it removes those links — only the hub's own, anything else of
// a name stays; else it links the targets not linked yet, the agent's
// skills directory created for that. A name taken by a real directory,
// a file or another link is skipped, never touched, and the message
// names the skipped; it is "" when all went well.
func togglePack(agentDir string, targets []string, agentName string) (string, error) {
	if packLinked(agentDir, targets) {
		for _, target := range targets {
			if err := os.Remove(filepath.Join(agentDir, filepath.Base(target))); err != nil {
				return "", fmt.Errorf("删除链接: %w", err)
			}
		}
		return "", nil
	}
	var taken []string
	for _, target := range targets {
		switch skillLink(agentDir, target) {
		case linkOurs:
			// linked already
		case linkTaken:
			taken = append(taken, filepath.Base(target))
		default:
			if err := os.MkdirAll(agentDir, 0o755); err != nil {
				return "", fmt.Errorf("创建目录: %w", err)
			}
			if err := os.Symlink(target, filepath.Join(agentDir, filepath.Base(target))); err != nil {
				return "", fmt.Errorf("创建链接: %w", err)
			}
		}
	}
	if len(taken) > 0 {
		return fmt.Sprintf("跳过 %d 个在 %s 中已存在的技能：%s", len(taken), agentName, strings.Join(taken, "、")), nil
	}
	return "", nil
}
