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
	tabs      *container.AppTabs
	selGroup  string
	selPool   string
	userIDs   []string
	userNames map[string]string
	agents    []string
}

func (a *App) showTenancy() { a.showTenancyAt(0, "", "") }

// showTenancyAt opens the Tenancy screen on one tab, with a group and a pool
// selected ("" for none).
func (a *App) showTenancyAt(tab int, group, pool string) {
	a.enter("tenancy")
	v := a.newTenancyViewAt(tab, group, pool)
	a.setMain(v.s.root)
	a.refreshAction = v.load
	// A language switch comes back to the tab and the selection that are open.
	a.reshow = func() { a.showTenancyAt(v.tabs.SelectedIndex(), v.selGroup, v.selPool) }
}

func (a *App) newTenancyView() *tenancyView { return a.newTenancyViewAt(0, "", "") }

func (a *App) newTenancyViewAt(tab int, group, pool string) *tenancyView {
	v := &tenancyView{app: a, userNames: map[string]string{}, selGroup: group, selPool: pool}
	v.groupList, v.groupDetail = vstack(spaceXS), vstack(spaceS)
	v.poolList, v.poolDetail = vstack(spaceXS), vstack(spaceS)

	listPane := func(add fyne.CanvasObject, list fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(vstack(spaceS, add, vSpacer(0)), nil, nil, nil, paneScroll(list))
	}
	detailPane := func(detail fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), paneScroll(detail))
	}
	groupSplit := newHSplitPane(
		listPane(actionButton(T("tenancy.groups.new"), theme.ContentAddIcon(), v.newGroup), v.groupList),
		detailPane(v.groupDetail), 0.32)
	poolSplit := newHSplitPane(
		listPane(actionButton(T("tenancy.pools.new"), theme.ContentAddIcon(), v.newPool), v.poolList),
		detailPane(v.poolDetail), 0.32)

	v.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon(T("tenancy.tabGroups"), iconUser, groupSplit),
		container.NewTabItemWithIcon(T("tenancy.tabPools"), theme.StorageIcon(), poolSplit),
	)
	if tab > 0 && tab < len(v.tabs.Items) {
		v.tabs.SelectIndex(tab)
	}
	v.s = a.newPane(T("tenancy.title"), T("tenancy.subtitle"),
		iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load), v.tabs)
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
		b.Add(container.NewPadded(muted(T("common.loading"))))
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
				v.groupDetail.Add(emptyStateCard(T("tenancy.unavailable"), msg, "", nil))
				v.poolDetail.Add(emptyStateCard(T("tenancy.unavailable"), msg, "", nil))
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
		v.s.meta.SetText(T("tenancy.meta", "groups", len(v.groups), "active", active, "pools", len(v.pools)))
		v.renderGroups()
		v.renderPools()
		if v.selGroup != "" {
			v.openGroup(v.selGroup)
		} else {
			v.placeholder(v.groupDetail, T("tenancy.groups.selectHint"))
		}
		if v.selPool != "" {
			v.openPool(v.selPool)
		} else {
			v.placeholder(v.poolDetail, T("tenancy.pools.selectHint"))
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
		v.groupList.Add(container.NewPadded(muted(T("tenancy.groups.empty"))))
	}
	for _, g := range v.groups {
		g := g
		sub := shortID(g.UUID)
		if g.DisbandedAt > 0 {
			sub += " · " + T("tenancy.groups.disbandedWhen", "when", relTime(g.DisbandedAt))
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
	v.groupDetail.Add(container.NewPadded(muted(T("tenancy.groups.loading"))))
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
	state := chip(T("tenancy.groups.stateActive"), toneSuccess)
	if disbanded {
		state = chip(T("tenancy.groups.stateDisbanded"), toneDanger)
	}
	head := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(widget.NewLabelWithStyle(g.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), vCenter(state)),
		nil, nil)
	info := grid(3,
		kvLine(T("tenancy.groups.kvUuid"), g.UUID),
		kvLine(T("tenancy.groups.master"), v.who(g.Master)),
		kvLine(T("tenancy.kvCreated"), relTime(g.CreatedAt)),
	)
	disband := actionButton(T("tenancy.groups.disbandButton"), theme.DeleteIcon(), func() { v.disband(g) })
	disband.Importance = widget.DangerImportance
	if disbanded {
		disband.Disable()
	}
	copyID := v.app.copyButton(g.UUID, T("tenancy.groups.kvUuid"))
	v.groupDetail.Add(panel(vstack(spaceS, container.NewBorder(nil, nil, nil, hstack(spaceXS, copyID, disband), head), info)))

	var rows [][]fyne.CanvasObject
	members := append([]api.Membership(nil), d.Members...)
	sort.Slice(members, func(i, j int) bool { return members[i].JoinedAt < members[j].JoinedAt })
	for _, m := range members {
		m := m
		role := chip(T("tenancy.groups.member"), toneNeutral)
		isMaster := m.User == g.Master
		if isMaster {
			role = chip(T("tenancy.groups.master"), toneAccent)
		}
		makeMaster := actionButton(T("tenancy.groups.makeMaster"), nil, func() { v.setMaster(g, m.User) })
		remove := iconButton(theme.ContentClearIcon(), T("common.remove"), func() { v.removeMember(g, m.User) })
		if isMaster || disbanded {
			makeMaster.Disable()
			remove.Disable()
		}
		rows = append(rows, []fyne.CanvasObject{
			vCenter(role), cellText(v.who(m.User)), cellText(relTime(m.JoinedAt)), makeMaster, remove,
		})
	}
	var table fyne.CanvasObject = centerNote(T("tenancy.groups.noMembers"))
	if len(rows) > 0 {
		table = simpleTable([]tableColumn{
			{title: T("tenancy.groups.colRole"), width: 74}, {title: T("tenancy.colTenant")}, {title: T("tenancy.groups.colJoined"), width: 90}, {title: "", width: 116}, {title: "", width: 36},
		}, rows)
	}
	add := v.userPicker(T("tenancy.groups.addPlaceholder"))
	addBtn := actionButton(T("tenancy.groups.addMember"), theme.ContentAddIcon(), func() {
		user := strings.TrimSpace(add.Text)
		if user == "" {
			v.s.notice(T("tenancy.groups.enterUuid"), toneWarning)
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.AddMember(ctx, g.UUID, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.groups.memberAdded"), toneSuccess)
			v.openGroup(g.UUID)
		})
	})
	addRow := container.NewBorder(nil, nil, nil, addBtn, controlFrame(add))
	if disbanded {
		addRow.Hide()
	}
	v.groupDetail.Add(section(T("tenancy.groups.membersTitle"), T("tenancy.groups.membersSubtitle"),
		vstack(spaceS, table, addRow)))
	v.groupDetail.Refresh()
	v.s.refresh()
}

func (v *tenancyView) newGroup() {
	name := widget.NewEntry()
	name.SetPlaceHolder("design-team")
	creator := v.userPicker(T("tenancy.groups.creatorPlaceholder"))
	errBanner := newBanner()
	form := vstack(spaceM,
		vstack(spaceXS, fieldLabel(T("common.name")), controlFrame(name)),
		vstack(spaceXS, fieldLabel(T("tenancy.creator")), controlFrame(creator),
			caption(T("tenancy.groups.creatorHint"))),
		errBanner)
	v.modal(T("tenancy.groups.new"), T("tenancy.groups.newSubtitle"), form, T("common.create"), func(done func()) {
		n, c := strings.TrimSpace(name.Text), strings.TrimSpace(creator.Text)
		if n == "" || c == "" {
			errBanner.Set(T("tenancy.groups.required"), toneDanger)
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
			v.app.setStatus(T("tenancy.groups.created", "name", n), toneSuccess)
			v.load()
		})
	})
}

func (v *tenancyView) setMaster(g api.Group, user string) {
	msg := T("tenancy.groups.changeMasterMessage", "name", g.Name, "from", v.who(g.Master), "to", v.who(user))
	showConfirmPrompt(v.app.win, T("tenancy.groups.changeMaster"), msg, T("tenancy.groups.changeMaster"), false, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.SetMaster(ctx, g.UUID, g.Master, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.groups.masterChanged"), toneSuccess)
			v.load()
		})
	})
}

func (v *tenancyView) removeMember(g api.Group, user string) {
	msg := T("tenancy.groups.removeMemberMessage", "who", v.who(user), "name", g.Name)
	showConfirmPrompt(v.app.win, T("tenancy.groups.removeMember"), msg, T("common.remove"), true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.RemoveMember(ctx, g.UUID, user) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.groups.memberRemoved"), toneWarning)
			v.openGroup(g.UUID)
		})
	})
}

func (v *tenancyView) disband(g api.Group) {
	msg := T("tenancy.groups.disbandMessage", "name", g.Name, "master", v.who(g.Master))
	showConfirmPrompt(v.app.win, T("tenancy.groups.disbandTitle"), msg, T("tenancy.groups.disbandConfirm"), true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.DisbandGroup(ctx, g.UUID, g.Master) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.groups.disbandDone"), toneWarning)
			v.load()
		})
	})
}

// ---- pools --------------------------------------------------------------------

func (v *tenancyView) renderPools() {
	clearBox(v.poolList)
	if len(v.pools) == 0 {
		v.poolList.Add(container.NewPadded(muted(T("tenancy.pools.empty"))))
	}
	for _, p := range v.pools {
		p := p
		owner := T("tenancy.pools.ownerTenant", "id", shortID(p.Owner.Tenant))
		if p.Owner.Group != "" {
			owner = T("tenancy.pools.ownerGroup", "id", shortID(p.Owner.Group))
		}
		row := listRow(p.Name, owner+" · "+T("tenancy.pools.retentionShort", "days", p.RetentionDays), theme.StorageIcon(), func() { v.openPool(p.UUID) })
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
		v.placeholder(v.poolDetail, T("tenancy.pools.selectHint"))
		return
	}
	clearBox(v.poolDetail)
	v.poolDetail.Add(container.NewPadded(muted(T("tenancy.pools.loadingGrants"))))
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
	ownerKind := T("tenancy.pools.kindTenant")
	if p.Owner.Group != "" {
		ownerKind = T("tenancy.pools.kindGroup")
		owner = p.Owner.Group
		for _, g := range v.groups {
			if g.UUID == p.Owner.Group {
				owner = g.Name + " · " + shortID(g.UUID)
			}
		}
	}
	head := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})), vCenter(chip(ownerKind, toneAccent))),
		v.app.copyButton("pool: "+p.Name, T("tenancy.pools.whatBinding")), nil)
	info := grid(3,
		kvLine(T("tenancy.pools.kvOwner"), owner),
		kvLine(T("tenancy.pools.kvRetention"), T("tenancy.pools.retentionValue", "days", p.RetentionDays)),
		kvLine(T("tenancy.kvCreated"), T("tenancy.pools.createdBy", "when", relTime(p.CreatedAt), "who", shortID(p.CreatedBy))),
	)
	binding := caption(T("tenancy.pools.bindStoreText", "binding", fmt.Sprintf("stores: { shared: { backend: <name>, table: <table>, pool: %s } }", p.Name)))
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
			tenant = T("tenancy.pools.everyMember")
		}
		mode := chip(T("tenancy.pools.modeRW"), toneSuccess)
		if g.Mode == "r" {
			mode = chip(T("tenancy.pools.modeR"), toneNeutral)
		}
		rows = append(rows, []fyne.CanvasObject{
			cellText(g.Agent), cellText(tenant), vCenter(mode),
			iconButton(theme.ContentClearIcon(), T("tenancy.pools.revoke"), func() { v.revokeGrant(p, g) }),
		})
	}
	var table fyne.CanvasObject = centerNote(T("tenancy.pools.noGrants"))
	if len(rows) > 0 {
		table = simpleTable([]tableColumn{{title: T("tenancy.colAgent"), width: 140}, {title: T("tenancy.colTenant")}, {title: T("tenancy.pools.colMode"), width: 96}, {title: "", width: 36}}, rows)
	}

	agent := widget.NewSelectEntry(v.agents)
	agent.SetPlaceHolder(T("tenancy.pools.agentPlaceholder"))
	tenant := v.userPicker(T("tenancy.pools.tenantPlaceholder"))
	mode := widget.NewSelect([]string{"rw", "r"}, nil)
	mode.SetSelected("rw")
	grantBtn := actionButton(T("tenancy.pools.grant"), theme.ConfirmIcon(), func() {
		ag := strings.TrimSpace(agent.Text)
		if ag == "" {
			v.s.notice(T("tenancy.pools.chooseAgent"), toneWarning)
			return
		}
		tn, md := strings.TrimSpace(tenant.Text), mode.Selected
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.SetGrant(ctx, p.UUID, ag, tn, md) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.pools.grantSaved"), toneSuccess)
			v.openPool(p.UUID)
		})
	})
	form := container.NewBorder(nil, nil, nil, grantBtn,
		grid(3, controlFrame(agent), controlFrame(tenant), controlFrame(mode)))
	v.poolDetail.Add(section(T("tenancy.pools.grantsTitle"), T("tenancy.pools.grantsSubtitle"),
		vstack(spaceS, table, form)))
	v.poolDetail.Refresh()
	v.s.refresh()
}

func (v *tenancyView) revokeGrant(p api.Pool, g api.Grant) {
	tenant := g.Tenant
	if tenant == "*" {
		tenant = ""
	}
	msg := T("tenancy.pools.revokeMessage", "agent", g.Agent, "mode", g.Mode, "name", p.Name)
	showConfirmPrompt(v.app.win, T("tenancy.pools.revokeTitle"), msg, T("tenancy.pools.revoke"), true, func(ok bool) {
		if !ok {
			return
		}
		asyncErr(20*time.Second, func(ctx context.Context) error { return v.app.api.RemoveGrant(ctx, p.UUID, g.Agent, tenant) }, func(err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("tenancy.pools.grantRevoked"), toneWarning)
			v.openPool(p.UUID)
		})
	})
}

func (v *tenancyView) newPool() {
	name := widget.NewEntry()
	name.SetPlaceHolder("team-notes")
	creator := v.userPicker(T("tenancy.pools.creatorPlaceholder"))
	kindTenant, kindGroup := T("tenancy.pools.ownerKindTenant"), T("tenancy.pools.ownerKindGroup")
	kind := widget.NewRadioGroup([]string{kindTenant, kindGroup}, nil)
	kind.Horizontal = true
	kind.SetSelected(kindGroup)
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
	ownerGroup.SetPlaceHolder(T("tenancy.pools.ownerGroupPlaceholder"))
	ownerTenant := v.userPicker(T("tenancy.pools.ownerTenantPlaceholder"))
	ownerTenant.Hide()
	kind.OnChanged = func(s string) {
		if s == kindTenant {
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
			vstack(spaceXS, fieldLabel(T("common.name")), controlFrame(name)),
			vstack(spaceXS, fieldLabel(T("tenancy.pools.retentionLabel")), controlFrame(retention))),
		vstack(spaceXS, fieldLabel(T("tenancy.creator")), controlFrame(creator)),
		vstack(spaceXS, fieldLabel(T("tenancy.pools.owner")), kind, controlFrame(container.NewStack(ownerGroup, ownerTenant))),
		caption(T("tenancy.pools.namesNote")),
		errBanner)
	v.modal(T("tenancy.pools.newTitle"), T("tenancy.pools.newSubtitle"), form, T("tenancy.pools.create"), func(done func()) {
		days, err := strconv.Atoi(strings.TrimSpace(retention.Text))
		if err != nil || days <= 0 {
			errBanner.Set(T("tenancy.pools.retentionInvalid"), toneDanger)
			return
		}
		d := api.PoolDraft{Name: strings.TrimSpace(name.Text), Creator: strings.TrimSpace(creator.Text), RetentionDays: days}
		if kind.Selected == kindTenant {
			d.OwnerTenant = strings.TrimSpace(ownerTenant.Text)
		} else {
			g := strings.TrimSpace(ownerGroup.Text)
			if id, ok := groupIDs[g]; ok {
				g = id
			}
			d.OwnerGroup = g
		}
		if d.Name == "" || d.Creator == "" || (d.OwnerTenant == "" && d.OwnerGroup == "") {
			errBanner.Set(T("tenancy.pools.required"), toneDanger)
			return
		}
		async(20*time.Second, func(ctx context.Context) (api.Pool, error) { return v.app.api.CreatePool(ctx, d) }, func(p api.Pool, err error) {
			if err != nil {
				errBanner.Set(err.Error(), toneDanger)
				return
			}
			done()
			v.selPool = p.UUID
			v.app.setStatus(T("tenancy.pools.created", "name", d.Name), toneSuccess)
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
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(140, controlFrame(ok))), nil)
	popup = showModalCard(v.app.win, title, subtitle, widthBox(580, form), footer)
}
