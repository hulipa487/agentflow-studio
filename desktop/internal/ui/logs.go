package ui

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// logBufferLines bounds what the Logs screen keeps in memory.
const logBufferLines = 5000

// logLevels orders the runtime's short level names (llog.ShortName).
var logLevels = []string{"DEV", "DBG", "INF", "WRN", "ERR"}

// logFloors are the level floors the toolbar offers: the message key of each
// option and the lowest level it shows.
var logFloors = []struct {
	key   string
	level int
}{
	{"logs.floorDebug", 1},
	{"logs.floorInfo", 2},
	{"logs.floorWarn", 3},
	{"logs.floorError", 4},
	{"logs.floorAll", 0},
}

func logLevelOf(line string) int {
	// "15:04:05 INF message k=v": the level is the second field.
	f := strings.Fields(line)
	if len(f) >= 2 {
		for i, l := range logLevels {
			if f[1] == l {
				return i
			}
		}
	}
	return 2
}

type logsView struct {
	app *App
	s   *screen

	lines    []string
	filtered []string
	list     *widget.List
	filter   string
	minLevel int
	paused   bool
	follow   bool
	pending  int
	status   *widget.Label
	pauseBtn *widget.Button
}

func (a *App) showLogs() {
	a.enter("logs")
	v := a.newLogsView()
	a.setMain(v.s.root)
	v.start(a.viewContext())
	a.refreshAction = func() {
		v.lines = nil
		v.apply()
		v.start(a.viewContext())
	}
}

func (a *App) newLogsView() *logsView {
	v := &logsView{app: a, minLevel: 1, follow: true}
	v.list = widget.NewList(
		func() int { return len(v.filtered) },
		func() fyne.CanvasObject {
			bar := canvas.NewRectangle(color.Transparent)
			bar.SetMinSize(fyne.NewSize(3, 0))
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.SizeName = theme.SizeNameCaptionText
			l.Wrapping = fyne.TextWrapOff
			l.Truncation = fyne.TextTruncateEllipsis
			return container.NewBorder(nil, nil, bar, nil, l)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(v.filtered) {
				return
			}
			line := v.filtered[id]
			box := o.(*fyne.Container)
			var l *widget.Label
			var bar *canvas.Rectangle
			for _, obj := range box.Objects {
				switch t := obj.(type) {
				case *widget.Label:
					l = t
				case *canvas.Rectangle:
					bar = t
				}
			}
			if l == nil || bar == nil {
				return
			}
			l.SetText(line)
			switch logLevelOf(line) {
			case 4:
				l.Importance = widget.DangerImportance
				bar.FillColor = toneColor(toneDanger)
			case 3:
				l.Importance = widget.WarningImportance
				bar.FillColor = toneColor(toneWarning)
			case 0, 1:
				l.Importance = widget.LowImportance
				bar.FillColor = color.Transparent
			default:
				l.Importance = widget.MediumImportance
				bar.FillColor = color.Transparent
			}
			bar.Refresh()
			l.Refresh()
		},
	)
	v.list.OnSelected = func(id widget.ListItemID) {
		if id < len(v.filtered) {
			v.app.win.Clipboard().SetContent(v.filtered[id])
			v.app.setStatus(T("logs.lineCopied"), toneSuccess)
		}
		v.list.UnselectAll()
	}

	search := newSearchBox(T("logs.filterPlaceholder"), func(q string) {
		v.filter = strings.ToLower(strings.TrimSpace(q))
		v.apply()
	})
	floors := make([]string, len(logFloors))
	for i, f := range logFloors {
		floors[i] = T(f.key)
	}
	var level *widget.Select
	level = widget.NewSelect(floors, func(string) {
		if i := level.SelectedIndex(); i >= 0 && i < len(logFloors) {
			v.minLevel = logFloors[i].level
		}
		v.apply()
	})
	level.SetSelectedIndex(0)
	v.pauseBtn = actionButton(T("logs.pause"), theme.MediaPauseIcon(), v.togglePause)
	clear := actionButton(T("common.clear"), theme.ContentClearIcon(), func() {
		v.lines = nil
		v.apply()
	})
	copyAll := iconButton(theme.ContentCopyIcon(), T("logs.copyVisible"), func() {
		v.app.win.Clipboard().SetContent(strings.Join(v.filtered, "\n"))
		v.app.setStatus(T("logs.linesCopied", "n", len(v.filtered)), toneSuccess)
	})
	toolbar := container.NewBorder(nil, nil, nil,
		hstack(spaceS, widthBox(190, level), v.pauseBtn, clear, copyAll),
		search.Root)
	content := container.NewBorder(toolbar, nil, nil, nil, panel(v.list))
	v.s = a.newPane(T("logs.title"), T("logs.subtitle"), nil, content)
	v.status = v.s.meta
	v.status.SetText(T("logs.connecting"))
	return v
}

func (v *logsView) start(ctx context.Context) {
	c := v.app.api
	go func() {
		backoff := time.Second
		for ctx.Err() == nil {
			// Every (re)connect replays the runtime's ring, so start from it
			// rather than appending a second copy of the history.
			onUI(func() {
				if ctx.Err() == nil {
					v.lines = nil
					v.apply()
					v.status.SetText(T("logs.streaming", "url", c.DisplayURL))
				}
			})
			err := c.Logs(ctx, func(line string) {
				onUI(func() {
					if ctx.Err() == nil {
						v.push(line)
					}
				})
			})
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}
			seconds := int(backoff / time.Second)
			msg := T("logs.endedReconnecting", "seconds", seconds)
			if err != nil {
				msg = T("logs.reconnecting", "message", err.Error(), "seconds", seconds)
			}
			onUI(func() { v.status.SetText(msg) })
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 15*time.Second {
				backoff *= 2
			}
		}
	}()
}

func (v *logsView) push(line string) {
	v.lines = append(v.lines, line)
	if len(v.lines) > logBufferLines {
		v.lines = v.lines[len(v.lines)-logBufferLines:]
	}
	if v.paused {
		v.pending++
		v.pauseBtn.SetText(T("logs.resumePending", "n", v.pending))
		return
	}
	if v.matches(line) {
		v.filtered = append(v.filtered, line)
		if len(v.filtered) > logBufferLines {
			v.filtered = v.filtered[len(v.filtered)-logBufferLines:]
		}
		v.list.Refresh()
		if v.follow {
			v.list.ScrollToBottom()
		}
	}
}

func (v *logsView) matches(line string) bool {
	if logLevelOf(line) < v.minLevel {
		return false
	}
	return v.filter == "" || strings.Contains(strings.ToLower(line), v.filter)
}

func (v *logsView) apply() {
	v.filtered = v.filtered[:0]
	for _, l := range v.lines {
		if v.matches(l) {
			v.filtered = append(v.filtered, l)
		}
	}
	v.list.Refresh()
	if v.follow {
		v.list.ScrollToBottom()
	}
}

func (v *logsView) togglePause() {
	v.paused = !v.paused
	if v.paused {
		v.pending = 0
		v.pauseBtn.SetText(T("logs.resume"))
		v.pauseBtn.SetIcon(theme.MediaPlayIcon())
		return
	}
	v.pending = 0
	v.pauseBtn.SetText(T("logs.pause"))
	v.pauseBtn.SetIcon(theme.MediaPauseIcon())
	v.apply()
}
