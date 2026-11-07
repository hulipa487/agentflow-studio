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
	got := Load()
	if got.URL != want.URL || got.Token != want.Token || got.Theme != want.Theme {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	// The single connection became one saved profile.
	if len(got.Profiles) != 1 || got.Active != "127.0.0.1:9090" || got.Profiles[0].URL != want.URL {
		t.Fatalf("migrated profiles = %+v (active %q)", got.Profiles, got.Active)
	}

	wantWithPalette := Config{URL: "http://127.0.0.1:9090", Token: "secret", Theme: "dark", Palette: "obsidian"}
	if err := Save(wantWithPalette); err != nil {
		t.Fatal(err)
	}
	got = Load()
	if got.Theme != "dark" || got.Palette != "obsidian" || got.Token != "secret" {
		t.Fatalf("loaded %+v, want %+v", got, wantWithPalette)
	}
}

// TestProfilesRoundTrip: two connections survive Save/Load, the active one's
// URL and token are mirrored, and selecting the other switches them.
func TestProfilesRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", p)

	c := Config{}
	c.RememberConnection("http://eu-west.internal:9090", "eu-token", true)
	c.RememberConnection("http://us-east.internal:9090", "us-token", true)
	if c.Active != "us-east.internal:9090" {
		t.Fatalf("active = %q", c.Active)
	}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}

	got := Load()
	if len(got.Profiles) != 2 || got.URL != "http://us-east.internal:9090" || got.Token != "us-token" {
		t.Fatalf("loaded %+v", got)
	}
	if !got.SelectProfile("eu-west.internal:9090") {
		t.Fatal("saved profile not selectable")
	}
	if got.URL != "http://eu-west.internal:9090" || got.Token != "eu-token" {
		t.Fatalf("selection did not switch the mirror: %+v", got)
	}
	// A forgotten token keeps the URL and drops the secret.
	name := got.RememberConnection("http://ap.internal:9090", "secret", false)
	got.SelectProfile(name)
	if got.ForgetToken != true || got.Token != "" {
		t.Fatalf("forget-token profile = %+v", got)
	}
}

// TestNormalizeFallsBackFromAMissingActiveProfile: a hand-edited active name
// must not leave the app with an empty URL.
func TestNormalizeFallsBackFromAMissingActiveProfile(t *testing.T) {
	c := Config{
		Profiles: []Profile{{Name: "a", URL: "http://a:9090", Token: "t"}},
		Active:   "gone",
	}
	c.normalize()
	if c.Active != "a" || c.URL != "http://a:9090" || c.Token != "t" {
		t.Fatalf("normalized %+v", c)
	}
}

// Two cells behind one host are two profiles, each selecting its own URL.
func TestProfilesOnOneHostDoNotCollide(t *testing.T) {
	var c Config
	eu := c.RememberConnection("https://agents.example.com/eu", "t-eu", true)
	us := c.RememberConnection("https://agents.example.com/us", "t-us", true)
	root := c.RememberConnection("https://agents.example.com", "t-root", true)
	again := c.RememberConnection("https://agents.example.com/", "t-root2", true)
	names := map[string]bool{eu: true, us: true, root: true, again: true}
	if len(names) != 4 || len(c.Profiles) != 4 {
		t.Fatalf("names %q %q %q %q, %d profiles; want four distinct", eu, us, root, again, len(c.Profiles))
	}
	if !c.SelectProfile(eu) || c.URL != "https://agents.example.com/eu" || c.Token != "t-eu" {
		t.Fatalf("selecting %q gave %s", eu, c.URL)
	}
	if !c.SelectProfile(us) || c.URL != "https://agents.example.com/us" || c.Token != "t-us" {
		t.Fatalf("selecting %q gave %s", us, c.URL)
	}
	// The same URL again reuses its profile instead of adding one.
	if n := c.RememberConnection("https://agents.example.com/us", "t-us2", true); n != us || len(c.Profiles) != 4 {
		t.Fatalf("re-login made %q and %d profiles", n, len(c.Profiles))
	}
}
