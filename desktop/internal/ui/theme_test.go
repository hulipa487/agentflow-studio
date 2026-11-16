package ui

import (
	"image/color"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"agentflow-studio/internal/appconfig"
)

func TestThemeFromConfig(t *testing.T) {
	cases := []struct {
		saved string
		os    fyne.ThemeVariant
		want  appTheme
	}{
		{"light", theme.VariantDark, themeLight},
		{"dark", theme.VariantLight, themeDark},
		{"", theme.VariantLight, themeLight},
		{"", theme.VariantDark, themeDark},
		{"unknown", theme.VariantDark, themeDark},
	}
	for _, tc := range cases {
		if got := themeFromConfig(tc.saved, tc.os); got != tc.want {
			t.Errorf("themeFromConfig(%q, %v) = %v, want %v", tc.saved, tc.os, got, tc.want)
		}
	}
}

func TestPaletteFromConfig(t *testing.T) {
	cases := []struct {
		saved string
		want  themePalette
	}{
		{"", paletteTrans},
		{"trans", paletteTrans},
		{"obsidian", paletteObsidian},
		{"aurora", paletteObsidian},
		{"unknown", paletteTrans},
	}
	for _, tc := range cases {
		if got := paletteFromConfig(tc.saved); got != tc.want {
			t.Errorf("paletteFromConfig(%q) = %v, want %v", tc.saved, got, tc.want)
		}
	}
}

func TestThemeRemapUpdatesCanvasObjects(t *testing.T) {
	setTheme(paletteTrans, themeDark)
	defer setTheme(paletteTrans, themeDark)

	old := activePalette
	rect := canvas.NewRectangle(colorSurface)
	text := canvas.NewText("status", colorText)
	img := canvas.NewImageFromResource(logoDarkResource)
	box := container.NewHBox(rect, text, img)

	setThemePalette(themeLight)
	remapThemeColors(box, old, activePalette)

	if got := rect.FillColor.(color.NRGBA); got != lightPalette.Surface {
		t.Fatalf("rectangle fill = %#v, want %#v", got, lightPalette.Surface)
	}
	if got := text.Color.(color.NRGBA); got != lightPalette.Text {
		t.Fatalf("text color = %#v, want %#v", got, lightPalette.Text)
	}
	if img.Resource == nil || img.Resource.Name() != logoLightResource.Name() {
		t.Fatalf("logo resource = %v, want %v", img.Resource, logoLightResource)
	}

	// Switch to Obsidian Light and verify in-place remap
	old = activePalette
	setTheme(paletteObsidian, themeLight)
	remapThemeColors(box, old, activePalette)

	if got := rect.FillColor.(color.NRGBA); got != obsidianLightPalette.Surface {
		t.Fatalf("obsidian light rectangle fill = %#v, want %#v", got, obsidianLightPalette.Surface)
	}
	if got := text.Color.(color.NRGBA); got != obsidianLightPalette.Text {
		t.Fatalf("obsidian light text color = %#v, want %#v", got, obsidianLightPalette.Text)
	}
	if img.Resource == nil || img.Resource.Name() != logoObsidianLightResource.Name() {
		t.Fatalf("obsidian light logo resource = %v, want %v", img.Resource, logoObsidianLightResource)
	}

	// Switch to Obsidian Dark and verify in-place remap back from Light
	old = activePalette
	setTheme(paletteObsidian, themeDark)
	remapThemeColors(box, old, activePalette)

	if got := rect.FillColor.(color.NRGBA); got != obsidianDarkPalette.Surface {
		t.Fatalf("obsidian dark rectangle fill = %#v, want %#v", got, obsidianDarkPalette.Surface)
	}
	if got := text.Color.(color.NRGBA); got != obsidianDarkPalette.Text {
		t.Fatalf("obsidian dark text color = %#v, want %#v", got, obsidianDarkPalette.Text)
	}
	if img.Resource == nil || img.Resource.Name() != logoObsidianDarkResource.Name() {
		t.Fatalf("obsidian dark logo resource = %v, want %v", img.Resource, logoObsidianDarkResource)
	}
}

func TestSettingsThemeAndPaletteSwitchingPersists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("AGENTFLOW_STUDIO_CONFIG", cfgPath)

	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("settings-test")

	a := New(fyneApp, win, appconfig.Config{Theme: "dark", Palette: "trans"})
	defer setTheme(paletteTrans, themeDark)

	a.content = container.NewStack()
	a.showSettings()

	// Switch to Obsidian palette in dark mode
	a.choosePalette(paletteObsidian)
	if a.palette != paletteObsidian {
		t.Fatalf("palette = %v, want obsidian", a.palette)
	}
	if got := appconfig.Load(); got.Palette != "obsidian" || got.Theme != "dark" {
		t.Fatalf("saved config = %+v, want palette=obsidian theme=dark", got)
	}

	// Switch to Light mode while keeping Obsidian palette
	a.chooseTheme(themeLight)
	if a.variant != themeLight || a.palette != paletteObsidian {
		t.Fatalf("variant=%v palette=%v, want light + obsidian", a.variant, a.palette)
	}
	if got := appconfig.Load(); got.Palette != "obsidian" || got.Theme != "light" {
		t.Fatalf("saved config = %+v, want palette=obsidian theme=light", got)
	}

	// Switch back to Trans Pride in light mode
	a.choosePalette(paletteTrans)
	if a.palette != paletteTrans || a.variant != themeLight {
		t.Fatalf("variant=%v palette=%v, want light + trans", a.variant, a.palette)
	}
	if got := appconfig.Load(); got.Palette != "trans" || got.Theme != "light" {
		t.Fatalf("saved config = %+v, want palette=trans theme=light", got)
	}
}
