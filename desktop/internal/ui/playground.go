package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

// playgroundView is the Playground: the agent picker, recent threads and the
// act-as field around one chatSession.
type playgroundView struct {
	*chatSession
	root fyne.CanvasObject

	agentSelect *widget.Select
	threadBox   *fyne.Container
	actAs       *widget.SelectEntry
}

func (a *App) showPlayground(agent string) {
	a.enter("playground")
	a.setMain(a.playgroundPage(nil, container.NewCenter(muted("Loading agents…"))))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rows, err := a.api.Agents(ctx)
		onUI(func() {
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
	p := &playgroundView{chatSession: newChatSession(a, agent, chatOptions{
		emptyNote: "The turn streams into the trace panel on the right.",
	})}
	p.onTurnEnd = p.loadThreads

	p.agentSelect = widget.NewSelect(names, func(name string) {
		if name == "" || name == p.agent {
			return
		}
		p.switchTo(name, newThreadID(), "ready", toneSuccess)
		p.loadThreads()
	})
	p.agentSelect.SetSelected(agent)

	newThread := primaryButton("New thread", theme.ContentAddIcon(), func() {
		id := newThreadID()
		p.switchTo(p.agent, id, "new thread "+shortID(id), toneAccent)
		p.loadThreads()
	})

	p.threadBox = vstack(spaceXS)
	threadScroll := container.NewVScroll(p.threadBox)

	p.actAs = widget.NewSelectEntry(nil)
	p.actAs.SetPlaceHolder("profile uuid")
	a.loadUserIDs(func(ids []string) { p.actAs.SetOptions(ids) })
	p.chatSession.actAs = func() string { return p.actAs.Text }

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

	inner := newHSplitPane(left, p.chatPanel, 0.28)
	outer := newHSplitPane(inner, p.tracePanel, 0.67)

	p.root = a.playgroundPage(hstack(spaceS, newThread), outer)
	p.updateHeader()
	p.loadThreads()
	return p
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
		onUI(func() {
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
	p.switchTo(p.agent, thread, "thread "+shortID(thread), toneAccent)
	p.loadTrace()
	p.loadThreads()
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
