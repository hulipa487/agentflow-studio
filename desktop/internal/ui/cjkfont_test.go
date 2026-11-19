package ui

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/go-text/typesetting/font"

	"agentflow-studio/internal/i18n"
)

// A two-face collection built by hand: each face has one table, and the second
// face's table sits after the first's.
func testCollection() []byte {
	face := func(tag string, off, length int) []byte {
		b := make([]byte, 12+16)
		copy(b, "OTTO")
		binary.BigEndian.PutUint16(b[4:], 1)
		copy(b[12:], tag)
		binary.BigEndian.PutUint32(b[12+8:], uint32(off))
		binary.BigEndian.PutUint32(b[12+12:], uint32(length))
		return b
	}
	const header = 12 + 2*4
	dirA, dirB := header, header+28
	dataA, dataB := header+56, header+56+8
	out := make([]byte, header)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[8:], 2)
	binary.BigEndian.PutUint32(out[12:], uint32(dirA))
	binary.BigEndian.PutUint32(out[16:], uint32(dirB))
	out = append(out, face("aaaa", dataA, 5)...)
	out = append(out, face("bbbb", dataB, 6)...)
	out = append(out, []byte("AAAAA\x00\x00\x00BBBBBB\x00\x00")...)
	return out
}

func TestSfntFaceLiftsOneFaceOutOfACollection(t *testing.T) {
	ttc := testCollection()
	for i, want := range []string{"AAAAA", "BBBBBB"} {
		one, err := sfntFace(ttc, i)
		if err != nil {
			t.Fatalf("face %d: %v", i, err)
		}
		if string(one[:4]) != "OTTO" {
			t.Fatalf("face %d starts with %q", i, one[:4])
		}
		off := binary.BigEndian.Uint32(one[12+8:])
		length := binary.BigEndian.Uint32(one[12+12:])
		if got := string(one[off : off+length]); got != want {
			t.Errorf("face %d table = %q, want %q", i, got, want)
		}
		if len(one)%4 != 0 {
			t.Errorf("face %d is not padded to four bytes", i)
		}
	}
	if _, err := sfntFace(ttc, 2); err == nil {
		t.Error("a face index past the end should fail")
	}
	if _, err := sfntFace([]byte("short"), 0); err == nil {
		t.Error("a truncated file should fail")
	}
	plain := append([]byte("OTTO"), make([]byte, 16)...)
	if one, err := sfntFace(plain, 0); err != nil || !bytes.Equal(one, plain) {
		t.Error("a single-face file should come back as it is")
	}
}

// TestCJKFontCoversItsLanguage checks, on a machine that has a CJK font, that
// the face chosen for each Chinese language parses and holds that language's
// characters. Without one there is nothing to check.
func TestCJKFontCoversItsLanguage(t *testing.T) {
	if cjkFont(i18n.En, false) != nil {
		t.Error("English should keep Fyne's own font")
	}
	samples := map[i18n.Lang]string{
		i18n.ZhHans: "这是简体中文设置语言",
		i18n.ZhHant: "這是繁體中文設定語言",
	}
	for _, l := range []i18n.Lang{i18n.ZhHans, i18n.ZhHant} {
		for _, bold := range []bool{false, true} {
			res := cjkFont(l, bold)
			if res == nil {
				t.Skipf("no CJK font on this system for %s", l)
			}
			face, err := font.ParseTTF(bytes.NewReader(res.Content()))
			if err != nil {
				t.Fatalf("%s bold=%v: %v", l, bold, err)
			}
			for _, r := range samples[l] {
				if _, ok := face.NominalGlyph(r); !ok {
					t.Errorf("%s bold=%v: the chosen font has no glyph for %q", l, bold, r)
				}
			}
			for _, r := range "Studio 123" {
				if _, ok := face.NominalGlyph(r); !ok {
					t.Errorf("%s bold=%v: the chosen font has no glyph for %q", l, bold, r)
				}
			}
		}
	}
}
