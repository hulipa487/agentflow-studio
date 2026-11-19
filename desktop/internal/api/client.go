package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Error is an HTTP or transport failure. Status 0 means the request never got
// a response (wrong URL, runtime down, TLS failure).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Client talks to one AgentFlow instance's admin API. It has no cookie jar and
// no browser: every request carries the bearer token explicitly.
type Client struct {
	// BaseURL is the HTTP origin used to build requests; for a local transport
	// it is a synthetic origin (http://unix or http://npipe).
	BaseURL string
	// DisplayURL is what the user entered, including unix:// or npipe://.
	DisplayURL string
	Token      string
	HTTP       *http.Client
}

func New(baseURL, token string) *Client {
	raw := strings.TrimSpace(baseURL)
	c := &Client{
		DisplayURL: raw,
		Token:      strings.TrimSpace(token),
		HTTP:       &http.Client{},
	}
	if kind, addr, ok := parseLocal(raw); ok {
		switch kind {
		case "unix":
			c.BaseURL = "http://unix"
		case "npipe":
			c.BaseURL = "http://npipe"
		}
		c.HTTP.Transport = localTransport(kind, addr)
		return c
	}
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	c.BaseURL = strings.TrimRight(raw, "/")
	c.DisplayURL = c.BaseURL
	return c
}

// Local reports whether the admin API is reached over a local socket/pipe.
func (c *Client) Local() bool { return isLocalURL(c.DisplayURL) }

func (c *Client) display() string {
	if c.DisplayURL != "" {
		return c.DisplayURL
	}
	return c.BaseURL
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if c.BaseURL == "" {
		return &Error{Message: "instance URL is required"}
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Message: fmt.Sprintf("cannot reach %s: %v", c.display(), err)}
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return errorFromBody(res)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func errorFromBody(res *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var body struct {
		Error  string   `json:"error"`
		Errors []string `json:"errors"`
	}
	_ = json.Unmarshal(b, &body)
	msg := body.Error
	if msg == "" && len(body.Errors) > 0 {
		msg = strings.Join(body.Errors, "; ")
	}
	if msg == "" {
		// Some routes answer an error as a bare JSON string.
		var s string
		if json.Unmarshal(b, &s) == nil {
			msg = strings.TrimSpace(s)
		}
	}
	if msg == "" {
		msg = strings.TrimSpace(string(b))
	}
	if msg == "" {
		msg = res.Status
	}
	if res.StatusCode == http.StatusUnauthorized {
		msg = "admin token rejected: " + msg
	}
	return &Error{Status: res.StatusCode, Message: msg}
}

// Ping verifies connectivity and the token.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/admin/api/state", nil, nil)
}

func (c *Client) Catalog(ctx context.Context) (Catalog, error) {
	var r struct {
		Catalog Catalog `json:"catalog"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/catalog", nil, &r)
	return r.Catalog, err
}

func (c *Client) Agents(ctx context.Context) ([]AgentRow, error) {
	var r struct {
		Agents []AgentRow `json:"agents"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/agents", nil, &r)
	return r.Agents, err
}

func (c *Client) Agent(ctx context.Context, name string) (AgentDetail, error) {
	var d AgentDetail
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/agents/"+url.PathEscape(name), nil, &d)
	return d, err
}

func (c *Client) Validate(ctx context.Context, req ValidateRequest) (ValidationResult, error) {
	var r ValidationResult
	err := c.do(ctx, http.MethodPost, "/admin/api/studio/agents/validate", req, &r)
	return r, err
}

func (c *Client) PutAgent(ctx context.Context, name string, req AgentRequest) (ApplyResult, error) {
	var r ApplyResult
	err := c.do(ctx, http.MethodPut, "/admin/api/studio/agents/"+url.PathEscape(name), req, &r)
	return r, err
}

func (c *Client) DeleteAgent(ctx context.Context, name string, persist, force bool) error {
	path := fmt.Sprintf("/admin/api/studio/agents/%s?persist=%t&force=%t",
		url.PathEscape(name), persist, force)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) Threads(ctx context.Context, agent string) ([]Thread, error) {
	path := "/admin/api/studio/threads"
	if agent != "" {
		path += "?agent=" + url.QueryEscape(agent)
	}
	var r struct {
		Threads []Thread `json:"threads"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &r)
	return r.Threads, err
}

func (c *Client) Trace(ctx context.Context, agent, thread string) ([]TraceEvent, error) {
	path := fmt.Sprintf("/admin/api/studio/trace/%s/%s", url.PathEscape(agent), url.PathEscape(thread))
	var r struct {
		Events []TraceEvent `json:"events"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &r)
	return r.Events, err
}

func (c *Client) StopThread(ctx context.Context, agent, thread string) error {
	path := fmt.Sprintf("/admin/api/studio/threads/%s/stop?agent=%s",
		url.PathEscape(thread), url.QueryEscape(agent))
	return c.do(ctx, http.MethodPost, path, struct{}{}, nil)
}

// Chat runs one playground turn and calls onEvent for every SSE event
// (trace, reply, error, done). It blocks until the stream ends or ctx is
// canceled. onEvent runs on the caller's goroutine, so a UI caller must
// marshal its updates onto the main thread itself.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onEvent func(ChatEvent)) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/admin/api/studio/chat", bytes.NewReader(b))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		hreq.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(hreq)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Message: fmt.Sprintf("cannot reach %s: %v", c.display(), err)}
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return errorFromBody(res)
	}

	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)

	var event string
	var data []string
	dispatch := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		var ev ChatEvent
		if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &ev); err != nil {
			return fmt.Errorf("bad stream event: %w", err)
		}
		if ev.Type == "" {
			ev.Type = event
		}
		onEvent(ev)
		event, data = "", nil
		return nil
	}

	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// keepalive comment
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("stream: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return dispatch()
}

// Channels lists the persisted gateway channels.
func (c *Client) Channels(ctx context.Context) (ChannelList, error) {
	var out ChannelList
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/channels", nil, &out)
	return out, err
}

// ValidateChannel dry-runs one draft. index -1 means a new channel.
func (c *Client) ValidateChannel(ctx context.Context, index int, draft ChannelDraft) ([]string, error) {
	var out struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
	}
	body := map[string]any{"index": index, "channel": draft}
	if err := c.do(ctx, http.MethodPost, "/admin/api/studio/channels/validate", body, &out); err != nil {
		return nil, err
	}
	return out.Errors, nil
}

// SaveChannel creates (index < 0) or replaces a channel and persists the set.
func (c *Client) SaveChannel(ctx context.Context, index int, draft ChannelDraft, mtime int64) (ChannelList, error) {
	var out ChannelList
	body := map[string]any{"channel": draft, "mtime": mtime}
	method, path := http.MethodPost, "/admin/api/studio/channels"
	if index >= 0 {
		method = http.MethodPut
		path = fmt.Sprintf("/admin/api/studio/channels/%d", index)
	}
	err := c.do(ctx, method, path, body, &out)
	return out, err
}

// DeleteChannel removes a channel and persists the remaining set.
func (c *Client) DeleteChannel(ctx context.Context, index int, mtime int64) (ChannelList, error) {
	var out ChannelList
	path := fmt.Sprintf("/admin/api/studio/channels/%d?mtime=%d", index, mtime)
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}

// Models lists live and configured models with their persistence state.
func (c *Client) Models(ctx context.Context) ([]Model, string, error) {
	var r struct {
		Models    []Model `json:"models"`
		FileError string  `json:"file_error"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/models", nil, &r)
	return r.Models, r.FileError, err
}

func (c *Client) PutModel(ctx context.Context, name string, d ModelDraft) error {
	return c.do(ctx, http.MethodPut, "/admin/api/models/"+url.PathEscape(name), d, nil)
}

func (c *Client) DeleteModel(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/admin/api/models/"+url.PathEscape(name), nil, nil)
}

func (c *Client) TestModel(ctx context.Context, name string) (ModelTest, error) {
	var r ModelTest
	err := c.do(ctx, http.MethodPost, "/admin/api/models/"+url.PathEscape(name)+"/test", struct{}{}, &r)
	return r, err
}

func (c *Client) PersistModels(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/api/models/persist", struct{}{}, nil)
}

func (c *Client) RevertModels(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/api/models/revert", struct{}{}, nil)
}

// Safety lists the persisted safety profiles and model-backed classifiers.
func (c *Client) Safety(ctx context.Context) (SafetyList, error) {
	var out SafetyList
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/safety", nil, &out)
	return out, err
}

// ValidateSafety dry-runs one profile or classifier draft.
func (c *Client) ValidateSafety(ctx context.Context, req SafetyValidateRequest) ([]string, error) {
	var out struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
	}
	if err := c.do(ctx, http.MethodPost, "/admin/api/studio/safety/validate", req, &out); err != nil {
		return nil, err
	}
	return out.Errors, nil
}

// SaveSafetyProfile creates (create) or replaces a named filter chain.
func (c *Client) SaveSafetyProfile(ctx context.Context, name string, create bool, d SafetyProfileDraft, mtime int64) (SafetyList, error) {
	var out SafetyList
	body := map[string]any{"profile": d, "mtime": mtime}
	method, path := http.MethodPost, "/admin/api/studio/safety/profiles"
	if !create {
		method = http.MethodPut
		path += "/" + url.PathEscape(name)
	}
	err := c.do(ctx, method, path, body, &out)
	return out, err
}

// DeleteSafetyProfile removes a profile; the runtime refuses while an agent
// still references it.
func (c *Client) DeleteSafetyProfile(ctx context.Context, name string, mtime int64) (SafetyList, error) {
	var out SafetyList
	path := fmt.Sprintf("/admin/api/studio/safety/profiles/%s?mtime=%d", url.PathEscape(name), mtime)
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}

// SaveSafetyClassifier creates (create) or replaces a model-backed classifier.
func (c *Client) SaveSafetyClassifier(ctx context.Context, name string, create bool, d SafetyClassifierDraft, mtime int64) (SafetyList, error) {
	var out SafetyList
	body := map[string]any{"classifier": d, "mtime": mtime}
	method, path := http.MethodPost, "/admin/api/studio/safety/classifiers"
	if !create {
		method = http.MethodPut
		path += "/" + url.PathEscape(name)
	}
	err := c.do(ctx, method, path, body, &out)
	return out, err
}

// DeleteSafetyClassifier removes a classifier; the runtime refuses while a
// profile still references it.
func (c *Client) DeleteSafetyClassifier(ctx context.Context, name string, mtime int64) (SafetyList, error) {
	var out SafetyList
	path := fmt.Sprintf("/admin/api/studio/safety/classifiers/%s?mtime=%d", url.PathEscape(name), mtime)
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}

// Memory lists the persisted memory backends and profiles.
func (c *Client) Memory(ctx context.Context) (MemoryList, error) {
	var out MemoryList
	err := c.do(ctx, http.MethodGet, "/admin/api/studio/memory", nil, &out)
	return out, err
}

// ValidateMemory dry-runs one profile or backend draft.
func (c *Client) ValidateMemory(ctx context.Context, req MemoryValidateRequest) ([]string, error) {
	var out struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
	}
	if err := c.do(ctx, http.MethodPost, "/admin/api/studio/memory/validate", req, &out); err != nil {
		return nil, err
	}
	return out.Errors, nil
}

// SaveMemoryProfile creates (create) or replaces a memory profile.
func (c *Client) SaveMemoryProfile(ctx context.Context, name string, create bool, d MemoryProfileDraft, mtime int64) (MemoryList, error) {
	return c.saveMemory(ctx, "profiles", name, create, map[string]any{"profile": d, "mtime": mtime})
}

// SaveMemoryBackend creates (create) or replaces a memory backend.
func (c *Client) SaveMemoryBackend(ctx context.Context, name string, create bool, d MemoryBackendDraft, mtime int64) (MemoryList, error) {
	return c.saveMemory(ctx, "backends", name, create, map[string]any{"backend": d, "mtime": mtime})
}

func (c *Client) saveMemory(ctx context.Context, kind, name string, create bool, body map[string]any) (MemoryList, error) {
	var out MemoryList
	method, path := http.MethodPost, "/admin/api/studio/memory/"+kind
	if !create {
		method = http.MethodPut
		path += "/" + url.PathEscape(name)
	}
	err := c.do(ctx, method, path, body, &out)
	return out, err
}

// DeleteMemoryProfile removes a profile; the runtime refuses while an agent
// or spawn template names it.
func (c *Client) DeleteMemoryProfile(ctx context.Context, name string, mtime int64) (MemoryList, error) {
	return c.deleteMemory(ctx, "profiles", name, mtime)
}

// DeleteMemoryBackend removes a backend; the runtime refuses while a store is
// on it.
func (c *Client) DeleteMemoryBackend(ctx context.Context, name string, mtime int64) (MemoryList, error) {
	return c.deleteMemory(ctx, "backends", name, mtime)
}

func (c *Client) deleteMemory(ctx context.Context, kind, name string, mtime int64) (MemoryList, error) {
	var out MemoryList
	path := fmt.Sprintf("/admin/api/studio/memory/%s/%s?mtime=%d", kind, url.PathEscape(name), mtime)
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}

// Credentials lists one credential bucket. An empty user means the
// engine-wide bucket.
func (c *Client) Credentials(ctx context.Context, user string) (CredentialList, error) {
	path := "/admin/api/credentials"
	if user != "" {
		path += "?user=" + url.QueryEscape(user)
	}
	var out CredentialList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// PutCredential creates or updates one secret. The response is the refreshed
// bucket list; the value never appears in it.
func (c *Client) PutCredential(ctx context.Context, d CredentialDraft) (CredentialList, error) {
	var out CredentialList
	err := c.do(ctx, http.MethodPost, "/admin/api/credentials", d, &out)
	return out, err
}

// DeleteCredential revokes one secret and returns the refreshed bucket.
func (c *Client) DeleteCredential(ctx context.Context, user, service string) (CredentialList, error) {
	path := "/admin/api/credentials?service=" + url.QueryEscape(service)
	if user != "" {
		path += "&user=" + url.QueryEscape(user)
	}
	var out CredentialList
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}
