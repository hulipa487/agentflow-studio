//go:build !windows

package ui

// Other platforms keep their native window frame. The desktop driver's
// undecorated-window control and the Win32 helpers used by the custom title
// bar are Windows-specific; returning false here means no fake title bar is
// added where it could not drag the window anyway.
func borderlessSupported() bool                     { return false }
func MakeBorderless(string)                         {}
func windowMinimize(string)                         {}
func windowToggleMaximize(string)                   {}
func windowIsMaximized(string) bool                 { return false }
func windowDragOffsets(string) (int32, int32, bool) { return 0, 0, false }
func windowCursor() (int32, int32, bool)            { return 0, 0, false }
func windowMove(string, int32, int32)               {}
func windowResizeToCursor(string)                   {}
func onWindowZoomChanged(func(bool))                {}
func setSidebarHitRegion(bool)                      {}
func onChromeHoverChanged(func(chromeControl))      {}
