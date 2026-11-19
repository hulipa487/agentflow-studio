package ui

import (
	"context"
	"encoding/json"
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
	"agentflow-studio/internal/appconfig"
)

var agentNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

const defaultModelOption = "default (per call)"

type builderView struct {
	app    *App
	name   string
	isNew  bool
	detail *api.AgentDetail
	mtime  int64
	root   fyne.CanvasObject

	nameEntry     *widget.Entry
	model         *widget.Select
	safety        *widget.Select
	history       *widget.Entry
	persistent    *widget.Check
	capsChecks    []*widget.Check
	skillChecks   []*widget.Check
	canContact    *widget.Entry
	credentials   *widget.Entry
	shell         *widget.Entry
	memory        *widget.Select
	memoryValues  map[string]string
	extras        *widget.Entry
	loop          *codeEditor
	instructions  *codeEditor
	yaml          *codeEditor
	rawDirty      bool
	suppressDirty bool
	dirty         bool

	// persist is the Apply button's choice, remembered in the app config.
	persist     bool
	messages    *fyne.Container
	title       *canvas.Text
	subtitle    *widget.Label
	stateDot    *canvas.Circle
	validateBtn *widget.Button
	applyBtn    *widget.Button
	applyMenu   *widget.Button
	runBtn      *toggleButton
	docsBtn     *toggleButton
	deleteBtn   *widget.Button
	backBtn     *widget.Button
	activity    *widget.Activity
	luaEditor   *luaEditor
	tabs        *container.AppTabs
	busy        bool

	// The dock beside the editor holds the run panel or the Luau reference;
	// dock is "run", "docs" or "" (closed).
	dock      string
	run       *chatSession
	runPane   fyne.CanvasObject
	docsPane  fyne.CanvasObject
	body      *fyne.Container
	editor    fyne.CanvasObject
	dockSplit *container.Split
}

func displayName(name string) string {
	if name == "" {
		return "New agent"
	}
	return name
}

func (a *App) showBuilder(name string, tpl *api.Template) {
	a.enter("builder")
	a.setBreadcrumb("Agents / " + displayName(name))

	if name == "" {
		b := a.newBuilderView("", nil, tpl)
		a.setMain(b.root)
		b.claimActions()
		return
	}

	a.setMain(container.NewCenter(widget.NewLabel("Loading agent…")))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		d, err := a.api.Agent(ctx, name)
		onUI(func() {
			if err != nil {
				a.showError(err)
				a.showAgents()
				return
			}
			b := a.newBuilderView(name, &d, nil)
			a.setMain(b.root)
			b.claimActions()
			a.setBreadcrumb("Agents / " + name)
		})
	}()
}

func (a *App) newBuilderView(name string, d *api.AgentDetail, tpl *api.Template) *builderView {
	a.closeLuaEditor()
	b := &builderView{app: a, name: name, isNew: name == "", detail: d}
	if d != nil {
		b.mtime = d.Files.Mtime
	}

	ag := api.AgentJSON{}
	if d != nil {
		ag = d.Agent
	}
	if tpl != nil {
		ag = api.AgentJSON{
			Loop:          tpl.Loop,
			Safety:        "default",
			HistoryBudget: 6000,
			Capabilities:  append([]string(nil), a.catalog.DefaultCapabilities...),
		}
	}
	if ag.HistoryBudget == 0 {
		ag.HistoryBudget = 6000
	}
	if ag.Safety == "" {
		ag.Safety = "default"
	}

	b.nameEntry = widget.NewEntry()
	b.nameEntry.SetText(name)
	b.nameEntry.SetPlaceHolder("researcher")
	if !b.isNew {
		b.nameEntry.Disable()
	}

	modelOpts := append([]string{defaultModelOption}, a.catalog.Models...)
	b.model = widget.NewSelect(modelOpts, nil)
	if ag.Model == "" {
		b.model.SetSelected(defaultModelOption)
	} else {
		b.model.SetSelected(ag.Model)
	}

	safetyOpts := a.catalog.SafetyProfiles
	if len(safetyOpts) == 0 {
		safetyOpts = []string{"default", "none"}
	}
	b.safety = widget.NewSelect(safetyOpts, nil)
	b.safety.SetSelected(ag.Safety)

	b.history = widget.NewEntry()
	b.history.SetText(strconv.Itoa(ag.HistoryBudget))

	b.persistent = widget.NewCheck("Daemon (persistent)", nil)
	b.persistent.SetChecked(ag.Persistent)

	b.capsChecks = newChecks(a.catalog.Capabilities, ag.Capabilities)
	b.skillChecks = newChecks(a.catalog.Tools, ag.Skills)
	for _, c := range b.capsChecks {
		c.OnChanged = func(bool) {
			b.markDirty()
			if b.luaEditor != nil {
				b.luaEditor.SetCapabilities()
			}
		}
	}
	for _, c := range b.skillChecks {
		c.OnChanged = func(bool) { b.markDirty() }
	}

	b.canContact = widget.NewEntry()
	b.canContact.SetPlaceHolder("pm, researcher")
	b.canContact.SetText(strings.Join(ag.CanContact, ", "))

	b.credentials = widget.NewEntry()
	b.credentials.SetPlaceHolder("openai, stripe")
	b.credentials.SetText(strings.Join(ag.Credentials, ", "))

	b.shell = widget.NewEntry()
	b.shell.SetPlaceHolder("(none)")
	b.shell.SetText(ag.Shell)

	inline := len(ag.MemoryInline) > 0
	memLabels, memValues := memoryChoices(a.catalog.MemoryProfiles, ag.Memory, inline)
	b.memoryValues = memValues
	b.memory = widget.NewSelect(memLabels, nil)
	b.memory.SetSelected(memoryLabel(ag.Memory, inline, memValues))

	b.extras = monoEntry(true)
	if len(ag.Extras) > 0 {
		if raw, err := json.MarshalIndent(ag.Extras, "", "  "); err == nil {
			b.extras.SetText(string(raw))
		}
	}
	b.extras.SetMinRowsVisible(3)

	loopSource := ""
	if d != nil {
		loopSource = d.Loop
	} else if tpl != nil {
		loopSource = tpl.Loop
	}
	b.luaEditor = newLuaEditor(a, "Loop (Luau)",
		"Luau · Ctrl+Space completion · Ctrl+F find · Ctrl+D next match · right-click for more",
		loopSource, a.catalog.PreludeVersion, b.effectiveCapabilities)
	b.luaEditor.onEdit = b.markDirty
	b.loop = b.luaEditor.editor
	a.luaEditor = b.luaEditor

	instrSource := ""
	if d != nil {
		instrSource = d.Instructions
	}
	instrArea := newCodeAreaText(instrSource, "markdown")
	b.instructions = instrArea.editor
	b.instructions.SetWrap(true)
	b.instructions.SetMinRowsVisible(16)

	yamlArea := newCodeAreaText("", "yaml")
	b.yaml = yamlArea.editor
	b.yaml.SetMinRowsVisible(20)
	if d != nil && d.AgentYAML != "" {
		b.yaml.SetText(d.AgentYAML)
	} else if y, err := b.buildYAML(); err == nil {
		b.yaml.SetText(y)
	}

	b.messages = vstack(spaceXS)
	b.messages.Hide()
	b.title = titleText(displayName(name), theme.SizeNameHeadingText)
	b.stateDot = dot(toneWarning)
	b.subtitle = caption("")
	b.activity = widget.NewActivity()
	b.activity.Hide()

	b.persist = a.cfg.BuilderPersist

	b.validateBtn = actionButton("Validate", checkMarkIcon, b.validate)
	b.applyBtn = primaryButton("Apply", theme.UploadIcon(), b.apply)
	b.applyMenu = primaryButton("", theme.MenuDropDownIcon(), nil)
	b.applyMenu.OnTapped = b.showApplyMenu
	b.runBtn = newToggleButton("Run", theme.MediaPlayIcon(), func() { b.toggleDock("run") })
	b.docsBtn = newToggleButton("", theme.HelpIcon(), func() { b.toggleDock("docs") })
	b.deleteBtn = iconButton(theme.DeleteIcon(), "Delete", func() {
		if b.name != "" {
			a.confirmDelete(b.name, false, "")
		}
	})
	b.backBtn = iconButton(theme.NavigateBackIcon(), "Back", b.back)

	// Install change tracking after the initial values are in place.
	b.nameEntry.OnChanged = func(string) { b.markDirty() }
	b.model.OnChanged = func(string) { b.markDirty() }
	b.safety.OnChanged = func(string) { b.markDirty() }
	b.history.OnChanged = func(string) { b.markDirty() }
	b.persistent.OnChanged = func(bool) { b.markDirty() }
	b.canContact.OnChanged = func(string) { b.markDirty() }
	b.credentials.OnChanged = func(string) { b.markDirty() }
	b.shell.OnChanged = func(string) { b.markDirty() }
	b.memory.OnChanged = func(string) { b.markDirty() }
	b.extras.OnChanged = func(string) { b.markDirty() }
	b.instructions.onChanged = b.markDirty
	b.yaml.onChanged = func() {
		if !b.suppressDirty {
			b.rawDirty = true
			b.markDirty()
		}
	}

	identityGrid := container.NewGridWithColumns(2,
		labeled("Name", b.nameEntry),
		labeled("Model", b.model),
		labeled("Safety profile", b.safety),
		labeled("History budget", b.history),
		labeled("Memory profile", b.memory),
	)
	identityCard := section("Identity",
		"Model, safety, memory and context budget. Memory profiles are set up on the Memory screen; a new one can be picked after a restart.",
		identityGrid)

	defaultsBtn := actionButton("Use defaults", nil, func() {
		b.suppressDirty = true
		clearChecks(b.capsChecks)
		b.suppressDirty = false
		b.markDirty()
		if b.luaEditor != nil {
			b.luaEditor.SetCapabilities()
		}
	})
	defaultsBtn.Importance = widget.LowImportance
	capsCard := sectionAction("Capabilities",
		"Empty uses the deployment defaults.",
		defaultsBtn, checkGrid(b.capsChecks, 2))

	var toolsBody fyne.CanvasObject
	switch {
	case len(b.skillChecks) == 0:
		toolsBody = caption("No tools are registered in this deployment.")
	case len(b.skillChecks) > 8:
		grid := checkGrid(b.skillChecks, 2)
		scroll := container.NewVScroll(grid)
		scroll.SetMinSize(fyne.NewSize(0, 180))
		toolsBody = scroll
	default:
		toolsBody = checkGrid(b.skillChecks, 2)
	}
	toolsCard := section("Tools (skills)",
		"Leave empty to expose every tool allowed by the deployment policy.",
		toolsBody)

	advancedTop := container.NewGridWithColumns(2,
		labeled("Can contact", b.canContact),
		labeled("Credentials", b.credentials),
		labeled("Shell profile", b.shell),
		vCenter(compactCheck(b.persistent)),
	)
	advancedCard := section("Advanced",
		"Runtime, fleet contact and credentials.",
		vstack(spaceS, advancedTop, labeled("Extras (JSON)", b.extras)))

	// The tab strip and the cards must start on the same vertical line. The
	// whole tab surface gets one gutter below; Config only adds vertical air.
	configTab := container.NewVScroll(container.New(
		layout.NewCustomPaddedLayout(spaceXS, pageBottom, 0, spaceS),
		vstack(spaceS, identityCard, capsCard, toolsCard, advancedCard)))

	copyLoop := iconButton(theme.ContentCopyIcon(), "Copy loop", func() {
		a.win.Clipboard().SetContent(b.loop.Text())
		a.setStatus("loop copied", toneSuccess)
	})
	b.luaEditor.addAction(copyLoop)
	loopTab := b.luaEditor.root

	copyInstr := iconButton(theme.ContentCopyIcon(), "Copy instructions", func() {
		a.win.Clipboard().SetContent(b.instructions.Text())
		a.setStatus("instructions copied", toneSuccess)
	})
	instrTab := editorFrame("Instructions",
		"Saved next to the loop as a prompt file; the definition points at it.",
		instrArea.root, copyInstr)

	rebuild := actionButton("Rebuild from form", theme.ViewRefreshIcon(), func() {
		y, err := b.buildYAML()
		if err != nil {
			b.setError(err.Error())
			return
		}
		b.suppressDirty = true
		b.yaml.SetText(y)
		b.suppressDirty = false
		b.rawDirty = false
	})
	copyYAML := iconButton(theme.ContentCopyIcon(), "Copy YAML", func() {
		a.win.Clipboard().SetContent(b.yaml.Text())
		a.setStatus("YAML copied", toneSuccess)
	})
	yamlTab := editorFrame("Raw YAML",
		"Sent to the runtime. The loop and instructions sources above are sent separately; editing here overrides the form until you rebuild.",
		yamlArea.root, rebuild, copyYAML)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Config", theme.SettingsIcon(), configTab),
		container.NewTabItemWithIcon("Loop", theme.DocumentIcon(), loopTab),
		container.NewTabItemWithIcon("Instructions", theme.FileTextIcon(), instrTab),
		container.NewTabItemWithIcon("YAML", theme.FileTextIcon(), yamlTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	tabs.OnSelected = func(item *container.TabItem) {
		if item != nil && item.Text == "Loop" {
			b.app.win.Canvas().Focus(b.loop)
		}
	}
	b.tabs = tabs

	// The dock: a run panel (chat and trace with this agent) or the inline
	// Luau guide. Both are built once, so the chat keeps its thread and the
	// guide its scroll position when the dock flips or closes.
	b.run = newChatSession(a, name, chatOptions{
		hint:        "Ctrl+Enter applies edits and sends · empty repeats the last",
		emptyTitle:  "No messages yet",
		emptyNote:   "Each send applies your edits first.",
		statusWidth: 64,
		actions: []fyne.CanvasObject{
			iconButton(theme.ContentAddIcon(), "New thread", func() {
				id := newThreadID()
				b.run.switchTo(b.run.agent, id, "new thread "+shortID(id), toneAccent)
			}),
			iconButton(theme.ComputerIcon(), "Open in the Playground", func() {
				if name := b.name; name != "" {
					b.leave(func() { b.app.showPlayground(name) })
				}
			}),
		},
	})
	b.run.onSend = b.runTurn
	if b.isNew {
		b.run.headerName.SetText("Not applied yet")
		b.run.setStatus("apply to start", toneNeutral)
	}
	runSplit := container.NewVSplit(b.run.chatPanel, b.run.tracePanel)
	runSplit.SetOffset(0.6)
	b.runPane = runSplit
	b.docsPane = b.luaDocsPane()
	dockPane := container.New(layout.NewCustomPaddedLayout(pageTop, pageBottom, spaceM, pageGutter),
		container.NewStack(b.runPane, b.docsPane))

	// One toolbar across the editor and the dock: the agent and its state on
	// the left, the actions on the right. Apply is a split button whose menu
	// chooses between applying live and applying and saving.
	applySplit := hstack(1, widthBox(112, b.applyBtn), widthBox(30, b.applyMenu))
	status := hstack(spaceXS, vCenter(dotBox(b.stateDot)), vCenter(compactLabel(b.subtitle, true)))
	titleLeft := hstack(spaceS, vCenter(b.backBtn), vCenter(vstack(0, b.title, status)))
	titleRight := hstack(spaceXS,
		vCenter(b.activity),
		vCenter(widthBox(96, b.validateBtn)),
		vCenter(applySplit),
		vCenter(widthBox(76, b.runBtn)),
		vCenter(widthBox(36, b.docsBtn)),
		vCenter(b.deleteBtn),
	)
	titleRow := container.NewBorder(nil, nil, titleLeft, titleRight, nil)
	head := vstack(spaceXS,
		container.New(layout.NewCustomPaddedLayout(spaceS, spaceXS, pageGutter, pageGutter), titleRow),
		hInset(b.messages),
		widget.NewSeparator(),
	)

	b.editor = container.New(layout.NewCustomPaddedLayout(spaceXS, 0, spaceM, spaceM), tabs)
	b.dockSplit = container.NewHSplit(b.editor, dockPane)
	b.dockSplit.SetOffset(0.58)
	b.body = container.NewStack()
	b.root = container.NewBorder(head, nil, nil, nil, b.body)
	b.setDock(a.cfg.BuilderDock)

	b.updateHeader()
	if b.isNew {
		b.deleteBtn.Disable()
	}
	return b
}

// claimActions binds the window shortcuts to this builder: Ctrl+S applies,
// Ctrl+Enter applies and runs, F1 flips the Luau reference, Esc goes back.
func (b *builderView) claimActions() {
	b.app.saveAction = b.apply
	b.app.sendAction = b.runTurn
	b.app.helpAction = func() { b.toggleDock("docs") }
	b.app.backAction = b.back
}

// setDock shows the dock's run panel or reference, or closes it so the
// editor takes the whole width.
func (b *builderView) setDock(dock string) {
	if dock != "run" && dock != "docs" {
		dock = ""
	}
	b.dock = dock
	b.runBtn.setActive(dock == "run")
	b.docsBtn.setActive(dock == "docs")
	if dock == "" {
		b.body.Objects = []fyne.CanvasObject{b.editor}
	} else {
		if dock == "run" {
			b.runPane.Show()
			b.docsPane.Hide()
		} else {
			b.docsPane.Show()
			b.runPane.Hide()
		}
		b.body.Objects = []fyne.CanvasObject{b.dockSplit}
	}
	b.body.Refresh()
}

// rememberDock keeps the dock choice for the next builder.
func (b *builderView) rememberDock() {
	if b.app.cfg.BuilderDock == b.dock {
		return
	}
	b.app.cfg.BuilderDock = b.dock
	_ = appconfig.Save(b.app.cfg)
}

func (b *builderView) toggleDock(dock string) {
	if b.dock == dock {
		dock = ""
	}
	b.setDock(dock)
	b.rememberDock()
	if dock == "run" {
		b.app.win.Canvas().Focus(b.run.input)
	}
}

// showApplyMenu offers the two ways to apply; the pick runs at once and
// becomes the button's choice.
func (b *builderView) showApplyMenu() {
	if b.busy {
		return
	}
	live := fyne.NewMenuItem("Apply: swap the live definition", func() { b.choosePersist(false) })
	live.Checked = !b.persist
	save := fyne.NewMenuItem("Apply & save: also write it to the config", func() { b.choosePersist(true) })
	save.Checked = b.persist
	menu := fyne.NewMenu("", live, save)
	c := b.app.win.Canvas()
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b.applyMenu)
	pop := widget.NewPopUpMenu(menu, c)
	w := pop.MinSize().Width
	pop.ShowAtPosition(fyne.NewPos(pos.X+b.applyMenu.Size().Width-w, pos.Y+b.applyMenu.Size().Height+4))
}

func (b *builderView) choosePersist(persist bool) {
	b.persist = persist
	if b.app.cfg.BuilderPersist != persist {
		b.app.cfg.BuilderPersist = persist
		_ = appconfig.Save(b.app.cfg)
	}
	b.applyBtn.SetText(b.applyLabel())
	b.applyWith(persist, nil)
}

// runTurn is Ctrl+Enter and the run panel's Send: it applies pending edits
// (live only, whatever the Apply choice), then sends the composer's message,
// or repeats the last one when the composer is empty.
func (b *builderView) runTurn() {
	if b.busy {
		return
	}
	if b.dock != "run" {
		b.setDock("run")
		b.rememberDock()
	}
	send := func() {
		if b.run.agent != b.name {
			b.run.switchTo(b.name, newThreadID(), "ready", toneSuccess)
		}
		typed := b.run.input.Text
		text := typed
		if strings.TrimSpace(text) == "" {
			text = b.run.last
		}
		if strings.TrimSpace(text) == "" {
			b.app.win.Canvas().Focus(b.run.input)
			return
		}
		if b.run.sending {
			b.app.setStatus("a turn is still running; stop it or wait", toneWarning)
			return
		}
		if b.run.sendText(text) && text == typed {
			b.run.input.SetText("")
		}
	}
	if b.dirty || b.isNew {
		b.applyWith(false, send)
		return
	}
	send()
}

// effectiveCapabilities is what the loop will actually run with: an explicit
// selection, or the deployment defaults when the agent leaves the list empty.
// nil means the catalog has no defaults to reason about, so capability checks
// are skipped rather than guessed.
func (b *builderView) effectiveCapabilities() []string {
	if caps := selectedChecks(b.capsChecks); len(caps) > 0 {
		return caps
	}
	def := b.app.catalog.DefaultCapabilities
	if len(def) == 0 {
		return nil
	}
	return append([]string(nil), def...)
}

func (b *builderView) applyLabel() string {
	if b.persist {
		return "Apply & save"
	}
	return "Apply"
}

// headerStatus keeps the title line to one short, unambiguous state.
func (b *builderView) headerStatus() (string, tone) {
	if b.dirty {
		return "Unsaved changes", toneAccent
	}
	if b.detail == nil {
		return "Not applied", toneWarning
	}
	if b.detail.Persisted {
		return "Saved", toneSuccess
	}
	return "Live", toneAccent
}

func (b *builderView) updateHeader() {
	b.title.Text = displayName(b.name)
	b.title.Refresh()
	text, t := b.headerStatus()
	b.stateDot.FillColor = toneColor(t)
	b.stateDot.Refresh()
	b.subtitle.SetText(text)
	b.applyBtn.SetText(b.applyLabel())
}

func (b *builderView) refreshMessages() {
	if b.root != nil {
		b.root.Refresh()
	}
}

func (b *builderView) setError(text string) {
	b.setNotice(text, toneDanger)
}

func (b *builderView) setNotice(text string, t tone) {
	if b.messages == nil {
		return
	}
	clearBox(b.messages)
	if text != "" {
		b.messages.Add(messageCard(text, t))
		b.messages.Show()
	} else {
		b.messages.Hide()
	}
	b.messages.Refresh()
	b.refreshMessages()
}

func (b *builderView) clearMessages() {
	b.setNotice("", toneNeutral)
}

func (b *builderView) markDirty() {
	if b.suppressDirty || b.dirty {
		return
	}
	b.dirty = true
	b.updateHeader()
}

func (b *builderView) back() { b.leave(b.app.showAgents) }

// leave runs next at once, or after the operator agrees to lose unapplied
// edits: every way out of the builder asks the same question.
func (b *builderView) leave(next func()) {
	if !b.dirty {
		next()
		return
	}
	showConfirmPrompt(b.app.win, "Discard changes?",
		"The agent has unsaved edits in this window. Leave without applying them?",
		"Discard", true,
		func(ok bool) {
			if ok {
				next()
			}
		})
}

func (b *builderView) agentName() string {
	return strings.TrimSpace(b.nameEntry.Text)
}

func (b *builderView) setBusy(busy bool, verb string) {
	b.busy = busy
	b.validateBtn.Disable()
	b.applyBtn.Disable()
	b.applyMenu.Disable()
	b.deleteBtn.Disable()
	b.backBtn.Disable()
	if busy {
		b.activity.Show()
		b.activity.Start()
		if verb != "" {
			b.applyBtn.SetText(verb)
		}
		return
	}
	b.activity.Stop()
	b.activity.Hide()
	b.validateBtn.Enable()
	b.applyBtn.Enable()
	b.applyMenu.Enable()
	b.applyBtn.SetText(b.applyLabel())
	b.backBtn.Enable()
	if !b.isNew {
		b.deleteBtn.Enable()
	}
}

func (b *builderView) validate() {
	if b.busy {
		return
	}
	name := b.agentName()
	if name == "" {
		b.setError("An agent name is required.")
		return
	}
	if !agentNameRe.MatchString(name) {
		b.setError("The name must be 1-64 characters of letters, digits, _ or -.")
		return
	}
	yaml, err := b.yamlText()
	if err != nil {
		b.setError(err.Error())
		return
	}
	b.clearMessages()
	b.setBusy(true, "Checking…")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := b.app.api.Validate(ctx, api.ValidateRequest{
			Name:         name,
			AgentYAML:    yaml,
			Loop:         b.loop.Text(),
			Instructions: b.instructions.Text(),
		})
		onUI(func() {
			b.setBusy(false, "")
			if err != nil {
				b.setError(err.Error())
				return
			}
			if !res.OK {
				b.setError(strings.Join(res.Errors, "\n"))
				return
			}
			msg := "Valid. The loop compiles and the definition passes boot validation."
			if len(res.Warnings) > 0 {
				msg += "\nWarnings: " + strings.Join(res.Warnings, "; ")
				b.setNotice(msg, toneWarning)
			} else {
				b.setNotice(msg, toneSuccess)
			}
			b.app.setStatus("validation passed", toneSuccess)
		})
	}()
}

func (b *builderView) apply() { b.applyWith(b.persist, nil) }

// applyWith applies the definition (and saves it with persist); after runs
// once the new definition is live.
func (b *builderView) applyWith(persist bool, after func()) {
	if b.busy {
		return
	}
	name := b.agentName()
	if name == "" {
		b.setError("An agent name is required.")
		return
	}
	if !agentNameRe.MatchString(name) {
		b.setError("The name must be 1-64 characters of letters, digits, _ or -.")
		return
	}
	yaml, err := b.yamlText()
	if err != nil {
		b.setError(err.Error())
		return
	}
	b.clearMessages()
	b.setBusy(true, "Applying…")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		res, err := b.app.api.PutAgent(ctx, name, api.AgentRequest{
			AgentYAML:    yaml,
			Loop:         b.loop.Text(),
			Instructions: b.instructions.Text(),
			Apply:        true,
			Persist:      persist,
			ExpectMtime:  b.mtime,
		})
		if err != nil {
			onUI(func() {
				b.setBusy(false, "")
				b.setError(err.Error())
			})
			return
		}
		var detail *api.AgentDetail
		if d, derr := b.app.api.Agent(ctx, name); derr == nil {
			detail = &d
		}
		onUI(func() {
			b.setBusy(false, "")
			b.dirty = false
			if b.isNew {
				b.isNew = false
				b.name = name
				b.suppressDirty = true
				b.nameEntry.SetText(name)
				b.nameEntry.Disable()
				b.suppressDirty = false
				b.deleteBtn.Enable()
				b.app.setBreadcrumb("Agents / " + name)
			}
			if detail != nil {
				b.detail = detail
				b.mtime = detail.Files.Mtime
			}
			b.updateHeader()
			switch {
			case res.PersistError != "":
				b.setNotice("Applied live, but persisting failed: "+res.PersistError, toneWarning)
			case res.Persisted:
				b.setNotice("Applied live and persisted. The definition loads at the next boot.", toneSuccess)
			default:
				b.setNotice("Applied live on this instance.", toneSuccess)
			}
			b.app.setStatus("applied "+name, toneSuccess)
			if after != nil {
				after()
			}
		})
	}()
}

// buildYAML renders the form as the agent document. JSON values are valid
// YAML; the runtime overrides the loop and instructions paths on apply.
func (b *builderView) buildYAML() (string, error) {
	var sb strings.Builder
	write := func(key string, v any) error {
		j, err := json.Marshal(v)
		if err != nil {
			return err
		}
		sb.WriteString(key)
		sb.WriteString(": ")
		sb.Write(j)
		sb.WriteByte('\n')
		return nil
	}

	name := b.agentName()
	loopRef := "./" + name + ".lua"
	if name == "" {
		loopRef = "./agent.lua"
	}
	if b.detail != nil && b.detail.Agent.Loop != "" {
		loopRef = b.detail.Agent.Loop
	}
	if err := write("loop", loopRef); err != nil {
		return "", err
	}
	if b.model.Selected != "" && b.model.Selected != defaultModelOption {
		if err := write("model", b.model.Selected); err != nil {
			return "", err
		}
	}
	if b.safety.Selected != "" {
		if err := write("safety", b.safety.Selected); err != nil {
			return "", err
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(b.history.Text)); err == nil && n > 0 {
		if err := write("history_budget", n); err != nil {
			return "", err
		}
	}
	if b.persistent.Checked {
		if err := write("persistent", true); err != nil {
			return "", err
		}
	}
	if caps := selectedChecks(b.capsChecks); len(caps) > 0 {
		if err := write("capabilities", caps); err != nil {
			return "", err
		}
	}
	if skills := selectedChecks(b.skillChecks); len(skills) > 0 {
		if err := write("skills", skills); err != nil {
			return "", err
		}
	}
	if cc := splitList(b.canContact.Text); len(cc) > 0 {
		if err := write("can_contact", cc); err != nil {
			return "", err
		}
	}
	if creds := splitList(b.credentials.Text); len(creds) > 0 {
		if err := write("credentials", creds); err != nil {
			return "", err
		}
	}
	if shell := strings.TrimSpace(b.shell.Text); shell != "" {
		if err := write("shell", shell); err != nil {
			return "", err
		}
	}
	switch mem := b.memoryValues[b.memory.Selected]; mem {
	case "":
	case memoryInline:
		if b.detail != nil && len(b.detail.Agent.MemoryInline) > 0 {
			if err := write("memory", b.detail.Agent.MemoryInline); err != nil {
				return "", err
			}
		}
	default:
		if err := write("memory", mem); err != nil {
			return "", err
		}
	}
	if extras := strings.TrimSpace(b.extras.Text); extras != "" {
		var v any
		if err := json.Unmarshal([]byte(extras), &v); err != nil {
			return "", fmt.Errorf("extras is not valid JSON: %w", err)
		}
		if err := write("extras", v); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

func (b *builderView) yamlText() (string, error) {
	if b.rawDirty {
		return b.yaml.Text(), nil
	}
	return b.buildYAML()
}
