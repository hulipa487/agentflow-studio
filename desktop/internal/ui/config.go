package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

// configView is the raw config editor. Everything AgentFlow can be configured
// with but Studio has no structured screen for (memory backends and profiles,
// prompts, tools.policy and tools.http, MCP servers, triggers, search, files,
// media, net, usage, audit, runtime.*) is edited here, through the same
// boot-equivalent validation and atomic, backed-up save the browser console
// uses.
type configView struct {
	app *App
	s   *screen

	editor   *codeEditor
	masked   *codeEditor
	saveBtn  *widget.Button
	path     string
	mtime    int64
	original string
	loaded   bool
}

func (a *App) showConfig() {
	a.enter("config")
	v := a.newConfigView()
	a.setMain(v.s.root)
	a.saveAction = v.save
	a.refreshAction = func() { v.reload(true) }
}

func (a *App) newConfigView() *configView {
	v := &configView{app: a}
	editArea := newCodeAreaText("", "yaml")
	v.editor = editArea.editor
	v.editor.onChanged = v.updateMeta
	maskedArea := newCodeAreaText("", "json")
	v.masked = maskedArea.editor
	v.masked.SetReadOnly(true)

	validate := actionButton("Validate", theme.ConfirmIcon(), v.validate)
	v.saveBtn = primaryButton("Save", theme.DocumentSaveIcon(), v.save)
	reload := actionButton("Reload", theme.ViewRefreshIcon(), func() { v.reload(true) })
	actions := hstack(spaceS, reload, validate, v.saveBtn)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Edit", theme.DocumentCreateIcon(),
			editorFrame("agentflow.yaml", "Env placeholders stay as written (${VAR}); secrets are never resolved into this text. Ctrl+S validates and saves.", editArea.root)),
		container.NewTabItemWithIcon("Masked view", theme.VisibilityIcon(),
			editorFrame("Parsed config", "What the runtime parsed from the file on disk, with sensitive keys masked. Read-only.", maskedArea.root)),
	)
	v.s = a.newPane("Config",
		"The deployment's config file, for everything without its own screen: memory, prompts, tools, MCP, triggers, search, files, net and runtime settings. Saving keeps a .bak; topology changes apply at the next restart.",
		actions, tabs)
	v.reload(false)
	return v
}

func (v *configView) dirty() bool { return v.loaded && v.editor.Text() != v.original }

func (v *configView) updateMeta() {
	if !v.loaded {
		return
	}
	parts := []string{v.path}
	if v.mtime > 0 {
		parts = append(parts, "modified "+time.Unix(v.mtime, 0).Local().Format("2006-01-02 15:04:05"))
	}
	lines := len(v.editor.lines)
	parts = append(parts, fmt.Sprintf("%d lines", lines))
	if v.dirty() {
		parts = append(parts, "● unsaved changes")
	}
	v.s.meta.SetText(strings.Join(parts, " · "))
}

func (v *configView) reload(confirm bool) {
	if confirm && v.dirty() {
		showConfirmPrompt(v.app.win, "Discard changes", "Reload the file from disk and discard your unsaved edits?", "Discard", true, func(ok bool) {
			if ok {
				v.reload(false)
			}
		})
		return
	}
	v.s.meta.SetText("loading…")
	async(20*time.Second, v.app.api.Config, func(f api.ConfigFile, err error) {
		if err != nil {
			v.s.notice(err.Error(), toneDanger)
			return
		}
		if !f.OK {
			v.loaded = false
			v.editor.SetText("")
			v.editor.SetReadOnly(true)
			v.saveBtn.Disable()
			v.s.meta.SetText("config directory deployment")
			v.s.notice(f.Error+"\n\nIn a directory deployment Studio edits the fragments through Agents, Channels, Models and Safety; other sections are edited in the files themselves.", toneWarning)
			return
		}
		v.s.banner.Clear()
		v.path, v.mtime = f.Path, f.Mtime
		v.original = f.Raw
		v.loaded = true
		v.editor.SetReadOnly(false)
		v.saveBtn.Enable()
		v.editor.SetText(f.Raw)
		if f.View != nil {
			if b, err := json.MarshalIndent(f.View, "", "  "); err == nil {
				v.masked.SetText(string(b))
			}
		}
		v.updateMeta()
	})
}

func (v *configView) validate() {
	if !v.loaded {
		return
	}
	raw := v.editor.Text()
	v.s.notice("Validating…", toneNeutral)
	async(30*time.Second, func(ctx context.Context) (api.ConfigResult, error) {
		return v.app.api.ValidateConfig(ctx, raw)
	}, func(r api.ConfigResult, err error) {
		switch {
		case err != nil:
			v.s.notice(err.Error(), toneDanger)
		case !r.OK:
			v.s.notice("Does not validate: "+r.Error, toneDanger)
		default:
			v.s.notice("Valid: the runtime would boot with this config (strict fields, env expansion and cross-references checked).", toneSuccess)
		}
	})
}

func (v *configView) save() {
	if !v.loaded {
		return
	}
	if !v.dirty() {
		v.app.setStatus("no changes to save", toneNeutral)
		return
	}
	raw, mtime := v.editor.Text(), v.mtime
	showConfirmPrompt(v.app.win, "Save config",
		"Validate and replace "+v.path+"? The current file is kept as .bak. Most changes apply when AgentFlow restarts.",
		"Save", false, func(ok bool) {
			if !ok {
				return
			}
			v.saveBtn.Disable()
			async(30*time.Second, func(ctx context.Context) (api.ConfigResult, error) {
				return v.app.api.SaveConfig(ctx, raw, mtime)
			}, func(r api.ConfigResult, err error) {
				v.saveBtn.Enable()
				var ae *api.Error
				switch {
				case errors.As(err, &ae) && ae.Status == 409:
					v.s.notice("The file changed on disk since it was loaded. Reload it (your edits are discarded) and apply them again.", toneWarning)
				case err != nil:
					v.s.notice(err.Error(), toneDanger)
				case !r.OK:
					v.s.notice(r.Error, toneDanger)
				default:
					v.original = raw
					if r.Mtime > 0 {
						v.mtime = r.Mtime
					}
					v.updateMeta()
					note := r.Note
					if note == "" {
						note = "saved; restart agentflow to apply"
					}
					v.s.notice(note, toneSuccess)
					v.app.reloadCatalog()
				}
			})
		})
}
