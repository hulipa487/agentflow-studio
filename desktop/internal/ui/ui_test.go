package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// TestViewsBuild constructs every screen with the headless test driver. It
// does not assert pixel output; it catches nil dereferences and layout
// mistakes that would otherwise only appear when the window opens.
func TestViewsBuild(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("test")

	// The headless driver has no UI goroutine: a background load that finishes
	// would run its completion on its own goroutine, touching widgets this
	// test is still laying out. Completions are queued instead, and settle
	// runs them here. Every load fails at once against the unreachable runtime
	// below, so a short quiet period means they have all arrived.
	pending := make(chan func(), 1024)
	run := func(fn func()) { pending <- fn }
	uiRunner.Store(&run)
	t.Cleanup(func() { uiRunner.Store(nil) })
	settle := func() {
		quiet := time.NewTimer(150 * time.Millisecond)
		defer quiet.Stop()
		for {
			select {
			case fn := <-pending:
				fn()
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(150 * time.Millisecond)
			case <-quiet.C:
				return
			}
		}
	}

	a := New(fyneApp, win, appconfig.Config{})
	a.api = api.New("http://127.0.0.1:1", "tok")
	a.catalog = api.Catalog{
		Models:              []string{"default"},
		Tools:               []string{"builtin:fetch"},
		Capabilities:        []string{"llm.chat", "tools"},
		DefaultCapabilities: []string{"llm.chat", "tools"},
		SafetyProfiles:      []string{"default", "none"},
		PreludeVersion:      1,
		Templates: []api.Template{{
			Name: "echo", Title: "Echo", Description: "echo", Loop: "-- af-prelude-version: 1\nfunction loop() end\n",
		}},
	}

	a.showLogin("")
	if win.Content() == nil {
		t.Fatal("login view is empty")
	}
	a.showMain()
	if a.content == nil {
		t.Fatal("main content not initialized")
	}
	settle()
	agents := a.agentsView()
	if agents == nil {
		t.Fatal("agents view is nil")
	}
	a.setMain(agents)

	tpl := a.catalog.Templates[0]
	builder := a.newBuilderView("", nil, &tpl)
	if builder == nil {
		t.Fatal("builder view is nil")
	}
	existing := a.newBuilderView("x", &api.AgentDetail{Agent: api.AgentJSON{Loop: "./x.lua"}}, nil)
	if existing == nil {
		t.Fatal("existing builder view is nil")
	}

	play := a.newPlaygroundView([]string{"echo"}, "echo")
	if play == nil {
		t.Fatal("playground view is nil")
	}

	safety := a.newSafetyView()
	if safety == nil {
		t.Fatal("safety view is nil")
	}
	creds := a.newCredentialsView()
	if creds == nil {
		t.Fatal("credentials view is nil")
	}

	// Let each screen's first (failing) load land before building the next.
	settle()

	// The operator screens: each must build (and survive its first failed
	// load against an unreachable runtime) without panicking.
	for name, build := range map[string]func() fyne.CanvasObject{
		"overview": func() fyne.CanvasObject { return a.newOverviewView().s.root },
		"users":    func() fyne.CanvasObject { return a.newUsersView().s.root },
		"tenancy":  func() fyne.CanvasObject { return a.newTenancyView().s.root },
		"usage":    func() fyne.CanvasObject { return a.newUsageView().s.root },
		"logs":     func() fyne.CanvasObject { return a.newLogsView().s.root },
		"config":   func() fyne.CanvasObject { return a.newConfigView().s.root },
	} {
		if build() == nil {
			t.Fatalf("%s view is nil", name)
		}
		settle()
	}
	// Every destination routes through enter(): it becomes the active view,
	// its rail row lights up alone, and the breadcrumb names it.
	for _, it := range a.nav {
		a.enter(it.id)
		settle()
		if a.view != it.id {
			t.Fatalf("enter %s left view %q", it.id, a.view)
		}
		for _, other := range a.nav {
			if other.row != nil && other.row.active != (other == it) {
				t.Fatalf("enter %s: rail row %s active=%v", it.id, other.id, other.row.active)
			}
		}
		if want := it.section + " / " + it.label; a.breadcrumb.Text != want {
			t.Fatalf("breadcrumb = %q, want %q", a.breadcrumb.Text, want)
		}
	}
	a.cancelView()
	settle()
}

func TestNavModel(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	a := New(fyneApp, fyneApp.NewWindow("nav"), appconfig.Config{})
	a.nav = a.buildNav()

	ids := map[string]bool{}
	keys := map[fyne.KeyName]string{}
	for _, it := range a.nav {
		if it.id == "" || it.label == "" || it.show == nil || it.icon == nil {
			t.Fatalf("incomplete nav item %+v", it)
		}
		if ids[it.id] {
			t.Fatalf("duplicate nav id %q", it.id)
		}
		ids[it.id] = true
		if !hasString(navSections, it.section) {
			t.Fatalf("%s: unknown section %q", it.id, it.section)
		}
		if it.key != "" {
			if other, dup := keys[it.key]; dup {
				t.Fatalf("shortcut %s used by %s and %s", it.keyLabel, other, it.id)
			}
			keys[it.key] = it.id
		}
	}
	for _, want := range []string{"overview", "agents", "playground", "channels", "models", "credentials", "safety", "users", "tenancy", "usage", "logs", "config", "docs", "settings"} {
		if !ids[want] {
			t.Fatalf("nav has no %q destination", want)
		}
	}
	if it := a.navByID("builder"); it == nil || it.id != "agents" {
		t.Fatalf("builder should highlight agents, got %+v", it)
	}
	// Every destination is reachable from the palette.
	cmds := a.paletteCommands()
	for _, it := range a.nav {
		found := false
		for _, c := range cmds {
			if c.title == "Go to "+it.label {
				found = true
			}
		}
		if !found {
			t.Fatalf("palette has no entry for %s", it.label)
		}
	}
	if _, ok := paletteMatch(paletteCommand{title: "Go to Users", hint: "Profiles"}, "use"); !ok {
		t.Fatal("prefix should match")
	}
	if _, ok := paletteMatch(paletteCommand{title: "Go to Users", hint: "Profiles"}, "zzz"); ok {
		t.Fatal("unrelated query should not match")
	}
}

func TestFormatting(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 9999: "9999", 12_345: "12.3k", 2_000_000: "2M", 1_500_000_000: "1.5B", -12_345: "-12.3k"}
	for n, want := range cases {
		if got := humanCount(n); got != want {
			t.Errorf("humanCount(%d) = %q, want %q", n, got, want)
		}
	}
	if got := humanDuration(3720); got != "1h 2m" {
		t.Errorf("humanDuration = %q", got)
	}
	if logLevelOf("10:00:00 ERR boom") != 4 || logLevelOf("10:00:00 DBG x") != 1 || logLevelOf("garbage") != 2 {
		t.Error("logLevelOf misparsed")
	}
}
