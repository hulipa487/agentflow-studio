package ui

import (
	"context"
	"encoding/json"
	"fmt"
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

type playgroundView struct {
	app    *App
	agent  string
	thread string
	root   fyne.CanvasObject

	agentSelect  *widget.Select
	avatarBox    *fyne.Container
	threadBox    *fyne.Container
	messages     *fyne.Container
	msgScroll    *container.Scroll
	traceBox     *fyne.Container
	traceScroll  *container.Scroll
	input        *widget.Entry
	actAs        *widget.SelectEntry
	sendBtn      *widget.Button
	stopBtn      *widget.Button
	activity     *widget.Activity
	status       *widget.Label
	statusDot    *canvas.Circle
	headerName   *widget.Label
	headerThread *widget.Label
	tokens       *widget.Label

	assistant *widget.Label
	sending   bool
	events    []api.TraceEvent
	filter    *widget.Select
	empty     fyne.CanvasObject
}

func (a *App) showPlayground(agent string) {
	a.enter("playground")
	a.setMain(a.playgroundPage(nil, container.NewCenter(muted("Loading agents…"))))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rows, err := a.api.Agents(ctx)
		fyne.Do(func() {
			if err != nil {
				a.showError(err)
				retry := widthBox(120, primaryButton("Retry", nil, func() { a.showPlayground("") }))
				body := container.NewVBox(
					container.NewCenter(wrapLabel(err.Error())),
					container.NewCenter(retry),
				)
				a.setMain(a.playgroundPage(nil, container.NewCenter(container.NewPadded(body))))
				return
			}
			names := make([]string, 0, len(rows))
			for _, r := range rows {
				if !r.Spawn {
					names = append(names, r.Name)
				}
			}
			if len(names) == 0 {
				body := emptyStateCard(
					"No agents yet",
					"Create an agent to start a chat and trace its turns.",
					"Create an agent",
					a.showAgents,
				)
				a.setMain(a.playgroundPage(nil, body))
				return
			}
			if agent == "" {
				agent = names[0]
			}
			p := a.newPlaygroundView(names, agent)
			a.setMain(p.root)
			a.sendAction = p.send
		})
	}()
}

// playgroundPage is the one shell for every Playground state: loading, error,
// empty and the live three-pane workspace.
func (a *App) playgroundPage(actions fyne.CanvasObject, body fyne.CanvasObject) fyne.CanvasObject {
	head := pageHeader("Playground",
		"Chat with an agent turn by turn. Each thread is its own session, with every operation traced.",
		actions)
	return container.NewBorder(
		vstack(0, pageInset(head), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(body),
	)
}

func (a *App) newPlaygroundView(names []string, agent string) *playgroundView {
	if agent == "" {
		agent = "agent"
	}
	p := &playgroundView{app: a, agent: agent, thread: newThreadID()}

	p.agentSelect = widget.NewSelect(names, func(name string) {
		if name == "" || name == p.agent {
			return
		}
		p.agent = name
		p.thread = newThreadID()
		p.clearChat()
		p.updateHeader()
		p.setStatus("ready", toneSuccess)
		p.loadThreads()
	})
	p.agentSelect.SetSelected(agent)

	newThread := primaryButton("New thread", theme.ContentAddIcon(), func() {
		p.thread = newThreadID()
		p.clearChat()
		p.updateHeader()
		p.setStatus("new thread "+shortID(p.thread), toneAccent)
		p.loadThreads()
	})

	p.threadBox = vstack(spaceXS)
	threadScroll := container.NewVScroll(p.threadBox)

	p.actAs = widget.NewSelectEntry(nil)
	p.actAs.SetPlaceHolder("profile uuid")
	a.loadUserIDs(func(ids []string) { p.actAs.SetOptions(ids) })

	leftTop := vstack(spaceXS,
		labeled("Agent", p.agentSelect),
		vSpacer(spaceXS),
		fieldLabel("Recent threads"),
	)
	leftBottom := vstack(spaceXS,
		widget.NewSeparator(),
		labeled("Act as user", p.actAs),
		caption("Optional. Tenant-less turns skip personal scopes and grants."),
	)
	left := panel(container.NewBorder(leftTop, leftBottom, nil, nil, threadScroll))

	p.messages = vstack(spaceS)
	p.empty = container.NewCenter(container.NewPadded(vstack(spaceXS,
		container.NewCenter(logoMark(48)),
		container.NewCenter(widget.NewLabelWithStyle("Say hello", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})),
		centerNote("The turn streams into the trace panel on the right."),
	)))
	p.showEmpty()
	p.msgScroll = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceXS, spaceXS), p.messages))

	p.input = widget.NewMultiLineEntry()
	p.input.SetMinRowsVisible(3)
	p.input.SetPlaceHolder("Message the agent…")
	p.sendBtn = primaryButton("Send", theme.MailSendIcon(), p.send)
	composerBar := container.New(layout.NewCustomPaddedLayout(2, 4, spaceS, 4),
		container.NewBorder(nil, nil,
			vCenter(caption("Ctrl+Enter sends · Enter adds a newline")),
			container.NewGridWrap(fyne.NewSize(92, 34), p.sendBtn),
			nil,
		))
	cleanInput := container.NewThemeOverride(p.input, borderlessEntryTheme{})
	composer := inputFrame(container.NewBorder(nil, composerBar, nil, nil, cleanInput))

	p.statusDot = dot(toneSuccess)
	p.status = widget.NewLabel("ready")
	p.status.SizeName = theme.SizeNameCaptionText
	p.status.Wrapping = fyne.TextWrapOff
	p.status.Truncation = fyne.TextTruncateEllipsis
	p.activity = widget.NewActivity()
	p.activity.Hide()

	p.headerName = widget.NewLabelWithStyle(agent, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.headerName.SizeName = theme.SizeNameSubHeadingText
	p.headerThread = mono("thread · "+shortID(p.thread), theme.SizeNameCaptionText)
	p.tokens = caption("")
	p.avatarBox = container.NewStack(avatar(strings.ToUpper(agent[:1]), 34))

	p.stopBtn = iconButton(theme.MediaStopIcon(), "Stop session", p.stop)
	titleStack := vstack(0, p.headerName, p.headerThread)
	headLeft := container.NewBorder(nil, nil, vCenter(p.avatarBox), nil, vCenter(titleStack))
	headRight := hstack(spaceXS,
		vCenter(p.activity),
		vCenter(dotBox(p.statusDot)),
		vCenter(widthBox(110, p.status)),
		vCenter(p.stopBtn),
	)
	head := container.NewBorder(nil, nil, nil, headRight, headLeft)

	center := panel(container.NewBorder(
		vstack(spaceS, head, widget.NewSeparator()),
		vstack(spaceS, widget.NewSeparator(), composer),
		nil, nil, p.msgScroll,
	))

	p.traceBox = vstack(spaceXS)
	p.traceScroll = container.NewVScroll(p.traceBox)
	p.filter = widget.NewSelect([]string{"All events", "Ops only", "Errors only"}, func(string) {
		p.renderTrace()
	})
	p.filter.SetSelected("All events")
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh trace", p.refreshTrace)
	traceHead := container.NewBorder(nil, nil,
		vstack(1, insetLeading(titleText("Trace", theme.SizeNameSubHeadingText)), p.tokens),
		hstack(spaceXS, vCenter(controlFrame(p.filter)), vCenter(refresh)),
	)
	trace := panel(container.NewBorder(
		vstack(spaceS, traceHead, widget.NewSeparator()),
		nil, nil, nil, p.traceScroll,
	))

	inner := container.NewHSplit(left, center)
	inner.SetOffset(0.28)
	outer := container.NewHSplit(inner, trace)
	outer.SetOffset(0.67)

	p.root = a.playgroundPage(hstack(spaceS, newThread), outer)
	p.updateHeader()
	p.loadThreads()
	return p
}

func (p *playgroundView) updateHeader() {
	p.headerName.SetText(p.agent)
	p.headerThread.SetText("thread · " + shortID(p.thread))
	letter := "?"
	if p.agent != "" {
		letter = strings.ToUpper(p.agent[:1])
	}
	if p.avatarBox != nil {
		p.avatarBox.Objects = []fyne.CanvasObject{avatar(letter, 34)}
		p.avatarBox.Refresh()
	}
}

func (p *playgroundView) setStatus(text string, t tone) {
	p.status.SetText(text)
	switch t {
	case toneDanger:
		p.status.Importance = widget.DangerImportance
	case toneWarning:
		p.status.Importance = widget.WarningImportance
	case toneSuccess:
		p.status.Importance = widget.SuccessImportance
	default:
		p.status.Importance = widget.MediumImportance
	}
	p.status.Refresh()
	p.statusDot.FillColor = toneColor(t)
	p.statusDot.Refresh()
}

func (p *playgroundView) clearChat() {
	clearBox(p.messages)
	p.showEmpty()
	clearBox(p.traceBox)
	p.events = nil
	p.assistant = nil
	p.tokens.SetText("")
}

func (p *playgroundView) showEmpty() {
	if p.empty != nil {
		p.messages.Add(p.empty)
		p.messages.Refresh()
	}
}

func (p *playgroundView) loadThreads() {
	agent := p.agent
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		threads, err := p.app.api.Threads(ctx, agent)
		if err != nil {
			return
		}
		fyne.Do(func() {
			if p.agent != agent {
				return
			}
			clearBox(p.threadBox)
			for _, t := range threads {
				t := t
				label := fmt.Sprintf("%s · %d events", shortID(t.Thread), t.Events)
				b := widget.NewButton(label, func() { p.openThread(t.Thread) })
				b.Alignment = widget.ButtonAlignLeading
				if t.Thread == p.thread {
					b.Importance = widget.HighImportance
				} else {
					b.Importance = widget.LowImportance
				}
				p.threadBox.Add(b)
			}
			if len(threads) == 0 {
				p.threadBox.Add(muted("No threads yet."))
			}
			p.threadBox.Refresh()
		})
	}()
}

func (p *playgroundView) openThread(thread string) {
	p.thread = thread
	p.clearChat()
	p.updateHeader()
	p.setStatus("thread "+shortID(thread), toneAccent)
	agent := p.agent
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := p.app.api.Trace(ctx, agent, thread)
		if err != nil {
			fyne.Do(func() { p.app.showError(err) })
			return
		}
		fyne.Do(func() {
			p.events = events
			p.renderTrace()
		})
		p.loadThreads()
	}()
}

func (p *playgroundView) refreshTrace() {
	agent, thread := p.agent, p.thread
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := p.app.api.Trace(ctx, agent, thread)
		if err != nil {
			fyne.Do(func() { p.app.showError(err) })
			return
		}
		fyne.Do(func() {
			p.events = events
			p.renderTrace()
		})
	}()
}

func (p *playgroundView) stop() {
	if p.app.chatCancel != nil {
		p.app.chatCancel()
	}
	agent, thread := p.agent, p.thread
	p.setStatus("stopping…", toneWarning)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := p.app.api.StopThread(ctx, agent, thread)
		fyne.Do(func() {
			if err != nil {
				p.app.showError(err)
				p.setStatus("error", toneDanger)
				return
			}
			p.setStatus("stop requested", toneWarning)
		})
	}()
}

func (p *playgroundView) send() {
	if p.sending || p.agent == "" || strings.TrimSpace(p.input.Text) == "" {
		return
	}
	text := p.input.Text
	p.input.SetText("")
	p.sending = true
	p.sendBtn.Disable()
	p.setStatus("running…", toneAccent)
	clearBox(p.traceBox)
	p.events = nil

	p.addBubble("You", text, true)
	p.assistant = p.addBubble("Agent", "…", false)

	agent, thread, actAs := p.agent, p.thread, strings.TrimSpace(p.actAs.Text)
	ctx, cancel := context.WithCancel(context.Background())
	p.app.chatCancel = cancel

	go func() {
		err := p.app.api.Chat(ctx, api.ChatRequest{
			Agent:  agent,
			Thread: thread,
			Text:   text,
			ActAs:  actAs,
		}, func(ev api.ChatEvent) {
			fyne.Do(func() {
				switch ev.Type {
				case "trace":
					if ev.Trace != nil {
						p.events = append(p.events, *ev.Trace)
						p.renderTrace()
					}
				case "reply":
					if ev.Reply != nil {
						p.appendReply(p.assistant, ev.Reply.Text)
					}
				case "error":
					p.setStatus("error: "+ev.Error, toneDanger)
				case "done":
					if ev.Done != nil && ev.Done.TimedOut {
						p.setStatus("turn timed out; the loop is still running", toneWarning)
					}
				}
			})
		})
		canceled := ctx.Err() != nil
		fyne.Do(func() {
			p.sending = false
			p.sendBtn.Enable()
			switch {
			case canceled:
				p.setStatus("turn canceled", toneWarning)
			case err != nil:
				p.setStatus("error: "+err.Error(), toneDanger)
			case strings.HasPrefix(p.status.Text, "running"):
				p.setStatus("done", toneSuccess)
			}
			p.loadThreads()
		})
	}()
}

func (p *playgroundView) addBubble(role, text string, mine bool) *widget.Label {
	fill := colorSurface2
	stroke := colorBorder
	roleColor := colorMuted
	if mine {
		fill = colorAccentDim
		stroke = colorAccent
		roleColor = colorAccent
	}
	bg := canvas.NewRectangle(fill)
	bg.CornerRadius = 10
	bg.StrokeColor = stroke
	bg.StrokeWidth = 1

	// Drop the empty state once a real message exists.
	if p.empty != nil {
		for i, obj := range p.messages.Objects {
			if obj == p.empty {
				p.messages.Objects = append(p.messages.Objects[:i], p.messages.Objects[i+1:]...)
				break
			}
		}
	}

	roleTxt := canvas.NewText(strings.ToUpper(role), roleColor)
	roleTxt.TextSize = 10
	roleTxt.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}

	timeTxt := canvas.NewText(time.Now().Format("15:04"), colorFaint)
	timeTxt.TextSize = 10
	timeTxt.TextStyle = fyne.TextStyle{Monospace: true}

	body := wrapLabel(text)
	head := container.NewBorder(nil, nil,
		insetLeading(roleTxt),
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, 4), timeTxt),
		nil,
	)
	inner := container.New(layout.NewCustomPaddedLayout(8, 8, 10, 10), vstack(2, head, body))
	bubble := container.NewStack(bg, inner)

	row := container.New(bubbleRowLayout{mine: mine}, bubble)
	p.messages.Add(row)
	p.messages.Refresh()
	p.msgScroll.ScrollToBottom()
	return body
}

func (p *playgroundView) appendReply(label *widget.Label, text string) {
	if label == nil {
		return
	}
	if label.Text == "…" || label.Text == "" {
		label.SetText(text)
	} else {
		label.SetText(label.Text + "\n\n" + text)
	}
	p.msgScroll.ScrollToBottom()
}

func (p *playgroundView) renderTrace() {
	clearBox(p.traceBox)
	filter := ""
	if p.filter != nil {
		filter = p.filter.Selected
	}
	shown := 0
	for _, ev := range p.events {
		if filter == "Ops only" && ev.Op == "" {
			continue
		}
		if filter == "Errors only" && ev.Err == "" && ev.Kind != "error" {
			continue
		}
		p.traceBox.Add(p.traceRow(ev))
		shown++
	}
	if shown == 0 {
		p.traceBox.Add(muted("No events match this filter yet."))
	}
	p.traceBox.Refresh()
	p.traceScroll.ScrollToBottom()
	p.tokens.SetText(fmt.Sprintf("%d tok", traceTokens(p.events)))
}

func (p *playgroundView) traceRow(ev api.TraceEvent) fyne.CanvasObject {
	l := mono(traceLine(ev), theme.SizeNameCaptionText)
	l.Wrapping = fyne.TextWrapWord
	l.Truncation = fyne.TextTruncateClip
	rowTone := toneAccent
	switch {
	case !ev.OK && ev.Err != "":
		l.Importance = widget.DangerImportance
		rowTone = toneDanger
	case ev.Kind == "turn.start" || ev.Kind == "turn.end":
		l.Importance = widget.LowImportance
		rowTone = toneNeutral
	case ev.Kind == "reply":
		rowTone = toneSuccess
	}
	details := iconButton(theme.InfoIcon(), "Event details", func() { p.showEvent(ev) })
	leftDot := vCenter(dotBox(dot(rowTone)))
	row := container.NewBorder(nil, nil, leftDot, vCenter(details), l)
	return subpanel(row)
}

func (p *playgroundView) showEvent(ev api.TraceEvent) {
	raw, _ := json.MarshalIndent(ev, "", "  ")
	body := widget.NewLabel(string(raw))
	body.TextStyle = fyne.TextStyle{Monospace: true}
	body.Wrapping = fyne.TextWrapWord
	body.Selectable = true
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(560, 400))
	showModalCard(p.app.win, "Trace event #"+strconv.FormatInt(ev.Seq, 10), ev.Kind, inputFrame(scroll), nil)
}

// bubbleRowLayout keeps a bubble readable at any pane width: it caps the
// bubble at 420px, leaves a 8px gutter, and aligns it to the sender's side.
type bubbleRowLayout struct{ mine bool }

func (l bubbleRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (l bubbleRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) == 0 {
		return
	}
	bubble := objs[0]
	const gutter, maxWidth = float32(8), float32(420)
	w := size.Width - 2*gutter
	if w > maxWidth {
		w = maxWidth
	}
	if w < 120 {
		w = 120
	}
	bubble.Resize(fyne.NewSize(w, bubble.Size().Height))
	h := bubble.MinSize().Height
	bubble.Resize(fyne.NewSize(w, h))
	x := gutter
	if l.mine {
		x = size.Width - w - gutter
	}
	bubble.Move(fyne.NewPos(x, 0))
}

func traceLine(ev api.TraceEvent) string {
	parts := []string{fmt.Sprintf("#%d", ev.Seq), ev.Kind}
	if ev.Op != "" {
		parts = append(parts, ev.Op)
	}
	if ev.Tool != "" {
		parts = append(parts, ev.Tool)
	}
	if ev.Model != "" {
		parts = append(parts, ev.Model)
	}
	if ev.DurationMS > 0 {
		parts = append(parts, strconv.FormatInt(ev.DurationMS, 10)+"ms")
	}
	if ev.TokensIn+ev.TokensOut > 0 {
		parts = append(parts, fmt.Sprintf("%d→%d tok", ev.TokensIn, ev.TokensOut))
	}
	if ev.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d call(s)", ev.ToolCalls))
	}
	if ev.Err != "" {
		parts = append(parts, "ERR: "+ev.Err)
	}
	return strings.Join(parts, "  ")
}

func traceTokens(events []api.TraceEvent) int {
	n := 0
	for _, ev := range events {
		n += ev.TokensIn + ev.TokensOut
	}
	return n
}
