//go:build darwin

package main

// Wails v2's macOS dialog bindings reference UTType, which lives in
// UniformTypeIdentifiers.framework. Newer Xcode SDKs no longer link it
// transitively through Cocoa/Foundation/WebKit, so link it explicitly.

/*
#cgo LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"
