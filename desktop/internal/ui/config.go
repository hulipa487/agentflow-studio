package ui

import (
	"context"
	"encoding/json"
	"errors"
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
	a.unsaved = func() bool { return v.dirty() }
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

	validate := actionButton(T("config.validate"), theme.ConfirmIcon(), v.validate)
	v.saveBtn = primaryButton(T("common.save"), theme.DocumentSaveIcon(), v.save)
	reload := actionButton(T("config.reload"), theme.ViewRefreshIcon(), func() { v.reload(true) })
	actions := hstack(spaceS, reload, validate, v.saveBtn)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(T("common.edit"), theme.DocumentCreateIcon(),
			editorFrame("agentflow.yaml", T("config.editHint", "placeholder", "${VAR}", "mod", "Ctrl"), editArea.root)),
		container.NewTabItemWithIcon(T("config.tabMasked"), theme.VisibilityIcon(),
			editorFrame(T("config.parsedTitle"), T("config.maskedHint"), maskedArea.root)),
	)
	v.s = a.newPane(T("config.title"), T("config.subtitle"), actions, tabs)
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
		parts = append(parts, T("config.modified", "time", time.Unix(v.mtime, 0).Local().Format("2006-01-02 15:04:05")))
	}
	lines := len(v.editor.lines)
	parts = append(parts, T("config.lines", "n", lines))
	if v.dirty() {
		parts = append(parts, T("config.unsaved"))
	}
	v.s.meta.SetText(strings.Join(parts, " · "))
}

func (v *configView) reload(confirm bool) {
	if confirm && v.dirty() {
		showConfirmPrompt(v.app.win, T("config.discardTitle"), T("config.reloadDiscard"), T("common.discard"), true, func(ok bool) {
			if ok {
				v.reload(false)
			}
		})
		return
	}
	v.s.meta.SetText(T("config.loadingMeta"))
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
			v.s.meta.SetText(T("config.directoryMeta"))
			v.s.notice(f.Error+"\n\n"+T("config.directoryNote"), toneWarning)
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
	v.s.notice(T("config.validating"), toneNeutral)
	async(30*time.Second, func(ctx context.Context) (api.ConfigResult, error) {
		return v.app.api.ValidateConfig(ctx, raw)
	}, func(r api.ConfigResult, err error) {
		switch {
		case err != nil:
			v.s.notice(err.Error(), toneDanger)
		case !r.OK:
			v.s.notice(T("config.invalid", "error", r.Error), toneDanger)
		default:
			v.s.notice(T("config.valid"), toneSuccess)
		}
	})
}

func (v *configView) save() {
	if !v.loaded {
		return
	}
	if !v.dirty() {
		v.app.setStatus(T("config.noChanges"), toneNeutral)
		return
	}
	raw, mtime := v.editor.Text(), v.mtime
	showConfirmPrompt(v.app.win, T("config.saveTitle"),
		T("config.saveMessage", "path", v.path),
		T("common.save"), false, func(ok bool) {
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
					v.s.notice(T("config.conflict"), toneWarning)
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
						note = T("config.saved")
					}
					v.s.notice(note, toneSuccess)
					v.app.reloadCatalog()
				}
			})
		})
}
