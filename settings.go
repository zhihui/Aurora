package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// agent is an agent the user added: its name and the directory of its
// skills, "~" for the home directory as the user writes it.
type agent struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

// defaultAgentName is the name of the built-in agent: always in the
// list, never deletable.
const defaultAgentName = "通用 Agent"

// defaultAgent is the built-in agent. Its directory is .agents/skills
// in the user's home, stored the way the platform's users write it, as
// every agent's directory is.
func defaultAgent() agent {
	home, err := os.UserHomeDir()
	if err != nil {
		return agent{defaultAgentName, "~/.agents/skills"}
	}
	return agent{defaultAgentName, displayHome(filepath.Join(home, ".agents", "skills"))}
}

// ensureDefaultAgent adds the built-in agent at the head of the list
// where it is missing, and restores its canonical directory where the
// saved one drifted — an update moved it, for example. It runs once at
// startup.
func (a *app) ensureDefaultAgent() {
	def := defaultAgent()
	for i, ag := range a.agents {
		if ag.Name == defaultAgentName {
			if ag.Dir != def.Dir {
				a.agents[i].Dir = def.Dir
				a.saveAgents()
			}
			return
		}
	}
	a.agents = append([]agent{def}, a.agents...)
	a.saveAgents()
}

// presetAgents are the agents the modal's 预设 tab offers.
var presetAgents = []agent{
	{"Claude", "~/.claude/skills"},
	{"Codex", "~/.codex/skills"},
	{"Deepseek Harness", "~/.dsh/skills"},
	{"Kimi Code", "~/.kimi-code/skills"},
	{"Kimi", "~/.kimi/skills"},
	{"Work Buddy", "~/.workbuddy/skills"},
	{"Pi Agent", "~/.pi/agent/skills"},
	{"Qoder CN", "~/.qoder-cn/skills"},
}

// avatarColors are the colors of the avatars, picked by the name so one
// agent's avatar is the same everywhere it shows.
var avatarColors = []string{"#0A84FF", "#BF5AF2", "#5E5CE6", "#FF375F", "#FF9F0A", "#30D158", "#64D2FF"}

// abbrev is the avatar's text: the name's first two letters, the first
// upper, or the first character of a name not in letters.
func abbrev(name string) string {
	r1, n := utf8.DecodeRuneInString(name)
	if r1 == utf8.RuneError || r1 > unicode.MaxASCII {
		return string(r1)
	}
	r2, _ := utf8.DecodeRuneInString(name[n:])
	if r2 == utf8.RuneError || r2 > unicode.MaxASCII || r2 == ' ' {
		return string(unicode.ToUpper(r1))
	}
	return string(unicode.ToUpper(r1)) + string(unicode.ToLower(r2))
}

// agentColor is the color of an agent's mark, picked by the name so one
// agent's mark is the same everywhere it shows.
func agentColor(name string) ui.Color {
	h := 0
	for _, b := range []byte(name) {
		h = h*31 + int(b)
	}
	if h < 0 {
		h = -h
	}
	return ui.Hex(avatarColors[h%len(avatarColors)])
}

// avatar is a rounded square in the name's color, with the name's first
// two letters: the mark of an agent in the preset list and the added
// list alike.
func avatar(c *ui.Context, name string, size float32) {
	ui.Row(c).Size(size, size).Radius(size / 4).Shrink(0).Background(agentColor(name)).Center().Children(func() {
		ui.Text(c, abbrev(name)).FontSize(size * 0.38).Bold().TextColor(ui.RGB(255, 255, 255))
	})
}

// tag is a small muted label capsule, as the 默认 of the built-in
// agent and the 已添加 of the presets.
func tag(c *ui.Context, text string) {
	t := c.Theme()
	ui.Row(c).Padding(2, 8).Radius(5).Shrink(0).Background(t.SurfaceHover).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(500).TextColor(t.TextMuted).SingleLine()
	})
}

func (a *app) hasAgent(ag agent) bool {
	return slices.ContainsFunc(a.agents, func(x agent) bool {
		return x.Name == ag.Name || x.Dir == ag.Dir
	})
}

func (a *app) addAgent(ag agent) {
	if a.hasAgent(ag) {
		return
	}
	a.agents = append(a.agents, ag)
	a.saveAgents()
	cleanBrokenLinks(expandHome(ag.Dir))
}

// cleanBrokenLinks removes the broken symbolic links in dir, the
// leftovers of skills that moved or went away. A directory that is not
// there is left alone: adding an agent never creates it.
func cleanBrokenLinks(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		// os.Stat follows the link; it fails with IsNotExist exactly
		// when the link's target is gone.
		if _, err := os.Stat(filepath.Join(dir, e.Name())); os.IsNotExist(err) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// loadAgents reads the agents the user added, from agentsPath; a save
// follows every change. The path is "" in tests, which stay in memory.
func (a *app) loadAgents() {
	if a.agentsPath == "" {
		return
	}
	data, err := os.ReadFile(a.agentsPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.agents)
}

func (a *app) saveAgents() {
	if a.agentsPath == "" {
		return
	}
	data, err := json.MarshalIndent(a.agents, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.agentsPath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(a.agentsPath, data, 0o644)
}

// deleteAgent takes the agent of the name off the list, saving after.
// Deleting an agent only drops it from the configuration; its directory
// stays as it is. The built-in agent is never deletable.
func (a *app) deleteAgent(name string) {
	if name == defaultAgentName {
		return
	}
	for i, ag := range a.agents {
		if ag.Name == name {
			a.agents = slices.Delete(a.agents, i, i+1)
			a.saveAgents()
			return
		}
	}
}

// settings is the pane of the 设置 menu: the agents the user added, and
// the button opening the modal that adds one.
func (a *app) settings(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.paneHeader(c, "设置", func() {
			ui.PrimaryButton(c, "添加 Agent…").OnClick(func() { a.addOpen, a.presetSel = true, "" })
		})
		ui.Divider(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(20, 28, 24, 28).Gap(24).MaxWidth(640).Children(func() {
				a.agentsSection(c)
				a.llmSection(c)
			})
		})
	})
}

// agentsSection is the settings' Agents block: its header, the grouped
// list of the agents, or the empty state when there are none.
func (a *app) agentsSection(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "Agents").FontSize(13).Bold()
			if len(a.agents) > 0 {
				ui.Badge(c, fmt.Sprint(len(a.agents)))
			}
		})
		ui.Text(c, "添加需要管理技能的 agent，以及它们的技能配置目录。").FontSize(12).TextColor(t.TextMuted)
		if len(a.agents) == 0 {
			ui.Column(c).Radius(10).Border(1, t.Border).Padding(24, 16).Center().Gap(8).Children(func() {
				ui.Image(c, agentIcon).Size(28, 28)
				ui.Text(c, "还没有添加任何 agent").FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "点击右上角「添加 Agent」开始").FontSize(11).TextColor(t.TextMuted)
			})
			return
		}
		a.agentList(c)
	})
}

// agentList is the settings' agents as one grouped list: a rounded
// container holding every row, hairlines between them inset to the
// name's leading edge — the way macOS groups like rows
// (lists-and-tables.md › Style) — and a footer noting the configuration
// directory and what deleting an agent means.
func (a *app) agentList(c *ui.Context) {
	t := c.Theme()
	list := ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Clip()
	list.Children(func() {
		for i, ag := range a.agents {
			if i > 0 {
				ui.Row(c).Children(func() {
					ui.Box(c).Width(62).Shrink(0)
					ui.Box(c).Grow(1).Height(1).Background(t.Border)
				})
			}
			a.agentRow(c, ag)
		}
	})
	if a.agentsPath != "" {
		ui.Text(c, "配置目录："+displayHome(filepath.Dir(a.agentsPath))+" · 删除 agent 只移除配置，不动目录。右键任意行可打开操作菜单。").FontSize(11).TextColor(t.TextMuted).FixedLineHeight(16)
	}
}

// agentRow is one agent of the grouped list: its avatar, its name with
// the built-in's 默认 badge, its skills directory, and how many skills
// live there now. The ⋯ at the trailing edge — dimmed until the pointer
// is near, so keyboard users can find it too — opens the row's menu, as
// a right-click anywhere on the row does. The row is not a button: the
// Agent 技能 pane's capsules pick the agent to view.
func (a *app) agentRow(c *ui.Context, ag agent) {
	t := c.Theme()
	count := len(scanAgentSkills(expandHome(ag.Dir), a.skillsDir))
	row := ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(9, 14)
	hovered := row.Hovered()
	if hovered {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		avatar(c, ag.Name, 36)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, ag.Name).FontWeight(600).SingleLine()
				// The built-in agent is never deletable.
				if ag.Name == defaultAgentName {
					defaultTag(c)
				}
			})
			ui.Text(c, ag.Dir).Font("monospace").FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, fmt.Sprintf("%d 个技能", count)).FontSize(12).TextColor(t.TextMuted).SingleLine()
		more := ui.ButtonBase(c.Key("agentmore:"+ag.Name)).Size(24, 24).Radius(5).Center().TextColor(t.TextMuted.Alpha(0.55)).Label(ag.Name + " 的操作")
		if hovered {
			more.TextColor(t.TextMuted)
		}
		if more.Hovered() {
			more.Background(t.Border).TextColor(t.Text)
		}
		more.Children(func() { ui.Icon(c, ellipsisIcon).Size(14, 14) })
		more.PopoverMenuButton(func(m *ui.Menu) { a.agentMenu(c, m, ag) })
	})
	row.ContextMenu(func(m *ui.Menu) { a.agentMenu(c, m, ag) })
}

// defaultTag is the badge of the built-in agent: a check and 默认 in
// the accent's tint.
func defaultTag(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(3, 8).Radius(6).Shrink(0).Background(t.Accent.Alpha(0.12)).Children(func() {
		ui.Icon(c, checkIcon).Size(10, 10).TextColor(t.Accent)
		ui.Text(c, "默认").FontSize(10).FontWeight(500).TextColor(t.Accent).SingleLine()
	})
}

// agentMenu is the agent row's menu, from the ⋯ button and from a
// right-click alike: 打开 shows the agent's skills directory in the
// system's file manager; 删除 — never for the built-in agent — takes
// the agent off the list. A menu item's choice runs outside the view's
// build, so it changes the app's state directly, as OnClick does.
func (a *app) agentMenu(c *ui.Context, m *ui.Menu, ag agent) {
	if m.Item("打开").Chosen() {
		if err := openDir(expandHome(ag.Dir)); err != nil {
			c.Toast("无法打开「" + ag.Dir + "」：" + err.Error())
		}
	}
	if ag.Name != defaultAgentName {
		m.Separator()
		if m.Item("删除").Chosen() {
			a.deleteAgent(ag.Name)
		}
	}
}

// modalClose is the small × at a modal's top right; close is what
// closing does, beyond hiding the modal.
func modalClose(c *ui.Context, close func()) {
	t := c.Theme()
	x := ui.ButtonBase(c.Key("close")).Size(24, 24).Radius(6).Center().TextColor(t.TextMuted).Label("关闭")
	if x.Hovered() {
		x.Background(t.SurfaceHover)
	}
	x.Children(func() { ui.Icon(c, xIcon).Size(12, 12) })
	x.OnClick(close)
}

// addAgentModal is the dialog adding an agent: the 预设 tab offers the
// known agents, the 自定义 tab takes a name and a directory.
func (a *app) addAgentModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.addOpen, func() {
		// ui.Modal's panel pads its content by t.Space(5); the negative
		// margin cancels it, so the sections' own padding rules and the
		// dividers span the modal edge to edge, as in the 导入技能 modal.
		ui.Column(c).Width(492).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "添加 Agent").FontSize(15).Bold().Grow(1)
					modalClose(c, func() { a.addOpen = false })
				})
				ui.Text(c, "选择一个预设 agent，或自定义名称与配置目录。").FontSize(12).TextColor(t.TextMuted)
			})
			ui.Divider(c)
			ui.Column(c).Padding(14, 16).Gap(14).Children(func() {
				a.segment(c)
				if a.addTab == 0 {
					a.presetList(c)
				} else {
					a.customForm(c)
				}
			})
			ui.Divider(c)
			a.addAgentFooter(c)
		})
	})
}

// segment is the modal's switch between 预设 and 自定义: a track of the
// theme's surface splitting in halves, the choice raised in white. Its
// margins are the list's padding, so it is exactly as wide as the cards.
func (a *app) segment(c *ui.Context) {
	t := c.Theme()
	parts := ui.SegmentedBase(c, &a.addTab, 2)
	track := parts.Track
	track.AlignItems(ui.Stretch).Padding(2).Gap(2).Radius(8).Background(t.Surface).Margin(0, 10, 0, 2).Label("添加方式")
	track.Children(func() {
		for i, label := range []string{"预设", "自定义"} {
			s := parts.Segment(i).Grow(1).Padding(5, 12).Radius(6).Center()
			if i == a.addTab {
				s.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			} else {
				s.TextColor(t.TextMuted)
			}
			s.Children(func() { ui.Text(c, label).SingleLine() })
		}
	})
}

func (a *app) presetList(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Height(336).Shrink(0).Children(func() {
		ui.Column(c).Gap(6).Padding(0, 10, 0, 2).Children(func() {
			for _, p := range presetAgents {
				added := a.hasAgent(p)
				chosen := a.presetSel == p.Name
				row := ui.ButtonBase(c.Key("preset:"+p.Name)).AlignItems(ui.Center).Gap(12).Padding(8, 12).Radius(10).
					Label("选择 "+p.Name).Border(1, t.Border).Disabled(added)
				switch {
				case added:
					row.Background(t.Surface)
				case chosen:
					row.Border(1, t.Accent).Background(t.Accent.Alpha(0.08))
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					avatar(c, p.Name, 36)
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, p.Name).Bold().SingleLine()
						ui.Text(c, p.Dir).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
					})
					if added {
						tag(c, "已添加")
					}
				})
				row.OnClick(func() { a.presetSel = p.Name })
			}
		})
	})
}

func (a *app) customForm(c *ui.Context) {
	// The margins are the list's padding, so the inputs are exactly as
	// wide as the cards of the other tab.
	ui.Column(c).Gap(12).Margin(0, 10, 0, 2).Children(func() {
		ui.TextInput(c, &a.addName).Placeholder("例如 My Agent").Label("名称")
		ui.TextInput(c, &a.addDir).Placeholder("例如 ~/.my-agent/skills").Label("配置目录")
	})
}

// addAgentFooter is the modal's buttons, for either tab: 添加 takes the
// preset chosen, or the name and the directory of the form.
func (a *app) addAgentFooter(c *ui.Context) {
	ui.Row(c).Padding(10, 16).Gap(10).Justify(ui.End).Children(func() {
		ui.Button(c, "取消").OnClick(func() { a.addOpen = false })
		add := ui.PrimaryButton(c, "添加")
		if a.addTab == 0 {
			add.Disabled(a.presetSel == "").OnClick(func() {
				for _, p := range presetAgents {
					if p.Name == a.presetSel {
						a.addAgent(p)
					}
				}
				a.addOpen = false
			})
		} else {
			empty := strings.TrimSpace(a.addName) == "" || strings.TrimSpace(a.addDir) == ""
			add.Disabled(empty).OnClick(func() {
				a.addAgent(agent{strings.TrimSpace(a.addName), strings.TrimSpace(a.addDir)})
				a.addName, a.addDir = "", ""
				a.addOpen = false
			})
		}
	})
}
