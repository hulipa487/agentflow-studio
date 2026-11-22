package main

// In-app self-updater for AgentFlow Studio desktop: checks GitHub Releases for
// the configured repository, verifies SHA-256 digests against SHA256SUMS.txt
// when present, atomically replaces the running executable (or macOS .app
// bundle), and spawns the updated binary after the old process exits.

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultUpdateRepo = "hulipa487/agentflow-studio"
	defaultGitHubAPI  = "https://api.github.com"
	maxUpdateBytes    = 256 << 20 // 256 MiB asset cap
	maxChecksumBytes  = 1 << 20   // 1 MiB SHA256SUMS.txt cap
	maxTarExtractSize = 512 << 20 // 512 MiB unpacked bundle cap
)

// updateStatus is returned to the UI by /__af/__desktop/update/{status,check}.
type updateStatus struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	IsDev           bool   `json:"is_dev"`
	Repo            string `json:"repo"`
	Platform        string `json:"platform"`
	Arch            string `json:"arch"`
	CanSelfUpdate   bool   `json:"can_self_update"`
	ManagedReason   string `json:"managed_reason,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	CheckedAt       string `json:"checked_at,omitempty"`
	Applying        bool   `json:"applying,omitempty"`
	Error           string `json:"error,omitempty"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	HTMLURL string               `json:"html_url"`
	Body    string               `json:"body"`
	Draft   bool                 `json:"draft"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type updater struct {
	mu sync.Mutex

	repo           string
	apiBase        string
	currentVersion string
	goos           string
	goarch         string
	exePath        string
	httpClient     *http.Client
	downloadClient *http.Client
	relaunch       func(exePath, appPath, goos string) error

	// Latest check state.
	st          updateStatus
	downloadURL string
	expectedSHA string
}

func newUpdater(currentVersion string) *updater {
	repo := strings.TrimSpace(os.Getenv("AGENTFLOW_STUDIO_UPDATE_REPO"))
	if repo == "" {
		repo = defaultUpdateRepo
	}
	apiBase := strings.TrimRight(strings.TrimSpace(os.Getenv("AGENTFLOW_STUDIO_UPDATE_API")), "/")
	if apiBase == "" {
		apiBase = defaultGitHubAPI
	}
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	u := &updater{
		repo:           repo,
		apiBase:        apiBase,
		currentVersion: currentVersion,
		goos:           goruntime.GOOS,
		goarch:         goruntime.GOARCH,
		exePath:        exe,
		httpClient:     &http.Client{Timeout: 20 * time.Second},
		downloadClient: &http.Client{Timeout: 5 * time.Minute},
		relaunch:       defaultRelaunch,
	}
	u.st = u.baseStatus()
	return u
}

func (u *updater) baseStatus() updateStatus {
	canUpdate, reason := u.canUpdateTarget()
	return updateStatus{
		CurrentVersion: u.currentVersion,
		IsDev:          !isSemver(u.currentVersion),
		Repo:           u.repo,
		Platform:       u.goos,
		Arch:           u.goarch,
		CanSelfUpdate:  canUpdate,
		ManagedReason:  reason,
	}
}

func (u *updater) canUpdateTarget() (bool, string) {
	if assetNameFor(u.goos, u.goarch) == "" {
		return false, "unsupported_platform"
	}
	if u.exePath == "" {
		return false, "unknown_executable"
	}
	targetDir := filepath.Dir(u.exePath)
	if app := appBundleRoot(u.exePath); app != "" {
		targetDir = filepath.Dir(app)
	}
	if !canWriteDir(targetDir) {
		return false, "readonly"
	}
	return true, ""
}

func (u *updater) status() updateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.st.CurrentVersion == "" {
		u.st = u.baseStatus()
	}
	return u.st
}

// check queries GitHub Releases for the newest non-draft release and updates
// cached status. A 404 (no published releases yet) clears any error cleanly.
func (u *updater) check(ctx context.Context) (updateStatus, error) {
	u.mu.Lock()
	repo, apiBase, goos, goarch, cur := u.repo, u.apiBase, u.goos, u.goarch, u.currentVersion
	base := u.baseStatus()
	u.mu.Unlock()

	url := apiBase + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return base, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agentflow-studio/"+cur)

	res, err := u.httpClient.Do(req)
	if err != nil {
		u.recordError(base, err.Error())
		return u.status(), err
	}
	defer res.Body.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	if res.StatusCode == http.StatusNotFound {
		base.CheckedAt = now
		u.setResult(base, "", "")
		return base, nil
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		msg := fmt.Sprintf("GitHub Releases returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
		u.recordError(base, msg)
		return u.status(), errors.New(msg)
	}

	var rel githubRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&rel); err != nil {
		u.recordError(base, err.Error())
		return u.status(), err
	}

	wantAsset := assetNameFor(goos, goarch)
	var downloadURL, sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case wantAsset:
			downloadURL = a.BrowserDownloadURL
		case "SHA256SUMS.txt":
			sumsURL = a.BrowserDownloadURL
		}
	}

	var expectedSHA string
	if downloadURL != "" && sumsURL != "" {
		sums, err := u.fetchChecksums(ctx, sumsURL)
		if err != nil {
			u.recordError(base, fmt.Sprintf("checksums download failed: %v", err))
			return u.status(), err
		}
		expectedSHA = sums[wantAsset]
	}

	allowDev := os.Getenv("AGENTFLOW_STUDIO_UPDATE_ALLOW_DEV") == "1"
	hasUpdate := false
	if !rel.Draft && rel.TagName != "" && isSemver(rel.TagName) && downloadURL != "" {
		if base.IsDev {
			hasUpdate = allowDev
		} else {
			hasUpdate = compareVersions(rel.TagName, cur) > 0
		}
	}

	next := base
	next.LatestVersion = rel.TagName
	next.UpdateAvailable = hasUpdate
	next.ReleaseURL = rel.HTMLURL
	next.ReleaseNotes = strings.TrimSpace(rel.Body)
	next.AssetName = wantAsset
	next.CheckedAt = now
	if downloadURL == "" && rel.TagName != "" {
		next.CanSelfUpdate = false
		if next.ManagedReason == "" {
			next.ManagedReason = "missing_asset"
		}
	}
	u.setResult(next, downloadURL, expectedSHA)
	return next, nil
}

func (u *updater) fetchChecksums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "agentflow-studio/"+u.currentVersion)
	res, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxChecksumBytes))
	if err != nil {
		return nil, err
	}
	return parseSHA256Sums(string(b)), nil
}

func (u *updater) recordError(base updateStatus, msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	base.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	base.Error = msg
	u.st = base
}

func (u *updater) setResult(st updateStatus, downloadURL, expectedSHA string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.st = st
	u.downloadURL = downloadURL
	u.expectedSHA = expectedSHA
}

// apply downloads the checked release asset, verifies SHA-256 (when available),
// atomically swaps the running binary or .app bundle, and launches the updated
// copy before calling quit.
func (u *updater) apply(ctx context.Context, quit func()) error {
	u.mu.Lock()
	if u.st.Applying {
		u.mu.Unlock()
		return errors.New("an update is already in progress")
	}
	if !u.st.UpdateAvailable || u.downloadURL == "" {
		u.mu.Unlock()
		return errors.New("no update is available")
	}
	if !u.st.CanSelfUpdate {
		reason := u.st.ManagedReason
		u.mu.Unlock()
		return fmt.Errorf("automatic update is unavailable on this installation (%s)", reason)
	}
	u.st.Applying = true
	u.st.Error = ""
	dlURL := u.downloadURL
	wantSHA := u.expectedSHA
	assetName := u.st.AssetName
	exePath := u.exePath
	goos := u.goos
	u.mu.Unlock()

	fail := func(err error) error {
		u.mu.Lock()
		u.st.Applying = false
		u.st.Error = err.Error()
		u.mu.Unlock()
		return err
	}

	tmpDir, err := os.MkdirTemp("", "af-studio-update-*")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmpDir)

	dlFile := filepath.Join(tmpDir, assetName)
	gotSHA, err := u.downloadToFile(ctx, dlURL, dlFile)
	if err != nil {
		return fail(err)
	}
	if wantSHA != "" && !strings.EqualFold(gotSHA, wantSHA) {
		return fail(fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, gotSHA, wantSHA))
	}

	appPath := appBundleRoot(exePath)
	switch {
	case goos == "windows":
		if err := swapExecutableFile(dlFile, exePath); err != nil {
			return fail(err)
		}
	case goos == "darwin" && appPath != "":
		if err := installMacAppArchive(dlFile, appPath); err != nil {
			return fail(err)
		}
	default:
		if err := installTarGzBinary(dlFile, exePath); err != nil {
			return fail(err)
		}
	}

	relaunch := u.relaunch
	go func() {
		// Brief delay lets the HTTP handler send its {"ok":true} JSON response
		// before the process exits.
		time.Sleep(150 * time.Millisecond)
		if relaunch != nil {
			_ = relaunch(exePath, appPath, goos)
		}
		if quit != nil {
			quit()
		}
	}()
	return nil
}

func (u *updater) downloadToFile(ctx context.Context, url, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "agentflow-studio/"+u.currentVersion)
	res, err := u.downloadClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, maxUpdateBytes+1))
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > maxUpdateBytes {
		return "", fmt.Errorf("download exceeds %d bytes limit", maxUpdateBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// swapExecutableFile replaces exePath with newFile atomically enough for a
// running Windows binary (Windows allows renaming a running .exe away, then
// putting the replacement in its path).
func swapExecutableFile(newFile, exePath string) error {
	dir := filepath.Dir(exePath)
	staged, err := os.CreateTemp(dir, ".af-studio-new-*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)

	src, err := os.Open(newFile)
	if err != nil {
		_ = staged.Close()
		return err
	}
	_, copyErr := io.Copy(staged, src)
	_ = src.Close()
	if closeErr := staged.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil && goruntime.GOOS != "windows" {
		return err
	}

	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)
	hadExe := false
	if _, err := os.Stat(exePath); err == nil {
		hadExe = true
		if err := os.Rename(exePath, oldPath); err != nil {
			return fmt.Errorf("move current binary aside: %w", err)
		}
	}
	if err := os.Rename(stagedPath, exePath); err != nil {
		if hadExe {
			_ = os.Rename(oldPath, exePath)
		}
		return fmt.Errorf("install new binary: %w", err)
	}
	// On POSIX systems the unlinked/renamed old binary can be removed at once;
	// on Windows the running process still holds oldPath open until restart.
	if goruntime.GOOS != "windows" {
		_ = os.Remove(oldPath)
	}
	return nil
}

// installTarGzBinary extracts the agentflow-studio binary from archivePath and
// swaps it into exePath.
func installTarGzBinary(archivePath, exePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	tmpOut, err := os.CreateTemp("", "af-studio-bin-*")
	if err != nil {
		return err
	}
	tmpOutPath := tmpOut.Name()
	defer os.Remove(tmpOutPath)

	found := false
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = tmpOut.Close()
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(hdr.Name))
		base := filepath.Base(clean)
		if base == "agentflow-studio" && !strings.Contains(clean, "..") {
			if _, err := io.Copy(tmpOut, io.LimitReader(tr, maxUpdateBytes+1)); err != nil {
				_ = tmpOut.Close()
				return err
			}
			found = true
			break
		}
	}
	if err := tmpOut.Close(); err != nil {
		return err
	}
	if !found {
		return errors.New("archive does not contain agentflow-studio binary")
	}
	return swapExecutableFile(tmpOutPath, exePath)
}

// installMacAppArchive safely extracts the .app bundle from archivePath and
// atomically swaps it into appPath.
func installMacAppArchive(archivePath, appPath string) error {
	parent := filepath.Dir(appPath)
	stage, err := os.MkdirTemp(parent, ".af-studio-app-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	if err := extractTarGzSafe(archivePath, stage); err != nil {
		return err
	}
	newApp, err := findAppBundle(stage, filepath.Base(appPath))
	if err != nil {
		return err
	}

	oldApp := appPath + ".old"
	_ = os.RemoveAll(oldApp)
	if err := os.Rename(appPath, oldApp); err != nil {
		return fmt.Errorf("move current app aside: %w", err)
	}
	if err := os.Rename(newApp, appPath); err != nil {
		_ = os.Rename(oldApp, appPath)
		return fmt.Errorf("install new app: %w", err)
	}
	_ = os.RemoveAll(oldApp)
	return nil
}

func findAppBundle(root, wantBase string) (string, error) {
	direct := filepath.Join(root, wantBase)
	if st, err := os.Stat(direct); err == nil && st.IsDir() {
		return direct, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(root, e.Name()), nil
		}
	}
	return "", errors.New("archive does not contain a .app bundle")
}

func extractTarGzSafe(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var total int64
	cleanDest := filepath.Clean(dest)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || name == "." {
			continue
		}
		if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
			return fmt.Errorf("archive entry has absolute path: %q", hdr.Name)
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(name))
		rel, err := filepath.Rel(cleanDest, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive entry escapes destination: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += hdr.Size
			if hdr.Size < 0 || total > maxTarExtractSize {
				return errors.New("archive exceeds unpacked size limit")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, hdr.Size+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if n != hdr.Size {
				return fmt.Errorf("truncated archive entry %q", hdr.Name)
			}
		case tar.TypeSymlink:
			linkDest := filepath.Clean(filepath.Join(filepath.Dir(target), filepath.FromSlash(hdr.Linkname)))
			linkRel, err := filepath.Rel(cleanDest, linkDest)
			if err != nil || filepath.IsAbs(hdr.Linkname) || linkRel == ".." || strings.HasPrefix(linkRel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("archive symlink escapes destination: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// defaultRelaunch waits for the current PID to exit (avoiding SingleInstanceLock
// collisions) and starts the newly installed executable or .app bundle.
func defaultRelaunch(exePath, appPath, goos string) error {
	pid := strconv.Itoa(os.Getpid())
	switch goos {
	case "windows":
		escExe := strings.ReplaceAll(exePath, "'", "''")
		ps := fmt.Sprintf("Wait-Process -Id %s -Timeout 10 -ErrorAction SilentlyContinue; Start-Sleep -Milliseconds 150; Start-Process -FilePath '%s'", pid, escExe)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps)
		return cmd.Start()
	case "darwin":
		target := exePath
		sh := `i=0; while kill -0 "$1" 2>/dev/null && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i+1)); done; exec "$2" >/dev/null 2>&1 &`
		if appPath != "" {
			target = appPath
			sh = `i=0; while kill -0 "$1" 2>/dev/null && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i+1)); done; exec open -n "$2"`
		}
		return exec.Command("/bin/sh", "-c", sh, "_", pid, target).Start()
	default:
		sh := `i=0; while kill -0 "$1" 2>/dev/null && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i+1)); done; exec "$2" >/dev/null 2>&1 &`
		return exec.Command("/bin/sh", "-c", sh, "_", pid, exePath).Start()
	}
}

// cleanupOldUpdateFiles removes leftover .old / .new files from a previous
// self-update on startup (especially on Windows where the running .exe could not
// delete its .old rename until exit).
func cleanupOldUpdateFiles() {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	cleanupOldUpdateFilesAt(exe)
}

func cleanupOldUpdateFilesAt(exePath string) {
	if exePath == "" {
		return
	}
	_ = os.Remove(exePath + ".old")
	_ = os.Remove(exePath + ".new")
	if app := appBundleRoot(exePath); app != "" {
		_ = os.RemoveAll(app + ".old")
	}
}

func appBundleRoot(exePath string) string {
	clean := filepath.Clean(exePath)
	macOSDir := filepath.Dir(clean)
	if filepath.Base(macOSDir) != "MacOS" {
		return ""
	}
	contentsDir := filepath.Dir(macOSDir)
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	appDir := filepath.Dir(contentsDir)
	if !strings.HasSuffix(strings.ToLower(appDir), ".app") {
		return ""
	}
	return appDir
}

func canWriteDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".af-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func assetNameFor(goos, goarch string) string {
	if goarch != "amd64" && goarch != "arm64" {
		return ""
	}
	switch goos {
	case "windows":
		return "agentflow-studio-windows-" + goarch + ".exe"
	case "darwin":
		return "agentflow-studio-darwin-" + goarch + ".tar.gz"
	case "linux":
		return "agentflow-studio-linux-" + goarch + ".tar.gz"
	default:
		return ""
	}
}

func parseSHA256Sums(text string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		name := strings.TrimPrefix(fields[1], "*")
		name = filepath.Base(filepath.FromSlash(name))
		if len(sum) == 64 && name != "" {
			out[name] = sum
		}
	}
	return out
}

type semver struct {
	nums [3]int
	pre  string
}

func isSemver(raw string) bool {
	_, ok := parseSemver(raw)
	return ok
}

func parseSemver(raw string) (semver, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if s == "" {
		return semver{}, false
	}
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	var pre string
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return semver{}, false
	}
	var out semver
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver{}, false
		}
		out.nums[i] = n
	}
	out.pre = pre
	return out, true
}

func compareVersions(a, b string) int {
	sa, oka := parseSemver(a)
	sb, okb := parseSemver(b)
	if !oka || !okb {
		return strings.Compare(strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v"))
	}
	for i := 0; i < 3; i++ {
		if sa.nums[i] < sb.nums[i] {
			return -1
		}
		if sa.nums[i] > sb.nums[i] {
			return 1
		}
	}
	if sa.pre == sb.pre {
		return 0
	}
	if sa.pre == "" {
		return 1
	}
	if sb.pre == "" {
		return -1
	}
	pa := strings.Split(sa.pre, ".")
	pb := strings.Split(sb.pre, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, erra := strconv.Atoi(pa[i])
		nb, errb := strconv.Atoi(pb[i])
		switch {
		case erra == nil && errb == nil:
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
		case erra == nil && errb != nil:
			return -1
		case erra != nil && errb == nil:
			return 1
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	if len(pa) < len(pb) {
		return -1
	}
	if len(pa) > len(pb) {
		return 1
	}
	return 0
}
