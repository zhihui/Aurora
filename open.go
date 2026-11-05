package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// openDir opens a directory in the system's file manager: Finder on
// macOS, Explorer on Windows, the default of xdg-open elsewhere. It is
// a variable, so the tests can stub it. Opening never creates the
// directory: a missing one is an error.
var openDir = func(dir string) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("目录不存在")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Explorer exits nonzero on success, so the exit status is not
	// checked; Wait only reaps the child.
	go func() { _ = cmd.Wait() }()
	return nil
}
