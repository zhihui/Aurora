//go:build windows

package main

import (
	"os"
	"os/exec"
)

// dirLink makes link point at the directory target: a symbolic link
// where Windows allows one — it takes a privilege or developer mode —
// else a junction, which a directory link needs neither for. Go reads
// both as a symlink (Lstat's ModeSymlink, Readlink, EvalSymlinks).
func dirLink(target, link string) error {
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}
