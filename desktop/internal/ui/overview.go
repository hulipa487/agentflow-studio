package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

// overviewCounters are the sampled counters the Overview charts, in order,
// each with the catalogue key of its label.
var overviewCounters = []struct{ name, labelKey string }{
	{"agentflow_llm_calls", "overview.counterLlmCalls"},
	{"agentflow_llm_tokens", "overview.counterLlmTokens"},
	{"agentflow_ingress_total", "overview.counterIngress"},
	{"agentflow_safety_drops", "overview.counterSafetyDrops"},
}

type overviewView struct {
	app *App
	s   *screen

	built     bool
	tiles     *fyne.Container
	sessions  *fyne.Container
	agents    *fyne.Container
	channels  *fyne.Container
	instances *fyne.Container
	counters  *fyne.Container
}

func (a *App) showOverview() {
	a.enter("overview")
	v := a.newOverviewView()
	a.setMain(v.s.root)
	ctx := a.viewContext()
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				onUI(func() {
					if ctx.Err() == nil {
						v.load(false)
					}
				})
			}
		}
	}()
}

func (a *App) newOverviewView() *overviewView {
	v := &overviewView{app: a}
	actions := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), func() { v.load(true) })
	v.s = a.newScreen(T("overview.title"), T("overview.subtitle"), actions)
	v.s.loading(T("overview.loading"))
	v.load(true)
	return v
}

type overviewData struct {
	state   api.State
	metrics api.Metrics
	merr    error
}

func (v *overviewView) load(show bool) {
	async(15*time.Second, func(ctx context.Context) (overviewData, error) {
		st, err := v.app.api.State(ctx)
		if err != nil {
			return overviewData{}, err
		}
		m, merr := v.app.api.Metrics(ctx)
		return overviewData{state: st, metrics: m, merr: merr}, nil
	}, func(d overviewData, err error) {
		if err != nil {
			if show || !v.built {
				v.built = false
				v.s.fail(err, func() { v.load(true) })
			} else {
				v.s.notice(T("overview.refreshFailed", "message", err.Error()), toneWarning)
			}
			return
		}
		v.s.banner.Clear()
		v.render(d)
	})
}

func (v *overviewView) render(d overviewData) {
	st := d.state
	parts := []string{}
	if st.Instance != "" {
		parts = append(parts, T("overview.metaInstance", "id", shortID(st.Instance)))
	}
	if st.Region != "" {
		parts = append(parts, T("overview.metaRegion", "region", st.Region))
	}
	if st.Zone != "" {
		parts = append(parts, T("overview.metaZone", "zone", st.Zone))
	}
	if st.Topology != "" {
		parts = append(parts, T("overview.metaTopology", "topology", st.Topology))
	}
	if n := len(st.Instances); n > 1 {
		parts = append(parts, T("overview.metaInstances", "n", n))
	}
	parts = append(parts, T("overview.metaVersion", "version", orDash(st.Version)), T("overview.metaUptime", "duration", humanDuration(st.UptimeS)))
	if st.ConfigEpoch != "" {
		parts = append(parts, T("overview.metaEpoch", "epoch", st.ConfigEpoch))
	}
	if st.ConfigPath != "" {
		parts = append(parts, st.ConfigPath)
	}
	v.s.meta.SetText(strings.Join(parts, " · "))

	if !v.built {
		v.tiles = grid(4)
		v.counters = grid(4)
		v.sessions = vstack(spaceS)
		v.agents = vstack(spaceS)
		v.channels = vstack(spaceS)
		v.instances = vstack(spaceS)
		left := section(T("overview.liveSessions"), T("overview.sessionsSubtitle"), v.sessions)
		right := vstack(spaceM,
			sectionAction(T("overview.agents"), T("overview.agentsSubtitle"),
				actionButton(T("common.open"), theme.NavigateNextIcon(), v.app.showAgents), v.agents),
			sectionAction(T("overview.channels"), T("overview.channelsSubtitle"),
				actionButton(T("common.open"), theme.NavigateNextIcon(), v.app.showChannels), v.channels),
		)
		// Instances has six fixed columns: on its own row it cannot push the
		// two-column grid wider than the page.
		v.s.setBody(
			v.tiles,
			v.counters,
			grid(2, left, right),
			section(T("overview.instances"), T("overview.instancesSubtitle"), v.instances),
		)
		v.built = true
	}

	// KPI tiles.
	clearBox(v.tiles)
	sessTone := toneNeutral
	if st.SessionsActive > 0 {
		sessTone = toneAccent
	}
	v.tiles.Add(statTile(T("overview.liveSessions"), fmt.Sprint(st.SessionsActive+st.SessionsIdle),
		T("overview.sessionsSub", "busy", st.SessionsActive, "idle", st.SessionsIdle), sessTone, nil))
	v.tiles.Add(statTile(T("overview.agents"), fmt.Sprint(len(st.Agents)), persistentSummary(st.Agents), toneNeutral, nil))
	v.tiles.Add(statTile(T("overview.channels"), fmt.Sprint(len(st.Channels)), channelSummary(st.Channels), toneNeutral, nil))
	credTone, credText := toneWarning, T("common.disabled")
	if st.CredentialsEnabled {
		credTone, credText = toneSuccess, T("common.enabled")
	}
	v.tiles.Add(statTile(T("overview.credentialStore"), credText, "runtime.credentials", credTone, nil))
	v.tiles.Refresh()

	// Counter sparklines.
	clearBox(v.counters)
	if d.merr == nil {
		for _, c := range overviewCounters {
			pts := d.metrics.Series[c.name]
			vals := make([]int64, len(pts))
			for i, p := range pts {
				vals[i] = p.Value
			}
			delta := ""
			if len(vals) > 1 {
				delta = T("overview.counterDelta", "delta", humanCount(vals[len(vals)-1]-vals[0]))
			}
			v.counters.Add(statTile(T(c.labelKey), humanCount(d.metrics.Latest[c.name]), delta, toneNeutral, newSparkline(vals, 28)))
		}
	} else {
		v.counters.Add(panel(caption(T("overview.countersUnavailable", "message", d.merr.Error()))))
	}
	v.counters.Refresh()

	// Sessions.
	clearBox(v.sessions)
	if len(st.Sessions) == 0 {
		v.sessions.Add(centerNote(T("overview.sessionsEmpty")))
	} else {
		rows := append([]api.StateSession(nil), st.Sessions...)
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Busy != rows[j].Busy {
				return rows[i].Busy
			}
			return rows[i].SessionID < rows[j].SessionID
		})
		var cells [][]fyne.CanvasObject
		for _, s := range rows {
			state := statusChip(T("overview.idle"), toneNeutral)
			if s.Busy {
				state = statusChip(T("overview.busy"), toneAccent)
			}
			cells = append(cells, []fyne.CanvasObject{
				vCenter(state), cellText(s.Agent), cellMono(s.SessionID), cellMono(orDash(shortID(s.ParentID))),
			})
		}
		v.sessions.Add(simpleTable([]tableColumn{
			{title: T("overview.colState"), width: 70}, {title: T("overview.colAgent"), width: 110}, {title: T("overview.colSession")}, {title: T("overview.colParent"), width: 90},
		}, cells))
	}
	v.sessions.Refresh()

	// Agents.
	clearBox(v.agents)
	if len(st.Agents) == 0 {
		v.agents.Add(centerNote(T("overview.agentsEmpty")))
	}
	agents := append([]api.StateAgent(nil), st.Agents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	for _, ag := range agents {
		name := ag.Name
		badges := hstack(spaceXS)
		if ag.Model != "" {
			badges.Add(vCenter(chip(ag.Model, toneAccent)))
		}
		if ag.Persistent {
			badges.Add(vCenter(chip(T("overview.daemon"), toneSuccess)))
		}
		row := container.NewBorder(nil, nil,
			hstack(spaceS, vCenter(widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), badges),
			hstack(spaceXS,
				iconButton(theme.MediaPlayIcon(), T("overview.chat"), func() { v.app.showPlayground(name) }),
				iconButton(theme.DocumentCreateIcon(), T("common.edit"), func() { v.app.showBuilder(name, nil) })),
			nil)
		v.agents.Add(row)
	}
	v.agents.Refresh()

	// Channels.
	clearBox(v.channels)
	if len(st.Channels) == 0 {
		v.channels.Add(centerNote(T("overview.channelsEmpty")))
	}
	for _, ch := range st.Channels {
		detail := "→ " + orDash(ch.Agent)
		if ch.Path != "" {
			detail += " · " + ch.Path
		}
		if ch.MediaEnabled {
			detail += " · " + T("overview.media")
		}
		v.channels.Add(container.NewBorder(nil, nil,
			hstack(spaceS, vCenter(chip(ch.Type, toneNeutral)), vCenter(widget.NewLabelWithStyle(ch.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))),
			nil, oneLine(detail)))
	}
	v.channels.Refresh()

	// Instances. The registry row for this process is marked, and a peer on a
	// different config epoch is the one thing here an operator must not miss:
	// the fleet is running two configurations.
	clearBox(v.instances)
	if len(st.Instances) == 0 {
		v.instances.Add(centerNote(T("overview.instancesEmpty")))
	} else {
		var cells [][]fyne.CanvasObject
		for _, in := range st.Instances {
			who := chip(T("overview.peer"), toneNeutral)
			if in.Self {
				who = chip(T("overview.thisInstance"), toneAccent)
			}
			last := T("overview.seenNow")
			if age := time.Now().Unix() - in.LastSeen; age > 5 {
				last = T("overview.seenAgo", "duration", humanDuration(age))
			}
			epoch := orDash(shortID(in.ConfigEpoch))
			if in.ConfigEpochDiffers {
				epoch = T("overview.epochDiffers", "epoch", epoch)
			}
			cells = append(cells, []fyne.CanvasObject{
				vCenter(who), cellMono(shortID(in.Owner)), cellText(orDash(in.Zone)),
				cellText(orDash(in.Version)), cellMono(epoch), cellText(last),
			})
		}
		v.instances.Add(simpleTable([]tableColumn{
			{title: T("overview.colState"), width: 110}, {title: T("overview.colOwner"), width: 110}, {title: T("overview.colZone"), width: 100},
			{title: T("overview.colVersion"), width: 80}, {title: T("overview.colEpoch"), width: 140}, {title: T("overview.colLastSeen")},
		}, cells))
	}
	v.instances.Refresh()
	v.s.refresh()
}

func persistentSummary(agents []api.StateAgent) string {
	n := 0
	for _, a := range agents {
		if a.Persistent {
			n++
		}
	}
	if n == 0 {
		return T("overview.noDaemons")
	}
	return T("overview.daemonCount", "n", n)
}

func channelSummary(chs []api.StateChannel) string {
	if len(chs) == 0 {
		return T("overview.noChannels")
	}
	count := map[string]int{}
	for _, c := range chs {
		count[c.Type]++
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, T("overview.channelCount", "n", count[k], "type", k))
	}
	return strings.Join(parts, " · ")
}
