//go:build !windows

package main

import "os"

// dirLink makes link a symbolic link to the directory target.
func dirLink(target, link string) error {
	return os.Symlink(target, link)
}
