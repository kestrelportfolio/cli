---
name: kestrel
description: |
  Interact with Kestrel Portfolio — a commercial real estate portfolio management
  system — via the kestrel CLI. Browse properties, leases, expenses, documents,
  and key dates; author lease abstractions end-to-end (the only write surface).
triggers:
  - kestrel
  - /kestrel
  - kestrel properties
  - kestrel leases
  - kestrel expenses
  - kestrel documents
  - kestrel abstractions
  - kestrel templates
  - lease abstraction
  - abstract this lease
  - abstract a lease
  - draft an abstraction
  - upload lease
  - attach lease pdf
  - list my properties
  - list my leases
  - show lease
  - lease expiration
  - key dates
  - source documents
  - kestrelportfolio.com
invocable: true
argument-hint: "[resource] [action] [args...]"
---

# /kestrel — Kestrel Portfolio Workflow Command

The CLI talks to the Kestrel Portfolio API at `/api/v1/`. Read surface is broad
(properties, leases, expenses, documents, key dates, component areas,
securities, lease clauses). **Write surface is abstractions-only** — any data
mutation must go through an abstraction workflow that a human approves via the
web UI before it goes live.

## Agent Invariants

**MUST follow these rules:**

1. **All writes go through abstractions.** There is no direct `POST /leases` or
   `PATCH /expenses`. To change any domain record, create an abstraction
   (greenfield or brownfield), add source documents, draft per-field changes.
   Approval and go-live are web-only — the CLI cannot apply changes.
2. **Cite a source document on every API-authored change.** The API enforces
   this — changes drafted without at least one `source_links` entry are
   rejected. Upload a PDF via `kestrel abstractions add-doc` first, then
   reference its `document_id` in `--source-links`.
3. **Use `--agent` for scripting**, `--json` when you need the envelope
   (breadcrumbs, meta), `--quiet`/no flag for human display.
4. **Check auth before anything else.** Exit code 3 means "no token or
   expired". Run `kestrel login` to fix.
5. **Dedup is automatic.** Re-POSTing `changes create` with the same
   `(action, target_type, target_field, sub_object_group)` tuple updates the
   existing pending change and returns 200 (not 201). Use this for idempotent
   retries; don't check for existence first.
6. **Sub-object fields group via UUID.** When creating a new KeyDate, Expense,
   or other sub-object, every field change for that instance shares a
   `sub_object_group` UUID. Mint one with `--sub-object-group new` on the
   first change; reuse the UUID in breadcrumbs for sibling fields.
7. **Channel-lock: API-authored changes can only be edited via the API.**
   Changes drafted in the web UI (source: "web_ui") are read-only from the
   CLI. 403 on update/delete means this.

## Output Modes

| Goal                          | Flag       | Format                                                                     |
|-------------------------------|------------|----------------------------------------------------------------------------|
| Script/agent data extraction  | `--agent`  | Success: raw `data` only (no envelope). Errors: `{ok:false, error, code}`. |
| Full JSON envelope            | `--json`   | `{ok, data, meta, summary, breadcrumbs}` — includes CLI-added hints.       |
| Human display                 | (default)  | Styled table/detail when TTY; JSON when piped. Breadcrumbs to stderr.      |
| Minimal output for scripts    | `--quiet`  | Same as human mode but suppresses success lines and hints.                 |

**Prefer `--agent` for most agent work** — you get clean `data` without having
to unwrap `.data` from an envelope. Use `--json` only when you need breadcrumbs
or pagination metadata.

## CLI Introspection

Every command supports `--help --agent` to return structured metadata as JSON:

```bash
kestrel abstractions --help --agent
```

Returns:

```json
{
  "command": "abstractions",
  "path": "kestrel abstractions",
  "short": "Author abstractions …",
  "long": "…",
  "subcommands": [{"name":"create","path":"kestrel abstractions create","short":"…"}, …],
  "flags": [{"name":"…","type":"…","default":"…","usage":"…"}]
}
```

Walk the tree: start at `kestrel --help --agent`, drill into any subcommand.
For the flat catalog (all commands at once) use `kestrel commands --json`.

## Quick Reference

| Task                                 | Command                                                                                                  |
|--------------------------------------|----------------------------------------------------------------------------------------------------------|
| Who am I?                            | `kestrel me --agent`                                                                                     |
| List properties                      | `kestrel properties list --agent`                                                                        |
| Show a property                      | `kestrel properties show <id> --agent`                                                                   |
| Leases on a property                 | `kestrel properties leases <prop-id> --agent`                                                            |
| Show a lease                         | `kestrel leases show <id> --agent`                                                                       |
| Lease expenses / docs / key dates    | `kestrel leases expenses \| documents \| key-dates <lease-id> --agent`                                   |
| Component areas, securities, clauses | `kestrel leases component-areas \| securities \| clauses <lease-id> --agent`                             |
| Expense with payments + increases    | `kestrel expenses show <id> --agent` / `kestrel expenses payments <id>` / `kestrel expenses increases <id>` |
| Document metadata                    | `kestrel documents show <id> --agent`                                                                    |
| Document file                        | `kestrel documents download <id> [-o file]`                                                              |
| Print signed URL only                | `kestrel documents download <id> --url`                                                                  |
| Org field config                     | `kestrel field-configs list --agent`                                                                     |
| List templates                       | `kestrel templates list --agent`                                                                         |
| Template authoring schema            | `kestrel templates schema <id> --agent`                                                                  |
| Create greenfield abstraction        | `kestrel abstractions create --template-id N --kind greenfield --agent`                                  |
| Create brownfield abstraction        | `kestrel abstractions create --template-id N --kind brownfield --target-property-id P --target-lease-id L --agent` |
| Show abstraction schema              | `kestrel abstractions schema <abs-id> --agent`                                                           |
| Add source PDF                       | `kestrel abstractions add-doc <abs-id> lease.pdf`                                                        |
| List attached sources                | `kestrel abstractions sources <abs-id> --agent`                                                          |
| Destroy a source doc                 | `kestrel abstractions remove-doc <abs-id> --document-id N`                                               |
| Draft a scalar update change         | `kestrel abstractions changes create <abs-id> --action update --target-type Lease --target-id L --payload '{"name":"…"}' --source-links '[{"document_id":D}]'` |
| Draft a sub-object create            | `kestrel abstractions changes create <abs-id> --action create --target-type KeyDate --sub-object-group new --payload '{"name":"…","date":"…"}' --source-links '[{"document_id":D}]'` |
| List staged changes                  | `kestrel abstractions changes list <abs-id> --agent`                                                     |
| Update a change                      | `kestrel abstractions changes update <abs-id> <change-id> --payload @new.json`                           |
| Delete a change                      | `kestrel abstractions changes delete <abs-id> <change-id>`                                               |
| Abandon an abstraction               | `kestrel abstractions abandon <abs-id>`                                                                  |

## Decision Trees

### Browsing existing data

```
Need to find something?
├── Know the property/lease/expense ID?
│   └── kestrel <resource> show <id> --agent
├── Listing by parent?
│   └── kestrel properties <leases|expenses|documents|key-dates> <prop-id> --agent
│   └── kestrel leases <expenses|documents|key-dates|component-areas|securities|clauses> <lease-id> --agent
├── Deep in dates dependency graph?
│   └── kestrel properties date-entries <prop-id> --agent
├── Looking for a payment or increase chain?
│   └── kestrel expenses payments <expense-id> --agent
│   └── kestrel expenses increases <expense-id> --agent
└── Need a document?
    └── kestrel documents show <id> --agent    (metadata)
    └── kestrel documents download <id> -o /tmp/  (binary)
```

### Authoring an abstraction

```
Want to change lease data?
├── 1. Discover the template
│   └── kestrel templates list --agent
│   └── kestrel templates schema <template-id> --agent   (what fields will be required)
├── 2. Start the abstraction
│   └── Greenfield (new property+lease):
│       kestrel abstractions create --template-id N --kind greenfield --agent
│   └── Brownfield (edit existing lease):
│       kestrel abstractions create --template-id N --kind brownfield \
│         --target-property-id P --target-lease-id L --agent
├── 3. Attach source PDFs
│   └── kestrel abstractions add-doc <abs-id> lease.pdf
│   (Repeat for every source document.)
├── 4. Discover what fields are expected
│   └── kestrel abstractions schema <abs-id> --agent
├── 5. Draft changes (MUST cite source_links)
│   └── Scalar update: --action update --target-type Lease --target-id L --payload '{…}' --source-links '[{"document_id":D}]'
│   └── Sub-object create: --action create --target-type KeyDate --sub-object-group new --payload '{…}' --source-links '[{"document_id":D}]'
└── 6. (Human completes in web UI: review, approve, go-live)
```

## Common Workflows

### Abstract a lease from a PDF end-to-end

```bash
# 1. Pick a template
kestrel templates list --agent
# → [{id: 3, name: "Standard Lease", kind: "greenfield", ...}]

# 2. Preview what fields it expects
kestrel templates schema 3 --agent

# 3. Start the abstraction (greenfield creates new Property + Lease on go-live)
ABS=$(kestrel abstractions create --template-id 3 --kind greenfield --agent | jq -r '.id')

# 4. Upload the lease PDF and attach it as a source
DOC=$(kestrel abstractions add-doc "$ABS" lease.pdf --agent | jq -r '.id')

# 5. Check what fields the abstraction still needs
kestrel abstractions schema "$ABS" --agent

# 6. Draft changes — each one cites the source document
kestrel abstractions changes create "$ABS" \
  --action create --target-type Property \
  --payload '{"name":"123 Main St","city":"Austin","country":"US"}' \
  --source-links "[{\"document_id\":$DOC}]" \
  --agent

kestrel abstractions changes create "$ABS" \
  --action create --target-type Lease \
  --payload '{"name":"Ground Floor","start_date":"2026-01-01","end_date":"2030-12-31"}' \
  --source-links "[{\"document_id\":$DOC}]" \
  --agent

# 7. Add sub-object: a KeyDate for lease expiration — mint a group UUID
#    The minted UUID is echoed in breadcrumbs; re-use it for sibling fields.
kestrel abstractions changes create "$ABS" \
  --action create --target-type KeyDate --sub-object-group new \
  --payload '{"name":"Expiration","date":"2030-12-31","notice_deadline":true}' \
  --source-links "[{\"document_id\":$DOC}]" \
  --json
# → breadcrumbs: ["Sub-object group UUID: …", "Add sibling fields: --sub-object-group …"]

# 8. Human reviews + approves + go-lives in the web UI. The CLI cannot do this.
```

### Revise a rejected change

```bash
# When the web reviewer rejects a change, the rejection reason is attached.
kestrel abstractions changes list 42 --agent | jq '.[] | select(.state == "rejected")'

# Resubmit with corrected payload — revised_from is auto-linked to the
# rejected predecessor, preserving the review chain.
kestrel abstractions changes create 42 \
  --action update --target-type Lease --target-id 7 \
  --payload '{"name":"Corrected Name"}' \
  --source-links '[{"document_id":87}]' \
  --agent
```

### Attach PDF highlights as provenance

Source links can include PDF fragment highlights — rectangles on specific
pages that back the claim in `payload`. Coordinates are normalized 0–1
relative to the page's width and height.

```bash
# Cite page 3 of document 87, highlighting a rectangle at (0.1, 0.2) spanning
# 30% of the page width and 5% of the page height.
kestrel abstractions changes create 42 \
  --action update --target-type Lease --target-id 7 \
  --payload '{"start_date":"2026-01-01"}' \
  --source-links '[{
    "document_id": 87,
    "fragments": [
      {"page_number": 3, "x": 0.1, "y": 0.2, "width": 0.3, "height": 0.05, "label": "Commencement Date"}
    ]
  }]' \
  --agent
```

### Remove a source document

```bash
# Destroys the Document entirely. Cascade: pending/rejected citing changes
# are destroyed; the join to this abstraction is cleared automatically.
# API-uploaded docs only, uploader only.
kestrel abstractions remove-doc 42 --document-id 87 --agent

# If the doc is cited by an approved/applied change, the API returns 422
# cited_by_locked_change. Reject the change in the web UI first.
```

## Resource Reference

### Read-only

| Resource            | Show                                           | Nested collections                                                              |
|---------------------|------------------------------------------------|---------------------------------------------------------------------------------|
| `me`                | `kestrel me`                                   | —                                                                               |
| `field-configs`     | `kestrel field-configs list`                   | —                                                                               |
| `properties`        | `kestrel properties show <id>`                 | `leases`, `expenses`, `documents`, `key-dates`, `date-entries`                  |
| `leases`            | `kestrel leases show <id>`                     | `expenses`, `documents`, `key-dates`, `component-areas`, `securities`, `clauses`|
| `expenses`          | `kestrel expenses show <id>`                   | `payments`, `increases`                                                         |
| `lease-securities`  | `kestrel lease-securities show <id>`           | `increases`                                                                     |
| `documents`         | `kestrel documents show <id>`                  | `download` (binary)                                                             |

### Write (abstraction-scoped)

| Operation                         | Command                                                                   |
|-----------------------------------|---------------------------------------------------------------------------|
| Discover templates                | `kestrel templates list \| show \| schema`                                 |
| Abstraction CRUD                  | `kestrel abstractions create \| show \| update \| abandon`                 |
| Authoring schema                  | `kestrel abstractions schema <abs-id>`                                    |
| Source docs (upload + attach)     | `kestrel abstractions add-doc <abs-id> <file>`                            |
| Source docs (destroy)             | `kestrel abstractions remove-doc <abs-id> --document-id N`                |
| Source docs (list)                | `kestrel abstractions sources <abs-id>`                                   |
| Per-field changes                 | `kestrel abstractions changes create \| list \| show \| update \| delete` |

## Error Handling

Structured errors in `--json` / `--agent` mode:

```json
{"ok": false, "error": "<template-id> required", "code": "usage", "hint": "kestrel abstractions create --template-id N --kind greenfield|brownfield"}
```

Key error codes:

| Code                      | Meaning                                                                          | Fix                                                               |
|---------------------------|----------------------------------------------------------------------------------|-------------------------------------------------------------------|
| `usage`                   | Missing required flag/arg                                                        | Read `hint` and `error` — `error` names the missing `<arg>`       |
| `validation`              | Server rejected payload (422)                                                    | Inspect `errors` array                                            |
| `unauthorized`            | No token or 401 from server                                                      | `kestrel login`                                                   |
| `token_expired`           | Token past its expiry                                                            | `kestrel login` to regenerate                                     |
| `api_disabled`            | Org has `api_access_enabled` off                                                 | Staff must enable in org settings                                 |
| `forbidden`               | RBAC denied, or channel-lock (web_ui-sourced change)                             | Check role permissions, or edit via web UI                        |
| `not_found`               | Record missing or not visible to your RBAC scope                                 | Verify ID and your permissions                                    |
| `rate_limited`            | 429 — exceeded per-token burst or per-minute caps                                | Wait + retry. Reads: 200/30s burst, 2000/min. Writes: 500/min.   |
| `cited_by_locked_change`  | Attempting to `remove-doc` a doc cited by an approved or applied change          | Reject the change in the web UI first                             |

## Exit Codes

| Exit | Meaning       | Typical cause                                                        |
|------|---------------|----------------------------------------------------------------------|
| 0    | OK            | —                                                                    |
| 1    | Usage         | Missing flag/arg (see `code: "usage"`), or 422 validation failure    |
| 2    | Not found     | Bad ID or not visible under RBAC                                     |
| 3    | Auth          | No token, 401, or token_expired                                      |
| 4    | Forbidden     | 403 — RBAC or channel-lock                                           |
| 5    | Rate limited  | 429 — back off                                                       |
| 6    | Network       | Connection refused, DNS, TLS                                         |
| 7    | API           | 5xx from server                                                      |

## Configuration

```
~/.config/kestrel/config.json    # token, base_url; created by `kestrel login`
```

Environment overrides:

- `KESTREL_TOKEN` — bearer token
- `KESTREL_BASE_URL` — `https://kestrelportfolio.com/api/v1` by default

Flag `--base-url` overrides both for a single command.

Check current identity and environment:

```bash
kestrel me --agent                    # who am I, what org?
kestrel doctor --json                 # install + auth + connectivity health
```

## Learn More

- API spec: `../kestrel_portfolio/docs/openapi.yaml` (OpenAPI 3.1)
- API narrative: `../kestrel_portfolio/docs/api.md`
- Abstraction concepts: `../kestrel_portfolio/docs/abstractions.md`
- CLI repo: https://github.com/kestrelportfolio/kestrel-cli
