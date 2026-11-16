package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentsAndAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/admin/api/studio/agents" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"ok":true,"agents":[{"name":"echo","model":"default","persisted":true,"capabilities":["tools"]}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "tok")
	agents, err := c.Agents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if len(agents) != 1 || agents[0].Name != "echo" || !agents[0].Persisted {
		t.Fatalf("agents = %+v", agents)
	}
}

func TestChatStreamEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/admin/api/studio/chat" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "event: trace\ndata: {\"type\":\"trace\",\"trace\":{\"seq\":1,\"kind\":\"turn.start\"}}\n\n")
		fl.Flush()
		fmt.Fprint(w, "event: reply\ndata: {\"type\":\"reply\",\"reply\":{\"text\":\"hi\",\"status\":\"delivered\"}}\n\n")
		fl.Flush()
		fmt.Fprint(w, ": keepalive\n\n")
		fl.Flush()
		fmt.Fprint(w, "event: done\ndata: {\"type\":\"done\",\"done\":{\"thread\":\"t1\",\"events\":2}}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var kinds []string
	var reply string
	var timedOut bool
	err := c.Chat(context.Background(), ChatRequest{Agent: "echo", Thread: "t1", Text: "hi"}, func(ev ChatEvent) {
		kinds = append(kinds, ev.Type)
		if ev.Reply != nil {
			reply = ev.Reply.Text
		}
		if ev.Done != nil {
			timedOut = ev.Done.TimedOut
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(kinds, ",") != "trace,reply,done" {
		t.Fatalf("event order = %v", kinds)
	}
	if reply != "hi" || timedOut {
		t.Fatalf("reply=%q timedOut=%v", reply, timedOut)
	}
}

func TestUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"unauthorized"}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "wrong").Agents(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	ae, ok := err.(*Error)
	if !ok || ae.Status != http.StatusUnauthorized || !strings.Contains(ae.Message, "token") {
		t.Fatalf("error = %#v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"ok":false,"errors":["loop source is empty","missing directive"]}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "tok").Validate(context.Background(), ValidateRequest{Name: "x"})
	ae, ok := err.(*Error)
	if !ok || ae.Status != http.StatusBadRequest {
		t.Fatalf("error = %#v", err)
	}
	if !strings.Contains(ae.Message, "loop source is empty") || !strings.Contains(ae.Message, "missing directive") {
		t.Fatalf("message = %q", ae.Message)
	}
}

func TestUnreachable(t *testing.T) {
	err := New("http://127.0.0.1:1", "tok").Ping(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestSafetyAndCredentials(t *testing.T) {
	var calls []string
	var profileDraft SafetyProfileDraft
	var classifierDraft SafetyClassifierDraft
	var validateReq SafetyValidateRequest
	var credentialDraft CredentialDraft
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/api/studio/safety":
			fmt.Fprint(w, `{"ok":true,"restart_required":true,"editable":true,"builtin_filters":["source-attribution","affect-guard"],"profiles":[{"name":"strict","filters":["affect-guard"],"agents":["main"],"live":true}],"classifiers":[{"name":"mod","model":"default","categories":{"safe":"allow"},"used_by":["strict"],"live":false}],"models":["default"],"prompts":["safety/moderation"]}`)
		case "POST /admin/api/studio/safety/validate":
			if err := json.NewDecoder(r.Body).Decode(&validateReq); err != nil {
				t.Errorf("validate decode: %v", err)
			}
			fmt.Fprint(w, `{"ok":false,"errors":["unknown filter"]}`)
		case "POST /admin/api/studio/safety/profiles", "PUT /admin/api/studio/safety/profiles/strict":
			var body struct {
				Profile SafetyProfileDraft `json:"profile"`
				MTime   int64              `json:"mtime"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("profile decode: %v", err)
			}
			profileDraft = body.Profile
			fmt.Fprint(w, `{"ok":true,"restart_required":true,"saved_note":"saved","profiles":[]}`)
		case "DELETE /admin/api/studio/safety/profiles/strict":
			fmt.Fprint(w, `{"ok":true,"profiles":[]}`)
		case "POST /admin/api/studio/safety/classifiers", "PUT /admin/api/studio/safety/classifiers/mod":
			var body struct {
				Classifier SafetyClassifierDraft `json:"classifier"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("classifier decode: %v", err)
			}
			classifierDraft = body.Classifier
			fmt.Fprint(w, `{"ok":true,"classifiers":[]}`)
		case "DELETE /admin/api/studio/safety/classifiers/mod":
			fmt.Fprint(w, `{"ok":true,"classifiers":[]}`)
		case "GET /admin/api/credentials":
			if got := r.URL.Query().Get("user"); got != "u 1" {
				t.Errorf("user query = %q", got)
			}
			fmt.Fprint(w, `{"ok":true,"enabled":true,"bucket":"u 1","users":["","u 1"],"credentials":[{"service":"github","kind":"api_key","header":"Authorization","scheme":"Bearer","fingerprint":"…abcd"}]}`)
		case "POST /admin/api/credentials":
			if err := json.NewDecoder(r.Body).Decode(&credentialDraft); err != nil {
				t.Errorf("credential decode: %v", err)
			}
			fmt.Fprint(w, `{"ok":true,"enabled":true,"bucket":"u 1","credentials":[]}`)
		case "DELETE /admin/api/credentials":
			if got := r.URL.Query().Get("service"); got != "github" {
				t.Errorf("service query = %q", got)
			}
			if got := r.URL.Query().Get("user"); got != "u 1" {
				t.Errorf("delete user query = %q", got)
			}
			fmt.Fprint(w, `{"ok":true,"enabled":true,"credentials":[]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	ctx := context.Background()
	list, err := c.Safety(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !list.RestartRequired || len(list.Profiles) != 1 || list.Profiles[0].Name != "strict" || list.Profiles[0].Agents[0] != "main" {
		t.Fatalf("safety list = %+v", list)
	}
	if len(list.Classifiers) != 1 || list.Classifiers[0].UsedBy[0] != "strict" || list.Classifiers[0].Categories["safe"] != "allow" {
		t.Fatalf("classifiers = %+v", list.Classifiers)
	}

	d := SafetyProfileDraft{Name: "strict", Filters: []string{"affect-guard"}}
	errs, err := c.ValidateSafety(ctx, SafetyValidateRequest{Kind: "profile", Creating: true, Profile: &d})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0] != "unknown filter" || validateReq.Kind != "profile" || !validateReq.Creating {
		t.Fatalf("validate errs=%v req=%+v", errs, validateReq)
	}
	if _, err := c.SaveSafetyProfile(ctx, "strict", false, d, 42); err != nil {
		t.Fatal(err)
	}
	if profileDraft.Name != "strict" || len(profileDraft.Filters) != 1 {
		t.Fatalf("profile draft = %+v", profileDraft)
	}
	if _, err := c.SaveSafetyProfile(ctx, "strict", true, d, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteSafetyProfile(ctx, "strict", 42); err != nil {
		t.Fatal(err)
	}

	threshold := 0.8
	cd := SafetyClassifierDraft{Name: "mod", Model: "default", Threshold: &threshold, Categories: map[string]string{"safe": "allow"}}
	if _, err := c.SaveSafetyClassifier(ctx, "mod", true, cd, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveSafetyClassifier(ctx, "mod", false, cd, 0); err != nil {
		t.Fatal(err)
	}
	if classifierDraft.Name != "mod" || classifierDraft.Model != "default" || classifierDraft.Threshold == nil || *classifierDraft.Threshold != 0.8 {
		t.Fatalf("classifier draft = %+v", classifierDraft)
	}
	if _, err := c.DeleteSafetyClassifier(ctx, "mod", 0); err != nil {
		t.Fatal(err)
	}

	creds, err := c.Credentials(ctx, "u 1")
	if err != nil {
		t.Fatal(err)
	}
	if !creds.Enabled || creds.Bucket != "u 1" || len(creds.Credentials) != 1 || creds.Credentials[0].Fingerprint != "…abcd" {
		t.Fatalf("credentials = %+v", creds)
	}
	if _, err := c.PutCredential(ctx, CredentialDraft{User: "u 1", Service: "github", Kind: "api_key", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if credentialDraft.Service != "github" || credentialDraft.Secret != "s" {
		t.Fatalf("credential draft = %+v", credentialDraft)
	}
	if _, err := c.DeleteCredential(ctx, "u 1", "github"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GET /admin/api/studio/safety",
		"POST /admin/api/studio/safety/validate",
		"PUT /admin/api/studio/safety/profiles/strict",
		"POST /admin/api/studio/safety/profiles",
		"DELETE /admin/api/studio/safety/profiles/strict?mtime=42",
		"POST /admin/api/studio/safety/classifiers",
		"PUT /admin/api/studio/safety/classifiers/mod",
		"DELETE /admin/api/studio/safety/classifiers/mod?mtime=0",
		"GET /admin/api/credentials?user=u+1",
		"POST /admin/api/credentials",
		"DELETE /admin/api/credentials?service=github&user=u+1",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestModelsCRUDAndPersistence(t *testing.T) {
	var calls []string
	var draft ModelDraft
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/api/models":
			fmt.Fprint(w, `{"ok":true,"models":[{"name":"default","provider":"openai","model":"gpt-4o-mini","has_key":true,"key_fingerprint":"…abcd","in_runtime":true,"in_file":true},{"name":"fast","provider":"anthropic","model":"claude-3-5-haiku","in_runtime":true,"drift":true}],"file_error":""}`)
		case "PUT /admin/api/models/fast":
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Errorf("draft decode: %v", err)
			}
			fmt.Fprint(w, `{"ok":true,"applied":"runtime"}`)
		case "POST /admin/api/models/fast/test":
			fmt.Fprint(w, `{"ok":false,"latency_ms":12,"class":"auth","error":"401 unauthorized"}`)
		case "DELETE /admin/api/models/fast", "POST /admin/api/models/persist", "POST /admin/api/models/revert":
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	models, fileErr, err := c.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fileErr != "" || len(models) != 2 || models[0].KeyFingerprint != "…abcd" || !models[1].Drift {
		t.Fatalf("models=%+v fileErr=%q", models, fileErr)
	}
	d := ModelDraft{Provider: "anthropic", Model: "claude-3-5-haiku", Thinking: "high", ServerTools: []string{"web_search"}}
	if err := c.PutModel(context.Background(), "fast", d); err != nil {
		t.Fatal(err)
	}
	if draft.Provider != "anthropic" || draft.Model != "claude-3-5-haiku" || draft.Thinking != "high" || len(draft.ServerTools) != 1 {
		t.Fatalf("draft = %+v", draft)
	}
	res, err := c.TestModel(context.Background(), "fast")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Class != "auth" || res.LatencyMS != 12 {
		t.Fatalf("test result = %+v", res)
	}
	if err := c.DeleteModel(context.Background(), "fast"); err != nil {
		t.Fatal(err)
	}
	if err := c.PersistModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.RevertModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /admin/api/models",
		"PUT /admin/api/models/fast",
		"POST /admin/api/models/fast/test",
		"DELETE /admin/api/models/fast",
		"POST /admin/api/models/persist",
		"POST /admin/api/models/revert",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v", calls)
	}
}
