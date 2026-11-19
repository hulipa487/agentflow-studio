package ui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

const usageDayLayout = "2006-01-02"

type usageView struct {
	app *App
	s   *screen

	day     time.Time
	dayLbl  *widget.Label
	groupBy *widget.Select
	data    api.UsageDay
	loaded  bool
	loadSeq int
}

func (a *App) showUsage() {
	a.enter("usage")
	v := a.newUsageView()
	a.setMain(v.s.root)
	a.refreshAction = v.load
}

func (a *App) newUsageView() *usageView {
	v := &usageView{app: a, day: time.Now().UTC()}
	v.dayLbl = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Monospace: true, Bold: true})
	v.groupBy = widget.NewSelect([]string{"By tenant", "By agent", "By model", "Every slice"}, func(string) {
		if v.loaded {
			v.render()
		}
	})
	v.groupBy.SetSelected("By tenant")
	prev := iconButton(theme.NavigateBackIcon(), "Previous day", func() { v.shift(-1) })
	next := iconButton(theme.NavigateNextIcon(), "Next day", func() { v.shift(1) })
	today := actionButton("Today", nil, func() {
		v.day = time.Now().UTC()
		v.load()
	})
	actions := hstack(spaceS, prev, widthBox(110, v.dayLbl), next, today, widthBox(150, v.groupBy),
		iconButton(theme.ViewRefreshIcon(), "Refresh", v.load))
	v.s = a.newScreen("Usage",
		"The token ledger's daily rollup (usage_daily): what every tenant, agent and model spent on a UTC day. It is the figure quotas and agent budgets are enforced against.",
		actions)
	v.load()
	return v
}

func (v *usageView) shift(days int) {
	next := v.day.AddDate(0, 0, days)
	if next.After(time.Now().UTC()) {
		return
	}
	v.day = next
	v.load()
}

func (v *usageView) load() {
	day := v.day.Format(usageDayLayout)
	v.dayLbl.SetText(day)
	v.loadSeq++
	seq := v.loadSeq
	v.s.loading("Reading the ledger for " + day + "…")
	async(20*time.Second, func(ctx context.Context) (api.UsageDay, error) {
		return v.app.api.Usage(ctx, day)
	}, func(d api.UsageDay, err error) {
		if seq != v.loadSeq {
			return
		}
		if err != nil {
			v.s.fail(err, v.load)
			return
		}
		v.s.banner.Clear()
		v.data = d
		v.loaded = true
		v.render()
	})
}

type usageGroup struct {
	key string
	api.UsageTotals
	slices int
}

func (v *usageView) render() {
	d := v.data
	var cached, reasoning, failed int64
	for _, r := range d.Rows {
		cached += r.Cached
		reasoning += r.Reasoning
		failed += r.Failed
	}
	v.s.meta.SetText(fmt.Sprintf("%s (UTC) · %d slices · %d tenants", d.Day, len(d.Rows), d.Tenants))
	tiles := grid(4,
		statTile("Billable", humanCount(d.Totals.Billable), fmt.Sprintf("in %s · out %s", humanCount(d.Totals.Input), humanCount(d.Totals.Output)), toneAccent, nil),
		statTile("Calls", humanCount(d.Totals.Calls), fmt.Sprintf("%d failed", failed), toneNeutral, nil),
		statTile("Cached input", humanCount(cached), "already counted in input", toneNeutral, nil),
		statTile("Reasoning", humanCount(reasoning), "already counted in output", toneNeutral, nil),
	)
	if len(d.Rows) == 0 {
		v.s.setBody(tiles, vSpacer(spaceL), emptyStateCard("Nothing metered on this day",
			"Every LLM call through a metered agent is recorded here. Pick another day, or chat with an agent and refresh.", "", nil))
		return
	}

	var cols []tableColumn
	var rows [][]fyne.CanvasObject
	numCols := []tableColumn{
		{title: "input", width: 80, right: true}, {title: "output", width: 80, right: true},
		{title: "billable", width: 84, right: true}, {title: "calls", width: 60, right: true}, {title: "failed", width: 60, right: true},
	}
	nums := func(t api.UsageTotals) []fyne.CanvasObject {
		failed := cellNum(t.Failed)
		if t.Failed > 0 {
			failed.Importance = widget.DangerImportance
		}
		return []fyne.CanvasObject{cellNum(t.Input), cellNum(t.Output), cellNum(t.Billable()), cellNum(t.Calls), failed}
	}
	tenantLabel := func(id string) string {
		if id == "" {
			return "(no tenant)"
		}
		return id
	}

	if v.groupBy.Selected == "Every slice" {
		cols = append([]tableColumn{{title: "tenant"}, {title: "agent", width: 120}, {title: "model", width: 150}}, numCols...)
		sorted := append([]api.UsageRow(nil), d.Rows...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Billable() > sorted[j].Billable() })
		for _, r := range sorted {
			rows = append(rows, append([]fyne.CanvasObject{cellMono(tenantLabel(r.UserID)), cellText(r.Agent), cellMono(r.Model)}, nums(r.UsageTotals)...))
		}
	} else {
		keyOf := func(r api.UsageRow) string { return tenantLabel(r.UserID) }
		title := "tenant"
		switch v.groupBy.Selected {
		case "By agent":
			keyOf, title = func(r api.UsageRow) string { return r.Agent }, "agent"
		case "By model":
			keyOf, title = func(r api.UsageRow) string { return r.Model }, "model"
		}
		groups := map[string]*usageGroup{}
		for _, r := range d.Rows {
			k := keyOf(r)
			g, ok := groups[k]
			if !ok {
				g = &usageGroup{key: k}
				groups[k] = g
			}
			g.Input += r.Input
			g.Output += r.Output
			g.Calls += r.Calls
			g.Failed += r.Failed
			g.slices++
		}
		list := make([]*usageGroup, 0, len(groups))
		for _, g := range groups {
			list = append(list, g)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Billable() > list[j].Billable() })
		cols = append([]tableColumn{{title: title}, {title: "share", width: 140}}, numCols...)
		total := d.Totals.Billable
		for _, g := range list {
			share := widget.NewProgressBar()
			share.Max = 1
			if total > 0 {
				share.SetValue(float64(g.Billable()) / float64(total))
			}
			rows = append(rows, append([]fyne.CanvasObject{cellMono(g.key), share}, nums(g.UsageTotals)...))
		}
	}
	v.s.setBody(tiles, panel(simpleTable(cols, rows)),
		caption("Billable = input + output, the rule quotas and budgets use. Per-call detail lives on each profile under Users when usage.events is on."))
}
