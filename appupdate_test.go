package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// stubAppUpdate swaps the app update's backend for synchronous stubs:
// check answers with up and checkErr, install reports progress and
// answers installErr, and runOnMain delivers on the spot, signaling
// applied. The returned relaunched tells whether 立即重启 ran.
func stubAppUpdate(t *testing.T, up *mygo.Update, checkErr, installErr error) (applied chan struct{}, relaunched *bool) {
	t.Helper()
	applied = make(chan struct{}, 16)
	relaunched = new(bool)
	oldEnabled, oldCheck, oldInstall := appUpdateEnabled, appUpdateCheck, appUpdateInstall
	oldVersion, oldRelaunch, oldRun := appVersion, appRelaunch, runOnMain
	t.Cleanup(func() {
		appUpdateEnabled, appUpdateCheck, appUpdateInstall = oldEnabled, oldCheck, oldInstall
		appVersion, appRelaunch, runOnMain = oldVersion, oldRelaunch, oldRun
	})
	appUpdateEnabled = func() bool { return true }
	appUpdateCheck = func(context.Context) (*mygo.Update, error) { return up, checkErr }
	appUpdateInstall = func(_ context.Context, _ *mygo.Update, progress func(int64, int64)) error {
		progress(50, 100)
		progress(100, 100)
		return installErr
	}
	appVersion = func() string { return "0.1.0" }
	appRelaunch = func() { *relaunched = true }
	runOnMain = func(f func()) { f(); applied <- struct{}{} }
	return applied, relaunched
}

// A manual check that finds a version opens the modal with its notes,
// and the settings' list notes it too.
func TestCheckAppUpdateFound(t *testing.T) {
	up := &mygo.Update{Version: "0.2.0", Notes: "新增版本更新提示。", Date: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}
	applied, _ := stubAppUpdate(t, up, nil, nil)

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	if !tt.HasText("0.1.0") {
		t.Fatalf("no current version in %q", tt.Texts())
	}
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if !a.appUpdOpen || a.appUpdAvail != up {
		t.Fatalf("open = %v, avail = %+v", a.appUpdOpen, a.appUpdAvail)
	}
	for _, s := range []string{"发现新版本 0.2.0", "0.1.0 → 0.2.0（2026-11-05）", "新增版本更新提示。", "下载并安装"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
}

// A check that finds nothing says so on the page and opens no modal.
func TestCheckAppUpdateNone(t *testing.T) {
	applied, _ := stubAppUpdate(t, nil, nil, nil)

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if a.appUpdOpen || !a.appUpdChecked {
		t.Fatalf("open = %v, checked = %v", a.appUpdOpen, a.appUpdChecked)
	}
	if !tt.HasText("已是最新版本。") {
		t.Errorf("no 已是最新 in %q", tt.Texts())
	}
}

// A manual check's failure shows on the page; the startup's automatic
// check stays quiet about it.
func TestCheckAppUpdateErr(t *testing.T) {
	applied, _ := stubAppUpdate(t, nil, errors.New("network down"), nil)

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if a.appUpdErr == "" || a.appUpdOpen {
		t.Fatalf("err = %q, open = %v", a.appUpdErr, a.appUpdOpen)
	}
	if !tt.HasText("检查更新失败") {
		t.Errorf("no failure in %q", tt.Texts())
	}

	// The automatic check of the same failure records nothing.
	b := &app{}
	b.checkAppUpdate(false)
	<-applied
	if b.appUpdErr != "" || b.appUpdOpen {
		t.Errorf("auto: err = %q, open = %v", b.appUpdErr, b.appUpdOpen)
	}
}

// The install's progress lands, its completion offers 立即重启, and the
// button relaunches the app.
func TestInstallAppUpdate(t *testing.T) {
	up := &mygo.Update{Version: "0.2.0", Notes: "说明。"}
	applied, relaunched := stubAppUpdate(t, up, nil, nil)

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	<-applied
	tt.Frame()
	if err := tt.Click("下载并安装"); err != nil {
		t.Fatal(err)
	}
	// One signal per progress callback and one for the result.
	<-applied
	<-applied
	<-applied
	tt.Frame()
	if !a.appUpdDone || a.appUpdInstalling || a.appUpdProgress != 1 {
		t.Fatalf("done = %v, installing = %v, progress = %v", a.appUpdDone, a.appUpdInstalling, a.appUpdProgress)
	}
	if !tt.HasText("更新已安装，重启后生效。") {
		t.Errorf("no completion in %q", tt.Texts())
	}
	if err := tt.Click("立即重启"); err != nil {
		t.Fatal(err)
	}
	if !*relaunched {
		t.Errorf("the app did not relaunch")
	}
}

// The install's failure is reported and offers 重试.
func TestInstallAppUpdateErr(t *testing.T) {
	up := &mygo.Update{Version: "0.2.0"}
	applied, _ := stubAppUpdate(t, up, nil, fmt.Errorf("签名校验失败"))

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	a.checkAppUpdate(true)
	<-applied
	tt.Frame()
	if err := tt.Click("下载并安装"); err != nil {
		t.Fatal(err)
	}
	<-applied
	<-applied
	<-applied
	tt.Frame()
	if a.appUpdDone || a.appUpdErr == "" {
		t.Fatalf("done = %v, err = %q", a.appUpdDone, a.appUpdErr)
	}
	if !tt.HasText("更新失败：签名校验失败") || !tt.HasText("重试") {
		t.Errorf("no failure in %q", tt.Texts())
	}
}

// A build without updates — the development build — shows why and its
// button does nothing.
func TestAppUpdateDisabled(t *testing.T) {
	_, _ = stubAppUpdate(t, nil, nil, nil)
	old := appUpdateEnabled
	t.Cleanup(func() { appUpdateEnabled = old })
	called := false
	oldCheck := appUpdateCheck
	t.Cleanup(func() { appUpdateCheck = oldCheck })
	appUpdateEnabled = func() bool { return false }
	appUpdateCheck = func(context.Context) (*mygo.Update, error) { called = true; return nil, nil }

	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 900)
	if !tt.HasText("开发构建不包含自动更新") {
		t.Errorf("no hint in %q", tt.Texts())
	}
	// The startup's check skips a build without updates.
	a.autoCheckAppUpdate()
	if called {
		t.Errorf("the automatic check ran on a build without updates")
	}
}

// Closing the modal cancels the check in flight, its stale result
// dropped by the generation.
func TestCloseAppUpdateCancels(t *testing.T) {
	up := &mygo.Update{Version: "0.2.0"}
	applied, _ := stubAppUpdate(t, up, nil, nil)

	a := &app{selected: "settings", focusSide: "sidebar"}
	a.appUpdChecking = true // a check the window still waits for
	a.checkAppUpdate(true)  // refused: one runs already
	select {
	case <-applied:
		t.Fatalf("the second check ran")
	default:
	}

	// A canceled check's result never lands.
	b := &app{}
	b.checkAppUpdate(true)
	b.closeAppUpdate()
	<-applied
	tt := ui.NewTester(b.view, 1000, 640)
	_ = tt
	if b.appUpdOpen || b.appUpdAvail != nil {
		t.Errorf("the canceled check's result landed: open = %v, avail = %+v", b.appUpdOpen, b.appUpdAvail)
	}
}
