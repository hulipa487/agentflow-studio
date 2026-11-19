package ui

import (
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// The dock holds the run panel or the Luau reference, never both; closing it
// gives the editor the whole width, and the choice outlives the builder.
func TestBuilderDockFlipsAndIsRemembered(t *testing.T) {
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", filepath.Join(t.TempDir(), "studio.json"))
	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("builder")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	a.content = container.NewStack()
	a.catalog = api.Catalog{Models: []string{"default"}, SafetyProfiles: []string{"default"}}
	b := a.newBuilderView("echo", &api.AgentDetail{Agent: api.AgentJSON{Loop: "./echo.lua"}, Loop: "-- af-prelude-version: 1\nfunction loop() end\n"}, nil)
	a.setMain(b.root)
	b.claimActions()
	win.Resize(fyne.NewSize(1200, 760))
	ui.settle(50 * time.Millisecond)

	if b.dock != "" || b.body.Objects[0] != b.editor {
		t.Fatalf("a fresh config should open with the dock closed; dock=%q", b.dock)
	}

	a.helpAction()
	if b.dock != "docs" || !b.docsPane.Visible() || b.runPane.Visible() {
		t.Fatalf("F1 should open the reference alone; dock=%q", b.dock)
	}
	if !b.docsBtn.active || b.runBtn.active {
		t.Fatal("the Docs toggle should read pressed, Run not")
	}

	b.toggleDock("run")
	if b.dock != "run" || !b.runPane.Visible() || b.docsPane.Visible() {
		t.Fatalf("Run should replace the reference in the dock; dock=%q", b.dock)
	}

	saved := appconfig.Load()
	if saved.BuilderDock != "run" {
		t.Fatalf("the dock choice was not saved; got %q", saved.BuilderDock)
	}

	b.toggleDock("run")
	if b.dock != "" || b.body.Objects[0] != b.editor {
		t.Fatalf("a second Run should close the dock; dock=%q", b.dock)
	}

	// A new builder opens the way the last one was left.
	a.cfg.BuilderDock = "docs"
	again := a.newBuilderView("echo", &api.AgentDetail{Agent: api.AgentJSON{Loop: "./echo.lua"}}, nil)
	if again.dock != "docs" {
		t.Fatalf("a new builder should reopen the remembered dock; got %q", again.dock)
	}
}

// The Apply button runs its remembered choice; a pick from its menu changes it.
func TestBuilderApplyChoiceIsRemembered(t *testing.T) {
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", filepath.Join(t.TempDir(), "studio.json"))
	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("builder")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark", BuilderPersist: true})
	a.content = container.NewStack()
	a.catalog = api.Catalog{Models: []string{"default"}, SafetyProfiles: []string{"default"}}
	b := a.newBuilderView("", nil, &api.Template{Name: "echo", Loop: "-- af-prelude-version: 1\nfunction loop() end\n"})
	a.setMain(b.root)
	ui.settle(50 * time.Millisecond)

	if b.applyBtn.Text != "Apply & save" {
		t.Fatalf("the saved choice should label the button; got %q", b.applyBtn.Text)
	}
	// An invalid name stops the apply before any request, so no runtime is needed.
	b.nameEntry.SetText("")
	b.choosePersist(false)
	if b.applyBtn.Text != "Apply" || b.persist {
		t.Fatalf("picking Apply should make it the choice; label=%q persist=%v", b.applyBtn.Text, b.persist)
	}
	saved := appconfig.Load()
	if saved.BuilderPersist {
		t.Fatal("the Apply choice was not saved")
	}
}
