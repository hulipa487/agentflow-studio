package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

type credentialsView struct {
	app  *App
	root fyne.CanvasObject

	list    *fyne.Container
	scroll  *container.Scroll
	banner  *banner
	count   *widget.Label
	meta    *widget.Label
	bucket  *widget.Select
	newBtn  *widget.Button
	addUser *widget.Button

	data    api.CredentialList
	buckets []string
	loadSeq int
}

func (a *App) showCredentials() {
	a.enter("credentials")
	a.setMain(a.newCredentialsView().root)
}

func (a *App) newCredentialsView() *credentialsView {
	v := &credentialsView{app: a, buckets: []string{""}}
	v.list = vstack(spaceS)
	v.scroll = container.NewVScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	v.bucket = widget.NewSelect([]string{"Engine-wide"}, func(string) { v.load() })
	v.bucket.SetSelected("Engine-wide")
	v.newBtn = primaryButton("New credential", theme.ContentAddIcon(), func() { v.editCredential(nil) })
	v.addUser = actionButton("User bucket…", theme.AccountIcon(), func() { v.addUserBucket() })
	refresh := iconButton(theme.ViewRefreshIcon(), "Refresh", v.load)

	head := pageHeader("Credentials",
		"Encrypted secrets resolved by the runtime at request time. Values are write-only: Studio can replace a secret but never read it back.",
		hstack(spaceS,
			widthBox(260, v.bucket),
			widthBox(120, v.addUser),
			widthBox(100, v.count),
			refresh,
			v.newBtn,
		))
	v.root = container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, v.meta)), hInset(v.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(v.scroll),
	)
	v.load()
	return v
}

func (v *credentialsView) setBanner(text string, t tone) {
	v.banner.Set(text, t)
	if v.root != nil {
		v.root.Refresh()
	}
}

func bucketLabel(user string) string {
	if user == "" {
		return "Engine-wide"
	}
	return user
}

func bucketValue(label string) string {
	if label == "Engine-wide" {
		return ""
	}
	return label
}

func (v *credentialsView) selectedBucket() string {
	if v.bucket == nil {
		return ""
	}
	return bucketValue(v.bucket.Selected)
}

// syncBuckets rewrites the selector without firing its change handler, and
// keeps a hand-entered uuid selectable even before it has a credential.
func (v *credentialsView) syncBuckets(selected string, extra ...string) {
	seen := map[string]bool{}
	all := make([]string, 0, len(v.buckets)+len(extra)+1)
	for _, u := range append(append([]string{}, v.buckets...), extra...) {
		if seen[u] {
			continue
		}
		seen[u] = true
		all = append(all, u)
	}
	if !seen[""] {
		all = append(all, "")
		seen[""] = true
	}
	if !seen[selected] {
		all = append(all, selected)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i] == "" {
			return all[j] != ""
		}
		if all[j] == "" {
			return false
		}
		return all[i] < all[j]
	})
	v.buckets = all
	labels := make([]string, len(all))
	for i, u := range all {
		labels[i] = bucketLabel(u)
	}
	v.bucket.OnChanged = nil
	v.bucket.Options = labels
	v.bucket.SetSelected(bucketLabel(selected))
	v.bucket.OnChanged = func(string) { v.load() }
	v.bucket.Refresh()
}

func (v *credentialsView) load() {
	v.banner.Clear()
	v.meta.SetText("loading…")
	user := v.selectedBucket()
	v.loadSeq++
	seq := v.loadSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list, err := v.app.api.Credentials(ctx, user)
		fyne.Do(func() {
			if seq != v.loadSeq {
				return
			}
			if err != nil {
				v.meta.SetText("")
				msg := err.Error()
				if ae, ok := err.(*api.Error); ok && ae.Status == 404 {
					msg = "This runtime does not expose the Studio credentials API yet; rebuild and restart AgentFlow, then reopen Credentials."
				}
				v.setBanner(msg, toneDanger)
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), v.load)))
				v.list.Refresh()
				return
			}
			v.data = list
			v.buckets = append([]string{}, list.Users...)
			v.syncBuckets(user)
			v.render()
		})
	}()
}

func (v *credentialsView) render() {
	v.banner.Clear()
	clearBox(v.list)
	n := len(v.data.Credentials)
	if n == 1 {
		v.count.SetText("1 secret")
	} else {
		v.count.SetText(fmt.Sprintf("%d secrets", n))
	}
	where := "engine-wide bucket"
	if v.data.Bucket != "" {
		where = "user " + v.data.Bucket
	}
	meta := where + " · write-only values"
	if !v.data.Enabled {
		v.newBtn.Disable()
		v.newBtn.Hide()
		v.addUser.Disable()
		v.addUser.Hide()
		v.meta.SetText("credential store disabled")
		v.setBanner(v.data.Note, toneWarning)
		v.list.Add(layout.NewSpacer())
		v.list.Add(emptyStateCard(
			"Credentials are disabled",
			"Set runtime.credentials.enabled: true and CREDENTIALS_MASTER_KEY, restart AgentFlow, then reopen this screen.",
			"", nil,
		))
		v.list.Add(layout.NewSpacer())
		v.list.Refresh()
		v.root.Refresh()
		return
	}
	v.newBtn.Enable()
	v.addUser.Enable()
	v.addUser.Show()
	v.meta.SetText(meta)
	if n == 0 {
		v.newBtn.Hide()
		v.list.Add(layout.NewSpacer())
		v.list.Add(emptyStateCard(
			"No credentials in this bucket",
			"Store an API key or token here; loops reference it by name and the runtime injects the value at request time.",
			"Add a credential",
			func() { v.editCredential(nil) },
		))
		v.list.Add(layout.NewSpacer())
		v.list.Refresh()
		v.root.Refresh()
		return
	}
	v.newBtn.Show()
	for _, c := range v.data.Credentials {
		v.list.Add(v.credentialCard(c))
	}
	v.list.Refresh()
	v.root.Refresh()
}

func (v *credentialsView) credentialCard(c api.Credential) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(c.Service, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	fp := c.Fingerprint
	if fp == "" {
		fp = "no fingerprint"
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(c.Kind, toneAccent)), vCenter(chip(fp, toneSuccess))),
		hstack(spaceXS,
			vCenter(actionButton("Edit", theme.DocumentCreateIcon(), func() { v.editCredential(&c) })),
			vCenter(iconButton(theme.DeleteIcon(), "Delete", func() { v.confirmDelete(c) })),
		),
		nil,
	)
	row := func(label string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, widthBox(108, fieldLabel(label)), nil, value)
	}
	auth := "Authorization: Bearer <secret>"
	switch {
	case c.Header != "" && c.Scheme != "":
		auth = c.Header + ": " + c.Scheme + " <secret>"
	case c.Header != "":
		auth = c.Header + ": <secret>"
	}
	updated := "—"
	if c.UpdatedAt > 0 {
		updated = time.Unix(c.UpdatedAt, 0).Local().Format("2006-01-02 15:04")
	}
	cells := []fyne.CanvasObject{
		row("kind", widget.NewLabel(c.Kind)),
		row("header", monoInline(auth)),
		row("fingerprint", monoInline(fp)),
		row("updated", widget.NewLabel(updated)),
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), container.NewGridWithColumns(2, cells...)))
}

func (v *credentialsView) editCredential(existing *api.Credential) {
	creating := existing == nil
	service := widget.NewEntry()
	service.SetPlaceHolder("github_token")
	kind := widget.NewSelect([]string{"api_key", "token", "password", "oauth", "generic"}, nil)
	kind.SetSelected("api_key")
	secret := widget.NewPasswordEntry()
	secret.SetPlaceHolder("the value (never returned again)")
	header := widget.NewEntry()
	header.SetText("Authorization")
	scheme := widget.NewEntry()
	scheme.SetText("Bearer")

	if existing != nil {
		service.SetText(existing.Service)
		service.Disable()
		if existing.Kind != "" {
			opts := kind.Options
			if !hasString(opts, existing.Kind) {
				opts = append(opts, existing.Kind)
				kind.Options = opts
				kind.Refresh()
			}
			kind.SetSelected(existing.Kind)
		}
		secret.SetPlaceHolder("leave blank to keep the stored value")
		if existing.Header != "" {
			header.SetText(existing.Header)
		}
		scheme.SetText(existing.Scheme)
	}

	errBanner := newBanner()
	field := func(label string, control fyne.CanvasObject) fyne.CanvasObject {
		return vstack(spaceXS, fieldLabel(label), controlFrame(control))
	}
	form := vstack(spaceM,
		container.NewGridWithColumns(2,
			field("Service", service),
			field("Kind", kind),
		),
		field("Secret value", secret),
		caption("Write-only: the stored value is encrypted at rest and never returned by the API. Leave blank on an edit to keep it."),
		container.NewGridWithColumns(2,
			field("Header", header),
			field("Scheme", scheme),
		),
		caption("Sent as <header>: <scheme> <value>. Defaults are Authorization: Bearer <value>."),
	)
	scroll := container.NewVScroll(panel(form))
	scroll.SetMinSize(fyne.NewSize(680, 420))

	build := func() (api.CredentialDraft, error) {
		d := api.CredentialDraft{
			User:    v.selectedBucket(),
			Service: strings.TrimSpace(service.Text),
			Kind:    kind.Selected,
			Secret:  secret.Text,
			Header:  strings.TrimSpace(header.Text),
			Scheme:  strings.TrimSpace(scheme.Text),
		}
		if d.Service == "" {
			return d, fmt.Errorf("a service name is required")
		}
		if creating && d.Secret == "" {
			return d, fmt.Errorf("a secret value is required")
		}
		return d, nil
	}

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	save := primaryButton("Save", theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(104, controlFrame(save))), nil)

	save.OnTapped = func() {
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		save.Disable()
		save.SetText("Saving…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			list, err := v.app.api.PutCredential(ctx, d)
			fyne.Do(func() {
				save.Enable()
				save.SetText("Save")
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.data = list
				v.buckets = append([]string{}, list.Users...)
				v.syncBuckets(d.User)
				v.render()
				v.app.setStatus("credential "+d.Service+" saved · live", toneSuccess)
			})
		}()
	}

	title := "New credential"
	subtitle := "Stored encrypted, resolved by the runtime at request time, and never shown again."
	if existing != nil {
		title = "Edit " + existing.Service
	}
	popup = showModalCard(v.app.win, title, subtitle,
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *credentialsView) confirmDelete(c api.Credential) {
	msg := fmt.Sprintf("Delete credential %q from the %s?", c.Service, bucketLabel(v.data.Bucket))
	msg += "\n\nAny loop or config reference that resolves it will fail until a new secret is stored."
	showConfirmPrompt(v.app.win, "Delete credential", msg, "Delete", true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			list, err := v.app.api.DeleteCredential(ctx, v.selectedBucket(), c.Service)
			fyne.Do(func() {
				if err != nil {
					v.setBanner(err.Error(), toneDanger)
					return
				}
				v.data = list
				v.buckets = append([]string{}, list.Users...)
				v.syncBuckets(v.selectedBucket())
				v.render()
				v.app.setStatus("credential "+c.Service+" deleted", toneWarning)
			})
		}()
	})
}

// addUserBucket lets an operator address a UUID before it holds a credential.
func (v *credentialsView) addUserBucket() {
	if !v.data.Enabled {
		return
	}
	entry := widget.NewSelectEntry(nil)
	entry.SetPlaceHolder("user uuid")
	v.app.loadUserIDs(func(ids []string) { entry.SetOptions(ids) })
	errBanner := newBanner()
	form := vstack(spaceS,
		vstack(spaceXS, fieldLabel("User UUID"), entry,
			caption("Per-user credentials resolve only for that tenant. Leave empty to use the engine-wide bucket.")),
		errBanner,
	)
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	choose := primaryButton("Open bucket", nil, func() {
		u := strings.TrimSpace(entry.Text)
		if u == "" {
			errBanner.Set("Enter a user UUID, or cancel and use the Engine-wide bucket.", toneDanger)
			return
		}
		closePopup()
		v.buckets = append(v.buckets, u)
		v.syncBuckets(u)
		v.load()
	})
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(140, choose)), nil)
	popup = showModalCard(v.app.win, "Open a user bucket",
		"Credentials are keyed by (user uuid, service). A user without a stored credential still needs to be named to add one.",
		form, footer)
	if c := v.app.win.Canvas(); c != nil && popup != nil {
		c.Focus(entry)
	}
}
