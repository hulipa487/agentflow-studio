package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (a *App) showSettings() {
	a.enter("settings")
	a.setMain(a.settingsView())
}

func swatchDot(c color.NRGBA) fyne.CanvasObject {
	circle := canvas.NewCircle(c)
	circle.StrokeColor = colorBorder
	circle.StrokeWidth = 1
	return container.NewGridWrap(fyne.NewSize(14, 14), circle)
}

func (a *App) settingsView() fyne.CanvasObject {
	var syncAppearanceUI func()

	lightBtn := actionButton(T("settings.modeLight"), lightModeIcon, func() {
		a.chooseTheme(themeLight)
		syncAppearanceUI()
	})
	darkBtn := actionButton(T("settings.modeDark"), darkModeIcon, func() {
		a.chooseTheme(themeDark)
		syncAppearanceUI()
	})

	transBtn := actionButton(paletteTrans.Title(), checkMarkIcon, func() {
		a.choosePalette(paletteTrans)
		syncAppearanceUI()
	})
	obsidianBtn := actionButton(paletteObsidian.Title(), checkMarkIcon, func() {
		a.choosePalette(paletteObsidian)
		syncAppearanceUI()
	})

	activeSummary := oneLine("")

	syncAppearanceUI = func() {
		if a.variant == themeLight {
			lightBtn.Importance = widget.HighImportance
			darkBtn.Importance = widget.LowImportance
		} else {
			darkBtn.Importance = widget.HighImportance
			lightBtn.Importance = widget.LowImportance
		}
		lightBtn.Refresh()
		darkBtn.Refresh()

		if a.palette == paletteTrans {
			transBtn.Importance = widget.HighImportance
			transBtn.SetText(T("settings.active"))
			transBtn.SetIcon(checkMarkIcon)
			obsidianBtn.Importance = widget.MediumImportance
			obsidianBtn.SetText(T("settings.usePalette", "name", paletteObsidian.Title()))
			obsidianBtn.SetIcon(nil)
		} else {
			obsidianBtn.Importance = widget.HighImportance
			obsidianBtn.SetText(T("settings.active"))
			obsidianBtn.SetIcon(checkMarkIcon)
			transBtn.Importance = widget.MediumImportance
			transBtn.SetText(T("settings.usePalette", "name", paletteTrans.Title()))
			transBtn.SetIcon(nil)
		}
		transBtn.Refresh()
		obsidianBtn.Refresh()

		activeSummary.SetText(T("settings.themeSummary", "palette", a.palette.Title(), "mode", a.variant.String()))
	}
	syncAppearanceUI()

	themeCard := func(p themePalette, sw1, sw2, sw3 color.NRGBA, btn *widget.Button) fyne.CanvasObject {
		swatches := hstack(spaceXS,
			vCenter(swatchDot(sw1)),
			vCenter(swatchDot(sw2)),
			vCenter(swatchDot(sw3)),
		)
		top := container.NewBorder(nil, nil,
			hstack(spaceS, vCenter(subheading(p.Title())), vCenter(swatches)),
			vCenter(widthBox(140, btn)),
			nil,
		)
		return subpanel(vstack(spaceXS, top, muted(p.Description())))
	}

	transCard := themeCard(
		paletteTrans,
		flagBlue,
		flagPink,
		color.NRGBA{0x13, 0x1C, 0x2B, 0xFF},
		transBtn,
	)
	obsidianCard := themeCard(
		paletteObsidian,
		obsidianViolet,
		obsidianTeal,
		color.NRGBA{0x12, 0x15, 0x21, 0xFF},
		obsidianBtn,
	)

	paletteGrid := grid(2, transCard, obsidianCard)
	modePicker := hstack(spaceS,
		widthBox(132, controlFrame(lightBtn)),
		widthBox(132, controlFrame(darkBtn)),
	)

	appearanceBody := vstack(spaceM,
		vstack(spaceXS, fieldLabel(T("settings.colorTheme")), paletteGrid),
		vstack(spaceXS, fieldLabel(T("settings.appearanceMode")), modePicker),
	)
	appearance := sectionAction(T("settings.appearance"),
		T("settings.appearanceHint"),
		chip(T("settings.savedAutomatically"), toneAccent),
		appearanceBody,
	)

	languageBody := vstack(spaceS, muted(T("settings.languageHintDesktop")), a.languageChoices())
	if cjkFontMissing() {
		languageBody.Add(messageCard(T("settings.cjkFontMissing"), toneWarning))
	}
	language := section(T("common.language"), "", languageBody)

	transport := "TCP"
	if a.api != nil && a.api.Local() {
		transport = T("settings.transportLocal")
	}
	endpoint := ""
	if a.api != nil {
		endpoint = a.api.DisplayURL
	}
	fact := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.New(settingsFactRow{}, fieldLabel(label), value)
	}
	connection := sectionAction(T("settings.connection"),
		T("settings.connectionHint"),
		chip(T("common.connected"), toneSuccess),
		vstack(spaceS,
			fact(T("settings.endpoint"), monoInline(endpoint)),
			fact(T("settings.transport"), widget.NewLabel(transport)),
			vSpacer(spaceXS),
			hstack(spaceS, widthBox(132, actionButton(T("settings.lock"), nil, a.lock))),
		),
	)

	shortcutRow := func(keys, desc string) fyne.CanvasObject {
		// The keys get one column wide enough for the longest chord, so every
		// description starts on the same line.
		return subpanel(container.NewBorder(nil, nil,
			widthBox(104, vLeft(onLine(chip(keys, toneNeutral)))),
			nil,
			vLeft(widget.NewLabel(desc)),
		))
	}
	shortcuts := section(T("settings.shortcuts"),
		T("settings.shortcutsHint"),
		grid(2,
			shortcutRow("Ctrl+N", T("settings.shortcutNewAgent")),
			shortcutRow("Ctrl+1 … 6", T("settings.shortcutViewsDesktop")),
			shortcutRow("Ctrl+S", T("settings.shortcutSaveDesktop")),
			shortcutRow("Ctrl+Enter", T("settings.shortcutSend")),
			shortcutRow("Ctrl+Space", T("settings.shortcutCompletion")),
			shortcutRow("F1 / Esc", T("settings.shortcutDocsBack")),
		),
	)

	about := section(T("settings.about"),
		"",
		vstack(spaceS,
			fact(T("settings.studioVersion"), monoInline(appVersion)),
			fact(T("settings.runtimeApi"), widget.NewLabel(T("settings.adminApi"))),
			fact(T("settings.activeTheme"), activeSummary),
		),
	)

	body := scrollInset(pageScroll(vstack(spaceM, appearance, language, connection, shortcuts, about)))
	head := pageHeader(T("nav.settings"), T("settings.subtitle"), nil)
	return container.NewBorder(
		vstack(0, pageInset(head), widget.NewSeparator()),
		nil, nil, nil, body,
	)
}

// settingsFactRow is a two-column fact line: a fixed caption column and a
// value that fills the rest, both vertically centred in the row.
type settingsFactRow struct{}

const settingsFactLabelWidth = float32(130)

func (settingsFactRow) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 2 {
		return fyne.Size{}
	}
	lm, vm := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(settingsFactLabelWidth+spaceS+vm.Width, fyne.Max(lm.Height, vm.Height)+4)
}

func (settingsFactRow) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) < 2 {
		return
	}
	lm, vm := objs[0].MinSize(), objs[1].MinSize()
	w := size.Width - settingsFactLabelWidth - spaceS
	if w < 1 {
		w = 1
	}
	objs[0].Resize(fyne.NewSize(settingsFactLabelWidth, lm.Height))
	objs[0].Move(fyne.NewPos(0, (size.Height-lm.Height)/2))
	objs[1].Resize(fyne.NewSize(w, vm.Height))
	objs[1].Move(fyne.NewPos(settingsFactLabelWidth+spaceS, (size.Height-vm.Height)/2))
}
