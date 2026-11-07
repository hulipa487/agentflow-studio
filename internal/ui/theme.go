package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// appTheme controls the luminance mode (Dark vs Light).
type appTheme int

const (
	themeDark appTheme = iota
	themeLight
)

func (t appTheme) String() string {
	if t == themeLight {
		return "light"
	}
	return "dark"
}

// themePalette selects the color theme family (Trans Pride vs Obsidian).
// Both themes support full dark and light variants.
type themePalette int

const (
	paletteTrans themePalette = iota
	paletteObsidian
)

func (p themePalette) String() string {
	if p == paletteObsidian {
		return "obsidian"
	}
	return "trans"
}

func (p themePalette) Title() string {
	if p == paletteObsidian {
		return "Obsidian"
	}
	return "Trans Pride"
}

func (p themePalette) Description() string {
	if p == paletteObsidian {
		return "Electric violet & teal accents over a carbon or alabaster shell."
	}
	return "Flag sky-blue, pastel pink & white over a calm deep-navy or ice shell."
}

func paletteFromConfig(saved string) themePalette {
	switch saved {
	case "obsidian", "aurora", "studio":
		return paletteObsidian
	default:
		return paletteTrans
	}
}

type palette struct {
	BG, Surface, Surface2, Border color.NRGBA
	Text, Muted, Faint            color.NRGBA
	Accent, Accent2, AccentDim    color.NRGBA
	Success, Warning, Danger      color.NRGBA
	Hover, Pressed                color.NRGBA
	White, Float, OnPrimary       color.NRGBA
	ToneAccent, ToneSuccess       color.NRGBA
	ToneWarning, ToneDanger       color.NRGBA
}

var (
	// Flag colors are used for the decorative stripe and logo in Trans Pride.
	flagBlue  = color.NRGBA{R: 0x5B, G: 0xCE, B: 0xFA, A: 0xFF}
	flagPink  = color.NRGBA{R: 0xF5, G: 0xA9, B: 0xB8, A: 0xFF}
	flagWhite = color.NRGBA{R: 0xFE, G: 0xFF, B: 0xFF, A: 0xFF}

	// Obsidian accent colors.
	obsidianViolet = color.NRGBA{R: 0x9D, G: 0x7B, B: 0xFF, A: 0xFF}
	obsidianTeal   = color.NRGBA{R: 0x2D, G: 0xD4, B: 0xBF, A: 0xFF}

	// Trans Pride — Dark
	darkPalette = palette{
		BG:          color.NRGBA{0x0B, 0x11, 0x1C, 0xFF},
		Surface:     color.NRGBA{0x11, 0x19, 0x27, 0xFF},
		Surface2:    color.NRGBA{0x18, 0x23, 0x34, 0xFF},
		Border:      color.NRGBA{0x25, 0x34, 0x4B, 0xFF},
		Text:        color.NRGBA{0xF4, 0xF8, 0xFC, 0xFF},
		Muted:       color.NRGBA{0x9E, 0xB1, 0xC7, 0xFF},
		Faint:       color.NRGBA{0x69, 0x7E, 0x97, 0xFF},
		Accent:      flagBlue,
		Accent2:     flagPink,
		AccentDim:   color.NRGBA{0x19, 0x31, 0x4A, 0xFF},
		Success:     color.NRGBA{0x5E, 0xDF, 0xB8, 0xFF},
		Warning:     color.NRGBA{0xF4, 0xCA, 0x64, 0xFF},
		Danger:      color.NRGBA{0xF2, 0x68, 0x88, 0xFF},
		Hover:       color.NRGBA{0x1E, 0x2C, 0x41, 0xFF},
		Pressed:     color.NRGBA{0x27, 0x39, 0x52, 0xFF},
		White:       flagWhite,
		Float:       color.NRGBA{0x0E, 0x15, 0x22, 0xFF},
		OnPrimary:   color.NRGBA{0x07, 0x11, 0x1D, 0xFF},
		ToneAccent:  color.NRGBA{0x13, 0x27, 0x3C, 0xFF},
		ToneSuccess: color.NRGBA{0x10, 0x2E, 0x29, 0xFF},
		ToneWarning: color.NRGBA{0x2E, 0x27, 0x15, 0xFF},
		ToneDanger:  color.NRGBA{0x30, 0x17, 0x23, 0xFF},
	}

	// Trans Pride — Light
	lightPalette = palette{
		BG:          color.NRGBA{0xF6, 0xFA, 0xFD, 0xFF},
		Surface:     color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF},
		Surface2:    color.NRGBA{0xED, 0xF5, 0xFA, 0xFF},
		Border:      color.NRGBA{0xCF, 0xE0, 0xEC, 0xFF},
		Text:        color.NRGBA{0x12, 0x1F, 0x2B, 0xFF},
		Muted:       color.NRGBA{0x5A, 0x6F, 0x82, 0xFF},
		Faint:       color.NRGBA{0x8A, 0x9E, 0xB0, 0xFF},
		Accent:      color.NRGBA{0x16, 0x82, 0xB0, 0xFF},
		Accent2:     color.NRGBA{0xD6, 0x68, 0x8B, 0xFF},
		AccentDim:   color.NRGBA{0xDC, 0xF0, 0xFA, 0xFF},
		Success:     color.NRGBA{0x15, 0x85, 0x6D, 0xFF},
		Warning:     color.NRGBA{0xAD, 0x74, 0x10, 0xFF},
		Danger:      color.NRGBA{0xD4, 0x46, 0x65, 0xFF},
		Hover:       color.NRGBA{0xE4, 0xF1, 0xF9, 0xFF},
		Pressed:     color.NRGBA{0xD5, 0xE7, 0xF3, 0xFF},
		White:       flagWhite,
		Float:       color.NRGBA{0xFA, 0xFC, 0xFE, 0xFF},
		OnPrimary:   color.NRGBA{0xFF, 0xFE, 0xFF, 0xFF},
		ToneAccent:  color.NRGBA{0xE5, 0xF5, 0xFC, 0xFF},
		ToneSuccess: color.NRGBA{0xE3, 0xF7, 0xF0, 0xFF},
		ToneWarning: color.NRGBA{0xFC, 0xF2, 0xDE, 0xFF},
		ToneDanger:  color.NRGBA{0xFC, 0xE7, 0xEE, 0xFF},
	}

	// Obsidian — Dark
	obsidianDarkPalette = palette{
		BG:          color.NRGBA{0x0A, 0x0C, 0x12, 0xFF},
		Surface:     color.NRGBA{0x11, 0x14, 0x1F, 0xFF},
		Surface2:    color.NRGBA{0x18, 0x1C, 0x2B, 0xFF},
		Border:      color.NRGBA{0x26, 0x2C, 0x40, 0xFF},
		Text:        color.NRGBA{0xF3, 0xF4, 0xFA, 0xFF},
		Muted:       color.NRGBA{0x9D, 0xA6, 0xBE, 0xFF},
		Faint:       color.NRGBA{0x68, 0x71, 0x8C, 0xFF},
		Accent:      obsidianViolet,
		Accent2:     obsidianTeal,
		AccentDim:   color.NRGBA{0x23, 0x1C, 0x44, 0xFF},
		Success:     color.NRGBA{0x34, 0xD3, 0x99, 0xFF},
		Warning:     color.NRGBA{0xFB, 0xBF, 0x24, 0xFF},
		Danger:      color.NRGBA{0xF8, 0x71, 0x71, 0xFF},
		Hover:       color.NRGBA{0x20, 0x25, 0x38, 0xFF},
		Pressed:     color.NRGBA{0x2A, 0x31, 0x4B, 0xFF},
		White:       flagWhite,
		Float:       color.NRGBA{0x0D, 0x10, 0x18, 0xFF},
		OnPrimary:   color.NRGBA{0x09, 0x07, 0x14, 0xFF},
		ToneAccent:  color.NRGBA{0x1C, 0x18, 0x36, 0xFF},
		ToneSuccess: color.NRGBA{0x10, 0x2C, 0x24, 0xFF},
		ToneWarning: color.NRGBA{0x30, 0x25, 0x11, 0xFF},
		ToneDanger:  color.NRGBA{0x33, 0x16, 0x1C, 0xFF},
	}

	// Obsidian — Light
	obsidianLightPalette = palette{
		BG:          color.NRGBA{0xF7, 0xF7, 0xFB, 0xFF},
		Surface:     color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF},
		Surface2:    color.NRGBA{0xEF, 0xEE, 0xF8, 0xFF},
		Border:      color.NRGBA{0xD8, 0xD6, 0xE9, 0xFF},
		Text:        color.NRGBA{0x15, 0x16, 0x24, 0xFF},
		Muted:       color.NRGBA{0x5A, 0x5F, 0x76, 0xFF},
		Faint:       color.NRGBA{0x8B, 0x90, 0xA8, 0xFF},
		Accent:      color.NRGBA{0x6D, 0x28, 0xD9, 0xFF},
		Accent2:     color.NRGBA{0x0D, 0x94, 0x88, 0xFF},
		AccentDim:   color.NRGBA{0xE8, 0xE2, 0xFC, 0xFF},
		Success:     color.NRGBA{0x0F, 0x8A, 0x5F, 0xFF},
		Warning:     color.NRGBA{0xB4, 0x69, 0x0E, 0xFF},
		Danger:      color.NRGBA{0xDC, 0x26, 0x26, 0xFF},
		Hover:       color.NRGBA{0xE7, 0xE5, 0xF6, 0xFF},
		Pressed:     color.NRGBA{0xD9, 0xD6, 0xF0, 0xFF},
		White:       flagWhite,
		Float:       color.NRGBA{0xFA, 0xFA, 0xFE, 0xFF},
		OnPrimary:   color.NRGBA{0xFF, 0xFE, 0xFF, 0xFF},
		ToneAccent:  color.NRGBA{0xEE, 0xE9, 0xFD, 0xFF},
		ToneSuccess: color.NRGBA{0xE2, 0xF6, 0xEE, 0xFF},
		ToneWarning: color.NRGBA{0xFD, 0xF1, 0xDC, 0xFF},
		ToneDanger:  color.NRGBA{0xFD, 0xE8, 0xE8, 0xFF},
	}
)

var (
	activeFamily  = paletteTrans
	activeTheme   = themeDark
	activePalette = darkPalette

	colorBG        = activePalette.BG
	colorSurface   = activePalette.Surface
	colorSurface2  = activePalette.Surface2
	colorBorder    = activePalette.Border
	colorText      = activePalette.Text
	colorMuted     = activePalette.Muted
	colorFaint     = activePalette.Faint
	colorAccent    = activePalette.Accent
	colorAccent2   = activePalette.Accent2
	colorAccentDim = activePalette.AccentDim
	colorSuccess   = activePalette.Success
	colorWarning   = activePalette.Warning
	colorDanger    = activePalette.Danger
	colorHover     = activePalette.Hover
	colorPressed   = activePalette.Pressed
	colorWhite     = activePalette.White
	colorFloat     = activePalette.Float

	toneAccentBg  = activePalette.ToneAccent
	toneSuccessBg = activePalette.ToneSuccess
	toneWarningBg = activePalette.ToneWarning
	toneDangerBg  = activePalette.ToneDanger
)

func resolvePalette(family themePalette, mode appTheme) palette {
	if family == paletteObsidian {
		if mode == themeLight {
			return obsidianLightPalette
		}
		return obsidianDarkPalette
	}
	if mode == themeLight {
		return lightPalette
	}
	return darkPalette
}

func setTheme(family themePalette, mode appTheme) {
	p := resolvePalette(family, mode)
	activeFamily = family
	activeTheme = mode
	activePalette = p

	colorBG = p.BG
	colorSurface = p.Surface
	colorSurface2 = p.Surface2
	colorBorder = p.Border
	colorText = p.Text
	colorMuted = p.Muted
	colorFaint = p.Faint
	colorAccent = p.Accent
	colorAccent2 = p.Accent2
	colorAccentDim = p.AccentDim
	colorSuccess = p.Success
	colorWarning = p.Warning
	colorDanger = p.Danger
	colorHover = p.Hover
	colorPressed = p.Pressed
	colorWhite = p.White
	colorFloat = p.Float

	toneAccentBg = p.ToneAccent
	toneSuccessBg = p.ToneSuccess
	toneWarningBg = p.ToneWarning
	toneDangerBg = p.ToneDanger
}

func setThemePalette(v appTheme) {
	setTheme(activeFamily, v)
}

func themeFromConfig(saved string, osVariant fyne.ThemeVariant) appTheme {
	switch saved {
	case "light":
		return themeLight
	case "dark":
		return themeDark
	}
	if osVariant == theme.VariantLight {
		return themeLight
	}
	return themeDark
}

type studioTheme struct{}

func (studioTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colorBG
	case theme.ColorNameForeground:
		return colorText
	case theme.ColorNameButton:
		return colorSurface2
	case theme.ColorNameDisabled:
		return colorFaint
	case theme.ColorNameDisabledButton:
		return colorSurface
	case theme.ColorNameError:
		return colorDanger
	case theme.ColorNameForegroundOnError:
		return colorWhite
	case theme.ColorNameSuccess:
		return colorSuccess
	case theme.ColorNameForegroundOnSuccess:
		return colorBG
	case theme.ColorNameWarning:
		return colorWarning
	case theme.ColorNameForegroundOnWarning:
		return colorBG
	case theme.ColorNameHover:
		return colorHover
	case theme.ColorNamePressed:
		return colorPressed
	case theme.ColorNameInputBackground:
		return colorBG
	case theme.ColorNameInputBorder:
		return colorBorder
	case theme.ColorNamePlaceHolder:
		return colorFaint
	case theme.ColorNamePrimary:
		return colorAccent
	case theme.ColorNameForegroundOnPrimary:
		return activePalette.OnPrimary
	case theme.ColorNameFocus:
		return focusColor()
	case theme.ColorNameSelection:
		return colorAccentDim
	case theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x32}
	case theme.ColorNameScrollBar:
		return colorBorder
	case theme.ColorNameScrollBarBackground:
		return color.Transparent
	case theme.ColorNameMenuBackground:
		return colorFloat
	case theme.ColorNameOverlayBackground:
		c := colorSurface
		c.A = 0xF5
		return c
	case theme.ColorNameHeaderBackground:
		return colorSurface
	case theme.ColorNameHyperlink:
		return colorAccent
	}
	if activeTheme == themeLight {
		return theme.DefaultTheme().Color(n, theme.VariantLight)
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (studioTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (studioTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	switch n {
	case theme.IconNameCheckButton:
		return checkEmptyIcon
	case theme.IconNameCheckButtonChecked:
		return checkCheckedIcon
	case theme.IconNameCheckButtonFill:
		return checkFillIcon
	case theme.IconNameArrowDropDown:
		return chevronDownIcon
	case theme.IconNameSearch:
		return iconSearch
	case theme.IconNameCancel:
		return iconClose
	case theme.IconNameContentAdd:
		return iconPlus
	case theme.IconNameViewRefresh:
		return iconRefresh
	case theme.IconNameDelete:
		return iconTrash
	case theme.IconNameDocumentCreate:
		return iconEdit
	case theme.IconNameMailSend:
		return iconSend
	case theme.IconNameSettings:
		return iconSliders
	case theme.IconNameHelp:
		return iconHelp
	case theme.IconNameLogout:
		return iconLogout
	case theme.IconNameMediaPlay:
		return iconPlay
	case theme.IconNameMediaStop:
		return iconStop
	case theme.IconNameNavigateBack:
		return iconArrowLeft
	case theme.IconNameNavigateNext:
		return iconArrowRight
	case theme.IconNameUpload:
		return iconUpload
	case theme.IconNameContentCopy:
		return iconCopy
	case theme.IconNameInfo:
		return iconInfo
	case theme.IconNameAccount:
		return iconAgent
	case theme.IconNameStorage:
		return iconChannels
	case theme.IconNameList:
		return iconModels
	case theme.IconNameDocument:
		return iconCode
	case theme.IconNameFileText:
		return iconFileText
	case theme.IconNameFolderOpen:
		return iconFolder
	case theme.IconNameDocumentSave:
		return iconSave
	case theme.IconNameVisibility:
		return iconEye
	case theme.IconNameVisibilityOff:
		return iconEyeOff
	}
	return theme.DefaultTheme().Icon(n)
}

func (studioTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 5
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 21
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInlineIcon:
		return 15
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameSelectionRadius:
		return 7
	case theme.SizeNameButtonRadius:
		return 8
	case theme.SizeNameCardRadius:
		return 10
	case theme.SizeNamePopupRadius:
		return 10
	case theme.SizeNameMenuRadius:
		return 10
	case theme.SizeNameDialogRadius:
		return 12
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameScrollBar:
		return 6
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameScrollBarRadius:
		return 3
	case theme.SizeNameModalBlurRadius:
		return 0
	}
	return theme.DefaultTheme().Size(n)
}

// borderlessEntryTheme strips the inner box border and background from an
// Entry when it is embedded inside a custom composite frame (such as searchBox
// or the Playground prompt composer).
type borderlessEntryTheme struct{ studioTheme }

func (borderlessEntryTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground, theme.ColorNameInputBorder, theme.ColorNameFocus:
		return color.Transparent
	}
	return studioTheme{}.Color(n, v)
}

func (borderlessEntryTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameInputBorder {
		return 0
	}
	return studioTheme{}.Size(n)
}

// applyStudioTheme installs the selected palette and window icon.
func applyStudioTheme(a fyne.App, family themePalette, v appTheme) {
	setTheme(family, v)
	a.Settings().SetTheme(studioTheme{})
	a.SetIcon(appIconResource())
}

// Minimalist Lucide-inspired 1.75px vector icon suite.
var (
	checkEmptyIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-check-empty.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M7 3.25h10A3.75 3.75 0 0 1 20.75 7v10A3.75 3.75 0 0 1 17 20.75H7A3.75 3.75 0 0 1 3.25 17V7A3.75 3.75 0 0 1 7 3.25zm0 1.5A2.25 2.25 0 0 0 4.75 7v10A2.25 2.25 0 0 0 7 19.25h10A2.25 2.25 0 0 0 19.25 17V7A2.25 2.25 0 0 0 17 4.75H7z"/>
</svg>`)))
	checkCheckedIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-check-checked.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M7 3.25h10A3.75 3.75 0 0 1 20.75 7v10A3.75 3.75 0 0 1 17 20.75H7A3.75 3.75 0 0 1 3.25 17V7A3.75 3.75 0 0 1 7 3.25zm9.28 5.97a.85.85 0 0 1 0 1.2l-5.4 5.4a.85.85 0 0 1-1.2 0l-2.46-2.46a.85.85 0 1 1 1.2-1.2l1.86 1.86 4.8-4.8a.85.85 0 0 1 1.2 0z"/>
</svg>`)))
	checkFillIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-check-fill.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"></svg>`)))
	checkMarkIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-check-mark.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M19.53 6.47a.85.85 0 0 1 0 1.2l-9.2 9.2a.85.85 0 0 1-1.2 0l-4.66-4.66a.85.85 0 1 1 1.2-1.2l4.06 4.06 8.6-8.6a.85.85 0 0 1 1.2 0z"/>
</svg>`)))
	chevronDownIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-chevron-down.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M6.47 9.47a.85.85 0 0 1 1.2 0L12 13.8l4.33-4.33a.85.85 0 1 1 1.2 1.2l-4.93 4.93a.85.85 0 0 1-1.2 0L6.47 10.67a.85.85 0 0 1 0-1.2z"/>
</svg>`)))
	iconEye = theme.NewThemedResource(fyne.NewStaticResource("studio-eye.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 5.25c-4.8 0-8.62 3.16-10.17 6.4a.85.85 0 0 0 0 .7C3.38 15.59 7.2 18.75 12 18.75s8.62-3.16 10.17-6.4a.85.85 0 0 0 0-.7C20.62 8.41 16.8 5.25 12 5.25zM3.58 12C5 9.35 8.16 6.95 12 6.95s7 2.4 8.42 5.05C19 14.65 15.84 17.05 12 17.05S5 14.65 3.58 12zm8.42-3.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7zm-1.8 3.5a1.8 1.8 0 1 1 3.6 0 1.8 1.8 0 0 1-3.6 0z"/>
</svg>`)))
	iconEyeOff = theme.NewThemedResource(fyne.NewStaticResource("studio-eye-off.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M3.47 3.47a.85.85 0 0 1 1.2 0l15.86 15.86a.85.85 0 1 1-1.2 1.2l-2.6-2.6A11.3 11.3 0 0 1 12 18.75c-4.8 0-8.62-3.16-10.17-6.4a.85.85 0 0 1 0-.7 11.9 11.9 0 0 1 4.18-4.88L3.47 4.67a.85.85 0 0 1 0-1.2zm3.76 4.96a10.2 10.2 0 0 0-3.65 3.57C5 14.65 8.16 17.05 12 17.05c1.27 0 2.48-.26 3.58-.73l-1.57-1.57a3.5 3.5 0 0 1-4.76-4.76L7.23 8.43zM12 5.25c4.8 0 8.62 3.16 10.17 6.4a.85.85 0 0 1 0 .7 12.1 12.1 0 0 1-2.68 3.68.85.85 0 1 1-1.18-1.22c.92-.89 1.65-1.86 2.11-2.81C19 9.35 15.84 6.95 12 6.95c-.75 0-1.48.1-2.18.28a.85.85 0 1 1-.43-1.64c.84-.22 1.71-.34 2.61-.34z"/>
</svg>`)))
	iconSearch = theme.NewThemedResource(fyne.NewStaticResource("studio-search.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M11 4.25a6.75 6.75 0 1 0 4.2 12.04l4.26 4.26a.85.85 0 1 0 1.2-1.2l-4.26-4.26A6.75 6.75 0 0 0 11 4.25zM5.95 11a5.05 5.05 0 1 1 10.1 0 5.05 5.05 0 0 1-10.1 0z"/>
</svg>`)))
	iconClose = theme.NewThemedResource(fyne.NewStaticResource("studio-close.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M6.47 6.47a.85.85 0 0 1 1.2 0L12 10.8l4.33-4.33a.85.85 0 1 1 1.2 1.2L13.2 12l4.33 4.33a.85.85 0 1 1-1.2 1.2L12 13.2l-4.33 4.33a.85.85 0 1 1-1.2-1.2L10.8 12 6.47 7.67a.85.85 0 0 1 0-1.2z"/>
</svg>`)))
	iconPlus = theme.NewThemedResource(fyne.NewStaticResource("studio-plus.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M12 4.5a.85.85 0 0 1 .85.85v5.8h5.8a.85.85 0 1 1 0 1.7h-5.8v5.8a.85.85 0 1 1-1.7 0v-5.8h-5.8a.85.85 0 1 1 0-1.7h5.8v-5.8A.85.85 0 0 1 12 4.5z"/>
</svg>`)))
	iconRefresh = theme.NewThemedResource(fyne.NewStaticResource("studio-refresh.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M12 4.25a7.75 7.75 0 0 1 6.45 3.45V5.5a.85.85 0 1 1 1.7 0v4.2a.85.85 0 0 1-.85.85h-4.2a.85.85 0 1 1 0-1.7h2.25A6.05 6.05 0 1 0 18.02 13a.85.85 0 1 1 1.7.15A7.75 7.75 0 1 1 12 4.25z"/>
</svg>`)))
	iconTrash = theme.NewThemedResource(fyne.NewStaticResource("studio-trash.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M9.5 3.25h5a1.75 1.75 0 0 1 1.75 1.75v.75h3.5a.85.85 0 1 1 0 1.7h-1.08l-.75 11.3A2.5 2.5 0 0 1 15.43 21H8.57a2.5 2.5 0 0 1-2.49-2.25l-.75-11.3H4.25a.85.85 0 1 1 0-1.7h3.5V5A1.75 1.75 0 0 1 9.5 3.25zm5 2.5V5h-5v.75h5zM7.04 7.45l.73 11.15a.8.8 0 0 0 .8.7h6.86a.8.8 0 0 0 .8-.7l.73-11.15H7.04zm3.21 2.55a.85.85 0 0 1 .85.85v5a.85.85 0 1 1-1.7 0v-5a.85.85 0 0 1 .85-.85zm3.5 0a.85.85 0 0 1 .85.85v5a.85.85 0 1 1-1.7 0v-5a.85.85 0 0 1 .85-.85z"/>
</svg>`)))
	iconEdit = theme.NewThemedResource(fyne.NewStaticResource("studio-edit.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M16.86 3.88a2.4 2.4 0 0 1 3.4 3.4L9.5 18.04a2 2 0 0 1-.88.51l-3.85.96a.85.85 0 0 1-1.03-1.03l.96-3.85a2 2 0 0 1 .51-.88L16.86 3.88zm2.2 1.2a.7.7 0 0 0-1 0L6.42 14.72l-.65 2.61 2.61-.65L19.06 6.08a.7.7 0 0 0 0-1z"/>
</svg>`)))
	iconSend = theme.NewThemedResource(fyne.NewStaticResource("studio-send.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M20.66 3.34a1.25 1.25 0 0 1 .27 1.36l-5.85 15.2a1.25 1.25 0 0 1-2.3.06l-2.66-5.9-5.9-2.66a1.25 1.25 0 0 1 .06-2.3l15.2-5.85a1.25 1.25 0 0 1 1.18.09zM6.56 10.22l4.26 1.92 4.3-4.3a.85.85 0 1 1 1.2 1.2l-4.3 4.3 1.92 4.26 4.46-11.58-11.84 4.2z"/>
</svg>`)))
	iconSliders = theme.NewThemedResource(fyne.NewStaticResource("studio-sliders.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M9.5 4.25a2.75 2.75 0 0 1 2.62 1.9h7.63a.85.85 0 1 1 0 1.7h-7.63a2.75 2.75 0 0 1-5.24 0H4.25a.85.85 0 1 1 0-1.7h2.63A2.75 2.75 0 0 1 9.5 4.25zm0 1.7a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1zm5.5 3.3a2.75 2.75 0 0 1 2.62 1.9h2.13a.85.85 0 1 1 0 1.7h-2.13a2.75 2.75 0 0 1-5.24 0H4.25a.85.85 0 1 1 0-1.7h8.13A2.75 2.75 0 0 1 15 9.25zm0 1.7a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1zm-6.5 3.3a2.75 2.75 0 0 1 2.62 1.9h8.63a.85.85 0 1 1 0 1.7h-8.63a2.75 2.75 0 0 1-5.24 0H4.25a.85.85 0 1 1 0-1.7h1.63A2.75 2.75 0 0 1 8.5 14.25zm0 1.7a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1z"/>
</svg>`)))
	iconHelp = theme.NewThemedResource(fyne.NewStaticResource("studio-help.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 3.25a8.75 8.75 0 1 0 0 17.5 8.75 8.75 0 0 0 0-17.5zM4.95 12a7.05 7.05 0 1 1 14.1 0 7.05 7.05 0 0 1-14.1 0zm7.05-3.9a1.45 1.45 0 0 0-1.4.95.85.85 0 1 1-1.6-.58 3.15 3.15 0 1 1 4.46 3.85c-.46.23-.61.48-.61.88a.85.85 0 1 1-1.7 0c0-1.15.65-1.92 1.56-2.38a1.45 1.45 0 0 0-.71-2.72zm0 7.5a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"/>
</svg>`)))
	iconLogout = theme.NewThemedResource(fyne.NewStaticResource("studio-logout.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M9.5 4.25a.85.85 0 0 1 0 1.7H6.25a.8.8 0 0 0-.8.8v10.5a.8.8 0 0 0 .8.8H9.5a.85.85 0 0 1 0 1.7H6.25a2.5 2.5 0 0 1-2.5-2.5V6.75a2.5 2.5 0 0 1 2.5-2.5H9.5zm6.47 3.72a.85.85 0 0 1 1.2 0l3.43 3.43a.85.85 0 0 1 0 1.2l-3.43 3.43a.85.85 0 1 1-1.2-1.2l1.98-1.98H9.5a.85.85 0 1 1 0-1.7h8.45l-1.98-1.98a.85.85 0 0 1 0-1.2z"/>
</svg>`)))
	iconPlay = theme.NewThemedResource(fyne.NewStaticResource("studio-play.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M7.05 4.65a1.5 1.5 0 0 1 2.28-1.28l10.2 6.35a1.5 1.5 0 0 1 0 2.56l-10.2 6.35a1.5 1.5 0 0 1-2.28-1.28V4.65zm1.7.36v11.98L17.97 11 8.75 5.01z"/>
</svg>`)))
	iconStop = theme.NewThemedResource(fyne.NewStaticResource("studio-stop.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M7 5.25h10A1.75 1.75 0 0 1 18.75 7v10A1.75 1.75 0 0 1 17 18.75H7A1.75 1.75 0 0 1 5.25 17V7A1.75 1.75 0 0 1 7 5.25zm0 1.7v10.1h10V6.95H7z"/>
</svg>`)))
	iconArrowLeft = theme.NewThemedResource(fyne.NewStaticResource("studio-arrow-left.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M10.88 5.97a.85.85 0 0 1 0 1.2L6.9 11.15h12.35a.85.85 0 1 1 0 1.7H6.9l3.98 3.98a.85.85 0 1 1-1.2 1.2l-5.43-5.43a.85.85 0 0 1 0-1.2l5.43-5.43a.85.85 0 0 1 1.2 0z"/>
</svg>`)))
	iconArrowRight = theme.NewThemedResource(fyne.NewStaticResource("studio-arrow-right.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M13.12 5.97a.85.85 0 0 1 1.2 0l5.43 5.43a.85.85 0 0 1 0 1.2l-5.43 5.43a.85.85 0 1 1-1.2-1.2l3.98-3.98H4.75a.85.85 0 1 1 0-1.7H17.1l-3.98-3.98a.85.85 0 0 1 0-1.2z"/>
</svg>`)))
	iconUpload = theme.NewThemedResource(fyne.NewStaticResource("studio-upload.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M11.4 4.47a.85.85 0 0 1 1.2 0l4.43 4.43a.85.85 0 1 1-1.2 1.2l-2.98-2.98v8.13a.85.85 0 1 1-1.7 0V7.12L8.17 10.1a.85.85 0 1 1-1.2-1.2l4.43-4.43zM5 18.15a.85.85 0 1 0 0 1.7h14a.85.85 0 1 0 0-1.7H5z"/>
</svg>`)))
	iconCopy = theme.NewThemedResource(fyne.NewStaticResource("studio-copy.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M9.5 3.25h7.75A2.5 2.5 0 0 1 19.75 5.75v7.75a2.5 2.5 0 0 1-2.5 2.5H16v1.25a2.5 2.5 0 0 1-2.5 2.5H5.75a2.5 2.5 0 0 1-2.5-2.5V9.5A2.5 2.5 0 0 1 5.75 7H7V5.75A2.5 2.5 0 0 1 9.5 3.25zM8.7 7h4.8A2.5 2.5 0 0 1 16 9.5v4.8h1.25a.8.8 0 0 0 .8-.8V5.75a.8.8 0 0 0-.8-.8H9.5a.8.8 0 0 0-.8.8V7zM5.75 8.7a.8.8 0 0 0-.8.8v7.75a.8.8 0 0 0 .8.8h7.75a.8.8 0 0 0 .8-.8V9.5a.8.8 0 0 0-.8-.8H5.75z"/>
</svg>`)))
	iconInfo = theme.NewThemedResource(fyne.NewStaticResource("studio-info.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 3.25a8.75 8.75 0 1 0 0 17.5 8.75 8.75 0 0 0 0-17.5zM4.95 12a7.05 7.05 0 1 1 14.1 0 7.05 7.05 0 0 1-14.1 0zm7.05-4.3a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm0 3.45a.85.85 0 0 1 .85.85v4a.85.85 0 1 1-1.7 0v-4a.85.85 0 0 1 .85-.85z"/>
</svg>`)))
	// iconUser is a person: profiles and tenants, distinct from iconAgent.
	iconUser = theme.NewThemedResource(fyne.NewStaticResource("studio-user.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 3.25a4.25 4.25 0 1 1 0 8.5 4.25 4.25 0 0 1 0-8.5zm0 1.7a2.55 2.55 0 1 0 0 5.1 2.55 2.55 0 0 0 0-5.1zM4.25 19.5c0-3.3 3.1-5.75 7.75-5.75s7.75 2.45 7.75 5.75a.85.85 0 1 1-1.7 0c0-2.1-2.15-4.05-6.05-4.05S5.95 17.4 5.95 19.5a.85.85 0 1 1-1.7 0z"/>
</svg>`)))
	iconAgent = theme.NewThemedResource(fyne.NewStaticResource("studio-agent.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 2.75a.85.85 0 0 1 .85.85v1.65h4.4A2.75 2.75 0 0 1 20 8v8.5a2.75 2.75 0 0 1-2.75 2.75H6.75A2.75 2.75 0 0 1 4 16.5V8a2.75 2.75 0 0 1 2.75-2.75h4.4V3.6a.85.85 0 0 1 .85-.85zM6.75 6.95A1.05 1.05 0 0 0 5.7 8v8.5c0 .58.47 1.05 1.05 1.05h10.5c.58 0 1.05-.47 1.05-1.05V8c0-.58-.47-1.05-1.05-1.05H6.75zM9 10.25a1.25 1.25 0 1 0 0 2.5 1.25 1.25 0 0 0 0-2.5zm6 0a1.25 1.25 0 1 0 0 2.5 1.25 1.25 0 0 0 0-2.5zm-5.25 4.2h4.5a.85.85 0 1 1 0 1.7h-4.5a.85.85 0 1 1 0-1.7z"/>
</svg>`)))
	iconChannels = theme.NewThemedResource(fyne.NewStaticResource("studio-channels.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M6.5 3.75a2.75 2.75 0 1 0 .85 5.37v5.76a2.75 2.75 0 1 0 1.7 0V9.12a2.75 2.75 0 0 0-.85-5.37h-1.7zm0 1.7a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1zm11 1.8a2.75 2.75 0 0 0-2.62 1.9H11.5a.85.85 0 1 0 0 1.7h3.38a2.75 2.75 0 1 0 2.62-3.6zm0 1.7a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1zm-11 7.5a1.05 1.05 0 1 0 0 2.1 1.05 1.05 0 0 0 0-2.1z"/>
</svg>`)))
	iconModels = theme.NewThemedResource(fyne.NewStaticResource("studio-models.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M11.62 3.34a.85.85 0 0 1 .76 0l8 4a.85.85 0 0 1 0 1.52l-8 4a.85.85 0 0 1-.76 0l-8-4a.85.85 0 0 1 0-1.52l8-4zM5.8 8.1 12 11.2l6.2-3.1L12 5 5.8 8.1zm-1.8 3.64a.85.85 0 0 1 1.14-.38L12 14.8l6.86-3.44a.85.85 0 1 1 .76 1.52l-7.24 3.62a.85.85 0 0 1-.76 0l-7.24-3.62a.85.85 0 0 1-.38-1.14zm0 4.2a.85.85 0 0 1 1.14-.38L12 19l6.86-3.44a.85.85 0 1 1 .76 1.52l-7.24 3.62a.85.85 0 0 1-.76 0l-7.24-3.62a.85.85 0 0 1-.38-1.14z"/>
</svg>`)))
	iconCode = theme.NewThemedResource(fyne.NewStaticResource("studio-code.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M8.83 6.97a.85.85 0 0 1 0 1.2L4.99 12l3.84 3.83a.85.85 0 1 1-1.2 1.2l-4.43-4.43a.85.85 0 0 1 0-1.2l4.43-4.43a.85.85 0 0 1 1.2 0zm6.34 0a.85.85 0 0 1 1.2 0l4.43 4.43a.85.85 0 0 1 0 1.2l-4.43 4.43a.85.85 0 1 1-1.2-1.2L19.01 12l-3.84-3.83a.85.85 0 0 1 0-1.2z"/>
</svg>`)))
	iconFileText = theme.NewThemedResource(fyne.NewStaticResource("studio-file-text.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M6.75 3.25h6.6a.85.85 0 0 1 .6.25l4.55 4.55a.85.85 0 0 1 .25.6v9.6a2.5 2.5 0 0 1-2.5 2.5H6.75a2.5 2.5 0 0 1-2.5-2.5V5.75a2.5 2.5 0 0 1 2.5-2.5zm6 1.7H6.75a.8.8 0 0 0-.8.8v12.5a.8.8 0 0 0 .8.8h9.5a.8.8 0 0 0 .8-.8V9.25h-3.5a.85.85 0 0 1-.85-.85V4.95zm1.7.7v2.05h2.05l-2.05-2.05zM8.5 12.15a.85.85 0 0 1 .85-.85h5.3a.85.85 0 1 1 0 1.7h-5.3a.85.85 0 0 1-.85-.85zm0 3.5a.85.85 0 0 1 .85-.85h3.5a.85.85 0 1 1 0 1.7h-3.5a.85.85 0 0 1-.85-.85z"/>
</svg>`)))
	iconFolder = theme.NewThemedResource(fyne.NewStaticResource("studio-folder.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M4.25 4.75h5.1c.48 0 .93.22 1.22.6l1.28 1.65h7.9A2.25 2.25 0 0 1 22 9.25v8.5A2.25 2.25 0 0 1 19.75 20H4.25A2.25 2.25 0 0 1 2 17.75v-10.75A2.25 2.25 0 0 1 4.25 4.75zm0 1.7a.55.55 0 0 0-.55.55v10.75c0 .3.25.55.55.55h15.5c.3 0 .55-.25.55-.55v-8.5a.55.55 0 0 0-.55-.55h-8.32a.85.85 0 0 1-.67-.33L9.28 6.45H4.25z"/>
</svg>`)))
	iconSave = theme.NewThemedResource(fyne.NewStaticResource("studio-save.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M5.75 3.25h9.6a.85.85 0 0 1 .6.25l3.55 3.55a.85.85 0 0 1 .25.6v10.6a2.5 2.5 0 0 1-2.5 2.5H5.75a2.5 2.5 0 0 1-2.5-2.5V5.75a2.5 2.5 0 0 1 2.5-2.5zm9.25 1.7H8.5v3.3h6.5v-3.3zm1.7.7v3.45a.85.85 0 0 1-.85.85H7.65a.85.85 0 0 1-.85-.85V4.95H5.75a.8.8 0 0 0-.8.8v12.5a.8.8 0 0 0 .8.8h1.05v-5.3a.85.85 0 0 1 .85-.85h7.7a.85.85 0 0 1 .85.85v5.3h1.05a.8.8 0 0 0 .8-.8V8.01L16.7 5.65zM14.5 19.05v-4.45h-6v4.45h6z"/>
</svg>`)))
	paletteIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-palette.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" d="M12 3a9 9 0 0 0 0 18c.83 0 1.5-.67 1.5-1.5 0-.39-.15-.74-.39-1.01-.23-.26-.38-.61-.38-.99 0-.83.67-1.5 1.5-1.5H16c2.76 0 5-2.24 5-5 0-4.42-4.03-8-9-8zm-5.5 9c-.83 0-1.5-.67-1.5-1.5S5.67 9 6.5 9 8 9.67 8 10.5 7.33 12 6.5 12zm3-4C8.67 8 8 7.33 8 6.5S8.67 5 9.5 5s1.5.67 1.5 1.5S10.33 8 9.5 8zm5 0c-.83 0-1.5-.67-1.5-1.5S13.67 5 14.5 5s1.5.67 1.5 1.5S15.33 8 14.5 8zm3 4c-.83 0-1.5-.67-1.5-1.5S16.67 9 17.5 9s1.5.67 1.5 1.5-.67 1.5-1.5 1.5z"/>
</svg>`)))
)

// focusColor is a subtle field highlight so focused inputs and open selects
// keep a minimalist aesthetic instead of turning into a solid accent slab.
func focusColor() color.Color {
	return colorHover
}

// lightModeIcon and darkModeIcon are minimalist sun/moon glyphs.
var (
	lightModeIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-sun.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M12 7.25a4.75 4.75 0 1 0 0 9.5 4.75 4.75 0 0 0 0-9.5zM8.95 12a3.05 3.05 0 1 1 6.1 0 3.05 3.05 0 0 1-6.1 0zM12 2.25a.85.85 0 0 1 .85.85v1.8a.85.85 0 1 1-1.7 0V3.1a.85.85 0 0 1 .85-.85zm0 16a.85.85 0 0 1 .85.85v1.8a.85.85 0 1 1-1.7 0v-1.8a.85.85 0 0 1 .85-.85zM5.1 5.1a.85.85 0 0 1 1.2 0l1.27 1.27a.85.85 0 1 1-1.2 1.2L5.1 6.3a.85.85 0 0 1 0-1.2zm11.33 11.33a.85.85 0 0 1 1.2 0l1.27 1.27a.85.85 0 1 1-1.2 1.2l-1.27-1.27a.85.85 0 0 1 0-1.2zM2.25 12a.85.85 0 0 1 .85-.85h1.8a.85.85 0 1 1 0 1.7H3.1a.85.85 0 0 1-.85-.85zm16 0a.85.85 0 0 1 .85-.85h1.8a.85.85 0 1 1 0 1.7h-1.8a.85.85 0 0 1-.85-.85zM6.37 16.43a.85.85 0 0 1 1.2 1.2L6.3 18.9a.85.85 0 1 1-1.2-1.2l1.27-1.27zm11.33-11.33a.85.85 0 0 1 1.2 1.2l-1.27 1.27a.85.85 0 1 1-1.2-1.2l1.27-1.27z"/>
</svg>`)))
	darkModeIcon = theme.NewThemedResource(fyne.NewStaticResource("studio-moon.svg", []byte(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#000" fill-rule="evenodd" d="M11.6 3.3a.85.85 0 0 1 .87 1.15 6.8 6.8 0 0 0 8.08 8.08.85.85 0 0 1 1.04 1.01A8.75 8.75 0 1 1 10.46 3.4a.85.85 0 0 1 1.14-.1zM10.3 5.25a7.05 7.05 0 1 0 8.45 8.45 8.5 8.5 0 0 1-8.45-8.45z"/>
</svg>`)))
)

const logoDarkSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#5BCEFA"/><stop offset="0.5" stop-color="#FFFFFF"/><stop offset="1" stop-color="#F5A9B8"/>
</linearGradient></defs>
<rect x="3" y="3" width="122" height="122" rx="30" fill="#111927" stroke="#25344B" stroke-width="3"/>
<path d="M64 26 L100 102 H86 L64 60 L42 102 H28 Z" fill="url(#g)"/>
</svg>`

const logoLightSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#1682B0"/><stop offset="0.5" stop-color="#F5A9B8"/><stop offset="1" stop-color="#D6688B"/>
</linearGradient></defs>
<rect x="3" y="3" width="122" height="122" rx="30" fill="#FFFFFF" stroke="#CFE0EC" stroke-width="3"/>
<path d="M64 26 L100 102 H86 L64 60 L42 102 H28 Z" fill="url(#g)"/>
</svg>`

const logoObsidianDarkSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#9D7BFF"/><stop offset="0.5" stop-color="#E2E8F0"/><stop offset="1" stop-color="#2DD4BF"/>
</linearGradient></defs>
<rect x="3" y="3" width="122" height="122" rx="30" fill="#11141F" stroke="#262C40" stroke-width="3"/>
<path d="M64 26 L100 102 H86 L64 60 L42 102 H28 Z" fill="url(#g)"/>
</svg>`

const logoObsidianLightSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#6D28D9"/><stop offset="0.5" stop-color="#9D7BFF"/><stop offset="1" stop-color="#0D9488"/>
</linearGradient></defs>
<rect x="3" y="3" width="122" height="122" rx="30" fill="#FFFFFF" stroke="#D8D6E9" stroke-width="3"/>
<path d="M64 26 L100 102 H86 L64 60 L42 102 H28 Z" fill="url(#g)"/>
</svg>`

var (
	logoDarkResource          = fyne.NewStaticResource("agentflow-logo-dark.svg", []byte(logoDarkSVG))
	logoLightResource         = fyne.NewStaticResource("agentflow-logo-light.svg", []byte(logoLightSVG))
	logoObsidianDarkResource  = fyne.NewStaticResource("agentflow-logo-obsidian-dark.svg", []byte(logoObsidianDarkSVG))
	logoObsidianLightResource = fyne.NewStaticResource("agentflow-logo-obsidian-light.svg", []byte(logoObsidianLightSVG))
)

func logoResource() fyne.Resource {
	if activeFamily == paletteObsidian {
		if activeTheme == themeLight {
			return logoObsidianLightResource
		}
		return logoObsidianDarkResource
	}
	if activeTheme == themeLight {
		return logoLightResource
	}
	return logoDarkResource
}

func appIconResource() fyne.Resource {
	if activeFamily == paletteObsidian {
		return fyne.NewStaticResource("agentflow-studio.svg", []byte(logoObsidianDarkSVG))
	}
	return fyne.NewStaticResource("agentflow-studio.svg", []byte(logoDarkSVG))
}

// themeTint returns the five accent bands for the shell stripe.
func themeTint() []color.Color {
	if activeFamily == paletteObsidian {
		mid := color.NRGBA{R: 0x65, G: 0xA8, B: 0xDF, A: 0xFF}
		center := colorAccent2
		if activeTheme == themeLight {
			mid = color.NRGBA{R: 0x4F, G: 0x63, B: 0xB6, A: 0xFF}
		}
		return []color.Color{colorAccent, mid, center, mid, colorAccent}
	}
	middle := color.Color(colorWhite)
	if activeTheme == themeLight {
		middle = color.NRGBA{R: 0xCB, G: 0xDD, B: 0xEA, A: 0xFF}
	}
	return []color.Color{colorAccent, colorAccent2, middle, colorAccent2, colorAccent}
}

// transStripe is the five-band accent bar used in the shell.
type transStripe struct {
	widget.BaseWidget
	minWidth float32
	height   float32
}

func newTransStripe(minWidth, height float32) *transStripe {
	s := &transStripe{minWidth: minWidth, height: height}
	s.ExtendBaseWidget(s)
	return s
}

func (s *transStripe) MinSize() fyne.Size { return fyne.NewSize(s.minWidth, s.height) }

type transStripeRenderer struct {
	stripe *transStripe
	bands  []*canvas.Rectangle
}

func (s *transStripe) CreateRenderer() fyne.WidgetRenderer {
	r := &transStripeRenderer{stripe: s}
	for _, c := range themeTint() {
		r.bands = append(r.bands, canvas.NewRectangle(c))
	}
	return r
}

func (r *transStripeRenderer) Layout(size fyne.Size) {
	if len(r.bands) == 0 {
		return
	}
	w := size.Width / float32(len(r.bands))
	for i, band := range r.bands {
		band.Resize(fyne.NewSize(w, size.Height))
		band.Move(fyne.NewPos(w*float32(i), 0))
	}
}

func (r *transStripeRenderer) MinSize() fyne.Size { return r.stripe.MinSize() }

func (r *transStripeRenderer) Refresh() {
	for i, c := range themeTint() {
		if i < len(r.bands) {
			r.bands[i].FillColor = c
			r.bands[i].Refresh()
		}
	}
}

func (r *transStripeRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(r.bands))
	for i, band := range r.bands {
		objs[i] = band
	}
	return objs
}

func (r *transStripeRenderer) Destroy() {}

// ---- building blocks -------------------------------------------------------

type tone int

const (
	toneNeutral tone = iota
	toneAccent
	toneSuccess
	toneWarning
	toneDanger
)

func toneColor(t tone) color.Color {
	switch t {
	case toneAccent:
		return colorAccent
	case toneSuccess:
		return colorSuccess
	case toneWarning:
		return colorWarning
	case toneDanger:
		return colorDanger
	}
	return colorMuted
}

func toneBackground(t tone) color.Color {
	switch t {
	case toneAccent:
		return toneAccentBg
	case toneSuccess:
		return toneSuccessBg
	case toneWarning:
		return toneWarningBg
	case toneDanger:
		return toneDangerBg
	}
	return colorSurface2
}

// panelPadding is the interior spacing for cards and inspector sections.
const panelPadding = 12

const (
	pageGutter = 20
	pageTop    = 14
	pageBottom = 14
	cardRadius = 10
)

// pageInset is the standard page margin.
func pageInset(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(pageTop, pageBottom, pageGutter, pageGutter), obj)
}

// hInset aligns full-width furniture to the page gutter.
func hInset(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(0, 0, pageGutter, pageGutter), obj)
}

// paneInset is the interior padding for a split-pane surface.
func paneInset(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(spaceM, spaceM, spaceM, spaceM), obj)
}

// compactLabelLayout trims the excess top/bottom (and optional leading)
// InnerPadding that Fyne's widget.Label adds via RichText, so field labels and
// section subtitles sit cleanly aligned without tall vertical gaps.
type compactLabelLayout struct {
	trimX bool
}

func (l compactLabelLayout) vTrim() float32 {
	pad := float32(6)
	if app := fyne.CurrentApp(); app != nil && app.Settings() != nil {
		pad = app.Settings().Theme().Size(theme.SizeNameInnerPadding)
	}
	if pad > 1 {
		return pad - 1
	}
	return 0
}

func (l compactLabelLayout) hTrim() float32 {
	if !l.trimX {
		return 0
	}
	pad := float32(6)
	if app := fyne.CurrentApp(); app != nil && app.Settings() != nil {
		pad = app.Settings().Theme().Size(theme.SizeNameInnerPadding)
	}
	if pad > 2 {
		return pad - 2
	}
	return 0
}

func (l compactLabelLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	min := objs[0].MinSize()
	vt := l.vTrim() * 2
	ht := l.hTrim()
	h := min.Height - vt
	if h < 12 {
		h = 12
	}
	w := min.Width - ht
	if w < 0 {
		w = 0
	}
	return fyne.NewSize(w, h)
}

func (l compactLabelLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	vt := l.vTrim()
	ht := l.hTrim()
	for _, obj := range objs {
		obj.Resize(fyne.NewSize(size.Width+ht, size.Height+vt*2))
		obj.Move(fyne.NewPos(-ht, -vt))
	}
}

func compactLabel(obj fyne.CanvasObject, trimX bool) fyne.CanvasObject {
	return container.New(compactLabelLayout{trimX: trimX}, obj)
}

// labeled stacks a field label over its control for dense two-column forms.
func labeled(label string, field fyne.CanvasObject) fyne.CanvasObject {
	return vstack(3, compactLabel(fieldLabel(label), true), controlFrame(field))
}

const controlHeight = float32(30)

type singleLineEntryLayout struct{}

func (singleLineEntryLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.NewSize(0, controlHeight)
	}
	min := objs[0].MinSize()
	return fyne.NewSize(min.Width, controlHeight)
}

func (singleLineEntryLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objs {
		eh := obj.MinSize().Height
		// Shift +1px down for optical cap-height centering inside the 30px frame.
		y := (size.Height-eh)/2 + 1
		obj.Resize(fyne.NewSize(size.Width-4, eh))
		obj.Move(fyne.NewPos(2, y))
	}
}

type controlFrameLayout struct{}

func isMultiLineEntry(obj fyne.CanvasObject) bool {
	if e, ok := obj.(*widget.Entry); ok && e.MultiLine {
		return true
	}
	return false
}

func (controlFrameLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	obj := objs[0]
	min := obj.MinSize()
	if isMultiLineEntry(obj) {
		return min
	}
	if _, isSelect := obj.(*widget.Select); isSelect {
		min.Height = controlHeight
		return min
	}
	if min.Height < controlHeight {
		min.Height = controlHeight
	}
	return min
}

func (controlFrameLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objs {
		if _, isSelect := obj.(*widget.Select); isSelect {
			obj.Resize(fyne.NewSize(size.Width, controlHeight))
			obj.Move(fyne.NewPos(0, (size.Height-controlHeight)/2))
			continue
		}
		obj.Resize(size)
		obj.Move(fyne.NewPos(0, 0))
	}
}

func controlFrame(obj fyne.CanvasObject) fyne.CanvasObject {
	if e, isEntry := obj.(*widget.Entry); isEntry {
		if e.MultiLine {
			return inputFrame(container.NewThemeOverride(e, borderlessEntryTheme{}))
		}
		bg := canvas.NewRectangle(colorBG)
		bg.CornerRadius = 8
		bg.StrokeColor = colorBorder
		bg.StrokeWidth = 1
		clean := container.NewThemeOverride(e, borderlessEntryTheme{})
		return container.NewStack(bg, container.New(singleLineEntryLayout{}, clean))
	}
	if _, isSelect := obj.(*widget.Select); isSelect {
		border := canvas.NewRectangle(color.Transparent)
		border.CornerRadius = 8
		border.StrokeColor = colorBorder
		border.StrokeWidth = 1
		return container.NewStack(container.New(controlFrameLayout{}, obj), border)
	}
	return container.New(controlFrameLayout{}, obj)
}

func pageHeader(title, subtitle string, actions fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := insetLeading(titleText(title, theme.SizeNameHeadingText))
	var top fyne.CanvasObject
	if actions == nil {
		top = container.NewBorder(nil, nil, vCenter(titleLabel), nil, nil)
	} else {
		top = container.NewBorder(nil, nil, vCenter(titleLabel), vCenter(actions), nil)
	}
	if subtitle == "" {
		return top
	}
	desc := muted(subtitle)
	desc.Wrapping = fyne.TextWrapWord
	return vstack(2, top, compactLabel(desc, false))
}

func panel(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = cardRadius
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	pad := layout.NewCustomPaddedLayout(panelPadding, panelPadding, panelPadding, panelPadding)
	return container.NewStack(bg, container.New(pad, content))
}

func subpanel(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSurface2)
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	pad := layout.NewCustomPaddedLayout(spaceS, spaceS, spaceM, spaceM)
	return container.NewStack(bg, container.New(pad, content))
}

func inputFrame(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorBG)
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	pad := layout.NewCustomPaddedLayout(4, 4, 8, 8)
	return container.NewStack(bg, container.New(pad, content))
}

func section(title, subtitle string, content fyne.CanvasObject) fyne.CanvasObject {
	return sectionAction(title, subtitle, nil, content)
}

func sectionAction(title, subtitle string, action fyne.CanvasObject, content fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := container.New(layout.NewCustomPaddedLayout(0, 0, 2, 0), titleText(title, theme.SizeNameSubHeadingText))
	head := vstack(2, titleLabel)
	if subtitle != "" {
		s := muted(subtitle)
		s.Wrapping = fyne.TextWrapWord
		head.Add(compactLabel(s, true))
	}
	var top fyne.CanvasObject = head
	if action != nil {
		top = container.NewBorder(nil, nil, nil, vCenter(action), head)
	}
	return panel(vstack(spaceS, top, content))
}

func insetLeading(obj fyne.CanvasObject) fyne.CanvasObject {
	pad := theme.DefaultTheme().Size(theme.SizeNameInnerPadding)
	if app := fyne.CurrentApp(); app != nil && app.Settings() != nil {
		pad = app.Settings().Theme().Size(theme.SizeNameInnerPadding)
	}
	return container.New(layout.NewCustomPaddedLayout(0, 0, pad, 0), obj)
}

func titleText(text string, sizeName fyne.ThemeSizeName) *canvas.Text {
	size := theme.TextSize()
	if app := fyne.CurrentApp(); app != nil && app.Settings() != nil {
		size = app.Settings().Theme().Size(sizeName)
	}
	t := canvas.NewText(text, colorText)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
}

func heading(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameHeadingText
	return l
}

func subheading(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameSubHeadingText
	return l
}

func muted(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	l.Wrapping = fyne.TextWrapWord
	return l
}

func centerNote(text string) fyne.CanvasObject {
	l := muted(text)
	l.Wrapping = fyne.TextWrapOff
	l.Alignment = fyne.TextAlignCenter
	return container.NewCenter(l)
}

func caption(text string) *widget.Label {
	l := muted(text)
	l.SizeName = theme.SizeNameCaptionText
	return l
}

func oneLine(text string) *widget.Label {
	l := caption(text)
	l.Wrapping = fyne.TextWrapOff
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

func mono(text string, sizeName fyne.ThemeSizeName) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Monospace: true}
	l.SizeName = sizeName
	l.Wrapping = fyne.TextWrapOff
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

func chip(text string, t tone) fyne.CanvasObject {
	bg := canvas.NewRectangle(toneBackground(t))
	bg.CornerRadius = 11
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	txt := canvas.NewText(text, toneColor(t))
	txt.TextSize = 10.5
	txt.TextStyle = fyne.TextStyle{Monospace: true}
	txt.Alignment = fyne.TextAlignCenter
	stack := container.NewStack(bg, container.NewCenter(txt))
	w := fyne.MeasureText(text, 10.5, txt.TextStyle).Width + 20
	return container.NewGridWrap(fyne.NewSize(w, 22), stack)
}

func chips(items []string, t tone) fyne.CanvasObject {
	box := hstack(spaceXS)
	for _, item := range items {
		box.Add(vCenter(chip(item, t)))
	}
	return box
}

func dot(t tone) *canvas.Circle {
	return canvas.NewCircle(toneColor(t))
}

func dotBox(c *canvas.Circle) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(8, 8), c)
}

func avatar(letter string, size float32) fyne.CanvasObject {
	bg := canvas.NewCircle(colorAccentDim)
	bg.StrokeColor = colorAccent
	bg.StrokeWidth = 1
	txt := canvas.NewText(letter, colorAccent)
	txt.TextSize = size * 0.40
	txt.TextStyle = fyne.TextStyle{Bold: true}
	txt.Alignment = fyne.TextAlignCenter
	stack := container.NewStack(bg, container.NewCenter(txt))
	return container.NewGridWrap(fyne.NewSize(size, size), stack)
}

func logoMark(size float32) fyne.CanvasObject {
	img := canvas.NewImageFromResource(logoResource())
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	img.ScaleMode = canvas.ImageScaleSmooth
	return img
}

func gradientRule() fyne.CanvasObject {
	g := canvas.NewLinearGradient(colorAccent, colorAccent2, 270)
	g.SetMinSize(fyne.NewSize(10, 2))
	return g
}

func iconButton(icon fyne.Resource, tooltip string, action func()) *widget.Button {
	b := widget.NewButtonWithIcon("", icon, action)
	b.Importance = widget.LowImportance
	return b
}

func actionButton(label string, icon fyne.Resource, action func()) *widget.Button {
	var b *widget.Button
	if icon == nil {
		b = widget.NewButton(label, action)
	} else {
		b = widget.NewButtonWithIcon(label, icon, action)
	}
	b.Importance = widget.MediumImportance
	return b
}

func primaryButton(label string, icon fyne.Resource, action func()) *widget.Button {
	var b *widget.Button
	if icon == nil {
		b = widget.NewButton(label, action)
	} else {
		b = widget.NewButtonWithIcon(label, icon, action)
	}
	b.Importance = widget.HighImportance
	return b
}

// emptyStateCard builds a unified centered empty-state callout card.
func emptyStateCard(title, guidance string, actionLabel string, action func()) fyne.CanvasObject {
	var btn fyne.CanvasObject
	if actionLabel != "" && action != nil {
		b := primaryButton(actionLabel, nil, action)
		w := fyne.MeasureText(actionLabel, 13, fyne.TextStyle{Bold: true}).Width + 48
		if w < 180 {
			w = 180
		}
		btn = container.NewCenter(widthBox(w, controlFrame(b)))
	}
	items := []fyne.CanvasObject{
		container.NewCenter(logoMark(52)),
		vSpacer(spaceXS),
		container.NewCenter(widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})),
		centerNote(guidance),
	}
	if btn != nil {
		items = append(items, vSpacer(spaceXS), btn)
	}
	body := vstack(spaceXS, items...)
	return container.NewCenter(panel(container.New(layout.NewCustomPaddedLayout(spaceL, spaceL, spaceXL, spaceXL), body)))
}

// editorFrame wraps a multi-line entry in a sleek borderless-interior panel
// with a title row and hint, so the instructions and YAML tabs read like clean
// code editor panes without nested double borders.
func editorFrame(title, hint string, entry *widget.Entry, actions ...fyne.CanvasObject) fyne.CanvasObject {
	head := container.NewBorder(nil, nil,
		subheading(title),
		hstack(spaceXS, actions...),
	)
	h := caption(hint)
	h.Wrapping = fyne.TextWrapWord
	cleanEntry := inputFrame(container.NewThemeOverride(entry, borderlessEntryTheme{}))
	return panel(container.NewBorder(
		vstack(spaceXS, head, h, vSpacer(spaceXS)),
		nil, nil, nil, cleanEntry,
	))
}

func kv(label, value string) fyne.CanvasObject {
	v := widget.NewLabel(value)
	v.TextStyle = fyne.TextStyle{Monospace: true}
	v.SizeName = theme.SizeNameCaptionText
	v.Wrapping = fyne.TextWrapWord
	return vstack(spaceXS, fieldLabel(label), v)
}

func kvLine(label, value string) fyne.CanvasObject {
	v := mono(value, theme.SizeNameCaptionText)
	return vstack(spaceXS, fieldLabel(label), v)
}

func fieldLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.SizeName = theme.SizeNameCaptionText
	l.Importance = widget.LowImportance
	return l
}

func messageCard(text string, t tone) fyne.CanvasObject {
	bg := canvas.NewRectangle(toneBackground(t))
	bg.CornerRadius = 8
	bg.StrokeColor = toneColor(t)
	bg.StrokeWidth = 1
	bar := canvas.NewRectangle(toneColor(t))
	bar.CornerRadius = 2
	bar.SetMinSize(fyne.NewSize(4, 0))
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	switch t {
	case toneDanger:
		label.Importance = widget.DangerImportance
	case toneWarning:
		label.Importance = widget.WarningImportance
	case toneSuccess:
		label.Importance = widget.SuccessImportance
	default:
		label.Importance = widget.MediumImportance
	}
	inner := container.NewBorder(nil, nil, bar, nil, container.NewPadded(label))
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 4, 4), inner))
}

type banner struct {
	*fyne.Container
	bg    *canvas.Rectangle
	bar   *canvas.Rectangle
	label *widget.Label
}

func newBanner() *banner {
	bg := canvas.NewRectangle(toneBackground(toneDanger))
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	bar := canvas.NewRectangle(toneColor(toneDanger))
	bar.CornerRadius = 2
	bar.SetMinSize(fyne.NewSize(4, 0))
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.DangerImportance
	inner := container.NewBorder(nil, nil, bar, nil, container.NewPadded(label))
	c := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 4, 4), inner))
	b := &banner{Container: c, bg: bg, bar: bar, label: label}
	b.Hide()
	return b
}

func (b *banner) Set(text string, t tone) {
	if text == "" {
		b.Hide()
		return
	}
	b.label.SetText(text)
	switch t {
	case toneDanger:
		b.label.Importance = widget.DangerImportance
	case toneWarning:
		b.label.Importance = widget.WarningImportance
	case toneSuccess:
		b.label.Importance = widget.SuccessImportance
	default:
		b.label.Importance = widget.MediumImportance
	}
	b.bg.FillColor = toneBackground(t)
	b.bg.StrokeColor = toneColor(t)
	b.bg.Refresh()
	if b.bar != nil {
		b.bar.FillColor = toneColor(t)
		b.bar.Refresh()
	}
	b.Show()
	b.Refresh()
}

func (b *banner) Clear() { b.Set("", toneNeutral) }
