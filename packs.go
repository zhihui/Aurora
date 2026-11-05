package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// pack is a skill pack: a named set of the hub's skills, linked to an
// agent in one click. The skills are the skills' directories — their
// identity on disk, not the names their SKILL.md declares, which two
// directories can share; one the hub no longer has shows as 已丢失.
type pack struct {
	Name   string   `json:"name"`
	Skills []string `json:"skills"`
}

// packRemoval is a skill clicked out of its pack; the removal waits for
// the loop over the packs to end: the slices change then, not while they
// are built from.
type packRemoval struct {
	pack  int
	skill string
}

// loadPacks reads the packs from packsPath; a save follows every change.
// The path is "" in tests, which stay in memory.
func (a *app) loadPacks() {
	if a.packsPath == "" {
		return
	}
	data, err := os.ReadFile(a.packsPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.packs)
}

func (a *app) savePacks() {
	if a.packsPath == "" {
		return
	}
	data, err := json.MarshalIndent(a.packs, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.packsPath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(a.packsPath, data, 0o644)
}

// dropSkill removes the skill from every pack: a pack never refers to a
// skill that is gone. It runs after the hub deletes a skill.
func (a *app) dropSkill(name string) {
	changed := false
	for i := range a.packs {
		p := &a.packs[i]
		for j := 0; j < len(p.Skills); j++ {
			if p.Skills[j] == name {
				p.Skills = slices.Delete(p.Skills, j, j+1)
				j--
				changed = true
			}
		}
	}
	if changed {
		a.savePacks()
	}
}

// matchPack reports whether the search text shows the pack: it matches
// the name and the skills, ignoring case.
func matchPack(p *pack, query string) bool {
	query = strings.ToLower(query)
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(p.Name), query) {
		return true
	}
	for _, name := range p.Skills {
		if strings.Contains(strings.ToLower(name), query) {
			return true
		}
	}
	return false
}

// packTargets resolves the pack's skills to their directories in the
// hub, skipping the ones the hub no longer has — a skill deleted outside
// the app shows in the pack as 已丢失 instead, and links nothing.
func (a *app) packTargets(p *pack) []string {
	have := make(map[string]bool, len(a.skills))
	for _, sk := range a.skills {
		have[sk.Dir] = true
	}
	var targets []string
	for _, dir := range p.Skills {
		if have[dir] {
			targets = append(targets, filepath.Join(a.skillsDir, dir))
		}
	}
	return targets
}

// packsPane is the pane of the 技能包 menu: a search field, the
// 新建技能包 button, and the packs as cards, one to a row.
func (a *app) packsPane(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.paneHeader(c, "技能包", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				capsuleSearch(c, &a.packSearch, "搜索技能包…", "搜索技能包", false).Width(200)
				ui.PrimaryButton(c, "新建技能包…").OnClick(func() { a.packEdit, a.packName, a.packOpen = -1, "", true })
			})
		})
		ui.Divider(c)
		query := strings.TrimSpace(a.packSearch)
		var shown []int
		for i := range a.packs {
			if matchPack(&a.packs[i], query) {
				shown = append(shown, i)
			}
		}
		if len(shown) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				if len(a.packs) == 0 {
					ui.Image(c, packsIcon).Size(40, 40)
					ui.Text(c, "还没有技能包").TextColor(t.TextMuted)
					ui.Text(c, "点击右上角「新建技能包」开始").FontSize(12).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "没有匹配的技能包").TextColor(t.TextMuted)
					ui.Text(c, "换个关键词试试").FontSize(12).TextColor(t.TextMuted)
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 28, 24, 28).Gap(14).Children(func() {
				ui.Text(c, "点击包底部的 agent 胶囊，将包内全部技能一次性软链接到该 agent。").FontSize(12).TextColor(t.TextMuted)
				var removals []packRemoval
				for _, pi := range shown {
					a.packCard(c, pi, &removals)
				}
				for _, r := range removals {
					p := &a.packs[r.pack]
					for j, name := range p.Skills {
						if name == r.skill {
							p.Skills = slices.Delete(p.Skills, j, j+1)
							break
						}
					}
				}
				if len(removals) > 0 {
					a.savePacks()
				}
			})
		})
	})
}

// packCard is one pack of the pane's list: its name and size, the skills
// as capsules — a click taking one out of the pack — the 添加技能
// capsule, and every agent as a capsule that links the whole pack to the
// agent. The 重命名 and 删除 buttons sit at its top right.
func (a *app) packCard(c *ui.Context, pi int, removals *[]packRemoval) {
	t := c.Theme()
	p := &a.packs[pi]
	names := make(map[string]string, len(a.skills))
	for _, sk := range a.skills {
		names[sk.Dir] = sk.Name
	}
	ui.Column(c).Gap(12).Padding(14, 16).Radius(12).Background(t.Surface).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, p.Name).FontSize(14).Bold().SingleLine()
			tag(c, fmt.Sprintf("%d 个技能", len(p.Skills)))
			ui.Box(c).Grow(1)
			iconAction(c, "packedit:"+strconv.Itoa(pi), "重命名技能包", pencilIcon, func() {
				if pi < len(a.packs) {
					a.packEdit, a.packName, a.packOpen = pi, a.packs[pi].Name, true
				}
			})
			iconAction(c, "packdel:"+strconv.Itoa(pi), "删除技能包", trashIcon, func() {
				a.delPack, a.delPackOpen = pi, true
			})
		})
		ui.Row(c).Wrap().Gap(chipGap).AlignItems(ui.Center).Children(func() {
			for _, dir := range p.Skills {
				display, ok := names[dir]
				if !ok {
					display = dir
				}
				a.packSkillChip(c, pi, dir, display, !ok, removals)
			}
			a.packAddButton(c, pi)
		})
		ui.Divider(c)
		ui.Row(c).Wrap().Gap(chipGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "分配给：").FontSize(12).TextColor(t.TextMuted).SingleLine()
			if len(a.agents) == 0 {
				ui.Text(c, "暂无 agent，请先在设置中添加").FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
			for _, ag := range a.agents {
				a.packAgentChip(c, pi, ag)
			}
		})
	})
}

// packSkillChip is one skill of a pack: an inert capsule whose × — the
// only control of it — takes the skill out of the pack; the skill
// itself stays in the hub. dir is the skill's identity, display what
// the capsule shows. One the hub no longer has shows as 已丢失, in the
// warning color; its × takes it out too.
func (a *app) packSkillChip(c *ui.Context, pi int, dir, display string, missing bool, removals *[]packRemoval) {
	t := c.Theme()
	ch := ui.Row(c).Height(chipHeight).AlignItems(ui.Center).Gap(5).Padding(0, 2, 0, 10).Radius(chipHeight / 2).Shrink(0)
	if missing {
		ch.Border(1, t.Warning.Alpha(0.5)).Background(t.Warning.Alpha(0.08))
	} else {
		ch.Border(1, t.Border).Background(t.Background)
	}
	ch.Children(func() {
		if missing {
			ui.Icon(c, alertIcon).Size(11, 11).TextColor(t.Warning)
		}
		txt := ui.Text(c, display).FontSize(11).FontWeight(500).SingleLine()
		if missing {
			txt.TextColor(t.Warning)
		} else {
			txt.TextColor(t.Text)
		}
		label := "从技能包移除 " + display
		if missing {
			label = "「" + display + "」已丢失，点击从技能包移除"
		}
		x := ui.ButtonBase(c.Key("packskillrm:"+strconv.Itoa(pi)+"/"+dir)).Size(20, 20).Radius(5).Center().TextColor(t.TextMuted).Label(label)
		if x.Hovered() {
			x.Background(t.SurfaceHover).TextColor(t.Text)
		}
		x.Children(func() { ui.Icon(c, xIcon).Size(10, 10) })
		if x.Clicked() {
			*removals = append(*removals, packRemoval{pi, dir})
		}
	})
}

// packAddButton is the 添加技能 capsule of a pack: an accent ring that
// opens the modal picking skills from the hub.
func (a *app) packAddButton(c *ui.Context, pi int) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("packadd:"+strconv.Itoa(pi))).Height(chipHeight).AlignItems(ui.Center).Padding(0, 10).Radius(chipHeight/2).Shrink(0).
		Border(1, t.Accent.Alpha(0.5)).TextColor(t.Accent).Label("添加技能到技能包")
	if b.Hovered() {
		b.Background(t.Accent.Alpha(0.08))
	}
	b.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Icon(c, plusIcon).Size(11, 11)
			ui.Text(c, "添加技能…").FontSize(11).FontWeight(500).SingleLine()
		})
	})
	b.OnClick(func() {
		a.pickPack, a.pickSearch, a.pickOpen = pi, "", true
	})
}

// packAgentChip is one agent of a pack's 分配给 row. Chosen — the agent
// links to every skill of the pack — its ring is blue on light blue
// with dark text and a check at its leading edge; else it is a gray
// ring on white with gray text carrying an empty circle, and the
// pointer deepens both. The glyph marks the state in more than color
// and is the same size in both states, so a toggle never shifts the
// layout and neither state is a blank space. A click links the whole
// pack, or removes the links when they are all there.
func (a *app) packAgentChip(c *ui.Context, pi int, ag agent) {
	t := c.Theme()
	p := &a.packs[pi]
	targets := a.packTargets(p)
	dir := expandHome(ag.Dir)
	on := packLinked(dir, targets)
	ch := ui.ButtonBase(c.Key("packagent:"+strconv.Itoa(pi)+"/"+ag.Name)).Height(chipHeight).AlignItems(ui.Center).Gap(4).Padding(0, 10).Radius(chipHeight / 2).Shrink(0).Label(ag.Name)
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
		switch {
		case len(p.Skills) == 0:
			c.Toast("技能包为空，请先添加技能")
		case len(targets) == 0:
			c.Toast("包内技能均已丢失，请先移除")
		default:
			msg, err := togglePack(dir, targets, ag.Name)
			switch {
			case err != nil:
				c.Toast("无法更新「" + p.Name + "」：" + err.Error())
			case msg != "":
				c.Toast(msg)
			}
		}
	}
}

// iconAction is a small icon button of a card's header, as the 重命名
// and 删除 of a pack: muted until the pointer deepens it.
func iconAction(c *ui.Context, key, label string, icon *ui.SVG, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(6).Center().TextColor(t.TextMuted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, icon).Size(14, 14) })
	b.OnClick(onClick)
}

// packNameModal is the dialog naming a pack: a new one (packEdit -1) or
// the rename of the one it edits. A name taken by another pack shows in
// red and keeps the button still.
func (a *app) packNameModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.packOpen, func() {
		rename := a.packEdit >= 0 && a.packEdit < len(a.packs)
		title, action := "新建技能包", "创建"
		if rename {
			title, action = "重命名技能包", "保存"
		}
		trim := strings.TrimSpace(a.packName)
		dup := false
		for i, p := range a.packs {
			if p.Name == trim && i != a.packEdit {
				dup = true
				break
			}
		}
		ui.Column(c).Width(420).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).FontSize(15).Bold().Grow(1)
					modalClose(c, func() { a.packOpen = false })
				})
				if !rename {
					ui.Text(c, "给一组常用技能起个名字，之后可一次性分配给 agent。").FontSize(12).TextColor(t.TextMuted)
				}
			})
			ui.Divider(c)
			ui.Column(c).Padding(14, 16).Gap(8).Children(func() {
				ui.TextInput(c, &a.packName).Placeholder("例如 前端助手").Label("名称")
				if dup {
					ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
						ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
						ui.Text(c, "已存在同名技能包").FontSize(12).TextColor(t.Danger)
					})
				}
			})
			ui.Divider(c)
			ui.Row(c).Padding(10, 16).Gap(10).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.packOpen = false })
				save := ui.PrimaryButton(c, action).Disabled(trim == "" || dup)
				if save.Clicked() && trim != "" && !dup {
					if rename {
						a.packs[a.packEdit].Name = trim
					} else {
						a.packs = append(a.packs, pack{Name: trim})
					}
					a.savePacks()
					a.packOpen = false
				}
			})
		})
	})
}

// deletePackModal asks before the card's 删除 deletes the pack: only the
// pack goes — the skills stay in the hub, and the links made stay with
// the agents.
func (a *app) deletePackModal(c *ui.Context) {
	name := ""
	if a.delPack >= 0 && a.delPack < len(a.packs) {
		name = a.packs[a.delPack].Name
	}
	if ui.AlertDialog(c, &a.delPackOpen, "删除技能包",
		"将删除技能包「"+name+"」，包内的技能与已创建的链接不受影响。",
		"取消", "删除") == 1 {
		if a.delPack >= 0 && a.delPack < len(a.packs) {
			a.packs = slices.Delete(a.packs, a.delPack, a.delPack+1)
			a.savePacks()
		}
	}
}

// pickSkillsModal is the dialog adding skills to a pack: the hub's
// skills as a list to pick from, one click adding one — the modal stays
// open, so several can be added in a row. One the pack has shows 已添加
// and does not answer.
func (a *app) pickSkillsModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.pickOpen, func() {
		name := ""
		if a.pickPack >= 0 && a.pickPack < len(a.packs) {
			name = a.packs[a.pickPack].Name
		}
		ui.Column(c).Width(520).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "添加技能到 "+name).FontSize(15).Bold().Grow(1).SingleLine()
					modalClose(c, func() { a.pickOpen = false })
				})
				ui.Text(c, "从技能中心选择技能，可连续添加多个。").FontSize(12).TextColor(t.TextMuted)
			})
			ui.Divider(c)
			ui.Column(c).Padding(12, 16).Gap(10).Children(func() {
				capsuleSearch(c, &a.pickSearch, "搜索技能名称或描述…", "搜索技能名称或描述", false)
				a.pickSkillsList(c)
			})
			ui.Divider(c)
			ui.Row(c).Padding(10, 16).Justify(ui.End).Children(func() {
				ui.Button(c, "完成").OnClick(func() { a.pickOpen = false })
			})
		})
	})
}

// pickSkillsList is the modal's list of the hub's skills, filtered by
// the search.
func (a *app) pickSkillsList(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Height(320).Shrink(0).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			if len(a.skills) == 0 {
				ui.Column(c).Height(120).Center().Children(func() {
					ui.Text(c, "技能中心还没有技能，请先导入").TextColor(t.TextMuted)
				})
				return
			}
			query := strings.ToLower(strings.TrimSpace(a.pickSearch))
			shown := 0
			for _, sk := range a.skills {
				if query != "" &&
					!strings.Contains(strings.ToLower(sk.Name), query) &&
					!strings.Contains(strings.ToLower(sk.Desc), query) {
					continue
				}
				shown++
				a.pickSkillRow(c, sk)
			}
			if shown == 0 {
				ui.Column(c).Height(120).Center().Children(func() {
					ui.Text(c, "没有匹配的技能").TextColor(t.TextMuted)
				})
			}
		})
	})
}

// pickSkillRow is one skill of the modal's list: its name and what it
// does — the directory's name beside the skill's where they differ,
// telling two skills of a name apart. A click adds it to the pack; one
// the pack has is marked 已添加 and does not answer. The pack takes the
// directory: the skill's identity on disk.
func (a *app) pickSkillRow(c *ui.Context, sk skill) {
	t := c.Theme()
	added := false
	if a.pickPack >= 0 && a.pickPack < len(a.packs) {
		added = slices.Contains(a.packs[a.pickPack].Skills, sk.Dir)
	}
	row := ui.ButtonBase(c.Key("pick:"+sk.Dir)).AlignItems(ui.Center).Padding(8, 10).Radius(8).Justify(ui.Start).
		Label("添加 " + sk.Name).Disabled(added)
	if row.Hovered() && !added {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				name := ui.Text(c, sk.Name).FontSize(13).Bold().SingleLine()
				if added {
					name.TextColor(t.TextMuted)
				}
				if sk.Dir != "" && sk.Dir != sk.Name {
					ui.Text(c, "("+sk.Dir+")").FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
			desc := sk.Desc
			if desc == "" {
				desc = "暂无简介"
			}
			ui.Text(c, desc).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if added {
			tag(c, "已添加")
		}
	})
	row.OnClick(func() {
		if a.pickPack >= 0 && a.pickPack < len(a.packs) && !slices.Contains(a.packs[a.pickPack].Skills, sk.Dir) {
			a.packs[a.pickPack].Skills = append(a.packs[a.pickPack].Skills, sk.Dir)
			a.savePacks()
		}
	})
}
