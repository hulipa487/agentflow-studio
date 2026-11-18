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

// overviewCounters are the sampled counters the Overview charts, in order.
var overviewCounters = []struct{ name, label string }{
	{"agentflow_llm_calls", "LLM calls"},
	{"agentflow_llm_tokens", "LLM tokens"},
	{"agentflow_ingress_total", "Inbound messages"},
	{"agentflow_safety_drops", "Safety drops"},
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
	actions := iconButton(theme.ViewRefreshIcon(), "Refresh", func() { v.load(true) })
	v.s = a.newScreen("Overview", "What this runtime is doing right now. Refreshes every 5 seconds while open.", actions)
	v.s.loading("Contacting the runtime…")
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
				v.s.notice("refresh failed: "+err.Error(), toneWarning)
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
		parts = append(parts, "instance "+shortID(st.Instance))
	}
	if st.Region != "" {
		parts = append(parts, "region "+st.Region)
	}
	if st.Zone != "" {
		parts = append(parts, "zone "+st.Zone)
	}
	if st.Topology != "" {
		parts = append(parts, "topology "+st.Topology)
	}
	if n := len(st.Instances); n > 1 {
		parts = append(parts, fmt.Sprintf("%d instances", n))
	}
	parts = append(parts, "version "+orDash(st.Version), "up "+humanDuration(st.UptimeS))
	if st.ConfigEpoch != "" {
		parts = append(parts, "epoch "+st.ConfigEpoch)
	}
	if st.ConfigPath != "" {
		parts = append(parts, st.ConfigPath)
	}
	v.s.meta.SetText(strings.Join(parts, " · "))

	if !v.built {
		v.tiles = container.NewGridWithColumns(4)
		v.counters = container.NewGridWithColumns(4)
		v.sessions = vstack(spaceS)
		v.agents = vstack(spaceS)
		v.channels = vstack(spaceS)
		v.instances = vstack(spaceS)
		left := section("Live sessions", "One actor per (agent, session key). Busy means a turn is running.", v.sessions)
		right := vstack(spaceS,
			sectionAction("Agents", "Configured at boot; Studio-applied agents appear under Agents.",
				actionButton("Open", theme.NavigateNextIcon(), v.app.showAgents), v.agents),
			sectionAction("Channels", "Bound when the runtime started.",
				actionButton("Open", theme.NavigateNextIcon(), v.app.showChannels), v.channels),
		)
		// Instances has six fixed columns: on its own row it cannot push the
		// two-column grid wider than the page.
		v.s.setBody(
			v.tiles,
			v.counters,
			container.NewGridWithColumns(2, left, right),
			section("Instances", "Every process currently heartbeating on this runtime store. An epoch that differs means two instances run different configs.", v.instances),
		)
		v.built = true
	}

	// KPI tiles.
	clearBox(v.tiles)
	sessTone := toneNeutral
	if st.SessionsActive > 0 {
		sessTone = toneAccent
	}
	v.tiles.Add(statTile("Live sessions", fmt.Sprint(st.SessionsActive+st.SessionsIdle),
		fmt.Sprintf("%d busy · %d idle", st.SessionsActive, st.SessionsIdle), sessTone, nil))
	v.tiles.Add(statTile("Agents", fmt.Sprint(len(st.Agents)), persistentSummary(st.Agents), toneNeutral, nil))
	v.tiles.Add(statTile("Channels", fmt.Sprint(len(st.Channels)), channelSummary(st.Channels), toneNeutral, nil))
	credTone, credText := toneWarning, "disabled"
	if st.CredentialsEnabled {
		credTone, credText = toneSuccess, "enabled"
	}
	v.tiles.Add(statTile("Credential store", credText, "runtime.credentials", credTone, nil))
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
				delta = fmt.Sprintf("+%s in the sampled window", humanCount(vals[len(vals)-1]-vals[0]))
			}
			v.counters.Add(statTile(c.label, humanCount(d.metrics.Latest[c.name]), delta, toneNeutral, newSparkline(vals, 28)))
		}
	} else {
		v.counters.Add(panel(caption("Counters unavailable: " + d.merr.Error())))
	}
	v.counters.Refresh()

	// Sessions.
	clearBox(v.sessions)
	if len(st.Sessions) == 0 {
		v.sessions.Add(centerNote("No live sessions. A session starts on its agent's first message."))
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
			state := statusChip("idle", toneNeutral)
			if s.Busy {
				state = statusChip("busy", toneAccent)
			}
			cells = append(cells, []fyne.CanvasObject{
				vCenter(state), cellText(s.Agent), cellMono(s.SessionID), cellMono(orDash(shortID(s.ParentID))),
			})
		}
		v.sessions.Add(simpleTable([]tableColumn{
			{title: "state", width: 70}, {title: "agent", width: 110}, {title: "session"}, {title: "parent", width: 90},
		}, cells))
	}
	v.sessions.Refresh()

	// Agents.
	clearBox(v.agents)
	if len(st.Agents) == 0 {
		v.agents.Add(centerNote("No agents in the config. Create one from Agents."))
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
			badges.Add(vCenter(chip("daemon", toneSuccess)))
		}
		row := container.NewBorder(nil, nil,
			hstack(spaceS, vCenter(widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), badges),
			hstack(spaceXS,
				iconButton(theme.MediaPlayIcon(), "Chat", func() { v.app.showPlayground(name) }),
				iconButton(theme.DocumentCreateIcon(), "Edit", func() { v.app.showBuilder(name, nil) })),
			nil)
		v.agents.Add(row)
	}
	v.agents.Refresh()

	// Channels.
	clearBox(v.channels)
	if len(st.Channels) == 0 {
		v.channels.Add(centerNote("No channels. Add one from Channels; it binds at the next start."))
	}
	for _, ch := range st.Channels {
		detail := "→ " + orDash(ch.Agent)
		if ch.Path != "" {
			detail += " · " + ch.Path
		}
		if ch.MediaEnabled {
			detail += " · media"
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
		v.instances.Add(centerNote("No live instances reported. This runtime may predate the instance registry."))
	} else {
		var cells [][]fyne.CanvasObject
		for _, in := range st.Instances {
			who := chip("peer", toneNeutral)
			if in.Self {
				who = chip("this instance", toneAccent)
			}
			last := "now"
			if age := time.Now().Unix() - in.LastSeen; age > 5 {
				last = humanDuration(age) + " ago"
			}
			epoch := orDash(shortID(in.ConfigEpoch))
			if in.ConfigEpochDiffers {
				epoch += " (differs)"
			}
			cells = append(cells, []fyne.CanvasObject{
				vCenter(who), cellMono(shortID(in.Owner)), cellText(orDash(in.Zone)),
				cellText(orDash(in.Version)), cellMono(epoch), cellText(last),
			})
		}
		v.instances.Add(simpleTable([]tableColumn{
			{title: "state", width: 110}, {title: "owner", width: 110}, {title: "zone", width: 100},
			{title: "version", width: 80}, {title: "config epoch", width: 140}, {title: "last seen"},
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
		return "none run as daemons"
	}
	return fmt.Sprintf("%d daemon", n)
}

func channelSummary(chs []api.StateChannel) string {
	if len(chs) == 0 {
		return "no front door yet"
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
		parts = append(parts, fmt.Sprintf("%d %s", count[k], k))
	}
	return strings.Join(parts, " · ")
}
