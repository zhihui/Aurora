package main

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

// The icons of the sidebar's items: colored rounded squares with the
// glyphs of Lucide (icons/*.svg, ISC license, see icons/LICENSE.lucide).
// Their colors are their own, so the sidebar's choice of the text color,
// blue on the item chosen, leaves them as they are.
var (
	//go:embed icons/hub.svg
	hubSVG []byte
	//go:embed icons/packs.svg
	packsSVG []byte
	//go:embed icons/agent.svg
	agentSVG []byte
	//go:embed icons/settings.svg
	settingsSVG []byte

	hubIcon      = ui.MustParseSVG(hubSVG)
	packsIcon    = ui.MustParseSVG(packsSVG)
	agentIcon    = ui.MustParseSVG(agentSVG)
	settingsIcon = ui.MustParseSVG(settingsSVG)

	// spacerIcon holds the place of an item's icon, which the sidebar
	// tints; nothing of it shows, and the colored tile draws over it.
	spacerIcon = tile(``)

	sidebarIcon = tile(`<rect x="1.5" y="2.5" width="13" height="11" rx="2" fill="none" stroke="currentColor" stroke-width="1.3"/>` +
		`<path d="M6 2.5v11" fill="none" stroke="currentColor" stroke-width="1.3"/>`)

	xIcon = tile(`<path d="M4 4l8 8M12 4l-8 8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>`)

	searchIcon = tile(`<circle cx="6.5" cy="6.5" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M10 10l4 4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>`)

	// The icons of the 导入技能 modal's ways: Lucide glyphs tinted with
	// the text color (ISC license, see icons/LICENSE.lucide).
	githubIcon = lucide(`<path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4"/>` +
		`<path d="M9 18c-4.51 2-5-2-7-2"/>`)
	terminalIcon = lucide(`<polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/>`)
	linkIcon     = lucide(`<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>` +
		`<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>`)
	folderIcon    = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	clipboardIcon = lucide(`<rect width="8" height="4" x="8" y="2" rx="1" ry="1"/>` +
		`<path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/>`)
	checkCircleIcon = lucide(`<circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/>`)
	pencilIcon      = lucide(`<path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/>`)
	trashIcon       = lucide(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>` +
		`<line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/>`)
	plusIcon   = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	unlinkIcon = lucide(`<path d="m18.84 12.25 1.72-1.71a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>` +
		`<path d="m5.17 11.75-1.72 1.71a5 5 0 0 0 7.07 7.07l1.72-1.71"/>` +
		`<line x1="8" x2="8" y1="2" y2="5"/><line x1="2" x2="5" y1="8" y2="8"/>` +
		`<line x1="16" x2="16" y1="19" y2="22"/><line x1="19" x2="22" y1="16" y2="16"/>`)
	downloadIcon = lucide(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>` +
		`<polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>`)
	// translateIcon is the languages glyph of the viewer's 中文 tab and
	// the 翻译 button's empty state.
	translateIcon = lucide(`<path d="m5 8 6 6"/><path d="m4 14 6-6 2-3"/><path d="M2 5h12"/><path d="M7 2h1"/>` +
		`<path d="m22 22-5-10-5 10"/><path d="M14 18h6"/>`)
	checkIcon = lucide(`<polyline points="20 6 9 17 4 12"/>`)
	// circleIcon pairs with checkIcon in the toggle capsules: the empty
	// circle of the unchosen state, so neither state is a blank space.
	circleIcon = lucide(`<circle cx="12" cy="12" r="10"/>`)
	// Filled dots read better than stroked rings at this size.
	ellipsisIcon = tile(`<circle cx="3" cy="8" r="1.3" fill="currentColor"/>` +
		`<circle cx="8" cy="8" r="1.3" fill="currentColor"/>` +
		`<circle cx="13" cy="8" r="1.3" fill="currentColor"/>`)
	alertIcon = lucide(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/>` +
		`<path d="M12 9v4"/><path d="M12 17h.01"/>`)
)

func tile(svg string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">` + svg + `</svg>`))
}

// lucide wraps Lucide's 24×24 stroke paths into a 16×16 icon tinted
// with the text color.
func lucide(paths string) *ui.SVG {
	return tile(`<g transform="scale(0.6667)" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</g>`)
}
