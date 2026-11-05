package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// What an entry of the agent's skills directory is to the hub.
const (
	agentSkillReal = iota // a real directory: importable
	agentSkillHub         // a link to the hub's skill: removable
	agentSkillLink        // a link to elsewhere: importable
	agentSkillOwn         // the hub's own directory, the agent's being the hub's
)

// agentSkill is a skill found in an agent's skills directory: what its
// SKILL.md says, what the entry is to the hub, and the path the card
// shows — the link's target for one of the hub's, the entry itself for
// the rest.
type agentSkill struct {
	skill
	kind int
	path string
}

// scanAgentSkills lists the skills of an agent's skills directory:
// every directory with a SKILL.md is one, a linked one too, as os.Stat
// follows it. A link into the hub is agentSkillHub, a link to elsewhere
// agentSkillLink, a real directory agentSkillReal — but where the
// agent's directory IS the hub's, its real directories are the hub's
// own, agentSkillOwn, which the page leaves alone. Broken links and
// entries without a SKILL.md are no skills and are skipped.
func scanAgentSkills(agentDir, skillsDir string) []agentSkill {
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return nil
	}
	var out []agentSkill
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(agentDir, name)
		st, err := os.Stat(full)
		if err != nil || !st.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(full, "SKILL.md"))
		if err != nil {
			continue
		}
		as := agentSkill{skill: parseSkill(data, name), path: full}
		as.Dir = name
		if fi, lerr := os.Lstat(full); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			as.kind = agentSkillLink
			if dst, rerr := os.Readlink(full); rerr == nil {
				if !filepath.IsAbs(dst) {
					dst = filepath.Join(agentDir, dst)
				}
				if samePath(filepath.Dir(dst), skillsDir) {
					as.kind = agentSkillHub
					as.path = dst
				}
			}
		} else if lerr == nil && samePath(agentDir, skillsDir) {
			as.kind = agentSkillOwn
		}
		out = append(out, as)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// importAgentSkill moves a skill of an agent into the hub: the content
// is copied into the hub's skills directory — over what the hub has of
// the name, as every import does — the agent's entry, a symbolic link
// or a real directory, is removed, and a link to the hub's skill takes
// its place. A link's target outside the agent's directory stays as it
// is; only the link goes.
func importAgentSkill(agentDir, skillsDir, name string) error {
	src := filepath.Join(agentDir, name)
	if samePath(agentDir, skillsDir) {
		return fmt.Errorf("「%s」已在技能中心", name)
	}
	// The entry linking to the hub's skill — the state right after an
	// import of it, as a second click on a stale view meets it — is
	// imported already; importing would delete the hub's copy before
	// reading it.
	if skillLink(agentDir, filepath.Join(skillsDir, name)) == linkOurs {
		return fmt.Errorf("「%s」已链接到技能中心，无需导入", name)
	}
	meta := importMeta{Source: "local", URL: src, ImportedAt: time.Now()}
	if err := importSkill(src, skillsDir, name, meta); err != nil {
		return err
	}
	// os.RemoveAll takes a symbolic link itself, not what it points at.
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("移除原目录: %w", err)
	}
	if err := os.Symlink(filepath.Join(skillsDir, name), src); err != nil {
		return fmt.Errorf("创建链接: %w", err)
	}
	return nil
}

// agentSkillsPane is the pane of the Agent 技能 menu: the skills of one
// agent as cards — a link to the hub removable, a real directory or a
// link to elsewhere importable into the hub. Everything derives from
// the file system as the view builds, so a change shows at once.
func (a *app) agentSkillsPane(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.paneHeader(c, "Agent 技能", nil)
		ui.Divider(c)
		if len(a.agents) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Image(c, agentIcon).Size(40, 40)
				ui.Text(c, "还没有添加任何 agent").TextColor(t.TextMuted)
				ui.Text(c, "请先在「设置」中添加").FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		// The agent the page shows: the choice, or the first of the list.
		cur := 0
		for i, ag := range a.agents {
			if ag.Name == a.agentView {
				cur = i
				break
			}
		}
		ag := a.agents[cur]
		skills := scanAgentSkills(expandHome(ag.Dir), a.skillsDir)
		query := strings.ToLower(strings.TrimSpace(a.agentSkillSearch))
		var shown []agentSkill
		for _, s := range skills {
			if query == "" ||
				strings.Contains(strings.ToLower(s.Name), query) ||
				strings.Contains(strings.ToLower(s.Desc), query) {
				shown = append(shown, s)
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 28, 24, 28).Gap(14).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Text(c, "查看某个 agent 已拥有的技能，分配或移除 · 当前：").FontSize(12).TextColor(t.TextMuted).SingleLine()
					ui.Text(c, ag.Name).FontSize(12).FontWeight(500).SingleLine()
				})
				focus := a.searchFocus
				a.searchFocus = false
				capsuleSearch(c, &a.agentSkillSearch, "在该 agent 中搜索技能…", "在该 agent 中搜索技能", focus)
				// Every agent as a capsule, wrapping to as many rows as
				// it takes.
				ui.Row(c).Wrap().Gap(chipGap).Children(func() {
					for _, x := range a.agents {
						a.agentPickChip(c, x, x.Name == ag.Name)
					}
				})
				a.agentSkillHint(c)
				if len(shown) == 0 {
					ui.Column(c).Height(160).Center().Gap(8).Children(func() {
						if len(skills) == 0 {
							ui.Text(c, "该 agent 还没有技能").TextColor(t.TextMuted)
						} else {
							ui.Text(c, "没有匹配的技能").TextColor(t.TextMuted)
							ui.Text(c, "换个关键词试试").FontSize(12).TextColor(t.TextMuted)
						}
					})
					return
				}
				w, _ := c.Size()
				ui.Grid(c).Columns(gridColumns(w, a.collapsed)).Gap(14).Children(func() {
					for _, s := range shown {
						a.agentSkillCard(c, ag, s)
					}
				})
			})
		})
	})
}

// agentSkillHint is the pane's note on what the cards' buttons do.
func (a *app) agentSkillHint(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "来源标签：").FontSize(12).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, "技能中心").FontSize(12).TextColor(t.Accent).SingleLine()
		ui.Text(c, " 软链接可移除；").FontSize(12).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, "真实目录").FontSize(12).TextColor(t.Success).SingleLine()
		ui.Text(c, " 与 ").FontSize(12).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, "外部软链接").FontSize(12).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, " 受保护、不会被删除，可导入到技能中心统一管理。").FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

// agentPickChip is one agent of the pane's selector: a capsule picking
// the agent whose skills the page shows. The choice is filled blue with
// white text; the rest carry a gray ring on white, and the pointer
// deepens both. The ring is 1 wide in every state, so a pick never
// shifts the layout.
func (a *app) agentPickChip(c *ui.Context, ag agent, on bool) {
	t := c.Theme()
	ch := ui.ButtonBase(c.Key("agpick:"+ag.Name)).Height(28).AlignItems(ui.Center).Padding(0, 14).Radius(14).Shrink(0).Label("查看 " + ag.Name + " 的技能")
	if on {
		ch.Border(1, t.Accent).Background(t.Accent).TextColor(t.AccentText)
		if ch.Hovered() {
			ch.Background(t.Accent.Alpha(0.85))
		}
	} else {
		ch.Border(1, t.Border).Background(t.Background).TextColor(t.Text)
		if ch.Hovered() {
			ch.Background(t.SurfaceHover)
		}
	}
	ch.Children(func() {
		ui.Text(c, ag.Name).FontSize(12).FontWeight(500).SingleLine()
	})
	ch.OnClick(func() { a.agentView = ag.Name })
}

// agentSkillCard is one skill of the agent: its name, what it does in
// at most two lines, the path it lives at, its source tag, and what can
// be done with it — 移除 for a link to the hub, 导入 for a real
// directory or a link to elsewhere. The card has no border, as every
// card of the app.
func (a *app) agentSkillCard(c *ui.Context, ag agent, s agentSkill) {
	t := c.Theme()
	desc := s.Desc
	if desc == "" {
		desc = "暂无简介"
	}
	ui.Column(c).Gap(8).Padding(14, 16).Radius(12).Background(t.Surface).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, s.Name).Bold().SingleLine()
			// The directory's name shows where it differs from the
			// skill's, telling two skills of a name apart.
			if s.Dir != "" && s.Dir != s.Name {
				ui.Text(c, "("+s.Dir+")").FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
			a.agentSkillTag(c, s.kind)
		})
		ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).FixedLineHeight(cardDescLine).MaxLines(2).Height(2 * cardDescLine)
		path := displayHome(s.path)
		if s.kind == agentSkillHub {
			path = "→ " + path
		}
		ui.Text(c, path).Font("monospace").FontSize(11).TextColor(t.TextMuted).SingleLine()
		ui.Row(c).Children(func() {
			ui.Box(c).Grow(1)
			switch s.kind {
			case agentSkillHub:
				a.agentSkillRemove(c, ag, s)
			case agentSkillReal, agentSkillLink:
				a.agentSkillImport(c, ag, s)
			}
		})
	})
}

// agentSkillTag is the card's tag of what the entry is to the hub:
// 技能中心 for a link into it — or the hub's own directory — 真实目录
// for a real directory and 外部软链接 for a link to elsewhere, the two
// importable.
func (a *app) agentSkillTag(c *ui.Context, kind int) {
	t := c.Theme()
	text, bg, fg := "外部软链接", t.SurfaceHover, t.TextMuted
	switch kind {
	case agentSkillHub, agentSkillOwn:
		text, bg, fg = "技能中心", t.Accent.Alpha(0.12), t.Accent
	case agentSkillReal:
		text, bg, fg = "真实目录", t.Success.Alpha(0.12), t.Success
	}
	ui.Row(c).Padding(2, 8).Radius(5).Shrink(0).Background(bg).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(500).TextColor(fg).SingleLine()
	})
}

// agentSkillRemove is the card's 移除 button: it deletes the agent's
// link to the hub's skill — the skill itself stays in the hub.
func (a *app) agentSkillRemove(c *ui.Context, ag agent, s agentSkill) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("agsrm:"+ag.Name+"/"+s.Dir)).Height(26).AlignItems(ui.Center).Gap(5).Padding(0, 10).Radius(6).
		Border(1, t.Border).Background(t.Background).Label("移除 " + s.Name)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, unlinkIcon).Size(12, 12).TextColor(t.TextMuted)
		ui.Text(c, "移除").FontSize(12).SingleLine()
	})
	if b.Clicked() {
		if err := os.Remove(filepath.Join(expandHome(ag.Dir), s.Dir)); err != nil {
			c.Toast("无法移除「" + s.Name + "」：" + err.Error())
		}
	}
}

// agentSkillImport is the card's 导入 button: it moves the skill into
// the hub — copied there, the entry removed, a link to the hub left in
// its place.
func (a *app) agentSkillImport(c *ui.Context, ag agent, s agentSkill) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("agsim:"+ag.Name+"/"+s.Dir)).Height(26).AlignItems(ui.Center).Gap(5).Padding(0, 10).Radius(6).
		Background(t.Accent).TextColor(t.AccentText).Label("导入 " + s.Name + " 到技能中心")
	if b.Hovered() {
		b.Background(t.Accent.Alpha(0.85))
	}
	b.Children(func() {
		ui.Icon(c, downloadIcon).Size(12, 12)
		ui.Text(c, "导入").FontSize(12).SingleLine()
	})
	if b.Clicked() {
		if err := importAgentSkill(expandHome(ag.Dir), a.skillsDir, s.Dir); err != nil {
			c.Toast("无法导入「" + s.Name + "」：" + err.Error())
		} else {
			c.Toast("已导入「" + s.Name + "」到技能中心")
			a.loadSkills()
		}
	}
}
