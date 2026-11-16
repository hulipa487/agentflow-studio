package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	af "agentflow-studio/internal/lua"
)

const (
	sourceName = "agentflow-lua"
	serverName = "agentflow-lua"
)

var errExit = errors.New("lsp: exit")

// Server is an in-process Luau language server. One Server can serve one
// connection; Studio creates one per editor.
type Server struct {
	mu       sync.Mutex
	docs     map[string]*document
	settings settings
	out      *writer
}

type document struct {
	text    string
	version int
}

// NewServer returns an empty server.
func NewServer() *Server {
	return &Server{docs: map[string]*document{}}
}

// Serve reads JSON-RPC messages until the peer closes the connection or sends
// exit.
func (s *Server) Serve(rw io.ReadWriteCloser) error {
	defer rw.Close()
	reader := bufio.NewReader(rw)
	s.out = &writer{w: rw}
	for {
		body, err := readMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			continue
		}
		if err := s.handle(&req); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			return err
		}
	}
}

type writer struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *writer) send(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := fmt.Fprintf(w.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = w.w.Write(body)
	return err
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(strings.ToLower(line), "content-length:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return nil, fmt.Errorf("lsp: bad content-length %q", v)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, errors.New("lsp: message without content-length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) handle(req *rpcRequest) error {
	isRequest := len(req.ID) > 0 && string(req.ID) != "null"
	respond := func(result any) error {
		if !isRequest {
			return nil
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return s.out.send(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: raw})
	}
	fail := func(code int, msg string) error {
		if !isRequest {
			return nil
		}
		return s.out.send(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}})
	}

	switch req.Method {
	case "initialize":
		var p initializeParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &p)
		}
		s.mu.Lock()
		s.settings = p.InitializationOptions
		s.mu.Unlock()
		res := initializeResult{
			Capabilities: serverCapabilities{
				TextDocumentSync: textDocumentSyncFull,
				CompletionProvider: &completionOptions{
					TriggerCharacters: []string{".", ":"},
				},
				HoverProvider:    true,
				PositionEncoding: utf32,
			},
		}
		res.ServerInfo.Name = serverName
		res.ServerInfo.Version = "1"
		return respond(res)
	case "initialized":
		return nil
	case "shutdown":
		return respond(nil)
	case "exit":
		return errExit
	case "textDocument/didOpen":
		var p didOpenParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		s.mu.Lock()
		s.docs[p.TextDocument.URI] = &document{text: p.TextDocument.Text, version: p.TextDocument.Version}
		s.mu.Unlock()
		return s.publish(p.TextDocument.URI)
	case "textDocument/didChange":
		var p didChangeParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		if len(p.ContentChanges) == 0 {
			return nil
		}
		s.mu.Lock()
		doc := s.docs[p.TextDocument.URI]
		if doc == nil {
			doc = &document{}
			s.docs[p.TextDocument.URI] = doc
		}
		doc.text = p.ContentChanges[len(p.ContentChanges)-1].Text
		doc.version = p.TextDocument.Version
		s.mu.Unlock()
		return s.publish(p.TextDocument.URI)
	case "textDocument/didClose":
		var p struct {
			TextDocument textDocumentID `json:"textDocument"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		s.mu.Lock()
		delete(s.docs, p.TextDocument.URI)
		s.mu.Unlock()
		return s.out.send(rpcNotification{
			JSONRPC: "2.0",
			Method:  "textDocument/publishDiagnostics",
			Params:  publishParams{URI: p.TextDocument.URI, Diagnostics: []diagnostic{}},
		})
	case "workspace/didChangeConfiguration":
		var p changeConfigParams
		if err := json.Unmarshal(req.Params, &p); err == nil {
			s.mu.Lock()
			s.settings = p.Settings
			uris := make([]string, 0, len(s.docs))
			for uri := range s.docs {
				uris = append(uris, uri)
			}
			s.mu.Unlock()
			for _, uri := range uris {
				if err := s.publish(uri); err != nil {
					return err
				}
			}
		}
		return nil
	case "textDocument/completion":
		var p completionParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "invalid completion params")
		}
		model := s.analyze(p.TextDocument.URI)
		items := af.Complete(model, p.Position.Line, p.Position.Character)
		out := completionList{Items: make([]completionItem, 0, len(items))}
		for _, it := range items {
			out.Items = append(out.Items, completionItem{
				Label:            it.Label,
				Kind:             completionKind(it.Kind),
				Detail:           it.Detail,
				Documentation:    it.Documentation,
				InsertText:       it.Insert,
				InsertTextFormat: 2,
				SortText:         completionSort(it.Kind, it.Label),
			})
		}
		return respond(out)
	case "textDocument/hover":
		var p hoverParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "invalid hover params")
		}
		model := s.analyze(p.TextDocument.URI)
		h := af.HoverAt(model, p.Position.Line, p.Position.Character)
		if h == nil {
			return respond(nil)
		}
		out := hover{
			Contents: markupContent{Kind: "markdown", Value: h.Markdown},
			Range: &rng{
				Start: position{Line: h.Line, Character: h.Col},
				End:   position{Line: h.EndLine, Character: h.EndCol},
			},
		}
		return respond(out)
	}
	if isRequest {
		return fail(-32601, "method not found: "+req.Method)
	}
	return nil
}

func (s *Server) analyze(uri string) *af.Model {
	s.mu.Lock()
	doc := s.docs[uri]
	settings := s.settings
	s.mu.Unlock()
	if doc == nil {
		return nil
	}
	res := af.Analyze(doc.text, af.Context{
		PreludeVersion: settings.PreludeVersion,
		Capabilities:   settings.Capabilities,
	})
	return res.Model
}

func (s *Server) publish(uri string) error {
	s.mu.Lock()
	doc := s.docs[uri]
	settings := s.settings
	s.mu.Unlock()
	if doc == nil {
		return nil
	}
	res := af.Analyze(doc.text, af.Context{
		PreludeVersion: settings.PreludeVersion,
		Capabilities:   settings.Capabilities,
	})
	diags := make([]diagnostic, 0, len(res.Diagnostics))
	for _, d := range res.Diagnostics {
		diags = append(diags, diagnostic{
			Range: rng{
				Start: position{Line: d.Line, Character: d.Col},
				End:   position{Line: d.EndLine, Character: d.EndCol},
			},
			Severity: int(d.Severity),
			Code:     d.Code,
			Source:   sourceName,
			Message:  d.Message,
		})
	}
	return s.out.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  publishParams{URI: uri, Version: doc.version, Diagnostics: diags},
	})
}

// completionKind maps the analyzer's labels to LSP CompletionItemKind values.
func completionKind(kind string) int {
	switch kind {
	case "function":
		return 3
	case "field":
		return 5
	case "variable":
		return 6
	case "module":
		return 9
	case "snippet":
		return 15
	}
	return 1
}

func completionSort(kind, label string) string {
	rank := "3"
	switch kind {
	case "variable":
		rank = "1"
	case "field":
		rank = "2"
	case "function":
		rank = "3"
	case "module":
		rank = "4"
	}
	return rank + label
}
