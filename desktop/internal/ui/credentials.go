package ui

import (
	"context"
	"errors"
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
	v.list = vstack(spaceM)
	v.scroll = pageScroll(v.list)
	v.banner = newBanner()
	v.count = oneLine("")
	v.meta = oneLine("")
	v.bucket = widget.NewSelect([]string{bucketLabel("")}, func(string) { v.load() })
	v.bucket.SetSelected(bucketLabel(""))
	v.newBtn = primaryButton(T("credentials.newCredential"), theme.ContentAddIcon(), func() { v.editCredential(nil) })
	v.addUser = actionButton(T("credentials.userBucketButton"), theme.AccountIcon(), func() { v.addUserBucket() })
	refresh := iconButton(theme.ViewRefreshIcon(), T("common.refresh"), v.load)

	head := pageHeader(T("credentials.title"),
		T("credentials.subtitle"),
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
		scrollInset(v.scroll),
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
		return T("credentials.engineWide")
	}
	return user
}

func bucketValue(label string) string {
	if label == T("credentials.engineWide") {
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
	v.meta.SetText(T("credentials.loadingState"))
	user := v.selectedBucket()
	v.loadSeq++
	seq := v.loadSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		list, err := v.app.api.Credentials(ctx, user)
		onUI(func() {
			if seq != v.loadSeq {
				return
			}
			if err != nil {
				v.meta.SetText("")
				msg := err.Error()
				if ae, ok := err.(*api.Error); ok && ae.Status == 404 {
					msg = T("credentials.apiMissing")
				}
				v.setBanner(msg, toneDanger)
				clearBox(v.list)
				v.list.Add(container.NewCenter(widget.NewButtonWithIcon(T("common.retry"), theme.ViewRefreshIcon(), v.load)))
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
	v.count.SetText(T("credentials.count", "n", n))
	where := T("credentials.bucketEngineWide")
	if v.data.Bucket != "" {
		where = T("credentials.bucketUser", "user", v.data.Bucket)
	}
	meta := where + " · " + T("credentials.writeOnlyValues")
	if !v.data.Enabled {
		v.newBtn.Disable()
		v.newBtn.Hide()
		v.addUser.Disable()
		v.addUser.Hide()
		v.meta.SetText(T("credentials.storeDisabled"))
		v.setBanner(v.data.Note, toneWarning)
		v.list.Add(layout.NewSpacer())
		v.list.Add(emptyStateCard(
			T("credentials.disabledTitle"),
			T("credentials.disabledGuidance"),
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
			T("credentials.emptyTitle"),
			T("credentials.emptyGuidance"),
			T("credentials.emptyAction"),
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
		fp = T("credentials.noFingerprint")
	}
	top := container.NewBorder(nil, nil,
		hstack(spaceS, vCenter(title), vCenter(chip(c.Kind, toneAccent)), vCenter(chip(fp, toneSuccess))),
		hstack(spaceXS,
			vCenter(actionButton(T("common.edit"), theme.DocumentCreateIcon(), func() { v.editCredential(&c) })),
			vCenter(iconButton(theme.DeleteIcon(), T("common.delete"), func() { v.confirmDelete(c) })),
		),
		nil,
	)
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
		detailRow(T("credentials.fact.kind"), detailText(c.Kind)),
		detailRow(T("credentials.fact.header"), detailText(auth)),
		detailRow(T("credentials.fact.fingerprint"), detailText(fp)),
		detailRow(T("credentials.fact.updated"), detailText(updated)),
	}
	return panel(vstack(spaceS, top, widget.NewSeparator(), grid(2, cells...)))
}

func (v *credentialsView) editCredential(existing *api.Credential) {
	creating := existing == nil
	service := widget.NewEntry()
	service.SetPlaceHolder("github_token")
	kind := widget.NewSelect([]string{"api_key", "token", "password", "oauth", "generic"}, nil)
	kind.SetSelected("api_key")
	secret := widget.NewPasswordEntry()
	secret.SetPlaceHolder(T("credentials.placeholder.secretNew"))
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
		secret.SetPlaceHolder(T("credentials.placeholder.secretKeep"))
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
		grid(2,
			field(T("credentials.field.service"), service),
			field(T("credentials.field.kind"), kind),
		),
		field(T("credentials.field.secret"), secret),
		caption(T("credentials.hint.secret")),
		grid(2,
			field(T("credentials.field.header"), header),
			field(T("credentials.field.scheme"), scheme),
		),
		caption(T("credentials.hint.sentAs")),
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
			return d, errors.New(T("credentials.error.serviceRequired"))
		}
		if creating && d.Secret == "" {
			return d, errors.New(T("credentials.error.secretRequired"))
		}
		return d, nil
	}

	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	save := primaryButton(T("common.save"), theme.UploadIcon(), nil)
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(104, controlFrame(save))), nil)

	save.OnTapped = func() {
		d, err := build()
		if err != nil {
			errBanner.Set(err.Error(), toneDanger)
			return
		}
		save.Disable()
		save.SetText(T("common.saving"))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			list, err := v.app.api.PutCredential(ctx, d)
			onUI(func() {
				save.Enable()
				save.SetText(T("common.save"))
				if err != nil {
					errBanner.Set(err.Error(), toneDanger)
					return
				}
				closePopup()
				v.data = list
				v.buckets = append([]string{}, list.Users...)
				v.syncBuckets(d.User)
				v.render()
				v.app.setStatus(T("credentials.saved", "service", d.Service), toneSuccess)
			})
		}()
	}

	title := T("credentials.newCredential")
	subtitle := T("credentials.editorSubtitle")
	if existing != nil {
		title = T("credentials.editNamed", "name", existing.Service)
	}
	popup = showModalCard(v.app.win, title, subtitle,
		container.NewBorder(errBanner, nil, nil, nil, scroll), footer)
}

func (v *credentialsView) confirmDelete(c api.Credential) {
	msg := T("credentials.deleteMessageEngine", "service", c.Service)
	if v.data.Bucket != "" {
		msg = T("credentials.deleteMessageUser", "service", c.Service, "user", v.data.Bucket)
	}
	showConfirmPrompt(v.app.win, T("credentials.deleteTitle"), msg, T("common.delete"), true, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			list, err := v.app.api.DeleteCredential(ctx, v.selectedBucket(), c.Service)
			onUI(func() {
				if err != nil {
					v.setBanner(err.Error(), toneDanger)
					return
				}
				v.data = list
				v.buckets = append([]string{}, list.Users...)
				v.syncBuckets(v.selectedBucket())
				v.render()
				v.app.setStatus(T("credentials.deleted", "service", c.Service), toneWarning)
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
	entry.SetPlaceHolder(T("credentials.bucketDialog.placeholder"))
	v.app.loadUserIDs(func(ids []string) { entry.SetOptions(ids) })
	errBanner := newBanner()
	form := vstack(spaceS,
		vstack(spaceXS, fieldLabel(T("credentials.bucketDialog.field")), entry,
			caption(T("credentials.bucketDialog.hint"))),
		errBanner,
	)
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	choose := primaryButton(T("credentials.bucketDialog.open"), nil, func() {
		u := strings.TrimSpace(entry.Text)
		if u == "" {
			errBanner.Set(T("credentials.bucketDialog.error"), toneDanger)
			return
		}
		closePopup()
		v.buckets = append(v.buckets, u)
		v.syncBuckets(u)
		v.load()
	})
	cancel := widthBox(100, controlFrame(actionButton(T("common.cancel"), nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, widthBox(140, choose)), nil)
	popup = showModalCard(v.app.win, T("credentials.bucketDialog.title"),
		T("credentials.bucketDialog.subtitle"),
		form, footer)
	if c := v.app.win.Canvas(); c != nil && popup != nil {
		c.Focus(entry)
	}
}
