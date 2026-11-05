package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// configDir is the directory of the app's configuration: .aurora in the
// user's home, on every platform — ~/.aurora on macOS and Linux,
// %USERPROFILE%\.aurora on Windows. Every file the app saves lives here;
// nothing goes to the platform's application data directory.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aurora"), nil
}

// ensureConfig creates the app's configuration where it is missing — the
// config directory, the skills directory the hub reads, and an empty
// agents.json — and returns the config directory. It runs once at
// startup.
func ensureConfig() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		return "", err
	}
	agents := filepath.Join(dir, "agents.json")
	if _, err := os.Stat(agents); os.IsNotExist(err) {
		if err := os.WriteFile(agents, []byte("[]\n"), 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// expandHome is the inverse of displayHome: it expands the home prefix
// as the platform's users write it — ~/… on macOS and Linux,
// %USERPROFILE%\… (and ~\…) on Windows — for the filesystem operations.
// Paths without the prefix come back as they are.
func expandHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	rest := ""
	switch {
	case dir == "~":
		// home itself
	case strings.HasPrefix(dir, "~/"):
		rest = dir[2:]
	case strings.HasPrefix(dir, `~\`):
		rest = strings.ReplaceAll(dir[2:], `\`, "/")
	case runtime.GOOS == "windows" && strings.EqualFold(dir, `%USERPROFILE%`):
		// home itself
	case runtime.GOOS == "windows" &&
		(strings.HasPrefix(strings.ToUpper(dir), `%USERPROFILE%/`) ||
			strings.HasPrefix(strings.ToUpper(dir), `%USERPROFILE%\`)):
		rest = strings.ReplaceAll(dir[len(`%USERPROFILE%`)+1:], `\`, "/")
	default:
		return dir
	}
	if rest == "" {
		return home
	}
	return filepath.Join(home, filepath.FromSlash(rest))
}

// displayHome shortens a path under the user's home for display, as the
// platform's users write it: ~/… on macOS and Linux, %USERPROFILE%\… on
// Windows. Paths not under the home show as they are.
func displayHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || (dir != home && !strings.HasPrefix(dir, home+string(os.PathSeparator))) {
		return dir
	}
	if runtime.GOOS == "windows" {
		return `%USERPROFILE%` + dir[len(home):]
	}
	return "~" + dir[len(home):]
}
