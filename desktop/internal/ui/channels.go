package ui

import (
	"context"
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
	v.list = vstack(spaceS)
	v.scroll = container.NewVScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.state = oneLine("")
	v.stateDot = dot(toneSuccess)
	v.newBtn = primaryButton("New channel", theme.ContentAddIcon(), func() { a.editChannel(v, nil) })
	v.dirBtn = iconButton(theme.FolderOpenIcon(), "Open config folder", func() { v.openConfigFolder() })
	v.dirBtn.Disable()
	stateRow := hstack(spaceXS, vCenter(dotBox(v.stateDot)), vCenter(widthBox(190, v.state)))

	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", func() { v.load() })
	head := pageHeader("Channels",
		"Entry points that route messages into agents. Saves go to the deployment config and bind at the next runtime start.",
		hstack(spaceS, widthBox(88, v.count), stateRow, v.dirBtn, refresh, v.newBtn))
	v.load()
	v.root = container.NewBorder(
		vstack(0, pageInset(head), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(v.scroll),
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
	v.state.SetText("loading…")
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
					msg = "This runtime does not expose the Studio channels API yet; rebuild and restart AgentFlow, then reopen Channels."
				}
				v.setBanner(msg, toneDanger)
				v.state.SetText("")
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), v.load)))
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
	state := "all changes live"
	toneOfState := toneSuccess
	switch {
	case pending == 1:
		state = "1 change · restart required"
		toneOfState = toneWarning
	case pending > 1:
		state = fmt.Sprintf("%d changes · restart required", pending)
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
	n := len(v.data.Channels)
	if n == 1 {
		v.count.SetText("1 channel")
	} else {
		v.count.SetText(fmt.Sprintf("%d channels", n))
	}
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
		sub.Add(vCenter(chip("live", toneSuccess)))
	} else {
		sub.Add(vCenter(chip("restart required", toneWarning)))
	}
	if len(ch.MediaAllow) > 0 {
		sub.Add(vCenter(chip("media", toneNeutral)))
	}
	actions := hstack(spaceXS,
		vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.app.editChannel(v, &ch) })),
		vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { v.app.confirmChannelDelete(v, ch) })),
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
		route = "driver default"
	}
	row := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, widthBox(104, fieldLabel(label)), nil, value)
	}
	cells := []fyne.CanvasObject{
		row("agent", widget.NewLabelWithStyle(agent, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})),
		row("path", monoInline(route)),
	}
	switch ch.Type {
	case "telegram":
		mode := ch.Mode
		if mode == "" {
			mode = "polling"
		}
		cells = append(cells, row("mode", widget.NewLabel(mode)))
		if len(ch.AllowUsers) > 0 {
			cells = append(cells, row("allow users", widget.NewLabel(fmt.Sprint(ch.AllowUsers))))
		}
		cells = append(cells, row("token", channelSecretValue(ch.HasToken, ch.TokenRef)),
			row("webhook secret", channelSecretValue(ch.HasSecretTok, ch.SecretTokRef)))
	case "ghhook":
		cells = append(cells, row("secret", channelSecretValue(ch.HasSecret, ch.SecretRef)))
	case "qq":
		mode := ch.Mode
		if mode == "" {
			mode = "websocket"
		}
		scenes := strings.Join(ch.Scenes, ", ")
		if scenes == "" {
			scenes = strings.Join(qqDefaultScenes, ", ")
		}
		cells = append(cells, row("mode", widget.NewLabel(mode)),
			row("app id", monoInline(ch.AppID)),
			row("scenes", widget.NewLabel(scenes)),
			row("app secret", channelSecretValue(ch.HasSecret, ch.SecretRef)))
		if ch.Markdown {
			cells = append(cells, row("replies", widget.NewLabel("markdown")))
		}
		if ch.APIBase != "" {
			cells = append(cells, row("api base", monoInline(ch.APIBase)))
		}
	case "bridge":
		if ch.DeliverURL != "" {
			cells = append(cells, row("deliver", monoInline(ch.DeliverURL)))
		}
		cells = append(cells, row("inbound secret", channelSecretValue(ch.HasSecret, ch.SecretRef)),
			row("outbound token", channelSecretValue(ch.HasToken, ch.TokenRef)))
		if ch.Timeout != "" {
			cells = append(cells, row("timeout", widget.NewLabel(ch.Timeout)))
		}
	case "webhook":
		if ch.Timeout != "" {
			cells = append(cells, row("timeout", widget.NewLabel(ch.Timeout)))
		}
		if ch.Async {
			cells = append(cells, row("mode", widget.NewLabel("async · job id")))
		}
	case "openai":
		cells = append(cells, row("auth", widget.NewLabel(ch.Auth)))
		if ch.Auth == "key" {
			cells = append(cells, row("keys", channelSecretValue(ch.HasKeys, strings.Join(ch.KeyRefs, ", "))))
		}
		journal := ch.Journal
		if journal == "" {
			journal = "metadata"
		}
		cells = append(cells, row("journal", widget.NewLabel(journal)))
		if ch.Timeout != "" {
			cells = append(cells, row("timeout", widget.NewLabel(ch.Timeout)))
		}
	}
	// The audit mode applies to every channel type. The openai card always
	// shows it (above); any other type shows it when it is not the default.
	if ch.Type != "openai" && ch.Journal != "" && ch.Journal != "full" {
		cells = append(cells, row("journal", widget.NewLabel(ch.Journal)))
	}
	if len(ch.MediaAllow) > 0 {
		cells = append(cells, row("media", monoInline(strings.Join(ch.MediaAllow, ", "))))
	}
	details := container.NewGridWithColumns(2, cells...)
	return panel(vstack(spaceS, top, widget.NewSeparator(), details))
}

func channelSecretValue(set bool, ref string) fyne.CanvasObject {
	value := "not set"
	tone := toneWarning
	if set {
		value = "configured"
		tone = toneSuccess
	}
	if ref != "" {
		value += " · " + ref
	}
	return chip(value, tone)
}

func emptyChannels(v *channelsView) fyne.CanvasObject {
	return emptyStateCard(
		"No channels yet",
		"Create a channel to route messages into an agent.",
		"Create a channel",
		func() { v.app.editChannel(v, nil) },
	)
}

func (v *channelsView) openConfigFolder() {
	if v.data.Path == "" {
		v.setBanner("This runtime did not report a channels config path.", toneWarning)
		return
	}
	if err := openConfigFolder(v.data.Path); err != nil {
		v.app.showError(err)
	}
}

func (a *App) confirmChannelDelete(v *channelsView, ch api.Channel) {
	name := channelTitle(ch)
	showConfirmPrompt(a.win, "Delete channel",
		fmt.Sprintf("Delete %q from the deployment config?\n\nThe running instance keeps it until the next restart.", name),
		"Delete", true,
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
					a.setStatus("channel deleted · restart required", toneWarning)
				})
			}()
		})
}

func (a *App) editChannel(v *channelsView, existing *api.Channel) {
	isNew := existing == nil
	if !v.data.Editable {
		note := v.data.Note
		if note == "" {
			note = "This deployment's channels cannot be edited from Studio."
		}
		showInfoPrompt(a.win, "Channels are read-only", note)
		return
	}
	if len(v.agents) == 0 {
		showConfirmPrompt(a.win, "Create an agent first",
			"A channel must route messages to an existing agent. Create an agent, then add the channel.",
			"Go to Agents", false,
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
	path.SetPlaceHolder("optional · e.g. /webhook/")
	mode := widget.NewSelect([]string{"polling", "webhook", "auto"}, nil)
	token := widget.NewPasswordEntry()
	secret := widget.NewPasswordEntry()
	secretToken := widget.NewPasswordEntry()
	clearToken := widget.NewCheck("Remove the stored token", nil)
	clearSecret := widget.NewCheck("Remove the stored secret", nil)
	clearSecretTok := widget.NewCheck("Remove the stored webhook secret", nil)
	openaiAgents := widget.NewEntry()
	openaiAgents.SetPlaceHolder("coder, helper")
	openaiAuth := widget.NewSelect([]string{"key", "user_token", "none"}, nil)
	openaiKeys := widget.NewPasswordEntry()
	openaiKeys.SetPlaceHolder("${COMPAT_API_KEY}")
	clearKeys := widget.NewCheck("Remove the stored keys", nil)
	openaiJournal := widget.NewSelect([]string{"metadata", "full", "off"}, nil)
	openaiMaxBytes := widget.NewEntry()
	openaiMaxBytes.SetPlaceHolder("33554432")
	exposeReasoning := widget.NewCheck("Send model reasoning as reasoning_content", nil)
	allowUsers := widget.NewEntry()
	allowUsers.SetPlaceHolder("123456789, 987654321")
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("55s")
	deliverURL := widget.NewEntry()
	deliverURL.SetPlaceHolder("https://adapter.example.com/deliver")
	async := widget.NewCheck("Async (202 + job id instead of a blocking reply)", nil)
	qqMode := widget.NewSelect([]string{"websocket", "webhook"}, nil)
	qqAppID := widget.NewEntry()
	qqAppID.SetPlaceHolder("102xxxxxx")
	qqMarkdown := widget.NewCheck("Send replies as Markdown", nil)
	qqAPIBase := widget.NewEntry()
	qqAPIBase.SetPlaceHolder("optional · default https://api.bot.qq.com")
	qqScenes := map[string]*widget.Check{}
	qqSceneRow := container.NewHBox()
	for _, s := range []struct{ id, label string }{
		{"c2c", "Private chats"}, {"group", "Group @-mentions"}, {"guild", "Guild @-mentions"}, {"dm", "Guild DMs"},
	} {
		c := widget.NewCheck(s.label, nil)
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
			openaiKeys.SetPlaceHolder("stored " + strings.Join(existing.KeyRefs, ", ") + " — leave blank to keep")
		} else if existing.HasKeys {
			openaiKeys.SetPlaceHolder("leave blank to keep the stored keys")
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
			return "stored " + ref
		case set:
			return "leave blank to keep the stored value"
		default:
			return empty
		}
	}
	token.SetPlaceHolder(secretHint(existing != nil && existing.HasToken, tokenRef(existing), "bot token"))
	secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), "GitHub webhook secret"))
	secretToken.SetPlaceHolder(secretHint(existing != nil && existing.HasSecretTok, secretTokRef(existing), "telegram webhook secret (optional)"))

	field := func(label string, control fyne.CanvasObject) fyne.CanvasObject {
		return vstack(spaceXS, fieldLabel(label), controlFrame(control))
	}
	typeBox := container.NewVBox()
	renderType := func() {
		clearBox(typeBox)
		switch typ.Selected {
		case "telegram":
			typeBox.Add(field("Telegram mode", mode))
			typeBox.Add(field("Bot token", token))
			if existing != nil && existing.HasToken {
				typeBox.Add(clearToken)
			}
			typeBox.Add(field("Webhook secret token", secretToken))
			if existing != nil && existing.HasSecretTok {
				typeBox.Add(clearSecretTok)
			}
			typeBox.Add(caption("Webhook and auto modes need gateway.public_url. Polling ignores the path."))
			typeBox.Add(field("Allowed Telegram user IDs", allowUsers))
			typeBox.Add(caption("Comma-separated numeric IDs; empty accepts anyone the bot can see."))
		case "qq":
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), "AppSecret, e.g. ${QQ_BOT_SECRET}"))
			typeBox.Add(field("AppID", qqAppID))
			typeBox.Add(field("AppSecret", secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption("From the bot's page on the QQ Open Platform (q.qq.com). The secret signs webhook callbacks and fetches the access token."))
			typeBox.Add(field("Event delivery", qqMode))
			if qqMode.Selected == "webhook" {
				typeBox.Add(caption("Set <public https url><path> as the callback URL in the console (ports 80, 443, 8080 or 8443). Path defaults to /webhook/qq/<name>/."))
			} else {
				typeBox.Add(caption("The runtime connects out to the QQ gateway; nothing needs to be reachable from the internet. Leave Path empty."))
			}
			typeBox.Add(fieldLabel("Scenes"))
			typeBox.Add(qqSceneRow)
			typeBox.Add(caption("Guild DMs need an intent the platform grants on application."))
			typeBox.Add(qqMarkdown)
			typeBox.Add(caption("Private chats and groups only; the platform grants markdown per bot. A refused markdown reply is resent as plain text."))
			typeBox.Add(field("API base", qqAPIBase))
		case "ghhook":
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), "GitHub webhook secret"))
			typeBox.Add(field("Webhook secret", secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption("Required. The HMAC signature is the only thing authenticating a delivery."))
		case "webhook":
			timeout.SetPlaceHolder("55s")
			typeBox.Add(field("Reply timeout", timeout))
			typeBox.Add(caption("Default 55s. Raise it for pipelines that outrun a single turn."))
			typeBox.Add(async)
		case "openai":
			timeout.SetPlaceHolder("600s")
			typeBox.Add(field("Agents exposed as model ids", openaiAgents))
			typeBox.Add(caption("Comma-separated. A client selects one by name as its model. The Agent field above is ignored for this type."))
			typeBox.Add(field("Auth", openaiAuth))
			switch openaiAuth.Selected {
			case "key":
				typeBox.Add(field("API keys", openaiKeys))
				if existing != nil && existing.HasKeys {
					typeBox.Add(clearKeys)
				}
				typeBox.Add(caption("Bearer keys clients send, comma-separated. ${VAR} and cred:<service> references are resolved at start."))
			case "user_token":
				typeBox.Add(caption("Per-user bearer tokens issued by the identity layer; runtime.identity.enabled must be on. Stored shared keys are removed on save."))
			default:
				typeBox.Add(caption("No auth: only allowed when gateway.listen is a loopback address. Stored shared keys are removed on save."))
			}
			typeBox.Add(field("Journal", openaiJournal))
			typeBox.Add(caption("metadata keeps ids and sizes and never the conversation text; full keeps text; off writes nothing."))
			typeBox.Add(field("Max request bytes", openaiMaxBytes))
			typeBox.Add(caption("Empty or 0 uses the default, 32 MiB."))
			typeBox.Add(exposeReasoning)
			typeBox.Add(caption("Exposing reasoning shows the model's thinking to the caller; off by default."))
			typeBox.Add(field("Request timeout", timeout))
			typeBox.Add(caption("Default 600s for the whole request, including tool rounds."))
		case "bridge":
			timeout.SetPlaceHolder("55s")
			typeBox.Add(field("Deliver URL", deliverURL))
			typeBox.Add(caption("The adapter receives neutral replies here as JSON. Absolute http(s) URL."))
			secret.SetPlaceHolder(secretHint(existing != nil && existing.HasSecret, secretRefOf(existing), "inbound HMAC secret"))
			typeBox.Add(field("Inbound HMAC secret", secret))
			if existing != nil && existing.HasSecret {
				typeBox.Add(clearSecret)
			}
			typeBox.Add(caption("Required. The adapter signs each inbound body with X-Agentflow-Signature."))
			typeBox.Add(field("Outbound bearer token", token))
			if existing != nil && existing.HasToken {
				typeBox.Add(clearToken)
			}
			typeBox.Add(caption("Optional. Sent as Authorization: Bearer when delivering to the adapter."))
			typeBox.Add(field("Delivery timeout", timeout))
			typeBox.Add(caption("Default 10s. The adapter must answer each delivery within it."))
			typeBox.Add(caption("Path is required for a bridge and must end with /."))
		}
		typeBox.Refresh()
	}
	typ.OnChanged = func(string) { renderType() }
	openaiAuth.OnChanged = func(string) { renderType() }
	qqMode.OnChanged = func(string) { renderType() }
	renderType()

	content := container.NewVBox(
		field("Name", name),
		field("Type", typ),
		field("Agent", agent),
		field("Path", path),
		caption("Optional route on the shared listener. An explicit path must end with /."),
		typeBox,
	)
	form := container.NewVBox(content,
		widget.NewSeparator(),
		subheading("Inbound media"),
		field("Allowed MIME patterns", mediaAllow),
		caption("Comma-separated, e.g. image/*, application/pdf. Empty disables attachments."),
		field("Max bytes", mediaMax),
		caption("Defaults to 8 MiB when media is enabled."),
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
			return d, fmt.Errorf("a channel name is required")
		}
		if d.Type == "" {
			return d, fmt.Errorf("choose a channel type")
		}
		if d.Type != "openai" && d.Agent == "" {
			return d, fmt.Errorf("choose an agent")
		}
		switch d.Type {
		case "telegram":
			d.Mode = mode.Selected
			for _, part := range splitList(allowUsers.Text) {
				id, err := strconv.ParseInt(part, 10, 64)
				if err != nil {
					return d, fmt.Errorf("allow users: %q is not a numeric id", part)
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
				return d, fmt.Errorf("the AppID is required")
			}
			if len(d.Scenes) == 0 {
				return d, fmt.Errorf("pick at least one scene")
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
					return d, fmt.Errorf("max request bytes must be a whole number, 0 or more")
				}
				d.MaxRequestBytes = n
			}
		}
		if t := strings.TrimSpace(mediaMax.Text); t != "" {
			n, err := strconv.Atoi(t)
			if err != nil || n < 0 {
				return d, fmt.Errorf("max bytes must be a positive integer")
			}
			d.MediaMaxBytes = n
		}
		return d, nil
	}

	title := "New channel"
	if !isNew {
		title = "Edit " + channelTitle(*existing)
	}
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	validate := actionButton("Validate", checkMarkIcon, nil)
	save := primaryButton("Save", theme.UploadIcon(), nil)
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
					errBanner.Set("Valid. Restart the runtime to bind it.", toneSuccess)
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
				a.setStatus("channel saved · restart required", toneWarning)
			})
		}()
	}

	popup = showModalCard(a.win, title, "Route inbound messages into an agent. Changes take effect on the next runtime start.", dialogContent, footer)
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
