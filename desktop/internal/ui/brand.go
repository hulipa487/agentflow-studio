package ui

import (
	_ "embed"
	"image/color"
	"regexp"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// The AgentFlow brand, from the shared asset kit (assets/brand/ at the repo
// root; these are the copies the app embeds). The mark is the same on every
// palette and in both themes: its slate hexagon reads on light and dark.

//go:embed brand/logo.svg
var brandLogoSVG []byte

//go:embed brand/logo.png
var brandLogoPNG []byte

// uniformScale expands a one-argument scale(n) to scale(n n). The SVG spec
// gives scale(n) a y factor equal to n, but Fyne's SVG renderer takes the
// missing argument as 0 and flattens the kit's mark to a line. The embedded
// file stays byte-for-byte the kit's; the fix is applied as it loads.
var uniformScale = regexp.MustCompile(`scale\(\s*([0-9.]+)\s*\)`)

func forFyne(svg []byte) []byte { return uniformScale.ReplaceAll(svg, []byte("scale($1 $1)")) }

var (
	// The name keeps the agentflow-logo prefix theme_remap.go looks for.
	brandLogoResource = fyne.NewStaticResource("agentflow-logo.svg", forFyne(brandLogoSVG))
	brandIconResource = fyne.NewStaticResource("agentflow-studio.png", brandLogoPNG)
)

func logoResource() fyne.Resource { return brandLogoResource }

func appIconResource() fyne.Resource { return brandIconResource }

// brandGradient is the kit's gradient: sky, indigo, purple, with the deeper
// light-mode stops on light backgrounds.
func brandGradient() [3]color.NRGBA {
	if activeTheme == themeLight {
		return [3]color.NRGBA{{0x02, 0x84, 0xC7, 0xFF}, {0x4F, 0x46, 0xE5, 0xFF}, {0x93, 0x33, 0xEA, 0xFF}}
	}
	return [3]color.NRGBA{{0x38, 0xBD, 0xF8, 0xFF}, {0x63, 0x66, 0xF1, 0xFF}, {0xA8, 0x55, 0xF7, 0xFF}}
}

// brandAt samples the brand gradient at t in [0, 1].
func brandAt(t float32) color.NRGBA {
	g := brandGradient()
	a, b := g[0], g[1]
	if t >= 0.5 {
		a, b, t = g[1], g[2], t-0.5
	}
	t *= 2
	mix := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xFF}
}

// brandWordmark is "AgentFlow" as the banner sets it: "Agent" in the text
// colour, "Flow" in the brand gradient. A canvas text takes one colour, so
// each letter of "Flow" gets its own sample of the gradient. Refresh re-reads
// the theme, so a light/dark switch recolours it in place.
type brandWordmark struct {
	widget.BaseWidget
	size  float32
	agent *canvas.Text
	flow  []*canvas.Text
}

func newBrandWordmark(size float32) *brandWordmark {
	w := &brandWordmark{size: size}
	style := fyne.TextStyle{Bold: true}
	w.agent = canvas.NewText("Agent", colorText)
	w.agent.TextSize, w.agent.TextStyle = size, style
	for _, r := range "Flow" {
		t := canvas.NewText(string(r), colorText)
		t.TextSize, t.TextStyle = size, style
		w.flow = append(w.flow, t)
	}
	w.ExtendBaseWidget(w)
	w.paint()
	return w
}

func (w *brandWordmark) paint() {
	w.agent.Color = colorText
	for i, t := range w.flow {
		t.Color = brandAt(float32(i) / float32(len(w.flow)-1))
	}
}

func (w *brandWordmark) CreateRenderer() fyne.WidgetRenderer {
	objs := []fyne.CanvasObject{w.agent}
	for _, t := range w.flow {
		objs = append(objs, t)
	}
	return &brandWordmarkRenderer{w: w, row: hstack(0, objs...)}
}

func (w *brandWordmark) Refresh() {
	w.paint()
	w.BaseWidget.Refresh()
}

type brandWordmarkRenderer struct {
	w   *brandWordmark
	row fyne.CanvasObject
}

func (r *brandWordmarkRenderer) Layout(s fyne.Size)           { r.row.Resize(s) }
func (r *brandWordmarkRenderer) MinSize() fyne.Size           { return r.row.MinSize() }
func (r *brandWordmarkRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.row} }
func (r *brandWordmarkRenderer) Destroy()                     {}
func (r *brandWordmarkRenderer) Refresh() {
	r.w.paint()
	for _, o := range r.Objects() {
		o.Refresh()
	}
	canvas.Refresh(r.row)
}

// brandBanner is the kit's banner, drawn natively: the mark beside the
// wordmark.
func brandBanner(markSize, textSize float32) fyne.CanvasObject {
	return hstack(spaceS, vCenter(logoMark(markSize)), vCenter(newBrandWordmark(textSize)))
}

// brandRule is a short hairline in the brand gradient, drawn as two
// two-stop segments because a canvas gradient has only two stops.
type brandRule struct {
	widget.BaseWidget
	width, height float32
	left, right   *canvas.LinearGradient
}

func newBrandRule(width, height float32) *brandRule {
	r := &brandRule{width: width, height: height}
	r.left = canvas.NewHorizontalGradient(color.Transparent, color.Transparent)
	r.right = canvas.NewHorizontalGradient(color.Transparent, color.Transparent)
	r.ExtendBaseWidget(r)
	r.paint()
	return r
}

func (r *brandRule) paint() {
	g := brandGradient()
	r.left.StartColor, r.left.EndColor = g[0], g[1]
	r.right.StartColor, r.right.EndColor = g[1], g[2]
}

func (r *brandRule) MinSize() fyne.Size { return fyne.NewSize(r.width, r.height) }

func (r *brandRule) CreateRenderer() fyne.WidgetRenderer { return &brandRuleRenderer{r: r} }

func (r *brandRule) Refresh() {
	r.paint()
	r.BaseWidget.Refresh()
}

type brandRuleRenderer struct{ r *brandRule }

func (b *brandRuleRenderer) Layout(s fyne.Size) {
	half := s.Width / 2
	b.r.left.Move(fyne.NewPos(0, 0))
	b.r.left.Resize(fyne.NewSize(half, s.Height))
	b.r.right.Move(fyne.NewPos(half, 0))
	b.r.right.Resize(fyne.NewSize(s.Width-half, s.Height))
}
func (b *brandRuleRenderer) MinSize() fyne.Size { return b.r.MinSize() }
func (b *brandRuleRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{b.r.left, b.r.right}
}
func (b *brandRuleRenderer) Destroy() {}
func (b *brandRuleRenderer) Refresh() {
	b.r.paint()
	b.r.left.Refresh()
	b.r.right.Refresh()
}
