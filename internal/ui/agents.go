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
	scroll := container.NewVScroll(list)
	banner := newBanner()

	var render func()
	sbox := newSearchBox("Filter agents…", func(string) {
		if render != nil {
			render()
		}
	})
	search := sbox.Entry
	searchWrap := widthBox(220, sbox.Root)
	countLabel := oneLine("")
	newBtn := primaryButton("New agent", theme.ContentAddIcon(), a.showNewAgent)

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
		if len(rows) == 1 {
			countLabel.SetText("1 agent")
		} else {
			countLabel.SetText(fmt.Sprintf("%d agents", len(rows)))
		}
		if len(static) == 0 {
			list.Add(centerNote("No agents match this filter."))
		}
		for _, row := range static {
			list.Add(agentCard(a, row))
		}
		if len(spawn) > 0 {
			names := make([]string, 0, len(spawn))
			for _, r := range spawn {
				names = append(names, r.Name)
			}
			list.Add(section("Spawn profiles", "Templates used by agent.spawn; read-only in Studio.", chips(names, toneNeutral)))
		}
		list.Refresh()
	}

	var load func()
	load = func() {
		clearBox(list)
		loading := widget.NewActivity()
		loading.Start()
		list.Add(container.NewCenter(container.NewVBox(loading, muted("Loading agents…"))))
		banner.Clear()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			got, err := a.api.Agents(ctx)
			fyne.Do(func() {
				loading.Stop()
				if err != nil {
					a.showError(err)
					clearBox(list)
					banner.Set(err.Error(), toneDanger)
					list.Add(container.NewCenter(widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), load)))
					return
				}
				rows = got
				render()
			})
		}()
	}
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", load)
	actions := hstack(spaceS, vCenter(widthBox(76, countLabel)), vCenter(searchWrap), vCenter(refresh), vCenter(newBtn))
	head := pageHeader("Agents",
		"Prototype a loop, apply it live, and keep the definition in your deployment config.",
		actions)
	load()

	return container.NewBorder(
		vstack(0, pageInset(head), widget.NewSeparator(), hInset(banner)),
		nil, nil, nil,
		pageInset(scroll),
	)
}

func agentCard(a *App, row api.AgentRow) fyne.CanvasObject {
	letter := "?"
	if row.Name != "" {
		letter = strings.ToUpper(row.Name[:1])
	}
	model := row.Model
	if model == "" {
		model = "default model"
	}

	stateTone := toneWarning
	stateLabel := "runtime only"
	if row.Persisted {
		stateTone, stateLabel = toneSuccess, "persisted"
	}

	nameLabel := widget.NewLabelWithStyle(row.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusChips := hstack(spaceXS, vCenter(chip(stateLabel, stateTone)))
	if row.Sessions > 0 {
		statusChips.Add(vCenter(chip(fmt.Sprintf("%d live", row.Sessions), toneAccent)))
	}
	buttons := hstack(spaceXS,
		vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { a.showBuilder(row.Name, nil) })),
		vCenter(actionButton("Chat", theme.MailSendIcon(), func() { a.showPlayground(row.Name) })),
		vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { a.confirmDelete(row.Name, false, "") })),
	)
	right := hstack(spaceS, vCenter(statusChips), hSpacer(spaceXS), vCenter(buttons))

	nameBlock := vstack(1, nameLabel, oneLine("model · "+model))
	nameRow := container.NewBorder(nil, nil,
		vCenter(avatar(letter, 38)), vCenter(right), nameBlock)

	loop := row.Loop
	if loop == "" {
		loop = "in-memory loop"
	}
	loopLabel := mono(loop, theme.SizeNameCaptionText)
	loopLabel.Wrapping = fyne.TextWrapOff
	loopLabel.Truncation = fyne.TextTruncateClip
	loopBox := container.NewBorder(nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), fieldLabel("loop")),
		nil, loopLabel)

	caps := row.Capabilities
	var capRow fyne.CanvasObject
	if len(caps) == 0 {
		capRow = caption("deployment-default capabilities")
	} else {
		shown := caps
		extra := 0
		if len(shown) > 4 {
			extra = len(shown) - 4
			shown = shown[:4]
		}
		box := chips(shown, toneAccent).(*fyne.Container)
		if extra > 0 {
			box.Add(vCenter(chip(fmt.Sprintf("+%d", extra), toneNeutral)))
		}
		capRow = box
	}

	meta := container.NewBorder(nil, nil, nil, vCenter(capRow), loopBox)
	body := vstack(spaceS, nameRow, widget.NewSeparator(), meta)
	return panel(body)
}

func emptyAgents(a *App) fyne.CanvasObject {
	return emptyStateCard(
		"No agents yet",
		"Start from a template: an echo agent needs no model.",
		"Create your first agent",
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

	blankUse := primaryButton("Use", theme.NavigateNextIcon(), func() {
		closePopup()
		a.showBuilder("", nil)
	})
	content.Add(subpanel(container.NewBorder(nil, nil, nil,
		vCenter(widthBox(96, blankUse)),
		vstack(spaceXS,
			hstack(spaceS, vCenter(subheading("Blank agent")), vCenter(chip("custom", toneNeutral))),
			muted("Start from an empty loop. No model is selected; choose one in the builder."),
		),
	)))

	for _, tpl := range a.catalog.Templates {
		tpl := tpl
		useBtn := primaryButton("Use", theme.NavigateNextIcon(), func() {
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

	title := insetLeading(titleText("New agent", theme.SizeNameHeadingText))
	closeBtn := iconButton(theme.CancelIcon(), "Close", closePopup)
	header := container.NewBorder(nil, nil, vCenter(title), vCenter(closeBtn), nil)
	subtitle := muted("Start from a template. Everything can be edited in the builder.")
	headerBlock := vstack(spaceXS, header, subtitle)

	cancel := widthBox(104, actionButton("Cancel", nil, closePopup))
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
	msg := fmt.Sprintf("Delete agent %q and its persisted files?", name)
	if force {
		msg = fmt.Sprintf("%s\n\nForce the delete?", reason)
		if reason == "" {
			msg = fmt.Sprintf("Agent %q has live sessions. Stop them and delete it?", name)
		}
	}
	showConfirmPrompt(a.win, "Delete agent", msg, "Delete", true, func(ok bool) {
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
				fyne.Do(func() { a.confirmDelete(name, true, ae.Message) })
				return
			}
			fyne.Do(func() { a.showError(err) })
			return
		}
		fyne.Do(func() {
			a.setStatus("deleted "+name, toneSuccess)
			a.showAgents()
		})
	}()
}
