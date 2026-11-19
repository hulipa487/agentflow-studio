// Package api is the desktop client for AgentFlow's /admin/api surface.
// It has no UI dependency, so it can be tested without a display.
package api

import "time"

type Template struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Loop        string `json:"loop"`
}

type Catalog struct {
	Models              []string   `json:"models"`
	Tools               []string   `json:"tools"`
	Capabilities        []string   `json:"capabilities"`
	DefaultCapabilities []string   `json:"default_capabilities"`
	SafetyProfiles      []string   `json:"safety_profiles"`
	MemoryProfiles      []string   `json:"memory_profiles"`
	PromptKeys          []string   `json:"prompt_keys"`
	PreludeVersion      int        `json:"prelude_version"`
	Templates           []Template `json:"templates"`
}

type AgentRow struct {
	Name         string   `json:"name"`
	Model        string   `json:"model"`
	Loop         string   `json:"loop"`
	Spawn        bool     `json:"spawn"`
	Persistent   bool     `json:"persistent"`
	Capabilities []string `json:"capabilities"`
	Persisted    bool     `json:"persisted"`
	Sessions     int      `json:"sessions"`
}

type AgentJSON struct {
	Loop          string         `json:"loop"`
	Model         string         `json:"model"`
	HistoryBudget int            `json:"history_budget"`
	Safety        string         `json:"safety"`
	Shell         string         `json:"shell"`
	Skills        []string       `json:"skills"`
	Capabilities  []string       `json:"capabilities"`
	CanContact    []string       `json:"can_contact"`
	Persistent    bool           `json:"persistent"`
	Credentials   []string       `json:"credentials"`
	InheritUser   *bool          `json:"inherit_user"`
	Extras        map[string]any `json:"extras"`
}

type AgentFiles struct {
	Mode             string `json:"mode"`
	ProfilePath      string `json:"profile_path"`
	LoopPath         string `json:"loop_path"`
	InstructionsPath string `json:"instructions_path"`
	Managed          bool   `json:"managed"`
	Mtime            int64  `json:"mtime"`
}

type AgentDetail struct {
	OK           bool       `json:"ok"`
	Name         string     `json:"name"`
	Source       string     `json:"source"`
	AgentYAML    string     `json:"agent_yaml"`
	Agent        AgentJSON  `json:"agent"`
	Loop         string     `json:"loop"`
	Instructions string     `json:"instructions"`
	Files        AgentFiles `json:"files"`
	Persisted    bool       `json:"persisted"`
	Spawn        bool       `json:"spawn"`
	Persistent   bool       `json:"persistent"`
	Model        string     `json:"model"`
	Capabilities []string   `json:"capabilities"`
	Skills       []string   `json:"skills"`
	Sessions     int        `json:"sessions"`
}

type TraceEvent struct {
	Seq             int64     `json:"seq"`
	Ts              time.Time `json:"ts"`
	Agent           string    `json:"agent"`
	Session         string    `json:"session"`
	Thread          string    `json:"thread"`
	Turn            string    `json:"turn"`
	Kind            string    `json:"kind"`
	Op              string    `json:"op"`
	Tool            string    `json:"tool"`
	Model           string    `json:"model"`
	OK              bool      `json:"ok"`
	Err             string    `json:"err"`
	DurationMS      int64     `json:"duration_ms"`
	TokensIn        int       `json:"tokens_in"`
	TokensOut       int       `json:"tokens_out"`
	TokensReasoning int       `json:"tokens_reasoning"`
	ToolCalls       int       `json:"tool_calls"`
	Messages        int       `json:"messages"`
	Text            string    `json:"text"`
}

type Reply struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

type ChatResult struct {
	Thread   string  `json:"thread"`
	Session  string  `json:"session"`
	Replies  []Reply `json:"replies"`
	Events   int     `json:"events"`
	TimedOut bool    `json:"timed_out"`
}

type ChatEvent struct {
	Type  string      `json:"type"`
	Trace *TraceEvent `json:"trace,omitempty"`
	Reply *Reply      `json:"reply,omitempty"`
	Done  *ChatResult `json:"done,omitempty"`
	Error string      `json:"error,omitempty"`
}

type Thread struct {
	Agent   string    `json:"agent"`
	Thread  string    `json:"thread"`
	Session string    `json:"session"`
	Last    time.Time `json:"last"`
	Events  int       `json:"events"`
}

type ValidationResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type ApplyResult struct {
	OK           bool       `json:"ok"`
	Applied      bool       `json:"applied"`
	Persisted    bool       `json:"persisted"`
	Files        AgentFiles `json:"files"`
	Note         string     `json:"note"`
	PersistError string     `json:"persist_error"`
}

type ChatRequest struct {
	Agent  string `json:"agent"`
	Thread string `json:"thread"`
	Text   string `json:"text"`
	ActAs  string `json:"act_as_user,omitempty"`
}

type AgentRequest struct {
	AgentYAML    string `json:"agent_yaml"`
	Loop         string `json:"loop"`
	Instructions string `json:"instructions"`
	Apply        bool   `json:"apply"`
	Persist      bool   `json:"persist"`
	Force        bool   `json:"force"`
	ExpectMtime  int64  `json:"expect_mtime"`
}

type ValidateRequest struct {
	Name         string `json:"name"`
	AgentYAML    string `json:"agent_yaml"`
	Loop         string `json:"loop"`
	Instructions string `json:"instructions"`
}

// Channel is one configured gateway channel as Studio sees it. Secrets are
// write-only on the API: Has* says one is stored, and *Ref carries an env or
// credential placeholder because it reveals nothing.
type Channel struct {
	Index           int      `json:"index"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Agent           string   `json:"agent"`
	Agents          []string `json:"agents,omitempty"`
	Mode            string   `json:"mode"`
	Path            string   `json:"path"`
	Timeout         string   `json:"timeout"`
	Async           bool     `json:"async"`
	DeliverURL      string   `json:"deliver_url"`
	AppID           string   `json:"app_id,omitempty"`
	Scenes          []string `json:"scenes,omitempty"`
	APIBase         string   `json:"api_base,omitempty"`
	Markdown        bool     `json:"markdown,omitempty"`
	Auth            string   `json:"auth,omitempty"`
	MaxRequestBytes int      `json:"max_request_bytes,omitempty"`
	ExposeReasoning bool     `json:"expose_reasoning,omitempty"`
	Journal         string   `json:"journal,omitempty"`
	AllowUsers      []int64  `json:"allow_users"`
	MediaAllow      []string `json:"media_allow"`
	MediaMaxBytes   int      `json:"media_max_bytes"`
	HasToken        bool     `json:"has_token"`
	HasSecret       bool     `json:"has_secret"`
	HasSecretTok    bool     `json:"has_secret_token"`
	HasKeys         bool     `json:"has_keys"`
	TokenRef        string   `json:"token_ref"`
	SecretRef       string   `json:"secret_ref"`
	SecretTokRef    string   `json:"secret_token_ref"`
	KeyRefs         []string `json:"key_refs,omitempty"`
	Live            bool     `json:"live"`
}

// ChannelList is the persisted channel set and its source.
type ChannelList struct {
	OK              bool      `json:"ok"`
	RestartRequired bool      `json:"restart_required"`
	SavedNote       string    `json:"saved_note"`
	Mode            string    `json:"mode"`
	Path            string    `json:"path"`
	Editable        bool      `json:"editable"`
	MTime           int64     `json:"mtime"`
	Channels        []Channel `json:"channels"`
	Note            string    `json:"note"`
}

// ChannelDraft is the editable shape of a channel. An empty secret keeps the
// stored one; Clear* removes it.
type ChannelDraft struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Agent           string   `json:"agent"`
	Agents          []string `json:"agents,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	Path            string   `json:"path,omitempty"`
	Token           string   `json:"token,omitempty"`
	Secret          string   `json:"secret,omitempty"`
	SecretToken     string   `json:"secret_token,omitempty"`
	ClearToken      bool     `json:"clear_token,omitempty"`
	ClearSecret     bool     `json:"clear_secret,omitempty"`
	ClearSecretTok  bool     `json:"clear_secret_token,omitempty"`
	Auth            string   `json:"auth,omitempty"`
	Keys            []string `json:"keys,omitempty"`
	ClearKeys       bool     `json:"clear_keys,omitempty"`
	MaxRequestBytes int      `json:"max_request_bytes,omitempty"`
	ExposeReasoning bool     `json:"expose_reasoning,omitempty"`
	Journal         string   `json:"journal,omitempty"`
	AllowUsers      []int64  `json:"allow_users,omitempty"`
	Timeout         string   `json:"timeout,omitempty"`
	Async           bool     `json:"async,omitempty"`
	DeliverURL      string   `json:"deliver_url,omitempty"`
	AppID           string   `json:"app_id,omitempty"`
	Scenes          []string `json:"scenes,omitempty"`
	APIBase         string   `json:"api_base,omitempty"`
	Markdown        bool     `json:"markdown,omitempty"`
	MediaAllow      []string `json:"media_allow,omitempty"`
	MediaMaxBytes   int      `json:"media_max_bytes,omitempty"`
}

// Model is one LLM endpoint from the runtime's models: section. Secrets are
// masked: HasKey plus a last-four fingerprint.
type Model struct {
	Name           string   `json:"name"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	BaseURL        string   `json:"base_url"`
	Timeout        string   `json:"timeout"`
	Retry          int      `json:"retry"`
	MaxTokens      int      `json:"max_tokens"`
	Thinking       string   `json:"thinking"`
	ServerTools    []string `json:"server_tools"`
	HasKey         bool     `json:"has_key"`
	KeyFingerprint string   `json:"key_fingerprint"`
	InRuntime      bool     `json:"in_runtime"`
	InFile         bool     `json:"in_file"`
	Drift          bool     `json:"drift"`
}

// ModelDraft is the editable shape sent on create/update. An empty APIKey on
// an existing model keeps the stored key.
type ModelDraft struct {
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	APIKey      string   `json:"api_key"`
	BaseURL     string   `json:"base_url"`
	Timeout     string   `json:"timeout"`
	Retry       int      `json:"retry"`
	MaxTokens   int      `json:"max_tokens"`
	Thinking    string   `json:"thinking"`
	ServerTools []string `json:"server_tools"`
}

// ModelTest is the verdict of a one-call live check.
type ModelTest struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Class     string `json:"class"`
	Error     string `json:"error"`
}

// SafetyProfile is one named core-owned filter chain, with the agents that
// reference it and whether the running process loaded it.
type SafetyProfile struct {
	Name          string   `json:"name"`
	Filters       []string `json:"filters"`
	Agents        []string `json:"agents"`
	SpawnProfiles []string `json:"spawn_profiles"`
	Classifiers   []string `json:"classifiers"`
	Live          bool     `json:"live"`
}

// SafetyClassifier is one model-backed moderation/classification policy.
type SafetyClassifier struct {
	Name           string                 `json:"name"`
	Model          string                 `json:"model"`
	Prompt         string                 `json:"prompt,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Directions     []string               `json:"directions,omitempty"`
	Sources        []string               `json:"sources,omitempty"`
	Threshold      *float64               `json:"threshold,omitempty"`
	Categories     map[string]string      `json:"categories"`
	Timeout        string                 `json:"timeout,omitempty"`
	OnError        string                 `json:"on_error,omitempty"`
	OnSaturated    string                 `json:"on_saturated,omitempty"`
	OnBudget       string                 `json:"on_budget_exhausted,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	MaxInputChars  int                    `json:"max_input_chars,omitempty"`
	MaxConcurrency int                    `json:"max_concurrency,omitempty"`
	Cache          SafetyClassifierCache  `json:"cache"`
	Budget         SafetyClassifierBudget `json:"budget"`
	Refusal        string                 `json:"refusal,omitempty"`
	Fallback       string                 `json:"fallback,omitempty"`
	UsedBy         []string               `json:"used_by"`
	Live           bool                   `json:"live"`
}

type SafetyClassifierCache struct {
	Size int    `json:"size,omitempty"`
	TTL  string `json:"ttl,omitempty"`
}

type SafetyClassifierBudget struct {
	TokensPerDay int64 `json:"tokens_per_day,omitempty"`
}

// SafetyList is the editable safety surface and its persistence state.
type SafetyList struct {
	OK              bool               `json:"ok"`
	RestartRequired bool               `json:"restart_required"`
	SavedNote       string             `json:"saved_note"`
	Mode            string             `json:"mode"`
	Path            string             `json:"path"`
	Editable        bool               `json:"editable"`
	MTime           int64              `json:"mtime"`
	BuiltinFilters  []string           `json:"builtin_filters"`
	Profiles        []SafetyProfile    `json:"profiles"`
	Classifiers     []SafetyClassifier `json:"classifiers"`
	Models          []string           `json:"models"`
	Prompts         []string           `json:"prompts"`
	Note            string             `json:"note"`
}

// SafetyProfileDraft is the editable shape of a profile. The filter order is
// the order the chain runs.
type SafetyProfileDraft struct {
	Name    string   `json:"name"`
	Filters []string `json:"filters"`
}

// SafetyClassifierDraft mirrors the config knobs. A nil Threshold keeps the
// runtime default (0.7).
type SafetyClassifierDraft struct {
	Name           string                 `json:"name"`
	Model          string                 `json:"model"`
	Prompt         string                 `json:"prompt,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Directions     []string               `json:"directions,omitempty"`
	Sources        []string               `json:"sources,omitempty"`
	Threshold      *float64               `json:"threshold,omitempty"`
	Categories     map[string]string      `json:"categories"`
	Timeout        string                 `json:"timeout,omitempty"`
	OnError        string                 `json:"on_error,omitempty"`
	OnSaturated    string                 `json:"on_saturated,omitempty"`
	OnBudget       string                 `json:"on_budget_exhausted,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	MaxInputChars  int                    `json:"max_input_chars,omitempty"`
	MaxConcurrency int                    `json:"max_concurrency,omitempty"`
	Cache          SafetyClassifierCache  `json:"cache"`
	Budget         SafetyClassifierBudget `json:"budget"`
	Refusal        string                 `json:"refusal,omitempty"`
	Fallback       string                 `json:"fallback,omitempty"`
}

// SafetyValidateRequest names one draft to dry-run.
type SafetyValidateRequest struct {
	Kind       string                 `json:"kind"` // profile | classifier
	Creating   bool                   `json:"creating"`
	Profile    *SafetyProfileDraft    `json:"profile,omitempty"`
	Classifier *SafetyClassifierDraft `json:"classifier,omitempty"`
}

// Credential is one stored secret's metadata. The value is never sent; the
// fingerprint is its last four characters.
type Credential struct {
	Service     string `json:"service"`
	Kind        string `json:"kind"`
	Header      string `json:"header,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// CredentialList is one bucket ("" = engine-wide) and the buckets that can be
// selected. Values are write-only throughout.
type CredentialList struct {
	OK          bool         `json:"ok"`
	Enabled     bool         `json:"enabled"`
	Bucket      string       `json:"bucket"`
	Users       []string     `json:"users"`
	Credentials []Credential `json:"credentials"`
	Note        string       `json:"note"`
}

// CredentialDraft creates or updates one secret. An empty Secret on an
// existing service keeps the stored value.
type CredentialDraft struct {
	User    string `json:"user"`
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Secret  string `json:"secret"`
	Header  string `json:"header"`
	Scheme  string `json:"scheme"`
}
