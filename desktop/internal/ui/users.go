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

func (a *App) showUsers() {
	a.enter("users")
	v := a.newUsersView()
	a.setMain(v.s.root)
	a.refreshAction = func() { v.load() }
}

func (a *App) newUsersView() *usersView {
	v := &usersView{app: a}
	v.list = vstack(spaceXS)
	v.detail = vstack(spaceS)
	v.count = oneLine("")
	search := newSearchBox("Filter by name, email or id", func(q string) {
		v.filter = strings.ToLower(strings.TrimSpace(q))
		v.renderList()
	})
	left := container.NewBorder(vstack(spaceS, search.Root, v.count), nil, nil, nil, paneScroll(v.list))
	split := newHSplitPane(left, container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), paneScroll(v.detail)), 0.34)
	actions := hstack(spaceS,
		iconButton(theme.ViewRefreshIcon(), "Refresh", func() { v.load() }),
		primaryButton("Invite", theme.MailSendIcon(), a.issueInvite),
	)
	v.s = a.newPane("Users",
		"Profiles behind the identity layer: linked channel handles, today's quota, and the per-user model and instruction layer. Requires runtime.identity.enabled.",
		actions, split)
	v.load()
	return v
}

func (v *usersView) load() {
	v.loadSeq++
	seq := v.loadSeq
	clearBox(v.list)
	v.list.Add(container.NewPadded(muted("Loading…")))
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
				v.detail.Add(emptyStateCard("The identity layer is off", msg, "", nil))
			} else {
				v.s.notice(err.Error(), toneDanger)
				v.list.Add(container.NewCenter(actionButton("Retry", theme.ViewRefreshIcon(), v.load)))
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
		v.s.meta.SetText(fmt.Sprintf("%d profiles · quota day %s (UTC)", len(v.users), l.Day))
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
		quota := fmt.Sprintf("%s today", humanCount(used))
		if limit > 0 {
			quota = fmt.Sprintf("%s / %s", humanCount(used), humanCount(limit))
		}
		handles := fmt.Sprintf("%d handles", u.Identities)
		if u.Identities == 1 {
			handles = "1 handle"
		}
		row := listRow(title, sub+" · "+quota+" · "+handles, iconUser, func() { v.open(u.UserID) })
		row.SetActive(u.UserID == v.selected)
		v.list.Add(row)
	}
	switch {
	case len(v.users) == 0:
		v.count.SetText("no profiles yet")
		v.list.Add(container.NewPadded(muted("Nobody has registered. Issue an invite, or let users register through /v1/users.")))
	case shown == 0:
		v.count.SetText("no match")
	default:
		v.count.SetText(fmt.Sprintf("%d of %d", shown, len(v.users)))
	}
	v.list.Refresh()
}

func (v *usersView) renderPlaceholder() {
	clearBox(v.detail)
	if len(v.users) > 0 {
		v.detail.Add(vSpacer(spaceXL))
		v.detail.Add(centerNote("Select a profile to see its handles, usage and audit trail."))
	}
	v.detail.Refresh()
}

func (v *usersView) open(id string) {
	v.selected = id
	v.renderList()
	clearBox(v.detail)
	v.detail.Add(container.NewPadded(muted("Loading profile…")))
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
		badges.Add(vCenter(chip("model "+p.Model, toneAccent)))
	}
	if p.InstructionsAppend != "" {
		badges.Add(vCenter(chip("+instructions", toneAccent)))
	}
	limitText := "default limit"
	if p.TokensPerDay > 0 {
		limitText = humanCount(p.TokensPerDay) + "/day"
	}
	badges.Add(vCenter(chip(limitText, toneNeutral)))

	idLabel := mono(p.UserID, theme.SizeNameCaptionText)
	idLabel.Truncation = fyne.TextTruncateOff
	idRow := hstack(spaceXS, vCenter(idLabel), v.app.copyButton(p.UserID, "user id"))
	head := container.NewBorder(nil, nil,
		hstack(spaceM, avatar(letter, 44), vstack(2,
			widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			idRow)),
		nil, nil)
	actions := hstack(spaceS,
		actionButton("Daily limit…", theme.InfoIcon(), func() { v.editLimit(p) }),
		actionButton("Model & instructions…", theme.SettingsIcon(), func() { v.editSettings(p) }),
		actionButton("Export…", theme.DownloadIcon(), func() { v.export(p) }),
		iconButton(theme.DeleteIcon(), "Delete profile", func() { v.confirmDelete(p) }),
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
	tiles.Add(statTile("Billable today", humanCount(today.Billable()), fmt.Sprintf("in %s · out %s", humanCount(today.Input), humanCount(today.Output)), toneNeutral, nil))
	tiles.Add(statTile("Calls", humanCount(today.Calls), fmt.Sprintf("%d failed", today.Failed), toneNeutral, nil))
	tiles.Add(statTile("Cached", humanCount(today.Cached), fmt.Sprintf("reasoning %s", humanCount(today.Reasoning)), toneNeutral, nil))
	tiles.Add(statTile("In flight", humanCount(inflight), "held against the quota", toneNeutral, nil))
	v.detail.Add(section("Quota", "Day "+orDash(d.Day)+" (UTC). The limit is the profile's own, or usage.default_tokens_per_day.",
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
		unlink := iconButton(theme.ContentClearIcon(), "Unlink", func() { v.confirmUnlink(p, idn) })
		if len(p.Identities) <= 1 {
			unlink.Disable()
		}
		handleRows = append(handleRows, []fyne.CanvasObject{
			vCenter(chip(idn.Channel, toneNeutral)), cellText(who), vCenter(chip(orDash(idn.Trust), trust)),
			cellText(relTime(idn.LastSeen)), unlink,
		})
	}
	var handles fyne.CanvasObject = centerNote("No linked handles.")
	if len(handleRows) > 0 {
		handles = simpleTable([]tableColumn{
			{title: "channel", width: 96}, {title: "handle"}, {title: "trust", width: 86}, {title: "last seen", width: 90}, {title: "", width: 36},
		}, handleRows)
	}
	v.detail.Add(section("Linked handles", "A person links a handle by sending /link <code> from it. The last handle cannot be unlinked.", handles))

	// Projects.
	if len(d.Projects) > 0 {
		v.detail.Add(section("File projects", "The user-scoped file store (files.*).", chips(d.Projects, toneNeutral)))
	}

	// Recent calls.
	if len(d.RecentCalls) > 0 {
		var rows [][]fyne.CanvasObject
		for _, e := range d.RecentCalls {
			okChip := chip("ok", toneSuccess)
			if !e.OK {
				okChip = chip("failed", toneDanger)
			}
			rows = append(rows, []fyne.CanvasObject{
				cellText(relTime(e.Ts)), cellText(e.Agent), cellMono(e.Model), cellNum(e.Billable()), vCenter(okChip),
			})
		}
		v.detail.Add(section("Recent calls", "Per-call detail (usage.events), last 7 days.",
			simpleTable([]tableColumn{{title: "when", width: 80}, {title: "agent", width: 110}, {title: "model"}, {title: "tokens", width: 70, right: true}, {title: "", width: 64}}, rows)))
	}

	// Audit tail.
	if len(d.Audit) > 0 {
		var rows [][]fyne.CanvasObject
		for _, m := range d.Audit {
			dir := chip("in", toneAccent)
			if m.Direction == "out" {
				dir = chip("out", toneNeutral)
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
		v.detail.Add(section("Audit trail", "The core-owned message journal for this person, newest first.",
			simpleTable([]tableColumn{{title: "when", width: 80}, {title: "dir", width: 44}, {title: "status", width: 120}, {title: "agent", width: 100}, {title: "text"}}, rows)))
	}
	v.detail.Refresh()
	v.s.refresh()
}

func (v *usersView) editLimit(p api.Profile) {
	initial := ""
	if p.TokensPerDay > 0 {
		initial = strconv.FormatInt(p.TokensPerDay, 10)
	}
	v.app.promptText("Daily token limit",
		"Billable tokens (input + output) per UTC day. 0 or empty inherits usage.default_tokens_per_day.",
		"Tokens per day", "e.g. 200000", initial, "Set limit",
		func(s string) error {
			if s == "" {
				return nil
			}
			n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
			if err != nil || n < 0 {
				return fmt.Errorf("enter a whole number of tokens, 0 or more")
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
				v.app.setStatus("limit updated", toneSuccess)
				v.load()
			})
		})
}

func (v *usersView) editSettings(p api.Profile) {
	models := append([]string{"(agent default)"}, v.app.catalog.Models...)
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
		vstack(spaceXS, fieldLabel("Model"), controlFrame(model),
			caption("Overrides the agent's model for this person's turns. Which model a person runs on is a cost decision, so only an operator can set it.")),
		vstack(spaceXS, fieldLabel("Instructions layer"), instr,
			caption("Appended to the agent's instructions for this person (agent.info().instructions). Empty clears it.")),
		errBanner,
	)
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	save := primaryButton("Save", theme.DocumentSaveIcon(), nil)
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
			v.app.setStatus("settings saved · applies from the next turn", toneSuccess)
			v.open(p.UserID)
		})
	}
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(110, controlFrame(save))), nil)
	popup = showModalCard(v.app.win, "Model & instructions", "Per-person layer on top of every agent this person talks to.", widthBox(620, form), footer)
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
				v.s.notice("export failed: "+err.Error(), toneDanger)
				return
			}
			v.app.setStatus("exported to "+w.URI().Name(), toneSuccess)
		}, v.app.win)
		d.SetFileName("agentflow-user-" + shortID(p.UserID) + ".json")
		d.Show()
	})
}

func (v *usersView) confirmUnlink(p api.Profile, idn api.Identity) {
	msg := fmt.Sprintf("Detach the %s handle %q from this profile?\n\nThat handle loses the personal scope until it is linked again.", idn.Channel, orDash(idn.NativeFrom))
	showConfirmPrompt(v.app.win, "Unlink handle", msg, "Unlink", true, func(ok bool) {
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
			v.app.setStatus("handle unlinked", toneWarning)
			v.load()
		})
	})
}

func (v *usersView) confirmDelete(p api.Profile) {
	v.app.promptText("Delete profile",
		"This purges the profile, its handles, usage, journal rows and files. Memory rows follow their store retention. It cannot be undone. Type the user id to confirm.",
		"User id", p.UserID, "", "Delete forever",
		func(s string) error {
			if s != p.UserID {
				return fmt.Errorf("type the exact user id to confirm")
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
				v.app.setStatus("profile deleted", toneWarning)
				v.load()
			})
		})
}

// issueInvite mints a single-use registration code and shows it once.
func (a *App) issueInvite() {
	async(20*time.Second, a.api.IssueInvite, func(inv api.Invite, err error) {
		if err != nil {
			if msg, off := featureOff(err); off {
				showInfoPrompt(a.win, "Invites unavailable", msg)
				return
			}
			a.showError(err)
			return
		}
		code := cellMono(inv.Invite)
		body := vstack(spaceS,
			panel(container.NewBorder(nil, nil, nil, a.copyButton(inv.Invite, "invite code"), code)),
			caption("Single use, expires in "+orDash(inv.ExpiresIn)+". "+inv.Hint),
			caption("Invites matter when runtime.users.registration is invite; with open registration anyone can register without one."),
		)
		var popup *widget.PopUp
		done := widthBox(100, controlFrame(primaryButton("Done", nil, func() {
			if popup != nil {
				popup.Hide()
			}
		})))
		popup = showModalCard(a.win, "Registration invite", "Give this code to the person; it is not shown again.",
			widthBox(520, body), container.NewBorder(nil, nil, nil, done, nil))
	})
}
