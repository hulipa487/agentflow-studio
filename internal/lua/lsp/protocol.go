// Package lsp hosts the AgentFlow Luau language server. It speaks the subset
// of LSP 3.17 the Studio editor uses, over any io.ReadWriteCloser; Studio runs
// it in-process over an in-memory pipe, so no extra binary is involved.
package lsp

import "encoding/json"

const (
	textDocumentSyncFull = 1
	utf32                = "utf-32"
)

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type rng struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    rng    `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type publishParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version,omitempty"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type textDocumentID struct {
	URI string `json:"uri"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

type completionParams struct {
	TextDocument textDocumentID `json:"textDocument"`
	Position     position       `json:"position"`
}

type hoverParams struct {
	TextDocument textDocumentID `json:"textDocument"`
	Position     position       `json:"position"`
}

type completionItem struct {
	Label            string `json:"label"`
	Kind             int    `json:"kind,omitempty"`
	Detail           string `json:"detail,omitempty"`
	Documentation    string `json:"documentation,omitempty"`
	InsertText       string `json:"insertText,omitempty"`
	InsertTextFormat int    `json:"insertTextFormat,omitempty"`
	SortText         string `json:"sortText,omitempty"`
}

type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []completionItem `json:"items"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hover struct {
	Contents markupContent `json:"contents"`
	Range    *rng          `json:"range,omitempty"`
}

type completionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type serverCapabilities struct {
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *completionOptions `json:"completionProvider,omitempty"`
	HoverProvider      bool               `json:"hoverProvider"`
	PositionEncoding   string             `json:"positionEncoding"`
}

type settings struct {
	PreludeVersion int      `json:"preludeVersion"`
	Capabilities   []string `json:"capabilities"`
}

type initializeParams struct {
	InitializationOptions settings `json:"initializationOptions"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type changeConfigParams struct {
	Settings settings `json:"settings"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}
