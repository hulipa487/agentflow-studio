package ui

import (
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"

	"agentflow-studio/internal/appconfig"
	"agentflow-studio/internal/i18n"
)

// T reads a message from the catalogues in the current language. Parameters
// are name, value pairs: T("common.required", "what", name).
func T(key string, args ...any) string { return i18n.T(key, args...) }

// SystemLocales lists the operating system's preferred locales, most
// preferred first, for the first launch: the POSIX variables where they are
// set, then what Fyne reads from the OS.
func SystemLocales() []string {
	var out []string
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			out = append(out, v)
			break
		}
	}
	return append(out, lang.SystemLocale().String())
}

// languageFromConfig is the saved language, or English when none (or an
// unknown one) is saved. Following the OS on a first launch is main's job, so
// a test building the app never depends on the machine it runs on.
func languageFromConfig(saved string) i18n.Lang {
	l, _ := i18n.Parse(saved)
	return l
}

// chooseLanguage switches the language in place and saves the choice. Text is
// built into the widgets, so the current view is built again in the new
// language; a screen holding unsaved edits is asked about first.
func (a *App) chooseLanguage(next i18n.Lang) {
	if i18n.Language() == next {
		return
	}
	apply := func() {
		i18n.SetLanguage(next)
		a.cfg.Language = string(next)
		_ = appconfig.Save(a.cfg)
		// The theme picks the font by language; setting it again makes Fyne
		// drop its font cache.
		a.fyneApp.Settings().SetTheme(studioTheme{})
		measureLines()
		a.relocalize()
	}
	if a.unsaved != nil && a.unsaved() {
		showConfirmPrompt(a.win, T("shell.languageDiscardTitle"), T("shell.languageDiscardMessage", "language", next.Name()),
			T("shell.languageDiscardConfirm"), true, func(ok bool) {
				if ok {
					apply()
				}
			})
		return
	}
	apply()
}

// relocalize builds the current view again after a language change.
func (a *App) relocalize() {
	if a.win != nil {
		a.win.SetTitle(T("shell.windowTitle"))
	}
	if a.relocalizeView != nil {
		// A view outside the shell (login) rebuilds itself, keeping what was
		// typed.
		a.relocalizeView()
		return
	}
	if a.shell == nil {
		return
	}
	view, reshow := a.view, a.reshow
	size := a.win.Canvas().Size()
	a.buildShell()
	a.win.Resize(size)
	switch {
	case reshow != nil:
		reshow()
	case a.navByID(view) != nil && a.navByID(view).id == view:
		a.navByID(view).show()
	default:
		a.showOverview()
	}
}

// languageToggle is the EN / 简体 / 繁體 switch: three cells, the current one
// filled. Each language is written in its own script whatever the current
// language is, so the Chinese labels carry their own font.
func (a *App) languageToggle() fyne.CanvasObject {
	const cellWidth, cellHeight = float32(42), float32(24)
	cells := make([]fyne.CanvasObject, 0, len(i18n.Langs))
	for _, l := range i18n.Langs {
		l := l
		active := l == i18n.Language()
		var ink color.Color = colorMuted
		if active {
			ink = colorText
		}
		label := canvas.NewText(l.Label(), ink)
		label.TextSize = 12
		label.Alignment = fyne.TextAlignCenter
		label.TextStyle = fyne.TextStyle{Bold: active}
		if l.Chinese() {
			label.FontSource = cjkFont(l, active)
		}
		var fill func() color.Color
		if active {
			fill = func() color.Color { return colorSurface2 }
		}
		cell := newTapSurface(container.NewCenter(label), 4, false, fill, func() { a.chooseLanguage(l) })
		cell.height = cellHeight
		cells = append(cells, widthBox(cellWidth, cell))
	}
	frame := canvas.NewRectangle(colorBG)
	frame.CornerRadius = 6
	frame.StrokeColor = colorBorder
	frame.StrokeWidth = 1
	return container.NewStack(frame,
		container.New(layout.NewCustomPaddedLayout(2, 2, 2, 2), hstack(2, cells...)))
}

// languageToggleWidth is what languageToggle measures, for the command bar's
// width budget.
const languageToggleWidth = float32(3*42 + 2*2 + 2*2)

// cjkFontMissing reports whether the current language needs a CJK font the
// system does not have.
func cjkFontMissing() bool {
	l := i18n.Language()
	return l.Chinese() && cjkFont(l, false) == nil
}

// languageChoices is the Settings screen's language picker: one button per
// language, each showing the language's full name in its own script (and
// font), the current one filled.
func (a *App) languageChoices() fyne.CanvasObject {
	const cellWidth = float32(120)
	cells := make([]fyne.CanvasObject, 0, len(i18n.Langs))
	for _, l := range i18n.Langs {
		l := l
		active := l == i18n.Language()
		var ink color.Color = colorMuted
		if active {
			ink = colorText
		}
		label := canvas.NewText(l.Name(), ink)
		label.Alignment = fyne.TextAlignCenter
		label.TextStyle = fyne.TextStyle{Bold: active}
		if l.Chinese() {
			label.FontSource = cjkFont(l, active)
		}
		var fill func() color.Color
		if active {
			fill = func() color.Color { return colorSurface2 }
		}
		cell := newTapSurface(container.NewCenter(label), 6, true, fill, func() { a.chooseLanguage(l) })
		cell.height = controlHeight
		cells = append(cells, widthBox(cellWidth, cell))
	}
	return hstack(spaceS, cells...)
}
