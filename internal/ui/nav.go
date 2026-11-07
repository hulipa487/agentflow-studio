package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
)

// navItem is one destination in the shell. The rail, the breadcrumb, the
// keyboard shortcuts and the command palette are all generated from the same
// list, so adding a screen is one entry here and its show function.
type navItem struct {
	id      string
	label   string
	section string
	icon    fyne.Resource
	// key is the Ctrl+<key> shortcut; empty for none. keyLabel is what the
	// rail and the palette print for it.
	key      fyne.KeyName
	keyLabel string
	// hint is the palette's one-line description.
	hint string
	show func()
	// also lists other view ids that highlight this row (builder → agents).
	also []string
	row  *railRow
}

// navSections fixes the rail's grouping and order.
var navSections = []string{"Home", "Build", "Connect", "Govern", "Operate", "Studio"}

func (a *App) buildNav() []*navItem {
	ctrl := func(n int) (fyne.KeyName, string) {
		keys := []fyne.KeyName{fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5, fyne.Key6, fyne.Key7, fyne.Key8, fyne.Key9}
		return keys[n-1], fmt.Sprintf("Ctrl+%d", n)
	}
	item := func(id, label, section string, icon fyne.Resource, n int, hint string, show func(), also ...string) *navItem {
		it := &navItem{id: id, label: label, section: section, icon: icon, hint: hint, show: show, also: also}
		if n > 0 {
			it.key, it.keyLabel = ctrl(n)
		}
		return it
	}
	items := []*navItem{
		item("overview", "Overview", "Home", theme.HomeIcon(), 1,
			"Instance health, live sessions, agents, channels and counters", a.showOverview),
		item("agents", "Agents", "Build", theme.AccountIcon(), 2,
			"Configured agents; create, edit, apply and delete", a.showAgents, "builder"),
		item("playground", "Playground", "Build", theme.MediaPlayIcon(), 3,
			"Chat with an agent and watch the live trace", func() { a.showPlayground("") }),
		item("channels", "Channels", "Connect", theme.StorageIcon(), 4,
			"Webhook, Telegram, GitHub and bridge channels", a.showChannels),
		item("models", "Models", "Connect", theme.ListIcon(), 5,
			"LLM endpoints: add, test, persist or revert", a.showModels),
		item("credentials", "Credentials", "Connect", theme.LoginIcon(), 6,
			"Encrypted secrets, engine-wide or per user", a.showCredentials),
		item("safety", "Safety", "Govern", theme.WarningIcon(), 7,
			"Safety profiles and model-backed classifiers", a.showSafety),
		item("users", "Users", "Govern", iconUser, 8,
			"Profiles, linked handles, quotas, model layer and invites", a.showUsers),
		item("tenancy", "Tenancy", "Govern", theme.GridIcon(), 0,
			"Groups, members, shared memory pools and grants", a.showTenancy),
		item("usage", "Usage", "Operate", theme.InfoIcon(), 0,
			"Token ledger per tenant, agent and model for a day", a.showUsage),
		item("logs", "Logs", "Operate", theme.FileTextIcon(), 9,
			"Live runtime log tail with filters", a.showLogs),
		item("config", "Config", "Operate", theme.DocumentIcon(), 0,
			"Raw config editor with boot-equivalent validation", a.showConfig),
		item("docs", "Documentation", "Studio", theme.HelpIcon(), 0,
			"Bundled AgentFlow guides and reference", a.openDocs),
		item("settings", "Settings", "Studio", theme.SettingsIcon(), 0,
			"Appearance and connection", a.showSettings),
	}
	for _, it := range items {
		if it.id == "docs" {
			it.keyLabel = "F1"
		}
	}
	return items
}

// navByID returns the destination with that id (or the one that also
// highlights for it), or nil.
func (a *App) navByID(id string) *navItem {
	for _, it := range a.nav {
		if it.id == id {
			return it
		}
		for _, other := range it.also {
			if other == id {
				return it
			}
		}
	}
	return nil
}

// enter is the common prologue of every screen: it releases what the previous
// screen held (language server, chat stream, polling), resets the keyboard
// actions, and updates the rail and breadcrumb. Esc returns to Overview from
// anywhere but Overview itself.
func (a *App) enter(view string) {
	a.closeLuaEditor()
	a.cancelChat()
	a.cancelView()
	a.clearActions()
	a.setActive(view)
	if it := a.navByID(view); it != nil {
		if it.id == view {
			a.setBreadcrumb(it.section + " / " + it.label)
		} else {
			a.setBreadcrumb(it.label)
		}
	}
	if view != "overview" {
		a.backAction = a.showOverview
	}
}

// viewContext returns a context that lives until the next screen is entered.
// Polling loops and streams started by a screen use it so they stop on their
// own when the operator navigates away.
func (a *App) viewContext() context.Context {
	a.cancelView()
	ctx, cancel := context.WithCancel(context.Background())
	a.viewCancel = cancel
	return ctx
}

func (a *App) cancelView() {
	if a.viewCancel != nil {
		a.viewCancel()
		a.viewCancel = nil
	}
}

func (a *App) setActive(view string) {
	a.view = view
	active := a.navByID(view)
	for _, it := range a.nav {
		if it.row != nil {
			it.row.SetActive(it == active)
		}
	}
	if active != nil {
		a.revealRailRow(active.row)
	}
}

// revealRailRow scrolls the rail so the active row is on screen; on a short
// window the lower sections would otherwise hide where you are.
func (a *App) revealRailRow(row *railRow) {
	if row == nil || a.navScroll == nil {
		return
	}
	y := row.Position().Y
	h := row.Size().Height
	view := a.navScroll.Size().Height
	off := a.navScroll.Offset.Y
	switch {
	case view <= 0:
		return
	case y < off:
		a.navScroll.ScrollToOffset(fyne.NewPos(0, y))
	case y+h > off+view:
		a.navScroll.ScrollToOffset(fyne.NewPos(0, y+h-view))
	}
}

func (a *App) installShortcuts() {
	c := a.win.Canvas()
	add := func(key fyne.KeyName, mod fyne.KeyModifier, fn func()) {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: key, Modifier: mod}, func(fyne.Shortcut) {
			if a.api != nil {
				fn()
			}
		})
	}
	for _, it := range a.nav {
		if it.key == "" {
			continue
		}
		show := it.show
		add(it.key, fyne.KeyModifierControl, show)
	}
	add(fyne.KeyN, fyne.KeyModifierControl, a.showNewAgent)
	add(fyne.KeyK, fyne.KeyModifierControl, a.showCommandPalette)
	add(fyne.KeyP, fyne.KeyModifierControl|fyne.KeyModifierShift, a.showCommandPalette)
	add(fyne.KeyF1, 0, a.openDocs)
	add(fyne.KeyF5, 0, a.refreshView)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if a.saveAction != nil {
			a.saveAction()
		}
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if a.sendAction != nil {
			a.sendAction()
		}
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeySpace, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if a.luaEditor != nil && a.luaEditor.focused() {
			a.luaEditor.editor.RequestCompletion(true)
		}
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape}, func(fyne.Shortcut) {
		if a.backAction != nil {
			a.backAction()
		}
	})
}

// refreshView re-enters the current screen (F5). Screens with unsaved state
// (builder, config editor) register their own refresh instead.
func (a *App) refreshView() {
	if a.refreshAction != nil {
		a.refreshAction()
		return
	}
	if it := a.navByID(a.view); it != nil && it.id == a.view {
		it.show()
	}
}
