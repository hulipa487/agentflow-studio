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

type safetyView struct {
	app  *App
	root fyne.CanvasObject

	list   *fyne.Container
	scroll *container.Scroll
	banner *banner
	count  *widget.Label
	meta   *widget.Label

	profileSection    *fyne.Container
	classifierSection *fyne.Container

	data    api.SafetyList
	loadSeq int
}

func (a *App) showSafety() {
	a.enter("safety")
	a.setMain(a.newSafetyView().root)
}

func (a *App) newSafetyView() *safetyView {
	v := &safetyView{app: a}
	v.list = vstack(spaceM)
	v.scroll = container.NewVScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", v.load)

	head := pageHeader("Safety",
		"Core-owned ingress and egress filter chains, plus the model-backed classifiers they can call. Saves go to the deployment config and bind on the next runtime start.",
		hstack(spaceS, widthBox(150, v.count), refresh))
	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(v.scroll),
	)
	v.load()
	return v
}

func (v *safetyView) setBanner(text string, t tone) {
	v.banner.Set(text, t)
	if v.root != nil {
		v.root.Refresh()
	}
}

func (v *safetyView) load() {
	v.banner.Clear()
	v.meta.SetText("loading…")
	v.loadSeq++
	seq := v.loadSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list, err := v.app.api.Safety(ctx)
		onUI(func() {
			if seq != v.loadSeq {
				return
			}
			if err != nil {
				v.meta.SetText("")
				msg := err.Error()
				if ae, ok := err.(*api.Error); ok && ae.Status == 404 {
					msg = "This runtime does not expose the Studio safety API yet; rebuild and restart AgentFlow, then reopen Safety."
				}
				v.setBanner(msg, toneDanger)
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), v.load)))
				v.list.Refresh()
				return
			}
			v.data = list
			v.render()
		})
	}()
}

func (v *safetyView) render() {
	v.banner.Clear()
	clearBox(v.list)
	nProfiles := len(v.data.Profiles)
	nClassifiers := len(v.data.Classifiers)
	if nProfiles == 1 {
		v.count.SetText("1 profile")
	} else {
		v.count.SetText(fmt.Sprintf("%d profiles", nProfiles))
	}
	classifiersLabel := fmt.Sprintf("%d classifiers", nClassifiers)
	if nClassifiers == 1 {
		classifiersLabel = "1 classifier"
	}
	meta := classifiersLabel
	if v.data.RestartRequired {
		meta += " · restart required"
	} else {
		meta += " · live"
	}
	v.meta.SetText(meta)
	if !v.data.Editable {
		note := v.data.Note
		if note == "" {
			note = "This deployment has no editable config source for safety profiles."
		}
		v.setBanner(note, toneWarning)
	}

	newProfile := func() { v.editProfile(nil) }
	newClassifier := func() { v.editClassifier(nil) }
	profileAction := iconButton(theme.ContentAddIcon(), "New profile", newProfile)
	classifierAction := iconButton(theme.ContentAddIcon(), "New classifier", newClassifier)
	profileCTA := actionButton("New profile", theme.ContentAddIcon(), newProfile)
	classifierCTA := actionButton("New classifier", theme.ContentAddIcon(), newClassifier)
	if !v.data.Editable {
		profileAction.Disable()
		classifierAction.Disable()
		profileCTA.Disable()
		classifierCTA.Disable()
	}

	v.profileSection = vstack(spaceS)
	for _, p := range v.data.Profiles {
		v.profileSection.Add(v.profileCard(p))
	}
	if nProfiles == 0 {
		v.profileSection.Add(subpanel(vstack(spaceS,
			muted("No named safety profiles. Agents can still use the built-in \"default\" chain or \"none\"."),
			hstack(spaceS, widthBox(160, profileCTA)),
		)))
	}

	v.classifierSection = vstack(spaceS)
	for _, c := range v.data.Classifiers {
		v.classifierSection.Add(v.classifierCard(c))
	}
	if nClassifiers == 0 {
		v.classifierSection.Add(subpanel(vstack(spaceS,
			muted("No model-backed classifiers yet. Add one to enable classifier:<name> filters in a profile."),
			hstack(spaceS, widthBox(172, classifierCTA)),
		)))
	}

	v.list.Add(safetySectionHeader("Filter profiles",
		"The chain runs in order. Builtins are compiled in; classifier:<name> entries call a model-backed policy.",
		profileAction))
	v.list.Add(v.profileSection)
	v.list.Add(safetySectionHeader("Model-backed classifiers",
		"A dedicated engine-keyed model returns a label and score; the policy here maps labels to allow, review, or block.",
		classifierAction))
	v.list.Add(v.classifierSection)
	v.list.Add(layout.NewSpacer())
	v.list.Refresh()
	if v.root != nil {
		v.root.Refresh()
	}
}

func safetySectionHeader(title, subtitle string, action fyne.CanvasObject) fyne.CanvasObject {
	desc := muted(subtitle)
	desc.Wrapping = fyne.TextWrapWord
	left := vstack(2, subheading(title), compactLabel(desc, true))
	if action == nil {
		return left
	}
	return container.NewBorder(nil, nil, nil, vCenter(action), left)
}

func (v *safetyView) profileCard(p api.SafetyProfile) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tone, label := toneWarning, "restart required"
	if p.Live {
		tone, label = toneSuccess, "live"
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(fmt.Sprintf("%d filters", len(p.Filters)), toneAccent)), vCenter(chip(label, tone))),
		hstack(spaceXS,
			vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.editProfile(&p) })),
			vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDeleteProfile(p) })),
		),
		nil,
	)

	chain := wrapLabel(chainText(p.Filters))
	chain.TextStyle = fyne.TextStyle{Monospace: true}
	chain.SizeName = theme.SizeNameCaptionText

	row := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, widthBox(108, fieldLabel(label)), nil, value)
	}
	used := "not referenced by an agent yet"
	if len(p.Agents) > 0 || len(p.SpawnProfiles) > 0 {
		parts := []string{}
		if len(p.Agents) > 0 {
			parts = append(parts, "agents: "+strings.Join(p.Agents, ", "))
		}
		if len(p.SpawnProfiles) > 0 {
			parts = append(parts, "spawn: "+strings.Join(p.SpawnProfiles, ", "))
		}
		used = strings.Join(parts, " · ")
	}
	cells := []fyne.CanvasObject{
		row("chain", chain),
		row("used by", widget.NewLabel(used)),
	}
	if len(p.Classifiers) > 0 {
		cells = append(cells, row("classifiers", monoInline(strings.Join(p.Classifiers, ", "))))
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(cells...)))
}

func (v *safetyView) classifierCard(c api.SafetyClassifier) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(c.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tone, label := toneWarning, "restart required"
	if c.Live {
		tone, label = toneSuccess, "live"
	}
	mode := c.Mode
	if mode == "" {
		mode = "enforce"
	}
	modeTone := toneSuccess
	if mode == "shadow" {
		modeTone = toneWarning
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(c.Model, toneAccent)), vCenter(chip(mode, modeTone)), vCenter(chip(label, tone))),
		hstack(spaceXS,
			vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.editClassifier(&c) })),
			vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDeleteClassifier(c) })),
		),
		nil,
	)

	row := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, widthBox(108, fieldLabel(label)), nil, value)
	}
	threshold := "0.7 (default)"
	if c.Threshold != nil {
		threshold = strconv.FormatFloat(*c.Threshold, 'f', -1, 64)
	}
	directions := "ingress + egress"
	if len(c.Directions) > 0 {
		directions = strings.Join(c.Directions, ", ")
	}
	sources := "channel + agent"
	if len(c.Sources) > 0 {
		sources = strings.Join(c.Sources, ", ")
	}
	used := "unused"
	if len(c.UsedBy) > 0 {
		used = strings.Join(c.UsedBy, ", ")
	}
	cells := []fyne.CanvasObject{
		row("model", widget.NewLabel(c.Model)),
		row("threshold", widget.NewLabel(threshold)),
		row("directions", widget.NewLabel(directions)),
		row("sources", widget.NewLabel(sources)),
		row("categories", monoInline(categorySummary(c.Categories))),
		row("used by", widget.NewLabel(used)),
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewGridWithColumns(2, cells...)))
}

func chainText(filters []string) string {
	if len(filters) == 0 {
		return "empty chain — the dispatcher runs and decides nothing"
	}
	return strings.Join(filters, "  →  ")
}

func categorySummary(categories map[string]string) string {
	if len(categories) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+categories[k])
	}
	return strings.Join(parts, ", ")
}

// ---- profile editor --------------------------------------------------------

func (v *safetyView) editProfile(existing *api.SafetyProfile) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, "Safety is read-only", v.readOnlyNote())
		return
	}
	creating := existing == nil
	name := widget.NewEntry()
	name.SetPlaceHolder("strict")
	filters := widget.NewEntry()
	filters.SetPlaceHolder("source-attribution, affect-guard")
	preview := wrapLabel("")
	preview.TextStyle = fyne.TextStyle{Monospace: true}
	preview.SizeName = theme.SizeNameCaptionText
	syncPreview := func() { preview.SetText(chainText(splitList(filters.Text))) }
	filters.OnChanged = func(string) { syncPreview() }

	available := append([]string{}, v.data.BuiltinFilters...)
	for _, c := range v.data.Classifiers {
		available = append(available, "classifier:"+c.Name)
	}
	add := func(f string) {
		parts := splitList(filters.Text)
		for _, p := range parts {
			if p == f {
				return
			}
		}
		parts = append(parts, f)
		filters.SetText(strings.Join(parts, ", "))
		syncPreview()
	}
	quick := container.NewVBox()
	row := hstack(spaceXS)
	for i, f := range available {
		b := actionButton(f, nil, func() { add(f) })
		row.Add(widthBox(210, b))
		if i%3 == 2 || i == len(available)-1 {
			quick.Add(row)
			row = hstack(spaceXS)
		}
	}

	if existing != nil {
		name.SetText(existing.Name)
		name.Disable()
		filters.SetText(strings.Join(existing.Filters, ", "))
	} else {
		filters.SetText("source-attribution, affect-guard")
	}
	syncPreview()

	errBanner := newBanner()
	form := vstack(spaceM,
		vstack(spaceXS, fieldLabel("Name"), name,
			caption("Immutable after creation. \"default\" and \"none\" are reserved references.")),
		vstack(spaceXS, fieldLabel("Filter chain"), filters,
			caption("Comma-separated, left to right. Click a name below to append it.")),
		vstack(spaceXS, fieldLabel("Preview"), subpanel(preview)),
		vstack(spaceXS, fieldLabel("Available filters"), quick),
		caption("Classifier filters need a saved classifier below; builtin names come from the running runtime."),
	)
	scroll := container.NewVScroll(panel(form))
	scroll.SetMinSize(fyne.NewSize(760, 480))

	build := func() (api.SafetyProfileDraft, error) {
		d := api.SafetyProfileDraft{Name: strings.TrimSpace(name.Text), Filters: splitList(filters.Text)}
		if d.Name == "" {
			return d, fmt.Errorf("a profile name is required")
		}
		return d, nil
	}

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	validate := actionButton("Validate", checkMarkIcon, nil)
	save := primaryButton("Save", theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil,
		hstack(spaceS, cancel, widthBox(108, controlFrame(validate)), widthBox(104, controlFrame(save))), nil)
	busy := false
	setBusy := func(b bool) {
		busy = b
		if b {
			validate.Disable()
			save.Disable()
		} else {
			validate.Enable()
			save.Enable()
		}
	}

	validate.OnTapped = func() {
		if busy {
			return
		}
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		setBusy(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			errs, err := v.app.api.ValidateSafety(ctx, api.SafetyValidateRequest{
				Kind: "profile", Creating: creating, Profile: &d,
			})
			onUI(func() {
				setBusy(false)
				switch {
				case err != nil:
					errBanner.Set(err.Error(), toneDanger)
				case len(errs) > 0:
					errBanner.Set(strings.Join(errs, "\n"), toneDanger)
				default:
					errBanner.Set("Valid. Save it to bind the chain on the next runtime start.", toneSuccess)
				}
			})
		}()
	}
	save.OnTapped = func() {
		if busy {
			return
		}
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		setBusy(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			list, err := v.app.api.SaveSafetyProfile(ctx, d.Name, creating, d, v.data.MTime)
			onUI(func() {
				setBusy(false)
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.data = list
				v.render()
				v.app.reloadCatalog()
				note := list.SavedNote
				if note == "" {
					note = "safety profile saved · restart required"
				}
				v.app.setStatus(note, toneWarning)
			})
		}()
	}

	title := "New safety profile"
	if existing != nil {
		title = "Edit " + existing.Name
	}
	popup = showModalCard(v.app.win, title,
		"Profiles are core-owned: a loop cannot uninstall the chain. Changes bind when agentflow starts.",
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *safetyView) confirmDeleteProfile(p api.SafetyProfile) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, "Safety is read-only", v.readOnlyNote())
		return
	}
	msg := fmt.Sprintf("Delete safety profile %q from the deployment config?", p.Name)
	if len(p.Agents) > 0 || len(p.SpawnProfiles) > 0 {
		msg += "\n\nIt is still referenced by " + profileUsage(p) + "; remove those references first."
	}
	msg += "\n\nThe running instance keeps its current chain until the next restart."
	showConfirmPrompt(v.app.win, "Delete safety profile", msg, "Delete", true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			list, err := v.app.api.DeleteSafetyProfile(ctx, p.Name, v.data.MTime)
			onUI(func() {
				if err != nil {
					v.setBanner(err.Error(), toneDanger)
					return
				}
				v.data = list
				v.render()
				v.app.reloadCatalog()
				v.app.setStatus("safety profile deleted · restart required", toneWarning)
			})
		}()
	})
}

func profileUsage(p api.SafetyProfile) string {
	parts := []string{}
	if len(p.Agents) > 0 {
		parts = append(parts, strings.Join(p.Agents, ", "))
	}
	if len(p.SpawnProfiles) > 0 {
		parts = append(parts, strings.Join(p.SpawnProfiles, ", "))
	}
	return strings.Join(parts, ", ")
}

func (v *safetyView) readOnlyNote() string {
	if v.data.Note != "" {
		return v.data.Note
	}
	return "This deployment has no editable config source, so safety profiles cannot be changed from Studio."
}

// ---- classifier editor -----------------------------------------------------

func (v *safetyView) editClassifier(existing *api.SafetyClassifier) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, "Safety is read-only", v.readOnlyNote())
		return
	}
	creating := existing == nil
	name := widget.NewEntry()
	name.SetPlaceHolder("moderation")
	modelOpts := append([]string{}, v.data.Models...)
	promptOpts := append([]string{"embedded default"}, v.data.Prompts...)
	model := widget.NewSelect(modelOpts, nil)
	prompt := widget.NewSelect(promptOpts, nil)
	prompt.SetSelected("embedded default")
	mode := widget.NewSelect([]string{"enforce", "shadow"}, nil)
	mode.SetSelected("enforce")
	ingress := widget.NewCheck("Ingress (inbound messages)", nil)
	egress := widget.NewCheck("Egress (outbound replies)", nil)
	ingress.SetChecked(true)
	egress.SetChecked(true)
	srcChannel := widget.NewCheck("channel", nil)
	srcAgent := widget.NewCheck("agent", nil)
	srcScheduler := widget.NewCheck("scheduler", nil)
	srcSystem := widget.NewCheck("system", nil)
	srcChannel.SetChecked(true)
	srcAgent.SetChecked(true)
	threshold := widget.NewEntry()
	threshold.SetPlaceHolder("0.7 (blank = runtime default)")
	categories := widget.NewMultiLineEntry()
	categories.SetPlaceHolder("safe: allow\nself_harm: block\nharassment: review")
	categories.SetMinRowsVisible(4)
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("2s")
	onError := widget.NewSelect([]string{"block", "pass"}, nil)
	onSaturated := widget.NewSelect([]string{"block", "pass"}, nil)
	onBudget := widget.NewSelect([]string{"block", "pass"}, nil)
	onError.SetSelected("block")
	onSaturated.SetSelected("block")
	onBudget.SetSelected("block")
	maxTokens := widget.NewEntry()
	maxTokens.SetPlaceHolder("64")
	maxInput := widget.NewEntry()
	maxInput.SetPlaceHolder("8000")
	maxConcurrency := widget.NewEntry()
	maxConcurrency.SetPlaceHolder("8")
	cacheSize := widget.NewEntry()
	cacheSize.SetPlaceHolder("10000")
	cacheTTL := widget.NewEntry()
	cacheTTL.SetPlaceHolder("5m")
	budget := widget.NewEntry()
	budget.SetPlaceHolder("500000")
	refusal := widget.NewEntry()
	refusal.SetPlaceHolder("optional ingress refusal text")
	fallback := widget.NewEntry()
	fallback.SetPlaceHolder("optional egress fallback text")

	if existing != nil {
		name.SetText(existing.Name)
		name.Disable()
		if existing.Model != "" {
			if !hasString(modelOpts, existing.Model) {
				modelOpts = append(modelOpts, existing.Model)
				model.Options = modelOpts
				model.Refresh()
			}
			model.SetSelected(existing.Model)
		}
		if existing.Prompt != "" {
			if !hasString(promptOpts, existing.Prompt) {
				promptOpts = append(promptOpts, existing.Prompt)
				prompt.Options = promptOpts
				prompt.Refresh()
			}
			prompt.SetSelected(existing.Prompt)
		}
		if existing.Mode != "" {
			mode.SetSelected(existing.Mode)
		}
		ingress.SetChecked(hasString(existing.Directions, "ingress") || len(existing.Directions) == 0)
		egress.SetChecked(hasString(existing.Directions, "egress") || len(existing.Directions) == 0)
		setSourceChecks(existing.Sources, srcChannel, srcAgent, srcScheduler, srcSystem)
		if existing.Threshold != nil {
			threshold.SetText(strconv.FormatFloat(*existing.Threshold, 'f', -1, 64))
		}
		categories.SetText(formatCategories(existing.Categories))
		timeout.SetText(existing.Timeout)
		if existing.OnError != "" {
			onError.SetSelected(existing.OnError)
		}
		if existing.OnSaturated != "" {
			onSaturated.SetSelected(existing.OnSaturated)
		}
		if existing.OnBudget != "" {
			onBudget.SetSelected(existing.OnBudget)
		}
		setIntEntry(maxTokens, existing.MaxTokens)
		setIntEntry(maxInput, existing.MaxInputChars)
		setIntEntry(maxConcurrency, existing.MaxConcurrency)
		setIntEntry(cacheSize, existing.Cache.Size)
		cacheTTL.SetText(existing.Cache.TTL)
		setInt64Entry(budget, existing.Budget.TokensPerDay)
		refusal.SetText(existing.Refusal)
		fallback.SetText(existing.Fallback)
	} else if len(modelOpts) > 0 {
		model.SetSelected(modelOpts[0])
	}

	errBanner := newBanner()
	field := func(label string, control fyne.CanvasObject) fyne.CanvasObject {
		return vstack(spaceXS, fieldLabel(label), controlFrame(control))
	}
	form := vstack(spaceM,
		container.NewGridWithColumns(2,
			field("Name", name),
			field("Model", model),
		),
		container.NewGridWithColumns(2,
			field("Mode", mode),
			field("Prompt", prompt),
		),
		vstack(spaceXS, fieldLabel("Directions"), hstack(spaceM, ingress, egress)),
		vstack(spaceXS, fieldLabel("Sources"), hstack(spaceM, srcChannel, srcAgent, srcScheduler, srcSystem)),
		container.NewGridWithColumns(2,
			field("Threshold", threshold),
			field("Timeout", timeout),
			field("On error", onError),
			field("On saturated", onSaturated),
			field("On budget exhausted", onBudget),
			field("Max tokens", maxTokens),
			field("Max input chars", maxInput),
			field("Max concurrency", maxConcurrency),
			field("Cache size", cacheSize),
			field("Cache TTL", cacheTTL),
			field("Daily token budget", budget),
		),
		vstack(spaceXS, fieldLabel("Categories"), categories,
			caption("One per line, label: allow|review|block. At least one allow label is required.")),
		container.NewGridWithColumns(2,
			field("Refusal (ingress block)", refusal),
			field("Fallback (egress block)", fallback),
		),
	)
	if len(modelOpts) == 0 {
		form.Add(caption("No engine-keyed classifier models are configured. Add one on the Models screen and save it to config first."))
	}
	scroll := container.NewVScroll(panel(form))
	scroll.SetMinSize(fyne.NewSize(780, 560))

	build := func() (api.SafetyClassifierDraft, error) {
		d := api.SafetyClassifierDraft{
			Name:  strings.TrimSpace(name.Text),
			Model: model.Selected,
			Mode:  mode.Selected,
		}
		if d.Name == "" {
			return d, fmt.Errorf("a classifier name is required")
		}
		if d.Model == "" {
			return d, fmt.Errorf("choose a classifier model")
		}
		if prompt.Selected != "" && prompt.Selected != "embedded default" {
			d.Prompt = prompt.Selected
		}
		if ingress.Checked {
			d.Directions = append(d.Directions, "ingress")
		}
		if egress.Checked {
			d.Directions = append(d.Directions, "egress")
		}
		if srcChannel.Checked {
			d.Sources = append(d.Sources, "channel")
		}
		if srcAgent.Checked {
			d.Sources = append(d.Sources, "agent")
		}
		if srcScheduler.Checked {
			d.Sources = append(d.Sources, "scheduler")
		}
		if srcSystem.Checked {
			d.Sources = append(d.Sources, "system")
		}
		if len(d.Directions) == 0 {
			return d, fmt.Errorf("choose at least one direction")
		}
		if len(d.Sources) == 0 {
			return d, fmt.Errorf("choose at least one source")
		}
		var err error
		if d.Threshold, err = parseOptionalThreshold(threshold.Text); err != nil {
			return d, err
		}
		if d.Categories, err = parseCategoriesEntry(categories.Text); err != nil {
			return d, err
		}
		d.Timeout = strings.TrimSpace(timeout.Text)
		d.OnError = onError.Selected
		d.OnSaturated = onSaturated.Selected
		d.OnBudget = onBudget.Selected
		if d.MaxTokens, err = optionalCount("max tokens", maxTokens.Text); err != nil {
			return d, err
		}
		if d.MaxInputChars, err = optionalCount("max input chars", maxInput.Text); err != nil {
			return d, err
		}
		if d.MaxConcurrency, err = optionalCount("max concurrency", maxConcurrency.Text); err != nil {
			return d, err
		}
		if d.Cache.Size, err = optionalCount("cache size", cacheSize.Text); err != nil {
			return d, err
		}
		d.Cache.TTL = strings.TrimSpace(cacheTTL.Text)
		if d.Budget.TokensPerDay, err = optionalCount64("daily token budget", budget.Text); err != nil {
			return d, err
		}
		d.Refusal = strings.TrimSpace(refusal.Text)
		d.Fallback = strings.TrimSpace(fallback.Text)
		return d, nil
	}

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	validate := actionButton("Validate", checkMarkIcon, nil)
	save := primaryButton("Save", theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil,
		hstack(spaceS, cancel, widthBox(108, controlFrame(validate)), widthBox(104, controlFrame(save))), nil)
	busy := false
	setBusy := func(b bool) {
		busy = b
		if b {
			validate.Disable()
			save.Disable()
		} else {
			validate.Enable()
			save.Enable()
		}
	}

	validate.OnTapped = func() {
		if busy {
			return
		}
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		setBusy(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			errs, err := v.app.api.ValidateSafety(ctx, api.SafetyValidateRequest{
				Kind: "classifier", Creating: creating, Classifier: &d,
			})
			onUI(func() {
				setBusy(false)
				switch {
				case err != nil:
					errBanner.Set(err.Error(), toneDanger)
				case len(errs) > 0:
					errBanner.Set(strings.Join(errs, "\n"), toneDanger)
				default:
					errBanner.Set("Valid. Save it, then reference classifier:"+d.Name+" from a profile.", toneSuccess)
				}
			})
		}()
	}
	save.OnTapped = func() {
		if busy {
			return
		}
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		setBusy(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			list, err := v.app.api.SaveSafetyClassifier(ctx, d.Name, creating, d, v.data.MTime)
			onUI(func() {
				setBusy(false)
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.data = list
				v.render()
				v.app.reloadCatalog()
				note := list.SavedNote
				if note == "" {
					note = "safety classifier saved · restart required"
				}
				v.app.setStatus(note, toneWarning)
			})
		}()
	}

	title := "New classifier"
	if existing != nil {
		title = "Edit " + existing.Name
	}
	popup = showModalCard(v.app.win, title,
		"Classifier spend is metered under __moderation_<name>; refusals and fallbacks are rate-limited per session.",
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *safetyView) confirmDeleteClassifier(c api.SafetyClassifier) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, "Safety is read-only", v.readOnlyNote())
		return
	}
	msg := fmt.Sprintf("Delete classifier %q from the deployment config?", c.Name)
	if len(c.UsedBy) > 0 {
		msg += "\n\nIt is referenced by profile(s): " + strings.Join(c.UsedBy, ", ") + ". Remove those references first."
	}
	msg += "\n\nThe running instance keeps using it until the next restart."
	showConfirmPrompt(v.app.win, "Delete classifier", msg, "Delete", true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			list, err := v.app.api.DeleteSafetyClassifier(ctx, c.Name, v.data.MTime)
			onUI(func() {
				if err != nil {
					v.setBanner(err.Error(), toneDanger)
					return
				}
				v.data = list
				v.render()
				v.app.reloadCatalog()
				v.app.setStatus("safety classifier deleted · restart required", toneWarning)
			})
		}()
	})
}

// ---- parsing helpers -------------------------------------------------------

func parseOptionalThreshold(raw string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 1 {
		return nil, fmt.Errorf("threshold must be a number between 0 and 1")
	}
	return &f, nil
}

func parseCategoriesEntry(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		label, action, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("categories: %q needs label: action", line)
		}
		label = strings.TrimSpace(label)
		action = strings.ToLower(strings.TrimSpace(action))
		if label == "" {
			return nil, fmt.Errorf("categories: %q has an empty label", line)
		}
		out[label] = action
	}
	return out, nil
}

func formatCategories(categories map[string]string) string {
	if len(categories) == 0 {
		return ""
	}
	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+categories[k])
	}
	return strings.Join(lines, "\n")
}

func optionalCount(label, raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", label)
	}
	return n, nil
}

func optionalCount64(label, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", label)
	}
	return n, nil
}

func setIntEntry(e *widget.Entry, n int) {
	if n > 0 {
		e.SetText(strconv.Itoa(n))
	}
}

func setInt64Entry(e *widget.Entry, n int64) {
	if n > 0 {
		e.SetText(strconv.FormatInt(n, 10))
	}
}

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func setSourceChecks(sources []string, channel, agent, scheduler, system *widget.Check) {
	if len(sources) == 0 {
		channel.SetChecked(true)
		agent.SetChecked(true)
		return
	}
	channel.SetChecked(hasString(sources, "channel"))
	agent.SetChecked(hasString(sources, "agent"))
	scheduler.SetChecked(hasString(sources, "scheduler"))
	system.SetChecked(hasString(sources, "system"))
}
