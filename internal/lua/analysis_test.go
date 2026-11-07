package lua

import (
	"strings"
	"testing"
)

const goodLoop = `-- af-prelude-version: 1

function loop()
  local msg = session.inbox()
  local reply = llm.chat({
    { role = "user", content = msg.text },
  })
  session.send(reply.text)
end
`

func codes(diags []Diagnostic) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = d.Code
	}
	return out
}

func hasCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func analyze(src string, ctx Context) []Diagnostic {
	return Analyze(src, ctx).Diagnostics
}

func TestAnalyzeCleanLoop(t *testing.T) {
	diags := analyze(goodLoop, Context{PreludeVersion: 1})
	if len(diags) != 0 {
		t.Fatalf("clean loop produced diagnostics: %v", diags)
	}
}

func TestDirectiveRules(t *testing.T) {
	src := "function loop()\nend\n"
	diags := analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "prelude.missing-directive") {
		t.Fatalf("missing directive not reported: %v", codes(diags))
	}

	src = "-- af-prelude-version: 2\nfunction loop()\nend\n"
	diags = analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "prelude.version-mismatch") {
		t.Fatalf("version mismatch not reported: %v", codes(diags))
	}

	src = "-- af-prelude-version: 1\n-- af-prelude-version: 1\nfunction loop()\nend\n"
	diags = analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "prelude.duplicate-directive") {
		t.Fatalf("duplicate directive not reported: %v", codes(diags))
	}
}

func TestEntryPoint(t *testing.T) {
	diags := analyze("-- af-prelude-version: 1\nlocal x = 1\n", Context{PreludeVersion: 1})
	if !hasCode(diags, "entry.missing-loop") {
		t.Fatalf("missing loop not reported: %v", codes(diags))
	}
	diags = analyze("-- af-prelude-version: 1\nlocal function loop()\nend\n", Context{PreludeVersion: 1})
	if !hasCode(diags, "entry.local-loop") {
		t.Fatalf("local loop not reported: %v", codes(diags))
	}
}

func TestUnknownGlobalAndField(t *testing.T) {
	src := `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  log.info(mesage)
  local r = llm.chta({})
end
`
	diags := analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "global.unknown") {
		t.Fatalf("unknown global not reported: %v", codes(diags))
	}
	if !hasCode(diags, "field.unknown") {
		t.Fatalf("unknown field not reported: %v", codes(diags))
	}
	found := false
	for _, d := range diags {
		if d.Code == "field.unknown" && strings.Contains(d.Message, `did you mean "chat"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("field suggestion missing: %v", diags)
	}
}

func TestCapabilityCheck(t *testing.T) {
	src := `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  memory.recall(msg.text)
end
`
	diags := analyze(src, Context{PreludeVersion: 1, Capabilities: []string{"llm.chat"}})
	if !hasCode(diags, "capability.missing") {
		t.Fatalf("capability warning missing: %v", codes(diags))
	}
	diags = analyze(src, Context{PreludeVersion: 1, Capabilities: nil})
	if hasCode(diags, "capability.missing") {
		t.Fatalf("capability checked with unknown context: %v", codes(diags))
	}
}

func TestStreamGenericFor(t *testing.T) {
	src := `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  for delta in llm.stream({{ role = "user", content = msg.text }}) do
    log.info(delta)
  end
end
`
	diags := analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "api.stream-for") {
		t.Fatalf("stream generic for not reported: %v", codes(diags))
	}
}

func TestTopLevelYieldAndReturn(t *testing.T) {
	src := `-- af-prelude-version: 1
local greeting = llm.chat({ { role = "user", content = "hi" } })
function loop()
  return
end
`
	diags := analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "api.top-level-yield") {
		t.Fatalf("top-level yield not reported: %v", codes(diags))
	}
	if !hasCode(diags, "api.return-loop") {
		t.Fatalf("loop return not reported: %v", codes(diags))
	}
}

func TestStructure(t *testing.T) {
	src := "-- af-prelude-version: 1\nfunction loop()\n  if true then\nend\n"
	diags := analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "syntax.unclosed-block") {
		t.Fatalf("unclosed block not reported: %v", codes(diags))
	}
	src = "-- af-prelude-version: 1\nfunction loop()\n  local x = (1\nend\n"
	diags = analyze(src, Context{PreludeVersion: 1})
	if !hasCode(diags, "syntax.unclosed-bracket") {
		t.Fatalf("unclosed bracket not reported: %v", codes(diags))
	}
}

func TestTypeAnnotationsAreNotGlobals(t *testing.T) {
	src := `-- af-prelude-version: 1
type Reply = { text: string }

local function format(value: string, count: number?): string
  local label: string = value
  return label
end

function loop()
  local msg: Message = session.inbox()
  session.send(format(msg.text, 1))
end
`
	diags := analyze(src, Context{PreludeVersion: 1})
	for _, d := range diags {
		if d.Code == "global.unknown" {
			t.Fatalf("type annotation flagged as a global: %v", d)
		}
	}
}

func TestCompletionAndHover(t *testing.T) {
	res := Analyze(goodLoop, Context{PreludeVersion: 1})
	items := Complete(res.Model, 5, 4)
	if len(items) == 0 {
		t.Fatal("completion returned nothing")
	}
	found := false
	for _, it := range items {
		if strings.Contains(it.Label, "llm") {
			found = true
		}
	}
	if !found {
		t.Fatalf("llm missing from completion: %+v", items[:min(len(items), 5)])
	}

	hover := HoverAt(res.Model, 4, 20) // inside llm.chat
	if hover == nil || !strings.Contains(hover.Markdown, "llm.chat") {
		t.Fatalf("hover = %+v", hover)
	}

	fieldItems := Complete(res.Model, 4, 14)
	if len(fieldItems) == 0 {
		t.Fatal("field completion returned nothing")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestPackageImportLenient(t *testing.T) {
	src := `-- af-prelude-version: 1
-- af-import: greeting
function loop()
  session.send(greeting("hi"))
end
`
	if diags := analyze(src, Context{PreludeVersion: 1}); hasCode(diags, "global.unknown") {
		t.Fatalf("package global flagged: %v", diags)
	}
	// The same call without the import is still a typo candidate.
	src = `-- af-prelude-version: 1
function loop()
  session.send(greeting("hi"))
end
`
	if diags := analyze(src, Context{PreludeVersion: 1}); !hasCode(diags, "global.unknown") {
		t.Fatalf("unimported global not flagged: %v", diags)
	}
}
