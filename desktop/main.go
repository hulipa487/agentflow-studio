package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	studiodocs "agentflow-studio-docs"

	"agentflow-studio/internal/appconfig"
	"agentflow-studio/internal/i18n"
	"agentflow-studio/internal/ui"
)

func main() {
	if !acquireSingleInstance() {
		return
	}
	defer releaseSingleInstance()

	fyneApp := app.NewWithID("io.agentflow.studio")
	// The title is the product name in every language; the borderless frame
	// on Windows finds the window by it.
	win := fyneApp.NewWindow(i18n.T("shell.windowTitle"))
	win.Resize(fyne.NewSize(1280, 860))
	win.CenterOnScreen()

	cfg := appconfig.Load()
	if cfg.Language == "" {
		// First launch: follow the OS. Nothing is saved until the operator
		// picks a language, so a later change of the OS language is followed
		// too.
		cfg.Language = string(i18n.Detect(ui.SystemLocales()...))
	}
	studio := ui.New(fyneApp, win, cfg)
	if err := studio.SetDocs(studiodocs.FS); err != nil {
		println("documentation:", err.Error())
	}
	studio.Start()
	win.Show()

	// On Windows this keeps the native resize frame but hides it, and installs
	// hit-testing for edges, the drag bar and the window buttons. Elsewhere it
	// is a no-op.
	ui.MakeBorderless(win.Title())

	fyneApp.Run()
}
