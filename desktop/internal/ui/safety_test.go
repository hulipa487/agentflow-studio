package ui

import (
	"strings"
	"testing"
)

func TestParseCategoriesEntry(t *testing.T) {
	got, err := parseCategoriesEntry("safe: allow\n self_harm : block\n\nharassment:review\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"safe":       "allow",
		"self_harm":  "block",
		"harassment": "review",
	}
	if len(got) != len(want) {
		t.Fatalf("categories = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, err := parseCategoriesEntry("safe=allow"); err == nil {
		t.Error("a line without a colon was accepted")
	}
	if _, err := parseCategoriesEntry(": allow"); err == nil {
		t.Error("an empty label was accepted")
	}
}

func TestParseOptionalThreshold(t *testing.T) {
	if v, err := parseOptionalThreshold(""); err != nil || v != nil {
		t.Fatalf("blank threshold = %v, %v", v, err)
	}
	if v, err := parseOptionalThreshold("0"); err != nil || v == nil || *v != 0 {
		t.Fatalf("explicit zero = %v, %v", v, err)
	}
	if _, err := parseOptionalThreshold("1.1"); err == nil {
		t.Error("threshold above 1 was accepted")
	}
	if _, err := parseOptionalThreshold("high"); err == nil {
		t.Error("non-numeric threshold was accepted")
	}
}

func TestOptionalCounts(t *testing.T) {
	if n, err := optionalCount("x", "12"); err != nil || n != 12 {
		t.Fatalf("count = %d, %v", n, err)
	}
	if _, err := optionalCount("x", "-1"); err == nil {
		t.Error("negative count was accepted")
	}
	if n, err := optionalCount64("x", ""); err != nil || n != 0 {
		t.Fatalf("blank count = %d, %v", n, err)
	}
}

func TestSafetyFormatting(t *testing.T) {
	if got := chainText(nil); !strings.Contains(got, "empty chain") {
		t.Errorf("empty chain = %q", got)
	}
	if got := chainText([]string{"a", "classifier:mod"}); !strings.Contains(got, "a") || !strings.Contains(got, "classifier:mod") {
		t.Errorf("chain = %q", got)
	}
	if got := formatCategories(map[string]string{"b": "block", "a": "allow"}); got != "a: allow\nb: block" {
		t.Errorf("categories = %q", got)
	}
	if got := categorySummary(map[string]string{"a": "allow", "b": "block"}); got != "a=allow, b=block" {
		t.Errorf("summary = %q", got)
	}
}
