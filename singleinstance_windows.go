//go:build windows

package main

import (
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const studioWindowTitle = "AgentFlow Studio"

var instanceMutex windows.Handle

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
)

// acquireSingleInstance prevents a second Studio window from stacking on top of
// the first. Two windows each draw their own min/max/close cluster, which reads
// as duplicated title-bar controls. If an instance already exists, this brings
// it forward and returns false.
func acquireSingleInstance() bool {
	name, err := windows.UTF16PtrFromString(`Local\AgentFlowStudio.SingleInstance`)
	if err != nil {
		return true
	}
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		focusExistingStudio()
		return false
	}
	if err != nil {
		// Never block startup because the guard itself failed.
		return true
	}
	instanceMutex = h
	return true
}

func releaseSingleInstance() {
	if instanceMutex != 0 {
		_ = windows.CloseHandle(instanceMutex)
		instanceMutex = 0
	}
}

func focusExistingStudio() {
	title, err := windows.UTF16PtrFromString(studioWindowTitle)
	if err != nil {
		return
	}
	// The other process may still be creating its window; retry briefly.
	for i := 0; i < 20; i++ {
		hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
		if hwnd != 0 {
			procShowWindow.Call(hwnd, 9) // SW_RESTORE
			procSetForegroundWin.Call(hwnd)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
