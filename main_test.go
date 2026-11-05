package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestView(t *testing.T) {
	a := &app{selected: "hub", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	for _, s := range []string{"技能中心", "技能包", "Agent 技能", "设置"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
	if err := tt.Click("技能包"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "packs" {
		t.Errorf("selected %q after a click on 技能包", a.selected)
	}

	// The background of the item chosen stays blue whatever has the focus,
	// and it survives a hide and a show of the sidebar.
	if err := tt.Click("技能中心"); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find("技能中心")
	if !ok {
		t.Fatal("no 技能中心")
	}
	bg := func() [4]byte {
		img := tt.Image()
		off := img.PixOffset(int(r.X)-6, int(r.Y+r.H/2))
		return [4]byte{img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]}
	}
	focused := bg()
	tt.ClickAt(600, 400) // the empty pane takes the keyboard focus away
	if a.focusSide != "" {
		t.Errorf("focusSide %q after a click on the pane", a.focusSide)
	}
	if bg := bg(); bg != focused {
		t.Errorf("the background of the item chosen is %v without the sidebar's focus, was %v", bg, focused)
	}
	if err := tt.Click("Hide Sidebar"); err != nil {
		t.Fatal(err)
	}
	if !a.collapsed {
		t.Error("the sidebar shows after Hide Sidebar")
	}
	tt.Advance(300 * time.Millisecond) // as it slides out
	if err := tt.Click("Show Sidebar"); err != nil {
		t.Fatal(err)
	}
	if a.collapsed {
		t.Error("the sidebar hides after Show Sidebar")
	}
	tt.Advance(300 * time.Millisecond) // as it slides back in
	if a.focusSide != "sidebar" {
		t.Errorf("focusSide %q after Show Sidebar", a.focusSide)
	}
	if bg := bg(); bg != focused {
		t.Errorf("the background of the item chosen is %v after a hide and a show, was %v", bg, focused)
	}
}

// While the sidebar hides, the header of a pane starts right of the
// sidebar's toggle, clear of the window controls over the pane's left
// edge; it starts at the pane's edge otherwise.
func TestHeaderLead(t *testing.T) {
	bar := ui.TitleBar{Left: 78}
	a := &app{}
	if lead := a.headerLead(bar); lead != 20 {
		t.Errorf("headerLead with the sidebar showing = %v, want 20", lead)
	}
	a.collapsed = true
	if lead := a.headerLead(bar); lead != 126 {
		t.Errorf("headerLead with the sidebar hidden = %v, want 126", lead)
	}
}
