package ui

import (
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
// and the versions. It is fed by the heartbeat, so it is true on every screen,
// not only on Overview.

const statusBarHeight = float32(24)

func (a *App) newStatusBar() fyne.CanvasObject {
	a.statusState = barText("● "+T("common.connected"), colorSuccess)
	a.statusFacts = barText(a.statusAddress(), colorFaint)
	right := container.NewCenter(barText(T("shell.versions", "prelude", a.catalog.PreludeVersion, "version", appVersion), colorFaint))
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

// barText is the status bar's 11px text.
func barText(text string, c color.Color) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = 11
	return t
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
		a.statusState.Text = "● " + T("common.connected")
		a.statusState.Color = colorSuccess
	} else {
		a.statusState.Text = "● " + T("common.offline")
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
		parts = append(parts, T("shell.instance", "id", shortID(st.Instance)))
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
	parts = append(parts, T("shell.uptime", "duration", humanDuration(st.UptimeS)),
		T("shell.sessions", "busy", st.SessionsActive, "idle", st.SessionsIdle))
	a.statusFacts.Text = strings.Join(parts, "  ·  ")
	a.statusFacts.Refresh()
}
