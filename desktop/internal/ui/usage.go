package ui

import (
	"context"
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
	// render reads the grouping by index, in this order.
	v.groupBy = widget.NewSelect([]string{T("usage.groupTenant"), T("usage.groupAgent"), T("usage.groupModel"), T("usage.groupSlice")}, func(string) {
		if v.loaded {
			v.render()
		}
	})
	v.groupBy.SetSelectedIndex(usageByTenant)
	prev := iconButton(theme.NavigateBackIcon(), T("usage.prevDay"), func() { v.shift(-1) })
	next := iconButton(theme.NavigateNextIcon(), T("usage.nextDay"), func() { v.shift(1) })
	today := actionButton(T("usage.today"), nil, func() {
		v.day = time.Now().UTC()
		v.load()
	})
	actions := hstack(spaceS, prev, widthBox(110, v.dayLbl), next, today, widthBox(150, v.groupBy),
		iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load))
	v.s = a.newScreen(T("usage.title"), T("usage.subtitle"), actions)
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
	v.s.loading(T("usage.loading", "day", day))
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

// The groupings, in the order the select offers them.
const (
	usageByTenant = iota
	usageByAgent
	usageByModel
	usageEverySlice
)

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
	v.s.meta.SetText(T("usage.meta", "day", d.Day, "slices", len(d.Rows), "tenants", d.Tenants))
	tiles := grid(4,
		statTile(T("usage.billable"), humanCount(d.Totals.Billable), T("usage.inOut", "input", humanCount(d.Totals.Input), "output", humanCount(d.Totals.Output)), toneAccent, nil),
		statTile(T("usage.calls"), humanCount(d.Totals.Calls), T("usage.failedCount", "n", failed), toneNeutral, nil),
		statTile(T("usage.cachedInput"), humanCount(cached), T("usage.inInput"), toneNeutral, nil),
		statTile(T("usage.reasoning"), humanCount(reasoning), T("usage.inOutput"), toneNeutral, nil),
	)
	if len(d.Rows) == 0 {
		v.s.setBody(tiles, vSpacer(spaceL), emptyStateCard(T("usage.emptyTitle"), T("usage.emptyGuidance"), "", nil))
		return
	}

	var cols []tableColumn
	var rows [][]fyne.CanvasObject
	numCols := []tableColumn{
		{title: T("usage.col.input"), width: 80, right: true}, {title: T("usage.col.output"), width: 80, right: true},
		{title: T("usage.col.billable"), width: 84, right: true}, {title: T("usage.col.calls"), width: 60, right: true}, {title: T("usage.col.failed"), width: 60, right: true},
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
			return T("usage.noTenant")
		}
		return id
	}

	if v.groupBy.SelectedIndex() == usageEverySlice {
		cols = append([]tableColumn{{title: T("usage.col.tenant")}, {title: T("usage.col.agent"), width: 120}, {title: T("usage.col.model"), width: 150}}, numCols...)
		sorted := append([]api.UsageRow(nil), d.Rows...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Billable() > sorted[j].Billable() })
		for _, r := range sorted {
			rows = append(rows, append([]fyne.CanvasObject{cellMono(tenantLabel(r.UserID)), cellText(r.Agent), cellMono(r.Model)}, nums(r.UsageTotals)...))
		}
	} else {
		keyOf := func(r api.UsageRow) string { return tenantLabel(r.UserID) }
		title := T("usage.col.tenant")
		switch v.groupBy.SelectedIndex() {
		case usageByAgent:
			keyOf, title = func(r api.UsageRow) string { return r.Agent }, T("usage.col.agent")
		case usageByModel:
			keyOf, title = func(r api.UsageRow) string { return r.Model }, T("usage.col.model")
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
		cols = append([]tableColumn{{title: title}, {title: T("usage.col.share"), width: 140}}, numCols...)
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
		caption(T("usage.footnote")))
}
