# Kestrel Portfolio CLI — Claude Context

## Stack
- **Go** (latest stable) with **Cobra** CLI framework
- **API target:** Kestrel Portfolio Rails app (`/api/v1/`), Bearer token auth
- **Distribution:** Single binary — Homebrew, GitHub Releases, direct download

## What This Is
A CLI tool for interacting with the Kestrel Portfolio API. Designed for two audiences:
1. **Human users** — property managers, asset managers at client organizations
2. **AI agents** — Claude Code, Cursor, ChatGPT (via MCP), and other AI tooling

The ultimate goal is lease abstraction: extracting structured data from lease documents and pushing it into Kestrel Portfolio via API.

## Architecture Patterns
Following patterns from the Basecamp CLI (github.com/basecamp/basecamp-cli):
- **Response envelope:** `{ok, data, summary, breadcrumbs, meta}` — breadcrumbs suggest next commands
- **Dual output mode:** Styled ANSI for TTY, JSON when piped. Flags: `--json`, `--quiet`
- **Config layering:** Flags > env vars > local config > global config > defaults
- **Agent integration:** SKILL.md, `--help --json`, `kestrel commands --json`

## API Reference
- OpenAPI spec: served at `/api/v1/openapi` on the Rails app
- API docs: `../kestrel_portfolio/docs/api.md`
- OpenAPI YAML: `../kestrel_portfolio/docs/openapi.yaml`

## Developer Notes
- The developer's primary experience is **Ruby and JavaScript** — explain Go concepts in terms of Ruby/JS analogues when commenting or discussing code
- Always present a plan before major code writing
- Prefer simple, readable code over clever Go idioms until the developer is comfortable with the language
- **Do not commit** — the developer handles all git commits themselves

## Conventions
- Package names: short, lowercase, no underscores (Go convention)
- Error handling: always check returned errors, wrap with context using `fmt.Errorf("doing X: %w", err)`
- CLI command files: one file per command or resource group
- Test files: `*_test.go` next to the code they test (Go convention)
