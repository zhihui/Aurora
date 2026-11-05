package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
)

// openSkillView opens the viewer on a skill: its SKILL.md, and its
// translation where there is one.
func (a *app) openSkillView(sk skill) {
	a.viewOpen = true
	a.viewDir = sk.Dir
	a.viewName = sk.Name
	a.viewTab = 0
	a.translateErr = ""
	a.viewZH = nil
	dir := filepath.Join(a.skillsDir, sk.Dir)
	if data, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		a.viewContent = string(data)
	} else {
		a.viewContent = ""
	}
	if tr, ok := loadTranslation(dir); ok {
		a.viewZH = tr
	}
}

// closeSkillView closes the viewer: a translation still running is
// canceled, its generation dropping the result.
func (a *app) closeSkillView() {
	a.viewOpen = false
	a.translateGen++
	if a.translateCancel != nil {
		a.translateCancel()
		a.translateCancel = nil
	}
	a.translating = false
	a.viewZH = nil
	a.viewContent = ""
	a.translateErr = ""
}

// startTranslate translates the viewed skill's SKILL.md in the
// background, with the configured model; the result is saved beside the
// skill, so the next look needs no model call.
func (a *app) startTranslate() {
	if a.translating {
		return
	}
	if a.llm.Endpoint == "" || a.llm.Model == "" || a.llm.APIKey == "" {
		a.translateErr = "请先在「设置」中配置大语言模型 API"
		return
	}
	if len(a.viewContent) > translateMaxLen {
		a.translateErr = "SKILL.md 过大，无法翻译"
		return
	}
	a.translating = true
	a.translateErr = ""
	a.translateGen++
	gen := a.translateGen
	ctx, cancel := context.WithCancel(context.Background())
	a.translateCancel = cancel
	cfg, dir, content := a.llm, filepath.Join(a.skillsDir, a.viewDir), a.viewContent
	go func() {
		out, err := llmChat(ctx, cfg, translateSystem, content)
		runOnMain(func() {
			if gen != a.translateGen {
				return
			}
			a.translating = false
			a.translateCancel = nil
			if err != nil {
				if ctx.Err() == nil {
					a.translateErr = err.Error()
				}
				return
			}
			tr := translation{
				SourceHash:   skillHash([]byte(content)),
				Model:        cfg.Model,
				TranslatedAt: time.Now(),
				Content:      out,
			}
			if err := saveTranslation(dir, tr); err != nil {
				a.translateErr = "译文保存失败：" + err.Error()
				return
			}
			a.viewZH = &tr
		})
	}()
}

// skillViewModal is the skill's viewer: its SKILL.md as 原文, or its
// Chinese translation as 中文 — the existing one, a 翻译 button where
// there is none, and a stale one asking to be redone where the SKILL.md
// changed since.
func (a *app) skillViewModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.viewOpen, func() {
		// ui.Modal's panel pads its content by t.Space(5); the negative
		// margin cancels it, so the sections' own padding rules and the
		// dividers span the modal edge to edge, as in the 导入技能 modal.
		ui.Column(c).Width(720).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, a.viewName).FontSize(15).Bold().SingleLine()
					if a.viewDir != "" && a.viewDir != a.viewName {
						ui.Text(c, "("+a.viewDir+")").FontSize(11).TextColor(t.TextMuted).SingleLine()
					}
					ui.Box(c).Grow(1)
					a.viewTabSwitch(c)
					modalClose(c, a.closeSkillView)
				})
				ui.Text(c, displayHome(filepath.Join(a.skillsDir, a.viewDir, "SKILL.md"))).Font("monospace").FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			ui.Divider(c)
			// The content takes the room the window has, 760 at most.
			h := float32(760)
			if _, wh := c.Size(); wh-96 < h {
				h = wh - 96
			}
			ui.Scroll(c).Height(max(h, 280)).Children(func() {
				ui.Column(c).Padding(14, 16).Children(func() {
					if a.viewTab == 1 {
						a.viewZHPane(c)
					} else {
						a.viewDoc(c, a.viewContent)
					}
				})
			})
		})
	})
}

// viewTabSwitch is the viewer's 原文/中文 switch: a small segmented
// control, as the hub's sort switch is.
func (a *app) viewTabSwitch(c *ui.Context) {
	t := c.Theme()
	parts := ui.SegmentedBase(c, &a.viewTab, 2)
	track := parts.Track
	track.AlignItems(ui.Center).Padding(2).Gap(2).Radius(8).Background(t.Surface).Label("语言")
	track.Children(func() {
		labels := []string{"原文", "中文"}
		for i, label := range labels {
			s := parts.Segment(i).Padding(4, 10).Radius(6).Center()
			if i == a.viewTab {
				s.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			} else {
				s.TextColor(t.TextMuted)
			}
			s.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					if i == 1 {
						ui.Icon(c, translateIcon).Size(11, 11)
					}
					ui.Text(c, label).FontSize(12).SingleLine()
				})
			})
		}
	})
}

// viewDoc is a document in the viewer: monospace, in a surface box.
func (a *app) viewDoc(c *ui.Context, content string) {
	t := c.Theme()
	ui.Column(c).Padding(14, 16).Radius(10).Background(t.Surface).Children(func() {
		ui.Text(c, content).Font("monospace").FontSize(12).FixedLineHeight(18)
	})
}

// viewZHPane is the 中文 tab: the fresh translation, a stale one with a
// warning, or a 翻译 button where there is none.
func (a *app) viewZHPane(c *ui.Context) {
	t := c.Theme()
	fresh := a.viewZH != nil && a.viewZH.SourceHash == skillHash([]byte(a.viewContent))
	switch {
	case fresh:
		a.viewDoc(c, a.viewZH.Content)
		ui.Row(c).Padding(8, 4, 0, 4).Children(func() {
			ui.Text(c, "由 "+a.viewZH.Model+" 翻译于 "+a.viewZH.TranslatedAt.Format("2006-01-02")).FontSize(10).TextColor(t.TextMuted)
		})
	case a.viewZH != nil:
		// The SKILL.md changed since: the old translation shows, marked.
		ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(0, 0, 10, 0).Children(func() {
			ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Warning)
			ui.Text(c, "SKILL.md 已更新，译文可能已过期").FontSize(12).TextColor(t.Warning).Grow(1)
			a.translateButton(c, "重新翻译")
		})
		a.viewDoc(c, a.viewZH.Content)
	default:
		ui.Column(c).Height(300).Center().Gap(10).Children(func() {
			ui.Icon(c, translateIcon).Size(28, 28).TextColor(t.TextMuted)
			if a.llm.Endpoint == "" || a.llm.Model == "" || a.llm.APIKey == "" {
				ui.Text(c, "请先在「设置」中配置大语言模型 API").FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "还没有中文译文").FontSize(12).TextColor(t.TextMuted)
				a.translateButton(c, "翻译")
			}
		})
	}
	if a.translateErr != "" {
		ui.Row(c).AlignItems(ui.Center).Gap(5).Padding(10, 0, 0, 0).Children(func() {
			ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
			ui.Text(c, a.translateErr).FontSize(12).TextColor(t.Danger).FixedLineHeight(18)
		})
	}
}

// translateButton starts the translation, 翻译中… and still while it
// runs.
func (a *app) translateButton(c *ui.Context, label string) {
	if a.translating {
		label = "翻译中…"
	}
	ui.PrimaryButton(c, label).Disabled(a.translating).OnClick(func() { a.startTranslate() })
}
