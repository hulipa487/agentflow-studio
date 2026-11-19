package ui

import (
	"context"
	"fmt"
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
	v.scroll = container.NewVScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", v.load)
	head := pageHeader("Memory",
		"Where agents keep what they remember: backends are the databases, profiles are the stores an agent reads and writes. Saves go to the deployment config and open on the next runtime start.",
		hstack(spaceS, widthBox(150, v.count), refresh))
	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(v.scroll),
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
	v.meta.SetText("loading…")
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
					msg = "This runtime does not expose the Studio memory API yet; rebuild and restart AgentFlow, then reopen Memory."
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

func (v *memoryView) render() {
	v.banner.Clear()
	clearBox(v.list)
	v.count.SetText(plural(len(v.data.Profiles), "profile"))
	meta := plural(len(v.data.Backends), "backend")
	if v.data.RestartRequired {
		meta += " · restart required"
	} else {
		meta += " · live"
	}
	v.meta.SetText(meta)
	switch {
	case !v.data.Editable:
		note := v.data.Note
		if note == "" {
			note = "This deployment has no editable config source for memory."
		}
		v.setBanner(note, toneWarning)
	case v.data.RestartRequired && v.data.SavedNote != "":
		v.setBanner("Restart required — "+v.data.SavedNote, toneWarning)
	}

	profileAction := iconButton(theme.ContentAddIcon(), "New profile", func() { v.editProfile(nil) })
	backendAction := iconButton(theme.ContentAddIcon(), "New backend", func() { v.editBackend(nil) })
	if !v.data.Editable {
		profileAction.Disable()
		backendAction.Disable()
	}

	profileSub := `Named store sets agents pick in the builder, besides the built-in "conversational" one.`
	if len(v.data.BuiltinAgents) > 0 {
		profileSub = fmt.Sprintf(`Named store sets agents pick in the builder. The built-in "conversational" profile is used by %s.`, strings.Join(v.data.BuiltinAgents, ", "))
	}
	v.list.Add(safetySectionHeader("Profiles", profileSub, profileAction))
	profiles := vstack(spaceS)
	for _, p := range v.data.Profiles {
		profiles.Add(v.profileCard(p))
	}
	if len(v.data.Profiles) == 0 {
		profiles.Add(subpanel(muted("No named memory profiles yet.")))
	}
	v.list.Add(profiles)

	v.list.Add(safetySectionHeader("Backends",
		"The databases stores live in. Secrets are never shown; use ${VAR} or cred:<service> to keep them out of the file.",
		backendAction))
	backends := vstack(spaceS)
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
		return chip("live", toneSuccess)
	}
	return chip("restart required", toneWarning)
}

func memoryRow(label string, value fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widthBox(108, fieldLabel(label)), nil, value)
}

func (v *memoryView) profileCard(p api.MemoryProfile) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	chips := []fyne.CanvasObject{vCenter(title), vCenter(chip(plural(len(p.Stores), "store"), toneAccent))}
	if p.Recall != "" {
		chips = append(chips, vCenter(chip(p.Recall, toneNeutral)))
	}
	chips = append(chips, vCenter(liveChip(p.Live)))
	del := iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDelete("profile", p.Name) })
	if memoryProfileInUse(p) {
		del.Disable()
	}
	top := container.NewBorder(nil, nil, hstack(spaceS, chips...),
		hstack(spaceXS,
			vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.editProfile(&p) })),
			vCenter(del),
		), nil)
	rows := []fyne.CanvasObject{}
	for _, s := range p.Stores {
		rows = append(rows, memoryRow(s.Name, monoInline(storeSummary(s))))
	}
	if p.EmbedModel != "" {
		e := p.EmbedModel
		if p.RerankModel != "" {
			e += " · rerank " + p.RerankModel
		}
		rows = append(rows, memoryRow("embedding", monoInline(e)))
	}
	rows = append(rows, memoryRow("used by", widget.NewLabel(memoryProfileUsedBy(p))))
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(rows...)))
}

func (v *memoryView) backendCard(b api.MemoryBackend) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(b.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	state := liveChip(b.Live)
	if b.Implicit {
		state = chip("built-in", toneNeutral)
	}
	actions := []fyne.CanvasObject{vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.editBackend(&b) }))}
	if !b.Implicit {
		del := iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDelete("backend", b.Name) })
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
		used = "built-in profile: " + strings.Join(v.data.BuiltinAgents, ", ")
	}
	rows := []fyne.CanvasObject{memoryRow("target", monoInline(backendTarget(b)))}
	if b.Dim > 0 {
		rows = append(rows, memoryRow("dimensions", monoInline(strconv.Itoa(b.Dim))))
	}
	rows = append(rows, memoryRow("used by", widget.NewLabel(used)))
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewVBox(rows...)))
}

func (v *memoryView) readOnly() bool {
	if v.data.Editable {
		return false
	}
	note := v.data.Note
	if note == "" {
		note = "This deployment has no editable config source, so memory cannot be changed from Studio."
	}
	showInfoPrompt(v.app.win, "Memory is read-only", note)
	return true
}

func (v *memoryView) confirmDelete(kind, name string) {
	if v.readOnly() {
		return
	}
	msg := fmt.Sprintf("Delete memory profile %q from the deployment config?\n\nThe stored records are not deleted.", name)
	if kind == "backend" {
		msg = fmt.Sprintf("Delete memory backend %q from the deployment config?\n\nThe database itself and its records are not touched.", name)
	}
	showConfirmPrompt(v.app.win, "Delete memory "+kind, msg, "Delete", true, func(ok bool) {
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
				v.applied(list, "memory "+kind+" deleted · restart required")
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
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	validateBtn := actionButton("Validate", checkMarkIcon, nil)
	saveBtn := primaryButton("Save", theme.UploadIcon(), nil)
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
	clearKey := widget.NewCheck("Remove the stored API key", nil)
	collection := monoEntry(false)
	collection.SetPlaceHolder("agentflow")
	distance := widget.NewSelect(append([]string{"Cosine (default)"}, qdrantDistances...), nil)
	distance.SetSelected("Cosine (default)")
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
		providerHint.SetText(providerHints[p])
		sameProvider := existing != nil && !existing.Implicit && existing.Provider == p
		has := func(k string) bool { return providerHas(p, k) }
		if has("path") {
			fields.Add(field("Path", "A file path, relative to the config. Empty uses ./data/agentflow.db.", path))
		}
		if has("url") {
			hint := "Required."
			if sameProvider {
				hint = storedSecretHint(existing.HasURL, existing.URLRef, existing.URLDisplay)
			}
			fields.Add(field("URL", hint+" Use ${VAR} or cred:<service> to keep the secret out of the file.", urlEntry))
		}
		if has("api_key") {
			hint := "Optional."
			if sameProvider {
				hint = storedSecretHint(existing.HasAPIKey, existing.APIKeyRef, "")
			}
			box := field("API key", hint, apiKey)
			if sameProvider && existing.HasAPIKey {
				box.(*fyne.Container).Add(compactCheck(clearKey))
			}
			fields.Add(box)
		}
		grid := []fyne.CanvasObject{}
		if has("collection") {
			grid = append(grid, field("Collection", "Empty uses the driver default.", collection))
		}
		if has("distance") {
			grid = append(grid, field("Distance", "", distance))
		}
		if has("timeout") {
			grid = append(grid, field("Timeout", "A duration, e.g. 5s.", timeout))
		}
		if has("dim") {
			grid = append(grid, field("Dimensions", "The embedding model's vector size.", dim))
		}
		if has("vector_scan_limit") {
			grid = append(grid, field("Vector scan limit", "Rows a vector search may scan. Default 50000.", scan))
		}
		if has("max_entries") {
			grid = append(grid, field("Max entries", "Bounds memory. Without it every store on this backend needs a retention.", maxEntries))
		}
		if len(grid) > 0 {
			fields.Add(container.NewGridWithColumns(2, grid...))
		}
		if !sameProvider && existing != nil && !existing.Implicit {
			fields.Add(caption("Changing provider drops the settings the old one used."))
		}
		if existing != nil && len(existing.OtherKeys) > 0 {
			fields.Add(caption("Also in the file and kept as they are: " + strings.Join(existing.OtherKeys, ", ") + ". Edit them in Config."))
		}
		fields.Refresh()
	}
	provider.OnChanged = func(string) { render() }
	render()
	providerHint.Wrapping = fyne.TextWrapWord

	body := vstack(spaceM,
		container.NewGridWithColumns(2,
			field("Name", "Stores refer to the backend by name, so it cannot change later.", name),
			field("Provider", "", provider),
		),
		providerHint,
		fields,
	)
	build := func() (api.MemoryBackendDraft, error) {
		dist := distance.Selected
		if strings.HasSuffix(dist, "(default)") {
			dist = ""
		}
		return buildBackendDraft(backendInput{
			Name: name.Text, Provider: provider.Selected, Path: path.Text, URL: urlEntry.Text,
			APIKey: apiKey.Text, ClearAPIKey: clearKey.Checked, Collection: collection.Text,
			Distance: dist, Timeout: timeout.Text, Dim: dim.Text, VectorScanLimit: scan.Text, MaxEntries: maxEntries.Text,
		})
	}
	title := "New memory backend"
	if existing != nil {
		title = "Edit " + existing.Name
		if existing.Implicit {
			title = "Write out main_db"
		}
	}
	memoryEditor(v, title, "A backend is a database the memory stores live in. Backends open when agentflow starts.",
		body, fyne.NewSize(640, 420), build,
		func(ctx context.Context, d api.MemoryBackendDraft) ([]string, error) {
			return v.app.api.ValidateMemory(ctx, api.MemoryValidateRequest{Kind: "backend", Creating: creating, Backend: &d})
		},
		func(ctx context.Context, d api.MemoryBackendDraft) (api.MemoryList, error) {
			return v.app.api.SaveMemoryBackend(ctx, d.Name, creating, d, v.data.MTime)
		},
		"Valid. Save it to open the backend on the next runtime start.",
		"memory backend saved · restart required")
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
	embed := widget.NewSelect(append([]string{"none"}, v.data.Models...), nil)
	rerank := widget.NewSelect(append([]string{"none"}, v.data.RerankModels...), nil)
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
	selectOrAdd(embed, draft.EmbedModel, "none")
	selectOrAdd(rerank, draft.RerankModel, "none")
	setIntEntry(oversample, draft.Oversample)

	storesBox := vstack(spaceS)
	var stores []*storeWidgets
	var addStore func(s api.MemoryStoreDraft)
	addStore = func(s api.MemoryStoreDraft) {
		w := &storeWidgets{
			name: monoEntry(false), backend: widget.NewSelect(backendNames, nil), table: monoEntry(false),
			retention: monoEntry(false), window: monoEntry(false), requires: map[string]*widget.Check{},
			scope: widget.NewSelect([]string{"per user (default)", "whole agent"}, nil), pool: monoEntry(false),
			locality: widget.NewCheck("instance-local is fine", nil),
		}
		w.name.SetPlaceHolder("store")
		w.table.SetPlaceHolder("table")
		w.retention.SetPlaceHolder("retention")
		w.window.SetPlaceHolder("window")
		w.pool.SetPlaceHolder("shared pool")
		w.name.SetText(s.Name)
		selectOrAdd(w.backend, s.Backend, "")
		w.table.SetText(s.Table)
		w.retention.SetText(s.Retention)
		setIntEntry(w.window, s.Window)
		w.scope.SetSelected("per user (default)")
		if s.Scope == "agent" {
			w.scope.SetSelected("whole agent")
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
		remove := iconButton(theme.ContentClearIcon(), "Remove store", nil)
		w.row = panel(vstack(spaceXS,
			container.NewBorder(nil, nil, nil, remove, container.NewGridWithColumns(5,
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
	addBtn := actionButton("Add store", theme.ContentAddIcon(), func() { addStore(api.MemoryStoreDraft{Backend: firstBackend}) })

	field := func(label, hint string, control fyne.CanvasObject) fyne.CanvasObject {
		box := vstack(spaceXS, fieldLabel(label), controlFrame(control))
		if hint != "" {
			c := caption(hint)
			c.Wrapping = fyne.TextWrapWord
			box.Add(c)
		}
		return box
	}
	header := container.NewBorder(nil, nil, nil, widthBox(36, layout.NewSpacer()), container.NewGridWithColumns(5,
		caption("store"), caption("backend"), caption("table"), caption("retention"), caption("window")))
	body := vstack(spaceM,
		field("Name", `Agents refer to the profile by name, so it cannot change later. "conversational" is the built-in one.`, name),
		vstack(spaceXS, fieldLabel("Stores"), header, storesBox, hstack(spaceS, widthBox(130, controlFrame(addBtn))),
			caption("Each logical store is a table on a backend. Retention is a duration such as 30d, or forever.")),
		container.NewGridWithColumns(2,
			field("Recall", "plugin:recency reads dialogue and facts; plugin:semantic searches the semantic store by meaning.", recall),
			field("Write", "Comma-separated write plugins, in order.", write),
			field("Embed model", "Embeds each record on write; required for semantic recall.", embed),
			field("Rerank model", "Optional: oversample vector hits and let a provider: rerank model order them.", rerank),
			field("Oversample", "With a rerank model: how many times k hits to rerank. Default 4.", oversample),
		),
	)
	build := func() (api.MemoryProfileDraft, error) {
		in := profileInput{Name: name.Text, Recall: recall.Text, Write: write.Text, Oversample: oversample.Text}
		if embed.Selected != "none" {
			in.EmbedModel = embed.Selected
		}
		if rerank.Selected != "none" {
			in.RerankModel = rerank.Selected
		}
		for _, w := range stores {
			s := storeInput{
				Name: w.name.Text, Backend: w.backend.Selected, Table: w.table.Text, Retention: w.retention.Text,
				Window: w.window.Text, Pool: w.pool.Text, LocalityOK: w.locality.Checked,
			}
			if w.scope.Selected == "whole agent" {
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
	title := "New memory profile"
	if existing != nil {
		title = "Edit " + existing.Name
	}
	memoryEditor(v, title, "Agents name a profile with memory: <name>. The built-in plugins look for stores called dialogue, facts and semantic.",
		body, fyne.NewSize(880, 560), build,
		func(ctx context.Context, d api.MemoryProfileDraft) ([]string, error) {
			return v.app.api.ValidateMemory(ctx, api.MemoryValidateRequest{Kind: "profile", Creating: creating, Profile: &d})
		},
		func(ctx context.Context, d api.MemoryProfileDraft) (api.MemoryList, error) {
			return v.app.api.SaveMemoryProfile(ctx, d.Name, creating, d, v.data.MTime)
		},
		"Valid. Save it, then pick it for an agent in the builder after the next restart.",
		"memory profile saved · restart required")
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

var providerHints = map[string]string{
	"sqlite":      "A local SQLite file: key-value, full-text and brute-force vector search. No server needed.",
	"postgres":    "A PostgreSQL server: key-value and full-text search, shared by every instance.",
	"pgvector":    "PostgreSQL with the pgvector extension: everything postgres does, plus indexed vector search.",
	"redis":       "A Redis server: key-value only, fast and shared.",
	"redisvector": "Redis Stack: key-value plus vector search.",
	"qdrant":      "A Qdrant server: vector search at scale.",
	"volatile":    "Process memory, lost on restart and shared by every session. Bound it with max entries or a store retention.",
}

var qdrantDistances = []string{"Cosine", "Euclid", "Dot", "Manhattan"}

func providerHas(provider, key string) bool { return hasString(memoryProviderKeys[provider], key) }

func storedSecretHint(has bool, ref, display string) string {
	switch {
	case !has:
		return "Nothing stored."
	case ref != "":
		return "Stored as the reference " + ref + "."
	case display != "":
		return "Stored: " + display + ". Leave empty to keep it."
	default:
		return "A value is stored. Leave empty to keep it."
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
			return fmt.Sprintf("max %d entries", b.MaxEntries)
		}
		return "unbounded"
	}
	where := b.URLRef
	if where == "" {
		where = b.URLDisplay
	}
	if where == "" {
		where = "no URL"
		if b.HasURL {
			where = "URL set"
		}
	}
	if b.Provider == "qdrant" && b.Collection != "" {
		where += " · " + b.Collection
	}
	return where
}

func backendUsedBy(b api.MemoryBackend, builtin []string) string {
	var parts []string
	var profiles []string
	for _, p := range b.Profiles {
		if p == "conversational" {
			if len(builtin) == 0 {
				continue
			}
			p = "conversational (built-in: " + strings.Join(builtin, ", ") + ")"
		}
		profiles = append(profiles, p)
	}
	if len(profiles) > 0 {
		parts = append(parts, "profiles: "+strings.Join(profiles, ", "))
	}
	var agents []string
	for _, a := range b.Agents {
		if !hasString(builtin, a) {
			agents = append(agents, a)
		}
	}
	if len(agents) > 0 {
		parts = append(parts, "agents: "+strings.Join(agents, ", "))
	}
	if len(parts) == 0 {
		return "unused"
	}
	return strings.Join(parts, " · ")
}

func backendInUse(b api.MemoryBackend, builtin []string) bool {
	return backendUsedBy(b, builtin) != "unused"
}

func memoryProfileUsedBy(p api.MemoryProfile) string {
	var parts []string
	if len(p.Agents) > 0 {
		parts = append(parts, "agents: "+strings.Join(p.Agents, ", "))
	}
	if len(p.SpawnProfiles) > 0 {
		parts = append(parts, "spawn: "+strings.Join(p.SpawnProfiles, ", "))
	}
	if len(parts) == 0 {
		return "not used by an agent yet"
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
		parts = append(parts, fmt.Sprintf("window %d", s.Window))
	}
	if len(s.Requires) > 0 {
		parts = append(parts, strings.Join(s.Requires, "+"))
	}
	if s.Pool != "" {
		parts = append(parts, "pool "+s.Pool)
	} else if s.Scope == "agent" {
		parts = append(parts, "whole agent")
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
		return d, fmt.Errorf("a backend name is required")
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
		if d.Dim, err = optionalCount("dimensions", in.Dim); err != nil {
			return d, err
		}
	}
	if has("vector_scan_limit") {
		if d.VectorScanLimit, err = optionalCount("vector scan limit", in.VectorScanLimit); err != nil {
			return d, err
		}
	}
	if has("max_entries") {
		if d.MaxEntries, err = optionalCount("max entries", in.MaxEntries); err != nil {
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
		return d, fmt.Errorf("a profile name is required")
	}
	for i, s := range in.Stores {
		label := fmt.Sprintf("store %d", i+1)
		if n := strings.TrimSpace(s.Name); n != "" {
			label = fmt.Sprintf("store %q", n)
		}
		w, err := optionalCount(label+": window", s.Window)
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
	if d.Oversample, err = optionalCount("oversample", in.Oversample); err != nil {
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

const (
	memoryNone    = "none"
	memoryBuiltin = "conversational (built-in)"
	memoryInline  = "inline profile (edit in Raw YAML)"
)

// memoryChoices is the builder's memory picker: labels in order, and the
// value each one writes ("" for none). A profile the definition names that
// the catalog does not list stays selectable.
func memoryChoices(catalog []string, current string, inline bool) ([]string, map[string]string) {
	labels := []string{memoryNone, memoryBuiltin}
	values := map[string]string{memoryNone: "", memoryBuiltin: "conversational"}
	for _, p := range catalog {
		if p == "conversational" || values[p] != "" {
			continue
		}
		labels = append(labels, p)
		values[p] = p
	}
	switch {
	case inline:
		labels = append(labels, memoryInline)
		values[memoryInline] = memoryInline
	case current != "" && current != "conversational" && !hasString(catalog, current):
		l := current + " · not in catalog"
		labels = append(labels, l)
		values[l] = current
	}
	return labels, values
}

// memoryLabel is the picker label for a definition's memory setting.
func memoryLabel(current string, inline bool, values map[string]string) string {
	if inline {
		return memoryInline
	}
	for l, v := range values {
		if v == current && l != memoryInline {
			return l
		}
	}
	return memoryNone
}
