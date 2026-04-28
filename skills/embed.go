// Package skills exposes the SKILL.md file embedded into the binary at build
// time. The Go equivalent of Sprockets/Webpacker bundling a static asset:
// callers read it as a regular fs.FS without touching the filesystem.
package skills

import "embed"

// FS holds the canonical SKILL.md. Read with skills.FS.ReadFile("kestrel/SKILL.md").
//
//go:embed kestrel/SKILL.md
var FS embed.FS
