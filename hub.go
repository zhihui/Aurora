package main

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// skill is a skill of the hub: its name, what it does and where it came
// from, read from a directory of the skills directory — the name from
// the SKILL.md inside, the directory's where it is missing. Dir is the
// directory's name: the skill's identity on disk, which every file
// operation goes by — two directories can declare the same name.
type skill struct {
	Name    string
	Desc    string
	Source  string // the card's source tag, as "github.com", 本地文件, 本地链接 or 未知来源
	Dir     string // the directory's name in the skills directory
	Meta    importMeta
	HasMeta bool      // the skill has an import record, in Meta
	Updated time.Time // the import's or the last update's time
}

// loadSkills reads the skills of the hub from the skills directory:
// every directory with a SKILL.md is one — a linked one (the 本地 way's
// 链接导入) too, as os.Stat follows it; its source tag is 本地链接 where
// no import record says otherwise. Only the head of each SKILL.md is
// read: the front matter it parses is at the file's start.
func (a *app) loadSkills() {
	a.skills = nil
	if a.skillsDir == "" {
		return
	}
	entries, err := os.ReadDir(a.skillsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(a.skillsDir, e.Name())
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			continue
		}
		data, err := readHead(filepath.Join(dir, "SKILL.md"), skillHeadLen)
		if err != nil {
			continue
		}
		sk := parseSkill(data, e.Name())
		sk.Dir = e.Name()
		meta, ok := loadImportMeta(dir)
		sk.Source = sourceLabel(meta, ok)
		sk.Meta, sk.HasMeta = meta, ok
		if ok {
			sk.Updated = meta.ImportedAt
		}
		if !ok {
			if le, lerr := os.Lstat(dir); lerr == nil && le.Mode()&os.ModeSymlink != 0 {
				sk.Source = "本地链接"
			}
		}
		a.skills = append(a.skills, sk)
	}
	sort.Slice(a.skills, func(i, j int) bool { return a.skills[i].Name < a.skills[j].Name })
}

// skillHeadLen is how much of a SKILL.md the hub reads: the front
// matter that names and describes the skill is at the file's start, so
// 16 KiB covers it with room to spare. A SKILL.md whose front matter
// outgrows it falls back to the directory's name.
const skillHeadLen = 16 * 1024

// readHead reads at most n bytes of the named file.
func readHead(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, n))
}

// parseSkill reads a SKILL.md: the name and description of its front
// matter, the fallback name where it says none. A description in a YAML
// block scalar — ">" folding its lines, "|" keeping them — is read over
// the indented lines that follow.
func parseSkill(data []byte, fallback string) skill {
	sk := skill{Name: fallback}
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return sk
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return sk
	}
	lines := strings.Split(text[3:3+end], "\n")
	for i := 0; i < len(lines); i++ {
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			if value = strings.Trim(value, `"'`); value != "" {
				sk.Name = value
			}
		case "description":
			if value != ">" && value != ">-" && value != ">+" &&
				value != "|" && value != "|-" && value != "|+" {
				sk.Desc = strings.Trim(value, `"'`)
				continue
			}
			sep := "\n"
			if value[0] == '>' {
				sep = " "
			}
			var block []string
			for i+1 < len(lines) {
				next := lines[i+1]
				if next == "" || strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") {
					i++
					if s := strings.TrimSpace(next); s != "" {
						block = append(block, s)
					}
					continue
				}
				break
			}
			sk.Desc = strings.Join(block, sep)
		}
	}
	return sk
}

// matchSkill reports whether the search text shows the skill: it matches
// the name, the description and the agents, ignoring case.
func matchSkill(sk skill, query string, agents []agent) bool {
	query = strings.ToLower(query)
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(sk.Name), query) ||
		strings.Contains(strings.ToLower(sk.Desc), query) {
		return true
	}
	for _, ag := range agents {
		if strings.Contains(strings.ToLower(ag.Name), query) {
			return true
		}
	}
	return false
}

// hub is the pane of the 技能中心 menu: a search field, the 导入技能
// button, and the skills as cards, two to a row.
func (a *app) hub(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.paneHeader(c, "技能中心", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				focus := a.searchFocus
				a.searchFocus = false
				capsuleSearch(c, &a.skillSearch, "搜索技能…", "搜索技能", focus).Width(200)
				a.sortSwitch(c)
				a.updateButton(c)
				ui.PrimaryButton(c, "导入技能…").OnClick(func() { a.importOpen = true })
			})
		})
		ui.Divider(c)
		query := strings.TrimSpace(a.skillSearch)
		var shown []skill
		for _, sk := range a.skills {
			if matchSkill(sk, query, a.agents) {
				shown = append(shown, sk)
			}
		}
		if a.skillSort == 1 {
			// Newest first; a skill without a record sorts last, its name
			// breaking the ties.
			sort.SliceStable(shown, func(i, j int) bool {
				if !shown[i].Updated.Equal(shown[j].Updated) {
					return shown[i].Updated.After(shown[j].Updated)
				}
				return shown[i].Name < shown[j].Name
			})
		}
		if len(shown) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				if len(a.skills) == 0 {
					ui.Text(c, "还没有技能").TextColor(t.TextMuted)
					ui.Text(c, "点击右上角「导入技能」开始").FontSize(12).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "没有匹配的技能").TextColor(t.TextMuted)
					ui.Text(c, "换个关键词试试").FontSize(12).TextColor(t.TextMuted)
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 28, 24, 28).Gap(14).Children(func() {
				ui.Text(c, "点击技能下方 agent 徽标即可分配／取消分配（创建或移除软链接）").FontSize(12).TextColor(t.TextMuted)
				w, _ := c.Size()
				ui.Grid(c).Columns(gridColumns(w, a.collapsed)).Gap(14).Children(func() {
					for _, sk := range shown {
						a.skillCard(c, sk)
					}
				})
			})
		})
	})
}

// sortSwitch is the header's order switch, 名称 or 时间: a small
// segmented control, the choice raised in white over the surface track,
// as the settings' segment is.
func (a *app) sortSwitch(c *ui.Context) {
	t := c.Theme()
	parts := ui.SegmentedBase(c, &a.skillSort, 2)
	track := parts.Track
	track.AlignItems(ui.Center).Padding(2).Gap(2).Radius(8).Background(t.Surface).Label("排序方式")
	track.Children(func() {
		for i, label := range []string{"名称", "时间"} {
			s := parts.Segment(i).Padding(4, 10).Radius(6).Center()
			if i == a.skillSort {
				s.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			} else {
				s.TextColor(t.TextMuted)
			}
			s.Children(func() { ui.Text(c, label).FontSize(12).SingleLine() })
		}
	})
}

// updateButton is the header's 更新技能: still while a check or an
// update runs, and while the hub has no git skill to check.
func (a *app) updateButton(c *ui.Context) {
	label := "更新技能"
	if a.updateChecking {
		label = "检查中…"
	} else if a.updating {
		label = "更新中…"
	}
	ui.Button(c, label).Disabled(a.updateChecking || a.updating || len(a.gitSkills()) == 0).
		OnClick(func() { a.checkUpdates() })
}

// capsuleSearch is a search field as a capsule, in a pane's header and
// in the 添加技能 modal alike: its ring is the background's color until
// the input takes the focus, which turns it a thick light blue. The
// ring is 2 wide in both states, so the focus never shifts the layout.
// A click anywhere on the capsule focuses the input; once it holds
// text, a clear button shows at its trailing edge, clearing the text
// and keeping the focus (search-fields.md: a search field displays a
// Search icon, a Clear button, and placeholder text). focus asks the
// input to take the focus as it builds, for the menu's 搜索…. The
// caller sets the width; unset, the capsule stretches with its column.
func capsuleSearch(c *ui.Context, value *string, placeholder, label string, focus bool) ui.Element {
	t := c.Theme()
	field := ui.Row(c).Height(28).AlignItems(ui.Center).Gap(6).Padding(0, 6, 0, 12).Radius(14).Background(t.Surface).Border(2, t.Surface).Cursor(ui.CursorText).Label(label)
	var input ui.Element
	field.Children(func() {
		ui.Icon(c, searchIcon).Size(13, 13).TextColor(t.TextMuted)
		input = ui.TextInputBase(c, value).Grow(1).Placeholder(placeholder).Label(label)
		if *value != "" {
			x := ui.ButtonBase(c.Key("clear:"+label)).Size(16, 16).Radius(4).Center().TextColor(t.TextMuted).Label("清除搜索")
			if x.Hovered() {
				x.Background(t.SurfaceHover).TextColor(t.Text)
			}
			x.Children(func() { ui.Icon(c, xIcon).Size(9, 9) })
			if x.Clicked() {
				*value = ""
				input.Focus()
			}
		}
	})
	if input.Focused() {
		field.Border(2, t.Accent.Alpha(0.45))
	}
	if focus {
		input.Focus()
	}
	field.OnClick(func() { input.Focus() })
	return field
}

// The metrics of the skills' cards: every card reserves two lines for
// the description, so cards of a short description are as high as the
// others and the grid stays even; the agent tags take the height they
// need.
const (
	cardDescLine = 17 // one line of the description, 12pt
	chipHeight   = 24 // one agent tag
	chipGap      = 6  // between the tags
)

// skillCard is one skill of the hub's grid: its name, what it does in at
// most two lines, and every agent as a capsule tag that links the skill
// to the agent. An × rests at its top right — dimmed until the pointer
// is near, so keyboard users and newcomers can find it too — asking
// before it deletes the skill. The card itself is a button opening the
// viewer; the chips and the × are the innermost controls, so their
// presses never open it.
func (a *app) skillCard(c *ui.Context, sk skill) {
	t := c.Theme()
	desc := sk.Desc
	if desc == "" {
		desc = "暂无简介"
	}
	card := ui.ButtonBase(c.Key("view:" + sk.Dir)).Radius(12).Background(t.Surface).Justify(ui.Start).Label("查看 " + sk.Name)
	if card.Hovered() {
		card.Background(t.SurfaceHover)
	}
	card.OnClick(func() { a.openSkillView(sk) })
	card.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Padding(14, 16).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, sk.Name).Bold().SingleLine()
				// The directory's name shows where it differs from the
				// skill's, telling two skills of a name apart.
				if sk.Dir != "" && sk.Dir != sk.Name {
					ui.Text(c, "("+sk.Dir+")").FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
			ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).FixedLineHeight(cardDescLine).MaxLines(2).Height(2 * cardDescLine)
			ui.Row(c).Wrap().Gap(chipGap).Children(func() {
				for _, ag := range a.agents {
					a.chip(c, sk, ag)
				}
			})
			// The card's bottom: where the skill came from, 有更新 when the
			// check found one, and when it was imported or last updated.
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				tag(c, sk.Source)
				if a.updateAvail[sk.Dir] {
					updateTag(c)
				}
				ui.Box(c).Grow(1)
				if !sk.Updated.IsZero() {
					ui.Text(c, "更新于 "+sk.Updated.Format("2006-01-02")).FontSize(10).TextColor(t.TextMuted).SingleLine()
				}
			})
			x := ui.ButtonBase(c.Key("del:"+sk.Dir)).Absolute().Top(10).Right(12).Size(20, 20).Radius(5).Center().TextColor(t.TextMuted.Alpha(0.55)).Label("删除 " + sk.Name)
			if x.Hovered() {
				x.Background(t.SurfaceHover).TextColor(t.Text)
			}
			x.Children(func() { ui.Icon(c, xIcon).Size(11, 11) })
			x.OnClick(func() { a.delName, a.delDir, a.delOpen = sk.Name, sk.Dir, true })
		})
	})
}

// updateTag is the card's mark of an update the check found waiting:
// accent-tinted, as the built-in agent's 默认 badge is.
func updateTag(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(2, 8).Radius(5).Shrink(0).Background(t.Accent.Alpha(0.12)).Children(func() {
		ui.Icon(c, downloadIcon).Size(10, 10).TextColor(t.Accent)
		ui.Text(c, "有更新").FontSize(10).FontWeight(500).TextColor(t.Accent).SingleLine()
	})
}

// deleteSkillModal asks before the × of a card deletes the skill: the
// links of every agent go first, then the skill's directory.
func (a *app) deleteSkillModal(c *ui.Context) {
	if ui.AlertDialog(c, &a.delOpen, "删除技能",
		"将删除「"+a.delName+"」以及所有 agent 中指向它的链接，此操作无法撤销。",
		"取消", "删除") == 1 {
		if err := removeSkill(a.agents, a.skillsDir, a.delDir); err != nil {
			c.Toast("无法删除「" + a.delName + "」：" + err.Error())
		} else {
			// The packs keep no reference to a skill that is gone.
			a.dropSkill(a.delDir)
			a.loadSkills()
		}
	}
}

// chip is a capsule tag with an agent's name, in the skills' cards: the
// switch between the skill and the agent, fully rounded and big enough
// to click. Chosen — the agent's skills directory links to the skill —
// its ring is blue on light blue with dark text; else it is a gray ring
// on white with gray text, and the pointer deepens both. The ring is 1
// wide in every state, so a toggle never shifts the layout. The glyph at
// its leading edge marks the state in more than color (toggles.md:
// avoid relying on color alone): a check when chosen, an empty circle
// when not — same size in both states, so a toggle never shifts the
// layout either, and neither state is a blank space.
func (a *app) chip(c *ui.Context, sk skill, ag agent) {
	t := c.Theme()
	target := filepath.Join(a.skillsDir, sk.Dir)
	on := skillLink(expandHome(ag.Dir), target) == linkOurs
	ch := ui.ButtonBase(c.Key("chip:"+sk.Dir+"/"+ag.Name)).Height(chipHeight).AlignItems(ui.Center).Gap(4).Padding(0, 10).Radius(chipHeight / 2).Shrink(0).Label(ag.Name)
	if on {
		ch.Border(1, t.Accent).Background(t.Accent.Alpha(0.12)).TextColor(t.Text)
		if ch.Hovered() {
			ch.Background(t.Accent.Alpha(0.2))
		}
	} else {
		ch.Border(1, t.Border).Background(t.Background).TextColor(t.TextMuted)
		if ch.Hovered() {
			ch.Background(t.SurfaceHover).TextColor(t.Text)
		}
	}
	glyph := circleIcon
	if on {
		glyph = checkIcon
	}
	ch.Children(func() {
		ui.Icon(c, glyph).Size(10, 10)
		ui.Text(c, ag.Name).FontSize(11).FontWeight(500).SingleLine()
	})
	if ch.Clicked() {
		msg, err := toggleSkillLink(expandHome(ag.Dir), target, ag.Name)
		switch {
		case err != nil:
			c.Toast("无法更新「" + sk.Name + "」：" + err.Error())
		case msg != "":
			c.Toast(msg)
		}
	}
}
