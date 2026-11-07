package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recorder captures the last request a handler saw.
type recorder struct {
	method, path, query, body string
}

func serve(t *testing.T, rec *recorder, respond func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*rec = recorder{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(b)}
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok")
}

func TestStateAndMetrics(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/state":
			fmt.Fprint(w, `{"ok":true,"version":"v1","uptime_s":61,"sessions":[{"session_id":"echo|a","agent":"echo","busy":true}],"sessions_active":1,"credentials_enabled":true}`)
		case "/admin/api/metrics":
			fmt.Fprint(w, `{"ok":true,"series":{"agentflow_llm_calls":[{"at":1,"v":2},{"at":2,"v":5}]},"latest":{"agentflow_llm_calls":5}}`)
		}
	})
	st, err := c.State(context.Background())
	if err != nil || st.Version != "v1" || len(st.Sessions) != 1 || !st.Sessions[0].Busy || !st.CredentialsEnabled {
		t.Fatalf("state = %+v, %v", st, err)
	}
	m, err := c.Metrics(context.Background())
	if err != nil || m.Latest["agentflow_llm_calls"] != 5 || m.Series["agentflow_llm_calls"][1].Value != 5 {
		t.Fatalf("metrics = %+v, %v", m, err)
	}
}

func TestLogsStream(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: 10:00:00 INF agentflow up\n\ndata: 10:00:01 WRN slow\n\n")
	})
	var lines []string
	if err := c.Logs(context.Background(), func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[1] != "10:00:01 WRN slow" {
		t.Fatalf("lines = %q", lines)
	}
}

func TestConfigSaveSendsMtimeAndReports409(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"ok":false,"error":"config file changed on disk since you loaded it; reload before saving"}`)
	})
	_, err := c.SaveConfig(context.Background(), "version: \"1\"\n", 42)
	var ae *Error
	if !errors.As(err, &ae) || ae.Status != 409 || !strings.Contains(ae.Message, "changed on disk") {
		t.Fatalf("err = %v", err)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(rec.body), &body)
	if rec.path != "/admin/api/config/save" || body["mtime"] != float64(42) {
		t.Fatalf("request = %+v", rec)
	}
}

func TestBareJSONStringErrorIsUnquoted(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `"does not validate: agents.x: unknown model"`)
	})
	_, err := c.SaveConfig(context.Background(), "x", 1)
	if err == nil || err.Error() != "does not validate: agents.x: unknown model" {
		t.Fatalf("err = %v", err)
	}
}

func TestUserCalls(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/admin/api/users":
			fmt.Fprint(w, `{"day":"2026-11-07","users":[{"user_id":"u1","display_name":"Ada","identities":2,"quota":{"used":10,"limit":100}}]}`)
		case strings.HasSuffix(r.URL.Path, "/limit"), strings.HasSuffix(r.URL.Path, "/settings"), strings.HasSuffix(r.URL.Path, "/unlink"):
			fmt.Fprint(w, `{"profile":{"user_id":"u1","tokens_per_day":500}}`)
		case strings.HasSuffix(r.URL.Path, "/export"):
			fmt.Fprint(w, `{"profile":{"user_id":"u1"},"exported_at":"now"}`)
		case r.Method == http.MethodDelete:
			fmt.Fprint(w, `{"ok":true,"deleted":"u1"}`)
		default:
			fmt.Fprint(w, `{"profile":{"user_id":"u1","identities":[{"id":"i1","channel":"telegram","trust":"verified"}]},"usage_today":{"input":3,"output":4},"projects":["notes"]}`)
		}
	})
	ctx := context.Background()
	l, err := c.Users(ctx)
	if err != nil || len(l.Users) != 1 || l.Users[0].Quota.Limit != 100 {
		t.Fatalf("users = %+v, %v", l, err)
	}
	d, err := c.User(ctx, "u1")
	if err != nil || d.UsageToday.Billable() != 7 || d.Profile.Identities[0].Channel != "telegram" || d.Projects[0] != "notes" {
		t.Fatalf("user = %+v, %v", d, err)
	}
	if _, err := c.SetUserLimit(ctx, "u1", 500); err != nil || rec.body != `{"tokens_per_day":500}` {
		t.Fatalf("limit body = %q, %v", rec.body, err)
	}
	m := ""
	if _, err := c.SetUserSettings(ctx, "u1", UserSettings{Model: &m}); err != nil || rec.method != http.MethodPut || rec.body != `{"model":""}` {
		t.Fatalf("settings = %+v, %v", rec, err)
	}
	if _, err := c.UnlinkIdentity(ctx, "u1", "i1"); err != nil || rec.body != `{"identity_id":"i1"}` {
		t.Fatalf("unlink = %+v, %v", rec, err)
	}
	out, err := c.ExportUser(ctx, "u1")
	if err != nil || !strings.Contains(string(out), "\n  \"exported_at\"") {
		t.Fatalf("export = %s, %v", out, err)
	}
	if err := c.DeleteUser(ctx, "u1"); err != nil || rec.query != "confirm=u1" {
		t.Fatalf("delete = %+v, %v", rec, err)
	}
}

func TestUsageDayQuery(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"day":"2026-11-06","rows":[{"user_id":"u1","agent":"a","model":"m","input":5,"output":6,"calls":1}],"totals":{"billable":11,"calls":1},"tenants":1}`)
	})
	u, err := c.Usage(context.Background(), "2026-11-06")
	if err != nil || rec.query != "day=2026-11-06" || u.Rows[0].Billable() != 11 || u.Totals.Billable != 11 {
		t.Fatalf("usage = %+v (%+v), %v", u, rec, err)
	}
}

func TestTenancyCalls(t *testing.T) {
	var rec recorder
	c := serve(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/admin/api/groups" && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"ok":true,"groups":[{"uuid":"g1","name":"team","master":"u1"}]}`)
		case r.URL.Path == "/admin/api/groups":
			fmt.Fprint(w, `{"ok":true,"group":{"uuid":"g2","name":"new","master":"u1"}}`)
		case r.URL.Path == "/admin/api/pools" && r.Method == http.MethodPost:
			fmt.Fprint(w, `{"ok":true,"pool":{"uuid":"p1","name":"notes","retention_days":30}}`)
		case strings.HasSuffix(r.URL.Path, "/grants") && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"ok":true,"grants":[{"pool":"p1","agent":"a","tenant":"*","mode":"rw"}]}`)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	})
	ctx := context.Background()
	gs, err := c.Groups(ctx)
	if err != nil || gs[0].Name != "team" {
		t.Fatalf("groups = %+v, %v", gs, err)
	}
	g, err := c.CreateGroup(ctx, "new", "u1")
	if err != nil || g.UUID != "g2" || rec.body != `{"creator":"u1","name":"new"}` {
		t.Fatalf("create group = %+v (%q), %v", g, rec.body, err)
	}
	if err := c.DisbandGroup(ctx, "g1", "u1"); err != nil || rec.method != http.MethodDelete || rec.body != `{"by":"u1"}` {
		t.Fatalf("disband = %+v, %v", rec, err)
	}
	if err := c.RemoveMember(ctx, "g1", "u2"); err != nil || rec.path != "/admin/api/groups/g1/members/u2" {
		t.Fatalf("remove member = %+v, %v", rec, err)
	}
	p, err := c.CreatePool(ctx, PoolDraft{Name: "notes", Creator: "u1", OwnerGroup: "g1", RetentionDays: 30})
	if err != nil || p.UUID != "p1" || strings.Contains(rec.body, "owner_tenant") {
		t.Fatalf("create pool = %+v (%q), %v", p, rec.body, err)
	}
	grants, err := c.Grants(ctx, "p1")
	if err != nil || grants[0].Tenant != "*" {
		t.Fatalf("grants = %+v, %v", grants, err)
	}
	if err := c.RemoveGrant(ctx, "p1", "a", ""); err != nil || rec.query != "agent=a&tenant=" {
		t.Fatalf("remove grant = %+v, %v", rec, err)
	}
}
