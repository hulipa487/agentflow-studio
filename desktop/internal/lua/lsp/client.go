package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

// Client talks to a Server over an in-memory pipe. It is deliberately a real
// LSP client rather than a direct function call: the editor exercises the same
// protocol an external editor would use.
type Client struct {
	conn net.Conn
	out  *writer

	mu      sync.Mutex
	nextID  int
	pending map[int]chan rpcResponse

	diagMu        sync.Mutex
	onDiagnostics func(uri string, version int, diags []Diagnostic)

	done     chan struct{}
	doneOnce sync.Once
	once     sync.Once
}

func (c *Client) markDone() { c.doneOnce.Do(func() { close(c.done) }) }

// Diagnostic is the client-side view of a published diagnostic.
type Diagnostic struct {
	StartLine, StartCol int
	EndLine, EndCol     int
	Severity            int
	Code                string
	Message             string
}

// CompletionItem is one completion result.
type CompletionItem struct {
	Label         string
	Kind          int
	Detail        string
	Documentation string
	Insert        string
}

// Hover is the documentation returned for a position.
type Hover struct {
	Markdown            string
	StartLine, StartCol int
	EndLine, EndCol     int
}

// FromServer starts a server on one end of an in-memory pipe and returns a
// connected client. Close stops both.
func FromServer() (*Client, error) {
	serverConn, clientConn := net.Pipe()
	go func() {
		s := NewServer()
		_ = s.Serve(serverConn)
	}()
	c := &Client{
		conn:    clientConn,
		out:     &writer{w: clientConn},
		pending: map[int]chan rpcResponse{},
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// SetDiagnosticsHandler installs the callback invoked for publishDiagnostics.
func (c *Client) SetDiagnosticsHandler(fn func(uri string, version int, diags []Diagnostic)) {
	c.diagMu.Lock()
	c.onDiagnostics = fn
	c.diagMu.Unlock()
}

// Initialize performs the LSP handshake and sends the analyzer settings.
func (c *Client) Initialize(ctx context.Context, preludeVersion int, capabilities []string) error {
	params := map[string]any{
		"processId": nil,
		"clientInfo": map[string]any{
			"name":    "agentflow-studio",
			"version": "1",
		},
		"rootUri": nil,
		"capabilities": map[string]any{
			"general": map[string]any{
				"positionEncodings": []string{"utf-32", "utf-16"},
			},
		},
		"initializationOptions": settings{
			PreludeVersion: preludeVersion,
			Capabilities:   capabilities,
		},
	}
	_, err := c.request(ctx, "initialize", params)
	if err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

// Configure updates the capability context; the server re-lints open documents.
func (c *Client) Configure(preludeVersion int, capabilities []string) error {
	return c.notify("workspace/didChangeConfiguration", changeConfigParams{
		Settings: settings{PreludeVersion: preludeVersion, Capabilities: capabilities},
	})
}

// Open sends textDocument/didOpen.
func (c *Client) Open(uri, text string, version int) error {
	return c.notify("textDocument/didOpen", didOpenParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "luau", Version: version, Text: text},
	})
}

// Change sends a full-text textDocument/didChange.
func (c *Client) Change(uri, text string, version int) error {
	p := didChangeParams{}
	p.TextDocument.URI = uri
	p.TextDocument.Version = version
	p.ContentChanges = append(p.ContentChanges, struct {
		Text string `json:"text"`
	}{Text: text})
	return c.notify("textDocument/didChange", p)
}

// Close sends didClose and shuts the pipe down.
func (c *Client) Close(uri string) {
	if uri != "" {
		_ = c.notify("textDocument/didClose", map[string]any{"textDocument": textDocumentID{URI: uri}})
	}
	c.once.Do(func() {
		_ = c.notify("shutdown", nil)
		_ = c.notify("exit", nil)
		c.markDone()
		_ = c.conn.Close()
	})
}

// Complete asks the server for completions at a position.
func (c *Client) Complete(ctx context.Context, uri string, line, col int) ([]CompletionItem, error) {
	raw, err := c.request(ctx, "textDocument/completion", completionParams{
		TextDocument: textDocumentID{URI: uri},
		Position:     position{Line: line, Character: col},
	})
	if err != nil {
		return nil, err
	}
	var list completionList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]CompletionItem, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, CompletionItem{
			Label: it.Label, Kind: it.Kind, Detail: it.Detail,
			Documentation: it.Documentation, Insert: it.InsertText,
		})
	}
	return out, nil
}

// Hover asks the server for documentation at a position.
func (c *Client) Hover(ctx context.Context, uri string, line, col int) (*Hover, error) {
	raw, err := c.request(ctx, "textDocument/hover", hoverParams{
		TextDocument: textDocumentID{URI: uri},
		Position:     position{Line: line, Character: col},
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var h hover
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, err
	}
	out := &Hover{Markdown: h.Contents.Value}
	if h.Range != nil {
		out.StartLine, out.StartCol = h.Range.Start.Line, h.Range.Start.Character
		out.EndLine, out.EndCol = h.Range.End.Line, h.Range.End.Character
	}
	return out, nil
}

func (c *Client) notify(method string, params any) error {
	return c.out.send(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan rpcResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	rawID, _ := json.Marshal(id)
	req := rpcRequest{JSONRPC: "2.0", ID: rawID, Method: method}
	req.Params, _ = json.Marshal(params)
	if err := c.out.send(req); err != nil {
		c.drop(id)
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("lsp %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.drop(id)
		return nil, ctx.Err()
	case <-c.done:
		return nil, errors.New("lsp: client closed")
	}
}

func (c *Client) drop(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.conn)
	for {
		body, err := readMessage(reader)
		if err != nil {
			c.markDone()
			return
		}
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			continue
		}
		if len(envelope.ID) > 0 && envelope.Method == "" {
			var resp rpcResponse
			if json.Unmarshal(body, &resp) != nil {
				continue
			}
			var id int
			if json.Unmarshal(resp.ID, &id) != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
			continue
		}
		if envelope.Method == "textDocument/publishDiagnostics" {
			var p publishParams
			if json.Unmarshal(envelope.Params, &p) != nil {
				continue
			}
			diags := make([]Diagnostic, 0, len(p.Diagnostics))
			for _, d := range p.Diagnostics {
				diags = append(diags, Diagnostic{
					StartLine: d.Range.Start.Line, StartCol: d.Range.Start.Character,
					EndLine: d.Range.End.Line, EndCol: d.Range.End.Character,
					Severity: d.Severity, Code: d.Code, Message: d.Message,
				})
			}
			c.diagMu.Lock()
			fn := c.onDiagnostics
			c.diagMu.Unlock()
			if fn != nil {
				fn(p.URI, p.Version, diags)
			}
		}
	}
}
