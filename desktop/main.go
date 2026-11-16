package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	studiodocs "agentflow-studio-docs"

	"agentflow-studio/internal/appconfig"
	"agentflow-studio/internal/ui"
)

func main() {
	if !acquireSingleInstance() {
		return
	}
	defer releaseSingleInstance()

	fyneApp := app.NewWithID("io.agentflow.studio")
	win := fyneApp.NewWindow("AgentFlow Studio")
	win.Resize(fyne.NewSize(1280, 860))
	win.CenterOnScreen()

	studio := ui.New(fyneApp, win, appconfig.Load())
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
