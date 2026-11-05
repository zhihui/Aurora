package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// importMethods are the ways the 导入技能 modal offers, with their
// subtitles and icons.
var importMethods = []struct {
	title, sub string
	icon       *ui.SVG
}{
	{"GitHub", "仓库 / 子目录", githubIcon},
	{"命令行", "npx skills add", terminalIcon},
	{"链接 URL", "SKILL.md / zip", linkIcon},
	{"本地", "文件夹 / 压缩包", folderIcon},
	{"粘贴 / 新建", "直接写 SKILL.md", clipboardIcon},
}

// parsedSkill is a skill the modal's 解析 found: whether it is chosen
// for the import, and whether the hub already has one of its name — an
// import of those overwrites. src is its directory: in the temporary
// clone or unpack, or the user's own folder of a local pick.
type parsedSkill struct {
	Name, Desc string
	Checked    bool
	Exists     bool
	src        string
}

// importCount is how many of the parsed skills are chosen.
func (a *app) importCount() int {
	n := 0
	for _, p := range a.importParsed {
		if p.Checked {
			n++
		}
	}
	return n
}

// clearClone removes the temporary clone of the last 解析.
func (a *app) clearClone() {
	if a.importClone != "" {
		_ = os.RemoveAll(a.importClone)
		a.importClone = ""
	}
}

// closeImport closes the modal: a 解析 still running is canceled, and
// the temporary clone is removed with it.
func (a *app) closeImport() {
	a.importOpen = false
	a.parseGen++
	if a.importCancel != nil {
		a.importCancel()
		a.importCancel = nil
	}
	a.importParsing = false
	a.clearClone()
}

// runOnMain runs a function on the main thread, where the view and the
// app's state live; a variable, so the tests can stub it. The background
// 解析 delivers its result through it.
var runOnMain = mygo.RunOnMain

// startParse runs a way's 解析 in the background — the modal's 解析 and
// the ways to import disable meanwhile — and lists the skills found, the
// hub's own marked 已存在 and unchosen, the others chosen. fetch reports
// the skills, where they came from (importMeta's source and commit) and
// a temporary directory for the caller to own, "" when there is none;
// branch is the branch the 解析 tracked, for the import record. A
// failure shows in the modal; closing the modal cancels it.
func (a *app) startParse(origin, branch string, fetch func(context.Context) (root, commit, source string, skills []foundSkill, err error)) {
	if a.importParsing {
		return
	}
	a.clearClone()
	a.importParsed = nil
	a.importErr = ""
	a.importParsing = true
	a.parseGen++
	gen := a.parseGen
	ctx, cancel := context.WithCancel(context.Background())
	a.importCancel = cancel
	go func() {
		root, commit, source, skills, err := fetch(ctx)
		runOnMain(func() {
			if gen != a.parseGen {
				// A newer 解析 or the closed modal makes this stale; its
				// temporary directory goes away unused.
				if root != "" {
					_ = os.RemoveAll(root)
				}
				return
			}
			a.importParsing = false
			a.importCancel = nil
			if err != nil {
				if ctx.Err() == nil {
					a.importErr = err.Error()
				}
				return
			}
			a.importClone = root
			a.importCommit = commit
			a.importOrigin = origin
			a.importSource = source
			a.importGitBranch = branch
			a.applyParsed(skills)
		})
	}()
}

// parseGit runs the GitHub way's 解析: it clones what the link names.
func (a *app) parseGit() {
	repo, branch := a.importRepo, a.importBranch
	a.startParse(strings.TrimSpace(repo), strings.TrimSpace(branch), func(ctx context.Context) (string, string, string, []foundSkill, error) {
		res, err := gitFetch(ctx, repo, branch)
		if err != nil {
			return "", "", "", nil, err
		}
		return res.root, res.commit, "git", res.skills, nil
	})
}

// parseCmd runs the 命令行 way's 解析: it reads the `skills add`
// command — a mistyped one fails right away — and fetches its source,
// git or local.
func (a *app) parseCmd() {
	spec, err := parseSkillsCmd(a.importCmd)
	if err != nil {
		a.clearClone()
		a.importParsed = nil
		a.importErr = err.Error()
		return
	}
	a.startParse(spec.source, "", func(ctx context.Context) (string, string, string, []foundSkill, error) {
		res, err := cmdFetch(ctx, spec)
		if err != nil {
			return "", "", "", nil, err
		}
		return res.root, res.commit, res.source, res.skills, nil
	})
}

// parseURL runs the 链接 URL way's 解析: it downloads the SKILL.md or
// zip the link points at.
func (a *app) parseURL() {
	raw := strings.TrimSpace(a.importURL)
	if raw == "" {
		a.clearClone()
		a.importParsed = nil
		a.importErr = "请输入文件链接"
		return
	}
	a.startParse(raw, "", func(ctx context.Context) (string, string, string, []foundSkill, error) {
		res, err := urlFetch(ctx, raw)
		if err != nil {
			return "", "", "", nil, err
		}
		return res.root, "", "url", res.skills, nil
	})
}

// applyParsed lists the skills a 解析 found: the hub's own marked 已存在
// and unchosen — importing one of them overwrites it — the others
// chosen.
func (a *app) applyParsed(skills []foundSkill) {
	taken := map[string]bool{}
	for _, sk := range a.skills {
		taken[sk.Name] = true
		taken[sk.Dir] = true
	}
	for _, f := range skills {
		p := parsedSkill{Name: f.Name, Desc: f.Desc, src: f.dir}
		if taken[p.Name] {
			p.Exists = true
		} else {
			p.Checked = true
		}
		a.importParsed = append(a.importParsed, p)
	}
}

// importSelected copies the skills chosen in the modal into the hub —
// over the ones the hub has of their names — recording where each came
// from; a folder picked in the 本地 way with 链接导入 on is linked
// instead of copied. It reports how many imported; a failure shows in
// the modal and what was copied stays.
func (a *app) importSelected() int {
	if a.importMethod < 0 || a.importMethod > 3 {
		return 0
	}
	a.importErr = ""
	n := 0
	for _, p := range a.importParsed {
		if !p.Checked {
			continue
		}
		var err error
		if a.importMethod == 3 && a.importLink && a.importLocalDir != "" {
			err = linkSkill(p.src, a.skillsDir, p.Name)
		} else {
			meta := importMeta{Source: a.importSource, URL: a.importOrigin, Commit: a.importCommit, ImportedAt: time.Now()}
			if a.importSource == "git" {
				// What an update of the skill goes by: the branch the
				// 解析 tracked, and the skill's directory inside the
				// clone.
				meta.Branch = a.importGitBranch
				if a.importClone != "" {
					if rel, rerr := filepath.Rel(a.importClone, p.src); rerr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
						meta.Path = filepath.ToSlash(rel)
					}
				}
			}
			err = importSkill(p.src, a.skillsDir, p.Name, meta)
		}
		if err != nil {
			a.importErr = err.Error()
			break
		}
		n++
	}
	a.loadSkills()
	if a.importErr == "" {
		a.closeImport()
		a.importParsed = nil
	}
	return n
}

// skillsDirDisplay is the hub's skills directory as the user sees it:
// ~/.aurora/skills/ on macOS and Linux, %USERPROFILE%\.aurora\skills\
// on Windows.
func skillsDirDisplay() string {
	if dir, err := configDir(); err == nil {
		return displayHome(filepath.Join(dir, "skills")) + string(filepath.Separator)
	}
	return "~/.aurora/skills/"
}

// importSkillModal is the dialog importing a skill: the ways to import
// on the left, the chosen way's form on the right. The GitHub, 命令行,
// 链接 URL and 本地 ways parse and import for real; 粘贴 / 新建 has no
// 解析, it writes what it is given.
func (a *app) importSkillModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.importOpen, func() {
		// ui.Modal's panel pads its content by t.Space(5); the negative
		// margin cancels it, so the sections' own padding rules and the
		// dividers span the modal edge to edge.
		ui.Column(c).Width(680).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "导入技能").FontSize(15).Bold().Grow(1)
					modalClose(c, a.closeImport)
				})
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "从外部来源复制技能到").FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, skillsDirDisplay()).Font("monospace").FontSize(12).TextColor(t.TextMuted)
				})
			})
			ui.Divider(c)
			ui.Row(c).Height(340).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(180).Shrink(0).Padding(8).Gap(2).Children(func() {
					for i, m := range importMethods {
						a.importMethodRow(c, i, m.title, m.sub, m.icon)
					}
				})
				ui.Box(c).Width(1).Shrink(0).Background(t.Border)
				if a.importMethod == 4 {
					// 粘贴 / 新建 has no parsed list to scroll: its form
					// fills the body, and with it the SKILL.md content.
					ui.Column(c).Grow(1).MinWidth(0).Padding(14, 16).Gap(12).Children(func() {
						a.importForm(c)
					})
				} else {
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).MinWidth(0).Padding(14, 16).Gap(12).Children(func() {
							a.importForm(c)
						})
					})
				}
			})
			ui.Divider(c)
			ui.Row(c).Padding(10, 16).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "仅接受含 SKILL.md 的内容 · 复制到中心后再分配").FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine()
				ui.Button(c, "取消").OnClick(func() { a.closeImport() })
				label := "导入"
				disabled := a.importCount() == 0
				if a.importMethod == 4 {
					label = "创建"
					disabled = strings.TrimSpace(a.importName) == "" || strings.TrimSpace(a.importContent) == ""
				} else if n := a.importCount(); n > 0 {
					label = fmt.Sprintf("导入 %d 个技能", n)
				}
				imp := ui.PrimaryButton(c, label).Disabled(disabled)
				if imp.Clicked() {
					if a.importMethod == 4 {
						if a.importPasted() {
							c.Toast("已创建技能")
						}
					} else if n := a.importSelected(); n > 0 && a.importErr == "" {
						c.Toast(fmt.Sprintf("已导入 %d 个技能", n))
					}
				}
			})
		})
	})
}

// importMethodRow is one way of the modal's left list: its icon beside
// its title and subtitle, a solid accent tile with white text for the
// way chosen.
func (a *app) importMethodRow(c *ui.Context, i int, title, sub string, icon *ui.SVG) {
	t := c.Theme()
	chosen := a.importMethod == i
	row := ui.ButtonBase(c.Key("import:"+title)).Padding(9, 12).Radius(8).Justify(ui.Start).Label(title).Disabled(a.importParsing)
	switch {
	case chosen:
		row.Background(t.Accent).TextColor(t.AccentText)
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, icon).Size(16, 16)
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(13).FontWeight(500).SingleLine()
				s := ui.Text(c, sub).FontSize(11).SingleLine()
				if chosen {
					s.TextColor(t.AccentText.Alpha(0.75))
				} else {
					s.TextColor(t.TextMuted)
				}
			})
		})
	})
	row.OnClick(func() {
		if a.importParsing {
			return
		}
		a.importMethod = i
		a.importParsed = nil
		a.importErr = ""
		a.importLocalDir = ""
		a.clearClone()
	})
}

// importLabel is a form field's label, with its hint beside it.
func importLabel(c *ui.Context, label, hint string) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500)
		if hint != "" {
			ui.Text(c, hint).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// importHelp is the small muted note under a form's fields.
func importHelp(c *ui.Context, text string) {
	t := c.Theme()
	ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).FixedLineHeight(18)
}

// importErrRow is the failure of the last 解析 or 导入, under the form.
func (a *app) importErrRow(c *ui.Context) {
	if a.importErr == "" {
		return
	}
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
		ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
		ui.Text(c, a.importErr).FontSize(12).TextColor(t.Danger).FixedLineHeight(18)
	})
}

// importParseButton is a form's 解析 button, 解析中… and still while a
// 解析 runs in the background.
func (a *app) importParseButton(c *ui.Context, parse func()) {
	label := "解析"
	if a.importParsing {
		label = "解析中…"
	}
	ui.PrimaryButton(c, label).Disabled(a.importParsing).OnClick(parse)
}

// importForm is the form of the way chosen in the modal.
func (a *app) importForm(c *ui.Context) {
	switch a.importMethod {
	case 0:
		ui.Column(c).Gap(6).Children(func() {
			importLabel(c, "仓库或子目录链接", "")
			importField(c, &a.importRepo, "https://github.com/owner/repo/tree/main/skills/pdf", "仓库或子目录链接输入框")
		})
		ui.Column(c).Gap(6).Children(func() {
			importLabel(c, "分支", "留空自动取默认分支")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				importField(c, &a.importBranch, "main", "分支输入框").Grow(1)
				a.importParseButton(c, a.parseGit)
			})
		})
		importHelp(c, "支持 owner/repo、…/tree/<分支>/<子目录>、.git 地址。解析后列出含 SKILL.md 的目录供勾选。")
		a.importErrRow(c)
	case 1:
		ui.Column(c).Gap(6).Children(func() {
			importLabel(c, "skills add 命令", "")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				importField(c, &a.importCmd, "npx skills add owner/repo", "skills add 命令输入框").Grow(1)
				a.importParseButton(c, a.parseCmd)
			})
		})
		importHelp(c, "支持仓库简写、仓库/子目录链接、SSH 地址与本地目录；--skill 指定技能，-g、-a 等安装选项不影响导入。")
		a.importErrRow(c)
	case 2:
		ui.Column(c).Gap(6).Children(func() {
			importLabel(c, "文件链接", "SKILL.md / zip")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				importField(c, &a.importURL, "https://example.com/SKILL.md", "文件链接输入框").Grow(1)
				a.importParseButton(c, a.parseURL)
			})
		})
		importHelp(c, "支持 SKILL.md 原文链接，或包含技能的 zip 压缩包链接（下载上限 10 MiB）。")
		a.importErrRow(c)
	case 3:
		ui.Column(c).Gap(6).Children(func() {
			importLabel(c, "本地文件", "文件夹 / 压缩包")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "选文件夹").Disabled(a.importParsing).OnClick(func() { a.pickLocalFolder() })
				ui.Button(c, "选压缩包").Disabled(a.importParsing).OnClick(func() { a.pickLocalArchive() })
			})
		})
		if a.importLocalDir != "" {
			ui.Column(c).Gap(4).Children(func() {
				ui.Checkbox(c, &a.importLink, "链接导入：源目录更新自动同步，不复制文件").Label("链接导入")
				ui.Text(c, "在 "+skillsDirDisplay()+" 下创建指向源目录的链接").FontSize(11).TextColor(c.Theme().TextMuted)
			})
		}
		importHelp(c, "选择包含 SKILL.md 的文件夹或 zip 压缩包，选择后立即解析。")
		a.importErrRow(c)
	case 4:
		ui.Column(c).Gap(6).Shrink(0).Children(func() {
			importLabel(c, "技能名称", "")
			importField(c, &a.importName, "my-skill", "技能名称输入框")
		})
		ui.Column(c).Gap(6).Grow(1).Children(func() {
			importLabel(c, "SKILL.md 内容", "")
			importTextArea(c, &a.importContent, "---\nname: my-skill\ndescription: …\n---", "SKILL.md 内容输入框")
		})
		a.importErrRow(c)
	}
	if a.importMethod != 4 && len(a.importParsed) > 0 {
		a.importParsedList(c)
	}
}

// importParsedList is what the 解析 found: a status line with the
// actions choosing all or none, and the skills as rows to choose.
func (a *app) importParsedList(c *ui.Context) {
	t := c.Theme()
	ui.Divider(c)
	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, checkCircleIcon).Size(14, 14).TextColor(t.Success)
		ui.Text(c, fmt.Sprintf("发现 %d 个技能 · 已选 %d 个", len(a.importParsed), a.importCount())).FontSize(13).FontWeight(500).SingleLine().Grow(1)
		linkAction(c, "全选", a.importCount() == len(a.importParsed), func() { a.selectAllParsed(true) })
		linkAction(c, "取消选择", a.importCount() == 0, func() { a.selectAllParsed(false) })
	})
	for i := range a.importParsed {
		a.parsedRow(c, i)
	}
}

// selectAllParsed chooses every skill the 解析 found, or none of them.
func (a *app) selectAllParsed(sel bool) {
	for i := range a.importParsed {
		a.importParsed[i].Checked = sel
	}
}

// linkAction is a small text action in the accent color, as the 全选 and
// 取消选择 of the parsed list; gray and still when it does not apply.
func linkAction(c *ui.Context, label string, disabled bool, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("link:"+label)).Padding(3, 6).Radius(5).Label(label).Disabled(disabled)
	if disabled {
		b.TextColor(t.TextMuted.Alpha(0.5))
	} else {
		b.TextColor(t.Accent)
		if b.Hovered() {
			b.Background(t.Accent.Alpha(0.12))
		}
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).SingleLine() })
	b.OnClick(onClick)
}

// parsedRow is one skill the 解析 found: a checkbox, its name with an
// 已存在 badge when the hub has it, and its description. A click
// anywhere on the row toggles the choice; importing a chosen one that
// exists overwrites it.
func (a *app) parsedRow(c *ui.Context, i int) {
	t := c.Theme()
	p := &a.importParsed[i]
	row := ui.ButtonBase(c.Key("parsed:"+p.Name)).AlignItems(ui.Center).Gap(12).Padding(10, 12).Radius(10).
		Border(1, t.Border).Background(t.Background).Justify(ui.Start).Label(p.Name)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Checkbox(c, &p.Checked, "").Label("选择 " + p.Name)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, p.Name).FontSize(13).Bold().SingleLine()
				if p.Exists {
					ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(1, 6).Radius(5).Shrink(0).Background(t.Warning.Alpha(0.15)).Children(func() {
						ui.Icon(c, alertIcon).Size(10, 10).TextColor(t.Warning)
						ui.Text(c, "已存在").FontSize(10).FontWeight(500).TextColor(t.Warning).SingleLine()
					})
				}
			})
			ui.Text(c, p.Desc).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
	row.OnClick(func() { p.Checked = !p.Checked })
}

// importField is a single-line input of the 导入技能 modal: borderless
// until focused, when a thick light-blue ring shows, as the hub's search
// capsule does. The ring is 2 wide in both states, so the focus never
// shifts the layout.
func importField(c *ui.Context, value *string, placeholder, label string) ui.Element {
	t := c.Theme()
	field := ui.Row(c).Height(32).AlignItems(ui.Center).Padding(0, 10).Radius(8).Background(t.Surface).Border(2, t.Surface).Cursor(ui.CursorText).Label(label)
	var input ui.Element
	field.Children(func() {
		input = ui.TextInputBase(c, value).Grow(1).Placeholder(placeholder).Label(label)
	})
	if input.Focused() {
		field.Border(2, t.Accent.Alpha(0.45))
	}
	field.OnClick(func() { input.Focus() })
	return field
}

// importTextArea is the modal's multi-line input, in importField's
// style.
func importTextArea(c *ui.Context, value *string, placeholder, label string) ui.Element {
	t := c.Theme()
	field := ui.Column(c).Grow(1).Padding(8, 10).Radius(8).Background(t.Surface).Border(2, t.Surface).Cursor(ui.CursorText).Label(label)
	var input ui.Element
	field.Children(func() {
		input = ui.TextAreaBase(c, value).Grow(1).Placeholder(placeholder).Label(label).Font("monospace").FontSize(12)
	})
	if input.Focused() {
		field.Border(2, t.Accent.Alpha(0.45))
	}
	field.OnClick(func() { input.Focus() })
	return field
}
