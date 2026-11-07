package lua

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExampleLoops runs the analyzer over the shipped AgentFlow example loops.
// testdata/ mirrors the core repository's examples/loops; refresh it when those
// change. The examples must all parse and pass the entry checks; warnings are
// logged so a rule that is too eager shows up here.
func TestExampleLoops(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.lua"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example loops: %v", err)
	}
	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			diags := Analyze(string(src), Context{
				PreludeVersion: 1,
				Capabilities:   nil, // the examples do not carry capability metadata
			}).Diagnostics
			for _, d := range diags {
				if d.Severity == SeverityError {
					t.Errorf("line %d: %s (%s)", d.Line+1, d.Message, d.Code)
				} else {
					t.Logf("warning line %d: %s (%s)", d.Line+1, d.Message, d.Code)
				}
			}
		})
	}
}
