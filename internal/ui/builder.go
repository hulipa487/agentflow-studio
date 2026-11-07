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
	extras        *widget.Entry
	loop          *codeEditor
	instructions  *widget.Entry
	yaml          *widget.Entry
	rawDirty      bool
	suppressDirty bool
	dirty         bool

	persist     *widget.Check
	messages    *fyne.Container
	title       *canvas.Text
	subtitle    *widget.Label
	stateDot    *canvas.Circle
	validateBtn *widget.Button
	applyBtn    *widget.Button
	testBtn     *widget.Button
	deleteBtn   *widget.Button
	backBtn     *widget.Button
	activity    *widget.Activity
	luaEditor   *luaEditor
	tabs        *container.AppTabs
	busy        bool
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
		a.saveAction = b.apply
		a.backAction = b.back
		return
	}

	a.setMain(container.NewCenter(widget.NewLabel("Loading agent…")))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		d, err := a.api.Agent(ctx, name)
		fyne.Do(func() {
			if err != nil {
				a.showError(err)
				a.showAgents()
				return
			}
			b := a.newBuilderView(name, &d, nil)
			a.setMain(b.root)
			a.saveAction = b.apply
			a.backAction = b.back
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
		"Luau · inline diagnostics · Ctrl+Space for API",
		loopSource, a.catalog.PreludeVersion, b.effectiveCapabilities)
	b.luaEditor.onEdit = b.markDirty
	b.loop = b.luaEditor.editor
	a.luaEditor = b.luaEditor

	b.instructions = widget.NewMultiLineEntry()
	b.instructions.Wrapping = fyne.TextWrapWord
	b.instructions.SetMinRowsVisible(16)
	if d != nil {
		b.instructions.SetText(d.Instructions)
	}

	b.yaml = monoEntry(true)
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

	b.persist = widget.NewCheck("Save to config", nil)
	b.persist.OnChanged = func(bool) { b.applyBtn.SetText(b.applyLabel()) }

	b.validateBtn = actionButton("Validate", checkMarkIcon, b.validate)
	b.applyBtn = primaryButton("Apply", theme.UploadIcon(), b.apply)
	b.testBtn = actionButton("Test", theme.MediaPlayIcon(), func() { a.showPlayground(b.name) })
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
	b.extras.OnChanged = func(string) { b.markDirty() }
	b.instructions.OnChanged = func(string) { b.markDirty() }
	b.yaml.OnChanged = func(string) {
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
	)
	identityCard := section("Identity", "Model and context budget.", identityGrid)

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
		"Runtime, fleet contact and credentials. Memory profiles and stores remain in Raw YAML.",
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
		a.win.Clipboard().SetContent(b.instructions.Text)
		a.setStatus("instructions copied", toneSuccess)
	})
	instrTab := editorFrame("Instructions",
		"Saved next to the loop as a prompt file; the definition points at it.",
		b.instructions, copyInstr)

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
		a.win.Clipboard().SetContent(b.yaml.Text)
		a.setStatus("YAML copied", toneSuccess)
	})
	yamlTab := editorFrame("Raw YAML",
		"Sent to the runtime. The loop and instructions sources above are sent separately; editing here overrides the form until you rebuild.",
		b.yaml, rebuild, copyYAML)

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

	// The right column is the inline Lua guide; Status/Files facts now live
	// in the title line (state) and the deployment config.
	inspector := b.luaDocsPane()

	// Balanced two-row header: title & state with Test/Delete on top, and the
	// save/validate/apply toolbar strip underneath.
	status := hstack(spaceXS, vCenter(dotBox(b.stateDot)), vCenter(compactLabel(b.subtitle, true)))
	titleLeft := hstack(spaceXS, vCenter(b.backBtn), vCenter(b.title), hSpacer(spaceXS), vCenter(status))
	titleRight := hstack(spaceXS, vCenter(widthBox(84, b.testBtn)), vCenter(b.deleteBtn))
	titleRow := container.NewBorder(nil, nil, titleLeft, titleRight, nil)

	actionsRight := hstack(spaceXS,
		vCenter(b.activity),
		vCenter(widthBox(100, b.validateBtn)),
		vCenter(widthBox(124, b.applyBtn)),
	)
	toolbarRow := container.NewBorder(nil, nil,
		vCenter(compactCheck(b.persist)),
		actionsRight,
		nil,
	)
	head := vstack(spaceXS,
		container.New(layout.NewCustomPaddedLayout(spaceS, 2, pageGutter, pageGutter), vstack(2, titleRow, toolbarRow)),
		hInset(b.messages),
		widget.NewSeparator(),
	)

	tabsPane := container.New(layout.NewCustomPaddedLayout(spaceXS, 0, spaceM, spaceM), tabs)
	inspectorPane := container.New(layout.NewCustomPaddedLayout(pageTop, pageBottom, spaceM, pageGutter), inspector)
	split := container.NewHSplit(container.NewBorder(head, nil, nil, nil, tabsPane), inspectorPane)
	split.SetOffset(0.68)
	b.root = split

	b.updateHeader()
	if b.isNew {
		b.testBtn.Disable()
		b.deleteBtn.Disable()
	}
	return b
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
	if b.persist.Checked {
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

func (b *builderView) back() {
	if !b.dirty {
		b.app.showAgents()
		return
	}
	showConfirmPrompt(b.app.win, "Discard changes?",
		"The agent has unsaved edits in this window. Leave without applying them?",
		"Discard", true,
		func(ok bool) {
			if ok {
				b.app.showAgents()
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
	b.testBtn.Disable()
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
	b.applyBtn.SetText(b.applyLabel())
	b.backBtn.Enable()
	if !b.isNew {
		b.testBtn.Enable()
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
			Instructions: b.instructions.Text,
		})
		fyne.Do(func() {
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

func (b *builderView) apply() {
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
	persist := b.persist.Checked
	b.clearMessages()
	b.setBusy(true, "Applying…")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		res, err := b.app.api.PutAgent(ctx, name, api.AgentRequest{
			AgentYAML:    yaml,
			Loop:         b.loop.Text(),
			Instructions: b.instructions.Text,
			Apply:        true,
			Persist:      persist,
			ExpectMtime:  b.mtime,
		})
		if err != nil {
			fyne.Do(func() {
				b.setBusy(false, "")
				b.setError(err.Error())
			})
			return
		}
		var detail *api.AgentDetail
		if d, derr := b.app.api.Agent(ctx, name); derr == nil {
			detail = &d
		}
		fyne.Do(func() {
			b.setBusy(false, "")
			b.dirty = false
			if b.isNew {
				b.isNew = false
				b.name = name
				b.suppressDirty = true
				b.nameEntry.SetText(name)
				b.nameEntry.Disable()
				b.suppressDirty = false
				b.testBtn.Enable()
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
		return b.yaml.Text, nil
	}
	return b.buildYAML()
}
