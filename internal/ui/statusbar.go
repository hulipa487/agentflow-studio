package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"agentflow-studio/internal/api"
)

// The status bar is the shell's bottom band: whether the runtime answers,
// which runtime it is (address, instance, region, version, uptime, sessions)
// and the three shortcuts worth remembering. It is fed by the heartbeat, so it
// is true on every screen, not only on Overview.

const statusBarHeight = float32(24)

func (a *App) newStatusBar() fyne.CanvasObject {
	a.statusState = monoText("● connected", 11, colorSuccess)
	a.statusFacts = monoText(a.statusAddress(), 11, colorMuted)

	hint := func(key, what string) fyne.CanvasObject {
		return hstack(spaceXS, container.NewCenter(kbd(key)), container.NewCenter(monoText(what, 11, colorFaint)))
	}
	right := hstack(spaceM,
		hint("Ctrl+K", "commands"),
		hint("F1", "docs"),
		hint("F5", "refresh"),
		container.NewCenter(monoText(fmt.Sprintf("prelude v%d · studio %s", a.catalog.PreludeVersion, appVersion), 11, colorFaint)),
	)
	left := hstack(spaceM, container.NewCenter(a.statusState), container.NewCenter(a.statusFacts))

	bg := canvas.NewRectangle(colorFloat)
	edge := canvas.NewRectangle(colorBorder)
	edge.SetMinSize(fyne.NewSize(0, 1))
	height := canvas.NewRectangle(color.Transparent)
	height.SetMinSize(fyne.NewSize(0, statusBarHeight))
	row := container.New(layout.NewCustomPaddedLayout(0, 0, spaceM, spaceM),
		container.NewBorder(nil, nil, left, right, nil))
	return container.NewStack(bg, height, container.NewBorder(edge, nil, nil, nil, row))
}

func (a *App) statusAddress() string {
	if a.api == nil {
		return ""
	}
	return a.api.DisplayURL
}

func (a *App) setStatusBarState(online bool) {
	if a.statusState == nil {
		return
	}
	if online {
		a.statusState.Text = "● connected"
		a.statusState.Color = colorSuccess
	} else {
		a.statusState.Text = "● offline"
		a.statusState.Color = colorDanger
	}
	a.statusState.Refresh()
}

// setRuntimeFacts writes the runtime's identity line from a state snapshot.
func (a *App) setRuntimeFacts(st api.State) {
	if a.statusFacts == nil {
		return
	}
	parts := []string{a.statusAddress()}
	if st.Instance != "" {
		parts = append(parts, "instance "+shortID(st.Instance))
	}
	if st.Region != "" {
		parts = append(parts, st.Region)
	}
	if st.Topology != "" {
		parts = append(parts, st.Topology)
	}
	if st.Version != "" {
		parts = append(parts, st.Version)
	}
	parts = append(parts, "up "+humanDuration(st.UptimeS),
		fmt.Sprintf("%d busy / %d idle", st.SessionsActive, st.SessionsIdle))
	a.statusFacts.Text = strings.Join(parts, "  ·  ")
	a.statusFacts.Refresh()
}
