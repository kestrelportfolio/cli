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
   cite it via `--cite-block` (parsed-block citations, preferred) or
   `--source-links` (raw JSON for coord-mode).
3. **Prefer block-ref citations over coord rectangles.** When the document has
   been parsed, cite by `document_block_id` — the server derives coords +
   `cited_text` from the parse. Narrow to a substring with `char_start`/`end`
   or to a specific table cell with `table_cell_row`/`col`. Use coord mode
   only when the parse hasn't rendered the thing you need to cite.
4. **Wait for parse before citing blocks.** Parses are triggered on attach,
   not upload. After `add-doc`, call `documents parse <doc-id> --wait` (or
   pass `--wait-parse` to `add-doc`) before you start reading blocks. Status
   `complete` means blocks are queryable; `failed` means fall back to coord
   mode or a different document.
5. **Use `--agent` for scripting**, `--json` when you need the envelope
   (breadcrumbs, meta), `--quiet`/no flag for human display.
6. **Check auth before anything else.** Exit code 3 means "no token or
   expired". Run `kestrel login` to fix.
7. **Dedup is automatic.** Re-POSTing `changes create` with the same
   `(action, target_type, target_field, sub_object_group)` tuple updates the
   existing pending change and returns 200 (not 201). Use this for idempotent
   retries; don't check for existence first.
8. **Sub-object fields group via UUID.** When creating a new KeyDate, Expense,
   or other sub-object, every field change for that instance shares a
   `sub_object_group` UUID. Mint one with `--sub-object-group new` on the
   first change; reuse the UUID in breadcrumbs for sibling fields.
9. **Channel-lock: API-authored changes can only be edited via the API.**
   Changes drafted in the web UI (source: "web_ui") are read-only from the
   CLI. 403 on update/delete means this.
10. **Prefer batch for multi-change drafts.** When staging more than a handful
    of changes for the same abstraction, use `changes create-batch` with a
    JSON file instead of looping `changes create`. One transaction, one
    all-or-nothing response, coalesced websocket broadcasts (the human
    watching the workspace sees one re-render per section instead of one per
    change). Max 500 items per batch.
11. **Use trigram search instead of paging.** `documents blocks --search "q"`
    hits a pg_trgm GIN index and matches across both block prose and table
    cell text. Queries under 4 chars are rejected client-side — trigrams
    need at least 3 chars to narrow, 4 is the ergonomic floor. Typos score
    partial matches, so spelling need not be exact.

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
| Parse status                         | `kestrel documents parse <id> --agent`                                                                   |
| Wait for parse                       | `kestrel documents parse <id> --wait [--timeout 300]`                                                    |
| Pages of parsed version              | `kestrel documents pages <id> --agent`                                                                   |
| Walk block graph                     | `kestrel documents blocks <id> [--page N] [--type heading] [--since-order N] --agent`                    |
| Search block text                    | `kestrel documents blocks <id> --search "commencement" --agent`    (trigram, 4+ chars, searches cells too) |
| Blocks near one block                | `kestrel documents blocks <id> --near <block-id> --window 3 --agent`                                     |
| Inspect single block                 | `kestrel documents block <block-id> --agent`                                                             |
| Org field config                     | `kestrel field-configs list --agent`                                                                     |
| List templates                       | `kestrel templates list --agent`                                                                         |
| Template authoring schema            | `kestrel templates schema <id> --agent`                                                                  |
| Create greenfield abstraction        | `kestrel abstractions create --template-id N --kind greenfield --agent`                                  |
| Create brownfield abstraction        | `kestrel abstractions create --template-id N --kind brownfield --target-property-id P --target-lease-id L --agent` |
| Show abstraction schema              | `kestrel abstractions schema <abs-id> --agent`                                                           |
| Add source PDF (and wait for parse)  | `kestrel abstractions add-doc <abs-id> lease.pdf --wait-parse`                                           |
| List attached sources                | `kestrel abstractions sources <abs-id> --agent`                                                          |
| Destroy a source doc                 | `kestrel abstractions remove-doc <abs-id> --document-id N`                                               |
| Draft a scalar update w/ block cite  | `kestrel abstractions changes create <abs-id> --action update --target-type Lease --target-id L --payload '{"name":"…"}' --cite-block <block-id>` |
| Cite a table cell                    | `kestrel abstractions changes create <abs-id> … --cite-block <block-id>:cell=2,1`                        |
| Cite a substring                     | `kestrel abstractions changes create <abs-id> … --cite-block <block-id>:chars=14-72`                     |
| Draft a sub-object create            | `kestrel abstractions changes create <abs-id> --action create --target-type KeyDate --sub-object-group new --payload '{"name":"…","date":"…"}' --cite-block <block-id>` |
| Coord-mode citation (fallback)       | `… --source-links '[{"document_id":D,"fragments":[{"page_number":3,"x":0.1,"y":0.2,"width":0.3,"height":0.05}]}]'` |
| List staged changes                  | `kestrel abstractions changes list <abs-id> --agent`                                                     |
| Filter by state                      | `kestrel abstractions changes list <abs-id> --state rejected --agent`  (repeatable)                      |
| Batch-create N changes atomically    | `kestrel abstractions changes create-batch <abs-id> --file @batch.json`  (max 500, all-or-nothing)       |
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
├── 3. Attach source PDFs and wait for the structured parse
│   └── kestrel abstractions add-doc <abs-id> lease.pdf --wait-parse
│   (Parses run on attach; --wait-parse blocks until complete.)
├── 4. Walk the block graph to find evidence
│   └── kestrel documents blocks <doc-id> --search "phrase" --agent   (trigram, searches cells too)
│   └── kestrel documents blocks <doc-id> --type heading --agent      (navigate by structure)
│   └── kestrel documents blocks <doc-id> --page 3 --agent            (page-scoped)
│   └── kestrel documents block <block-id> --agent                    (confirm text + bbox)
│   └── kestrel documents blocks <doc-id> --near <id> --window 3 --agent  (context)
├── 5. Discover what fields are expected
│   └── kestrel abstractions schema <abs-id> --agent
├── 6. Draft changes — cite blocks
│   ├── For many changes at once (typical agent pass):
│   │   └── kestrel abstractions changes create-batch <abs-id> --file @batch.json
│   │       (all-or-nothing, ≤500 items, coalesced broadcasts)
│   └── For single changes (exploratory):
│       └── Scalar update: --action update --target-type Lease --target-id L --payload '{…}' --cite-block <block-id>
│       └── Substring: --cite-block <block-id>:chars=14-72
│       └── Table cell: --cite-block <block-id>:cell=2,1
│       └── Sub-object create: --action create --target-type KeyDate --sub-object-group new --payload '{…}' --cite-block <block-id>
└── 7. (Human completes in web UI: review, approve, go-live)
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

# 4. Upload the lease PDF, attach it, and wait for the structured parse.
#    Parses run on attach; --wait-parse blocks until the block graph is ready.
#    The parse envelope comes back as the result — pull document_id from it.
DOC=$(kestrel abstractions add-doc "$ABS" lease.pdf --wait-parse --agent | jq -r '.document_id')

# 5. Find evidence fast — trigram search beats paging by a mile.
kestrel documents blocks "$DOC" --search "commencement" --agent
# → [{id: 4240, text: "…commencement date shall be January 1, 2026…", reading_order: 42}]

kestrel documents blocks "$DOC" --search "base rent" --type table --agent
# → table block whose cells match; inspect metadata.cells[] to pick the cell

# 6. Or walk by structure if search is too loose.
kestrel documents blocks "$DOC" --type heading --agent
# → [{id: 4201, text: "1. Premises", page_number: 1, reading_order: 8}, …]

kestrel documents blocks "$DOC" --near 4201 --window 3 --agent
# → blocks surrounding the "Premises" heading

# 7. Confirm the exact block before citing (optional but cheap).
kestrel documents block 4215 --agent

# 8. Batch-draft the whole abstraction in one atomic call.
#    One transaction, coalesced broadcasts, per-item rollback on failure.
#    Max 500 items per batch.
cat > /tmp/batch.json <<EOF
[
  {
    "action": "create", "target_type": "Property",
    "payload": {"name":"123 Main St","city":"Austin","country":"US"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4215}]}]
  },
  {
    "action": "create", "target_type": "Lease",
    "payload": {"name":"Ground Floor","start_date":"2026-01-01","end_date":"2030-12-31"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4240,"char_start":14,"char_end":72}]}]
  },
  {
    "action": "create", "target_type": "Expense", "sub_object_group": "$(uuidgen)",
    "payload": {"name":"Base Rent","amount":5000,"currency":"USD"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4301,"table_cell_row":2,"table_cell_col":1}]}]
  },
  {
    "action": "create", "target_type": "KeyDate", "sub_object_group": "$(uuidgen)",
    "payload": {"name":"Expiration","date":"2030-12-31","notice_deadline":true},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4280}]}]
  }
]
EOF

kestrel abstractions changes create-batch "$ABS" --file @/tmp/batch.json --agent

# 9. If any item failed, every change rolled back and you get per-item errors
#    keyed by input index. Fix the flagged items and retry the whole batch.

# 10. Audit what landed — list view includes source_links_preview per change.
kestrel abstractions changes list "$ABS" --agent
kestrel abstractions changes list "$ABS" --state rejected --agent   # revisions that need rework

# 11. Human reviews + approves + go-lives in the web UI. The CLI cannot do this.
```

### Iterative single-change drafting (exploratory flow)

For discovery work — "does this field exist, does this citation make sense" —
draft one change at a time with `--cite-block`. Once you know the full set
of changes, switch to `create-batch` with a JSON file for the real pass.

```bash
# Single-change create — the CLI resolves each cite-block's document_id for you.
kestrel abstractions changes create "$ABS" \
  --action create --target-type Lease \
  --payload '{"name":"Ground Floor"}' \
  --cite-block 4240 --agent
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

### Citation modes

Every API-authored change must cite at least one source document. Two modes:

**Block-ref (preferred)** — the document has been parsed. Cite by block id;
the server derives coords + `cited_text` from the parse. Use `--cite-block`
for the common case; drop to `--source-links` for multi-doc or mixed modes.

| Shape                        | Spec                        | JSON fragment                                                            |
|------------------------------|-----------------------------|--------------------------------------------------------------------------|
| Whole block                  | `--cite-block 4821`         | `{"document_block_id": 4821}`                                            |
| Substring of a paragraph     | `--cite-block 4823:chars=14-72` | `{"document_block_id": 4823, "char_start": 14, "char_end": 72}`       |
| Specific table cell          | `--cite-block 4830:cell=2,1`    | `{"document_block_id": 4830, "table_cell_row": 2, "table_cell_col": 1}` |

**Coord mode (fallback)** — the document hasn't been parsed, the parse
failed, or you need to cite something the parse didn't capture (figure
region, scan artifact, etc.). Coordinates are normalized 0–1 relative to
page width and height.

```bash
kestrel abstractions changes create 42 \
  --action update --target-type Lease --target-id 7 \
  --payload '{"start_date":"2026-01-01"}' \
  --source-links '[{
    "document_id": 87,
    "fragments": [
      {"page_number": 3, "x": 0.1, "y": 0.2, "width": 0.3, "height": 0.05,
       "cited_text": "Commencement: January 1, 2026"}
    ]
  }]' \
  --agent
```

Read responses echo back `document_block_id`, `cited_text`, `needs_review`
(true for agent-emitted block-refs awaiting review), and `stale` (true when
a later reparse removed the anchor block).

### Poll parse status manually

When you skipped `--wait-parse` on upload (e.g. batch-attaching several docs
before any citation work), poll until each one is ready:

```bash
# Status is queued|processing|complete|failed|stale. Progress shape:
# {stage, stage_pct, overall_pct, current_page, total_pages, elapsed_ms, …}
kestrel documents parse "$DOC" --agent | jq '.status, .progress'

# Or let the CLI block until terminal. Exits non-zero (code: parse_failed
# or parse_timeout) if the parse doesn't reach complete.
kestrel documents parse "$DOC" --wait --timeout 600 --agent
```

A `failed` or `stale` status means the block graph isn't available — fall
back to coord-mode citations for that document or attach a fresh version.

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
| `documents`         | `kestrel documents show <id>`                  | `download` (binary), `parse`, `pages`, `blocks`, `block <block-id>`             |

### Write (abstraction-scoped)

| Operation                         | Command                                                                   |
|-----------------------------------|---------------------------------------------------------------------------|
| Discover templates                | `kestrel templates list \| show \| schema`                                 |
| Abstraction CRUD                  | `kestrel abstractions create \| show \| update \| abandon`                 |
| Authoring schema                  | `kestrel abstractions schema <abs-id>`                                    |
| Source docs (upload + attach)     | `kestrel abstractions add-doc <abs-id> <file>`                            |
| Source docs (destroy)             | `kestrel abstractions remove-doc <abs-id> --document-id N`                |
| Source docs (list)                | `kestrel abstractions sources <abs-id>`                                   |
| Per-field changes                 | `kestrel abstractions changes create \| create-batch \| list \| show \| update \| delete` |

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
| `parse_missing`           | `documents parse <id>` — the document has never been parsed                      | Attach it to an abstraction via `add-doc` to trigger a parse      |
| `parse_failed`            | `documents parse --wait` — parse terminated in `failed`                          | Inspect `error_message`; fall back to coord-mode citations        |
| `parse_timeout`           | `documents parse --wait` — deadline elapsed before terminal status               | Retry with `--timeout` raised, or poll manually later             |
| `batch_rejected`          | `changes create-batch` — any per-item failure rolls the whole batch back         | Inspect `errors[]` — each entry has `index` (input position) + messages |

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
