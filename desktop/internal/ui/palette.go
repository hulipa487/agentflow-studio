package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// paletteCommand is one entry in the Ctrl+K palette.
type paletteCommand struct {
	title    string
	hint     string
	shortcut string
	icon     fyne.Resource
	run      func()
}

// paletteEntry is a search field that hands navigation keys to the palette
// instead of moving the caret.
type paletteEntry struct {
	widget.Entry
	onKey func(*fyne.KeyEvent) bool
}

func newPaletteEntry() *paletteEntry {
	e := &paletteEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *paletteEntry) TypedKey(ev *fyne.KeyEvent) {
	if e.onKey != nil && e.onKey(ev) {
		return
	}
	e.Entry.TypedKey(ev)
}

// paletteCommands lists every destination plus the global actions.
func (a *App) paletteCommands() []paletteCommand {
	var out []paletteCommand
	for _, it := range a.nav {
		out = append(out, paletteCommand{
			title: T("shell.palette.goTo", "screen", it.label), hint: it.hint, shortcut: it.keyLabel, icon: it.icon, run: it.show,
		})
	}
	out = append(out,
		paletteCommand{title: T("shell.newAgent"), hint: T("shell.palette.newAgentHint"), shortcut: "Ctrl+N", icon: theme.ContentAddIcon(), run: a.showNewAgent},
		paletteCommand{title: T("shell.palette.refresh"), hint: T("shell.palette.refreshHint"), shortcut: "F5", icon: theme.ViewRefreshIcon(), run: a.refreshView},
		paletteCommand{title: T("shell.toggleTheme"), hint: T("shell.palette.toggleThemeHint"), icon: theme.ColorPaletteIcon(), run: func() {
			if a.variant == themeDark {
				a.chooseTheme(themeLight)
			} else {
				a.chooseTheme(themeDark)
			}
		}},
		paletteCommand{title: T("shell.toggleRail"), hint: T("shell.palette.toggleRailHint"), icon: theme.MenuIcon(), run: a.toggleRail},
		paletteCommand{title: T("shell.palette.invite"), hint: T("shell.palette.inviteHint"), icon: theme.MailSendIcon(), run: func() {
			a.showUsers()
			a.issueInvite()
		}},
		paletteCommand{title: T("shell.palette.lock"), hint: T("shell.palette.lockHint"), icon: theme.LogoutIcon(), run: a.lock},
	)
	return out
}

func paletteMatch(c paletteCommand, q string) (int, bool) {
	if q == "" {
		return 0, true
	}
	title := strings.ToLower(c.title)
	hint := strings.ToLower(c.hint)
	// "Go to Users" matches "use" by the screen's name, in any language.
	goTo := strings.ToLower(T("shell.palette.goTo", "screen", ""))
	switch {
	case strings.HasPrefix(strings.TrimPrefix(title, goTo), q):
		return 0, true
	case strings.Contains(title, q):
		return 1, true
	case strings.Contains(hint, q):
		return 2, true
	}
	// every query word somewhere in title or hint
	for _, w := range strings.Fields(q) {
		if !strings.Contains(title, w) && !strings.Contains(hint, w) {
			return 0, false
		}
	}
	return 3, true
}

// showCommandPalette opens the Ctrl+K palette: type to filter, Up/Down to
// move, Enter to run, Esc to close. Agents are added once the list loads, so
// "chat assistant" or "edit echo" works too.
func (a *App) showCommandPalette() {
	if a.api == nil || a.win == nil || a.win.Canvas() == nil {
		return
	}
	commands := a.paletteCommands()
	entry := newPaletteEntry()
	entry.SetPlaceHolder(T("shell.palette.placeholder"))
	results := vstack(2)
	selected := 0
	var shown []paletteCommand

	var popup *widget.PopUp
	prevBack := a.backAction
	closePopup := func() {
		if popup != nil {
			popup.Hide()
			popup = nil
		}
		a.backAction = prevBack
	}
	runCmd := func(c paletteCommand) {
		closePopup()
		if c.run != nil {
			c.run()
		}
	}

	var render func()
	render = func() {
		q := strings.ToLower(strings.TrimSpace(entry.Text))
		type ranked struct {
			c    paletteCommand
			rank int
			i    int
		}
		var hits []ranked
		for i, c := range commands {
			if r, ok := paletteMatch(c, q); ok {
				hits = append(hits, ranked{c, r, i})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].rank != hits[j].rank {
				return hits[i].rank < hits[j].rank
			}
			return hits[i].i < hits[j].i
		})
		shown = shown[:0]
		for _, h := range hits {
			shown = append(shown, h.c)
		}
		if len(shown) > 12 {
			shown = shown[:12]
		}
		if selected >= len(shown) {
			selected = len(shown) - 1
		}
		if selected < 0 {
			selected = 0
		}
		clearBox(results)
		if len(shown) == 0 {
			results.Add(container.NewPadded(muted(T("shell.palette.noMatch"))))
		}
		for i, c := range shown {
			c := c
			row := newRailRow(c.title, c.icon, c.shortcut, func() { runCmd(c) })
			row.SetActive(i == selected)
			hint := caption(c.hint)
			hint.Wrapping = fyne.TextWrapOff
			hint.Truncation = fyne.TextTruncateEllipsis
			results.Add(container.NewBorder(nil, nil, nil, nil, vstack(0, row)))
			if i == selected && c.hint != "" {
				results.Add(insetLeading(hint))
			}
		}
		results.Refresh()
	}
	entry.OnChanged = func(string) {
		selected = 0
		render()
	}
	entry.OnSubmitted = func(string) {
		if selected < len(shown) {
			runCmd(shown[selected])
		}
	}
	entry.onKey = func(ev *fyne.KeyEvent) bool {
		switch ev.Name {
		case fyne.KeyDown:
			if selected < len(shown)-1 {
				selected++
				render()
			}
			return true
		case fyne.KeyUp:
			if selected > 0 {
				selected--
				render()
			}
			return true
		case fyne.KeyEscape:
			closePopup()
			return true
		}
		return false
	}
	a.backAction = closePopup
	render()

	body := vstack(spaceS, controlFrame(entry), widthBox(560, results))
	popup = showModalCard(a.win, T("shell.palette.title"), T("shell.palette.subtitle"), body, nil)
	a.win.Canvas().Focus(entry)

	// Agents arrive asynchronously; add "Edit" and "Chat with" entries.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rows, err := a.api.Agents(ctx)
		if err != nil {
			return
		}
		onUI(func() {
			if popup == nil {
				return
			}
			for _, r := range rows {
				name := r.Name
				if r.Spawn {
					continue
				}
				commands = append(commands,
					paletteCommand{title: T("shell.palette.editAgent", "name", name), hint: T("shell.palette.editAgentHint", "name", name), icon: theme.DocumentCreateIcon(),
						run: func() { a.showBuilder(name, nil) }},
					paletteCommand{title: T("shell.palette.chatWith", "name", name), hint: T("shell.palette.chatWithHint", "name", name), icon: theme.MediaPlayIcon(),
						run: func() { a.showPlayground(name) }},
				)
			}
			render()
		})
	}()
}
