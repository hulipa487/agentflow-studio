package lsp

import (
	"context"
	"strings"
	"testing"
	"time"
)

const sample = `-- af-prelude-version: 1

function loop()
  local msg = session.inbox()
  local reply = llm.chat({
    { role = "user", content = msg.text },
  })
  session.send(reply.text)
end
`

type event struct {
	uri     string
	version int
	diags   []Diagnostic
}

func waitEvent(t *testing.T, ch <-chan event) event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for diagnostics")
		return event{}
	}
}

func TestServerDiagnosticsCompletionHover(t *testing.T) {
	c, err := FromServer()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close("")
	ch := make(chan event, 8)
	c.SetDiagnosticsHandler(func(uri string, version int, diags []Diagnostic) {
		ch <- event{uri: uri, version: version, diags: diags}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const uri = "file:///agentflow/loop.lua"
	if err := c.Initialize(ctx, 1, []string{"llm.chat", "tools"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Open(uri, sample, 1); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(t, ch); len(e.diags) != 0 {
		t.Fatalf("clean loop diagnostics: %+v", e.diags)
	}

	// Capability context: memory is not enabled.
	broken := strings.Replace(sample, "session.send(reply.text)", "memory.recall(msg.text)", 1)
	if err := c.Change(uri, broken, 2); err != nil {
		t.Fatal(err)
	}
	e := waitEvent(t, ch)
	found := false
	for _, d := range e.diags {
		if d.Code == "capability.missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("capability diagnostic missing: %+v", e.diags)
	}

	items, err := c.Complete(ctx, uri, 4, 16)
	if err != nil {
		t.Fatal(err)
	}
	hasLLM := false
	for _, it := range items {
		if it.Label == "llm" {
			hasLLM = true
		}
	}
	if !hasLLM {
		t.Fatalf("llm not offered: %+v", items)
	}
	fields, err := c.Complete(ctx, uri, 4, 20)
	if err != nil {
		t.Fatal(err)
	}
	hasChat := false
	for _, it := range fields {
		if it.Label == "chat" {
			hasChat = true
			if !strings.Contains(it.Insert, "chat(") {
				t.Fatalf("completion insert = %q", it.Insert)
			}
		}
	}
	if !hasChat {
		t.Fatalf("llm.chat not offered: %+v", fields)
	}

	hover, err := c.Hover(ctx, uri, 4, 20)
	if err != nil {
		t.Fatal(err)
	}
	if hover == nil || !strings.Contains(hover.Markdown, "llm.chat") {
		t.Fatalf("hover = %+v", hover)
	}

	// Clearing capabilities makes every gated call a warning.
	if err := c.Change(uri, sample, 3); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, ch)
	if err := c.Configure(1, []string{}); err != nil {
		t.Fatal(err)
	}
	e = waitEvent(t, ch)
	count := 0
	for _, d := range e.diags {
		if d.Code == "capability.missing" {
			count++
		}
	}
	if count < 1 {
		t.Fatalf("expected an llm.chat capability warning, got %+v", e.diags)
	}
}
