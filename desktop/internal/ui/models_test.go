package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// The empty Models screen owns the add action: the header button is hidden so
// the same action does not appear twice.
func TestEmptyModelsScreenHidesHeaderAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/api/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": []api.Model{}})
	}))
	defer srv.Close()

	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("models")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	a.api = api.New(srv.URL, "token")
	a.content = container.NewStack()

	v := a.newModelsView()
	ui.waitFor(3*time.Second, func() bool {
		labels, _ := collectLabelsAndButtons(v.list)
		return v.meta.Text != "loading…" && !v.newBtn.Visible() && containsString(labels, "No models yet")
	})
	if v.meta.Text != "all changes saved to config" {
		t.Fatalf("meta = %q", v.meta.Text)
	}
	if v.count.Text != "0 models" {
		t.Fatalf("count = %q", v.count.Text)
	}
	if v.newBtn.Visible() {
		t.Fatal("header New model button should be hidden while the list is empty")
	}
	labels, buttons := collectLabelsAndButtons(v.list)
	if !containsString(labels, "No models yet") {
		t.Fatalf("empty title missing; labels=%v", labels)
	}
	if !containsString(buttons, "Add a model") {
		t.Fatalf("empty action missing; buttons=%v", buttons)
	}
}
