package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

// tenancyView edits the runtime's tenancy registry: groups (a master and its
// members) and named memory pools with per-agent grants. All of it is live:
// the registry lives in the runtime store, not in the config file.
type tenancyView struct {
	app *App
	s   *screen

	groupList   *fyne.Container
	groupDetail *fyne.Container
	poolList    *fyne.Container
	poolDetail  *fyne.Container

	groups    []api.Group
	pools     []api.Pool
	selGroup  string
	selPool   string
	userIDs   []string
	userNames map[string]string
	agents    []string
}

func (a *App) showTenancy() {
	a.enter("tenancy")
	v := a.newTenancyView()
	a.setMain(v.s.root)
	a.refreshAction = v.load
}

func (a *App) newTenancyView() *tenancyView {
	v := &tenancyView{app: a, userNames: map[string]string{}}
	v.groupList, v.groupDetail = vstack(spaceXS), vstack(spaceS)
	v.poolList, v.poolDetail = vstack(spaceXS), vstack(spaceS)

	listPane := func(add fyne.CanvasObject, list fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(vstack(spaceS, add, vSpacer(0)), nil, nil, nil, paneScroll(list))
	}
	detailPane := func(detail fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), paneScroll(detail))
	}
	groupSplit := newHSplitPane(
		listPane(actionButton("New group", theme.ContentAddIcon(), v.newGroup), v.groupList),
		detailPane(v.groupDetail), 0.32)
	poolSplit := newHSplitPane(
		listPane(actionButton("New pool", theme.ContentAddIcon(), v.newPool), v.poolList),
		detailPane(v.poolDetail), 0.32)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Groups", iconUser, groupSplit),
		container.NewTabItemWithIcon("Memory pools", theme.StorageIcon(), poolSplit),
	)
	v.s = a.newPane("Tenancy",
		"Groups give several people one shared context; pools are named memory stores that agents bind with pool: <name>, opened to tenants by grants. Changes are live.",
		iconButton(theme.ViewRefreshIcon(), "Refresh", v.load), tabs)
	v.load()
	return v
}

type tenancyData struct {
	groups []api.Group
	pools  []api.Pool
	users  []api.UserRow
	agents []api.AgentRow
}

func (v *tenancyView) load() {
	for _, b := range []*fyne.Container{v.groupList, v.poolList} {
		clearBox(b)
		b.Add(container.NewPadded(muted("Loading…")))
		b.Refresh()
	}
	async(20*time.Second, func(ctx context.Context) (tenancyData, error) {
		var d tenancyData
		var err error
		if d.groups, err = v.app.api.Groups(ctx); err != nil {
			return d, err
		}
		if d.pools, err = v.app.api.Pools(ctx); err != nil {
			return d, err
		}
		// Pickers only: a runtime without identity still has tenancy.
		if l, err := v.app.api.Users(ctx); err == nil {
			d.users = l.Users
		}
		d.agents, _ = v.app.api.Agents(ctx)
		return d, nil
	}, func(d tenancyData, err error) {
		if err != nil {
			for _, b := range []*fyne.Container{v.groupList, v.poolList, v.groupDetail, v.poolDetail} {
				clearBox(b)
				b.Refresh()
			}
			if msg, off := featureOff(err); off {
				v.groupDetail.Add(emptyStateCard("Tenancy is unavailable", msg, "", nil))
				v.poolDetail.Add(emptyStateCard("Tenancy is unavailable", msg, "", nil))
			} else {
				v.s.notice(err.Error(), toneDanger)
			}
			v.s.refresh()
			return
		}
		v.s.banner.Clear()
		v.groups, v.pools = d.groups, d.pools
		sort.Slice(v.groups, func(i, j int) bool { return v.groups[i].Name < v.groups[j].Name })
		sort.Slice(v.pools, func(i, j int) bool { return v.pools[i].Name < v.pools[j].Name })
		v.userIDs = v.userIDs[:0]
		for _, u := range d.users {
			v.userIDs = append(v.userIDs, u.UserID)
			v.userNames[u.UserID] = userTitle(u)
		}
		sort.Strings(v.userIDs)
		v.agents = v.agents[:0]
		for _, a := range d.agents {
			if !a.Spawn {
				v.agents = append(v.agents, a.Name)
			}
		}
		sort.Strings(v.agents)
		active := 0
		for _, g := range v.groups {
			if g.DisbandedAt == 0 {
				active++
			}
		}
		v.s.meta.SetText(fmt.Sprintf("%s (%d active) · %s", plural(len(v.groups), "group"), active, plural(len(v.pools), "pool")))
		v.renderGroups()
		v.renderPools()
		if v.selGroup != "" {
			v.openGroup(v.selGroup)
		} else {
			v.placeholder(v.groupDetail, "Select a group to manage its members.")
		}
		if v.selPool != "" {
			v.openPool(v.selPool)
		} else {
			v.placeholder(v.poolDetail, "Select a pool to manage its grants.")
		}
	})
}

func (v *tenancyView) placeholder(box *fyne.Container, text string) {
	clearBox(box)
	box.Add(vSpacer(spaceXL))
	box.Add(centerNote(text))
	box.Refresh()
}

// who renders a tenant uuid with its display name when known.
func (v *tenancyView) who(id string) string {
	if id == "" {
		return "—"
	}
	if n, ok := v.userNames[id]; ok && n != id {
		return n + " · " + shortID(id)
	}
	return id
}

func (v *tenancyView) userPicker(placeholder string) *widget.SelectEntry {
	e := widget.NewSelectEntry(v.userIDs)
	e.SetPlaceHolder(placeholder)
	return e
}

// ---- groups -------------------------------------------------------------------

func (v *tenancyView) renderGroups() {
	clearBox(v.groupList)
	if len(v.groups) == 0 {
		v.groupList.Add(container.NewPadded(muted("No groups. A group has one master and any number of members; a chat enters it with /group <uuid>.")))
	}
	for _, g := range v.groups {
		g := g
		sub := shortID(g.UUID)
		if g.DisbandedAt > 0 {
			sub += " · disbanded " + relTime(g.DisbandedAt)
		}
		row := listRow(g.Name, sub, iconUser, func() { v.openGroup(g.UUID) })
		row.SetActive(g.UUID == v.selGroup)
		v.groupList.Add(row)
	}
	v.groupList.Refresh()
}

func (v *tenancyView) openGroup(id string) {
	v.selGroup = id
	v.renderGroups()
	clearBox(v.groupDetail)
	v.groupDetail.Add(container.NewPadded(muted("Loading group…")))
	v.groupDetail.Refresh()
	async(20*time.Second, func(ctx context.Context) (api.GroupDetail, error) {
		return v.app.api.Group(ctx, id)
	}, func(d api.GroupDetail, err error) {
		if v.selGroup != id {
			return
		}
		clearBox(v.groupDetail)
		if err != nil {
			v.groupDetail.Add(messageCard(err.Error(), toneDanger))
			v.groupDetail.Refresh()
			return
		}
		v.renderGroup(d)
	})
}

func (v *tenancyView) renderGroup(d api.GroupDetail) {
	g := d.Group
	disbanded := g.DisbandedAt > 0
	state := chip("active", toneSuccess)
	if disbanded {
		state = chip("disbanded", toneDanger)
	}
	head := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(widget.NewLabelWithStyle(g.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), vCenter(state)),
		nil, nil)
	info := grid(3,
		kvLine("group uuid", g.UUID),
		kvLine("master", v.who(g.Master)),
		kvLine("created", relTime(g.CreatedAt)),
	)
	disband := actionButton("Disband…", theme.DeleteIcon(), func() { v.disband(g) })
	disband.Importance = widget.DangerImportance
	if disbanded {
		disband.Disable()
	}
	copyID := v.app.copyButton(g.UUID, "group uuid")
	v.groupDetail.Add(panel(vstack(spaceS, container.NewBorder(nil, nil, nil, hstack(spaceXS, copyID, disband), head), info)))

	var rows [][]fyne.CanvasObject
	members := append([]api.Membership(nil), d.Members...)
	sort.Slice(members, func(i, j int) bool { return members[i].JoinedAt < members[j].JoinedAt })
	for _, m := range members {
		m := m
		role := chip("member", toneNeutral)
		isMaster := m.User == g.Master
		if isMaster {
			role = chip("master", toneAccent)
		}
		makeMaster := actionButton("Make master", nil, func() { v.setMaster(g, m.User) })
		remove := iconButton(theme.ContentClearIcon(), "Remove", func() { v.removeMember(g, m.User) })
		if isMaster || disbanded {
			makeMaster.Disable()
			remove.Disable()
		}
		rows = append(rows, []fyne.CanvasObject{
			vCenter(role), cellText(v.who(m.User)), cellText(relTime(m.JoinedAt)), makeMaster, remove,
		})
	}
	var table fyne.CanvasObject = centerNote("No members.")
	if len(rows) > 0 {
		table = simpleTable([]tableColumn{
			{title: "role", width: 74}, {title: "tenant"}, {title: "joined", width: 90}, {title: "", width: 116}, {title: "", width: 36},
		}, rows)
	}
	add := v.userPicker("tenant uuid to add")
	addBtn := actionButton("Add member", theme.ContentAddIcon(), func() {
		user := strings.TrimSpace(add.Text)
		if user == "" {
			v.s.notice("Enter the tenant uuid to add.", toneWarning)
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.AddMember(ctx, g.UUID, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("member added", toneSuccess)
			v.openGroup(g.UUID)
		})
	})
	addRow := container.NewBorder(nil, nil, nil, addBtn, controlFrame(add))
	if disbanded {
		addRow.Hide()
	}
	v.groupDetail.Add(section("Members", "A turn acts in the group only when the sender is an active member; the engine validates it, the route only proposes.",
		vstack(spaceS, table, addRow)))
	v.groupDetail.Refresh()
	v.s.refresh()
}

func (v *tenancyView) newGroup() {
	name := widget.NewEntry()
	name.SetPlaceHolder("design-team")
	creator := v.userPicker("creator's tenant uuid (becomes master)")
	errBanner := newBanner()
	form := vstack(spaceM,
		vstack(spaceXS, fieldLabel("Name"), controlFrame(name)),
		vstack(spaceXS, fieldLabel("Creator"), controlFrame(creator),
			caption("The creator's personal uuid. They become the master and the first member.")),
		errBanner)
	v.modal("New group", "Groups are recorded in the runtime store and are live immediately.", form, "Create", func(done func()) {
		n, c := strings.TrimSpace(name.Text), strings.TrimSpace(creator.Text)
		if n == "" || c == "" {
			errBanner.Set("A name and a creator are required.", toneDanger)
			return
		}
		async(20*time.Second, func(ctx context.Context) (api.Group, error) {
			return v.app.api.CreateGroup(ctx, n, c)
		}, func(g api.Group, err error) {
			if err != nil {
				errBanner.Set(err.Error(), toneDanger)
				return
			}
			done()
			v.selGroup = g.UUID
			v.app.setStatus("group "+n+" created", toneSuccess)
			v.load()
		})
	})
}

func (v *tenancyView) setMaster(g api.Group, user string) {
	msg := fmt.Sprintf("Hand %q from %s to %s?", g.Name, v.who(g.Master), v.who(user))
	showConfirmPrompt(v.app.win, "Change master", msg, "Change master", false, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.SetMaster(ctx, g.UUID, g.Master, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("master changed", toneSuccess)
			v.load()
		})
	})
}

func (v *tenancyView) removeMember(g api.Group, user string) {
	msg := fmt.Sprintf("Remove %s from %q? Their turns stop acting in this group immediately.", v.who(user), g.Name)
	showConfirmPrompt(v.app.win, "Remove member", msg, "Remove", true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.RemoveMember(ctx, g.UUID, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("member removed", toneWarning)
			v.openGroup(g.UUID)
		})
	})
}

func (v *tenancyView) disband(g api.Group) {
	msg := fmt.Sprintf("Disband %q? Access ends now for every member; its rows are deleted when the disband retention elapses. This is done as the master (%s).", g.Name, v.who(g.Master))
	showConfirmPrompt(v.app.win, "Disband group", msg, "Disband", true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.DisbandGroup(ctx, g.UUID, g.Master) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("group disbanded", toneWarning)
			v.load()
		})
	})
}

// ---- pools --------------------------------------------------------------------

func (v *tenancyView) renderPools() {
	clearBox(v.poolList)
	if len(v.pools) == 0 {
		v.poolList.Add(container.NewPadded(muted("No pools. Create one, grant agents access, then bind a store to it with pool: <name>.")))
	}
	for _, p := range v.pools {
		p := p
		owner := "tenant " + shortID(p.Owner.Tenant)
		if p.Owner.Group != "" {
			owner = "group " + shortID(p.Owner.Group)
		}
		row := listRow(p.Name, fmt.Sprintf("%s · %dd retention", owner, p.RetentionDays), theme.StorageIcon(), func() { v.openPool(p.UUID) })
		row.SetActive(p.UUID == v.selPool)
		v.poolList.Add(row)
	}
	v.poolList.Refresh()
}

func (v *tenancyView) poolByID(id string) (api.Pool, bool) {
	for _, p := range v.pools {
		if p.UUID == id {
			return p, true
		}
	}
	return api.Pool{}, false
}

func (v *tenancyView) openPool(id string) {
	v.selPool = id
	v.renderPools()
	p, ok := v.poolByID(id)
	if !ok {
		v.placeholder(v.poolDetail, "Select a pool to manage its grants.")
		return
	}
	clearBox(v.poolDetail)
	v.poolDetail.Add(container.NewPadded(muted("Loading grants…")))
	v.poolDetail.Refresh()
	async(20*time.Second, func(ctx context.Context) ([]api.Grant, error) {
		return v.app.api.Grants(ctx, id)
	}, func(grants []api.Grant, err error) {
		if v.selPool != id {
			return
		}
		clearBox(v.poolDetail)
		if err != nil {
			v.poolDetail.Add(messageCard(err.Error(), toneDanger))
			v.poolDetail.Refresh()
			return
		}
		v.renderPool(p, grants)
	})
}

func (v *tenancyView) renderPool(p api.Pool, grants []api.Grant) {
	owner := v.who(p.Owner.Tenant)
	ownerKind := "tenant"
	if p.Owner.Group != "" {
		ownerKind = "group"
		owner = p.Owner.Group
		for _, g := range v.groups {
			if g.UUID == p.Owner.Group {
				owner = g.Name + " · " + shortID(g.UUID)
			}
		}
	}
	head := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), vCenter(chip(ownerKind+" pool", toneAccent))),
		v.app.copyButton("pool: "+p.Name, "store binding"), nil)
	info := grid(3,
		kvLine("owner", owner),
		kvLine("retention", fmt.Sprintf("%d days (immutable)", p.RetentionDays)),
		kvLine("created", relTime(p.CreatedAt)+" by "+shortID(p.CreatedBy)),
	)
	binding := caption(fmt.Sprintf("Bind a memory store to it: stores: { shared: { backend: <name>, table: <table>, pool: %s } }", p.Name))
	v.poolDetail.Add(panel(vstack(spaceS, head, info, binding)))

	var rows [][]fyne.CanvasObject
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].Agent != grants[j].Agent {
			return grants[i].Agent < grants[j].Agent
		}
		return grants[i].Tenant < grants[j].Tenant
	})
	for _, g := range grants {
		g := g
		tenant := v.who(g.Tenant)
		if g.Tenant == "*" || g.Tenant == "" {
			tenant = "every active member"
		}
		mode := chip("read/write", toneSuccess)
		if g.Mode == "r" {
			mode = chip("read only", toneNeutral)
		}
		rows = append(rows, []fyne.CanvasObject{
			cellText(g.Agent), cellText(tenant), vCenter(mode),
			iconButton(theme.ContentClearIcon(), "Revoke", func() { v.revokeGrant(p, g) }),
		})
	}
	var table fyne.CanvasObject = centerNote("No grants: no agent can read or write this pool yet.")
	if len(rows) > 0 {
		table = simpleTable([]tableColumn{{title: "agent", width: 140}, {title: "tenant"}, {title: "mode", width: 96}, {title: "", width: 36}}, rows)
	}

	agent := widget.NewSelectEntry(v.agents)
	agent.SetPlaceHolder("agent")
	tenant := v.userPicker("tenant uuid (empty = every member)")
	mode := widget.NewSelect([]string{"rw", "r"}, nil)
	mode.SetSelected("rw")
	grantBtn := actionButton("Grant", theme.ConfirmIcon(), func() {
		ag := strings.TrimSpace(agent.Text)
		if ag == "" {
			v.s.notice("Choose the agent to grant.", toneWarning)
			return
		}
		tn, md := strings.TrimSpace(tenant.Text), mode.Selected
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.SetGrant(ctx, p.UUID, ag, tn, md) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("grant saved", toneSuccess)
			v.openPool(p.UUID)
		})
	})
	form := container.NewBorder(nil, nil, nil, grantBtn,
		grid(3, controlFrame(agent), controlFrame(tenant), controlFrame(mode)))
	v.poolDetail.Add(section("Grants", "Per (agent, tenant). The member wildcard covers everyone active in the owning group; a tenant pool's wildcard covers nobody but its owner.",
		vstack(spaceS, table, form)))
	v.poolDetail.Refresh()
	v.s.refresh()
}

func (v *tenancyView) revokeGrant(p api.Pool, g api.Grant) {
	tenant := g.Tenant
	if tenant == "*" {
		tenant = ""
	}
	msg := fmt.Sprintf("Revoke %s's %s access to pool %q?", g.Agent, g.Mode, p.Name)
	showConfirmPrompt(v.app.win, "Revoke grant", msg, "Revoke", true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.RemoveGrant(ctx, p.UUID, g.Agent, tenant) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus("grant revoked", toneWarning)
			v.openPool(p.UUID)
		})
	})
}

func (v *tenancyView) newPool() {
	name := widget.NewEntry()
	name.SetPlaceHolder("team-notes")
	creator := v.userPicker("creator tenant uuid")
	kind := widget.NewRadioGroup([]string{"Tenant", "Group"}, nil)
	kind.Horizontal = true
	kind.SetSelected("Group")
	groupNames := []string{}
	groupIDs := map[string]string{}
	for _, g := range v.groups {
		if g.DisbandedAt == 0 {
			label := g.Name + " · " + shortID(g.UUID)
			groupNames = append(groupNames, label)
			groupIDs[label] = g.UUID
		}
	}
	ownerGroup := widget.NewSelectEntry(groupNames)
	ownerGroup.SetPlaceHolder("group (name or uuid)")
	ownerTenant := v.userPicker("owner tenant uuid")
	ownerTenant.Hide()
	kind.OnChanged = func(s string) {
		if s == "Tenant" {
			ownerGroup.Hide()
			ownerTenant.Show()
		} else {
			ownerTenant.Hide()
			ownerGroup.Show()
		}
	}
	retention := widget.NewEntry()
	retention.SetText("90")
	errBanner := newBanner()
	form := vstack(spaceM,
		grid(2,
			vstack(spaceXS, fieldLabel("Name"), controlFrame(name)),
			vstack(spaceXS, fieldLabel("Retention (days)"), controlFrame(retention))),
		vstack(spaceXS, fieldLabel("Creator"), controlFrame(creator)),
		vstack(spaceXS, fieldLabel("Owner"), kind, controlFrame(container.NewStack(ownerGroup, ownerTenant))),
		caption("Names are global and are what config stores bind to. Retention is immutable once set: extending it means a new pool."),
		errBanner)
	v.modal("New memory pool", "A named shared store owned by exactly one tenant or group.", form, "Create pool", func(done func()) {
		days, err := strconv.Atoi(strings.TrimSpace(retention.Text))
		if err != nil || days <= 0 {
			errBanner.Set("Retention must be a positive number of days.", toneDanger)
			return
		}
		d := api.PoolDraft{Name: strings.TrimSpace(name.Text), Creator: strings.TrimSpace(creator.Text), RetentionDays: days}
		if kind.Selected == "Tenant" {
			d.OwnerTenant = strings.TrimSpace(ownerTenant.Text)
		} else {
			g := strings.TrimSpace(ownerGroup.Text)
			if id, ok := groupIDs[g]; ok {
				g = id
			}
			d.OwnerGroup = g
		}
		if d.Name == "" || d.Creator == "" || (d.OwnerTenant == "" && d.OwnerGroup == "") {
			errBanner.Set("A name, a creator and an owner are required.", toneDanger)
			return
		}
		async(20*time.Second, func(ctx context.Context) (api.Pool, error) { return v.app.api.CreatePool(ctx, d) }, func(p api.Pool, err error) {
			if err != nil {
				errBanner.Set(err.Error(), toneDanger)
				return
			}
			done()
			v.selPool = p.UUID
			v.app.setStatus("pool "+d.Name+" created · grant agents before binding it", toneSuccess)
			v.load()
		})
	})
}

// modal shows a form with Cancel and a primary action; submit receives a
// function that closes the modal on success.
func (v *tenancyView) modal(title, subtitle string, form fyne.CanvasObject, okLabel string, submit func(done func())) {
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	ok := primaryButton(okLabel, nil, func() { submit(closePopup) })
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(140, controlFrame(ok))), nil)
	popup = showModalCard(v.app.win, title, subtitle, widthBox(580, form), footer)
}
