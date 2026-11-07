package main

import "embed"

// studioDocsFS is the documentation set shipped with Studio. Keeping the
// embed in the root package lets the content stay in docs/ as ordinary files.
//
//go:embed docs
var studioDocsFS embed.FS
