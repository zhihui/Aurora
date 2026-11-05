package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	titleBarHeight = 52 // the height of an inset title bar
	sidebarWidth   = 220
)

// item is an entry of the sidebar: its ID, its label, the colored tile of
// its icon and the value at its trailing edge, "" for none.
type item struct {
	id, label string
	icon      *ui.SVG
	detail    string
}

var menuItems = []item{
	{"hub", "技能中心", hubIcon, ""},
	{"packs", "技能包", packsIcon, ""},
	{"agent", "Agent 技能", agentIcon, ""},
	{"settings", "设置", settingsIcon, ""},
}

// app is the state the window shows. Its view builds the interface from
// it, on the main thread, whenever the window needs a frame: after input,
// after Window.Update, and while something animates.
type app struct {
	selected  string // the item chosen in the sidebar
	focusSide string // the focus of the window: "sidebar" or ""
	collapsed bool   // the sidebar is hidden

	agents     []agent // the agents the user added
	agentsPath string  // where the agents persist; "" in tests
	addOpen    bool    // the Add Agent modal shows
	addTab     int     // its tab: 0 预设, 1 自定义
	presetSel  string  // the name of the preset chosen in the modal
	addName    string  // the custom tab's name
	addDir     string  // the custom tab's directory

	llm         llmConfig // the saved LLM API configuration
	llmPath     string    // where it persists; "" in tests
	llmEdit     bool      // the settings' form edits it
	llmTypeIdx  int       // the form's type: an index of llmTypes
	llmEndpoint string    // the form's endpoint
	llmModel    string    // the form's model name
	llmKey      string    // the form's key; empty keeps the saved one

	viewOpen        bool   // the skill viewer shows
	viewDir         string // the skill it shows: the directory's name
	viewName        string // its display name
	viewTab         int    // 0 原文, 1 中文
	viewContent     string // the SKILL.md it shows
	viewZH          *translation
	translating     bool   // a translation runs in the background
	translateErr    string // its failure, shown in the viewer
	translateGen    int    // its generation, to tell a stale result
	translateCancel context.CancelFunc

	skillSearch string // the text the hub searches by
	skillSort   int    // the hub's order: 0 名称, 1 时间
	searchFocus bool   // the menu's 搜索… asks the current pane's search field to take the focus
	skillsDir   string // the directory the hub's skills live in; "" in tests
	skills      []skill
	delName     string // the name of the skill the card's × asks about
	delDir      string // its directory: the identity the deletion goes by
	delOpen     bool   // the delete confirmation shows

	updateOpen     bool            // the update modal shows
	updateChecking bool            // a check runs in the background
	updating       bool            // an update runs in the background
	updateAvail    map[string]bool // the skills with an update, by directory
	updateGroups   []updateGroup   // what the check found, grouped by repository and branch
	updateErrs     []string        // what the check could not place
	updateFinished bool            // the update's results show in the modal
	updateGen      int             // the running check or update's generation, to tell its stale result
	updateCancel   context.CancelFunc

	agentView        string // the agent whose skills the Agent 技能 pane shows; "" for the first
	agentSkillSearch string // the text that pane searches by

	packs       []pack // the skill packs the user made
	packsPath   string // where the packs persist; "" in tests
	packSearch  string // the text the 技能包 pane searches by
	packOpen    bool   // the 新建/重命名技能包 modal shows
	packEdit    int    // the pack it renames; -1 for a new one
	packName    string // its name input
	delPack     int    // the pack the delete confirmation asks about
	delPackOpen bool   // the delete confirmation shows
	pickOpen    bool   // the 添加技能 modal shows
	pickPack    int    // the pack it adds to
	pickSearch  string // the text it searches by

	importOpen      bool          // the 导入技能 modal shows
	importMethod    int           // its way: 0 Github, 1 命令行, 2 URL, 3 本地文件, 4 粘贴/新建
	importRepo      string        // the Github way's repository or subdirectory
	importBranch    string        // the Github way's branch
	importCmd       string        // the 命令行 way's command
	importURL       string        // the URL way's link
	importLocalDir  string        // the folder the 本地 way picked; "" for an archive or none
	importLink      bool          // the 本地 way links a picked folder's skills instead of copying
	importName      string        // the 粘贴/新建 way's skill name
	importContent   string        // the 粘贴/新建 way's SKILL.md
	importParsed    []parsedSkill // the skills the last 解析 found
	importClone     string        // the temporary clone of the last 解析
	importCommit    string        // the commit it cloned
	importOrigin    string        // the link the user gave, as of the 解析
	importSource    string        // where the 解析's skills come from: "git" or "local"
	importGitBranch string        // the branch the 解析 tracked, for the import record
	importErr       string        // the failure of the last 解析 or 导入, shown in the modal
	importParsing   bool          // a 解析 runs in the background
	importCancel    context.CancelFunc
	parseGen        int // the running 解析's generation, to tell its stale result
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	// The material shows wherever the view draws no background, where the
	// window shows one: the root draws none then, nor does the sidebar.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	shown := row.Animate("sidebar", b2f(!a.collapsed), 250*time.Millisecond)
	w := sidebarWidth * shown
	row.Children(func() {
		// The sidebar slides out to the left as it hides.
		ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
			side := ui.Column(c).Width(sidebarWidth).Shrink(0).Margin(0, 0, 0, w-sidebarWidth)
			if !vibrant {
				side.Background(t.Surface)
			}
			side.Children(func() { a.sidebar(c, vibrant) })
		})
		if w > 0 {
			ui.Box(c).Width(1).Background(t.Border)
		}
		a.pane(c)
	})

	// At the right edge of the sidebar's top row while it shows, next to
	// the window controls while it hides.
	label := "Hide Sidebar"
	if a.collapsed {
		label = "Show Sidebar"
	}
	toggle := ui.ButtonBase(c.Key("toggle")).Absolute().Left(max(bar.Left+8, w-40)).Top((titleBarHeight-28)/2).Size(32, 28).
		Radius(6).Center().TextColor(t.TextMuted).Label(label).Tooltip(label)
	if toggle.Hovered() {
		toggle.Background(t.SurfaceHover)
	}
	toggle.Children(func() { ui.Icon(c, sidebarIcon).Size(16, 16) })
	if toggle.Clicked() {
		a.toggleSidebar()
	}

	a.addAgentModal(c)
	a.importSkillModal(c)
	a.updateModal(c)
	a.skillViewModal(c)
	a.deleteSkillModal(c)
	a.packNameModal(c)
	a.deletePackModal(c)
	a.pickSkillsModal(c)
}

// sidebar is the items to choose from, under the room the window controls
// take.
func (a *app) sidebar(c *ui.Context, vibrant bool) {
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()
	list := ui.Sidebar(c, &a.selected, func() {
		for _, it := range menuItems {
			a.item(c, it)
		}
	}).Grow(1).Label("Sidebar").FocusBind(&a.focusSide, "sidebar")
	if vibrant {
		list.Background(ui.Transparent)
	}
}

// item builds one row of the sidebar: a colored tile, a label and the
// value at its trailing edge.
func (a *app) item(c *ui.Context, it item) {
	t := c.Theme()
	// The item chosen stays blue, whatever has the focus, in this window
	// or outside it.
	active := a.selected == it.id
	// The sidebar tints the icons of its items with the color of the text,
	// so the item takes an invisible placeholder and the colored tile draws
	// over it, where the placeholder sits.
	e := ui.SidebarItem(c, it.id, spacerIcon, it.label)
	if active {
		e.Background(t.Accent).TextColor(t.AccentText)
	}
	e.Children(func() {
		ui.Image(c, it.icon).Absolute().Left(t.Space(2)).Top(t.Space(1.5)).Size(16, 16)
		if it.detail != "" {
			d := ui.Text(c, it.detail).FontSize(t.FontSize - 1).SingleLine()
			if active {
				d.TextColor(t.AccentText)
			} else {
				d.TextColor(t.TextMuted)
			}
		}
	})
}

// paneHeader is the top row of a pane: the title and the pane's actions.
// While the sidebar hides, the window controls and the sidebar's toggle
// sit over the pane's left edge, so the title starts right of the
// toggle; it starts at the pane's edge otherwise.
func (a *app) paneHeader(c *ui.Context, title string, actions func()) {
	ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).Padding(0, 20, 0, a.headerLead(c.TitleBar())).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(15).Bold().Grow(1).SingleLine()
		if actions != nil {
			actions()
		}
	})
}

// headerLead is the header's left padding: while the sidebar hides, the
// window controls and the sidebar's toggle sit over the pane's left
// edge, so the header starts right of the toggle, which rests at
// bar.Left+8 and is 32 wide.
func (a *app) headerLead(bar ui.TitleBar) float32 {
	if a.collapsed {
		return bar.Left + 48
	}
	return 20
}

// pane is the content area: the page of the item chosen in the sidebar.
func (a *app) pane(c *ui.Context) {
	if a.selected == "settings" {
		a.settings(c)
		return
	}
	if a.selected == "hub" {
		a.hub(c)
		return
	}
	if a.selected == "packs" {
		a.packsPane(c)
		return
	}
	if a.selected == "agent" {
		a.agentSkillsPane(c)
		return
	}
	t := c.Theme()
	label := ""
	for _, it := range menuItems {
		if it.id == a.selected {
			label = it.label
		}
	}
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.paneHeader(c, label, nil)
		ui.Divider(c)
	})
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// toggleSidebar hides or shows the sidebar, from the header's toggle
// button and the View menu alike. The sidebar returns with its focus, and
// with it the background of the item chosen.
func (a *app) toggleSidebar() {
	a.collapsed = !a.collapsed
	if !a.collapsed {
		a.focusSide = "sidebar"
	}
}

// gridColumns picks the columns of the cards' grid from the window's
// width: the layout adapts to the space there is instead of fixing two
// columns at every width (layout.md › Adaptability).
func gridColumns(winWidth float32, collapsed bool) int {
	content := winWidth - 56 // the pane's horizontal padding
	if !collapsed {
		content -= sidebarWidth + 1
	}
	switch {
	case content < 620:
		return 1
	case content < 1010:
		return 2
	default:
		return 3
	}
}

// installMenu puts the app's commands in the menu bar, where macOS
// people look for them (toolbars.md › Desktop): 搜索… focuses the
// current pane's search field, 设置… opens the settings pane, 切换边栏
// does what the header's toggle does.
func (a *app) installMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Aurora", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				if w != nil {
					w.Update(func() { a.selected = "settings" })
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo}, {Role: mygo.RoleRedo},
			mygo.Separator(),
			{Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste},
			{Role: mygo.RolePasteAndMatchStyle}, {Role: mygo.RoleDelete}, {Role: mygo.RoleSelectAll},
			mygo.Separator(),
			{Label: "搜索…", Accelerator: "CmdOrCtrl+F", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				if w != nil {
					w.Update(func() { a.searchFocus = true })
				}
			}},
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "切换边栏", Accelerator: "Control+CmdOrCtrl+S", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				if w != nil {
					w.Update(func() { a.toggleSidebar() })
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

func main() {
	a := &app{selected: "hub", focusSide: "sidebar", packEdit: -1}
	mygo.App.WhenReady(func() {
		// The first start creates the configuration: ~/.aurora/, its
		// skills directory and an empty agents.json.
		if dir, err := ensureConfig(); err == nil {
			a.agentsPath = filepath.Join(dir, "agents.json")
			a.packsPath = filepath.Join(dir, "packs.json")
			a.llmPath = filepath.Join(dir, "llm.json")
			a.skillsDir = filepath.Join(dir, "skills")
			a.loadAgents()
			a.loadLLM()
			a.ensureDefaultAgent()
			a.loadSkills()
			a.loadPacks()
		} else {
			log.Printf("aurora: cannot set up the configuration: %v", err)
		}
		a.installMenu()
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Aurora",
			Width:     1000,
			Height:    640,
			MinWidth:  720,
			MinHeight: 480,
			// Hide the title bar and inset the traffic lights over the
			// sidebar (macOS).
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: titleBarHeight,
			// The material shows wherever the view draws no background
			// (macOS, Windows 11 22H2); ui.Context.Vibrancy tells where it
			// does.
			Vibrancy: mygo.VibrancySidebar,
			// Opens where the user left it last time.
			StateKey: "main",
			Content:  ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
