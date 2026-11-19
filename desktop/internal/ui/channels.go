package ui

import (
	"context"
	"errors"
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

type channelsView struct {
	app  *App
	root fyne.CanvasObject

	list     *fyne.Container
	scroll   *container.Scroll
	banner   *banner
	count    *widget.Label
	state    *widget.Label
	stateDot *canvas.Circle
	dirBtn   *widget.Button
	newBtn   *widget.Button

	data   api.ChannelList
	agents []string
}

func (a *App) showChannels() {
	a.enter("channels")
	v := a.newChannelsView()
	a.setMain(v.root)
}

func (a *App) newChannelsView() *channelsView {
	v := &channelsView{app: a}
	v.list = vstack(spaceM)
	v.scroll = pageScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.state = oneLine("")
	v.stateDot = dot(toneSuccess)
	v.newBtn = primaryButton(T("channels.newChannel"), theme.ContentAddIcon(), func() { a.editChannel(v, nil) })
	v.dirBtn = iconButton(theme.FolderOpenIcon(), T("channels.openConfigFolder"), func() { v.openConfigFolder() })
	v.dirBtn.Disable()
	stateRow := hstack(spaceXS, vCenter(dotBox(v.stateDot)), vCenter(widthBox(190, v.state)))

	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), func() { v.load() })
	head := pageHeader(T("channels.title"),
		T("channels.subtitle"),
		hstack(spaceS, widthBox(88, v.count), stateRow, v.dirBtn, refresh, v.newBtn))
	v.load()
	v.root = container.NewBorder(
		vstack(0, pageInset(head), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		scrollInset(v.scroll),
	)
	return v
}

func (v *channelsView) setBanner(text string, tone tone) {
	v.banner.Set(text, tone)
	// Show/Hide changes the banner's min size; the containing VBox only
	// reflows when it is refreshed, or the message stays invisible.
	if v.root != nil {
		v.root.Refresh()
	}
}

func (v *channelsView) load() {
	v.banner.Clear()
	v.state.SetText(T("channels.loadingState"))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list, err := v.app.api.Channels(ctx)
		var agents []string
		if err == nil {
			if rows, aerr := v.app.api.Agents(ctx); aerr == nil {
				for _, row := range rows {
					if !row.Spawn {
						agents = append(agents, row.Name)
					}
				}
			}
		}
		onUI(func() {
			if err != nil {
				msg := err.Error()
				if ae, ok := err.(*api.Error); ok && ae.Status == 404 {
					msg = T("channels.apiMissing")
				}
				v.setBanner(msg, toneDanger)
				v.state.SetText("")
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon(T("common.retry"), theme.ViewRefreshIcon(), v.load)))
				v.list.Refresh()
				return
			}
			v.data = list
			v.agents = agents
			v.render()
		})
	}()
}

func (v *channelsView) render() {
	clearBox(v.list)
	editable := v.data.Editable
	v.newBtn.Enable()
	if !editable {
		v.newBtn.Disable()
		v.setBanner(v.data.Note, toneWarning)
	}
	pending := 0
	for _, ch := range v.data.Channels {
		if !ch.Live {
			pending++
		}
	}
	state := T("channels.allLive")
	toneOfState := toneSuccess
	if pending > 0 {
		state = T("channels.pendingRestart", "n", pending)
		toneOfState = toneWarning
	}
	v.state.SetText(state)
	v.stateDot.FillColor = toneColor(toneOfState)
	v.stateDot.Refresh()
	if v.data.Path != "" {
		v.dirBtn.Enable()
		v.dirBtn.Show()
	} else {
		v.dirBtn.Disable()
		v.dirBtn.Hide()
	}
	v.count.SetText(T("channels.count", "n", len(v.data.Channels)))
	if len(v.data.Channels) == 0 {
		// The empty state carries the primary action; keep the header to just
		// refresh so the same button does not appear twice.
		v.newBtn.Hide()
		// Match the agents empty state: the list fills the viewport when it is
		// short, so spacers centre the empty group instead of pinning it top.
		v.list.Add(layout.NewSpacer())
		v.list.Add(emptyChannels(v))
		v.list.Add(layout.NewSpacer())
		v.list.Refresh()
		v.root.Refresh()
		return
	}
	v.newBtn.Show()
	v.root.Refresh()
	for i := range v.data.Channels {
		ch := v.data.Channels[i]
		v.list.Add(v.channelCard(ch))
	}
	v.list.Refresh()
}

func channelTitle(ch api.Channel) string {
	if ch.Name != "" {
		return ch.Name
	}
	return fmt.Sprintf("%s #%d", ch.Type, ch.Index+1)
}

func (v *channelsView) channelCard(ch api.Channel) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(channelTitle(ch), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	sub := container.NewHBox(
		vCenter(chip(ch.Type, toneAccent)),
	)
	if ch.Live {
		sub.Add(vCenter(chip(T("channels.chip.live"), toneSuccess)))
	} else {
		sub.Add(vCenter(chip(T("channels.chip.restartRequired"), toneWarning)))
	}
	if len(ch.MediaAllow) > 0 {
		sub.Add(vCenter(chip(T("channels.chip.media"), toneNeutral)))
	}
	actions := hstack(spaceXS,
		vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.app.editChannel(v, &ch) })),
		vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.app.confirmChannelDelete(v, ch) })),
	)
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), sub), actions, nil)

	agent := ch.Agent
	if len(ch.Agents) > 0 {
		agent = strings.Join(ch.Agents, ", ")
	}
	if agent == "" {
		agent = "—"
	}
	route := ch.Path
	if route == "" {
		route = T("channels.driverDefault")
	}
	cells := []fyne.CanvasObject{
		detailRow(T("channels.fact.agent"), detailText(agent)),
		detailRow(T("channels.fact.path"), detailText(route)),
	}
	switch ch.Type {
	case "telegram":
		mode := ch.Mode
		if mode == "" {
			mode = "polling"
		}
		cells = append(cells, detailRow(T("channels.fact.mode"), detailText(mode)))
		if len(ch.AllowUsers) > 0 {
			cells = append(cells, detailRow(T("channels.fact.allowUsers"), detailText(fmt.Sprint(ch.AllowUsers))))
		}
		cells = append(cells, detailRow(T("channels.fact.token"), channelSecretValue(ch.HasToken, ch.TokenRef)),
			detailRow(T("channels.fact.webhookSecret"), channelSecretValue(ch.HasSecretTok, ch.SecretTokRef)))
	case "ghhook":
		cells = append(cells, detailRow(T("channels.fact.secret"), channelSecretValue(ch.HasSecret, ch.SecretRef)))
	case "qq":
		mode := ch.Mode
		if mode == "" {
			mode = "websocket"
		}
		scenes := strings.Join(ch.Scenes, ", ")
		if scenes == "" {
			scenes = strings.Join(qqDefaultScenes, ", ")
		}
		cells = append(cells, detailRow(T("channels.fact.mode"), detailText(mode)),
			detailRow(T("channels.fact.appId"), detailText(ch.AppID)),
			detailRow(T("channels.fact.scenes"), detailText(scenes)),
			detailRow(T("channels.fact.appSecret"), channelSecretValue(ch.HasSecret, ch.SecretRef)))
		if ch.Markdown {
			cells = append(cells, detailRow(T("channels.fact.replies"), detailText(T("channels.fact.repliesMarkdown"))))
		}
		if ch.APIBase != "" {
			cells = append(cells, detailRow(T("channels.fact.apiBase"), detailText(ch.APIBase)))
		}
	case "bridge":
		if ch.DeliverURL != "" {
			cells = append(cells, detailRow(T("channels.fact.deliver"), detailText(ch.DeliverURL)))
		}
		cells = append(cells, detailRow(T("channels.fact.inboundSecret"), channelSecretValue(ch.HasSecret, ch.SecretRef)),
			detailRow(T("channels.fact.outboundToken"), channelSecretValue(ch.HasToken, ch.TokenRef)))
		if ch.Timeout != "" {
			cells = append(cells, detailRow(T("channels.fact.timeout"), detailText(ch.Timeout)))
		}
	case "webhook":
		if ch.Timeout != "" {
			cells = append(cells, detailRow(T("channels.fact.timeout"), detailText(ch.Timeout)))
		}
		if ch.Async {
			cells = append(cells, detailRow(T("channels.fact.mode"), detailText(T("channels.fact.asyncJob"))))
		}
	case "openai":
		cells = append(cells, detailRow(T("channels.fact.auth"), detailText(ch.Auth)))
		if ch.Auth == "key" {
			cells = append(cells, detailRow(T("channels.fact.keys"), channelSecretValue(ch.HasKeys, strings.Join(ch.KeyRefs, ", "))))
		}
		journal := ch.Journal
		if journal == "" {
			journal = "metadata"
		}
		cells = append(cells, detailRow(T("channels.fact.journal"), detailText(journal)))
		if ch.Timeout != "" {
			cells = append(cells, detailRow(T("channels.fact.timeout"), detailText(ch.Timeout)))
		}
	}
	// The audit mode applies to every channel type. The openai card always
	// shows it (above); any other type shows it when it is not the default.
	if ch.Type != "openai" && ch.Journal != "" && ch.Journal != "full" {
		cells = append(cells, detailRow(T("channels.fact.journal"), detailText(ch.Journal)))
	}
	if len(ch.MediaAllow) > 0 {
		cells = append(cells, detailRow(T("channels.fact.media"), detailText(strings.Join(ch.MediaAllow, ", "))))
	}
	details := grid(2, cells...)
	return panel(vstack(spaceS, top, widget.NewSeparator(), details))
}

func channelSecretValue(set bool, ref string) fyne.CanvasObject {
	// One message per case: the reference sits inside the sentence, so each
	// language places it.
	switch {
	case set && ref != "":
		return chip(T("channels.secret.configuredRef", "ref", ref), toneSuccess)
	case set:
		return chip(T("channels.secret.configured"), toneSuccess)
	case ref != "":
		return chip(T("channels.secret.notSetRef", "ref", ref), toneWarning)
	}
	return chip(T("channels.secret.notSet"), toneWarning)
}

func emptyChannels(v *channelsView) fyne.CanvasObject {
	return emptyStateCard(
		T("channels.emptyTitle"),
		T("channels.emptyGuidance"),
		T("channels.emptyAction"),
		func() { v.app.editChannel(v, nil) },
	)
}

func (v *channelsView) openConfigFolder() {
	if v.data.Path == "" {
		v.setBanner(T("channels.noConfigPath"), toneWarning)
		return
	}
	if err := openConfigFolder(v.data.Path); err != nil {
		v.app.showError(err)
	}
}

func (a *App) confirmChannelDelete(v *channelsView, ch api.Channel) {
	name := channelTitle(ch)
	showConfirmPrompt(a.win, T("channels.deleteTitle"),
		T("channels.deleteMessage", "name", name),
		T("common.delete"), true,
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				list, err := a.api.DeleteChannel(ctx, ch.Index, v.data.MTime)
				onUI(func() {
					if err != nil {
						v.setBanner(err.Error(), toneDanger)
						return
					}
					v.data = list
					v.render()
					a.setStatus(T("channels.deleted"), toneWarning)
				})
			}()
		})
}

func (a *App) editChannel(v *channelsView, existing *api.Channel) {
	isNew := existing == nil
	if !v.data.Editable {
		note := v.data.Note
		if note == "" {
			note = T("channels.readOnlyMessage")
		}
		showInfoPrompt(a.win, T("channels.readOnlyTitle"), note)
		return
	}
	if len(v.agents) == 0 {
		showConfirmPrompt(a.win, T("channels.needAgentTitle"),
			T("channels.needAgentMessage"),
			T("channels.goToAgents"), false,
			func(ok bool) {
				if ok {
					a.showAgents()
				}
			})
		return
	}

	name := widget.NewEntry()
	name.SetPlaceHolder("telegram-main")
	typ := widget.NewSelect([]string{"webhook", "telegram", "qq", "ghhook", "bridge", "openai"}, nil)
	agent := widget.NewSelect(v.agents, nil)
	path := widget.NewEntry()
	path.SetPlaceHolder(T("channels.placeholder.pathDefault"))
	mode := widget.NewSelect([]string{"polling", "webhook", "auto"}, nil)
	token := widget.NewPasswordEntry()
	secret := widget.NewPasswordEntry()
	secretToken := widget.NewPasswordEntry()
	clearToken := widget.NewCheck(T("channels.clearToken"), nil)
	clearSecret := widget.NewCheck(T("channels.clearSecret"), nil)
	clearSecretTok := widget.NewCheck(T("channels.clearWebhookSecret"), nil)
	openaiAgents := widget.NewEntry()
	openaiAgents.SetPlaceHolder("coder, helper")
	openaiAuth := widget.NewSelect([]string{"key", "user_token", "none"}, nil)
	openaiKeys := widget.NewPasswordEntry()
	openaiKeys.SetPlaceHolder("${COMPAT_API_KEY}")
	clearKeys := widget.NewCheck(T("channels.clearKeys"), nil)
	openaiJournal := widget.NewSelect([]string{"metadata", "full", "off"}, nil)
	openaiMaxBytes := widget.NewEntry()
	openaiMaxBytes.SetPlaceHolder("33554432")
	exposeReasoning := widget.NewCheck(T("channels.exposeReasoning"), nil)
	allowUsers := widget.NewEntry()
	allowUsers.SetPlaceHolder("123456789, 987654321")
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("55s")
	deliverURL := widget.NewEntry()
	deliverURL.SetPlaceHolder("https://adapter.example.com/deliver")
	async := widget.NewCheck(T("channels.asyncReply"), nil)
	qqMode := widget.NewSelect([]string{"websocket", "webhook"}, nil)
	qqAppID := widget.NewEntry()
	qqAppID.SetPlaceHolder("102xxxxxx")
	qqMarkdown := widget.NewCheck(T("channels.markdownReplies"), nil)
	qqAPIBase := widget.NewEntry()
	qqAPIBase.SetPlaceHolder(T("channels.placeholder.apiBaseDesktop"))
	qqScenes := map[string]*widget.Check{}
	qqSceneRow := container.NewHBox()
	for _, s := range []struct{ id, labelKey string }{
		{"c2c", "channels.scene.c2c"}, {"group", "channels.scene.group"}, {"guild", "channels.scene.guild"}, {"dm", "channels.scene.dm"},
	} {
		c := widget.NewCheck(T(s.labelKey), nil)
		qqScenes[s.id] = c
		qqSceneRow.Add(c)
	}
	mediaAllow := widget.NewEntry()
	mediaAllow.SetPlaceHolder("image/*, application/pdf")
	mediaMax := widget.NewEntry()
	mediaMax.SetPlaceHolder("8388608")
	errBanner := newBanner()

	if existing != nil {
		name.SetText(existing.Name)
		typ.SetSelected(existing.Type)
		if existing.Agent != "" {
			agent.SetSelected(existing.Agent)
		}
		path.SetText(existing.Path)
		if existing.Mode != "" {
			mode.SetSelected(existing.Mode)
		}
		if existing.Timeout != "" {
			timeout.SetText(existing.Timeout)
		}
		deliverURL.SetText(existing.DeliverURL)
		if len(existing.AllowUsers) > 0 {
			ids := make([]string, len(existing.AllowUsers))
			for i, id := range existing.AllowUsers {
				ids[i] = strconv.FormatInt(id, 10)
			}
			allowUsers.SetText(strings.Join(ids, ", "))
		}
		async.SetChecked(existing.Async)
		if existing.Type == "qq" {
			qqMode.SetSelected(existing.Mode)
			qqAppID.SetText(existing.AppID)
			qqAPIBase.SetText(existing.APIBase)
			qqMarkdown.SetChecked(existing.Markdown)
		}
		mediaAllow.SetText(strings.Join(existing.MediaAllow, ", "))
		if existing.MediaMaxBytes > 0 {
			mediaMax.SetText(strconv.Itoa(existing.MediaMaxBytes))
		}
		if len(existing.Agents) > 0 {
			openaiAgents.SetText(strings.Join(existing.Agents, ", "))
		} else if existing.Type == "openai" && existing.Agent != "" {
			// The file used the agent: shorthand for a single agent. Without
			// this the list would open empty and saving the form unchanged
			// would be refused for having no agents.
			openaiAgents.SetText(existing.Agent)
		}
		if existing.Auth != "" {
			openaiAuth.SetSelected(existing.Auth)
		}
		if existing.Journal != "" {
			openaiJournal.SetSelected(existing.Journal)
		}
		if existing.MaxRequestBytes > 0 {
			openaiMaxBytes.SetText(strconv.Itoa(existing.MaxRequestBytes))
		}
		exposeReasoning.SetChecked(existing.ExposeReasoning)
		if len(existing.KeyRefs) > 0 {
			openaiKeys.SetPlaceHolder(T("channels.placeholder.keysStoredRefs", "refs", strings.Join(existing.KeyRefs, ", ")))
		} else if existing.HasKeys {
			openaiKeys.SetPlaceHolder(T("channels.placeholder.keepStoredKeys"))
		}
	} else {
		typ.SetSelected("webhook")
		if len(v.agents) > 0 {
			agent.SetSelected(v.agents[0])
		}
		mode.SetSelected("polling")
		openaiAuth.SetSelected("key")
		openaiJournal.SetSelected("metadata")
	}
	if mode.Selected == "" {
		mode.SetSelected("polling")
	}
	if qqMode.Selected == "" {
		qqMode.SetSelected("websocket")
	}
	scenes := qqDefaultScenes
	if existing != nil && existing.Type == "qq" && len(existing.Scenes) > 0 {
		scenes = existing.Scenes
	}
	for _, s := range scenes {
		if c := qqScenes[strings.ToLower(s)]; c != nil {
			c.SetChecked(true)
		}
	}

	secretHint := func(set bool, ref, empty string) string {
		switch {
		case ref != "":
			return T("channels.placeholder.storedRef", "ref", ref)
		case set:
			return T("channels.placeholder.keepStoredValue")
		default:
			return empty
		}
	}
	token.SetPlaceHolder(secretHint(existing != nil && existing.HasToken, tokenRef(existing), T("channels.placeholder.botToken")))
	secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), T("channels.placeholder.githubSecret")))
	secretToken.SetPlaceHolder(secretHint(existing != nil && existing.HasSecretTok, secretTokRef(existing), T("channels.placeholder.telegramWebhookSecret")))

	field := func(label string, control fyne.CanvasObject) fyne.CanvasObject {
		return vstack(spaceXS, fieldLabel(label), controlFrame(control))
	}
	typeBox := container.NewVBox()
	renderType := func() {
		clearBox(typeBox)
		switch typ.Selected {
		case "telegram":
			typeBox.Add(field(T("channels.field.telegramMode"), mode))
			typeBox.Add(field(T("channels.field.botToken"), token))
			if existing != nil && existing.HasToken {
				typeBox.Add(clearToken)
			}
			typeBox.Add(field(T("channels.field.webhookSecretToken"), secretToken))
			if existing != nil && existing.HasSecretTok {
				typeBox.Add(clearSecretTok)
			}
			typeBox.Add(caption(T("channels.hint.telegramMode")))
			typeBox.Add(field(T("channels.field.allowedUsers"), allowUsers))
			typeBox.Add(caption(T("channels.hint.allowedUsers")))
		case "qq":
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), T("channels.placeholder.appSecret")))
			typeBox.Add(field("AppID", qqAppID))
			typeBox.Add(field("AppSecret", secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption(T("channels.hint.qqCredentialsDesktop")))
			typeBox.Add(field(T("channels.field.eventDelivery"), qqMode))
			if qqMode.Selected == "webhook" {
				typeBox.Add(caption(T("channels.hint.qqWebhookDesktop")))
			} else {
				typeBox.Add(caption(T("channels.hint.qqWebsocketDesktop")))
			}
			typeBox.Add(fieldLabel(T("channels.field.scenes")))
			typeBox.Add(qqSceneRow)
			typeBox.Add(caption(T("channels.hint.scenes")))
			typeBox.Add(qqMarkdown)
			typeBox.Add(caption(T("channels.hint.replies")))
			typeBox.Add(field(T("channels.field.apiBase"), qqAPIBase))
		case "ghhook":
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), T("channels.placeholder.githubSecret")))
			typeBox.Add(field(T("channels.field.webhookSecret"), secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption(T("channels.hint.githubSecret")))
		case "webhook":
			timeout.SetPlaceHolder("55s")
			typeBox.Add(field(T("channels.field.replyTimeout"), timeout))
			typeBox.Add(caption(T("channels.hint.replyTimeout")))
			typeBox.Add(async)
		case "openai":
			timeout.SetPlaceHolder("600s")
			typeBox.Add(field(T("channels.field.agentsExposed"), openaiAgents))
			typeBox.Add(caption(T("channels.hint.agentsExposedDesktop")))
			typeBox.Add(field(T("channels.field.auth"), openaiAuth))
			switch openaiAuth.Selected {
			case "key":
				typeBox.Add(field(T("channels.field.apiKeys"), openaiKeys))
				if existing != nil && existing.HasKeys {
					typeBox.Add(clearKeys)
				}
				typeBox.Add(caption(T("channels.hint.apiKeys")))
			case "user_token":
				typeBox.Add(caption(T("channels.hint.authUserToken")))
			default:
				typeBox.Add(caption(T("channels.hint.authNone")))
			}
			typeBox.Add(field(T("channels.field.journal"), openaiJournal))
			typeBox.Add(caption(T("channels.hint.journal")))
			typeBox.Add(field(T("channels.field.maxRequestBytes"), openaiMaxBytes))
			typeBox.Add(caption(T("channels.hint.maxRequestBytes")))
			typeBox.Add(exposeReasoning)
			typeBox.Add(caption(T("channels.hint.exposeReasoning")))
			typeBox.Add(field(T("channels.field.requestTimeout"), timeout))
			typeBox.Add(caption(T("channels.hint.requestTimeout")))
		case "bridge":
			timeout.SetPlaceHolder("55s")
			typeBox.Add(field(T("channels.field.deliverUrl"), deliverURL))
			typeBox.Add(caption(T("channels.hint.deliverUrl")))
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), T("channels.placeholder.inboundSecret")))
			typeBox.Add(field(T("channels.field.inboundSecret"), secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption(T("channels.hint.inboundSecret")))
			typeBox.Add(field(T("channels.field.outboundToken"), token))
			if existing != nil && existing.HasToken {
				typeBox.Add(clearToken)
			}
			typeBox.Add(caption(T("channels.hint.outboundToken")))
			typeBox.Add(field(T("channels.field.deliveryTimeout"), timeout))
			typeBox.Add(caption(T("channels.hint.deliveryTimeout")))
			typeBox.Add(caption(T("channels.hint.pathBridge")))
		}
		typeBox.Refresh()
	}
	typ.OnChanged = func(string) { renderType() }
	openaiAuth.OnChanged = func(string) { renderType() }
	qqMode.OnChanged = func(string) { renderType() }
	renderType()

	content := container.NewVBox(
		field(T("common.name"), name),
		field(T("channels.field.type"), typ),
		field(T("channels.field.agent"), agent),
		field(T("channels.field.path"), path),
		caption(T("channels.hint.pathDefault")),
		typeBox,
	)
	form := container.NewVBox(content,
		widget.NewSeparator(),
		subheading(T("channels.mediaHeading")),
		field(T("channels.field.mediaAllow"), mediaAllow),
		caption(T("channels.hint.mediaAllow")),
		field(T("channels.field.mediaMax"), mediaMax),
		caption(T("channels.hint.mediaMax")),
	)
	scroll := container.NewVScroll(panel(form))
	scroll.SetMinSize(fyne.NewSize(700, 520))
	dialogContent := container.NewBorder(errBanner, nil, nil, nil, scroll)

	build := func() (api.ChannelDraft, error) {
		d := api.ChannelDraft{
			Name: strings.TrimSpace(name.Text), Type: typ.Selected,
			Agent: agent.Selected, Path: strings.TrimSpace(path.Text),
			Token: token.Text, Secret: secret.Text, SecretToken: secretToken.Text,
			ClearToken: clearToken.Checked, ClearSecret: clearSecret.Checked, ClearSecretTok: clearSecretTok.Checked,
			AllowUsers: []int64{}, MediaAllow: splitList(mediaAllow.Text),
		}
		if d.Name == "" {
			return d, errors.New(T("channels.error.nameRequired"))
		}
		if d.Type == "" {
			return d, errors.New(T("channels.chooseType"))
		}
		if d.Type != "openai" && d.Agent == "" {
			return d, errors.New(T("channels.chooseAgent"))
		}
		switch d.Type {
		case "telegram":
			d.Mode = mode.Selected
			for _, part := range splitList(allowUsers.Text) {
				id, err := strconv.ParseInt(part, 10, 64)
				if err != nil {
					return d, errors.New(T("channels.error.allowUserNotNumeric", "value", part))
				}
				d.AllowUsers = append(d.AllowUsers, id)
			}
		case "webhook":
			d.Timeout = strings.TrimSpace(timeout.Text)
			d.Async = async.Checked
		case "qq":
			d.Mode = qqMode.Selected
			d.AppID = strings.TrimSpace(qqAppID.Text)
			d.APIBase = strings.TrimSpace(qqAPIBase.Text)
			d.Markdown = qqMarkdown.Checked
			for _, s := range []string{"c2c", "group", "guild", "dm"} {
				if qqScenes[s].Checked {
					d.Scenes = append(d.Scenes, s)
				}
			}
			if d.AppID == "" {
				return d, errors.New(T("channels.error.appIdRequired"))
			}
			if len(d.Scenes) == 0 {
				return d, errors.New(T("channels.error.sceneRequired"))
			}
			if d.Mode == "websocket" {
				d.Path = ""
			}
		case "bridge":
			d.DeliverURL = strings.TrimSpace(deliverURL.Text)
			d.Timeout = strings.TrimSpace(timeout.Text)
		case "openai":
			d.Agent = ""
			d.Agents = splitList(openaiAgents.Text)
			d.Auth = openaiAuth.Selected
			d.Keys = splitList(openaiKeys.Text)
			d.ClearKeys = clearKeys.Checked
			d.Journal = openaiJournal.Selected
			d.ExposeReasoning = exposeReasoning.Checked
			d.Timeout = strings.TrimSpace(timeout.Text)
			if t := strings.TrimSpace(openaiMaxBytes.Text); t != "" {
				n, err := strconv.Atoi(t)
				if err != nil || n < 0 {
					return d, errors.New(T("channels.error.maxRequestBytes"))
				}
				d.MaxRequestBytes = n
			}
		}
		if t := strings.TrimSpace(mediaMax.Text); t != "" {
			n, err := strconv.Atoi(t)
			if err != nil || n < 0 {
				return d, errors.New(T("channels.error.mediaMax"))
			}
			d.MediaMaxBytes = n
		}
		return d, nil
	}

	title := T("channels.newChannel")
	if !isNew {
		title = T("channels.editNamed", "name", channelTitle(*existing))
	}
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	validate := actionButton(T("channels.validate"), checkMarkIcon, nil)
	save := primaryButton(T("common.save"), theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(108, controlFrame(validate)), widthBox(104, controlFrame(save))), nil)

	busy := false
	setBusy := func(b bool) {
		busy = b
		if b {
			validate.Disable()
			save.Disable()
		} else {
			validate.Enable()
			save.Enable()
		}
	}
	validate.OnTapped = func() {
		if busy {
			return
		}
		draft, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		setBusy(true)
		idx := -1
		if existing != nil {
			idx = existing.Index
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			errs, err := a.api.ValidateChannel(ctx, idx, draft)
			onUI(func() {
				setBusy(false)
				switch {
				case err != nil:
					errBanner.Set(err.Error(), toneDanger)
				case len(errs) > 0:
					errBanner.Set(strings.Join(errs, "\n"), toneDanger)
				default:
					errBanner.Set(T("channels.valid"), toneSuccess)
				}
			})
		}()
	}
	save.OnTapped = func() {
		if busy {
			return
		}
		draft, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		idx := -1
		if existing != nil {
			idx = existing.Index
		}
		setBusy(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			list, err := a.api.SaveChannel(ctx, idx, draft, v.data.MTime)
			onUI(func() {
				setBusy(false)
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.data = list
				v.render()
				a.setStatus(T("channels.saved"), toneWarning)
			})
		}()
	}

	popup = showModalCard(a.win, title, T("channels.editorSubtitle"), dialogContent, footer)
}

func tokenRef(ch *api.Channel) string {
	if ch == nil {
		return ""
	}
	return ch.TokenRef
}

func secretRefOf(ch *api.Channel) string {
	if ch == nil {
		return ""
	}
	return ch.SecretRef
}

func secretTokRef(ch *api.Channel) string {
	if ch == nil {
		return ""
	}
	return ch.SecretTokRef
}

// monoInline is a monospace value with no truncation: in an HBox a truncating
// label reports a tiny minimum and collapses to an ellipsis.
func monoInline(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Monospace: true}
	l.SizeName = theme.SizeNameCaptionText
	l.Wrapping = fyne.TextWrapOff
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

// qqDefaultScenes mirrors the runtime's default for a qq channel with no
// scenes listed.
var qqDefaultScenes = []string{"c2c", "group", "guild"}
