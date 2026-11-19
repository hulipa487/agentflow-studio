package ui

import (
	"context"
	"errors"
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

// memoryView is the Memory screen: the backends memory lives in
// (memory.backends) and the named profiles agents pick (profiles.memory). It
// is the desktop twin of the web app's screens/Memory.tsx; the pure helpers
// at the bottom match screens/memory/helpers.ts.
type memoryView struct {
	app  *App
	root fyne.CanvasObject

	list   *fyne.Container
	scroll *container.Scroll
	banner *banner
	count  *widget.Label
	meta   *widget.Label

	data    api.MemoryList
	loadSeq int
}

func (a *App) showMemory() {
	a.enter("memory")
	a.setMain(a.newMemoryView().root)
}

func (a *App) newMemoryView() *memoryView {
	v := &memoryView{app: a}
	v.list = vstack(spaceM)
	v.scroll = pageScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load)
	guide := actionButton(T("memory.guide"), theme.HelpIcon(), func() { a.showDocs("memory") })
	head := pageHeader(T("memory.title"), T("memory.subtitle"),
		hstack(spaceS, widthBox(150, v.count), guide, refresh))
	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		scrollInset(v.scroll),
	)
	v.load()
	return v
}

func (v *memoryView) setBanner(text string, t tone) {
	v.banner.Set(text, t)
	if v.root != nil {
		v.root.Refresh()
	}
}

func (v *memoryView) load() {
	v.banner.Clear()
	v.meta.SetText(T("common.loading"))
	v.loadSeq++
	seq := v.loadSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list, err := v.app.api.Memory(ctx)
		onUI(func() {
			if seq != v.loadSeq {
				return
			}
			if err != nil {
				v.meta.SetText("")
				msg := err.Error()
				if ae, ok := err.(*api.Error); ok && ae.Status == 404 {
					msg = T("memory.apiMissing")
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

func (v *memoryView) render() {
	v.banner.Clear()
	clearBox(v.list)
	v.count.SetText(T("memory.profileCount", "n", len(v.data.Profiles)))
	meta := T("memory.backendCount", "n", len(v.data.Backends))
	if v.data.RestartRequired {
		meta += " · " + T("safety.restartRequired")
	} else {
		meta += " · " + T("safety.live")
	}
	v.meta.SetText(meta)
	switch {
	case !v.data.Editable:
		note := v.data.Note
		if note == "" {
			note = T("memory.readOnlyBanner")
		}
		v.setBanner(note, toneWarning)
	case v.data.RestartRequired && v.data.SavedNote != "":
		v.setBanner(T("safety.restartRequiredNote", "note", v.data.SavedNote), toneWarning)
	}

	profileAction := iconButton(theme.ContentAddIcon(), T("memory.newProfile"), func() { v.editProfile(nil) })
	backendAction := iconButton(theme.ContentAddIcon(), T("memory.newBackend"), func() { v.editBackend(nil) })
	if !v.data.Editable {
		profileAction.Disable()
		backendAction.Disable()
	}

	profileSub := T("memory.profilesSubtitle")
	if len(v.data.BuiltinAgents) > 0 {
		profileSub = T("memory.profilesSubtitleBuiltin", "agents", strings.Join(v.data.BuiltinAgents, ", "))
	}
	v.list.Add(safetySectionHeader(T("memory.profiles"), profileSub, profileAction))
	profiles := vstack(spaceM)
	for _, p := range v.data.Profiles {
		profiles.Add(v.profileCard(p))
	}
	if len(v.data.Profiles) == 0 {
		profiles.Add(subpanel(muted(T("memory.noProfiles"))))
	}
	v.list.Add(profiles)

	v.list.Add(safetySectionHeader(T("memory.backends"), T("memory.backendsSubtitle"), backendAction))
	backends := vstack(spaceM)
	for _, b := range v.data.Backends {
		backends.Add(v.backendCard(b))
	}
	v.list.Add(backends)
	v.list.Add(layout.NewSpacer())
	v.list.Refresh()
	if v.root != nil {
		v.root.Refresh()
	}
}

func liveChip(live bool) fyne.CanvasObject {
	if live {
		return chip(T("safety.live"), toneSuccess)
	}
	return chip(T("safety.restartRequired"), toneWarning)
}

func (v *memoryView) profileCard(p api.MemoryProfile) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	chips := []fyne.CanvasObject{vCenter(title), vCenter(chip(T("memory.storeCount", "n", len(p.Stores)), toneAccent))}
	if p.Recall != "" {
		chips = append(chips, vCenter(chip(p.Recall, toneNeutral)))
	}
	chips = append(chips, vCenter(liveChip(p.Live)))
	del := iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDelete("profile", p.Name) })
	if memoryProfileInUse(p) {
		del.Disable()
	}
	top := container.NewBorder(nil, nil, hstack(spaceS, chips...),
		hstack(spaceXS,
			vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.editProfile(&p) })),
			vCenter(del),
		), nil)
	rows := []fyne.CanvasObject{}
	for _, s := range p.Stores {
		rows = append(rows, detailRow(s.Name, detailText(storeSummary(s))))
	}
	if p.EmbedModel != "" {
		e := p.EmbedModel
		if p.RerankModel != "" {
			e = T("memory.embeddingRerank", "embed", p.EmbedModel, "rerank", p.RerankModel)
		}
		rows = append(rows, detailRow(T("memory.row.embedding"), detailText(e)))
	}
	rows = append(rows, detailRow(T("memory.row.usedBy"), detailText(memoryProfileUsedBy(p))))
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(rows...)))
}

func (v *memoryView) backendCard(b api.MemoryBackend) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(b.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	state := liveChip(b.Live)
	if b.Implicit {
		state = chip(T("memory.builtinChip"), toneNeutral)
	}
	actions := []fyne.CanvasObject{vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.editBackend(&b) }))}
	if !b.Implicit {
		del := iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDelete("backend", b.Name) })
		if backendInUse(b, v.data.BuiltinAgents) {
			del.Disable()
		}
		actions = append(actions, vCenter(del))
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(b.Provider, toneAccent)), vCenter(state)),
		hstack(spaceXS, actions...), nil)
	used := backendUsedBy(b, v.data.BuiltinAgents)
	if b.Implicit && len(v.data.BuiltinAgents) > 0 {
		used = T("memory.builtinProfileUsedBy", "agents", strings.Join(v.data.BuiltinAgents, ", "))
	}
	rows := []fyne.CanvasObject{detailRow(T("memory.row.target"), detailText(backendTarget(b)))}
	if b.Dim > 0 {
		rows = append(rows, detailRow(T("memory.row.dimensions"), detailText(strconv.Itoa(b.Dim))))
	}
	rows = append(rows, detailRow(T("memory.row.usedBy"), detailText(used)))
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(rows...)))
}

func (v *memoryView) readOnly() bool {
	if v.data.Editable {
		return false
	}
	note := v.data.Note
	if note == "" {
		note = T("memory.readOnlyMessage")
	}
	showInfoPrompt(v.app.win, T("memory.readOnlyTitle"), note)
	return true
}

func (v *memoryView) confirmDelete(kind, name string) {
	if v.readOnly() {
		return
	}
	title, msg := T("memory.deleteProfileTitle"), T("memory.deleteProfileMessage", "name", name)
	deleted := T("memory.profileDeleted")
	if kind == "backend" {
		title, msg = T("memory.deleteBackendTitle"), T("memory.deleteBackendMessage", "name", name)
		deleted = T("memory.backendDeleted")
	}
	showConfirmPrompt(v.app.win, title, msg, T("common.delete"), true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var list api.MemoryList
			var err error
			if kind == "profile" {
				list, err = v.app.api.DeleteMemoryProfile(ctx, name, v.data.MTime)
			} else {
				list, err = v.app.api.DeleteMemoryBackend(ctx, name, v.data.MTime)
			}
			onUI(func() {
				if err != nil {
					v.setBanner(err.Error(), toneDanger)
					return
				}
				v.applied(list, deleted)
			})
		}()
	})
}

// applied shows the list a write returned. The catalog carries the profile
// names the builder offers, so it is reloaded too.
func (v *memoryView) applied(list api.MemoryList, fallback string) {
	v.data = list
	v.render()
	v.app.reloadCatalog()
	note := list.SavedNote
	if note == "" {
		note = fallback
	}
	v.app.setStatus(note, toneWarning)
}

// memoryEditor runs the validate / save flow both dialogs share. build turns
// the form into a draft or says why it cannot.
func memoryEditor[D any](v *memoryView, title, subtitle string, body fyne.CanvasObject, size fyne.Size,
	build func() (D, error),
	validate func(context.Context, D) ([]string, error),
	save func(context.Context, D) (api.MemoryList, error),
	validText, savedText string,
) {
	errBanner := newBanner()
	scroll := container.NewVScroll(panel(body))
	scroll.SetMinSize(size)

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	validateBtn := actionButton(T("safety.validate"), checkMarkIcon, nil)
	saveBtn := primaryButton(T("common.save"), theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil,
		hstack(spaceS, cancel, widthBox(108, controlFrame(validateBtn)), widthBox(104, controlFrame(saveBtn))), nil)
	busy := false
	setBusy := func(b bool) {
		busy = b
		if b {
			validateBtn.Disable()
			saveBtn.Disable()
		} else {
			validateBtn.Enable()
			saveBtn.Enable()
		}
	}
	run := func(work func(ctx context.Context, d D)) {
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
			work(ctx, d)
		}()
	}
	validateBtn.OnTapped = func() {
		run(func(ctx context.Context, d D) {
			errs, err := validate(ctx, d)
			onUI(func() {
				setBusy(false)
				switch {
				case err != nil:
					errBanner.Set(err.Error(), toneDanger)
				case len(errs) > 0:
					errBanner.Set(strings.Join(errs, "\n"), toneDanger)
				default:
					errBanner.Set(validText, toneSuccess)
				}
			})
		})
	}
	saveBtn.OnTapped = func() {
		run(func(ctx context.Context, d D) {
			list, err := save(ctx, d)
			onUI(func() {
				setBusy(false)
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.applied(list, savedText)
			})
		})
	}
	popup = showModalCard(v.app.win, title, subtitle, container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

// ---- backend editor --------------------------------------------------------

func (v *memoryView) editBackend(existing *api.MemoryBackend) {
	if v.readOnly() {
		return
	}
	creating := existing == nil || existing.Implicit
	name := monoEntry(false)
	name.SetPlaceHolder("vectors")
	provider := widget.NewSelect(v.data.Providers, nil)
	providerHint := caption("")
	path := monoEntry(false)
	path.SetPlaceHolder("./data/memory.db")
	urlEntry := widget.NewPasswordEntry()
	urlEntry.SetPlaceHolder("${PG_URL}")
	apiKey := widget.NewPasswordEntry()
	apiKey.SetPlaceHolder("cred:qdrant")
	clearKey := widget.NewCheck(T("memory.backend.clearApiKey"), nil)
	collection := monoEntry(false)
	collection.SetPlaceHolder("agentflow")
	// The first option stands for no distance (the driver default); its
	// label is read once, so a later language change cannot unmatch it.
	distanceDefault := T("memory.backend.distanceDefault")
	distance := widget.NewSelect(append([]string{distanceDefault}, qdrantDistances...), nil)
	distance.SetSelected(distanceDefault)
	timeout := monoEntry(false)
	timeout.SetPlaceHolder("5s")
	dim := monoEntry(false)
	dim.SetPlaceHolder("1536")
	scan := monoEntry(false)
	scan.SetPlaceHolder("50000")
	maxEntries := monoEntry(false)
	maxEntries.SetPlaceHolder("10000")

	provider.SetSelected("sqlite")
	if existing != nil {
		name.SetText(existing.Name)
		name.Disable()
		provider.SetSelected(existing.Provider)
		path.SetText(existing.Path)
		collection.SetText(existing.Collection)
		if existing.Distance != "" {
			distance.SetSelected(existing.Distance)
		}
		timeout.SetText(existing.Timeout)
		setIntEntry(dim, existing.Dim)
		setIntEntry(scan, existing.VectorScanLimit)
		setIntEntry(maxEntries, existing.MaxEntries)
	}

	field := func(label, hint string, control fyne.CanvasObject) fyne.CanvasObject {
		box := vstack(spaceXS, fieldLabel(label), controlFrame(control))
		if hint != "" {
			c := caption(hint)
			c.Wrapping = fyne.TextWrapWord
			box.Add(c)
		}
		return box
	}
	fields := vstack(spaceM)
	render := func() {
		clearBox(fields)
		p := provider.Selected
		providerHint.SetText(providerHintText(p))
		sameProvider := existing != nil && !existing.Implicit && existing.Provider == p
		has := func(k string) bool { return providerHas(p, k) }
		if has("path") {
			fields.Add(field(T("memory.backend.path"), T("memory.backend.pathHint"), path))
		}
		if has("url") {
			hint := T("memory.backend.urlRequired")
			if sameProvider {
				hint = storedSecretHint(existing.HasURL, existing.URLRef, existing.URLDisplay)
			}
			fields.Add(field(T("memory.backend.url"), T("memory.backend.urlHint", "stored", hint), urlEntry))
		}
		if has("api_key") {
			hint := T("memory.backend.optional")
			if sameProvider {
				hint = storedSecretHint(existing.HasAPIKey, existing.APIKeyRef, "")
			}
			box := field(T("memory.backend.apiKey"), hint, apiKey)
			if sameProvider && existing.HasAPIKey {
				box.(*fyne.Container).Add(compactCheck(clearKey))
			}
			fields.Add(box)
		}
		cells := []fyne.CanvasObject{}
		if has("collection") {
			cells = append(cells, field(T("memory.backend.collection"), T("memory.backend.collectionHint"), collection))
		}
		if has("distance") {
			cells = append(cells, field(T("memory.backend.distance"), "", distance))
		}
		if has("timeout") {
			cells = append(cells, field(T("memory.backend.timeout"), T("memory.backend.timeoutHint"), timeout))
		}
		if has("dim") {
			cells = append(cells, field(T("memory.backend.dimensions"), T("memory.backend.dimensionsHint"), dim))
		}
		if has("vector_scan_limit") {
			cells = append(cells, field(T("memory.backend.vectorScanLimit"), T("memory.backend.vectorScanLimitHint"), scan))
		}
		if has("max_entries") {
			cells = append(cells, field(T("memory.backend.maxEntries"), T("memory.backend.maxEntriesHint"), maxEntries))
		}
		if len(cells) > 0 {
			fields.Add(grid(2, cells...))
		}
		if !sameProvider && existing != nil && !existing.Implicit {
			fields.Add(caption(T("memory.backend.providerChanged")))
		}
		if existing != nil && len(existing.OtherKeys) > 0 {
			fields.Add(caption(T("memory.backend.otherKeysPlain", "keys", strings.Join(existing.OtherKeys, ", "))))
		}
		fields.Refresh()
	}
	provider.OnChanged = func(string) { render() }
	render()
	providerHint.Wrapping = fyne.TextWrapWord

	body := vstack(spaceM,
		grid(2,
			field(T("common.name"), T("memory.backend.nameHint"), name),
			field(T("memory.backend.provider"), "", provider),
		),
		providerHint,
		fields,
	)
	build := func() (api.MemoryBackendDraft, error) {
		dist := distance.Selected
		if dist == distanceDefault {
			dist = ""
		}
		return buildBackendDraft(backendInput{
			Name: name.Text, Provider: provider.Selected, Path: path.Text, URL: urlEntry.Text,
			APIKey: apiKey.Text, ClearAPIKey: clearKey.Checked, Collection: collection.Text,
			Distance: dist, Timeout: timeout.Text, Dim: dim.Text, VectorScanLimit: scan.Text, MaxEntries: maxEntries.Text,
		})
	}
	title := T("memory.backend.newTitle")
	if existing != nil {
		title = T("memory.editWhat", "name", existing.Name)
		if existing.Implicit {
			title = T("memory.backend.writeOutTitle")
		}
	}
	memoryEditor(v, title, T("memory.backend.subtitle"),
		body, fyne.NewSize(640, 420), build,
		func(ctx context.Context, d api.MemoryBackendDraft) ([]string, error) {
			return v.app.api.ValidateMemory(ctx, api.MemoryValidateRequest{Kind: "backend", Creating: creating, Backend: &d})
		},
		func(ctx context.Context, d api.MemoryBackendDraft) (api.MemoryList, error) {
			return v.app.api.SaveMemoryBackend(ctx, d.Name, creating, d, v.data.MTime)
		},
		T("memory.backend.validText"),
		T("memory.backendSaved"))
}

// ---- profile editor --------------------------------------------------------

type storeWidgets struct {
	row       fyne.CanvasObject
	name      *widget.Entry
	backend   *widget.Select
	table     *widget.Entry
	retention *widget.Entry
	window    *widget.Entry
	requires  map[string]*widget.Check
	scope     *widget.Select
	pool      *widget.Entry
	locality  *widget.Check
}

func (v *memoryView) editProfile(existing *api.MemoryProfile) {
	if v.readOnly() {
		return
	}
	creating := existing == nil
	backendNames := make([]string, 0, len(v.data.Backends))
	for _, b := range v.data.Backends {
		backendNames = append(backendNames, b.Name)
	}
	requires := v.data.Requires
	if len(requires) == 0 {
		requires = []string{"kv", "text_search", "vector"}
	}

	name := monoEntry(false)
	name.SetPlaceHolder("group_chat")
	recall := widget.NewSelectEntry(v.data.RecallPlugins)
	recall.SetPlaceHolder("plugin:recency")
	write := widget.NewSelectEntry(v.data.WritePlugins)
	write.SetPlaceHolder("plugin:routing_table")
	// Labels read once per dialog: the options and the checks in build must
	// agree even if the language changes while it is open.
	none := T("common.none")
	scopeUser, scopeAgent := T("memory.scopeUser"), T("memory.scopeAgent")
	embed := widget.NewSelect(append([]string{none}, v.data.Models...), nil)
	rerank := widget.NewSelect(append([]string{none}, v.data.RerankModels...), nil)
	oversample := monoEntry(false)
	oversample.SetPlaceHolder("4")

	draft := newProfileDraft(backendNames)
	if existing != nil {
		draft = existing.MemoryProfileDraft
		name.SetText(existing.Name)
		name.Disable()
	}
	recall.SetText(draft.Recall)
	write.SetText(strings.Join(draft.Write, ", "))
	selectOrAdd(embed, draft.EmbedModel, none)
	selectOrAdd(rerank, draft.RerankModel, none)
	setIntEntry(oversample, draft.Oversample)

	storesBox := vstack(spaceS)
	var stores []*storeWidgets
	var addStore func(s api.MemoryStoreDraft)
	addStore = func(s api.MemoryStoreDraft) {
		w := &storeWidgets{
			name: monoEntry(false), backend: widget.NewSelect(backendNames, nil), table: monoEntry(false),
			retention: monoEntry(false), window: monoEntry(false), requires: map[string]*widget.Check{},
			scope: widget.NewSelect([]string{scopeUser, scopeAgent}, nil), pool: monoEntry(false),
			locality: widget.NewCheck(T("memory.store.localityOk"), nil),
		}
		w.name.SetPlaceHolder(T("memory.col.store"))
		w.table.SetPlaceHolder(T("memory.col.table"))
		w.retention.SetPlaceHolder(T("memory.col.retention"))
		w.window.SetPlaceHolder(T("memory.col.window"))
		w.pool.SetPlaceHolder(T("memory.store.poolPlaceholder"))
		w.name.SetText(s.Name)
		selectOrAdd(w.backend, s.Backend, "")
		w.table.SetText(s.Table)
		w.retention.SetText(s.Retention)
		setIntEntry(w.window, s.Window)
		w.scope.SetSelected(scopeUser)
		if s.Scope == "agent" {
			w.scope.SetSelected(scopeAgent)
		}
		w.pool.SetText(s.Pool)
		w.locality.SetChecked(s.LocalityOK)
		checks := []fyne.CanvasObject{}
		for _, r := range requires {
			c := widget.NewCheck(r, nil)
			c.SetChecked(hasString(s.Requires, r))
			w.requires[r] = c
			checks = append(checks, compactCheck(c))
		}
		remove := iconButton(theme.ContentClearIcon(), T("memory.store.remove"), nil)
		w.row = panel(vstack(spaceXS,
			container.NewBorder(nil, nil, nil, remove, grid(5,
				controlFrame(w.name), controlFrame(w.backend), controlFrame(w.table), controlFrame(w.retention), controlFrame(w.window))),
			hstack(spaceM, append(checks, widthBox(170, controlFrame(w.scope)), widthBox(150, controlFrame(w.pool)), compactCheck(w.locality))...),
		))
		remove.OnTapped = func() {
			for i, x := range stores {
				if x == w {
					stores = append(stores[:i], stores[i+1:]...)
					break
				}
			}
			storesBox.Remove(w.row)
			storesBox.Refresh()
		}
		stores = append(stores, w)
		storesBox.Add(w.row)
		storesBox.Refresh()
	}
	for _, s := range draft.Stores {
		addStore(s)
	}
	firstBackend := ""
	if len(backendNames) > 0 {
		firstBackend = backendNames[0]
	}
	addBtn := actionButton(T("memory.store.add"), theme.ContentAddIcon(), func() { addStore(api.MemoryStoreDraft{Backend: firstBackend}) })

	field := func(label, hint string, control fyne.CanvasObject) fyne.CanvasObject {
		box := vstack(spaceXS, fieldLabel(label), controlFrame(control))
		if hint != "" {
			c := caption(hint)
			c.Wrapping = fyne.TextWrapWord
			box.Add(c)
		}
		return box
	}
	header := container.NewBorder(nil, nil, nil, widthBox(36, layout.NewSpacer()), grid(5,
		caption(T("memory.col.store")), caption(T("memory.col.backend")), caption(T("memory.col.table")),
		caption(T("memory.col.retention")), caption(T("memory.col.window"))))
	body := vstack(spaceM,
		field(T("common.name"), T("memory.profile.nameHint"), name),
		vstack(spaceXS, fieldLabel(T("memory.profile.stores")), header, storesBox, hstack(spaceS, widthBox(130, controlFrame(addBtn))),
			caption(T("memory.profile.storesHint"))),
		grid(2,
			field(T("memory.profile.recall"), T("memory.profile.recallHint"), recall),
			field(T("memory.profile.write"), T("memory.profile.writeHint"), write),
			field(T("memory.profile.embedModel"), T("memory.profile.embedHintAlways"), embed),
			field(T("memory.profile.rerankModel"), T("memory.profile.rerankHint"), rerank),
			field(T("memory.profile.oversample"), T("memory.profile.oversampleHintAlways"), oversample),
		),
	)
	build := func() (api.MemoryProfileDraft, error) {
		in := profileInput{Name: name.Text, Recall: recall.Text, Write: write.Text, Oversample: oversample.Text}
		if embed.Selected != none {
			in.EmbedModel = embed.Selected
		}
		if rerank.Selected != none {
			in.RerankModel = rerank.Selected
		}
		for _, w := range stores {
			s := storeInput{
				Name: w.name.Text, Backend: w.backend.Selected, Table: w.table.Text, Retention: w.retention.Text,
				Window: w.window.Text, Pool: w.pool.Text, LocalityOK: w.locality.Checked,
			}
			if w.scope.Selected == scopeAgent {
				s.Scope = "agent"
			}
			for _, r := range requires {
				if w.requires[r].Checked {
					s.Requires = append(s.Requires, r)
				}
			}
			in.Stores = append(in.Stores, s)
		}
		return buildProfileDraft(in)
	}
	title := T("memory.profile.newTitle")
	if existing != nil {
		title = T("memory.editWhat", "name", existing.Name)
	}
	memoryEditor(v, title, T("memory.profile.subtitle"),
		body, fyne.NewSize(880, 560), build,
		func(ctx context.Context, d api.MemoryProfileDraft) ([]string, error) {
			return v.app.api.ValidateMemory(ctx, api.MemoryValidateRequest{Kind: "profile", Creating: creating, Profile: &d})
		},
		func(ctx context.Context, d api.MemoryProfileDraft) (api.MemoryList, error) {
			return v.app.api.SaveMemoryProfile(ctx, d.Name, creating, d, v.data.MTime)
		},
		T("memory.profile.validText"),
		T("memory.profileSaved"))
}

// selectOrAdd selects value, adding it to the options when the list does not
// have it, so opening an entry never silently changes it. Empty selects empty.
func selectOrAdd(s *widget.Select, value, empty string) {
	if value == "" {
		value = empty
	}
	if value == "" {
		return
	}
	if !hasString(s.Options, value) {
		s.Options = append(s.Options, value)
		s.Refresh()
	}
	s.SetSelected(value)
}

// ---- pure helpers (match web/src/screens/memory/helpers.ts) ----------------

// memoryProviderKeys is which config keys each provider reads.
var memoryProviderKeys = map[string][]string{
	"sqlite":      {"path", "vector_scan_limit"},
	"postgres":    {"url", "dim"},
	"pgvector":    {"url", "dim"},
	"redis":       {"url"},
	"redisvector": {"url", "dim"},
	"qdrant":      {"url", "api_key", "collection", "dim", "distance", "timeout"},
	"volatile":    {"max_entries"},
}

// providerHintKeys is the message key of the one-line hint for each provider.
var providerHintKeys = map[string]string{
	"sqlite":      "memory.providerHint.sqlite",
	"postgres":    "memory.providerHint.postgres",
	"pgvector":    "memory.providerHint.pgvector",
	"redis":       "memory.providerHint.redis",
	"redisvector": "memory.providerHint.redisvector",
	"qdrant":      "memory.providerHint.qdrant",
	"volatile":    "memory.providerHint.volatile",
}

// providerHintText is the hint for a provider in the current language, or ""
// for one Studio does not know.
func providerHintText(provider string) string {
	if key, ok := providerHintKeys[provider]; ok {
		return T(key)
	}
	return ""
}

var qdrantDistances = []string{"Cosine", "Euclid", "Dot", "Manhattan"}

func providerHas(provider, key string) bool { return hasString(memoryProviderKeys[provider], key) }

func storedSecretHint(has bool, ref, display string) string {
	switch {
	case !has:
		return T("memory.backend.storedNothing")
	case ref != "":
		return T("memory.backend.storedRef", "ref", ref)
	case display != "":
		return T("memory.backend.storedDisplay", "display", display)
	default:
		return T("memory.backend.storedValue")
	}
}

// backendTarget says where a backend points, in one line and without secrets.
func backendTarget(b api.MemoryBackend) string {
	switch b.Provider {
	case "sqlite":
		if b.Path == "" {
			return "./data/agentflow.db"
		}
		return b.Path
	case "volatile":
		if b.MaxEntries > 0 {
			return T("memory.target.maxEntries", "n", b.MaxEntries)
		}
		return T("memory.target.unbounded")
	}
	where := b.URLRef
	if where == "" {
		where = b.URLDisplay
	}
	if where == "" {
		where = T("memory.target.noUrl")
		if b.HasURL {
			where = T("memory.target.urlSet")
		}
	}
	if b.Provider == "qdrant" && b.Collection != "" {
		where += " · " + b.Collection
	}
	return where
}

// backendUsers is the profiles and agents a backend row names: the built-in
// profile counts only while an agent is on it, and agents on the built-in
// profile are named through it.
func backendUsers(b api.MemoryBackend, builtin []string) (profiles, agents []string) {
	for _, p := range b.Profiles {
		if p == "conversational" && len(builtin) == 0 {
			continue
		}
		profiles = append(profiles, p)
	}
	for _, a := range b.Agents {
		if !hasString(builtin, a) {
			agents = append(agents, a)
		}
	}
	return profiles, agents
}

func backendUsedBy(b api.MemoryBackend, builtin []string) string {
	var parts []string
	profiles, agents := backendUsers(b, builtin)
	for i, p := range profiles {
		if p == "conversational" {
			profiles[i] = T("memory.usedBy.builtin", "name", p, "agents", strings.Join(builtin, ", "))
		}
	}
	if len(profiles) > 0 {
		parts = append(parts, T("memory.usedBy.profiles", "list", strings.Join(profiles, ", ")))
	}
	if len(agents) > 0 {
		parts = append(parts, T("memory.usedBy.agents", "list", strings.Join(agents, ", ")))
	}
	if len(parts) == 0 {
		return T("memory.usedBy.unused")
	}
	return strings.Join(parts, " · ")
}

func backendInUse(b api.MemoryBackend, builtin []string) bool {
	profiles, agents := backendUsers(b, builtin)
	return len(profiles) > 0 || len(agents) > 0
}

func memoryProfileUsedBy(p api.MemoryProfile) string {
	var parts []string
	if len(p.Agents) > 0 {
		parts = append(parts, T("memory.usedBy.agents", "list", strings.Join(p.Agents, ", ")))
	}
	if len(p.SpawnProfiles) > 0 {
		parts = append(parts, T("memory.usedBy.spawn", "list", strings.Join(p.SpawnProfiles, ", ")))
	}
	if len(parts) == 0 {
		return T("memory.usedBy.noAgent")
	}
	return strings.Join(parts, " · ")
}

func memoryProfileInUse(p api.MemoryProfile) bool {
	return len(p.Agents) > 0 || len(p.SpawnProfiles) > 0
}

// storeSummary is one store in a line: "main_db.dialogue · 30d · window 200".
func storeSummary(s api.MemoryStoreDraft) string {
	t := s.Backend
	if s.Table != "" {
		t += "." + s.Table
	}
	parts := []string{t}
	if s.Retention != "" {
		parts = append(parts, s.Retention)
	}
	if s.Window > 0 {
		parts = append(parts, T("memory.summary.window", "n", s.Window))
	}
	if len(s.Requires) > 0 {
		parts = append(parts, strings.Join(s.Requires, "+"))
	}
	if s.Pool != "" {
		parts = append(parts, T("memory.summary.pool", "pool", s.Pool))
	} else if s.Scope == "agent" {
		parts = append(parts, T("memory.scopeAgent"))
	}
	return strings.Join(parts, " · ")
}

// backendInput is the backend form as text, before it becomes a draft.
type backendInput struct {
	Name, Provider, Path, URL, APIKey string
	ClearAPIKey                       bool
	Collection, Distance, Timeout     string
	Dim, VectorScanLimit, MaxEntries  string
}

// buildBackendDraft keeps only the provider's own keys, as the web app does.
func buildBackendDraft(in backendInput) (api.MemoryBackendDraft, error) {
	d := api.MemoryBackendDraft{Name: strings.TrimSpace(in.Name), Provider: in.Provider}
	has := func(k string) bool { return providerHas(in.Provider, k) }
	if d.Name == "" {
		return d, errors.New(T("memory.err.backendNameRequired"))
	}
	if has("path") {
		d.Path = strings.TrimSpace(in.Path)
	}
	if has("url") {
		d.URL = strings.TrimSpace(in.URL)
	}
	if has("api_key") {
		if d.APIKey = strings.TrimSpace(in.APIKey); d.APIKey == "" && in.ClearAPIKey {
			d.ClearAPIKey = true
		}
	}
	if has("collection") {
		d.Collection = strings.TrimSpace(in.Collection)
	}
	if has("distance") {
		d.Distance = strings.TrimSpace(in.Distance)
	}
	if has("timeout") {
		d.Timeout = strings.TrimSpace(in.Timeout)
	}
	var err error
	if has("dim") {
		if d.Dim, err = optionalCount(T("memory.backend.dimensions"), in.Dim); err != nil {
			return d, err
		}
	}
	if has("vector_scan_limit") {
		if d.VectorScanLimit, err = optionalCount(T("memory.backend.vectorScanLimit"), in.VectorScanLimit); err != nil {
			return d, err
		}
	}
	if has("max_entries") {
		if d.MaxEntries, err = optionalCount(T("memory.backend.maxEntries"), in.MaxEntries); err != nil {
			return d, err
		}
	}
	return d, nil
}

type storeInput struct {
	Name, Backend, Table, Retention, Window string
	Requires                                []string
	Scope, Pool                             string
	LocalityOK                              bool
}

type profileInput struct {
	Name                                   string
	Stores                                 []storeInput
	Write, Recall, EmbedModel, RerankModel string
	Oversample                             string
}

func buildProfileDraft(in profileInput) (api.MemoryProfileDraft, error) {
	d := api.MemoryProfileDraft{
		Name:        strings.TrimSpace(in.Name),
		Stores:      []api.MemoryStoreDraft{},
		Write:       splitList(in.Write),
		Recall:      strings.TrimSpace(in.Recall),
		EmbedModel:  strings.TrimSpace(in.EmbedModel),
		RerankModel: strings.TrimSpace(in.RerankModel),
	}
	if d.Name == "" {
		return d, errors.New(T("memory.err.profileNameRequired"))
	}
	for i, s := range in.Stores {
		label := T("memory.storeWindowNumbered", "n", i+1)
		if n := strings.TrimSpace(s.Name); n != "" {
			label = T("memory.storeWindowNamed", "name", n)
		}
		w, err := optionalCount(label, s.Window)
		if err != nil {
			return d, err
		}
		st := api.MemoryStoreDraft{
			Name: strings.TrimSpace(s.Name), Backend: strings.TrimSpace(s.Backend), Table: strings.TrimSpace(s.Table),
			Retention: strings.TrimSpace(s.Retention), Window: w, Requires: s.Requires, LocalityOK: s.LocalityOK,
		}
		if p := strings.TrimSpace(s.Pool); p != "" {
			st.Pool = p
		} else {
			st.Scope = s.Scope
		}
		d.Stores = append(d.Stores, st)
	}
	var err error
	if d.Oversample, err = optionalCount(T("memory.profile.oversample"), in.Oversample); err != nil {
		return d, err
	}
	return d, nil
}

// newProfileDraft starts a profile like the built-in conversational one, on
// main_db when there is one.
func newProfileDraft(backends []string) api.MemoryProfileDraft {
	b := ""
	if len(backends) > 0 {
		b = backends[0]
	}
	if hasString(backends, "main_db") {
		b = "main_db"
	}
	return api.MemoryProfileDraft{
		Stores: []api.MemoryStoreDraft{
			{Name: "dialogue", Backend: b, Table: "dialogue", Retention: "30d", Window: 1000, Requires: []string{"kv"}},
			{Name: "facts", Backend: b, Table: "facts", Retention: "forever", Requires: []string{"kv", "text_search"}},
		},
		Write:  []string{"plugin:routing_table"},
		Recall: "plugin:recency",
	}
}

// ---- builder ---------------------------------------------------------------

// The builder picker's fixed entries, as English text: memoryInline is also
// the value the inline entry writes, which the builder checks for. What the
// picker shows comes from the catalogues (memoryNoneLabel and friends); in
// English the two are the same text.
const (
	memoryNone    = "none"
	memoryBuiltin = "conversational (built-in)"
	memoryInline  = "inline profile (edit in Raw YAML)"
)

func memoryNoneLabel() string    { return T("common.none") }
func memoryBuiltinLabel() string { return T("memory.choice.builtin") }
func memoryInlineLabel() string  { return T("memory.choice.inline") }

// memoryChoices is the builder's memory picker: labels in order, and the
// value each one writes ("" for none). A profile the definition names that
// the catalog does not list stays selectable.
func memoryChoices(catalog []string, current string, inline bool) ([]string, map[string]string) {
	none, builtin := memoryNoneLabel(), memoryBuiltinLabel()
	labels := []string{none, builtin}
	values := map[string]string{none: "", builtin: "conversational"}
	for _, p := range catalog {
		if p == "conversational" || values[p] != "" {
			continue
		}
		labels = append(labels, p)
		values[p] = p
	}
	switch {
	case inline:
		labels = append(labels, memoryInlineLabel())
		values[memoryInlineLabel()] = memoryInline
	case current != "" && current != "conversational" && !hasString(catalog, current):
		l := T("memory.choice.notInCatalog", "name", current)
		labels = append(labels, l)
		values[l] = current
	}
	return labels, values
}

// memoryLabel is the picker label for a definition's memory setting.
func memoryLabel(current string, inline bool, values map[string]string) string {
	if inline {
		return memoryInlineLabel()
	}
	for l, v := range values {
		if v == current && v != memoryInline {
			return l
		}
	}
	return memoryNoneLabel()
}
