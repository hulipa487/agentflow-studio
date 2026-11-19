// Package i18n is the AgentFlow Studio message catalogues as an embedded file
// system.
//
// There is one directory per language (en, zh-Hans, zh-Hant) and one JSON file
// per namespace inside it; a key is "<namespace>.<name>" and lives in the file
// named after its namespace. Both Studio apps ship these files: the desktop app
// embeds them through this package (a module of its own, because a Go embed
// cannot reach outside its module), and the web app bundles the same files at
// build time. There is one copy, here, so the two apps cannot drift apart in
// wording. See README.md for the message syntax and the glossary.
package i18n

import "embed"

// FS holds one directory per language.
//
//go:embed en zh-Hans zh-Hant
var FS embed.FS
