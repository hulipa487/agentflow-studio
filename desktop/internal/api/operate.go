package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// This file covers the operator side of /admin/api: instance state, metrics,
// the log tail, the raw config, users, usage and tenancy. Every route here
// already exists in the runtime; Studio only had no screen for it.

// ---- instance state -----------------------------------------------------------

type StateAgent struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Loop       string `json:"loop"`
	Persistent bool   `json:"persistent"`
	Singleton  bool   `json:"singleton"`
}

type StateChannel struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Agent        string `json:"agent"`
	Path         string `json:"path"`
	MediaEnabled bool   `json:"media_enabled"`
	Index        int    `json:"index"`
}

type StateSession struct {
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	ParentID  string `json:"parent_id"`
	Busy      bool   `json:"busy"`
}

// StateInstance is one live process in the runtime's instance registry.
type StateInstance struct {
	Owner              string `json:"owner"`
	Region             string `json:"region"`
	Zone               string `json:"zone"`
	Version            string `json:"version"`
	Topology           string `json:"topology"`
	ConfigEpoch        string `json:"config_epoch"`
	ConfigEpochDiffers bool   `json:"config_epoch_differs"`
	StartedAt          int64  `json:"started_at"`
	LastSeen           int64  `json:"last_seen"`
	Self               bool   `json:"self"`
}

type State struct {
	OK                 bool            `json:"ok"`
	Version            string          `json:"version"`
	StartedAt          int64           `json:"started_at"`
	UptimeS            int64           `json:"uptime_s"`
	ConfigPath         string          `json:"config_path"`
	ConfigEpoch        string          `json:"config_epoch"`
	Instance           string          `json:"instance"`
	Region             string          `json:"region"`
	Topology           string          `json:"topology"`
	Zone               string          `json:"zone"`
	Instances          []StateInstance `json:"instances"`
	Agents             []StateAgent    `json:"agents"`
	Channels           []StateChannel  `json:"channels"`
	Sessions           []StateSession  `json:"sessions"`
	SessionsActive     int             `json:"sessions_active"`
	SessionsIdle       int             `json:"sessions_idle"`
	CredentialsEnabled bool            `json:"credentials_enabled"`
}

// State is the instance snapshot behind the Overview screen.
func (c *Client) State(ctx context.Context) (State, error) {
	var s State
	err := c.do(ctx, http.MethodGet, "/admin/api/state", nil, &s)
	return s, err
}

// ---- metrics ----------------------------------------------------------------

type MetricPoint struct {
	At    int64 `json:"at"`
	Value int64 `json:"v"`
}

type Metrics struct {
	OK     bool                     `json:"ok"`
	Series map[string][]MetricPoint `json:"series"`
	Latest map[string]int64         `json:"latest"`
}

// Metrics returns the in-process sampled counter history (~2h at 5s).
func (c *Client) Metrics(ctx context.Context) (Metrics, error) {
	var m Metrics
	err := c.do(ctx, http.MethodGet, "/admin/api/metrics", nil, &m)
	return m, err
}

// ---- logs -------------------------------------------------------------------

// Logs streams the runtime's log tail: the ring is replayed first, then live
// lines arrive until ctx is canceled. onLine runs on the caller's goroutine.
func (c *Client) Logs(ctx context.Context, onLine func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/admin/api/logs", nil)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "text/event-stream")
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
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "data:") {
			onLine(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("log stream: %w", err)
	}
	return nil
}

// ---- raw config ---------------------------------------------------------------

// ConfigFile is the raw config text. OK is false (with Error set) when the
// instance runs from a config directory, which this editor does not write.
type ConfigFile struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Path  string `json:"path"`
	Raw   string `json:"raw"`
	Mtime int64  `json:"mtime"`
	View  any    `json:"view"`
}

type ConfigResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Mtime int64  `json:"mtime"`
	Note  string `json:"note"`
}

func (c *Client) Config(ctx context.Context) (ConfigFile, error) {
	var f ConfigFile
	err := c.do(ctx, http.MethodGet, "/admin/api/config", nil, &f)
	return f, err
}

// ValidateConfig runs boot-equivalent validation; a refusal is OK=false with
// the reason in Error, not a transport error.
func (c *Client) ValidateConfig(ctx context.Context, raw string) (ConfigResult, error) {
	var r ConfigResult
	err := c.do(ctx, http.MethodPost, "/admin/api/config/validate", map[string]string{"raw": raw}, &r)
	return r, err
}

// SaveConfig validates, backs up to .bak and atomically replaces the file. A
// 409 means the file changed on disk since mtime was read.
func (c *Client) SaveConfig(ctx context.Context, raw string, mtime int64) (ConfigResult, error) {
	var r ConfigResult
	err := c.do(ctx, http.MethodPost, "/admin/api/config/save", map[string]any{"raw": raw, "mtime": mtime}, &r)
	return r, err
}

// ---- users --------------------------------------------------------------------

type Quota struct {
	Used     int64 `json:"used"`
	Limit    int64 `json:"limit"`
	InFlight int64 `json:"in_flight"`
}

type UserRow struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	TokensPerDay int64  `json:"tokens_per_day"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	Identities   int    `json:"identities"`
	Quota        *Quota `json:"quota"`
}

type UserList struct {
	Day   string    `json:"day"`
	Users []UserRow `json:"users"`
}

type Identity struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Channel     string `json:"channel"`
	NativeFrom  string `json:"native_from"`
	ReplyTo     string `json:"reply_to"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Trust       string `json:"trust"`
	Deliverable bool   `json:"deliverable"`
	Linkable    bool   `json:"linkable"`
	FirstSeen   int64  `json:"first_seen"`
	LastSeen    int64  `json:"last_seen"`
}

type Profile struct {
	UserID             string     `json:"user_id"`
	DisplayName        string     `json:"display_name"`
	Email              string     `json:"email"`
	TokensPerDay       int64      `json:"tokens_per_day"`
	Model              string     `json:"model"`
	InstructionsAppend string     `json:"instructions_append"`
	CreatedAt          int64      `json:"created_at"`
	UpdatedAt          int64      `json:"updated_at"`
	Identities         []Identity `json:"identities"`
}

type UsageTotals struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	Cached     int64 `json:"cached"`
	CacheWrite int64 `json:"cache_write"`
	Reasoning  int64 `json:"reasoning"`
	Calls      int64 `json:"calls"`
	Failed     int64 `json:"failed"`
}

// Billable mirrors the runtime's rule: input + output.
func (t UsageTotals) Billable() int64 { return t.Input + t.Output }

type UsageEvent struct {
	Ts    int64  `json:"ts"`
	Kind  string `json:"kind"`
	Model string `json:"model"`
	Agent string `json:"agent"`
	UsageTotals
	OK bool `json:"ok"`
}

type JournalEntry struct {
	ID        string `json:"id"`
	Ts        int64  `json:"ts"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	Channel   string `json:"channel"`
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	UserUUID  string `json:"user_uuid"`
	Agent     string `json:"agent"`
	SessionID string `json:"session_id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	Err       string `json:"err"`
}

type UserDetail struct {
	Profile     Profile        `json:"profile"`
	Day         string         `json:"day"`
	UsageToday  *UsageTotals   `json:"usage_today"`
	RecentCalls []UsageEvent   `json:"recent_calls"`
	Audit       []JournalEntry `json:"audit"`
	Quota       *Quota         `json:"quota"`
	Projects    []string       `json:"projects"`
}

type Invite struct {
	Invite    string `json:"invite"`
	ExpiresIn string `json:"expires_in"`
	Hint      string `json:"hint"`
}

// UserSettings sets a profile's model and instruction layer. A nil field is
// left unchanged; an empty string clears it.
type UserSettings struct {
	Model              *string `json:"model,omitempty"`
	InstructionsAppend *string `json:"instructions_append,omitempty"`
}

func (c *Client) Users(ctx context.Context) (UserList, error) {
	var l UserList
	err := c.do(ctx, http.MethodGet, "/admin/api/users", nil, &l)
	return l, err
}

func (c *Client) User(ctx context.Context, id string) (UserDetail, error) {
	var d UserDetail
	err := c.do(ctx, http.MethodGet, "/admin/api/users/"+url.PathEscape(id), nil, &d)
	return d, err
}

// SetUserLimit sets the profile's daily token limit (0 inherits the default).
func (c *Client) SetUserLimit(ctx context.Context, id string, tokensPerDay int64) (Profile, error) {
	var r struct {
		Profile Profile `json:"profile"`
	}
	err := c.do(ctx, http.MethodPost, "/admin/api/users/"+url.PathEscape(id)+"/limit",
		map[string]int64{"tokens_per_day": tokensPerDay}, &r)
	return r.Profile, err
}

func (c *Client) SetUserSettings(ctx context.Context, id string, s UserSettings) (Profile, error) {
	var r struct {
		Profile Profile `json:"profile"`
	}
	err := c.do(ctx, http.MethodPut, "/admin/api/users/"+url.PathEscape(id)+"/settings", s, &r)
	return r.Profile, err
}

// UnlinkIdentity detaches one channel handle; the runtime refuses the last one.
func (c *Client) UnlinkIdentity(ctx context.Context, id, identityID string) (Profile, error) {
	var r struct {
		Profile Profile `json:"profile"`
	}
	err := c.do(ctx, http.MethodPost, "/admin/api/users/"+url.PathEscape(id)+"/unlink",
		map[string]string{"identity_id": identityID}, &r)
	return r.Profile, err
}

func (c *Client) IssueInvite(ctx context.Context) (Invite, error) {
	var inv Invite
	err := c.do(ctx, http.MethodPost, "/admin/api/users/invites", struct{}{}, &inv)
	return inv, err
}

// ExportUser returns everything held for a profile as pretty-printed JSON.
func (c *Client) ExportUser(ctx context.Context, id string) ([]byte, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/admin/api/users/"+url.PathEscape(id)+"/export", nil, &raw); err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw, nil
	}
	return json.MarshalIndent(v, "", "  ")
}

// DeleteUser purges a profile. The runtime requires ?confirm=<id>.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	path := "/admin/api/users/" + url.PathEscape(id) + "?confirm=" + url.QueryEscape(id)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// ---- usage --------------------------------------------------------------------

type UsageRow struct {
	UserID string `json:"user_id"`
	Day    string `json:"day"`
	Agent  string `json:"agent"`
	Model  string `json:"model"`
	UsageTotals
}

type UsageDay struct {
	OK     bool       `json:"ok"`
	Day    string     `json:"day"`
	Rows   []UsageRow `json:"rows"`
	Totals struct {
		Input    int64 `json:"input"`
		Output   int64 `json:"output"`
		Billable int64 `json:"billable"`
		Calls    int64 `json:"calls"`
	} `json:"totals"`
	Tenants int `json:"tenants"`
}

// Usage returns the token rollup for one UTC day (YYYY-MM-DD; empty = today).
func (c *Client) Usage(ctx context.Context, day string) (UsageDay, error) {
	path := "/admin/api/usage"
	if day != "" {
		path += "?day=" + url.QueryEscape(day)
	}
	var u UsageDay
	err := c.do(ctx, http.MethodGet, path, nil, &u)
	return u, err
}

// ---- tenancy --------------------------------------------------------------------

type Group struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Master      string `json:"master"`
	CreatedAt   int64  `json:"created_at"`
	DisbandedAt int64  `json:"disbanded_at"`
}

type Membership struct {
	Group    string `json:"group"`
	User     string `json:"user"`
	JoinedAt int64  `json:"joined_at"`
}

type GroupDetail struct {
	Group   Group        `json:"group"`
	Members []Membership `json:"members"`
}

type Pool struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Owner struct {
		Tenant string `json:"tenant"`
		Group  string `json:"group"`
	} `json:"owner"`
	RetentionDays int    `json:"retention_days"`
	CreatedAt     int64  `json:"created_at"`
	CreatedBy     string `json:"created_by"`
}

type Grant struct {
	Pool   string `json:"pool"`
	Agent  string `json:"agent"`
	Tenant string `json:"tenant"`
	Mode   string `json:"mode"`
}

type PoolDraft struct {
	Name          string `json:"name"`
	Creator       string `json:"creator"`
	OwnerTenant   string `json:"owner_tenant,omitempty"`
	OwnerGroup    string `json:"owner_group,omitempty"`
	RetentionDays int    `json:"retention_days"`
}

func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var r struct {
		Groups []Group `json:"groups"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/groups", nil, &r)
	return r.Groups, err
}

func (c *Client) Group(ctx context.Context, id string) (GroupDetail, error) {
	var d GroupDetail
	err := c.do(ctx, http.MethodGet, "/admin/api/groups/"+url.PathEscape(id), nil, &d)
	return d, err
}

// CreateGroup creates a group whose master is the creator's personal uuid.
func (c *Client) CreateGroup(ctx context.Context, name, creator string) (Group, error) {
	var r struct {
		Group Group `json:"group"`
	}
	err := c.do(ctx, http.MethodPost, "/admin/api/groups", map[string]string{"name": name, "creator": creator}, &r)
	return r.Group, err
}

func (c *Client) AddMember(ctx context.Context, group, user string) error {
	return c.do(ctx, http.MethodPost, "/admin/api/groups/"+url.PathEscape(group)+"/members",
		map[string]string{"user": user}, nil)
}

func (c *Client) RemoveMember(ctx context.Context, group, user string) error {
	return c.do(ctx, http.MethodDelete,
		"/admin/api/groups/"+url.PathEscape(group)+"/members/"+url.PathEscape(user), nil, nil)
}

// SetMaster hands the group from the current master (from) to a member.
func (c *Client) SetMaster(ctx context.Context, group, from, user string) error {
	return c.do(ctx, http.MethodPut, "/admin/api/groups/"+url.PathEscape(group)+"/master",
		map[string]string{"from": from, "user": user}, nil)
}

// DisbandGroup ends access now; by must be the current master.
func (c *Client) DisbandGroup(ctx context.Context, group, by string) error {
	return c.do(ctx, http.MethodDelete, "/admin/api/groups/"+url.PathEscape(group),
		map[string]string{"by": by}, nil)
}

func (c *Client) Pools(ctx context.Context) ([]Pool, error) {
	var r struct {
		Pools []Pool `json:"pools"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/pools", nil, &r)
	return r.Pools, err
}

func (c *Client) CreatePool(ctx context.Context, d PoolDraft) (Pool, error) {
	var r struct {
		Pool Pool `json:"pool"`
	}
	err := c.do(ctx, http.MethodPost, "/admin/api/pools", d, &r)
	return r.Pool, err
}

func (c *Client) Grants(ctx context.Context, pool string) ([]Grant, error) {
	var r struct {
		Grants []Grant `json:"grants"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/api/pools/"+url.PathEscape(pool)+"/grants", nil, &r)
	return r.Grants, err
}

// SetGrant gives agent access to the pool for tenant ("" = every member) in
// mode "rw" or "r".
func (c *Client) SetGrant(ctx context.Context, pool, agent, tenant, mode string) error {
	return c.do(ctx, http.MethodPut, "/admin/api/pools/"+url.PathEscape(pool)+"/grants",
		map[string]string{"agent": agent, "tenant": tenant, "mode": mode}, nil)
}

func (c *Client) RemoveGrant(ctx context.Context, pool, agent, tenant string) error {
	q := url.Values{}
	q.Set("agent", agent)
	q.Set("tenant", tenant)
	return c.do(ctx, http.MethodDelete, "/admin/api/pools/"+url.PathEscape(pool)+"/grants?"+q.Encode(), nil, nil)
}
