package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// A deployment with no agents shows the normal channel empty state; opening
// the editor must still explain that an agent is required first.
func TestChannelsWithoutAgentsGuideThroughChannelCreation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/studio/channels":
			_ = json.NewEncoder(w).Encode(api.ChannelList{
				OK: true, Mode: "file", Path: "agentflow.yaml", Editable: true,
			})
		case "/admin/api/studio/agents":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "agents": []api.AgentRow{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("channels")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	a.api = api.New(srv.URL, "token")
	a.content = container.NewStack()

	v := a.newChannelsView()
	// The empty state is rendered as spacer / content / spacer so it can centre
	// in the viewport. Wait for the content itself, not just the first object,
	// or the assertion can run between the two Adds.
	guidance := "Create a channel to route messages into an agent."
	ui.waitFor(3*time.Second, func() bool {
		labels, _ := collectLabelsAndButtons(v.list)
		return v.data.Mode != "" && containsString(labels, guidance)
	})
	if v.data.Mode == "" {
		t.Fatal("channels view did not load")
	}
	if len(v.agents) != 0 {
		t.Fatalf("agents = %v, want none", v.agents)
	}

	labels, buttons := collectLabelsAndButtons(v.list)
	if !containsString(labels, guidance) {
		t.Fatalf("empty-state guidance missing; labels=%v", labels)
	}
	if !containsString(buttons, "Create a channel") {
		t.Fatalf("empty-state action missing; buttons=%v", buttons)
	}

	a.editChannel(v, nil)
	if win.Canvas().Overlays().Top() == nil {
		t.Fatal("clicking New channel with no agents showed no dialog")
	}
}

func collectLabelsAndButtons(obj fyne.CanvasObject) (labels, buttons []string) {
	if obj == nil {
		return labels, buttons
	}
	switch o := obj.(type) {
	case *widget.Label:
		labels = append(labels, o.Text)
	case *widget.Button:
		buttons = append(buttons, o.Text)
	case *fyne.Container:
		for _, child := range o.Objects {
			l, b := collectLabelsAndButtons(child)
			labels = append(labels, l...)
			buttons = append(buttons, b...)
		}
	}
	return labels, buttons
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

// A qq channel's card shows its delivery mode, app id and scenes, with the
// runtime's defaults filled in when the file leaves them out, and the secret
// as stored-or-not, never its value.
func TestQQChannelCardShowsModeAppAndScenes(t *testing.T) {
	fyneApp := test.NewApp()
	claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("channels")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	a.content = container.NewStack()
	v := &channelsView{app: a}

	card := v.channelCard(api.Channel{Name: "qq", Type: "qq", Agent: "assistant", AppID: "1024", HasSecret: true, SecretRef: "${QQ_BOT_SECRET}"})
	labels, _ := collectLabelsAndButtons(card)
	joined := strings.Join(labels, " | ")
	for _, want := range []string{"websocket", "1024", "c2c, group, guild"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("card lacks %q; labels=%v", want, labels)
		}
	}

	card = v.channelCard(api.Channel{Name: "qq", Type: "qq", Agent: "assistant", Mode: "webhook", AppID: "1024", Scenes: []string{"dm"}, Markdown: true})
	labels, _ = collectLabelsAndButtons(card)
	joined = strings.Join(labels, " | ")
	if !strings.Contains(joined, "webhook") || !strings.Contains(joined, "dm") || strings.Contains(joined, "c2c") || !strings.Contains(joined, "markdown") {
		t.Fatalf("stored mode and scenes not shown as stored; labels=%v", labels)
	}
}
