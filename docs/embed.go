// Package docs is the AgentFlow documentation set as an embedded file system.
//
// The Markdown guides, the users OpenAPI contract and the Luau type stubs in
// this directory are ordinary files, edited as files. Both Studio apps ship
// them: the desktop app embeds them through this package (it is a module of
// its own because a Go embed cannot reach outside its module), and the web app
// bundles the same files at build time. There is one copy, here.
package docs

import "embed"

// FS holds the guides at its root, their Simplified and Traditional Chinese
// translations under zh-Hans/ and zh-Hant/ (same file names), and the
// contract files under openapi/ and luau/.
//
//go:embed *.md luau openapi zh-Hans zh-Hant
var FS embed.FS
