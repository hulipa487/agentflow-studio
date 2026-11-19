package ui

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"

	"fyne.io/fyne/v2"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"

	"agentflow-studio/internal/i18n"
)

// Fyne's bundled text font has no Chinese glyphs, and the system font Fyne
// adds behind it is chosen by the OS locale, not by Studio's language: an
// English Windows or Linux showing Studio in Chinese would print boxes. So in
// a Chinese language the theme hands Fyne a CJK font from the system instead.
//
// Which one matters. Simplified and Traditional share code points but not
// glyph shapes, so 简体 asks for an SC face and 繁體 for an HK / TC one before
// falling back to whatever covers the text. Nothing is bundled: a CJK font is
// 16 MB a weight, and every Windows, macOS and Chinese-capable Linux has one.
// A system with none shows Chinese as boxes; Settings says so.

var cjkFamilies = map[i18n.Lang][]string{
	i18n.ZhHans: {
		"Noto Sans CJK SC", "Source Han Sans SC", "Source Han Sans CN", "PingFang SC",
		"Microsoft YaHei UI", "Microsoft YaHei", "Hiragino Sans GB", "WenQuanYi Micro Hei",
		"WenQuanYi Zen Hei", "Noto Sans SC", "SimHei", "Droid Sans Fallback",
		"Noto Sans CJK TC", "Microsoft JhengHei", "PingFang TC",
	},
	i18n.ZhHant: {
		"Noto Sans CJK HK", "Noto Sans CJK TC", "Source Han Sans HK", "Source Han Sans TC",
		"Source Han Sans TW", "PingFang HK", "PingFang TC", "Microsoft JhengHei UI",
		"Microsoft JhengHei", "Heiti TC", "Noto Sans HK", "Noto Sans TC", "WenQuanYi Micro Hei",
		"WenQuanYi Zen Hei", "Droid Sans Fallback", "Noto Sans CJK SC", "Microsoft YaHei", "PingFang SC",
	},
}

// A character each script writes its own way, to ask the font map for a face
// that really covers the language.
var cjkProbe = map[i18n.Lang]rune{i18n.ZhHans: '这', i18n.ZhHant: '這'}

type cjkFontKey struct {
	lang i18n.Lang
	bold bool
}

var (
	cjkMu      sync.Mutex
	cjkMap     *fontscan.FontMap
	cjkMapDone bool
	cjkCache   = map[cjkFontKey]fyne.Resource{}
	cjkFiles   = map[string][]byte{}
)

type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}

// cjkFont returns a system font for a Chinese language, or nil when the system
// has none (or the language is not Chinese). The first call scans the system
// fonts, which go-text caches on disk, so later launches are quick.
func cjkFont(lang i18n.Lang, bold bool) fyne.Resource {
	if !lang.Chinese() {
		return nil
	}
	cjkMu.Lock()
	defer cjkMu.Unlock()
	key := cjkFontKey{lang, bold}
	if res, ok := cjkCache[key]; ok {
		return res
	}
	res := findCJKFont(lang, bold)
	cjkCache[key] = res
	return res
}

func findCJKFont(lang i18n.Lang, bold bool) fyne.Resource {
	if !cjkMapDone {
		cjkMapDone = true
		fm := fontscan.NewFontMap(quietLogger{})
		if err := fm.UseSystemFonts(""); err == nil {
			cjkMap = fm
		}
	}
	if cjkMap == nil {
		return nil
	}
	aspect := font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal, Stretch: font.StretchNormal}
	if bold {
		aspect.Weight = font.WeightBold
	}
	cjkMap.SetQuery(fontscan.Query{Families: cjkFamilies[lang], Aspect: aspect})
	face := cjkMap.ResolveFace(cjkProbe[lang])
	if face == nil || face.Font == nil {
		return nil
	}
	if _, ok := face.NominalGlyph(cjkProbe[lang]); !ok {
		return nil
	}
	loc := cjkMap.FontLocation(face.Font)
	if loc.File == "" {
		return nil
	}
	data, ok := cjkFiles[loc.File]
	if !ok {
		b, err := os.ReadFile(loc.File)
		if err != nil {
			return nil
		}
		data = b
		cjkFiles[loc.File] = data
	}
	one, err := sfntFace(data, int(loc.Index))
	if err != nil {
		return nil
	}
	name := "cjk-" + string(lang)
	if bold {
		name += "-bold"
	}
	return fyne.NewStaticResource(name+".otf", one)
}

// sfntFace returns face index of an OpenType file as a file of its own. Fyne
// loads the first face of a collection whatever it is asked for, and the Noto
// collection keeps Japanese first; a face is only a table directory pointing
// into the file, so it can be lifted out by copying the tables it names.
func sfntFace(data []byte, index int) ([]byte, error) {
	bad := errors.New("not an OpenType font")
	if len(data) < 12 {
		return nil, bad
	}
	if string(data[:4]) != "ttcf" {
		if index != 0 {
			return nil, bad
		}
		return data, nil
	}
	n := int(binary.BigEndian.Uint32(data[8:12]))
	if index < 0 || index >= n || len(data) < 12+4*n {
		return nil, bad
	}
	dir := int(binary.BigEndian.Uint32(data[12+4*index:]))
	if dir < 0 || dir+12 > len(data) {
		return nil, bad
	}
	tables := int(binary.BigEndian.Uint16(data[dir+4:]))
	head := 12 + 16*tables
	if dir+head > len(data) {
		return nil, bad
	}
	out := make([]byte, head, head+len(data)/2)
	copy(out, data[dir:dir+head])
	for i := 0; i < tables; i++ {
		rec := 12 + 16*i
		off := int(binary.BigEndian.Uint32(data[dir+rec+8:]))
		length := int(binary.BigEndian.Uint32(data[dir+rec+12:]))
		if off < 0 || length < 0 || off+length > len(data) {
			return nil, bad
		}
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		out = append(out, data[off:off+length]...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out, nil
}
