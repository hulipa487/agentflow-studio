package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

type usersView struct {
	app *App
	s   *screen

	list   *fyne.Container
	detail *fyne.Container
	count  *widget.Label
	filter string

	users    []api.UserRow
	day      string
	selected string
	loadSeq  int
}

func (a *App) showUsers() { a.showUsersAt("") }

// showUsersAt opens the Users screen with one profile selected ("" for none).
func (a *App) showUsersAt(selected string) {
	a.enter("users")
	v := a.newUsersViewAt(selected)
	a.setMain(v.s.root)
	a.refreshAction = func() { v.load() }
	// A language switch comes back to the profile that is open.
	a.reshow = func() { a.showUsersAt(v.selected) }
}

func (a *App) newUsersView() *usersView { return a.newUsersViewAt("") }

func (a *App) newUsersViewAt(selected string) *usersView {
	v := &usersView{app: a, selected: selected}
	v.list = vstack(spaceXS)
	v.detail = vstack(spaceS)
	v.count = oneLine("")
	search := newSearchBox(T("users.filterPlaceholder"), func(q string) {
		v.filter = strings.ToLower(strings.TrimSpace(q))
		v.renderList()
	})
	left := container.NewBorder(vstack(spaceS, search.Root, v.count), nil, nil, nil, paneScroll(v.list))
	split := newHSplitPane(left, container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), paneScroll(v.detail)), 0.34)
	actions := hstack(spaceS,
		iconButton(theme.ViewRefreshIcon(), T("common.refresh"), func() { v.load() }),
		primaryButton(T("users.invite"), theme.MailSendIcon(), a.issueInvite),
	)
	v.s = a.newPane(T("users.title"), T("users.subtitle"), actions, split)
	v.load()
	return v
}

func (v *usersView) load() {
	v.loadSeq++
	seq := v.loadSeq
	clearBox(v.list)
	v.list.Add(container.NewPadded(muted(T("common.loading"))))
	v.list.Refresh()
	async(20*time.Second, v.app.api.Users, func(l api.UserList, err error) {
		if seq != v.loadSeq {
			return
		}
		if err != nil {
			clearBox(v.list)
			clearBox(v.detail)
			if msg, off := featureOff(err); off {
				v.s.banner.Clear()
				v.detail.Add(emptyStateCard(T("users.identityOff"), msg, "", nil))
			} else {
				v.s.notice(err.Error(), toneDanger)
				v.list.Add(container.NewCenter(actionButton(T("common.retry"), theme.ViewRefreshIcon(), v.load)))
			}
			v.list.Refresh()
			v.detail.Refresh()
			v.s.refresh()
			return
		}
		v.s.banner.Clear()
		v.users = l.Users
		v.day = l.Day
		sort.Slice(v.users, func(i, j int) bool { return userTitle(v.users[i]) < userTitle(v.users[j]) })
		v.s.meta.SetText(T("users.meta", "n", len(v.users), "day", l.Day))
		v.renderList()
		if v.selected != "" {
			v.open(v.selected)
		} else {
			v.renderPlaceholder()
		}
	})
}

func userTitle(u api.UserRow) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	if u.Email != "" {
		return u.Email
	}
	return u.UserID
}

func (v *usersView) renderList() {
	clearBox(v.list)
	shown := 0
	for _, u := range v.users {
		hay := strings.ToLower(u.DisplayName + " " + u.Email + " " + u.UserID)
		if v.filter != "" && !strings.Contains(hay, v.filter) {
			continue
		}
		shown++
		u := u
		title := userTitle(u)
		sub := shortID(u.UserID)
		if u.Email != "" && u.Email != title {
			sub = u.Email + " · " + sub
		}
		used, limit := int64(0), u.TokensPerDay
		if u.Quota != nil {
			used, limit = u.Quota.Used, u.Quota.Limit
		}
		quota := T("users.usedToday", "used", humanCount(used))
		if limit > 0 {
			quota = fmt.Sprintf("%s / %s", humanCount(used), humanCount(limit))
		}
		handles := T("users.handleCount", "n", u.Identities)
		row := listRow(title, sub+" · "+quota+" · "+handles, iconUser, func() { v.open(u.UserID) })
		row.SetActive(u.UserID == v.selected)
		v.list.Add(row)
	}
	switch {
	case len(v.users) == 0:
		v.count.SetText(T("users.countNone"))
		v.list.Add(container.NewPadded(muted(T("users.emptyList"))))
	case shown == 0:
		v.count.SetText(T("users.countNoMatch"))
	default:
		v.count.SetText(T("users.countShown", "shown", shown, "total", len(v.users)))
	}
	v.list.Refresh()
}

func (v *usersView) renderPlaceholder() {
	clearBox(v.detail)
	if len(v.users) > 0 {
		v.detail.Add(vSpacer(spaceXL))
		v.detail.Add(centerNote(T("users.selectHint")))
	}
	v.detail.Refresh()
}

func (v *usersView) open(id string) {
	v.selected = id
	v.renderList()
	clearBox(v.detail)
	v.detail.Add(container.NewPadded(muted(T("users.loadingProfile"))))
	v.detail.Refresh()
	async(20*time.Second, func(ctx context.Context) (api.UserDetail, error) {
		return v.app.api.User(ctx, id)
	}, func(d api.UserDetail, err error) {
		if v.selected != id {
			return
		}
		clearBox(v.detail)
		if err != nil {
			v.detail.Add(messageCard(err.Error(), toneDanger))
			v.detail.Refresh()
			return
		}
		v.renderDetail(d)
	})
}

func (v *usersView) renderDetail(d api.UserDetail) {
	p := d.Profile
	clearBox(v.detail)
	title := p.DisplayName
	if title == "" {
		title = orDash(p.Email)
	}
	letter := "?"
	if t := strings.TrimSpace(title); t != "" && t != "—" {
		letter = strings.ToUpper(t[:1])
	}
	badges := hstack(spaceXS)
	if p.Model != "" {
		badges.Add(vCenter(chip(T("users.chipModel", "model", p.Model), toneAccent)))
	}
	if p.InstructionsAppend != "" {
		badges.Add(vCenter(chip(T("users.chipInstructions"), toneAccent)))
	}
	limitText := T("users.chipDefaultLimit")
	if p.TokensPerDay > 0 {
		limitText = T("users.chipLimit", "n", humanCount(p.TokensPerDay))
	}
	badges.Add(vCenter(chip(limitText, toneNeutral)))

	idLabel := mono(p.UserID, theme.SizeNameCaptionText)
	idLabel.Truncation = fyne.TextTruncateOff
	idRow := hstack(spaceXS, vCenter(idLabel), v.app.copyButton(p.UserID, T("users.whatUserId")))
	head := container.NewBorder(nil, nil,
		hstack(spaceM, avatar(letter, 44), vstack(2,
			widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			idRow)),
		nil, nil)
	actions := hstack(spaceS,
		actionButton(T("users.dailyLimit"), theme.InfoIcon(), func() { v.editLimit(p) }),
		actionButton(T("users.modelAndInstructions"), theme.SettingsIcon(), func() { v.editSettings(p) }),
		actionButton(T("users.export"), theme.DownloadIcon(), func() { v.export(p) }),
		iconButton(theme.DeleteIcon(), T("users.deleteProfile"), func() { v.confirmDelete(p) }),
	)
	v.detail.Add(panel(vstack(spaceS, head, badges, widget.NewSeparator(), actions)))

	// Quota and today's usage.
	var used, limit, inflight int64
	if d.Quota != nil {
		used, limit, inflight = d.Quota.Used, d.Quota.Limit, d.Quota.InFlight
	}
	tiles := grid(4)
	var today api.UsageTotals
	if d.UsageToday != nil {
		today = *d.UsageToday
	}
	tiles.Add(statTile(T("users.quota.billableToday"), humanCount(today.Billable()), T("users.quota.inOut", "input", humanCount(today.Input), "output", humanCount(today.Output)), toneNeutral, nil))
	tiles.Add(statTile(T("users.quota.calls"), humanCount(today.Calls), T("users.quota.failed", "n", today.Failed), toneNeutral, nil))
	tiles.Add(statTile(T("users.quota.cached"), humanCount(today.Cached), T("users.quota.reasoning", "n", humanCount(today.Reasoning)), toneNeutral, nil))
	tiles.Add(statTile(T("users.quota.inFlight"), humanCount(inflight), T("users.quota.held"), toneNeutral, nil))
	v.detail.Add(section(T("users.quota.title"), T("users.quota.subtitle", "day", orDash(d.Day)),
		vstack(spaceS, meterBar(used, limit), tiles)))

	// Linked handles.
	var handleRows [][]fyne.CanvasObject
	for _, idn := range p.Identities {
		idn := idn
		who := idn.Username
		if who == "" {
			who = idn.Name
		}
		if who == "" {
			who = idn.NativeFrom
		}
		trust := toneWarning
		if idn.Trust == "verified" {
			trust = toneSuccess
		}
		unlink := iconButton(theme.ContentClearIcon(), T("users.handles.unlink"), func() { v.confirmUnlink(p, idn) })
		if len(p.Identities) <= 1 {
			unlink.Disable()
		}
		handleRows = append(handleRows, []fyne.CanvasObject{
			vCenter(chip(idn.Channel, toneNeutral)), cellText(who), vCenter(chip(orDash(idn.Trust), trust)),
			cellText(relTime(idn.LastSeen)), unlink,
		})
	}
	var handles fyne.CanvasObject = centerNote(T("users.handles.empty"))
	if len(handleRows) > 0 {
		handles = simpleTable([]tableColumn{
			{title: T("users.handles.colChannel"), width: 96}, {title: T("users.handles.colHandle")}, {title: T("users.handles.colTrust"), width: 86}, {title: T("users.handles.colLastSeen"), width: 90}, {title: "", width: 36},
		}, handleRows)
	}
	v.detail.Add(section(T("users.handles.title"), T("users.handles.subtitle"), handles))

	// Projects.
	if len(d.Projects) > 0 {
		v.detail.Add(section(T("users.projects.title"), T("users.projects.subtitle"), chips(d.Projects, toneNeutral)))
	}

	// Recent calls.
	if len(d.RecentCalls) > 0 {
		var rows [][]fyne.CanvasObject
		for _, e := range d.RecentCalls {
			okChip := chip(T("users.calls.ok"), toneSuccess)
			if !e.OK {
				okChip = chip(T("users.calls.failed"), toneDanger)
			}
			rows = append(rows, []fyne.CanvasObject{
				cellText(relTime(e.Ts)), cellText(e.Agent), cellMono(e.Model), cellNum(e.Billable()), vCenter(okChip),
			})
		}
		v.detail.Add(section(T("users.calls.title"), T("users.calls.subtitle"),
			simpleTable([]tableColumn{{title: T("users.calls.colWhen"), width: 80}, {title: T("users.calls.colAgent"), width: 110}, {title: T("users.calls.colModel")}, {title: T("users.calls.colTokens"), width: 70, right: true}, {title: "", width: 64}}, rows)))
	}

	// Audit tail.
	if len(d.Audit) > 0 {
		var rows [][]fyne.CanvasObject
		for _, m := range d.Audit {
			dir := chip(T("users.audit.in"), toneAccent)
			if m.Direction == "out" {
				dir = chip(T("users.audit.out"), toneNeutral)
			}
			st := toneNeutral
			switch m.Status {
			case "failed", "blocked_safety", "dropped_queue":
				st = toneDanger
			case "delivered", "routed":
				st = toneSuccess
			}
			text := strings.ReplaceAll(m.Text, "\n", " ")
			if m.Err != "" {
				text = m.Err
			}
			rows = append(rows, []fyne.CanvasObject{
				cellText(relTime(m.Ts)), vCenter(dir), vCenter(chip(m.Status, st)), cellText(orDash(m.Agent)), cellText(text),
			})
		}
		v.detail.Add(section(T("users.audit.title"), T("users.audit.subtitle"),
			simpleTable([]tableColumn{{title: T("users.calls.colWhen"), width: 80}, {title: T("users.audit.colDir"), width: 44}, {title: T("users.audit.colStatus"), width: 120}, {title: T("users.calls.colAgent"), width: 100}, {title: T("users.audit.colText")}}, rows)))
	}
	v.detail.Refresh()
	v.s.refresh()
}

func (v *usersView) editLimit(p api.Profile) {
	initial := ""
	if p.TokensPerDay > 0 {
		initial = strconv.FormatInt(p.TokensPerDay, 10)
	}
	v.app.promptText(T("users.limit.title"), T("users.limit.subtitle"),
		T("users.limit.label"), T("users.limit.placeholder"), initial, T("users.limit.ok"),
		func(s string) error {
			if s == "" {
				return nil
			}
			n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
			if err != nil || n < 0 {
				return errors.New(T("users.limit.invalid"))
			}
			return nil
		},
		func(s string) {
			n, _ := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
			async(20*time.Second, func(ctx context.Context) (api.Profile, error) {
				return v.app.api.SetUserLimit(ctx, p.UserID, n)
			}, func(_ api.Profile, err error) {
				if err != nil {
					v.s.notice(err.Error(), toneDanger)
					return
				}
				v.app.setStatus(T("users.limit.updated"), toneSuccess)
				v.load()
			})
		})
}

func (v *usersView) editSettings(p api.Profile) {
	models := append([]string{T("users.settings.agentDefault")}, v.app.catalog.Models...)
	model := widget.NewSelect(models, nil)
	if p.Model != "" {
		if !hasString(models, p.Model) {
			model.Options = append(model.Options, p.Model)
		}
		model.SetSelected(p.Model)
	} else {
		model.SetSelected(models[0])
	}
	instr := monoEntry(true)
	instr.SetText(p.InstructionsAppend)
	instr.SetMinRowsVisible(8)
	errBanner := newBanner()
	form := vstack(spaceM,
		vstack(spaceXS, fieldLabel(T("users.settings.model")), controlFrame(model),
			caption(T("users.settings.modelHint"))),
		vstack(spaceXS, fieldLabel(T("users.settings.instructions")), instr,
			caption(T("users.settings.instructionsHint"))),
		errBanner,
	)
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	save := primaryButton(T("common.save"), theme.DocumentSaveIcon(), nil)
	save.OnTapped = func() {
		m := model.Selected
		if m == models[0] {
			m = ""
		}
		text := instr.Text
		settings := api.UserSettings{Model: &m, InstructionsAppend: &text}
		save.Disable()
		async(20*time.Second, func(ctx context.Context) (api.Profile, error) {
			return v.app.api.SetUserSettings(ctx, p.UserID, settings)
		}, func(_ api.Profile, err error) {
			save.Enable()
			if err != nil {
				errBanner.Set(err.Error(), toneDanger)
				return
			}
			closePopup()
			v.app.setStatus(T("users.settings.saved"), toneSuccess)
			v.open(p.UserID)
		})
	}
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(110, controlFrame(save))), nil)
	popup = showModalCard(v.app.win, T("users.settings.title"), T("users.settings.subtitle"), widthBox(620, form), footer)
}

func (v *usersView) export(p api.Profile) {
	async(30*time.Second, func(ctx context.Context) ([]byte, error) {
		return v.app.api.ExportUser(ctx, p.UserID)
	}, func(data []byte, err error) {
		if err != nil {
			v.s.notice(err.Error(), toneDanger)
			return
		}
		d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
			if err != nil || w == nil {
				return
			}
			defer w.Close()
			if _, err := w.Write(data); err != nil {
				v.s.notice(T("users.exportFailed", "message", err.Error()), toneDanger)
				return
			}
			v.app.setStatus(T("users.exported", "name", w.URI().Name()), toneSuccess)
		}, v.app.win)
		d.SetFileName("agentflow-user-" + shortID(p.UserID) + ".json")
		d.Show()
	})
}

func (v *usersView) confirmUnlink(p api.Profile, idn api.Identity) {
	msg := T("users.unlink.message", "channel", idn.Channel, "handle", orDash(idn.NativeFrom))
	showConfirmPrompt(v.app.win, T("users.unlink.title"), msg, T("users.handles.unlink"), true, func(ok bool) {
		if !ok {
			return
		}
		async(20*time.Second, func(ctx context.Context) (api.Profile, error) {
			return v.app.api.UnlinkIdentity(ctx, p.UserID, idn.ID)
		}, func(_ api.Profile, err error) {
			if err != nil {
				v.s.notice(err.Error(), toneDanger)
				return
			}
			v.app.setStatus(T("users.unlink.done"), toneWarning)
			v.load()
		})
	})
}

func (v *usersView) confirmDelete(p api.Profile) {
	v.app.promptText(T("users.deleteProfile"), T("users.delete.message"),
		T("users.delete.label"), p.UserID, "", T("users.delete.confirm"),
		func(s string) error {
			if s != p.UserID {
				return errors.New(T("users.delete.mismatch"))
			}
			return nil
		},
		func(string) {
			asyncErr(30*time.Second, func(ctx context.Context) error {
				return v.app.api.DeleteUser(ctx, p.UserID)
			}, func(err error) {
				if err != nil {
					v.s.notice(err.Error(), toneDanger)
					return
				}
				v.selected = ""
				v.app.setStatus(T("users.delete.done"), toneWarning)
				v.load()
			})
		})
}

// issueInvite mints a single-use registration code and shows it once.
func (a *App) issueInvite() {
	async(20*time.Second, a.api.IssueInvite, func(inv api.Invite, err error) {
		if err != nil {
			if msg, off := featureOff(err); off {
				showInfoPrompt(a.win, T("users.invitesUnavailable"), msg)
				return
			}
			a.showError(err)
			return
		}
		code := cellMono(inv.Invite)
		body := vstack(spaceS,
			panel(container.NewBorder(nil, nil, nil, a.copyButton(inv.Invite, T("users.whatInviteCode")), code)),
			caption(T("users.inviteDialog.singleUse", "expires", orDash(inv.ExpiresIn), "hint", inv.Hint)),
			caption(T("users.inviteDialog.registrationNote")),
		)
		var popup *widget.PopUp
		done := widthBox(100, controlFrame(primaryButton(T("common.done"), nil, func() {
			if popup != nil {
				popup.Hide()
			}
		})))
		popup = showModalCard(a.win, T("users.inviteDialog.title"), T("users.inviteDialog.subtitle"),
			widthBox(520, body), container.NewBorder(nil, nil, nil, done, nil))
	})
}
