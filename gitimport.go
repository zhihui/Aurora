package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// importMetaFile is the file recording where an imported skill came
// from, inside the skill's directory. It lives and dies with the skill:
// removeSkill's os.RemoveAll takes it away, and nothing can leave it
// behind.
const importMetaFile = ".aurora.json"

// importMeta is what importMetaFile holds: the way the skill was
// imported and the link the user gave. Branch and Path are what an
// update of a git skill goes by — the branch the import tracked and the
// skill's directory inside the repository; records from before them
// have neither, and the update resolves the branch again and falls back
// to matching by name.
type importMeta struct {
	Source     string    `json:"source"` // "git", "url" or "local"
	URL        string    `json:"url"`    // the link the user gave
	Commit     string    `json:"commit,omitempty"`
	Branch     string    `json:"branch,omitempty"` // the branch the import tracked, "" for the link's or the default
	Path       string    `json:"path,omitempty"`   // the skill's directory inside the repository, "/" separated
	ImportedAt time.Time `json:"importedAt"`
}

// parseGitURL splits the link the user gave into the URL git clones and
// the tail of a subdirectory link — "/tree/<branch>/<subdir>", or
// "/-/tree/…" of GitLab — "" for a whole repository. It takes owner/repo
// shorthand, http(s) and scp-like SSH addresses.
func parseGitURL(raw string) (cloneURL, treeRest string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", fmt.Errorf("请输入仓库链接")
	}
	// owner/repo shorthand is GitHub.
	if !strings.Contains(s, "://") && !strings.Contains(s, "@") {
		parts := strings.Split(strings.Trim(s, "/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("无法识别的地址「%s」", s)
		}
		return "https://github.com/" + parts[0] + "/" + parts[1], "", nil
	}
	// scp-like SSH, git@host:owner/repo.git, has no subdirectory form.
	if !strings.Contains(s, "://") {
		return s, "", nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("无法识别的地址「%s」", s)
	}
	u.RawQuery, u.Fragment = "", ""
	path := strings.TrimSuffix(u.Path, "/")
	for _, marker := range []string{"/-/tree/", "/tree/"} {
		if i := strings.Index(path, marker); i >= 0 {
			u.Path = strings.TrimSuffix(path[:i], ".git")
			return u.String(), path[i+len(marker):], nil
		}
	}
	u.Path = strings.TrimSuffix(path, ".git")
	return u.String(), "", nil
}

// resolveTreeRest picks the branch and the subdirectory of a
// "/tree/<branch>/<subdir>" tail: the longest head of the tail naming a
// branch of the repository is the branch, what is left the subdirectory.
// A branch the user gave wins. Without a tail both are "", which clones
// the default branch and scans everything.
func resolveTreeRest(branches []string, rest, branch string) (string, string) {
	if branch != "" {
		return branch, rest
	}
	if rest == "" {
		return "", ""
	}
	segs := strings.Split(rest, "/")
	for i := len(segs); i > 0; i-- {
		cand := strings.Join(segs[:i], "/")
		if slices.Contains(branches, cand) {
			return cand, strings.Join(segs[i:], "/")
		}
	}
	// No branch matched: the first segment is the branch, as the link
	// has it, and the clone says if it is not.
	return segs[0], strings.Join(segs[1:], "/")
}

// foundSkill is a skill found in a clone: what its SKILL.md says and
// where it is.
type foundSkill struct {
	skill
	dir string // its directory, inside the clone
}

// findGitSkills walks root for directories with a SKILL.md — each is a
// skill, and is not walked deeper. Hidden directories, .git among them,
// are skipped.
func findGitSkills(root string) []foundSkill {
	var found []foundSkill
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
		if err != nil {
			return nil
		}
		found = append(found, foundSkill{parseSkill(data, d.Name()), path})
		return fs.SkipDir
	})
	return found
}

// gitResult is the outcome of the GitHub way's 解析: a temporary clone
// and the skills of the subdirectory the link points at, or of the whole
// repository.
type gitResult struct {
	root   string // the temporary clone; the caller removes it
	commit string
	skills []foundSkill
}

// gitFetch is what the GitHub way's 解析 calls; a variable, so the tests
// can stub it.
var gitFetch = fetchGit

// fetchGit clones what the link names — shallow, the one branch — into a
// temporary directory and lists its skills. Canceling the context stops
// it, and whatever the clone left behind is removed either way: an error
// never leaves the directory.
func fetchGit(ctx context.Context, rawURL, branch string) (*gitResult, error) {
	cloneURL, rest, err := parseGitURL(rawURL)
	if err != nil {
		return nil, err
	}
	// The branches of the repository resolve the link's tail.
	var branches []string
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{URLs: []string{cloneURL}})
	if refs, err := rem.ListContext(ctx, &git.ListOptions{}); err == nil {
		for _, r := range refs {
			if r.Name().IsBranch() {
				branches = append(branches, r.Name().Short())
			}
		}
	}
	branch, sub := resolveTreeRest(branches, rest, strings.TrimSpace(branch))
	dir, err := os.MkdirTemp("", "aurora-clone-*")
	if err != nil {
		return nil, err
	}
	opts := &git.CloneOptions{URL: cloneURL, Depth: 1}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		opts.SingleBranch = true
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, opts)
	if err != nil {
		os.RemoveAll(dir)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("克隆失败，请检查链接与网络: %v", err)
	}
	commit := ""
	if head, err := repo.Head(); err == nil {
		commit = head.Hash().String()
	}
	root := filepath.Join(dir, filepath.FromSlash(sub))
	skills := findGitSkills(root)
	if len(skills) == 0 {
		os.RemoveAll(dir)
		if sub != "" {
			return nil, fmt.Errorf("子目录「%s」中没有含 SKILL.md 的技能", sub)
		}
		return nil, fmt.Errorf("仓库中没有含 SKILL.md 的技能")
	}
	return &gitResult{root: dir, commit: commit, skills: skills}, nil
}

// importSkill copies a found skill into the hub — over what the hub has
// of the name, which goes away whole first — recording where it came
// from. The agents' links to the skill point at its path, so they keep
// working over an overwrite. The source being the destination — a link
// to the hub's skill imported again, the hub picked as a local folder —
// is refused: removing the old copy first would delete the source.
func importSkill(src, skillsDir, name string, meta importMeta) error {
	dst := filepath.Join(skillsDir, name)
	if samePath(src, dst) {
		return fmt.Errorf("「%s」的源与目标相同，取消导入", name)
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("覆盖「%s」: %w", name, err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return fmt.Errorf("复制「%s」: %w", name, err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(dst, importMetaFile), append(data, '\n'), 0o644)
	}
	return nil
}

// loadImportMeta reads the record of where a skill came from, false
// where it has none — a skill the hub was given by hand, say.
func loadImportMeta(dir string) (importMeta, bool) {
	data, err := os.ReadFile(filepath.Join(dir, importMetaFile))
	if err != nil {
		return importMeta{}, false
	}
	var meta importMeta
	if json.Unmarshal(data, &meta) != nil {
		return importMeta{}, false
	}
	return meta, true
}

// sourceLabel is the card's tag of where a skill came from: the host of
// a git or URL link, 本地文件 of a local import, 未知来源 without a
// record.
func sourceLabel(meta importMeta, ok bool) string {
	if !ok {
		return "未知来源"
	}
	switch meta.Source {
	case "git":
		if host := gitHost(meta.URL); host != "" {
			return host
		}
		return "Git"
	case "url":
		if u, err := url.Parse(meta.URL); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
		return "链接"
	case "local":
		return "本地文件"
	case "paste":
		return "手动创建"
	default:
		return "未知来源"
	}
}

// gitHost is the host of a git link, http(s) or scp-like SSH
// (git@host:owner/repo); owner/repo shorthand is GitHub, as parseGitURL
// resolves it.
func gitHost(raw string) string {
	clone, _, err := parseGitURL(raw)
	if err != nil {
		return ""
	}
	if strings.Contains(clone, "://") {
		if u, err := url.Parse(clone); err == nil {
			return u.Hostname()
		}
		return ""
	}
	if at := strings.Index(clone, "@"); at >= 0 {
		if i := strings.Index(clone[at+1:], ":"); i > 0 {
			return clone[at+1:][:i]
		}
	}
	return ""
}
