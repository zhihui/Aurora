package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// repoRefs is what a remote advertises: its branch heads, and the head
// of the default branch where the remote tells it.
type repoRefs struct {
	branches map[string]string // branch name → head commit
	head     string            // the default branch's head commit, "" unknown
}

// listRemoteRefs asks a remote for its refs; a variable, so the tests
// can stub it. One call serves every skill of the repository.
var listRemoteRefs = fetchRemoteRefs

func fetchRemoteRefs(ctx context.Context, cloneURL string) (*repoRefs, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{URLs: []string{cloneURL}})
	refs, err := rem.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return nil, err
	}
	r := &repoRefs{branches: map[string]string{}}
	for _, ref := range refs {
		switch {
		case ref.Name().IsBranch():
			r.branches[ref.Name().Short()] = ref.Hash().String()
		case ref.Name() == "HEAD":
			r.head = ref.Hash().String()
		}
	}
	return r, nil
}

// updateClone clones one branch of a repository, shallowly; a variable,
// so the tests can stub it.
var updateClone = cloneBranch

// cloneBranch is the update's fetch: a shallow clone of the branch in a
// temporary directory the caller removes.
func cloneBranch(ctx context.Context, cloneURL, branch string) (root, commit string, err error) {
	dir, err := os.MkdirTemp("", "aurora-clone-*")
	if err != nil {
		return "", "", err
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
			return "", "", ctx.Err()
		}
		return "", "", fmt.Errorf("克隆失败，请检查网络: %v", err)
	}
	if head, err := repo.Head(); err == nil {
		commit = head.Hash().String()
	}
	return dir, commit, nil
}

// updateItem is a git skill with an update waiting: what the check
// found, whether the user picked it, and how its update went.
type updateItem struct {
	Dir, Name string
	Meta      importMeta
	NewCommit string
	Checked   bool
	Done      bool   // its update landed
	Err       string // why its update failed
}

// updateGroup is one clone's worth of updates: the skills of a
// repository and branch, updated together so the repository is cloned
// once however many of them there are.
type updateGroup struct {
	CloneURL string
	Branch   string
	Items    []updateItem
	Err      string // why the whole group's update failed
}

// repoLabel is a clone URL as the update modal shows it: owner/repo.
func repoLabel(cloneURL string) string {
	s := strings.TrimSuffix(cloneURL, ".git")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// scp-like SSH, git@host:owner/repo.
	if at := strings.Index(s, "@"); at >= 0 {
		if colon := strings.Index(s[at:], ":"); colon >= 0 {
			s = s[at+colon+1:]
		}
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return s
}

// skillBranch is the branch a skill's updates come from: the one the
// import tracked, the one its link names, or the repository's default.
func skillBranch(meta importMeta, refs *repoRefs) string {
	if meta.Branch != "" {
		return meta.Branch
	}
	if _, rest, err := parseGitURL(meta.URL); err == nil && rest != "" {
		names := make([]string, 0, len(refs.branches))
		for name := range refs.branches {
			names = append(names, name)
		}
		branch, _ := resolveTreeRest(names, rest, "")
		return branch
	}
	for name, head := range refs.branches {
		if head == refs.head {
			return name
		}
	}
	// The remote did not name its default branch; the usual ones are the
	// best guess left.
	for _, name := range []string{"main", "master"} {
		if _, ok := refs.branches[name]; ok {
			return name
		}
	}
	return ""
}

// gitSkills are the hub's skills a git import brought, the ones the
// update cares about.
func (a *app) gitSkills() []skill {
	var out []skill
	for _, sk := range a.skills {
		if sk.HasMeta && sk.Meta.Source == "git" {
			out = append(out, sk)
		}
	}
	return out
}

// checkUpdates asks every repository the hub's git skills came from for
// its branch heads — one call per repository however many skills it
// gave — and opens the update modal with what is newer than what the
// hub recorded. The check runs in the background; its result lands on
// the main thread through runOnMain, stale generations dropped.
func (a *app) checkUpdates() {
	if a.updateChecking || a.updating {
		return
	}
	targets := a.gitSkills()
	if len(targets) == 0 {
		return
	}
	a.updateChecking = true
	a.updateGen++
	gen := a.updateGen
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	go func() {
		avail, groups, errs := checkTargets(ctx, targets)
		runOnMain(func() {
			if gen != a.updateGen {
				return
			}
			a.updateChecking = false
			a.updateCancel = nil
			if ctx.Err() != nil {
				return
			}
			a.updateAvail = avail
			a.updateGroups = groups
			a.updateErrs = errs
			a.updateFinished = false
			a.updateOpen = true
		})
	}()
}

// checkTargets compares the hub's records with the remotes': the skills
// with a newer commit, grouped by repository and branch so one clone
// updates them all, and what the check could not place — a repository
// that would not answer, a branch that is gone.
func checkTargets(ctx context.Context, targets []skill) (avail map[string]bool, groups []updateGroup, errs []string) {
	avail = map[string]bool{}
	refs := map[string]*repoRefs{}
	byGroup := map[string]int{}
	for _, sk := range targets {
		cloneURL, _, err := parseGitURL(sk.Meta.URL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", sk.Name, err))
			continue
		}
		r, seen := refs[cloneURL]
		if !seen {
			r, err = listRemoteRefs(ctx, cloneURL)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", repoLabel(cloneURL), err))
				r = nil
			}
			refs[cloneURL] = r
		}
		if r == nil {
			// The repository's failure is already reported.
			continue
		}
		branch := skillBranch(sk.Meta, r)
		head := r.branches[branch]
		if head == "" {
			errs = append(errs, fmt.Sprintf("%s: 仓库中找不到分支「%s」", sk.Name, branch))
			continue
		}
		if head == sk.Meta.Commit {
			continue
		}
		avail[sk.Dir] = true
		key := cloneURL + "\n" + branch
		gi, ok := byGroup[key]
		if !ok {
			gi = len(groups)
			byGroup[key] = gi
			groups = append(groups, updateGroup{CloneURL: cloneURL, Branch: branch})
		}
		groups[gi].Items = append(groups[gi].Items, updateItem{
			Dir: sk.Dir, Name: sk.Name, Meta: sk.Meta, NewCommit: head, Checked: true,
		})
	}
	return avail, groups, errs
}

// runUpdates updates the skills picked in the modal: each group is one
// clone, its skills copied over what the hub has, the records refreshed
// with the new commit — old records gain the branch and path they never
// noted. The work runs in the background on a deep copy of the groups,
// and the modal lists the per-skill results when it lands.
func (a *app) runUpdates() {
	if a.updating || a.updateChecking {
		return
	}
	groups := make([]updateGroup, len(a.updateGroups))
	for i, g := range a.updateGroups {
		groups[i] = g
		groups[i].Items = slices.Clone(g.Items)
	}
	a.updating = true
	a.updateGen++
	gen := a.updateGen
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	skillsDir := a.skillsDir
	go func() {
		for gi := range groups {
			g := &groups[gi]
			root, commit, err := updateClone(ctx, g.CloneURL, g.Branch)
			if err != nil {
				if ctx.Err() == nil {
					g.Err = err.Error()
				}
				continue
			}
			for ii := range g.Items {
				it := &g.Items[ii]
				if !it.Checked {
					continue
				}
				src, rel := locateUpdate(root, *it)
				if src == "" {
					it.Err = "上游已不存在该技能"
					continue
				}
				meta := it.Meta
				meta.Commit = commit
				meta.ImportedAt = time.Now()
				if meta.Branch == "" {
					meta.Branch = g.Branch
				}
				if meta.Path == "" {
					meta.Path = rel
				}
				if err := importSkill(src, skillsDir, it.Dir, meta); err != nil {
					it.Err = err.Error()
				} else {
					it.Done = true
					it.Meta = meta
				}
			}
			_ = os.RemoveAll(root)
		}
		runOnMain(func() {
			if gen != a.updateGen {
				return
			}
			a.updating = false
			a.updateCancel = nil
			a.updateGroups = groups
			for _, g := range groups {
				for _, it := range g.Items {
					if it.Done {
						delete(a.updateAvail, it.Dir)
					}
				}
			}
			a.updateFinished = true
			a.loadSkills()
		})
	}()
}

// locateUpdate finds the skill's directory in a fresh clone: the
// recorded path first; a record from before paths — where the import
// noted none — falls back to the skill's identity, the directory's name
// first and the declared name where exactly one matches. The second
// return is the path to record, "/" separated.
func locateUpdate(root string, it updateItem) (src, rel string) {
	if it.Meta.Path != "" {
		src = filepath.Join(root, filepath.FromSlash(it.Meta.Path))
		if st, err := os.Stat(filepath.Join(src, "SKILL.md")); err == nil && !st.IsDir() {
			return src, it.Meta.Path
		}
		return "", ""
	}
	var matches []foundSkill
	for _, f := range findGitSkills(root) {
		if filepath.Base(f.dir) == it.Dir {
			return f.dir, slashRel(root, f.dir)
		}
		if f.Name == it.Name {
			matches = append(matches, f)
		}
	}
	if len(matches) == 1 {
		return matches[0].dir, slashRel(root, matches[0].dir)
	}
	return "", ""
}

// slashRel is dir inside root, "/" separated for the import record;
// the root itself is "".
func slashRel(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// closeUpdate closes the update modal: a check or update still running
// is canceled, its generation dropping the result. Whatever an update
// copied before the cancel stays, so the hub's list is read again.
func (a *app) closeUpdate() {
	a.updateOpen = false
	a.updateGen++
	if a.updateCancel != nil {
		a.updateCancel()
		a.updateCancel = nil
	}
	a.updateChecking = false
	a.updating = false
	a.updateFinished = false
	a.updateErrs = nil
	a.loadSkills()
}

// updateCount is how many of the modal's skills wait for their update.
func (a *app) updateCount() int {
	n := 0
	for _, g := range a.updateGroups {
		for _, it := range g.Items {
			if it.Checked && !it.Done && it.Err == "" {
				n++
			}
		}
	}
	return n
}

// updateModal is the 更新技能 dialog: the check's outcome — nothing to
// do, or the updates grouped by repository, each group cloned once —
// and then the update's per-skill results.
func (a *app) updateModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.updateOpen, func() {
		// ui.Modal's panel pads its content by t.Space(5); the negative
		// margin cancels it, so the sections' own padding rules and the
		// dividers span the modal edge to edge, as in the 导入技能 modal.
		ui.Column(c).Width(520).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "更新技能").FontSize(15).Bold().Grow(1)
					modalClose(c, a.closeUpdate)
				})
				ui.Text(c, "同一仓库的技能共用一次克隆；更新会覆盖本地内容，agent 的链接不受影响。").FontSize(12).TextColor(t.TextMuted)
			})
			ui.Divider(c)
			ui.Scroll(c).Height(300).Children(func() {
				ui.Column(c).Padding(14, 16).Gap(12).Children(func() {
					a.updateBody(c)
				})
			})
			ui.Divider(c)
			a.updateFooter(c)
		})
	})
}

// updateBody is the modal's content: the check's failures, and the
// updates by repository — or that there is nothing to do.
func (a *app) updateBody(c *ui.Context) {
	t := c.Theme()
	if len(a.updateGroups) == 0 {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, checkCircleIcon).Size(14, 14).TextColor(t.Success)
			ui.Text(c, "所有 git 来源的技能已是最新。").FontSize(13)
		})
	}
	a.updateErrList(c)
	for gi := range a.updateGroups {
		a.updateGroupView(c, gi)
	}
}

// updateErrList is what the check could not place, in the danger color.
func (a *app) updateErrList(c *ui.Context) {
	t := c.Theme()
	for _, e := range a.updateErrs {
		ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
			ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
			ui.Text(c, e).FontSize(12).TextColor(t.Danger).FixedLineHeight(18)
		})
	}
}

// updateGroupView is one repository of the modal: its name and branch,
// the group's own failure where the clone would not come, and its
// skills as rows to choose.
func (a *app) updateGroupView(c *ui.Context, gi int) {
	t := c.Theme()
	g := &a.updateGroups[gi]
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, githubIcon).Size(13, 13).TextColor(t.TextMuted)
			ui.Text(c, repoLabel(g.CloneURL)).FontSize(12).FontWeight(500).SingleLine()
			if g.Branch != "" {
				tag(c, g.Branch)
			}
		})
		if g.Err != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
				ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
				ui.Text(c, g.Err).FontSize(12).TextColor(t.Danger).FixedLineHeight(18)
			})
		}
		for ii := range g.Items {
			a.updateItemRow(c, gi, ii)
		}
	})
}

// updateItemRow is one skill of a group: a checkbox, its name, and its
// state — the commit waiting, 已更新 once it landed, or why it failed.
// A click anywhere on the row toggles the choice; a row whose update is
// done or impossible is still.
func (a *app) updateItemRow(c *ui.Context, gi, ii int) {
	t := c.Theme()
	it := &a.updateGroups[gi].Items[ii]
	still := a.updating || it.Done || it.Err != ""
	row := ui.ButtonBase(c.Key("upd:"+it.Dir)).AlignItems(ui.Center).Gap(12).Padding(10, 12).Radius(10).
		Border(1, t.Border).Background(t.Background).Justify(ui.Start).Label(it.Name).Disabled(still)
	if row.Hovered() && !still {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Checkbox(c, &it.Checked, "").Label("选择 " + it.Name)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, it.Name).FontSize(13).Bold().SingleLine()
				if it.Dir != it.Name {
					ui.Text(c, "("+it.Dir+")").FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
			if it.Err != "" {
				ui.Text(c, it.Err).FontSize(11).TextColor(t.Danger).SingleLine()
			}
		})
		if it.Done {
			ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(2, 8).Radius(5).Shrink(0).Background(t.Success.Alpha(0.12)).Children(func() {
				ui.Icon(c, checkIcon).Size(10, 10).TextColor(t.Success)
				ui.Text(c, "已更新").FontSize(10).FontWeight(500).TextColor(t.Success).SingleLine()
			})
		} else if it.NewCommit != "" && it.Err == "" {
			commit := it.NewCommit
			if len(commit) > 7 {
				commit = commit[:7]
			}
			ui.Text(c, "→ "+commit).Font("monospace").FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
	})
	row.OnClick(func() {
		if still {
			return
		}
		it.Checked = !it.Checked
	})
}

// updateFooter is the modal's buttons: 取消 while there is a choice to
// make, 更新 running the picked ones, 完成 once the results are in.
func (a *app) updateFooter(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(10, 16).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "只统计 git 来源的技能").FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine()
		if a.updateFinished {
			ui.PrimaryButton(c, "完成").OnClick(func() { a.closeUpdate() })
			return
		}
		ui.Button(c, "取消").OnClick(func() { a.closeUpdate() })
		label := "更新"
		disabled := a.updating || a.updateCount() == 0
		if a.updating {
			label = "更新中…"
		} else if n := a.updateCount(); n > 0 {
			label = fmt.Sprintf("更新 %d 个技能", n)
		}
		ui.PrimaryButton(c, label).Disabled(disabled).OnClick(func() { a.runUpdates() })
	})
}
