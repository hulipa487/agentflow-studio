package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
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

// chatSession is one conversation with an agent: the thread header, the
// transcript, the composer and the trace of each turn. The Playground and the
// builder's run panel both draw it; each adds its own surroundings.
type chatSession struct {
	app    *App
	agent  string
	thread string

	chatPanel  fyne.CanvasObject
	tracePanel fyne.CanvasObject

	messages     *fyne.Container
	msgScroll    *container.Scroll
	traceBox     *fyne.Container
	traceScroll  *container.Scroll
	input        *widget.Entry
	sendBtn      *widget.Button
	stopBtn      *widget.Button
	activity     *widget.Activity
	status       *widget.Label
	statusDot    *canvas.Circle
	headerName   *widget.Label
	headerThread *widget.Label
	tokens       *widget.Label
	filter       *widget.Select
	empty        fyne.CanvasObject

	assistant *widget.Label
	sending   bool
	events    []api.TraceEvent
	// epoch is bumped by every switch and every turn, so a late event from an
	// earlier turn, thread or agent is never drawn.
	epoch int
	// last is the most recent message sent, which an empty run repeats.
	last string

	// onSend is what Send and Ctrl+Enter do; the builder applies first.
	onSend func()
	// actAs names the user a turn runs as; nil runs tenant-less.
	actAs func() string
	// onTurnEnd runs after every turn (the Playground reloads its threads).
	onTurnEnd func()
}

type chatOptions struct {
	// hint is the line beside Send.
	hint string
	// emptyTitle and emptyNote fill the transcript before the first message.
	emptyTitle, emptyNote string
	// actions are extra header buttons, before Stop.
	actions []fyne.CanvasObject
	// statusWidth is the room the status word gets; 0 is 110px. A docked
	// panel passes less, so the agent's name keeps its place.
	statusWidth float32
}

func newChatSession(a *App, agent string, opts chatOptions) *chatSession {
	s := &chatSession{app: a, agent: agent, thread: newThreadID()}
	s.onSend = s.send

	if opts.hint == "" {
		opts.hint = "Ctrl+Enter sends · Enter adds a newline"
	}
	if opts.emptyTitle == "" {
		opts.emptyTitle = "Say hello"
	}
	if opts.emptyNote == "" {
		opts.emptyNote = "The turn streams into the trace panel."
	}

	s.messages = vstack(spaceS)
	s.empty = container.NewCenter(container.NewPadded(vstack(spaceXS,
		container.NewCenter(widget.NewLabelWithStyle(opts.emptyTitle, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})),
		centerNote(opts.emptyNote),
	)))
	s.showEmpty()
	s.msgScroll = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceXS, spaceXS), s.messages))

	s.input = widget.NewMultiLineEntry()
	s.input.SetMinRowsVisible(3)
	s.input.SetPlaceHolder("Message the agent…")
	s.sendBtn = primaryButton("Send", theme.MailSendIcon(), func() { s.onSend() })
	hint := caption(opts.hint)
	hint.Truncation = fyne.TextTruncateEllipsis
	composerBar := container.New(layout.NewCustomPaddedLayout(2, 4, spaceS, 4),
		container.NewBorder(nil, nil, nil,
			container.NewGridWrap(fyne.NewSize(92, 34), s.sendBtn),
			// A VBox gives the hint the bar's whole width, so it truncates
			// at the Send button instead of shrinking to its first letters.
			container.NewVBox(layout.NewSpacer(), hint, layout.NewSpacer()),
		))
	cleanInput := container.NewThemeOverride(s.input, borderlessEntryTheme{})
	composer := inputFrame(container.NewBorder(nil, composerBar, nil, nil, cleanInput))

	s.statusDot = dot(toneSuccess)
	s.status = widget.NewLabel("ready")
	s.status.SizeName = theme.SizeNameCaptionText
	s.status.Wrapping = fyne.TextWrapOff
	s.status.Truncation = fyne.TextTruncateEllipsis
	s.activity = widget.NewActivity()
	s.activity.Hide()

	s.headerName = widget.NewLabelWithStyle(agent, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.headerName.SizeName = theme.SizeNameSubHeadingText
	s.headerName.Truncation = fyne.TextTruncateEllipsis
	s.headerThread = mono("thread · "+shortID(s.thread), theme.SizeNameCaptionText)
	s.tokens = caption("")

	s.stopBtn = iconButton(theme.MediaStopIcon(), "Stop session", s.stop)
	titleStack := vstack(0, compactLabel(s.headerName, false), compactLabel(s.headerThread, false))
	if opts.statusWidth == 0 {
		opts.statusWidth = 110
	}
	right := []fyne.CanvasObject{vCenter(s.activity), vCenter(dotBox(s.statusDot)), vCenter(widthBox(opts.statusWidth, s.status))}
	for _, act := range opts.actions {
		right = append(right, vCenter(act))
	}
	right = append(right, vCenter(s.stopBtn))
	head := container.NewBorder(nil, nil, nil, hstack(spaceXS, right...), vLeft(titleStack))

	s.chatPanel = panel(container.NewBorder(
		vstack(spaceS, head, widget.NewSeparator()),
		vstack(spaceS, widget.NewSeparator(), composer),
		nil, nil, s.msgScroll,
	))

	s.traceBox = vstack(spaceXS)
	s.traceScroll = container.NewVScroll(s.traceBox)
	s.filter = widget.NewSelect([]string{"All events", "Ops only", "Errors only"}, func(string) {
		s.renderTrace()
	})
	s.filter.SetSelected("All events")
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh trace", s.refreshTrace)
	s.tokens.Wrapping = fyne.TextWrapOff
	traceHead := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(insetLeading(titleText("Trace", theme.SizeNameSubHeadingText))), vCenter(compactLabel(s.tokens, false))),
		hstack(spaceXS, vCenter(widthBox(124, controlFrame(s.filter))), vCenter(refresh)),
	)
	s.tracePanel = panel(container.NewBorder(
		vstack(spaceS, traceHead, widget.NewSeparator()),
		nil, nil, nil, s.traceScroll,
	))
	return s
}

// switchTo moves to another agent or thread: the running stream is dropped
// and the chat and trace start empty.
func (s *chatSession) switchTo(agent, thread string, status string, t tone) {
	s.epoch++
	s.app.cancelChat()
	s.agent = agent
	s.thread = thread
	s.sending = false
	s.sendBtn.Enable()
	s.clearChat()
	s.updateHeader()
	s.setStatus(status, t)
}

func (s *chatSession) updateHeader() {
	s.headerName.SetText(s.agent)
	s.headerThread.SetText("thread · " + shortID(s.thread))
}

func (s *chatSession) setStatus(text string, t tone) {
	s.status.SetText(text)
	switch t {
	case toneDanger:
		s.status.Importance = widget.DangerImportance
	case toneWarning:
		s.status.Importance = widget.WarningImportance
	case toneSuccess:
		s.status.Importance = widget.SuccessImportance
	default:
		s.status.Importance = widget.MediumImportance
	}
	s.status.Refresh()
	s.statusDot.FillColor = toneColor(t)
	s.statusDot.Refresh()
}

func (s *chatSession) clearChat() {
	clearBox(s.messages)
	s.showEmpty()
	clearBox(s.traceBox)
	s.events = nil
	s.assistant = nil
	s.tokens.SetText("")
}

func (s *chatSession) showEmpty() {
	if s.empty != nil {
		s.messages.Add(s.empty)
		s.messages.Refresh()
	}
}

// loadTrace replaces the trace with the thread's whole ring.
func (s *chatSession) loadTrace() {
	agent, thread, epoch := s.agent, s.thread, s.epoch
	if agent == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := s.app.api.Trace(ctx, agent, thread)
		onUI(func() {
			if s.epoch != epoch {
				return
			}
			if err != nil {
				s.app.showError(err)
				return
			}
			s.events = events
			s.renderTrace()
		})
	}()
}

func (s *chatSession) refreshTrace() { s.loadTrace() }

func (s *chatSession) stop() {
	if s.agent == "" {
		return
	}
	s.app.cancelChat()
	agent, thread := s.agent, s.thread
	s.setStatus("stopping…", toneWarning)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := s.app.api.StopThread(ctx, agent, thread)
		onUI(func() {
			if s.agent != agent || s.thread != thread {
				return
			}
			if err != nil {
				s.app.showError(err)
				s.setStatus("error", toneDanger)
				return
			}
			s.setStatus("stop requested", toneWarning)
		})
	}()
}

// send sends what the composer holds.
func (s *chatSession) send() {
	text := s.input.Text
	if s.sendText(text) {
		s.input.SetText("")
	}
}

// sendText starts a turn; false when one is running or there is nothing to send.
func (s *chatSession) sendText(text string) bool {
	if s.sending || s.agent == "" || strings.TrimSpace(text) == "" {
		return false
	}
	s.sending = true
	s.last = text
	s.sendBtn.Disable()
	s.setStatus("running…", toneAccent)
	clearBox(s.traceBox)
	s.events = nil

	s.addBubble("You", text, true)
	s.assistant = s.addBubble("Agent", "…", false)

	actAs := ""
	if s.actAs != nil {
		actAs = strings.TrimSpace(s.actAs())
	}
	s.epoch++
	agent, thread, epoch := s.agent, s.thread, s.epoch
	ctx, cancel := context.WithCancel(context.Background())
	s.app.chatCancel = cancel
	live := func() bool { return s.epoch == epoch }

	go func() {
		err := s.app.api.Chat(ctx, api.ChatRequest{
			Agent:  agent,
			Thread: thread,
			Text:   text,
			ActAs:  actAs,
		}, func(ev api.ChatEvent) {
			onUI(func() {
				if !live() {
					return
				}
				switch ev.Type {
				case "trace":
					if ev.Trace != nil {
						s.events = append(s.events, *ev.Trace)
						s.renderTrace()
					}
				case "reply":
					if ev.Reply != nil {
						s.appendReply(s.assistant, ev.Reply.Text)
					}
				case "error":
					s.setStatus("error: "+ev.Error, toneDanger)
				case "done":
					if ev.Done != nil && ev.Done.TimedOut {
						s.setStatus("turn timed out; the loop is still running", toneWarning)
					}
				}
			})
		})
		canceled := ctx.Err() != nil
		onUI(func() {
			if !live() {
				return
			}
			s.sending = false
			s.sendBtn.Enable()
			switch {
			case canceled:
				s.setStatus("turn canceled", toneWarning)
			case err != nil:
				s.setStatus("error: "+err.Error(), toneDanger)
			case strings.HasPrefix(s.status.Text, "running"):
				s.setStatus("done", toneSuccess)
			}
			if s.onTurnEnd != nil {
				s.onTurnEnd()
			}
		})
	}()
	return true
}

func (s *chatSession) addBubble(role, text string, mine bool) *widget.Label {
	// Both sides are neutral: the side of the thread says who spoke, so the
	// sender's bubble needs no accent fill on top of it.
	fill := colorSurface
	stroke := colorBorder
	roleColor := colorMuted
	if mine {
		fill = colorSurface2
	}
	bg := canvas.NewRectangle(fill)
	bg.CornerRadius = cardRadius
	bg.StrokeColor = stroke
	bg.StrokeWidth = 1

	// Drop the empty state once a real message exists.
	if s.empty != nil {
		for i, obj := range s.messages.Objects {
			if obj == s.empty {
				s.messages.Objects = append(s.messages.Objects[:i], s.messages.Objects[i+1:]...)
				break
			}
		}
	}

	roleTxt := canvas.NewText(sentenceCase(role), roleColor)
	roleTxt.TextSize = 11

	timeTxt := canvas.NewText(time.Now().Format("15:04"), colorFaint)
	timeTxt.TextSize = 11
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
	s.messages.Add(row)
	s.messages.Refresh()
	s.msgScroll.ScrollToBottom()
	return body
}

func (s *chatSession) appendReply(label *widget.Label, text string) {
	if label == nil {
		return
	}
	if label.Text == "…" || label.Text == "" {
		label.SetText(text)
	} else {
		label.SetText(label.Text + "\n\n" + text)
	}
	s.msgScroll.ScrollToBottom()
}

func (s *chatSession) renderTrace() {
	clearBox(s.traceBox)
	filter := ""
	if s.filter != nil {
		filter = s.filter.Selected
	}
	shown := 0
	for _, ev := range s.events {
		if filter == "Ops only" && ev.Op == "" {
			continue
		}
		if filter == "Errors only" && ev.Err == "" && ev.Kind != "error" {
			continue
		}
		s.traceBox.Add(s.traceRow(ev))
		shown++
	}
	if shown == 0 {
		s.traceBox.Add(muted("No events match this filter yet."))
	}
	s.traceBox.Refresh()
	s.traceScroll.ScrollToBottom()
	s.tokens.SetText(fmt.Sprintf("%d tok", traceTokens(s.events)))
}

// traceRow is one event as a two-line entry: sequence and kind on the first
// line with the cost on the right, the op/tool/model (or the error) beneath.
// The whole row opens the raw event.
func (s *chatSession) traceRow(ev api.TraceEvent) fyne.CanvasObject {
	rowTone := toneAccent
	switch {
	case !ev.OK && ev.Err != "":
		rowTone = toneDanger
	case ev.Kind == "turn.start" || ev.Kind == "turn.end":
		rowTone = toneNeutral
	case ev.Kind == "reply":
		rowTone = toneSuccess
	}
	seq := monoText(fmt.Sprintf("#%d", ev.Seq), 10.5, colorFaint)
	kind := monoText(ev.Kind, 11.5, colorText)
	kind.TextStyle.Bold = true
	if rowTone == toneNeutral {
		kind.Color = colorMuted
	}
	if rowTone == toneDanger {
		kind.Color = colorDanger
	}
	var cost []string
	if ev.DurationMS > 0 {
		cost = append(cost, strconv.FormatInt(ev.DurationMS, 10)+"ms")
	}
	if ev.TokensIn+ev.TokensOut > 0 {
		cost = append(cost, fmt.Sprintf("%d→%d tok", ev.TokensIn, ev.TokensOut))
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, container.NewCenter(dotBox(dot(rowTone))), container.NewCenter(seq), container.NewCenter(kind)),
		container.NewCenter(monoText(strings.Join(cost, " · "), 10.5, colorMuted)), nil)

	var detail []string
	for _, v := range []string{ev.Op, ev.Tool, ev.Model} {
		if v != "" {
			detail = append(detail, v)
		}
	}
	if ev.ToolCalls > 0 {
		detail = append(detail, fmt.Sprintf("%d call(s)", ev.ToolCalls))
	}
	body := vstack(3, top)
	if len(detail) > 0 {
		d := mono(strings.Join(detail, "  "), theme.SizeNameCaptionText)
		d.Importance = widget.LowImportance
		body.Add(container.New(layout.NewCustomPaddedLayout(0, 0, 16, 0), compactLabel(d, true)))
	}
	if ev.Err != "" {
		e := mono(ev.Err, theme.SizeNameCaptionText)
		e.Importance = widget.DangerImportance
		e.Wrapping = fyne.TextWrapWord
		e.Truncation = fyne.TextTruncateOff
		body.Add(container.New(layout.NewCustomPaddedLayout(0, 0, 16, 0), compactLabel(e, true)))
	}
	return newTapSurface(container.New(layout.NewCustomPaddedLayout(6, 6, spaceS, spaceS), body), 7, false,
		func() color.Color { return colorSurface2 }, func() { s.showEvent(ev) })
}

func (s *chatSession) showEvent(ev api.TraceEvent) {
	raw, _ := json.MarshalIndent(ev, "", "  ")
	body := widget.NewLabel(string(raw))
	body.TextStyle = fyne.TextStyle{Monospace: true}
	body.Wrapping = fyne.TextWrapWord
	body.Selectable = true
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(560, 400))
	showModalCard(s.app.win, "Trace event #"+strconv.FormatInt(ev.Seq, 10), ev.Kind, inputFrame(scroll), nil)
}
