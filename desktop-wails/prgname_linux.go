//go:build linux

package main

// WebKitGTK keeps the webview's storage (localStorage, so the saved
// connections, language and theme) in $XDG_DATA_HOME/<program name>. GLib
// takes the program name from the executable unless it is set first, and
// Wails' ProgramName option is applied only after the webview exists, so a
// renamed binary would start with empty storage. Setting it here, before
// main runs and before GTK starts, pins the folder to one name.

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
static void set_prgname(void) { g_set_prgname("agentflow-studio"); }
*/
import "C"

func init() { C.set_prgname() }
