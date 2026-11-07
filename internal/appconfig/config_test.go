package appconfig

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "config.json")
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", p)

	if got := Load(); got.URL != "" || got.Token != "" {
		t.Fatalf("missing file loaded as %+v", got)
	}
	want := Config{URL: "http://127.0.0.1:9090", Token: "secret", Theme: "light"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got != want {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}

	wantWithPalette := Config{URL: "http://127.0.0.1:9090", Token: "secret", Theme: "dark", Palette: "obsidian"}
	if err := Save(wantWithPalette); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got != wantWithPalette {
		t.Fatalf("loaded %+v, want %+v", got, wantWithPalette)
	}
}
