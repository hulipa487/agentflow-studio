package ui

import "testing"

func TestCheckGridSelection(t *testing.T) {
	checks := newChecks([]string{"llm.chat", "memory", "tools"}, []string{"memory"})
	if got := selectedChecks(checks); len(got) != 1 || got[0] != "memory" {
		t.Fatalf("selected = %v", got)
	}
	clearChecks(checks)
	if got := selectedChecks(checks); len(got) != 0 {
		t.Fatalf("selected after clear = %v", got)
	}
}
