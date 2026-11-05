package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// cmdSpec is what the 命令行 way's 解析 got out of a `skills add`
// command: the source it names and the skills its --skill options pick,
// empty for all of them.
type cmdSpec struct {
	source string
	skills []string
}

// splitShell splits a command line the way a shell would: blanks part
// the words, single and double quotes group them, and a backslash
// escapes the next character outside single quotes.
func splitShell(s string) ([]string, error) {
	var toks []string
	var tok strings.Builder
	var quote rune // '\'' or '"', 0 outside quotes
	flush := func() {
		if tok.Len() > 0 {
			toks = append(toks, tok.String())
			tok.Reset()
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				tok.WriteRune(r)
			}
		case quote == '"':
			if r == '"' {
				quote = 0
			} else if r == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\') {
				i++
				tok.WriteRune(runes[i])
			} else {
				tok.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '\\' && i+1 < len(runes):
			i++
			tok.WriteRune(runes[i])
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			tok.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("命令的引号没有闭合")
	}
	// An empty pair of quotes is a word too ("--skill ''"), but the
	// builder cannot tell it from nothing; such a command is invalid
	// anyway, and the parse below says so.
	flush()
	return toks, nil
}

// parseSkillsCmd reads a `skills add` command — `npx skills add …`,
// `npx -y skills@latest add …`, a plain `skills add …` — into its source
// and --skill picks. The options that steer an installation (-g, -a,
// -y, --all and the like) do not steer an import and are ignored.
func parseSkillsCmd(raw string) (cmdSpec, error) {
	toks, err := splitShell(raw)
	if err != nil {
		return cmdSpec{}, err
	}
	// The command is its "add" word, which a "skills" word — the
	// package, "skills@latest" too — comes before.
	at := -1
	for i, tok := range toks {
		if tok != "add" {
			continue
		}
		if slices.ContainsFunc(toks[:i], func(t string) bool { return strings.HasPrefix(t, "skills") }) {
			at = i
			break
		}
	}
	if at < 0 {
		return cmdSpec{}, fmt.Errorf("无法识别的命令，请粘贴完整的 npx skills add 命令")
	}
	var spec cmdSpec
	rest := toks[at+1:]
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		switch {
		case tok == "-s" || tok == "--skill":
			i++
			if i >= len(rest) {
				return cmdSpec{}, fmt.Errorf("%s 缺少技能名称", tok)
			}
			spec.skills = append(spec.skills, rest[i])
		case strings.HasPrefix(tok, "--skill="):
			spec.skills = append(spec.skills, strings.TrimPrefix(tok, "--skill="))
		case tok == "-a" || tok == "--agent":
			i++ // its value does not matter to an import
		case strings.HasPrefix(tok, "-"):
			// -g, -y, --all, --copy and the rest steer an installation,
			// not an import.
		case spec.source == "":
			spec.source = tok
		default:
			return cmdSpec{}, fmt.Errorf("多余的参数「%s」", tok)
		}
	}
	if spec.source == "" {
		return cmdSpec{}, fmt.Errorf("命令缺少技能来源")
	}
	return spec, nil
}

// localSkillDir tells whether the source names a directory of the
// machine — one starting with ".", "~", "%", "/" or a drive letter has
// to, any other does if it is one — and returns it expanded. A source
// that looks local but is no directory is an error, not a git address.
func localSkillDir(source string) (dir string, local bool, err error) {
	s := strings.TrimSpace(source)
	expanded := expandHome(s)
	looks := strings.HasPrefix(s, ".") || strings.HasPrefix(s, "~") ||
		strings.HasPrefix(s, "%") || strings.HasPrefix(s, "/") || filepath.IsAbs(expanded)
	if st, statErr := os.Stat(expanded); statErr == nil && st.IsDir() {
		return expanded, true, nil
	}
	if looks {
		return "", true, fmt.Errorf("本地目录「%s」不存在或不是目录", s)
	}
	return "", false, nil
}

// filterFound keeps the skills the command's --skill options name; "*"
// keeps them all. A name nothing answers to is an error.
func filterFound(skills []foundSkill, names []string) ([]foundSkill, error) {
	if len(names) == 0 || slices.Contains(names, "*") {
		return skills, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []foundSkill
	for _, sk := range skills {
		if want[sk.Name] {
			out = append(out, sk)
			delete(want, sk.Name)
		}
	}
	if len(want) > 0 {
		return nil, fmt.Errorf("来源中没有技能「%s」", strings.Join(slices.Sorted(maps.Keys(want)), "」、「"))
	}
	return out, nil
}

// cmdResult is the outcome of the 命令行 way's 解析: the skills found
// and where they came from. root is the temporary clone of a git
// source, for the caller to remove; a local source leaves it "", as its
// directory is the user's own.
type cmdResult struct {
	root   string
	commit string
	source string // importMeta.Source: "git" or "local"
	skills []foundSkill
}

// cmdFetch is what the 命令行 way's 解析 calls; a variable, so the
// tests can stub it.
var cmdFetch = fetchCmd

// fetchCmd resolves the command's source: a local directory is scanned
// where it lies, anything else is a git address for the GitHub way's
// machinery. The --skill picks filter what is found.
func fetchCmd(ctx context.Context, spec cmdSpec) (*cmdResult, error) {
	if dir, local, err := localSkillDir(spec.source); local {
		if err != nil {
			return nil, err
		}
		skills, err := filterFound(findGitSkills(dir), spec.skills)
		if err != nil {
			return nil, err
		}
		if len(skills) == 0 {
			return nil, fmt.Errorf("目录「%s」中没有含 SKILL.md 的技能", spec.source)
		}
		return &cmdResult{source: "local", skills: skills}, nil
	}
	res, err := gitFetch(ctx, spec.source, "")
	if err != nil {
		return nil, err
	}
	skills, err := filterFound(res.skills, spec.skills)
	if err != nil {
		os.RemoveAll(res.root)
		return nil, err
	}
	return &cmdResult{root: res.root, commit: res.commit, source: "git", skills: skills}, nil
}
