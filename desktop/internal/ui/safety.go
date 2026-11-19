package ui

import (
	"context"
	"errors"
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
	v.scroll = pageScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load)

	head := pageHeader(T("safety.title"), T("safety.subtitle"),
		hstack(spaceS, widthBox(150, v.count), refresh))
	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		scrollInset(v.scroll),
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
	v.meta.SetText(T("common.loading"))
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
					msg = T("safety.apiMissing")
				}
				v.setBanner(msg, toneDanger)
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon(T("common.retry"), theme.ViewRefreshIcon(), v.load)))
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
	v.count.SetText(T("safety.profileCount", "n", nProfiles))
	meta := T("safety.classifierCount", "n", nClassifiers)
	if v.data.RestartRequired {
		meta += " · " + T("safety.restartRequired")
	} else {
		meta += " · " + T("safety.live")
	}
	v.meta.SetText(meta)
	if !v.data.Editable {
		note := v.data.Note
		if note == "" {
			note = T("safety.readOnlyBanner")
		}
		v.setBanner(note, toneWarning)
	}

	newProfile := func() { v.editProfile(nil) }
	newClassifier := func() { v.editClassifier(nil) }
	profileAction := iconButton(theme.ContentAddIcon(), T("safety.newProfile"), newProfile)
	classifierAction := iconButton(theme.ContentAddIcon(), T("safety.newClassifier"), newClassifier)
	profileCTA := actionButton(T("safety.newProfile"), theme.ContentAddIcon(), newProfile)
	classifierCTA := actionButton(T("safety.newClassifier"), theme.ContentAddIcon(), newClassifier)
	if !v.data.Editable {
		profileAction.Disable()
		classifierAction.Disable()
		profileCTA.Disable()
		classifierCTA.Disable()
	}

	v.profileSection = vstack(spaceM)
	for _, p := range v.data.Profiles {
		v.profileSection.Add(v.profileCard(p))
	}
	if nProfiles == 0 {
		v.profileSection.Add(subpanel(vstack(spaceS,
			muted(T("safety.noProfiles")),
			hstack(spaceS, widthBox(160, profileCTA)),
		)))
	}

	v.classifierSection = vstack(spaceM)
	for _, c := range v.data.Classifiers {
		v.classifierSection.Add(v.classifierCard(c))
	}
	if nClassifiers == 0 {
		v.classifierSection.Add(subpanel(vstack(spaceS,
			muted(T("safety.noClassifiers")),
			hstack(spaceS, widthBox(172, classifierCTA)),
		)))
	}

	v.list.Add(safetySectionHeader(T("safety.profiles"), T("safety.profilesSubtitle"), profileAction))
	v.list.Add(v.profileSection)
	v.list.Add(safetySectionHeader(T("safety.classifiers"), T("safety.classifiersSubtitle"), classifierAction))
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
	tone, label := toneWarning, T("safety.restartRequired")
	if p.Live {
		tone, label = toneSuccess, T("safety.live")
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(T("safety.filterCount", "n", len(p.Filters)), toneAccent)), vCenter(chip(label, tone))),
		hstack(spaceXS,
			vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.editProfile(&p) })),
			vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDeleteProfile(p) })),
		),
		nil,
	)

	chain := wrapLabel(chainText(p.Filters))
	chain.TextStyle = fyne.TextStyle{Monospace: true}
	chain.SizeName = theme.SizeNameCaptionText

	used := T("safety.usedBy.none")
	if len(p.Agents) > 0 || len(p.SpawnProfiles) > 0 {
		parts := []string{}
		if len(p.Agents) > 0 {
			parts = append(parts, T("safety.usedBy.agents", "list", strings.Join(p.Agents, ", ")))
		}
		if len(p.SpawnProfiles) > 0 {
			parts = append(parts, T("safety.usedBy.spawn", "list", strings.Join(p.SpawnProfiles, ", ")))
		}
		used = strings.Join(parts, " · ")
	}
	cells := []fyne.CanvasObject{
		detailRow(T("safety.row.chain"), chain),
		detailRow(T("safety.row.usedBy"), detailText(used)),
	}
	if len(p.Classifiers) > 0 {
		cells = append(cells, detailRow(T("safety.row.classifiers"), detailText(strings.Join(p.Classifiers, ", "))))
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(cells...)))
}

func (v *safetyView) classifierCard(c api.SafetyClassifier) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(c.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tone, label := toneWarning, T("safety.restartRequired")
	if c.Live {
		tone, label = toneSuccess, T("safety.live")
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
			vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.editClassifier(&c) })),
			vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDeleteClassifier(c) })),
		),
		nil,
	)

	threshold := T("safety.thresholdDefault")
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
	used := T("safety.unused")
	if len(c.UsedBy) > 0 {
		used = strings.Join(c.UsedBy, ", ")
	}
	cells := []fyne.CanvasObject{
		detailRow(T("safety.row.model"), detailText(c.Model)),
		detailRow(T("safety.row.threshold"), detailText(threshold)),
		detailRow(T("safety.row.directions"), detailText(directions)),
		detailRow(T("safety.row.sources"), detailText(sources)),
		detailRow(T("safety.row.categories"), detailText(categorySummary(c.Categories))),
		detailRow(T("safety.row.usedBy"), detailText(used)),
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), grid(2, cells...)))
}

func chainText(filters []string) string {
	if len(filters) == 0 {
		return T("safety.emptyChain")
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
		showInfoPrompt(v.app.win, T("safety.readOnlyTitle"), v.readOnlyNote())
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
		vstack(spaceXS, fieldLabel(T("common.name")), name,
			caption(T("safety.profile.nameHint"))),
		vstack(spaceXS, fieldLabel(T("safety.profile.chain")), filters,
			caption(T("safety.profile.chainHintText"))),
		vstack(spaceXS, fieldLabel(T("safety.profile.preview")), subpanel(preview)),
		vstack(spaceXS, fieldLabel(T("safety.profile.available")), quick),
		caption(T("safety.profile.previewHint")),
	)
	scroll := container.NewVScroll(panel(form))
	scroll.SetMinSize(fyne.NewSize(760, 480))

	build := func() (api.SafetyProfileDraft, error) {
		d := api.SafetyProfileDraft{Name: strings.TrimSpace(name.Text), Filters: splitList(filters.Text)}
		if d.Name == "" {
			return d, errors.New(T("safety.err.profileNameRequired"))
		}
		return d, nil
	}

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	validate := actionButton(T("safety.validate"), checkMarkIcon, nil)
	save := primaryButton(T("common.save"), theme.UploadIcon(), nil)
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
					errBanner.Set(T("safety.profile.validText"), toneSuccess)
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
					note = T("safety.profileSaved")
				}
				v.app.setStatus(note, toneWarning)
			})
		}()
	}

	title := T("safety.profile.newTitle")
	if existing != nil {
		title = T("safety.editWhat", "name", existing.Name)
	}
	popup = showModalCard(v.app.win, title, T("safety.profile.subtitle"),
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *safetyView) confirmDeleteProfile(p api.SafetyProfile) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, T("safety.readOnlyTitle"), v.readOnlyNote())
		return
	}
	msg := T("safety.deleteProfileMessage", "name", p.Name)
	if len(p.Agents) > 0 || len(p.SpawnProfiles) > 0 {
		msg = T("safety.deleteProfileMessageInUse", "name", p.Name, "refs", profileUsage(p))
	}
	showConfirmPrompt(v.app.win, T("safety.deleteProfileTitle"), msg, T("common.delete"), true, func(ok bool) {
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
				v.app.setStatus(T("safety.profileDeleted"), toneWarning)
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
	return T("safety.readOnlyMessage")
}

// ---- classifier editor -----------------------------------------------------

func (v *safetyView) editClassifier(existing *api.SafetyClassifier) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, T("safety.readOnlyTitle"), v.readOnlyNote())
		return
	}
	creating := existing == nil
	name := widget.NewEntry()
	name.SetPlaceHolder("moderation")
	modelOpts := append([]string{}, v.data.Models...)
	// The first prompt option stands for no prompt (the embedded default);
	// its label is read once so build always recognises it.
	embeddedDefault := T("safety.classifier.embeddedDefault")
	promptOpts := append([]string{embeddedDefault}, v.data.Prompts...)
	model := widget.NewSelect(modelOpts, nil)
	prompt := widget.NewSelect(promptOpts, nil)
	prompt.SetSelected(embeddedDefault)
	mode := widget.NewSelect([]string{"enforce", "shadow"}, nil)
	mode.SetSelected("enforce")
	ingress := widget.NewCheck(T("safety.classifier.ingress"), nil)
	egress := widget.NewCheck(T("safety.classifier.egress"), nil)
	ingress.SetChecked(true)
	egress.SetChecked(true)
	srcChannel := widget.NewCheck("channel", nil)
	srcAgent := widget.NewCheck("agent", nil)
	srcScheduler := widget.NewCheck("scheduler", nil)
	srcSystem := widget.NewCheck("system", nil)
	srcChannel.SetChecked(true)
	srcAgent.SetChecked(true)
	threshold := widget.NewEntry()
	threshold.SetPlaceHolder(T("safety.classifier.thresholdPlaceholder"))
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
	refusal.SetPlaceHolder(T("safety.classifier.refusalPlaceholder"))
	fallback := widget.NewEntry()
	fallback.SetPlaceHolder(T("safety.classifier.fallbackPlaceholder"))

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
		grid(2,
			field(T("common.name"), name),
			field(T("safety.classifier.model"), model),
		),
		grid(2,
			field(T("safety.classifier.mode"), mode),
			field(T("safety.classifier.prompt"), prompt),
		),
		vstack(spaceXS, fieldLabel(T("safety.classifier.directions")), hstack(spaceM, ingress, egress)),
		vstack(spaceXS, fieldLabel(T("safety.classifier.sources")), hstack(spaceM, srcChannel, srcAgent, srcScheduler, srcSystem)),
		grid(2,
			field(T("safety.classifier.threshold"), threshold),
			field(T("safety.classifier.timeout"), timeout),
			field(T("safety.classifier.onError"), onError),
			field(T("safety.classifier.onSaturated"), onSaturated),
			field(T("safety.classifier.onBudget"), onBudget),
			field(T("safety.classifier.maxTokens"), maxTokens),
			field(T("safety.classifier.maxInputChars"), maxInput),
			field(T("safety.classifier.maxConcurrency"), maxConcurrency),
			field(T("safety.classifier.cacheSize"), cacheSize),
			field(T("safety.classifier.cacheTTL"), cacheTTL),
			field(T("safety.classifier.budget"), budget),
		),
		vstack(spaceXS, fieldLabel(T("safety.classifier.categories")), categories,
			caption(T("safety.classifier.categoriesHint"))),
		grid(2,
			field(T("safety.classifier.refusal"), refusal),
			field(T("safety.classifier.fallback"), fallback),
		),
	)
	if len(modelOpts) == 0 {
		form.Add(caption(T("safety.classifier.noModels")))
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
			return d, errors.New(T("safety.err.classifierNameRequired"))
		}
		if d.Model == "" {
			return d, errors.New(T("safety.err.chooseModel"))
		}
		if prompt.Selected != "" && prompt.Selected != embeddedDefault {
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
			return d, errors.New(T("safety.err.chooseDirection"))
		}
		if len(d.Sources) == 0 {
			return d, errors.New(T("safety.err.chooseSource"))
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
		if d.MaxTokens, err = optionalCount(T("safety.classifier.maxTokens"), maxTokens.Text); err != nil {
			return d, err
		}
		if d.MaxInputChars, err = optionalCount(T("safety.classifier.maxInputChars"), maxInput.Text); err != nil {
			return d, err
		}
		if d.MaxConcurrency, err = optionalCount(T("safety.classifier.maxConcurrency"), maxConcurrency.Text); err != nil {
			return d, err
		}
		if d.Cache.Size, err = optionalCount(T("safety.classifier.cacheSize"), cacheSize.Text); err != nil {
			return d, err
		}
		d.Cache.TTL = strings.TrimSpace(cacheTTL.Text)
		if d.Budget.TokensPerDay, err = optionalCount64(T("safety.classifier.budget"), budget.Text); err != nil {
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
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	validate := actionButton(T("safety.validate"), checkMarkIcon, nil)
	save := primaryButton(T("common.save"), theme.UploadIcon(), nil)
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
					errBanner.Set(T("safety.classifier.validText", "name", d.Name), toneSuccess)
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
					note = T("safety.classifierSaved")
				}
				v.app.setStatus(note, toneWarning)
			})
		}()
	}

	title := T("safety.classifier.newTitle")
	if existing != nil {
		title = T("safety.editWhat", "name", existing.Name)
	}
	popup = showModalCard(v.app.win, title, T("safety.classifier.subtitle"),
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *safetyView) confirmDeleteClassifier(c api.SafetyClassifier) {
	if !v.data.Editable {
		showInfoPrompt(v.app.win, T("safety.readOnlyTitle"), v.readOnlyNote())
		return
	}
	msg := T("safety.deleteClassifierMessage", "name", c.Name)
	if len(c.UsedBy) > 0 {
		msg = T("safety.deleteClassifierMessageInUse", "name", c.Name, "n", len(c.UsedBy), "profiles", strings.Join(c.UsedBy, ", "))
	}
	showConfirmPrompt(v.app.win, T("safety.deleteClassifierTitle"), msg, T("common.delete"), true, func(ok bool) {
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
				v.app.setStatus(T("safety.classifierDeleted"), toneWarning)
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
		return nil, errors.New(T("safety.err.thresholdRange"))
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
			return nil, errors.New(T("safety.err.categoriesNoColon", "line", strconv.Quote(line)))
		}
		label = strings.TrimSpace(label)
		action = strings.ToLower(strings.TrimSpace(action))
		if label == "" {
			return nil, errors.New(T("safety.err.categoriesEmptyLabel", "line", strconv.Quote(line)))
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

// optionalCount parses an optional count: blank is 0 (unset), otherwise a
// non-negative integer. label is the field's name, already translated.
func optionalCount(label, raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, errors.New(T("safety.err.nonNegative", "what", label))
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
		return 0, errors.New(T("safety.err.nonNegative", "what", label))
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
