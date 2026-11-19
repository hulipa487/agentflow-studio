package ui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// These match web/src/screens/memory/helpers.test.ts case for case, so the two
// apps build the same drafts.

func TestBuildBackendDraftKeepsTheProvidersKeys(t *testing.T) {
	d, err := buildBackendDraft(backendInput{Name: " vec ", Provider: "sqlite", Path: "./v.db", URL: "postgres://x", Dim: "1536", VectorScanLimit: "5000"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (api.MemoryBackendDraft{Name: "vec", Provider: "sqlite", Path: "./v.db", VectorScanLimit: 5000}); d != want {
		t.Fatalf("sqlite draft = %+v", d)
	}
	d, err = buildBackendDraft(backendInput{Name: "qd", Provider: "qdrant", URL: "${QDRANT_URL}", APIKey: "cred:qdrant", Collection: "docs", Dim: "768", Path: "./ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (api.MemoryBackendDraft{Name: "qd", Provider: "qdrant", URL: "${QDRANT_URL}", APIKey: "cred:qdrant", Collection: "docs", Dim: 768}); d != want {
		t.Fatalf("qdrant draft = %+v", d)
	}
	if d, _ := buildBackendDraft(backendInput{Name: "qd", Provider: "qdrant", ClearAPIKey: true}); !d.ClearAPIKey {
		t.Error("clearing the API key was not sent")
	}
	if _, err := buildBackendDraft(backendInput{Name: "v", Provider: "volatile", MaxEntries: "1e3"}); err == nil {
		t.Error("a count that is not a whole number was accepted")
	}
	// An empty URL is left out, so the runtime keeps the stored one.
	b, _ := json.Marshal(api.MemoryBackendDraft{Name: "pg", Provider: "postgres"})
	if strings.Contains(string(b), "url") {
		t.Errorf("an empty URL was sent: %s", b)
	}
}

func TestBuildProfileDraft(t *testing.T) {
	d, err := buildProfileDraft(profileInput{
		Name:   "p",
		Write:  "plugin:routing_table, plugin:fact_extractor",
		Recall: "plugin:semantic", EmbedModel: "embed",
		Stores: []storeInput{
			{Name: "facts", Backend: "main_db", Window: "200", Requires: []string{"kv"}, Scope: "agent", Pool: " team "},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Stores[0]
	if s.Pool != "team" || s.Scope != "" || s.Window != 200 {
		t.Fatalf("store = %+v", s)
	}
	if !reflect.DeepEqual(d.Write, []string{"plugin:routing_table", "plugin:fact_extractor"}) {
		t.Fatalf("write = %v", d.Write)
	}
	if _, err := buildProfileDraft(profileInput{Name: "p", Stores: []storeInput{{Name: "facts", Window: "lots"}}}); err == nil || !strings.Contains(err.Error(), `store "facts": window`) {
		t.Fatalf("a bad window: %v", err)
	}
	fresh := newProfileDraft([]string{"vectors", "main_db"})
	if len(fresh.Stores) != 2 || fresh.Stores[0].Backend != "main_db" || fresh.Recall != "plugin:recency" {
		t.Fatalf("new profile = %+v", fresh)
	}
}

func TestMemorySummaries(t *testing.T) {
	if got := storeSummary(api.MemoryStoreDraft{Backend: "main_db", Table: "d", Retention: "30d", Window: 200, Requires: []string{"kv"}}); got != "main_db.d · 30d · window 200 · kv" {
		t.Errorf("store summary = %q", got)
	}
	if got := backendTarget(api.MemoryBackend{Provider: "qdrant", HasURL: true, URLRef: "${Q}", Collection: "docs"}); got != "${Q} · docs" {
		t.Errorf("qdrant target = %q", got)
	}
	if got := backendTarget(api.MemoryBackend{Provider: "postgres", HasURL: true, URLDisplay: "postgres://app:xxxxx@db/mem"}); got != "postgres://app:xxxxx@db/mem" {
		t.Errorf("postgres target = %q", got)
	}
	main := api.MemoryBackend{Name: "main_db", Profiles: []string{"conversational", "rag"}, Agents: []string{"chat", "notes"}}
	if got := backendUsedBy(main, []string{"chat"}); got != "profiles: conversational (built-in: chat), rag · agents: notes" {
		t.Errorf("used by = %q", got)
	}
	if backendInUse(api.MemoryBackend{Profiles: []string{"conversational"}}, nil) {
		t.Error("the built-in profile counted as a user with no agent on it")
	}
}

func TestMemoryChoices(t *testing.T) {
	labels, values := memoryChoices([]string{"notes", "conversational"}, "", false)
	if !reflect.DeepEqual(labels, []string{memoryNone, memoryBuiltin, "notes"}) {
		t.Fatalf("labels = %v", labels)
	}
	if memoryLabel("conversational", false, values) != memoryBuiltin || memoryLabel("", false, values) != memoryNone {
		t.Error("labels for the built-in profile and for none")
	}
	labels, values = memoryChoices(nil, "gone", false)
	if labels[len(labels)-1] != "gone · not in catalog" || values["gone · not in catalog"] != "gone" {
		t.Fatalf("a profile the catalog does not list: %v", labels)
	}
	labels, _ = memoryChoices(nil, "", true)
	if labels[len(labels)-1] != memoryInline {
		t.Fatalf("an inline profile: %v", labels)
	}
}

// The builder writes the memory key after the shell, the same bytes the web
// builder writes (web/src/screens/builder/model.test.ts).
func TestBuilderWritesMemory(t *testing.T) {
	fyneApp := test.NewApp()
	claimUI(t)
	defer fyneApp.Quit()
	a := New(fyneApp, fyneApp.NewWindow("builder"), appconfig.Config{})
	a.api = api.New("http://127.0.0.1:1", "tok")
	a.catalog = api.Catalog{SafetyProfiles: []string{"default", "none"}, MemoryProfiles: []string{"group_chat"}}

	inline := map[string]any{"stores": map[string]any{"facts": map[string]any{"backend": "main_db", "table": "notes"}}, "recall": "plugin:recency"}
	b := a.newBuilderView("a", &api.AgentDetail{Agent: api.AgentJSON{Loop: "./a.lua", MemoryInline: inline}}, nil)
	if b.memory.Selected != memoryInline {
		t.Fatalf("an inline profile shows as %q", b.memory.Selected)
	}
	got, err := b.buildYAML()
	if err != nil {
		t.Fatal(err)
	}
	if want := "loop: \"./a.lua\"\nsafety: \"default\"\nhistory_budget: 6000\nmemory: {\"recall\":\"plugin:recency\",\"stores\":{\"facts\":{\"backend\":\"main_db\",\"table\":\"notes\"}}}\n"; got != want {
		t.Fatalf("inline memory:\n got %q\nwant %q", got, want)
	}
	b.memory.SetSelected("group_chat")
	b.shell.SetText(" sandbox ")
	if got, _ := b.buildYAML(); got != "loop: \"./a.lua\"\nsafety: \"default\"\nhistory_budget: 6000\nshell: \"sandbox\"\nmemory: \"group_chat\"\n" {
		t.Fatalf("named memory: %q", got)
	}
	b.memory.SetSelected(memoryNone)
	if got, _ := b.buildYAML(); strings.Contains(got, "memory") {
		t.Fatalf("no memory still wrote the key: %q", got)
	}
}
