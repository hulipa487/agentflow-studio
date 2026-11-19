package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var others = []Lang{ZhHans, ZhHant}

// TestCataloguesHoldTheSameKeys fails when a key exists in English but not in
// a Chinese catalogue, or the reverse.
func TestCataloguesHoldTheSameKeys(t *testing.T) {
	en := messages[En]
	if len(en) == 0 {
		t.Fatal("the English catalogue is empty")
	}
	for _, l := range others {
		var missing, extra []string
		for k := range en {
			if _, ok := messages[l][k]; !ok {
				missing = append(missing, k)
			}
		}
		for k := range messages[l] {
			if _, ok := en[k]; !ok {
				extra = append(extra, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing) > 0 {
			t.Errorf("%s is missing %d keys: %s", l, len(missing), strings.Join(missing, ", "))
		}
		if len(extra) > 0 {
			t.Errorf("%s has %d keys English does not: %s", l, len(extra), strings.Join(extra, ", "))
		}
	}
}

func TestCataloguesReadTheSameParameters(t *testing.T) {
	for k, msg := range messages[En] {
		want := strings.Join(MessageParams(msg), ",")
		for _, l := range others {
			other, ok := messages[l][k]
			if !ok {
				continue
			}
			if got := strings.Join(MessageParams(other), ","); got != want {
				t.Errorf("%s %s reads {%s}, English reads {%s}", l, k, got, want)
			}
			if strings.TrimSpace(other) == "" {
				t.Errorf("%s %s is empty", l, k)
			}
		}
	}
}

// TestCodeAsksForKeysThatExist reads the desktop sources for T("key") and
// i18n.T("key") and checks each key against the English catalogue.
func TestCodeAsksForKeysThatExist(t *testing.T) {
	call := regexp.MustCompile(`(?:\bT|Translate\([^,]+,|Key:|Key =)\s*\(?\s*"([a-z][A-Za-z0-9]*\.[A-Za-z0-9_.]+)"`)
	namespaces := map[string]bool{}
	for k := range messages[En] {
		namespaces[strings.SplitN(k, ".", 2)[0]] = true
	}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range call.FindAllStringSubmatch(string(b), -1) {
			// A prefix ("nav.section." + id) is checked where its names are
			// listed; a dotted value outside every namespace is not a key.
			if strings.HasSuffix(m[1], ".") || !namespaces[strings.SplitN(m[1], ".", 2)[0]] {
				continue
			}
			if !Has(m[1]) {
				t.Errorf("%s asks for %q, which is not in the English catalogue", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFormat(t *testing.T) {
	en := category(En)
	cases := []struct {
		msg    string
		params map[string]any
		want   string
	}{
		{"Copy {what}", map[string]any{"what": "id"}, "Copy id"},
		{"{a} and {b}", map[string]any{"a": 1, "b": "two"}, "1 and two"},
		{"keeps {missing}", nil, "keeps {missing}"},
		{"{n, plural, =0 {no agents} one {# agent} other {# agents}}", map[string]any{"n": 0}, "no agents"},
		{"{n, plural, =0 {no agents} one {# agent} other {# agents}}", map[string]any{"n": 1}, "1 agent"},
		{"{n, plural, =0 {no agents} one {# agent} other {# agents}}", map[string]any{"n": int64(12)}, "12 agents"},
		{"{mode, select, file {Saved to {path}} other {Applied}}", map[string]any{"mode": "file", "path": "a.yaml"}, "Saved to a.yaml"},
		{"{mode, select, file {Saved to {path}} other {Applied}}", map[string]any{"mode": "live"}, "Applied"},
		{`Send {"ok": true} or ${ENV}`, nil, `Send {"ok": true} or ${ENV}`},
		{"a } b { c", nil, "a } b { c"},
		{"issue #3 of {n}", map[string]any{"n": 4}, "issue #3 of 4"},
	}
	for _, c := range cases {
		if got := format(c.msg, c.params, en, "", false); got != c.want {
			t.Errorf("format(%q) = %q, want %q", c.msg, got, c.want)
		}
	}
	if got := format("{n, plural, other {# 个智能体}}", map[string]any{"n": 1}, category(ZhHans), "", false); got != "1 个智能体" {
		t.Errorf("Chinese plural = %q", got)
	}
}

func TestTranslateFallsBack(t *testing.T) {
	if got := Translate(ZhHant, "common.save"); got != "儲存" {
		t.Errorf("zh-Hant common.save = %q", got)
	}
	if got := Translate(ZhHans, "no.such.key"); got != "no.such.key" {
		t.Errorf("a missing key = %q", got)
	}
	if got := Translate(En, "common.required", "what", "Name"); got != "Name is required" {
		t.Errorf("common.required = %q", got)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		in   []string
		want Lang
	}{
		{[]string{"fr_FR.UTF-8", "zh_HK.UTF-8"}, ZhHant},
		{[]string{"zh_CN.UTF-8"}, ZhHans},
		{[]string{"zh"}, ZhHans},
		{[]string{"zh-Hant-CN"}, ZhHant},
		{[]string{"zh-Hans-HK"}, ZhHans},
		{[]string{"zh-TW"}, ZhHant},
		{[]string{"en-GB"}, En},
		{[]string{"ja", "de"}, En},
		{nil, En},
		{[]string{"C", "POSIX", ""}, En},
	}
	for _, c := range cases {
		if got := Detect(c.in...); got != c.want {
			t.Errorf("Detect(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}
