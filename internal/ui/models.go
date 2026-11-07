package ui

import (
	"context"
	"fmt"
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
}

var modelNameRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

var modelProviders = []string{"openai", "openai-responses", "anthropic", "gemini", "rerank"}

func (a *App) showModels() {
	a.enter("models")
	a.setMain(a.newModelsView().root)
}

func (a *App) newModelsView() *modelsView {
	v := &modelsView{app: a}
	v.list = vstack(spaceS)
	v.scroll = container.NewVScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	v.newBtn = primaryButton("New model", theme.ContentAddIcon(), func() { v.showEditor(nil) })
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", v.load)

	head := pageHeader("Models",
		"LLM endpoints available to agents. Changes apply live; save them to the deployment config to keep them across restarts.",
		hstack(spaceS, widthBox(88, v.count), refresh, v.newBtn))
	v.drift = hstack(spaceS,
		actionButton("Save to config", theme.DocumentSaveIcon(), v.persist),
		actionButton("Revert to config", theme.ViewRefreshIcon(), v.revert))
	v.drift.Hide()

	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta, v.drift)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(v.scroll),
	)
	v.load()
	return v
}

func (v *modelsView) load() {
	v.banner.Clear()
	v.meta.SetText("loading…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, fileErr, err := v.app.api.Models(ctx)
		fyne.Do(func() {
			if err != nil {
				v.meta.SetText("")
				v.banner.Set(err.Error(), toneDanger)
				clearBox(v.list)
				v.list.Refresh()
				return
			}
			v.models = models
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
	if n == 1 {
		v.count.SetText("1 model")
	} else {
		v.count.SetText(fmt.Sprintf("%d models", n))
	}

	drift := 0
	for _, m := range v.models {
		if m.Drift {
			drift++
		}
	}
	meta := "all changes saved to config"
	if drift == 1 {
		meta = "1 unsaved model change"
	} else if drift > 1 {
		meta = fmt.Sprintf("%d unsaved model changes", drift)
	}
	if fileErr != "" {
		meta += " · config read failed"
	}
	v.meta.SetText(meta)
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
		"No models yet",
		"Add an OpenAI, Anthropic or Gemini endpoint to give agents a model.",
		"Add a model",
		func() { v.showEditor(nil) },
	)
}

func (v *modelsView) card(m api.Model) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(m.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tone, label := toneSuccess, "saved"
	switch {
	case m.InRuntime && !m.InFile:
		tone, label = toneWarning, "runtime only"
	case !m.InRuntime && m.InFile:
		tone, label = toneWarning, "file only"
	case m.Drift:
		tone, label = toneWarning, "unsaved"
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(m.Provider, toneAccent)), vCenter(chip(label, tone))),
		hstack(spaceXS,
			vCenter(actionButton("Test", theme.MediaPlayIcon(), func() { v.test(m.Name) })),
			vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.showEditor(&m) })),
			vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDelete(m) })),
		),
		nil,
	)

	key := chip("not set", toneWarning)
	if m.HasKey {
		value := "configured"
		if m.KeyFingerprint != "" {
			value += " · " + m.KeyFingerprint
		}
		key = chip(value, toneSuccess)
	}
	base := m.BaseURL
	if base == "" {
		base = "provider default"
	}
	row := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, widthBox(104, fieldLabel(label)), nil, value)
	}
	cells := []fyne.CanvasObject{
		row("model", monoInline(m.Model)),
		row("api key", key),
	}
	cells = append(cells, row("base url", monoInline(base)))
	if m.Timeout != "" {
		cells = append(cells, row("timeout", widget.NewLabel(m.Timeout)))
	} else if m.MaxTokens > 0 {
		cells = append(cells, row("max tokens", widget.NewLabel(strconv.Itoa(m.MaxTokens))))
	}
	details := container.NewGridWithColumns(2, cells...)
	return panel(vstack(spaceS, top, widget.NewSeparator(), details))
}

func (v *modelsView) test(name string) {
	v.app.setStatus("testing "+name+"…", toneAccent)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		res, err := v.app.api.TestModel(ctx, name)
		fyne.Do(func() {
			if err != nil {
				v.app.showError(err)
				return
			}
			if res.OK {
				v.app.setStatus(fmt.Sprintf("%s ok · %dms", name, res.LatencyMS), toneSuccess)
				return
			}
			msg := res.Error
			if res.Class != "" {
				msg = res.Class + ": " + msg
			}
			showInfoPrompt(v.app.win, "Model test failed", msg)
		})
	}()
}

func (v *modelsView) confirmDelete(m api.Model) {
	if !m.InRuntime && m.InFile {
		showInfoPrompt(v.app.win, "Model is only in the config",
			"This model is not live on the running instance. Save the live model set to config to remove it, or revert to config to load it.")
		return
	}
	msg := fmt.Sprintf("Remove model %q from the running instance?", m.Name)
	if m.InFile {
		msg += "\n\nIt stays in the config file until you save changes to the config."
	}
	showConfirmPrompt(v.app.win, "Delete model", msg, "Delete", true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := v.app.api.DeleteModel(ctx, m.Name); err != nil {
				fyne.Do(func() { v.app.showError(err) })
				return
			}
			fyne.Do(func() {
				v.app.setStatus("model removed · live", toneWarning)
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
			fyne.Do(func() { v.app.showError(err) })
			return
		}
		fyne.Do(func() {
			v.app.setStatus("models saved to config", toneSuccess)
			v.load()
		})
	}()
}

func (v *modelsView) revert() {
	showConfirmPrompt(v.app.win, "Revert model changes?",
		"Discard live model changes and reload the deployment config's models?",
		"Revert", true,
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				if err := v.app.api.RevertModels(ctx); err != nil {
					fyne.Do(func() { v.app.showError(err) })
					return
				}
				fyne.Do(func() {
					v.app.setStatus("models reverted to config", toneSuccess)
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
	baseURL.SetPlaceHolder("optional")
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("60s")
	retry := widget.NewEntry()
	retry.SetPlaceHolder("0")
	maxTokens := widget.NewEntry()
	maxTokens.SetPlaceHolder("0")
	thinking := widget.NewSelect([]string{"provider default", "off", "low", "medium", "high", "xhigh", "max"}, nil)
	thinking.SetSelected("provider default")
	tools := widget.NewEntry()
	tools.SetPlaceHolder("web_search, google_search")

	title := "New model"
	if existing != nil {
		title = "Edit " + existing.Name
		name.SetText(existing.Name)
		name.Disable()
		if existing.Provider != "" {
			provider.SetSelected(existing.Provider)
		}
		modelID.SetText(existing.Model)
		key.SetPlaceHolder("leave blank to keep the stored key")
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
		container.NewGridWithColumns(2,
			labeled("Name", name),
			labeled("Provider", provider),
			labeled("Model", modelID),
			labeled("API key", key),
		),
		container.NewGridWithColumns(2,
			labeled("Base URL", baseURL),
			labeled("Timeout", timeout),
			labeled("Retry", retry),
			labeled("Max tokens", maxTokens),
		),
		container.NewGridWithColumns(2,
			labeled("Thinking", thinking),
			labeled("Server tools", tools),
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
		if d.Thinking == "provider default" {
			d.Thinking = ""
		}
		d.ServerTools = splitList(tools.Text)
		if n := strings.TrimSpace(name.Text); n == "" || !modelNameRe.MatchString(n) {
			return d, fmt.Errorf("name must be 1-64 of letters, digits, _ . : -")
		}
		if d.Provider == "" {
			return d, fmt.Errorf("choose a provider")
		}
		if d.Model == "" {
			return d, fmt.Errorf("model is required")
		}
		if s := strings.TrimSpace(retry.Text); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return d, fmt.Errorf("retry must be a non-negative number")
			}
			d.Retry = n
		}
		if s := strings.TrimSpace(maxTokens.Text); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return d, fmt.Errorf("max tokens must be a non-negative number")
			}
			d.MaxTokens = n
		}
		return d, nil
	}

	saveBtn := primaryButton("Save", nil, nil)
	saveBtn.OnTapped = func() {
		draft, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		saveBtn.Disable()
		saveBtn.SetText("Saving…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := v.app.api.PutModel(ctx, strings.TrimSpace(name.Text), draft)
			fyne.Do(func() {
				saveBtn.Enable()
				saveBtn.SetText("Save")
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.app.setStatus("model "+strings.TrimSpace(name.Text)+" applied · save to config to keep", toneWarning)
				v.load()
				v.app.reloadCatalog()
			})
		}()
	}

	var testBtn fyne.CanvasObject
	if existing != nil {
		testBtn = actionButton("Test live", theme.MediaPlayIcon(), func() { v.test(existing.Name) })
	}
	cancel := widthBox(104, actionButton("Cancel", nil, closePopup))
	footer := container.NewBorder(nil, nil, testBtn, hstack(spaceS, cancel, widthBox(104, saveBtn)), nil)

	head := container.NewBorder(nil, nil,
		insetLeading(titleText(title, theme.SizeNameHeadingText)),
		vCenter(iconButton(theme.CancelIcon(), "Close", closePopup)), nil)
	subtitle := muted("Secrets are write-only: leave the API key blank to keep the stored value.")
	body := vstack(spaceL, vstack(spaceXS, head, subtitle), scroll, footer)
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(18, 16, 20, 20), body))
	popup = widget.NewModalPopUp(card, v.app.win.Canvas())
	popup.Show()
}
