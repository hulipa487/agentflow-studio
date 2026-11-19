package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

func (a *App) showAgents() {
	a.enter("agents")
	a.setMain(a.agentsView())
}

func (a *App) agentsView() fyne.CanvasObject {
	var rows []api.AgentRow
	list := vstack(spaceS)
	scroll := pageScroll(list)
	banner := newBanner()

	var render func()
	sbox := newSearchBox(T("agents.filterPlaceholder"), func(string) {
		if render != nil {
			render()
		}
	})
	search := sbox.Entry
	searchWrap := widthBox(220, sbox.Root)
	countLabel := oneLine("")
	newBtn := primaryButton(T("agents.newAgent"), theme.ContentAddIcon(), a.showNewAgent)

	render = func() {
		clearBox(list)
		filter := strings.ToLower(strings.TrimSpace(search.Text))
		var static, spawn []api.AgentRow
		for _, r := range rows {
			if filter != "" {
				match := strings.Contains(strings.ToLower(r.Name), filter) ||
					strings.Contains(strings.ToLower(r.Model), filter) ||
					strings.Contains(strings.ToLower(strings.Join(r.Capabilities, " ")), filter)
				if !match {
					continue
				}
			}
			if r.Spawn {
				spawn = append(spawn, r)
			} else {
				static = append(static, r)
			}
		}
		if len(rows) == 0 {
			countLabel.SetText("")
			newBtn.Hide()
			list.Add(layout.NewSpacer())
			list.Add(emptyAgents(a))
			list.Add(layout.NewSpacer())
			list.Refresh()
			return
		}
		newBtn.Show()
		countLabel.SetText(T("agents.count", "n", len(rows)))
		if len(static) == 0 {
			list.Add(centerNote(T("agents.noMatch")))
		}
		if len(static) > 0 {
			table := vstack(0, agentListHeader(), widget.NewSeparator())
			for i, row := range static {
				if i > 0 {
					table.Add(widget.NewSeparator())
				}
				table.Add(agentRow(a, row))
			}
			list.Add(panelFlush(table))
		}
		if len(spawn) > 0 {
			names := make([]string, 0, len(spawn))
			for _, r := range spawn {
				names = append(names, r.Name)
			}
			list.Add(section(T("agents.spawnTitle"), T("agents.spawnSubtitle"), chips(names, toneNeutral)))
		}
		list.Refresh()
	}

	var load func()
	load = func() {
		clearBox(list)
		loading := widget.NewActivity()
		loading.Start()
		list.Add(container.NewCenter(container.NewVBox(loading, muted(T("agents.loading")))))
		banner.Clear()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			got, err := a.api.Agents(ctx)
			onUI(func() {
				loading.Stop()
				if err != nil {
					a.showError(err)
					clearBox(list)
					banner.Set(err.Error(), toneDanger)
					list.Add(container.NewCenter(widget.NewButtonWithIcon(T("common.retry"), theme.ViewRefreshIcon(), load)))
					return
				}
				rows = got
				render()
			})
		}()
	}
	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), load)
	actions := hstack(spaceS, vCenter(widthBox(76, countLabel)), vCenter(searchWrap), vCenter(refresh), vCenter(newBtn))
	head := pageHeader(T("agents.title"),
		T("agents.subtitle"),
		actions)
	load()

	return container.NewBorder(
		vstack(0, pageInset(head), widget.NewSeparator(), hInset(banner)),
		nil, nil, nil,
		scrollInset(scroll),
	)
}

// Agent list columns. The loop path takes the remaining width.
const (
	agentColName  = float32(210)
	agentColCaps  = float32(250)
	agentColState = float32(190)
	agentColActs  = float32(104)
)

func agentListHeader() fyne.CanvasObject {
	h := func(text string, w float32) fyne.CanvasObject {
		t := canvas.NewText(sentenceCase(text), colorFaint)
		t.TextSize = 11
		if w == 0 {
			return vLeft(t)
		}
		return widthBox(w, t)
	}
	row := container.NewBorder(nil, nil,
		h(T("agents.colAgent"), agentColName),
		hstack(spaceS, h(T("agents.colCapabilities"), agentColCaps), h(T("agents.colState"), agentColState), h("", agentColActs)),
		h(T("agents.colLoop"), 0))
	return container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceM, spaceM), row)
}

// agentRow is one line of the agents list: identity, loop path, capabilities,
// state and actions. Double-click-free: the whole row opens the builder.
func agentRow(a *App, row api.AgentRow) fyne.CanvasObject {
	model := row.Model
	if model == "" {
		model = T("agents.defaultModel")
	}
	name := widget.NewLabel(row.Name)
	name.Truncation = fyne.TextTruncateEllipsis
	modelText := caption(model)
	modelText.Wrapping = fyne.TextWrapOff
	identity := vLeft(vstack(0, compactLabel(name, false), compactLabel(modelText, false)))

	loop := row.Loop
	if loop == "" {
		loop = T("agents.inMemoryLoop")
	}
	loopLabel := mono(loop, theme.SizeNameCaptionText)
	loopLabel.Importance = widget.LowImportance

	var capRow fyne.CanvasObject
	if len(row.Capabilities) == 0 {
		capRow = vLeft(monoText(T("agents.deploymentDefaults"), 10.5, colorFaint))
	} else {
		shown, extra := row.Capabilities, 0
		if len(shown) > 3 {
			extra = len(shown) - 3
			shown = shown[:3]
		}
		box := chips(shown, toneAccent).(*fyne.Container)
		if extra > 0 {
			box.Add(vCenter(chip(fmt.Sprintf("+%d", extra), toneNeutral)))
		}
		capRow = container.NewHBox(box)
	}

	state := hstack(spaceXS)
	if row.Persisted {
		state.Add(vCenter(statusChip(T("agents.persisted"), toneSuccess)))
	} else {
		state.Add(vCenter(statusChip(T("agents.runtimeOnly"), toneWarning)))
	}
	if row.Sessions > 0 {
		state.Add(vCenter(statusChip(T("agents.liveSessions", "n", row.Sessions), toneAccent)))
	}

	actions := hstack(0,
		vCenter(iconButton(theme.MailSendIcon(), T("agents.chat"), func() { a.showPlayground(row.Name) })),
		vCenter(iconButton(theme.DocumentCreateIcon(), T("common.edit"), func() { a.showBuilder(row.Name, nil) })),
		vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { a.confirmDelete(row.Name, false, "") })),
	)

	line := container.NewBorder(nil, nil,
		widthBox(agentColName, identity),
		hstack(spaceS,
			widthBox(agentColCaps, capRow),
			widthBox(agentColState, container.NewHBox(state)),
			widthBox(agentColActs, container.NewHBox(layout.NewSpacer(), actions))),
		vLeft(loopLabel))
	body := container.New(layout.NewCustomPaddedLayout(spaceS-1, spaceS-1, spaceM, spaceS), line)
	return newTapSurface(body, 0, false, nil, func() { a.showBuilder(row.Name, nil) })
}

func emptyAgents(a *App) fyne.CanvasObject {
	return emptyStateCard(
		T("agents.emptyTitle"),
		T("agents.emptyGuidance"),
		T("agents.emptyAction"),
		a.showNewAgent,
	)
}

func (a *App) showNewAgent() {
	if a.api == nil {
		return
	}
	content := vstack(spaceS)
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	blankUse := primaryButton(T("agents.use"), theme.NavigateNextIcon(), func() {
		closePopup()
		a.showBuilder("", nil)
	})
	content.Add(subpanel(container.NewBorder(nil, nil, nil,
		vCenter(widthBox(96, blankUse)),
		vstack(spaceXS,
			hstack(spaceS, vCenter(subheading(T("agents.blankTitle"))), vCenter(chip(T("agents.blankTag"), toneNeutral))),
			muted(T("agents.blankDescription")),
		),
	)))

	for _, tpl := range a.catalog.Templates {
		tpl := tpl
		useBtn := primaryButton(T("agents.use"), theme.NavigateNextIcon(), func() {
			closePopup()
			a.showBuilder("", &tpl)
		})
		card := subpanel(container.NewBorder(nil, nil, nil,
			vCenter(widthBox(96, useBtn)),
			vstack(spaceXS,
				hstack(spaceS, vCenter(subheading(tpl.Title)), vCenter(chip(tpl.Name, toneAccent))),
				muted(tpl.Description),
			),
		))
		content.Add(card)
	}

	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(640, 340))

	title := insetLeading(titleText(T("agents.newTitle"), theme.SizeNameHeadingText))
	closeBtn := iconButton(theme.CancelIcon(), T("common.close"), closePopup)
	header := container.NewBorder(nil, nil, vCenter(title), vCenter(closeBtn), nil)
	subtitle := muted(T("agents.newSubtitle"))
	headerBlock := vstack(spaceXS, header, subtitle)

	cancel := widthBox(104, actionButton(T("common.cancel"), nil, closePopup))
	footer := container.NewBorder(nil, nil, nil, cancel, nil)

	body := vstack(spaceM, headerBlock, widget.NewSeparator(), scroll, footer)
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(18, 16, 20, 20), body))
	if c := a.win.Canvas(); c != nil {
		c.Refresh(c.Content())
	}
	popup = widget.NewModalPopUp(card, a.win.Canvas())
	popup.Show()
}

func (a *App) confirmDelete(name string, force bool, reason string) {
	msg := T("agents.deleteMessage", "name", name)
	if force {
		msg = T("agents.forceMessage", "reason", reason)
		if reason == "" {
			msg = T("agents.liveSessionsMessage", "name", name)
		}
	}
	showConfirmPrompt(a.win, T("agents.deleteTitle"), msg, T("common.delete"), true, func(ok bool) {
		if !ok {
			return
		}
		a.deleteAgent(name, force)
	})
}

func (a *App) deleteAgent(name string, force bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := a.api.DeleteAgent(ctx, name, true, force)
		if err != nil {
			if ae, isAPI := err.(*api.Error); isAPI && ae.Status == 409 && !force {
				onUI(func() { a.confirmDelete(name, true, ae.Message) })
				return
			}
			onUI(func() { a.showError(err) })
			return
		}
		onUI(func() {
			a.setStatus(T("agents.deleted", "name", name), toneSuccess)
			a.showAgents()
		})
	}()
}
