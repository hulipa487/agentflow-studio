package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
	"agentflow-studio/internal/i18n"
)

// shownText is every piece of text drawn under obj, widgets' insides included.
func shownText(obj fyne.CanvasObject) []string {
	var out []string
	for _, o := range test.LaidOutObjects(obj) {
		switch x := o.(type) {
		case *canvas.Text:
			out = append(out, x.Text)
		case *widget.Label:
			out = append(out, x.Text)
		case *widget.Button:
			out = append(out, x.Text)
		}
	}
	return out
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestLanguageSwitchRebuildsInPlaceAndPersists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", cfgPath)

	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("test")
	win.Resize(fyne.NewSize(1280, 860))

	a := New(fyneApp, win, appconfig.Config{})
	defer i18n.SetLanguage(i18n.En)
	a.api = api.New("http://127.0.0.1:1", "tok")
	a.catalog = api.Catalog{PreludeVersion: 1}
	a.showMain()
	ui.settle(150 * time.Millisecond)
	a.showSettings()
	ui.settle(150 * time.Millisecond)

	before := a.shell
	a.chooseLanguage(i18n.ZhHant)
	ui.settle(150 * time.Millisecond)

	if i18n.Language() != i18n.ZhHant {
		t.Fatalf("language = %s after choosing zh-Hant", i18n.Language())
	}
	if a.shell == before {
		t.Error("the shell was not rebuilt in the new language")
	}
	if a.view != "settings" {
		t.Errorf("view = %q after the switch, want settings", a.view)
	}
	han := 0
	for _, s := range shownText(win.Content()) {
		if hasHan(s) {
			han++
		}
	}
	if han < 10 {
		t.Errorf("only %d pieces of Chinese text on screen after switching to zh-Hant", han)
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved appconfig.Config
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Language != "zh-Hant" {
		t.Errorf("saved language = %q, want zh-Hant", saved.Language)
	}

	// Back in English, the only Chinese left is the switch's own labels.
	a.chooseLanguage(i18n.En)
	ui.settle(150 * time.Millisecond)
	for _, s := range shownText(win.Content()) {
		if hasHan(s) && s != i18n.ZhHans.Label() && s != i18n.ZhHant.Label() && s != i18n.ZhHans.Name() && s != i18n.ZhHant.Name() {
			t.Errorf("Chinese text %q left on screen after switching back to English", s)
		}
	}
}

func TestLanguageSwitchAsksBeforeDroppingEdits(t *testing.T) {
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("test")
	win.Resize(fyne.NewSize(1280, 860))

	a := New(fyneApp, win, appconfig.Config{})
	defer i18n.SetLanguage(i18n.En)
	a.api = api.New("http://127.0.0.1:1", "tok")
	a.showMain()
	ui.settle(150 * time.Millisecond)

	a.unsaved = func() bool { return true }
	a.chooseLanguage(i18n.ZhHans)
	if i18n.Language() != i18n.En {
		t.Fatal("the language changed without asking about unsaved edits")
	}
	if win.Canvas().Overlays().Top() == nil {
		t.Fatal("no confirmation was shown")
	}
}

func TestLanguageFromConfig(t *testing.T) {
	for in, want := range map[string]i18n.Lang{"": i18n.En, "zh-Hans": i18n.ZhHans, "zh-Hant": i18n.ZhHant, "fr": i18n.En} {
		if got := languageFromConfig(in); got != want {
			t.Errorf("languageFromConfig(%q) = %s, want %s", in, got, want)
		}
	}
}
