package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The app update's backend, variables so the tests can stub them: a
// development build has no update feed (mygo.Updater.Enabled is false),
// so the UI would never show otherwise.
var (
	appUpdateEnabled = mygo.Updater.Enabled
	appUpdateCheck   = func(ctx context.Context) (*mygo.Update, error) { return mygo.Updater.Check(ctx) }
	appUpdateInstall = func(ctx context.Context, up *mygo.Update, progress func(downloaded, total int64)) error {
		return up.Install(ctx, progress)
	}
	appVersion  = mygo.App.Version
	appRelaunch = func() { mygo.App.Relaunch() }
)

// checkAppUpdate asks the update feed for a newer version, in the
// background; a found update opens the update modal. A manual check (the
// settings' button) reports its failure on the page; the startup's
// automatic check stays quiet, as a laptop offline is not an error to
// show.
func (a *app) checkAppUpdate(manual bool) {
	if a.appUpdChecking || a.appUpdInstalling {
		return
	}
	a.appUpdChecking = true
	a.appUpdErr = ""
	a.appUpdChecked = false
	a.appUpdGen++
	gen := a.appUpdGen
	ctx, cancel := context.WithCancel(context.Background())
	a.appUpdCancel = cancel
	go func() {
		up, err := appUpdateCheck(ctx)
		runOnMain(func() {
			if gen != a.appUpdGen {
				return
			}
			a.appUpdChecking = false
			a.appUpdCancel = nil
			if ctx.Err() != nil {
				return
			}
			switch {
			case err != nil:
				// The startup's automatic check stays quiet; a manual
				// one reports its failure on the page.
				if !manual {
					return
				}
				if errors.Is(err, mygo.ErrUpdatesDisabled) {
					a.appUpdErr = "此构建未启用自动更新。"
				} else {
					a.appUpdErr = fmt.Sprintf("检查更新失败：%v", err)
				}
			case up != nil:
				a.appUpdAvail = up
				a.appUpdDone = false
				a.appUpdOpen = true
			default:
				a.appUpdChecked = true
				a.appUpdAvail = nil
			}
		})
	}()
}

// installAppUpdate downloads and installs the update the check found,
// in the background; the download's progress lands on the main thread
// through runOnMain, as the check's result does. The running app is not
// affected: the new version takes over on relaunch.
func (a *app) installAppUpdate() {
	if a.appUpdInstalling || a.appUpdAvail == nil {
		return
	}
	a.appUpdInstalling = true
	a.appUpdErr = ""
	a.appUpdProgress = 0
	a.appUpdGen++
	gen := a.appUpdGen
	ctx, cancel := context.WithCancel(context.Background())
	a.appUpdCancel = cancel
	up := a.appUpdAvail
	go func() {
		err := appUpdateInstall(ctx, up, func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			p := float64(downloaded) / float64(total)
			runOnMain(func() {
				if gen == a.appUpdGen {
					a.appUpdProgress = p
				}
			})
		})
		runOnMain(func() {
			if gen != a.appUpdGen {
				return
			}
			a.appUpdInstalling = false
			a.appUpdCancel = nil
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				a.appUpdErr = fmt.Sprintf("更新失败：%v", err)
				return
			}
			a.appUpdDone = true
		})
	}()
}

// closeAppUpdate closes the update modal: a check or install still
// running is canceled, its generation dropping the result. An update
// already installed stays staged and takes over at the next launch.
func (a *app) closeAppUpdate() {
	a.appUpdOpen = false
	a.appUpdGen++
	if a.appUpdCancel != nil {
		a.appUpdCancel()
		a.appUpdCancel = nil
	}
	a.appUpdChecking = false
	a.appUpdInstalling = false
}

// appUpdateSection is the settings' 版本更新 block: the running version,
// the check's outcome, and the button starting one.
func (a *app) appUpdateSection(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "版本更新").FontSize(13).Bold()
		ui.Text(c, "检查并安装 Aurora 的新版本；更新包经过签名验证。").FontSize(12).TextColor(t.TextMuted)
		list := ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Clip()
		list.Children(func() {
			a.llmRow(c, "当前版本", func() {
				ui.Text(c, appVersion()).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
			}, func() {
				label := "检查更新"
				if a.appUpdChecking {
					label = "检查中…"
				}
				linkAction(c, label, a.appUpdChecking || a.appUpdInstalling || !appUpdateEnabled(), func() {
					if !appUpdateEnabled() {
						return
					}
					a.checkAppUpdate(true)
				})
			})
			if a.appUpdAvail != nil && !a.appUpdDone {
				a.llmDivider(c)
				a.appUpdAvailRow(c)
			} else if a.appUpdErr != "" {
				a.llmDivider(c)
				a.appUpdStatusRow(c, alertIcon, t.Danger, a.appUpdErr)
			} else if a.appUpdDone {
				a.llmDivider(c)
				a.appUpdStatusRow(c, checkCircleIcon, t.Success, "更新已安装，重启后生效。")
			} else if a.appUpdChecked {
				a.llmDivider(c)
				a.appUpdStatusRow(c, checkCircleIcon, t.Success, "已是最新版本。")
			}
		})
		if !appUpdateEnabled() {
			ui.Text(c, "开发构建不包含自动更新；正式安装包才能检查和安装新版本。").FontSize(11).TextColor(t.TextMuted).FixedLineHeight(16)
		}
	})
}

// appUpdAvailRow is the settings' notice of the update the check found,
// with 查看 opening the update modal.
func (a *app) appUpdAvailRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(10, 14).Children(func() {
		ui.Icon(c, downloadIcon).Size(13, 13).TextColor(t.Accent)
		ui.Text(c, "发现新版本 "+a.appUpdAvail.Version).FontSize(12).FontWeight(500).TextColor(t.Accent).Grow(1).SingleLine()
		linkAction(c, "查看", a.appUpdInstalling, func() { a.appUpdOpen = true })
	})
}

// appUpdStatusRow is one line of the check's outcome in the settings'
// container: an icon in a semantic color and the message.
func (a *app) appUpdStatusRow(c *ui.Context, icon *ui.SVG, color ui.Color, msg string) {
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(10, 14).Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(color)
		ui.Text(c, msg).FontSize(12).TextColor(color).Grow(1).FixedLineHeight(18)
	})
}

// appUpdateModal is the version update dialog: the new version's notes,
// the install's progress, and the relaunch once it is staged.
func (a *app) appUpdateModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.appUpdOpen, func() {
		// ui.Modal's panel pads its content by t.Space(5); the negative
		// margin cancels it, so the sections' own padding rules and the
		// dividers span the modal edge to edge, as in the 导入技能 modal.
		ui.Column(c).Width(492).Margin(-t.Space(5)).Children(func() {
			ui.Column(c).Padding(14, 16, 12, 16).Gap(6).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "版本更新").FontSize(15).Bold().Grow(1)
					modalClose(c, a.closeAppUpdate)
				})
				if up := a.appUpdAvail; up != nil {
					sub := fmt.Sprintf("%s → %s", appVersion(), up.Version)
					if !up.Date.IsZero() {
						sub += "（" + up.Date.Format("2006-01-02") + "）"
					}
					ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted)
				}
			})
			ui.Divider(c)
			ui.Column(c).Padding(14, 16).Gap(10).Children(func() {
				a.appUpdBody(c)
			})
			ui.Divider(c)
			a.appUpdFooter(c)
		})
	})
}

// appUpdBody is the modal's content: the update's notes, or the
// install's progress while it downloads, or why it failed.
func (a *app) appUpdBody(c *ui.Context) {
	t := c.Theme()
	switch {
	case a.appUpdInstalling:
		ui.Column(c).Gap(6).Children(func() {
			ui.Progress(c, a.appUpdProgress)
			ui.Text(c, fmt.Sprintf("正在下载更新… %d%%", int(a.appUpdProgress*100))).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	case a.appUpdDone:
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, checkCircleIcon).Size(14, 14).TextColor(t.Success)
			ui.Text(c, "更新已安装，重启 Aurora 后生效。").FontSize(13)
		})
	default:
		notes := ""
		if a.appUpdAvail != nil {
			notes = a.appUpdAvail.Notes
		}
		if notes == "" {
			notes = "该版本没有更新说明。"
		}
		ui.Scroll(c).Height(220).Shrink(0).Children(func() {
			ui.Column(c).Radius(8).Background(t.Surface).Padding(10, 12).Children(func() {
				ui.Text(c, notes).FontSize(12).FixedLineHeight(18)
			})
		})
		if a.appUpdErr != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
				ui.Icon(c, alertIcon).Size(12, 12).TextColor(t.Danger)
				ui.Text(c, a.appUpdErr).FontSize(12).TextColor(t.Danger).FixedLineHeight(18)
			})
		}
	}
}

// appUpdFooter is the modal's buttons: 稍后 leaves the choice for
// later, 下载并安装 starts it, 重试 another attempt after a failure,
// 立即重启 once the update is staged.
func (a *app) appUpdFooter(c *ui.Context) {
	ui.Row(c).Padding(10, 16).Gap(10).Justify(ui.End).Children(func() {
		if a.appUpdInstalling {
			ui.PrimaryButton(c, "下载中…").Disabled(true)
			return
		}
		ui.Button(c, "稍后").OnClick(func() { a.closeAppUpdate() })
		if a.appUpdDone {
			ui.PrimaryButton(c, "立即重启").OnClick(func() { appRelaunch() })
			return
		}
		label := "下载并安装"
		if a.appUpdErr != "" {
			label = "重试"
		}
		ui.PrimaryButton(c, label).Disabled(a.appUpdAvail == nil).OnClick(func() { a.installAppUpdate() })
	})
}

// autoCheckAppUpdate is the startup's check: quiet unless a newer
// version exists, which opens the update modal.
func (a *app) autoCheckAppUpdate() {
	if !appUpdateEnabled() {
		return
	}
	a.checkAppUpdate(false)
}
