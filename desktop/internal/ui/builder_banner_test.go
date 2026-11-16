package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

func TestBuilderShowsValidationError(t *testing.T) {
	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("builder")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	a.content = container.NewStack()
	a.catalog = api.Catalog{Models: []string{"default"}, SafetyProfiles: []string{"default"}}
	b := a.newBuilderView("", nil, &api.Template{Name: "echo", Loop: "-- af-prelude-version: 1\nfunction loop() end\n"})
	a.setMain(b.root)
	win.Resize(fyne.NewSize(1100, 760))
	// The loop editor's language server reports its first diagnostics from its
	// own goroutine; run them here before driving the form.
	ui.settle(50 * time.Millisecond)
	b.nameEntry.SetText("")
	b.validate()
	if len(b.messages.Objects) == 0 {
		t.Fatal("validation error message was not added")
	}
	labels, _ := collectLabelsAndButtons(b.messages)
	if !containsString(labels, "An agent name is required.") {
		t.Fatalf("validation error text missing; labels=%v", labels)
	}
	if b.messages.Size().Height <= 0 {
		t.Fatalf("validation message slot has zero height: %v", b.messages.Size())
	}
}
