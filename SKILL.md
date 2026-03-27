# Kestrel Portfolio CLI — Agent Skill

## Overview

Command-line interface for the Kestrel Portfolio API. Manages commercial real estate properties and leases for portfolio management organizations.

**Install:** Download the binary from GitHub Releases or `brew install kestrel`.

**Discover commands:** `kestrel commands --json`

## Rules

- Always use `--json` when parsing output programmatically.
- When output is piped (non-TTY), `--json` is automatic — but always pass it explicitly to be safe.
- Verify authentication with `kestrel me --json` before running other commands.
- IDs are organization-scoped sequential integers (not UUIDs). They are unique within an organization.
- Paginated endpoints return 50 items per page. Check `meta.next_page` — if present, more pages exist.
- All errors go to stderr. A non-zero exit code means the command failed.

## Authentication

```bash
# Interactive — prompts for token
kestrel login

# Non-interactive — pass token directly
kestrel login --token <API_TOKEN>

# Override API URL (for staging/development)
kestrel login --token <API_TOKEN> --base-url https://staging.example.com/api/v1
```

Token is saved to `~/.config/kestrel/config.json`. You can also set `KESTREL_TOKEN` and `KESTREL_BASE_URL` environment variables.

## Commands

### Core

| Command | Description |
|---------|-------------|
| `kestrel me --json` | Current user + organization info |
| `kestrel commands --json` | List all commands (this catalog) |
| `kestrel version` | CLI version |

### Properties

| Command | Description |
|---------|-------------|
| `kestrel properties list --json` | List all properties (paginated) |
| `kestrel properties list --page 2 --json` | Get page 2 |
| `kestrel properties show <id> --json` | Single property by ID |

### Leases

| Command | Description |
|---------|-------------|
| `kestrel leases list --json` | List all leases (paginated) |
| `kestrel leases list --page 2 --json` | Get page 2 |
| `kestrel leases show <id> --json` | Single lease by ID |

### Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Force JSON output (automatic when piped) |
| `--quiet` | Minimal output |
| `--base-url URL` | Override API base URL |

## Output Format

All JSON responses use the API's envelope format:

```json
{
  "ok": true,
  "data": { ... },
  "meta": {
    "page": 1,
    "next_page": 2,
    "count": 150
  }
}
```

- **Single record:** `data` is an object.
- **Collection:** `data` is an array.
- **Pagination:** `meta.next_page` is `null` when on the last page.

### Error responses

Errors print to stderr and the command exits with code 1. With `--json`, the error message is in the stderr output.

Common error codes from the API:
- `unauthorized` — missing or invalid token
- `token_expired` — token has expired, re-run `kestrel login`
- `api_disabled` — API access is not enabled for this organization
- `not_found` — resource does not exist
- `forbidden` — user does not have permission

## Common Workflows

### Verify authentication
```bash
kestrel me --json
```

### Browse the portfolio
```bash
# List all properties
kestrel properties list --json

# Get details for a specific property
kestrel properties show 1 --json

# List all leases
kestrel leases list --json

# Get details for a specific lease
kestrel leases show 1 --json
```

### Paginate through results
```bash
# Page 1 (default)
kestrel properties list --json
# Check meta.next_page in response, then:
kestrel properties list --page 2 --json
```
