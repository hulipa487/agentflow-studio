package ui

import (
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// TestSnapshots renders the main screens with the headless test driver and
// writes PNGs when STUDIO_SNAPSHOT_DIR is set. It is a visual development aid,
// not an assertion, so it is skipped in normal test runs.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("STUDIO_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set STUDIO_SNAPSHOT_DIR to write snapshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	var emptyAgentsList, emptyChannelsList atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/admin/api/studio/agents":
			if emptyAgentsList.Load() {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "agents": []api.AgentRow{}})
				return
			}
			agents := []api.AgentRow{
				{Name: "assistant", Model: "default", Loop: "./examples/loops/chat.lua", Persisted: true, Capabilities: []string{"llm.chat", "memory", "tools"}, Sessions: 1},
				{Name: "echo", Loop: "./examples/loops/echo.lua", Persisted: true, Capabilities: []string{"tools"}},
				{Name: "lab", Loop: "./loops/lab.lua", Capabilities: []string{"llm.chat"}},
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "agents": agents})
		case r.URL.Path == "/admin/api/studio/channels":
			if emptyChannelsList.Load() {
				_ = json.NewEncoder(w).Encode(api.ChannelList{
					OK: true, Mode: "configdir", Path: "/srv/agentflow/channels.yaml",
					Editable: true, Note: "Channels bind when agentflow starts; saved changes apply after a restart.",
				})
				return
			}
			channels := []api.Channel{
				{Index: 0, Name: "telegram-main", Type: "telegram", Agent: "assistant", Mode: "polling", HasToken: true, TokenRef: "${TELEGRAM_BOT_TOKEN}", Live: true},
				{Index: 1, Name: "github", Type: "ghhook", Agent: "echo", Path: "/gh/", HasSecret: true, MediaAllow: []string{"image/*"}, MediaMaxBytes: 8388608},
			}
			_ = json.NewEncoder(w).Encode(api.ChannelList{
				OK: true, Mode: "configdir", Path: "/srv/agentflow/channels.yaml",
				Editable: true, Note: "Channels bind when agentflow starts; saved changes apply after a restart.",
				Channels: channels,
			})
		case r.URL.Path == "/admin/api/models":
			models := []api.Model{
				{Name: "default", Provider: "openai", Model: "gpt-4o-mini", Timeout: "60s", MaxTokens: 4096, HasKey: true, KeyFingerprint: "…abcd", InRuntime: true, InFile: true},
				{Name: "fast", Provider: "anthropic", Model: "claude-3-5-haiku", BaseURL: "https://api.anthropic.com", HasKey: true, KeyFingerprint: "…wxyz", InRuntime: true, Drift: true},
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": models})
		case r.URL.Path == "/admin/api/studio/threads":
			threads := []api.Thread{
				{Agent: "assistant", Thread: "4f2a91c3d8b14e6f", Events: 12, Last: time.Now()},
				{Agent: "assistant", Thread: "9b7d02e5a1c34f80", Events: 4, Last: time.Now().Add(-time.Hour)},
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "threads": threads})
		default:
			if !operatorFixture(w, r) {
				http.NotFound(w, r)
			}
		}
	}))
	defer srv.Close()

	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("snapshot")
	win.Resize(fyne.NewSize(1280, 860))

	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	if err := a.SetDocs(os.DirFS("../../../docs")); err != nil {
		t.Fatal(err)
	}
	a.api = api.New(srv.URL, "token")
	a.catalog = api.Catalog{
		Models:              []string{"default", "fast"},
		Tools:               []string{"builtin:fetch", "builtin:web_search", "builtin:browser", "builtin:shell.exec"},
		Capabilities:        []string{"llm.chat", "memory", "tools", "agent.send", "agent.request", "net.http", "net.mail", "shell.exec", "files", "session.state", "users", "channel.push"},
		DefaultCapabilities: []string{"agent.send", "llm.chat", "memory", "net.http", "tools", "users"},
		SafetyProfiles:      []string{"default", "none", "strict"},
		PreludeVersion:      1,
		Templates: []api.Template{
			{Name: "echo", Title: "Echo", Description: "Reply with the text you received; no model required.", Loop: "-- af-prelude-version: 1\nfunction loop() end\n"},
			{Name: "llm-chat", Title: "LLM chat", Description: "One model call per message with a system prompt.", Loop: "-- af-prelude-version: 1\nfunction loop() end\n"},
		},
	}

	capture := func(name string) {
		t.Helper()
		ui.wait(150 * time.Millisecond)
		img := win.Canvas().Capture()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	d := &api.AgentDetail{
		OK:        true,
		Name:      "assistant",
		Source:    "config",
		AgentYAML: "loop: ./examples/loops/chat.lua\nmodel: default\nsafety: default\nhistory_budget: 6000\ncapabilities:\n  - llm.chat\n  - memory\n  - tools\nskills:\n  - builtin:fetch\n",
		Agent: api.AgentJSON{
			Loop: "./examples/loops/chat.lua", Model: "default", Safety: "default",
			HistoryBudget: 6000,
			Capabilities:  []string{"llm.chat", "memory", "tools"},
			Skills:        []string{"builtin:fetch"},
		},
		Loop:         "-- af-prelude-version: 1\n\nlocal function handle(msg)\n  local info = agent.info()\n  local reply = llm.chat({\n    { role = \"system\", content = info.instructions },\n    { role = \"user\", content = msg.text },\n  })\n  session.send(reply.text)\nend\n\nfunction loop()\n  while true do\n    local msg = session.inbox()\n    handle(msg)\n  end\nend\n",
		Instructions: "You are the example assistant for an AgentFlow deployment.\n\n- Be concise and concrete.\n- When you use a tool, say what you are doing.\n- If a request is outside your tools, say so.",
		Files: api.AgentFiles{
			Mode: "configdir", ProfilePath: "/srv/agentflow/profiles/assistant.yaml",
			LoopPath: "/srv/agentflow/loops/assistant.lua", InstructionsPath: "/srv/agentflow/prompts/assistant.md",
			Managed: true, Mtime: time.Now().Unix(),
		},
		Persisted: true, Model: "default", Sessions: 1,
	}
	demoPlayground := func() *playgroundView {
		a.setActive("playground")
		a.setBreadcrumb("Workspace / Playground")
		p := a.newPlaygroundView([]string{"assistant", "echo", "lab"}, "assistant")
		a.setMain(p.root)
		p.addBubble("You", "Summarize what this agent can do, and check the weather in Hong Kong.", true)
		p.assistant = p.addBubble("Agent", `I can answer with the configured model, remember earlier turns in the dialogue store, and call the tools you enabled.

I don't have a weather tool enabled in this example, so I can't fetch a live forecast.`, false)
		p.events = []api.TraceEvent{
			{Seq: 1, Kind: "turn.start", Turn: "m1"},
			{Seq: 2, Kind: "op.end", Op: "llm.chat", Model: "default", OK: true, DurationMS: 842, TokensIn: 412, TokensOut: 96},
			{Seq: 3, Kind: "op.end", Op: "store.put", OK: true, DurationMS: 3},
			{Seq: 4, Kind: "reply", OK: true},
			{Seq: 5, Kind: "turn.end"},
		}
		p.renderTrace()
		return p
	}

	render := func(suffix string) {
		a.showLogin("")
		capture("01-login" + suffix)

		a.showLogin("The admin token is required.")
		ui.wait(150 * time.Millisecond)
		capture("01-login-error" + suffix)

		a.showMain()
		ui.wait(400 * time.Millisecond)
		capture("02-overview" + suffix)
		a.showAgents()
		ui.wait(250 * time.Millisecond)
		capture("02-agents" + suffix)

		test.MoveMouse(win.Canvas(), fyne.NewPos(24, 30))
		ui.wait(100 * time.Millisecond)
		capture("02-agents-logo-hover" + suffix)
		a.toggleRail()
		ui.wait(100 * time.Millisecond)
		capture("02-agents-rail-collapsed" + suffix)
		a.toggleRail()
		test.MoveMouse(win.Canvas(), fyne.NewPos(600, 400))

		emptyAgentsList.Store(true)
		a.showAgents()
		ui.wait(250 * time.Millisecond)
		capture("02-agents-empty" + suffix)
		emptyAgentsList.Store(false)

		b := a.newBuilderView("assistant", d, nil)
		a.setMain(b.root)
		ui.wait(300 * time.Millisecond)
		capture("03-builder" + suffix)
		b.model.Tapped(&fyne.PointEvent{})
		ui.wait(250 * time.Millisecond)
		capture("03-builder-select" + suffix)
		if top := win.Canvas().Overlays().Top(); top != nil {
			top.Hide()
		}
		b.tabs.SelectIndex(1)
		ui.wait(700 * time.Millisecond)
		capture("03-builder-loop" + suffix)

		b.luaEditor.editor.selectWord(8, 10)
		ui.wait(150 * time.Millisecond)
		capture("03-builder-occurrences" + suffix)

		b.luaEditor.editor.SetCursor(8, 10)
		b.luaEditor.editor.ShowHover("session.send(text, opts?) · Reply to the active inbound message.")
		ui.wait(150 * time.Millisecond)
		capture("03-builder-hover" + suffix)
		b.luaEditor.editor.HideHover()

		b.loop.SetText("function loop()\n  local msg = session.inbox()\n  local reply = llm.chta({})\n  mesage = msg.text\nend\n")
		ui.wait(900 * time.Millisecond)
		capture("03-builder-loop-issues" + suffix)

		b.luaEditor.hoverAt(0, 0, true)
		ui.wait(150 * time.Millisecond)
		capture("03-builder-error-hover" + suffix)
		b.luaEditor.editor.HideHover()

		b.loop.SetText("function loop()\n  session.\nend\n")
		b.luaEditor.editor.SetCursor(1, len([]rune("  session.")))
		ui.wait(500 * time.Millisecond)
		b.luaEditor.editor.RequestCompletion(true)
		ui.wait(800 * time.Millisecond)
		capture("03-builder-completion" + suffix)

		nb := a.newBuilderView("", nil, &a.catalog.Templates[0])
		a.setMain(nb.root)
		ui.wait(400 * time.Millisecond)
		capture("03-builder-new" + suffix)

		nb.nameEntry.SetText("")
		nb.validate()
		ui.wait(250 * time.Millisecond)
		capture("03-builder-validation-error" + suffix)

		a.showNewAgent()
		ui.wait(250 * time.Millisecond)
		capture("03-new-agent-dialog" + suffix)
		if top := win.Canvas().Overlays().Top(); top != nil {
			top.Hide()
		}

		demoPlayground()
		capture("04-playground" + suffix)

		emptyAgentsList.Store(true)
		a.showPlayground("")
		ui.wait(300 * time.Millisecond)
		capture("04-playground-empty" + suffix)
		emptyAgentsList.Store(false)

		a.showChannels()
		ui.wait(600 * time.Millisecond)
		capture("07-channels" + suffix)

		emptyChannelsList.Store(true)
		a.showChannels()
		ui.wait(250 * time.Millisecond)
		capture("07-channels-empty" + suffix)

		// Also cover the empty channel screen when there are no agent targets.
		emptyAgentsList.Store(true)
		a.showChannels()
		ui.wait(250 * time.Millisecond)
		capture("07-channels-no-agents" + suffix)
		emptyAgentsList.Store(false)
		emptyChannelsList.Store(false)

		a.showSettings()
		ui.wait(200 * time.Millisecond)
		capture("10-settings" + suffix)

		a.showModels()
		ui.wait(500 * time.Millisecond)
		capture("07-models" + suffix)

		mv := a.newModelsView()
		a.setMain(mv.root)
		ui.wait(350 * time.Millisecond)
		mv.showEditor(&api.Model{Name: "fast", Provider: "anthropic", Model: "claude-3-5-haiku", HasKey: true})
		ui.wait(300 * time.Millisecond)
		capture("07-models-editor" + suffix)
		if top := win.Canvas().Overlays().Top(); top != nil {
			top.Hide()
		}

		a.showUsers()
		ui.wait(400 * time.Millisecond)
		capture("12-users" + suffix)
		a.newUsersViewOpen("u-7f3a")
		ui.wait(500 * time.Millisecond)
		capture("12-users-detail" + suffix)

		a.showTenancy()
		ui.wait(500 * time.Millisecond)
		capture("13-tenancy" + suffix)

		a.showUsage()
		ui.wait(400 * time.Millisecond)
		capture("14-usage" + suffix)

		a.showLogs()
		ui.wait(500 * time.Millisecond)
		capture("15-logs" + suffix)

		a.showConfig()
		ui.wait(400 * time.Millisecond)
		capture("16-config" + suffix)

		a.showOverview()
		ui.wait(300 * time.Millisecond)
		a.showCommandPalette()
		ui.wait(300 * time.Millisecond)
		capture("17-palette" + suffix)
		if top := win.Canvas().Overlays().Top(); top != nil {
			top.Hide()
		}

		a.showDocs("getting-started")
		ui.wait(400 * time.Millisecond)
		capture("08-docs" + suffix)
		a.showDocs("configuration")
		ui.wait(700 * time.Millisecond)
		capture("09-docs-config" + suffix)
	}

	render("")
	// Switch modes with the playground on screen: this captures the in-place
	// color remap before the views are rebuilt in the light palette.
	a.applyTheme(themeLight)
	capture("05-playground-remap-light")
	render("-light")

	// Render the Obsidian theme in both dark and light modes.
	a.applyAppearance(paletteObsidian, themeDark)
	a.showLogin("")
	capture("11-obsidian-login-dark")
	a.showMain()
	ui.wait(200 * time.Millisecond)
	capture("11-obsidian-agents-dark")
	a.showSettings()
	ui.wait(200 * time.Millisecond)
	capture("11-obsidian-settings-dark")
	demoPlayground()
	capture("11-obsidian-playground-dark")

	a.applyAppearance(paletteObsidian, themeLight)
	a.showSettings()
	ui.wait(200 * time.Millisecond)
	capture("11-obsidian-settings-light")
	demoPlayground()
	capture("11-obsidian-playground-light")

	// Restore default theme for narrow viewport checks.
	a.applyAppearance(paletteTrans, themeDark)

	// The window minimum is 1080x700; render the dense screens at that width.
	win.Resize(fyne.NewSize(1100, 780))
	demoPlayground()
	capture("06-playground-narrow")
	a.showMain()
	ui.wait(200 * time.Millisecond)
	win.Resize(fyne.NewSize(1100, 780))
	capture("06-agents-narrow")
	b := a.newBuilderView("assistant", d, nil)
	a.setMain(b.root)
	win.Resize(fyne.NewSize(1100, 780))
	capture("06-builder-narrow")
}

// newUsersViewOpen shows Users with one profile selected (snapshots only).
func (a *App) newUsersViewOpen(id string) {
	a.enter("users")
	v := a.newUsersView()
	v.selected = id
	a.setMain(v.s.root)
}

// operatorFixture answers the operator routes (state, metrics, users, tenancy,
// usage, logs, config) with plausible data for the snapshot run.
func operatorFixture(w http.ResponseWriter, r *http.Request) bool {
	now := time.Now().Unix()
	enc := json.NewEncoder(w)
	series := func(base, step int64) []api.MetricPoint {
		pts := make([]api.MetricPoint, 60)
		for i := range pts {
			pts[i] = api.MetricPoint{At: now - int64(60-i)*5, Value: base + step*int64(i) + int64((i*7)%5)}
		}
		return pts
	}
	switch p := r.URL.Path; {
	case p == "/admin/api/state":
		_ = enc.Encode(api.State{
			OK: true, Version: "v0.9.0", UptimeS: 3*3600 + 420, ConfigPath: "/srv/agentflow/agentflow.yaml",
			ConfigEpoch: "a41c9e", Instance: "host-1-7f2c9a", Region: "eu-west",
			Agents: []api.StateAgent{
				{Name: "assistant", Model: "default", Loop: "./loops/chat.lua"},
				{Name: "digest", Model: "fast", Loop: "./loops/digest.lua", Persistent: true},
				{Name: "echo", Loop: "./loops/echo.lua"},
			},
			Channels: []api.StateChannel{
				{Name: "telegram-main", Type: "telegram", Agent: "assistant"},
				{Name: "local", Type: "webhook", Agent: "echo", Path: "/webhook/local/"},
				{Name: "teams", Type: "bridge", Agent: "assistant", Path: "/bridge/teams/", MediaEnabled: true},
			},
			Sessions: []api.StateSession{
				{SessionID: "assistant|telegram-main:5501", Agent: "assistant", Busy: true},
				{SessionID: "assistant|telegram-main:9120", Agent: "assistant"},
				{SessionID: "digest|boot", Agent: "digest"},
			},
			SessionsActive: 1, SessionsIdle: 2, CredentialsEnabled: true,
		})
	case p == "/admin/api/metrics":
		_ = enc.Encode(api.Metrics{OK: true,
			Series: map[string][]api.MetricPoint{
				"agentflow_llm_calls":     series(1200, 3),
				"agentflow_llm_tokens":    series(840000, 2100),
				"agentflow_ingress_total": series(900, 2),
				"agentflow_safety_drops":  series(4, 0),
			},
			Latest: map[string]int64{"agentflow_llm_calls": 1380, "agentflow_llm_tokens": 963900, "agentflow_ingress_total": 1020, "agentflow_safety_drops": 6},
		})
	case p == "/admin/api/users":
		_ = enc.Encode(api.UserList{Day: "2026-11-07", Users: []api.UserRow{
			{UserID: "u-7f3a", DisplayName: "Ada Lovelace", Email: "ada@example.com", Identities: 2, Quota: &api.Quota{Used: 41200, Limit: 100000}},
			{UserID: "u-91bc", DisplayName: "Grace Hopper", Identities: 1, Quota: &api.Quota{Used: 88100, Limit: 100000}},
			{UserID: "u-c0de", Email: "linus@example.com", Identities: 1, Quota: &api.Quota{Used: 1200}},
		}})
	case strings.HasPrefix(p, "/admin/api/users/"):
		_ = enc.Encode(api.UserDetail{
			Day: "2026-11-07",
			Profile: api.Profile{UserID: "u-7f3a", DisplayName: "Ada Lovelace", Email: "ada@example.com", TokensPerDay: 100000, Model: "fast",
				Identities: []api.Identity{
					{ID: "i1", Channel: "telegram", Username: "@ada", Trust: "verified", LastSeen: now - 300},
					{ID: "i2", Channel: "bridge", NativeFrom: "U0142", Trust: "asserted", LastSeen: now - 86400},
				}},
			UsageToday: &api.UsageTotals{Input: 30100, Output: 11100, Cached: 8000, Calls: 37, Failed: 1},
			Quota:      &api.Quota{Used: 41200, Limit: 100000, InFlight: 900},
			Projects:   []string{"notes", "thesis"},
			Audit: []api.JournalEntry{
				{Ts: now - 120, Direction: "in", Status: "routed", Channel: "telegram-main", Agent: "assistant", Text: "Summarise the engine notes"},
				{Ts: now - 110, Direction: "out", Status: "delivered", Channel: "telegram-main", Agent: "assistant", Text: "Here is the summary: …"},
			},
		})
	case p == "/admin/api/groups":
		_ = enc.Encode(map[string]any{"ok": true, "groups": []api.Group{
			{UUID: "g-5d21aa", Name: "analytical-engine", Master: "u-7f3a", CreatedAt: now - 9*86400},
			{UUID: "g-0bb7", Name: "old-team", Master: "u-91bc", CreatedAt: now - 90*86400, DisbandedAt: now - 3*86400},
		}})
	case p == "/admin/api/pools":
		pool := api.Pool{UUID: "p-31f0", Name: "team-notes", RetentionDays: 90, CreatedAt: now - 5*86400, CreatedBy: "u-7f3a"}
		pool.Owner.Group = "g-5d21aa"
		_ = enc.Encode(map[string]any{"ok": true, "pools": []api.Pool{pool}})
	case p == "/admin/api/usage":
		_ = enc.Encode(api.UsageDay{OK: true, Day: "2026-11-07", Tenants: 3, Rows: []api.UsageRow{
			{UserID: "u-91bc", Agent: "assistant", Model: "default", UsageTotals: api.UsageTotals{Input: 60100, Output: 28000, Calls: 52}},
			{UserID: "u-7f3a", Agent: "assistant", Model: "fast", UsageTotals: api.UsageTotals{Input: 30100, Output: 11100, Calls: 37, Failed: 1}},
			{UserID: "", Agent: "digest", Model: "fast", UsageTotals: api.UsageTotals{Input: 9000, Output: 2400, Calls: 4}},
		}, Totals: struct {
			Input    int64 `json:"input"`
			Output   int64 `json:"output"`
			Billable int64 `json:"billable"`
			Calls    int64 `json:"calls"`
		}{Input: 99200, Output: 41500, Billable: 140700, Calls: 93}})
	case p == "/admin/api/logs":
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range []string{
			"10:41:02 INF config loaded source=/srv/agentflow/agentflow.yaml epoch=a41c9e",
			"10:41:02 INF op pool workers=8",
			"10:41:03 DBG op dispatch op=llm.chat model=default blocking=true",
			"10:41:04 WRN channel skipped: unresolved credential channel=teams field=token",
			"10:41:05 INF agentflow up agents=3 channels=3",
			"10:42:17 ERR loop error err=attempt to index nil with 'text'",
			"10:42:18 WRN loop crashed; restarting in 1s",
		} {
			fmt.Fprintf(w, "data: %s\n\n", l)
		}
	case p == "/admin/api/config":
		_ = enc.Encode(api.ConfigFile{OK: true, Path: "/srv/agentflow/agentflow.yaml", Mtime: now - 3600,
			Raw:  "version: \"1\"\n\nmodels:\n  default:\n    provider: openai\n    model: llama3.1\n    base_url: http://127.0.0.1:11434/v1\n    api_key: ${OPENAI_API_KEY:-}\n\nruntime:\n  persistence: sqlite://./data/agentflow.db\n  limits:\n    sessions_per_tenant: 50\n    egress_per_minute: 60\n\nagents:\n  echo:\n    loop: ./loops/echo.lua\n",
			View: map[string]any{"version": "1", "models": map[string]any{"default": map[string]any{"provider": "openai", "api_key": "••••"}}}})
	default:
		return false
	}
	return true
}
