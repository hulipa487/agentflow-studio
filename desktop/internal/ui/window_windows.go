//go:build windows

package ui

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"golang.org/x/sys/windows"
)

const (
	// Window messages handled by the custom-chrome window procedure.
	wmSize              = 0x0005
	wmWindowPosChanging = 0x0046
	wmGetMinMaxInfo     = 0x0024
	wmNCCalcSize        = 0x0083
	wmNCHitTest         = 0x0084
	wmNCActivate        = 0x0086
	wmNCMouseMove       = 0x00A0
	wmNCLButtonDown     = 0x00A1
	wmNCMouseLeave      = 0x02A3
	wmEnterSizeMove     = 0x0231
	wmExitSizeMove      = 0x0232
	wmClose             = 0x0010

	// TrackMouseEvent flags used to receive WM_NCMOUSELEAVE.
	tmeLeave     = 0x0002
	tmeNonClient = 0x0010

	// Hit-test results.
	htClient      = 1
	htCaption     = 2
	htMinButton   = 8
	htMaxButton   = 9
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
	htClose       = 20

	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
	swpNoZOrder    = 0x0004
	swpNoActivate  = 0x0010
	swpFrameChange = 0x0020

	swMinimize = 6
	swMaximize = 3
	swRestore  = 9

	// GWLP_WNDPROC is -4; pass its low 32 bits.
	wndProcIndex = uintptr(0xFFFFFFFC)

	// GWL_STYLE / GWL_EXSTYLE are -16 and -20.
	gwlStyle   = uintptr(^uint(15))
	gwlExStyle = uintptr(^uint(19))

	// Remove the caption style so Windows/DWM cannot paint native
	// min/max/close glyphs beside our custom chrome. Resizing (WS_THICKFRAME),
	// the system menu, and min/max boxes stay enabled.
	wsCaption = 0x00C00000

	// MonitorFromWindow flags.
	monitorDefaultToNearest = 2

	// DWMWA_* attributes.
	dwmwaCornerPreference = 33
	dwmwaBorderColor      = 34
	dwmCornerRound        = 2
	dwmColorNone          = 0xFFFFFFFE
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW           = user32.NewProc("FindWindowW")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW       = user32.NewProc("CallWindowProcW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procClientToScreen        = user32.NewProc("ClientToScreen")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procShowWindow            = user32.NewProc("ShowWindow")
	procIsZoomed              = user32.NewProc("IsZoomed")
	procGetDpiForWindow       = user32.NewProc("GetDpiForWindow")
	procMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW       = user32.NewProc("GetMonitorInfoW")
	procTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procAdjustWindowRect      = user32.NewProc("AdjustWindowRectEx")
	procAdjustWindowRectDPI   = user32.NewProc("AdjustWindowRectExForDpi")
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmExtendFrame        = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	originalWndProc uintptr
	wndProcCallback = windows.NewCallback(studioWndProc)
	chromeInstalled bool

	zoomMu        sync.Mutex
	zoomObserver  func(bool)
	lastZoomState bool

	chromeMu            sync.Mutex
	chromeHoverObserver func(chromeControl)
	lastChromeControl   chromeControl

	chromeRefreshPending atomic.Bool
	// skipFrameAdjust bypasses the WM_WINDOWPOSCHANGING compensation for our
	// own corrections, which already use the final outer size.
	skipFrameAdjust atomic.Bool
	// inSizeMove is set between WM_ENTERSIZEMOVE and WM_EXITSIZEMOVE.
	inSizeMove bool

	// sidebarRegion reports whether the current screen draws the sidebar
	// toggle as the leftmost window control. The hit test leaves a client-area
	// hole only while it is visible.
	sidebarRegion atomic.Bool
)

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type minMaxInfo struct {
	Reserved     winPoint
	MaxSize      winPoint
	MaxPosition  winPoint
	MinTrackSize winPoint
	MaxTrackSize winPoint
}

type nccalcSizeParams struct {
	Rect  [3]winRect
	Place uintptr
}

type windowPos struct {
	HWnd   uintptr
	Insert uintptr
	X, Y   int32
	CX, CY int32
	Flags  uint32
}

type monitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HwndTrack uintptr
	HoverTime uint32
}

type margins struct {
	Left   int32
	Right  int32
	Top    int32
	Bottom int32
}

func borderlessSupported() bool { return true }

func findWindow(title string) (windows.Handle, bool) {
	p, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, false
	}
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(p)))
	return windows.Handle(h), h != 0
}

// MakeBorderless keeps the native resize/caption styles (so Windows still
// knows the window is resizable and snap-capable), then hides the non-client
// area with WM_NCCALCSIZE and classifies the surface with WM_NCHITTEST.
func MakeBorderless(title string) {
	h, ok := findWindow(title)
	if !ok {
		return
	}
	if !chromeInstalled {
		orig, _, _ := procGetWindowLongPtrW.Call(uintptr(h), wndProcIndex)
		originalWndProc = orig
		procSetWindowLongPtrW.Call(uintptr(h), wndProcIndex, wndProcCallback)
		chromeInstalled = true
	}
	if style, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlStyle); style != 0 {
		procSetWindowLongPtrW.Call(uintptr(h), gwlStyle, style&^wsCaption)
	}
	if z, _, _ := procIsZoomed.Call(uintptr(h)); z != 0 {
		lastZoomState = true
	}
	// Re-run WM_NCCALCSIZE now that the subclass is in place.
	procSetWindowPos.Call(uintptr(h), 0, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoZOrder|swpFrameChange)

	// DWM: native shadow/rounded corners, no visible border. Failures are
	// harmless (older Windows builds reject some attributes).
	m := margins{Left: -1, Right: -1, Top: -1, Bottom: -1}
	procDwmExtendFrame.Call(uintptr(h), uintptr(unsafe.Pointer(&m)))
	corner := int32(dwmCornerRound)
	procDwmSetWindowAttribute.Call(uintptr(h), uintptr(dwmwaCornerPreference),
		uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	none := uint32(dwmColorNone)
	procDwmSetWindowAttribute.Call(uintptr(h), uintptr(dwmwaBorderColor),
		uintptr(unsafe.Pointer(&none)), unsafe.Sizeof(none))

	// The first window size was requested before this procedure existed, so
	// its client may carry the hidden frame. Correct that once; subsequent
	// resizes are compensated in WM_WINDOWPOSCHANGING.
	time.AfterFunc(250*time.Millisecond, func() { trimClientToCanvas(uintptr(h)) })
}

// studioWndProc gives the frameless window native hit-testing. Everything it
// does not handle is passed to GLFW's original procedure.

func studioWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmNCMouseMove:
		// The window buttons are non-client regions, so Fyne never sees their
		// mouse moves. Mirror the control under the cursor into the drawn
		// hover state, then ask for a leave notification.
		if x, y, ok := windowCursor(); ok {
			if code, ok := hitTestAt(hwnd, x, y); ok {
				notifyChromeControl(controlFromHit(code))
			}
		}
		trackNCMouseLeave(hwnd)
	case wmNCMouseLeave:
		notifyChromeControl(controlNone)
	case wmNCLButtonDown:
		// The window buttons are classified as native non-client regions, but
		// GLFW swallows the default caption handling. Perform the action here.
		switch int(wparam) {
		case htMinButton:
			procShowWindow.Call(hwnd, swMinimize)
			return 0
		case htMaxButton:
			if z, _, _ := procIsZoomed.Call(hwnd); z != 0 {
				procShowWindow.Call(hwnd, swRestore)
			} else {
				procShowWindow.Call(hwnd, swMaximize)
			}
			return 0
		case htClose:
			procPostMessageW.Call(hwnd, wmClose, 0, 0)
			return 0
		}
	case wmEnterSizeMove:
		inSizeMove = true
	case wmExitSizeMove:
		inSizeMove = false
	case wmWindowPosChanging:
		// GLFW computes a window size that includes the non-client frame even
		// though WM_NCCALCSIZE hides it. Removing the frame here means every
		// programmatic resize lands on the requested client size instead of
		// growing the window by the frame.
		// A restore request arrives while the window still reports maximized.
		// Leave it alone: Windows restores the pre-maximize rect, and adjusting
		// it would shrink the restored window by the hidden frame.
		if inSizeMove || skipFrameAdjust.Load() || lastZoomState {
			break
		}
		if z, _, _ := procIsZoomed.Call(hwnd); z != 0 {
			break
		}
		wp := (*windowPos)(pointerAt(lparam))
		if wp.Flags&swpNoSize != 0 {
			break
		}
		// GLFW calls SetWindowPos with SWP_NOACTIVATE; Windows-initiated
		// layout changes (Aero Snap, restore) do not. Only compensate for the
		// former, so native layout keeps its exact geometry.
		if wp.Flags&swpNoActivate == 0 {
			break
		}
		// A pure move repeats the current size; shrinking it would be wrong.
		var cur winRect
		if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&cur))); ret != 0 {
			if wp.CX == cur.Right-cur.Left && wp.CY == cur.Bottom-cur.Top {
				break
			}
		}
		// Maximize/restore requests cover the monitor; leave those alone.
		if work, ok := monitorWorkArea(hwnd); ok {
			if wp.CX >= work.Right-work.Left-32 && wp.CY >= work.Bottom-work.Top-32 {
				break
			}
		}
		fw, fh := windowFrameSize(hwnd)
		if wp.CX > fw && wp.CY > fh {
			wp.CX -= fw
			wp.CY -= fh
		}
		return 0
	case wmSize:
		// Windows handles the maximize button itself (HTMAXBUTTON), so Fyne
		// never sees the click. Track the zoomed state so the drawn glyph can
		// switch between maximize and restore.
		zoomed := false
		if z, _, _ := procIsZoomed.Call(hwnd); z != 0 {
			zoomed = true
		}
		if zoomed != lastZoomState {
			lastZoomState = zoomed
			zoomMu.Lock()
			fn := zoomObserver
			zoomMu.Unlock()
			if fn != nil {
				onUI(func() { fn(zoomed) })
			}
		}
		// The new layout may shift the controls' parent containers without
		// moving the buttons, so re-read their positions once it has settled.
		if chromeRefreshPending.CompareAndSwap(false, true) {
			onUI(func() {
				chromeRefreshPending.Store(false)
				refreshChromeGeometry()
			})
		}
	case wmNCCalcSize:
		if wparam != 0 {
			// The client covers the whole window so no native caption/frame is
			// painted. A maximized window's rect is inflated beyond the work
			// area by the frame it no longer shows, so clamp the client to the
			// monitor work area instead of letting content slide under the
			// taskbar.
			if maximized, _, _ := procIsZoomed.Call(hwnd); maximized != 0 {
				if work, ok := monitorWorkArea(hwnd); ok {
					params := (*nccalcSizeParams)(pointerAt(lparam))
					params.Rect[0] = work
				}
			}
			return 0
		}
	case wmNCHitTest:
		if code, ok := chromeHitTest(hwnd, lparam); ok {
			return uintptr(code)
		}
	case wmNCActivate:
		notifyChromeControl(controlNone)
		return 1
	case wmGetMinMaxInfo:
		scale := hitTestScale(hwnd)
		mmi := (*minMaxInfo)(pointerAt(lparam))
		mmi.MinTrackSize.X = int32(1080 * scale)
		mmi.MinTrackSize.Y = int32(700 * scale)
		// Without WS_CAPTION Windows maximizes to the full monitor. Constrain
		// the maximum rect to the monitor work area so the taskbar stays
		// visible, like a normal maximized window.
		if mon, work, ok := monitorRects(hwnd); ok {
			mmi.MaxPosition.X = work.Left - mon.Left
			mmi.MaxPosition.Y = work.Top - mon.Top
			mmi.MaxSize.X = work.Right - work.Left
			mmi.MaxSize.Y = work.Bottom - work.Top
		}
		return 0
	}
	if originalWndProc != 0 {
		r, _, _ := procCallWindowProcW.Call(originalWndProc, hwnd, msg, wparam, lparam)
		return r
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

// pointerAt reinterprets an address that Windows passed through a callback.
// The buffer belongs to the OS and is valid for the duration of the message;
// unsafe.Add avoids the uintptr-to-pointer conversion the vet unsafeptr check
// rejects.
func pointerAt(addr uintptr) unsafe.Pointer {
	return unsafe.Add(nil, addr)
}

func hitTestScale(hwnd uintptr) float64 {
	if s := float64(chromeGeometrySnapshot().scale); s > 0 {
		return s
	}
	return dpiScale(hwnd)
}

func trimClientToCanvas(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if zoomed, _, _ := procIsZoomed.Call(hwnd); zoomed != 0 {
		return
	}
	geo := chromeGeometrySnapshot()
	scale := hitTestScale(hwnd)
	if geo.canvasW <= 0 || geo.canvasH <= 0 || scale <= 0 {
		return
	}
	var cr winRect
	if ret, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr))); ret == 0 {
		return
	}
	clientW := cr.Right - cr.Left
	clientH := cr.Bottom - cr.Top
	wantW := int32(math.Round(float64(geo.canvasW) * scale))
	wantH := int32(math.Round(float64(geo.canvasH) * scale))
	dw := clientW - wantW
	dh := clientH - wantH
	if absInt32(dw) <= 2 && absInt32(dh) <= 2 {
		return
	}
	// Only correct a frame-sized overhang. A larger difference means the
	// canvas has not caught up with a user resize yet; let it settle.
	if dw < 0 || dh < 0 || dw > 160 || dh > 160 {
		return
	}
	skipFrameAdjust.Store(true)
	procSetWindowPos.Call(hwnd, 0, 0, 0,
		uintptr(uint32(wantW)), uintptr(uint32(wantH)),
		swpNoMove|swpNoZOrder|swpNoActivate)
	skipFrameAdjust.Store(false)
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// windowFrameSize returns the width/height the hidden non-client frame adds
// to a window size at the window's DPI.
func windowFrameSize(hwnd uintptr) (int32, int32) {
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
	exStyle, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	// GLFW computed its window frame while the caption style still existed.
	// We clear WS_CAPTION afterwards, so measure the frame the way GLFW still
	// does; otherwise every Fyne resize request adds the old caption height.
	style |= wsCaption
	const base = int32(200)
	r := winRect{Right: base, Bottom: base}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	ok, _, _ := procAdjustWindowRectDPI.Call(
		uintptr(unsafe.Pointer(&r)), style, 0, exStyle, dpi)
	if ok == 0 {
		r = winRect{Right: base, Bottom: base}
		procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), style, 0, exStyle)
	}
	fw := (r.Right - r.Left) - base
	fh := (r.Bottom - r.Top) - base
	if fw < 0 {
		fw = 0
	}
	if fh < 0 {
		fh = 0
	}
	return fw, fh
}

func dpiScale(hwnd uintptr) float64 {
	if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
		return float64(dpi) / 96.0
	}
	return 1
}

func monitorRects(hwnd uintptr) (winRect, winRect, bool) {
	m, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if m == 0 {
		return winRect{}, winRect{}, false
	}
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ret, _, _ := procGetMonitorInfoW.Call(m, uintptr(unsafe.Pointer(&mi))); ret == 0 {
		return winRect{}, winRect{}, false
	}
	return mi.Monitor, mi.Work, true
}

func monitorWorkArea(hwnd uintptr) (winRect, bool) {
	_, work, ok := monitorRects(hwnd)
	return work, ok
}

// clientOrigin returns the screen position of the client area's top-left corner
// and its size. The client origin is not the window origin once the window is
// maximized, because the invisible frame sits outside the work area.
func clientOrigin(hwnd uintptr) (int32, int32, int32, int32, bool) {
	var r winRect
	if ret, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 0, 0, 0, 0, false
	}
	p := winPoint{}
	if ret, _, _ := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p))); ret == 0 {
		return 0, 0, 0, 0, false
	}
	return p.X, p.Y, r.Right - r.Left, r.Bottom - r.Top, true
}

// chromeHitTest classifies the window surface from WM_NCHITTEST's screen
// point:
//   - a 7px border -> native resize
//   - the top bar -> HTCAPTION (move, double-click maximize, Aero Snap)
//   - the right-hand window buttons -> HTCLOSE / HTMAXBUTTON / HTMINBUTTON
//     (HTMAXBUTTON is what enables the Windows 11 Snap Layouts flyout)
//   - the sidebar toggle -> HTCLIENT so Fyne receives the click
//   - the rest -> HTCLIENT so Fyne receives input.
func chromeHitTest(hwnd, lparam uintptr) (int, bool) {
	// WM_NCHITTEST carries the screen point in lParam (x in the low word, y in
	// the high word); use it rather than sampling the cursor again.
	screenX := int32(int16(lparam & 0xFFFF))
	screenY := int32(int16((lparam >> 16) & 0xFFFF))
	return hitTestAt(hwnd, screenX, screenY)
}

func hitTestAt(hwnd uintptr, screenX, screenY int32) (int, bool) {
	originX, originY, w, h, ok := clientOrigin(hwnd)
	if !ok {
		return 0, false
	}
	x := screenX - originX
	y := screenY - originY
	if x < 0 || y < 0 || x >= w || y >= h {
		return htClient, true
	}

	geo := chromeGeometrySnapshot()
	scale := hitTestScale(hwnd)
	border := int32(7 * scale)
	maximized, _, _ := procIsZoomed.Call(hwnd)
	if maximized == 0 {
		left, right, top, bottom := x < border, x >= w-border, y < border, y >= h-border
		switch {
		case left && top:
			return htTopLeft, true
		case right && top:
			return htTopRight, true
		case left && bottom:
			return htBottomLeft, true
		case right && bottom:
			return htBottomRight, true
		case left:
			return htLeft, true
		case right:
			return htRight, true
		case top:
			return htTop, true
		case bottom:
			return htBottom, true
		}
	}

	// The GL viewport can sit one frame-height below the client top, so map
	// Fyne's canvas coordinates to the window using the unused vertical space.
	offsetY := int32(float64(h) - float64(geo.canvasH)*scale)
	if offsetY < 0 {
		offsetY = 0
	}

	if y < offsetY+int32(44*scale) {
		switch {
		case inChromeControl(geo.close, x, y, scale, offsetY):
			return htClose, true
		case inChromeControl(geo.max, x, y, scale, offsetY):
			return htMaxButton, true
		case inChromeControl(geo.min, x, y, scale, offsetY):
			return htMinButton, true
		}
		// Everything to the right of the drag handle belongs to Fyne: the
		// sidebar toggle, New agent, help and the gaps between buttons. The
		// welcome strip keeps its theme switch at the top-left.
		actions := int32(200 * scale)
		if sidebarRegion.Load() {
			actions = int32(420 * scale)
		}
		if cw := chromeActionsWidth.Load(); cw > 0 {
			actions = int32(float64(cw) * scale)
		}
		if x >= w-actions {
			return htClient, true
		}
		if !sidebarRegion.Load() && x < int32(220*scale) {
			return htClient, true
		}
		return htCaption, true
	}
	return htClient, true
}

func inChromeControl(r chromeRect, x, y int32, scale float64, offsetY int32) bool {
	if r.W <= 0 || r.H <= 0 {
		return false
	}
	x0 := int32(float64(r.X) * scale)
	x1 := int32(float64(r.X+r.W) * scale)
	y0 := int32(float64(r.Y)*scale) + offsetY
	y1 := int32(float64(r.Y+r.H)*scale) + offsetY
	return x >= x0 && x < x1 && y >= y0 && y < y1
}

func controlFromHit(code int) chromeControl {
	switch code {
	case htClose:
		return controlClose
	case htMaxButton:
		return controlMaximize
	case htMinButton:
		return controlMinimize
	}
	return controlNone
}

func trackNCMouseLeave(hwnd uintptr) {
	tme := trackMouseEvent{
		Size:      uint32(unsafe.Sizeof(trackMouseEvent{})),
		Flags:     tmeLeave | tmeNonClient,
		HwndTrack: hwnd,
	}
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
}

// setSidebarHitRegion tells the hit test whether the main screen's sidebar
// toggle occupies the leftmost control slot. On the login strip that area is
// empty caption space and should drag the window.
func setSidebarHitRegion(v bool) {
	sidebarRegion.Store(v)
	if !v {
		clearChromeSidebar()
	}
}

// onChromeHoverChanged registers the hover bridge for the natively hit-tested
// buttons. Only the latest registration is kept because each screen builds a
// fresh set of controls.
func onChromeHoverChanged(fn func(chromeControl)) {
	chromeMu.Lock()
	chromeHoverObserver = fn
	lastChromeControl = controlNone
	chromeMu.Unlock()
}

func notifyChromeControl(c chromeControl) {
	chromeMu.Lock()
	if c == lastChromeControl {
		chromeMu.Unlock()
		return
	}
	lastChromeControl = c
	fn := chromeHoverObserver
	chromeMu.Unlock()
	if fn != nil {
		onUI(func() { fn(c) })
	}
}

// onWindowZoomChanged registers the callback used to keep the drawn maximize
// glyph in sync with native maximize/restore. Only the latest registration is
// kept because each screen builds a fresh set of window controls.
func onWindowZoomChanged(fn func(bool)) {
	zoomMu.Lock()
	zoomObserver = fn
	zoomMu.Unlock()
}

func windowMinimize(title string) {
	if h, ok := findWindow(title); ok {
		procShowWindow.Call(uintptr(h), swMinimize)
	}
}

func windowToggleMaximize(title string) {
	h, ok := findWindow(title)
	if !ok {
		return
	}
	if maximized, _, _ := procIsZoomed.Call(uintptr(h)); maximized != 0 {
		procShowWindow.Call(uintptr(h), swRestore)
		return
	}
	procShowWindow.Call(uintptr(h), swMaximize)
}

func windowIsMaximized(title string) bool {
	h, ok := findWindow(title)
	if !ok {
		return false
	}
	v, _, _ := procIsZoomed.Call(uintptr(h))
	return v != 0
}

// windowDragOffsets returns the cursor's position relative to the window's
// top-left. The native HTCAPTION hit test normally handles dragging now; these
// helpers remain for the resize grip and as a fallback.
func windowDragOffsets(title string) (int32, int32, bool) {
	h, ok := findWindow(title)
	if !ok {
		return 0, 0, false
	}
	var r winRect
	if ret, _, _ := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 0, 0, false
	}
	var p winPoint
	if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); ret == 0 {
		return 0, 0, false
	}
	return p.X - r.Left, p.Y - r.Top, true
}

func windowCursor() (int32, int32, bool) {
	var p winPoint
	if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); ret == 0 {
		return 0, 0, false
	}
	return p.X, p.Y, true
}

func windowMove(title string, x, y int32) {
	h, ok := findWindow(title)
	if !ok {
		return
	}
	procSetWindowPos.Call(uintptr(h), 0,
		uintptr(uint32(x)), uintptr(uint32(y)), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate)
}

func windowResizeToCursor(title string) {
	h, ok := findWindow(title)
	if !ok {
		return
	}
	var r winRect
	if ret, _, _ := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r))); ret == 0 {
		return
	}
	x, y, ok := windowCursor()
	if !ok {
		return
	}
	w := x - r.Left
	hh := y - r.Top
	if w < 900 {
		w = 900
	}
	if hh < 600 {
		hh = 600
	}
	procSetWindowPos.Call(uintptr(h), 0, 0, 0,
		uintptr(uint32(w)), uintptr(uint32(hh)),
		swpNoMove|swpNoZOrder|swpNoActivate)
}
