package ui

import (
	"context"
	"errors"
	"regexp"
	"strconv"
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

type modelsView struct {
	app  *App
	root fyne.CanvasObject

	list   *fyne.Container
	scroll *container.Scroll
	banner *banner
	count  *widget.Label
	meta   *widget.Label
	drift  *fyne.Container
	newBtn *widget.Button

	models []api.Model
	// topology is the runtime's deployment tier, read alongside the model list.
	// A regional fleet applies a hot model edit to one instance only, and the
	// screen has to say so rather than let an operator believe it rolled out.
	topology string
}

var modelNameRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

var modelProviders = []string{"openai", "openai-responses", "anthropic", "gemini", "rerank"}

func (a *App) showModels() {
	a.enter("models")
	a.setMain(a.newModelsView().root)
}

func (a *App) newModelsView() *modelsView {
	v := &modelsView{app: a}
	v.list = vstack(spaceM)
	v.scroll = pageScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	v.newBtn = primaryButton(T("models.newModel"), theme.ContentAddIcon(), func() { v.showEditor(nil) })
	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load)

	head := pageHeader(T("models.title"),
		T("models.subtitle"),
		hstack(spaceS, widthBox(88, v.count), refresh, v.newBtn))
	v.drift = hstack(spaceS,
		actionButton(T("models.saveToConfig"), theme.DocumentSaveIcon(), v.persist),
		actionButton(T("models.revertToConfig"), theme.ViewRefreshIcon(), v.revert))
	v.drift.Hide()

	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta, v.drift)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		scrollInset(v.scroll),
	)
	v.load()
	return v
}

func (v *modelsView) load() {
	v.banner.Clear()
	v.meta.SetText(T("models.loadingState"))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, fileErr, err := v.app.api.Models(ctx)
		state, _ := v.app.api.State(ctx)
		onUI(func() {
			if err != nil {
				v.meta.SetText("")
				v.banner.Set(err.Error(), toneDanger)
				clearBox(v.list)
				v.list.Refresh()
				return
			}
			v.models = models
			v.topology = state.Topology
			v.render(fileErr)
		})
	}()
}

func (v *modelsView) render(fileErr string) {
	clearBox(v.list)
	n := len(v.models)
	if n == 0 {
		v.newBtn.Hide()
	} else {
		v.newBtn.Show()
	}
	v.count.SetText(T("models.count", "n", n))

	drift := 0
	for _, m := range v.models {
		if m.Drift {
			drift++
		}
	}
	// The line is a list of separate facts, each a whole message.
	meta := []string{T("models.allSaved")}
	if drift > 0 {
		meta[0] = T("models.unsavedChanges", "n", drift)
	}
	if fileErr != "" {
		meta = append(meta, T("models.configReadFailed"))
	}
	if v.topology == "regional" {
		meta = append(meta, T("models.regionalNote"))
	}
	v.meta.SetText(strings.Join(meta, " · "))
	if drift > 0 {
		v.drift.Show()
	} else {
		v.drift.Hide()
	}
	if root, ok := v.root.(*fyne.Container); ok {
		root.Refresh()
	}

	if n == 0 {
		v.list.Add(layout.NewSpacer())
		v.list.Add(v.empty())
		v.list.Add(layout.NewSpacer())
		v.list.Refresh()
		v.root.Refresh()
		return
	}
	for _, m := range v.models {
		v.list.Add(v.card(m))
	}
	v.list.Refresh()
	v.root.Refresh()
}

func (v *modelsView) empty() fyne.CanvasObject {
	return emptyStateCard(
		T("models.emptyTitle"),
		T("models.emptyGuidance"),
		T("models.emptyAction"),
		func() { v.showEditor(nil) },
	)
}

func (v *modelsView) card(m api.Model) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(m.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tone, label := toneSuccess, T("models.state.saved")
	switch {
	case m.InRuntime && !m.InFile:
		tone, label = toneWarning, T("models.state.runtimeOnly")
	case !m.InRuntime && m.InFile:
		tone, label = toneWarning, T("models.state.fileOnly")
	case m.Drift:
		tone, label = toneWarning, T("models.state.unsaved")
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(m.Provider, toneAccent)), vCenter(chip(label, tone))),
		hstack(spaceXS,
			vCenter(actionButton(T("common.test"), theme.MediaPlayIcon(), func() { v.test(m.Name) })),
			vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.showEditor(&m) })),
			vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDelete(m) })),
		),
		nil,
	)

	key := chip(T("models.key.notSet"), toneWarning)
	if m.HasKey {
		value := T("models.key.configured")
		if m.KeyFingerprint != "" {
			value = T("models.key.configuredFingerprint", "fingerprint", m.KeyFingerprint)
		}
		key = chip(value, toneSuccess)
	}
	base := m.BaseURL
	if base == "" {
		base = T("models.providerDefault")
	}
	cells := []fyne.CanvasObject{
		detailRow(T("models.fact.model"), detailText(m.Model)),
		detailRow(T("models.fact.apiKey"), key),
	}
	cells = append(cells, detailRow(T("models.fact.baseUrl"), detailText(base)))
	if m.Timeout != "" {
		cells = append(cells, detailRow(T("models.fact.timeout"), detailText(m.Timeout)))
	} else if m.MaxTokens > 0 {
		cells = append(cells, detailRow(T("models.fact.maxTokens"), detailText(strconv.Itoa(m.MaxTokens))))
	}
	details := grid(2, cells...)
	return panel(vstack(spaceS, top, widget.NewSeparator(), details))
}

func (v *modelsView) test(name string) {
	v.app.setStatus(T("models.testing", "name", name), toneAccent)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		res, err := v.app.api.TestModel(ctx, name)
		onUI(func() {
			if err != nil {
				v.app.showError(err)
				return
			}
			if res.OK {
				v.app.setStatus(T("models.testOk", "name", name, "ms", res.LatencyMS), toneSuccess)
				return
			}
			msg := res.Error
			if res.Class != "" {
				msg = res.Class + ": " + msg
			}
			showInfoPrompt(v.app.win, T("models.testFailedTitle"), msg)
		})
	}()
}

func (v *modelsView) confirmDelete(m api.Model) {
	if !m.InRuntime && m.InFile {
		showInfoPrompt(v.app.win, T("models.onlyInConfigTitle"),
			T("models.onlyInConfigMessage"))
		return
	}
	msg := T("models.removeConfirm", "name", m.Name)
	if m.InFile {
		msg = T("models.removeConfirmInFile", "name", m.Name)
	}
	showConfirmPrompt(v.app.win, T("models.deleteTitle"), msg, T("common.delete"), true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := v.app.api.DeleteModel(ctx, m.Name); err != nil {
				onUI(func() { v.app.showError(err) })
				return
			}
			onUI(func() {
				v.app.setStatus(T("models.removed"), toneWarning)
				v.load()
				v.app.reloadCatalog()
			})
		}()
	})
}

func (v *modelsView) persist() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := v.app.api.PersistModels(ctx); err != nil {
			onUI(func() { v.app.showError(err) })
			return
		}
		onUI(func() {
			v.app.setStatus(T("models.savedToConfig"), toneSuccess)
			v.load()
		})
	}()
}

func (v *modelsView) revert() {
	showConfirmPrompt(v.app.win, T("models.revertTitle"),
		T("models.revertMessage"),
		T("common.revert"), true,
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				if err := v.app.api.RevertModels(ctx); err != nil {
					onUI(func() { v.app.showError(err) })
					return
				}
				onUI(func() {
					v.app.setStatus(T("models.reverted"), toneSuccess)
					v.load()
					v.app.reloadCatalog()
				})
			}()
		})
}

func (v *modelsView) showEditor(existing *api.Model) {
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	name := widget.NewEntry()
	name.SetPlaceHolder("assistant-default")
	provider := widget.NewSelect(modelProviders, nil)
	provider.SetSelected("openai")
	modelID := widget.NewEntry()
	modelID.SetPlaceHolder("gpt-4o-mini")
	key := widget.NewPasswordEntry()
	key.SetPlaceHolder("sk-…")
	baseURL := widget.NewEntry()
	baseURL.SetPlaceHolder(T("common.optional"))
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("60s")
	retry := widget.NewEntry()
	retry.SetPlaceHolder("0")
	maxTokens := widget.NewEntry()
	maxTokens.SetPlaceHolder("0")
	// The first option stands for "no level"; its label is translated, so the
	// draft compares against the same string.
	providerDefault := T("models.providerDefault")
	thinking := widget.NewSelect([]string{providerDefault, "off", "low", "medium", "high", "xhigh", "max"}, nil)
	thinking.SetSelected(providerDefault)
	tools := widget.NewEntry()
	tools.SetPlaceHolder("web_search, google_search")

	title := T("models.newModel")
	if existing != nil {
		title = T("models.editNamed", "name", existing.Name)
		name.SetText(existing.Name)
		name.Disable()
		if existing.Provider != "" {
			provider.SetSelected(existing.Provider)
		}
		modelID.SetText(existing.Model)
		key.SetPlaceHolder(T("models.keepStoredKey"))
		baseURL.SetText(existing.BaseURL)
		timeout.SetText(existing.Timeout)
		if existing.Retry > 0 {
			retry.SetText(strconv.Itoa(existing.Retry))
		}
		if existing.MaxTokens > 0 {
			maxTokens.SetText(strconv.Itoa(existing.MaxTokens))
		}
		if existing.Thinking != "" {
			thinking.SetSelected(existing.Thinking)
		}
		tools.SetText(strings.Join(existing.ServerTools, ", "))
	}

	errBanner := newBanner()
	form := vstack(spaceS,
		grid(2,
			labeled(T("common.name"), name),
			labeled(T("models.field.provider"), provider),
			labeled(T("models.field.model"), modelID),
			labeled(T("models.field.apiKey"), key),
		),
		grid(2,
			labeled(T("models.field.baseUrl"), baseURL),
			labeled(T("models.field.timeout"), timeout),
			labeled(T("models.field.retry"), retry),
			labeled(T("models.field.maxTokens"), maxTokens),
		),
		grid(2,
			labeled(T("models.field.thinking"), thinking),
			labeled(T("models.field.serverTools"), tools),
		),
	)
	scroll := container.NewVScroll(container.New(
		layout.NewCustomPaddedLayout(0, 0, 0, spaceS),
		vstack(spaceS, errBanner, form),
	))
	scroll.SetMinSize(fyne.NewSize(620, 360))

	build := func() (api.ModelDraft, error) {
		d := api.ModelDraft{
			Provider: provider.Selected,
			Model:    strings.TrimSpace(modelID.Text),
			APIKey:   strings.TrimSpace(key.Text),
			BaseURL:  strings.TrimSpace(baseURL.Text),
			Timeout:  strings.TrimSpace(timeout.Text),
			Thinking: thinking.Selected,
		}
		if d.Thinking == providerDefault {
			d.Thinking = ""
		}
		d.ServerTools = splitList(tools.Text)
		if n := strings.TrimSpace(name.Text); n == "" || !modelNameRe.MatchString(n) {
			return d, errors.New(T("models.error.name"))
		}
		if d.Provider == "" {
			return d, errors.New(T("models.error.chooseProvider"))
		}
		if d.Model == "" {
			return d, errors.New(T("models.error.modelRequired"))
		}
		if s := strings.TrimSpace(retry.Text); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return d, errors.New(T("models.error.retry"))
			}
			d.Retry = n
		}
		if s := strings.TrimSpace(maxTokens.Text); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return d, errors.New(T("models.error.maxTokens"))
			}
			d.MaxTokens = n
		}
		return d, nil
	}

	saveBtn := primaryButton(T("common.save"), nil, nil)
	saveBtn.OnTapped = func() {
		draft, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		saveBtn.Disable()
		saveBtn.SetText(T("common.saving"))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := v.app.api.PutModel(ctx, strings.TrimSpace(name.Text), draft)
			onUI(func() {
				saveBtn.Enable()
				saveBtn.SetText(T("common.save"))
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				msg := T("models.applied", "name", strings.TrimSpace(name.Text))
				if v.topology == "regional" {
					msg = T("models.appliedRegional", "name", strings.TrimSpace(name.Text))
				}
				v.app.setStatus(msg, toneWarning)
				v.load()
				v.app.reloadCatalog()
			})
		}()
	}

	var testBtn fyne.CanvasObject
	if existing != nil {
		testBtn = actionButton(T("models.testLive"), theme.MediaPlayIcon(), func() { v.test(existing.Name) })
	}
	cancel := widthBox(104, actionButton(T("common.cancel"), nil, closePopup))
	footer := container.NewBorder(nil, nil, testBtn, hstack(spaceS, cancel, widthBox(104, saveBtn)), nil)

	head := container.NewBorder(nil, nil,
		insetLeading(titleText(title, theme.SizeNameHeadingText)),
		vCenter(iconButton(theme.CancelIcon(), T("common.close"), closePopup)), nil)
	subtitle := muted(T("models.editorSubtitle"))
	body := vstack(spaceL, vstack(spaceXS, head, subtitle), scroll, footer)
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(18, 16, 20, 20), body))
	popup = widget.NewModalPopUp(card, v.app.win.Canvas())
	popup.Show()
}
