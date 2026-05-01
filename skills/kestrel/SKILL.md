---
name: kestrel
description: |
  Interact with Kestrel Portfolio — a lease administration system for global
  retailors and other occupiers — via the kestrel CLI. Purpose built for
  authoring lease abstractions end-to-end.
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
3. **Search first. Walk second.** For any specific value you need to cite —
   a date, rent amount, party name, address, clause — start with
   `kestrel documents search <doc-id> "<phrase>"`. Trigram-indexed, searches
   both prose and table cell text, returns in reading order, tolerates typos
   ("commencment" still hits "commencement"). Walking structurally with
   `documents blocks --type heading` or `--near` is 5–10× more expensive on
   tokens and almost always unnecessary when you know *what* you're looking
   for. Reserve structural walks for when you don't know what you're
   looking for (initial document scan) or need context around a block you
   already found (`--near <block-id>`). Min 4 characters on the query.
4. **Read the template `guidance` on every required field before drafting.**
   Each field in the schema response carries optional `guidance` prose
   authored by the template designer. It encodes house conventions the raw
   document doesn't teach, for example:
   - "If the lease is silent on this clause, write 'Lease is silent for
     this clause' rather than skipping it."
   - "Use absolute dates; resolve relative phrases like '30 days after
     delivery' to a concrete date."
   - "Include suffixes like LLC, Inc. in the tenant name."

   Skipping a required field because the document doesn't address it
   cleanly is almost never correct — the template usually wants an
   explicit sentinel answer. Call `kestrel abstractions schema <abs-id>`
   (or `templates schema <tpl-id>` before create) and scan the Guidance
   block before drafting values. In `--agent` / `--json` mode the
   `guidance` string lives on each field-entry in the JSON response.
5. **Prefer block-ref citations over coord rectangles.** When the document has
   been parsed, cite by `document_block_id` — the server derives coords +
   `cited_text` from the parse. Narrow to a substring with `char_start`/`end`
   or to a specific table cell with `table_cell_row`/`col`. Use coord mode
   only when the parse hasn't rendered the thing you need to cite.
6. **Wait for parse before citing blocks.** Parses are triggered on attach,
   not upload. After `add-doc`, call `documents parse <doc-id> --wait` (or
   pass `--wait-parse` to `add-doc`) before you start reading blocks. Status
   `complete` means blocks are queryable; `failed` means fall back to coord
   mode or a different document.
7. **Use `--agent` for scripting**, `--json` when you need the envelope
   (breadcrumbs, meta), `--quiet`/no flag for human display.
8. **Check auth before anything else.** Exit code 3 means "no token or
   expired". Run `kestrel login` to fix.
9. **Dedup is automatic on pending api-sourced changes.** Re-POSTing
   `changes create` with the same
   `(action, target_type, target_field, sub_object_group)` tuple updates the
   existing pending api-sourced change and returns 200 (not 201). Payload is
   replaced; `source_links` is replaced when provided (omit to leave existing
   links alone); `revised_from_id` is write-once and not recomputed. Use this
   for idempotent retries; don't check for existence first. **Rejected,
   approved, and applied changes are terminal** — they're part of the
   revised_from chain and the audit trail, so PATCH/DELETE refuse with 422.
   To change a rejected value, POST a new create; `revised_from_id`
   auto-links to the rejected predecessor.
10. **Sub-object fields group via a server-minted UUID.** Every new sub-object
    instance (a new KeyDate, Expense, LeaseClause, etc.) is identified by a
    `sub_object_group` UUID that must be issued by the server. Client-
    generated UUIDs are rejected with `sub_object_group_unknown`. Two ways
    to mint:
    - Transparent: `kestrel abstractions changes create ... --sub-object-group new`
      auto-mints before POST and echoes the UUID in breadcrumbs. Use this
      for streaming single creates.
    - Explicit: `kestrel abstractions changes new-group <abs-id> --target-type <T>`
      prints the UUID to stdout. Use this when composing a batch JSON file —
      mint one UUID per sub-object instance first, then embed each in the
      batch items that belong to that instance.

    A minted UUID is bound to the target_type it was minted for; using it
    with a different target_type hits `sub_object_group_target_type_mismatch`.
    Property/Lease creates MUST NOT carry a group — they write directly
    against the abstraction's root record.
11. **Channel-lock: one pending non-default change per tuple, per channel.**
    Every field has a pending slot on `(action, target_type, target_field,
    sub_object_group)`. At most one `pending` non-default change can occupy
    it at a time; the source of that change identifies which channel holds
    it. Consequences:
    - Editing (PATCH/DELETE): api changes can only be edited via the API;
      web_ui changes are read-only to the CLI (403).
    - Drafting (POST): api POSTs against a slot already held by a pending
      web_ui change return 422 `channel_locked` naming the conflict id —
      reject the web_ui change in the web UI first, then POST.
    - The one exception: `default`-sourced scaffolds transparently supersede
      on POST (matching payload → no-op, differing payload → default
      rejected + new change linked via `revised_from_id`). Conditional
      sub-object scaffolds (e.g. pre-filled "Lease Expiration" KeyDate) are
      `default`-sourced, so agents can overwrite them cleanly.
12. **Stream single creates as you find fields; batch only for bulk imports.**
    When reading a document and extracting fields one at a time, POST each
    change the moment you've confirmed the citation — don't hoard them into
    a batch. Streaming gives the reviewer live progress, preserves partial
    work on crash or context limit, and costs fewer tokens than composing a
    large JSON blob. `changes create-batch` is the specialized tool for
    when you *already have* the whole set assembled from another source
    (CSV import, cross-abstraction copy, programmatic translation). Max 500
    items per batch when you do use it.
13. **One payload key per change. Always — except for opaque target types.**
    The server rejects multi-key payloads on any create or update with 422
    `payload_extra_keys` for every target_type *except* the explicitly
    opaque ones — currently `Increase`, where one change carries the whole
    series spec (effective_date + type-specific fields + tiers + recurrence)
    with `target_field` left null. The carve-out is whitelisted server-side;
    don't try it on other types. The provenance binding rationale still
    holds for non-opaque types: one value per change, one change per value,
    so the reviewer's 1:1 field→evidence check works. Streaming per-field
    creates keeps each value paired with exactly its source block(s), and
    multi-field record creation still works — the applier merges per-field
    siblings at go-live, same as sub-objects do via `sub_object_group`.
    Round-trip-sensitive flows compose N per-field items into
    `changes create-batch`; every batch item has the same shape regardless
    of target_type. **Increase** authoring goes through the dedicated
    `kestrel abstractions increase create` command (see "Increases and
    date dependencies" below) — it constructs the opaque payload and
    handles the indexation tier shape, anchored effective_date, and
    recurrence flags.
14. **Don't download the PDF to read it yourself.** When a document has a
    complete parse, `documents search "q"` or `documents blocks --type heading` is
    faster, produces block-anchored citations the abstraction-review flow
    depends on, and costs a fraction of the tokens of pulling raw bytes.
    Downloading and re-extracting loses reading order, table structure,
    and the `document_block_id` needed to cite cleanly — you'd end up in
    coord mode with guessed rectangles. Use `documents download` only for
    file transfer (compliance, forwarding, non-Kestrel consumption), never
    for data extraction.

## Output Modes

| Goal                          | Flag       | Format                                                                     |
|-------------------------------|------------|----------------------------------------------------------------------------|
| Script/agent data extraction  | `--agent`  | Success: raw `data` for detail/scalar responses; `{data, meta}` for paginated lists (so truncation is detectable). Errors: `{ok:false, error, code}`. |
| Full JSON envelope            | `--json`   | `{ok, data, meta, summary, breadcrumbs}` — includes CLI-added hints.       |
| Human display                 | (default)  | Styled table/detail when TTY; JSON when piped. Breadcrumbs to stderr.      |
| Minimal output for scripts    | `--quiet`  | Same as human mode but suppresses success lines and hints.                 |

**Prefer `--agent` for most agent work** — you get clean `data` without having
to unwrap `.data` from an envelope. Use `--json` only when you need breadcrumbs.

### Pagination in `--agent` mode

Detail/scalar responses emit just the record:

```bash
kestrel leases show 42 --agent
# → { "id": 42, "name": "...", ... }
```

Paginated list responses wrap as `{data, meta}` so you can detect truncation
(a 50-item page on a 69-item abstraction would look identical to "all 50" if
we returned a bare array):

```bash
kestrel abstractions changes list 42 --agent
# → { "data": [ ... ], "meta": { "page": 1, "next_page": 2, "count": 69 } }

kestrel documents blocks 87 --agent
# → { "data": [ ... ], "meta": { "count": 500, "limit": 500, "next_since_order": 4821 } }
```

Check `meta.next_page` (offset paging) or `meta.next_since_order` (cursor
paging) to tell whether more data exists. A `null` or missing value means
you've got everything.

## CLI Introspection

Two discovery entrypoints return JSON:

- `kestrel <cmd> --help --agent` — per-command metadata (short/long/flags/subcommands). Walk the tree from the root.
- `kestrel commands --json` — flat catalog of every command.

## Quick Reference

Read surface (properties, leases, expenses, lease-securities, field-configs,
`documents show`, `documents download`, `me`) is uniform — discover via
`kestrel commands --json` or `kestrel <cmd> --help --agent`. Rows below are
abstraction- and parse-specific patterns that aren't obvious from
introspection.

| Task                                 | Command                                                                                                  |
|--------------------------------------|----------------------------------------------------------------------------------------------------------|
| Find a value in a parsed doc         | `kestrel documents search <doc-id> "commencement" --agent`    (trigram, 4+ chars, searches cells too, tolerates typos) |
| Walk block graph by structure        | `kestrel documents blocks <doc-id> [--page N] [--type heading] [--since-order N] --agent`                |
| Blocks near one block                | `kestrel documents blocks <doc-id> --near <block-id> --window 3 --agent`                                 |
| Inspect single block                 | `kestrel documents block <block-id> --agent`                                                             |
| Wait for parse                       | `kestrel documents parse <doc-id> --wait [--timeout 300]`                                                |
| Templates + authoring schema         | `kestrel templates list --agent` / `kestrel templates schema <id> --agent`                               |
| Create greenfield abstraction        | `kestrel abstractions create --template-id N --kind greenfield --agent`                                  |
| Create brownfield abstraction        | `kestrel abstractions create --template-id N --kind brownfield --target-property-id P --target-lease-id L --agent` |
| Show abstraction schema              | `kestrel abstractions schema <abs-id> --agent`                                                           |
| Add source PDF + wait for parse      | `kestrel abstractions add-doc <abs-id> lease.pdf --wait-parse`                                           |
| List / destroy source docs           | `kestrel abstractions sources <abs-id> --agent` / `remove-doc <abs-id> --document-id N`                  |
| Mint sub-object group up-front       | `GROUP=$(kestrel abstractions changes new-group <abs-id> --target-type KeyDate)`                         |
| Draft a scalar update w/ block cite  | `kestrel abstractions changes create <abs-id> --action update --target-type Lease --target-id L --target-field name --payload '{"name":"…"}' --cite-block <block-id>` |
| Cite narrowing                       | `--cite-block <block-id>:chars=14-72` (substring) / `--cite-block <block-id>:cell=2,1` (table cell)      |
| Draft a sub-object field (new inst.) | `--action create --target-type KeyDate --sub-object-group new --target-field name --payload '{"name":"…"}' --cite-block <block-id>`  (reuse minted UUID for sibling fields) |
| Coord-mode citation (fallback)       | `--source-links '[{"document_id":D,"fragments":[{"page_number":3,"x":0.1,"y":0.2,"width":0.3,"height":0.05}]}]'` |
| List / filter staged changes         | `kestrel abstractions changes list <abs-id> [--state rejected] --agent`                                  |
| Bulk import N changes                | `kestrel abstractions changes create-batch <abs-id> --file @batch.json`  (max 500, all-or-nothing)       |
| Update / delete a change             | `kestrel abstractions changes update <abs-id> <change-id> --payload @new.json` / `delete <abs-id> <change-id>` |
| Discover anchor refs                 | `kestrel abstractions anchorable-dates <abs-id> --agent`                                                 |
| Anchor a date field (relative)       | add `--anchor "target_type=…,target_field=…[,target_id=N|sub_object_group=UUID],offset_months=N,offset_days=N,inclusive=true|false"` (repeatable) + `--anchor-resolution earliest_of|latest_of` to `changes create` |
| Override CLI provisional_date        | `--provisional-date YYYY-MM-DD` on `changes create` or `increase create`                                 |
| Draft an Increase (rent escalation)  | `kestrel abstractions increase create <abs-id> --increasable-sub-object-group $G --type fixed|percentage|indexation [type-flags] --effective-date YYYY-MM-DD | --anchor "…" --cite-block <id>` |
| Indexation tiers                     | repeat `--tier "<lo>:<hi>:<rate>"` (blank = open-ended; e.g. `:4:100` and `4::50`)                       |
| Increase recurrence                  | `--recurrence-cadence lease_anniversary|expense_start_anniversary|january_1|custom` (custom adds `--recurrence-interval-amount N --recurrence-interval-unit years|quarters|months`) |
| Abandon an abstraction               | `kestrel abstractions abandon <abs-id>`                                                                  |

## Authoring decision tree

For read-only browsing of properties/leases/expenses, use `kestrel commands --json`
or `kestrel <resource> --help --agent` to discover the shape — the read surface is
uniform (list / show / nested collections by parent).

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
│   └── kestrel documents search <doc-id> "phrase" --agent   (trigram, searches cells too)
│   └── kestrel documents blocks <doc-id> --type heading --agent      (navigate by structure)
│   └── kestrel documents blocks <doc-id> --page 3 --agent            (page-scoped)
│   └── kestrel documents block <block-id> --agent                    (confirm text + bbox)
│   └── kestrel documents blocks <doc-id> --near <id> --window 3 --agent  (context)
├── 5. Discover what fields are expected
│   └── kestrel abstractions schema <abs-id> --agent
├── 6. Draft changes — stream one per field as you confirm each citation
│   ├── Scalar update: --action update --target-type Lease --target-id L --payload '{…}' --cite-block <block-id>
│   ├── Substring citation: --cite-block <block-id>:chars=14-72
│   ├── Table cell citation: --cite-block <block-id>:cell=2,1
│   ├── Sub-object create: --action create --target-type KeyDate --sub-object-group new --payload '{…}' --cite-block <block-id>
│   ├── Anchored date (relative): add --anchor "target_type=…,target_field=…,…" + --anchor-resolution
│   │   (CLI infers provisional_date; pre-discover anchors with `abstractions anchorable-dates <abs-id>`)
│   ├── Increase / rent escalation: kestrel abstractions increase create <abs-id> --increasable-sub-object-group $G --type fixed|percentage|indexation …
│   └── Bulk import path (only when you already have the whole set):
│       kestrel abstractions changes create-batch <abs-id> --file @batch.json
└── 7. (Human completes in web UI: review, approve, go-live)
```

## Increases and date dependencies

Two related features that go beyond plain per-field abstraction changes:

- **Increases** are rent escalations / amount changes on an Expense. Each
  one is fixed (new absolute amount), percentage (% step on the prior
  resolved amount), or indexation (CPI-linked, derived from inflation
  observations between two periods, optionally tiered and clamped by
  floor/ceiling). On the abstraction side they are an **opaque target
  type**: one `AbstractionChange` carries the whole series spec with
  `target_field=null` and a multi-key payload. Don't try to draft them
  one-field-at-a-time — use the dedicated command.
- **Date dependencies** let a date field anchor to another date in the
  same abstraction (or a live record on the target Property/Lease). Same
  change endpoint, but the value position carries a `{mode:"relative",
  resolution, provisional_date, anchors:[…]}` payload instead of a literal
  date. Cascade resolves the resolved date at go-live. Increases are also
  date-dependency-aware — `effective_date` accepts the same relative
  payload, anchored to the parent Expense's start/end (or the Lease's
  dates).

Both features require `Organization#date_dependencies_enabled` to be on for
the org. When it's off, `anchorable-dates` returns an empty list and
relative-date payloads are rejected.

### Discover available anchor refs first

```bash
kestrel abstractions anchorable-dates 42 --agent
# → {anchors:[
#     {kind:"primary_target_live",  target_type:"Lease",   target_field:"start_date",
#      label:"Lease: Start Date",   current_value:"2026-01-01"},
#     {kind:"draft",                target_type:"KeyDate", target_field:"date",
#      sub_object_group:"e8c2…",   label:"Lease Expiration",
#      current_value:null,          state:"pending"},
#     ...
#   ]}
```

The list is the source of truth for what you can anchor against — same as
the web picker. Each entry maps directly into a `--anchor` spec: copy
`target_type`, `target_field`, and whichever of `target_id` /
`sub_object_group` is set.

### Anchor a date field with `--anchor` (non-Increase)

For any date field on a non-opaque target type (KeyDate.date,
Lease.start_date, Expense.start_date/end_date, LeaseSecurity.required_date,
SalesRentTerm.start_date/end_date), use `--anchor` on the generic
`changes create`. Repeat `--anchor` for multi-anchor specs. Add
`--anchor-resolution earliest_of|latest_of` (required when 2+ anchors).
The CLI builds the relative payload around `--target-field` and computes
a best-guess `provisional_date` — fetched from `anchorable-dates` and
shifted by each anchor's offset, falling back to today's date when every
anchor is itself a draft. Pass `--provisional-date` to override.

```bash
# KeyDate.date = Lease.start_date + 90 days (point in time)
kestrel abstractions changes create 42 \
  --action create --target-type KeyDate --sub-object-group new \
  --target-field date \
  --anchor "target_type=Lease,target_field=start_date,offset_days=90,inclusive=false" \
  --cite-block 4280

# Multi-anchor: earliest of (Handover + 3 months) or Store Open
kestrel abstractions changes create 42 \
  --action create --target-type Lease --target-field rent_commencement_date \
  --anchor "target_type=KeyDate,target_field=date,sub_object_group=$HANDOVER_GROUP,offset_months=3,inclusive=false" \
  --anchor "target_type=KeyDate,target_field=date,sub_object_group=$STORE_OPEN_GROUP,offset_months=0,offset_days=0,inclusive=false" \
  --anchor-resolution earliest_of \
  --cite-block 4302
```

Anchor spec keys (comma-separated, no spaces around `=`):

| Key                | Required             | Notes                                                     |
|--------------------|----------------------|-----------------------------------------------------------|
| `target_type`      | yes                  | Lease, Property, KeyDate, Expense, LeaseSecurity, SalesRentTerm, Increase |
| `target_field`     | yes                  | Date column on the anchor record                          |
| `target_id`        | xor sub_object_group | Use for live records (brownfield).                        |
| `sub_object_group` | xor target_id        | Use for sibling draft sub-objects in the same abstraction.|
| `offset_months`    | no (default 0)       | Negative for "before".                                    |
| `offset_days`      | no (default 0)       |                                                           |
| `inclusive`        | no (default true)    | Real-estate "term to/from" math. Set false for "N days after". |

### Draft an Increase with `kestrel abstractions increase create`

The dedicated write surface for the opaque Increase target type. Builds the
multi-key payload, the indexation tier rows, the anchored or absolute
`effective_date`, and the optional `recurrence` block.

```bash
# Greenfield draft expense — recurring CPI escalation, anchored to lease start
RENT_GROUP=$(kestrel abstractions changes new-group 42 --target-type Expense)
kestrel abstractions changes create 42 \
  --action create --target-type Expense --sub-object-group "$RENT_GROUP" \
  --target-field name --payload '{"name":"Base Rent"}' --cite-block 4301:cell=2,1
# (… plus amount/currency/start_date/end_date sibling fields on $RENT_GROUP …)

kestrel abstractions increase create 42 \
  --increasable-sub-object-group "$RENT_GROUP" \
  --type indexation \
  --inflation-series-id 7 --start-period 2027-01 --end-period 2028-01 \
  --floor 0 --ceiling 5 \
  --tier ":4:100" --tier "4::50" \
  --anchor "target_type=Expense,sub_object_group=$RENT_GROUP,target_field=start_date,offset_months=12,inclusive=false" \
  --anchor-resolution earliest_of \
  --recurrence-cadence lease_anniversary \
  --notes "Annual CPI escalation, 100% to 4%, 50% above" \
  --cite-block 4350 --cite-block 4351

# Brownfield existing expense — fixed bump on a known date
kestrel abstractions increase create 42 \
  --increasable-target-id 17 \
  --type fixed --fixed-amount 12500 \
  --effective-date 2027-04-01 \
  --cite-block 4980
```

**Parent linkage** — exactly one of:

- `--increasable-sub-object-group <uuid>` — the parent Expense is a sibling
  draft in the same abstraction (greenfield, or a brownfield-with-new-expense).
  Pass the Expense's `sub_object_group` UUID. The applier resolves it to the
  live Expense at go-live time via the same UUID.
- `--increasable-target-id <expense-id>` — the parent Expense is already live
  (brownfield, modifying an existing expense's series).

**Effective date** — exactly one of:

- `--effective-date YYYY-MM-DD` (absolute), or
- `--anchor` (repeatable) + `--anchor-resolution` + optional
  `--provisional-date`. Identical syntax and semantics to `changes create`'s
  `--anchor`. The CLI infers `provisional_date` and prints a notice; pass
  `--provisional-date` to override.

**Indexation tiers** — repeatable `--tier "<lo>:<hi>:<rate>"`:

| Spec        | Meaning                                                        |
|-------------|----------------------------------------------------------------|
| `":4:100"`  | open below, capped at 4%, applied at 100%                      |
| `"4:8:50"`  | between 4% and 8%, applied at 50%                              |
| `"8::25"`   | above 8% (open above), applied at 25%                          |

Local validation runs before POST: contiguity (each tier's lo equals the
previous tier's hi), open-above on the last tier, rate in [0, 100]. Empty
tier list = 100% pass-through of the index change.

**Recurrence** (optional — expanded at go-live, not stored as separate
draft changes):

- `--recurrence-cadence lease_anniversary` — annual on the lease start anniv.
- `--recurrence-cadence expense_start_anniversary` — annual on expense start.
- `--recurrence-cadence january_1` — annual on January 1.
- `--recurrence-cadence custom --recurrence-interval-amount N --recurrence-interval-unit years|quarters|months`

Anchored effective_date + recurrence is the **star pattern**: every
recurrence shares the same anchor record with offsets accumulated by
`interval_amount × occurrence`. One change, N live increases at go-live.

### When does the CLI compute `provisional_date`?

Only when `--anchor` is supplied and `--provisional-date` is omitted. The
CLI fetches `/anchorable_dates` and:

1. Resolves each anchor's `current_value` and applies the spec's offset
   (with end-of-month-aware month math, mirroring the server).
2. Combines candidates via `earliest_of` / `latest_of`.
3. **Falls back to today** when no anchor has a known `current_value`
   (every anchor is itself a pending draft). This is the "best guess" the
   API needs at draft time so column-level NOT-NULL invariants hold; phase
   2 of go-live overwrites with the authoritative resolution.

When the inferred date matters, validate it with `--provisional-date`. The
CLI prints a breadcrumb whenever it inferred the value.

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
kestrel documents search "$DOC" "commencement" --agent
# → [{id: 4240, text: "…commencement date shall be January 1, 2026…", reading_order: 42}]

kestrel documents search "$DOC" "base rent" --type table --agent
# → table block whose cells match; inspect metadata.cells[] to pick the cell

# 6. Or walk by structure if search is too loose.
kestrel documents blocks "$DOC" --type heading --agent
# → [{id: 4201, text: "1. Premises", page_number: 1, reading_order: 8}, …]

kestrel documents blocks "$DOC" --near 4201 --window 3 --agent
# → blocks surrounding the "Premises" heading

# 7. Confirm the exact block before citing (optional but cheap).
kestrel documents block 4215 --agent

# 8. Draft changes — stream one per field as you confirm each citation.
#    Each POST is idempotent (dedup by tuple), so retries are safe.
#    The reviewer sees work land live; partial progress survives crashes.

kestrel abstractions changes create "$ABS" \
  --action create --target-type Property \
  --payload '{"name":"123 Main St","city":"Austin","country":"US"}' \
  --cite-block 4215 --agent

kestrel abstractions changes create "$ABS" \
  --action create --target-type Lease \
  --payload '{"name":"Ground Floor","start_date":"2026-01-01","end_date":"2030-12-31"}' \
  --cite-block 4240:chars=14-72 --agent

# Sub-object — mint one UUID server-side, reuse it for every field in the instance.
EXPENSE_GROUP=$(kestrel abstractions changes new-group "$ABS" --target-type Expense)
kestrel abstractions changes create "$ABS" \
  --action create --target-type Expense --sub-object-group "$EXPENSE_GROUP" \
  --target-field name --payload '{"name":"Base Rent"}' \
  --cite-block 4301:cell=2,1 --agent
kestrel abstractions changes create "$ABS" \
  --action create --target-type Expense --sub-object-group "$EXPENSE_GROUP" \
  --target-field amount --payload '{"amount":5000}' \
  --cite-block 4301:cell=2,2 --agent
kestrel abstractions changes create "$ABS" \
  --action create --target-type Expense --sub-object-group "$EXPENSE_GROUP" \
  --target-field currency --payload '{"currency":"USD"}' \
  --cite-block 4301:cell=2,3 --agent

# KeyDate — same pattern. Note: for interactive single creates you can also
# use --sub-object-group new to mint transparently on the first call, then
# reuse the returned UUID (echoed via breadcrumb) for the siblings.
KEYDATE_GROUP=$(kestrel abstractions changes new-group "$ABS" --target-type KeyDate)
kestrel abstractions changes create "$ABS" \
  --action create --target-type KeyDate --sub-object-group "$KEYDATE_GROUP" \
  --target-field name --payload '{"name":"Expiration"}' \
  --cite-block 4280 --agent
kestrel abstractions changes create "$ABS" \
  --action create --target-type KeyDate --sub-object-group "$KEYDATE_GROUP" \
  --target-field date --payload '{"date":"2030-12-31"}' \
  --cite-block 4280 --agent

# 9. Audit what landed — list view includes source_links_preview per change.
#    Output is {data, meta} in --agent mode so you can detect truncation.
kestrel abstractions changes list "$ABS" --agent
kestrel abstractions changes list "$ABS" --state rejected --agent   # revisions that need rework

# 10. Human reviews + approves + go-lives in the web UI. The CLI cannot do this.
```

### Bulk import (when you already have the whole set)

`changes create-batch` is for the specialized case where you've already
assembled the full list of changes from another source — a CSV, another
abstraction, an external system — and just need to load them. All-or-
nothing transaction, coalesced broadcasts, up to 500 items per call.
Don't use it during interactive authoring; streaming creates gives the
reviewer live progress, preserves partial work on failure, and costs
fewer tokens than composing a large JSON file.

```bash
# Example: loading an abstraction from a pre-translated CSV.
# Every batch item is a single-field change. Sub-object instances need a
# server-minted group UUID — mint one per instance BEFORE composing the
# batch and embed the UUIDs in the relevant items.
RENT_GROUP=$(kestrel abstractions changes new-group "$ABS" --target-type Expense)
EXPIRY_GROUP=$(kestrel abstractions changes new-group "$ABS" --target-type KeyDate)

cat > /tmp/batch.json <<EOF
[
  {
    "action": "create", "target_type": "Property", "target_field": "name",
    "payload": {"name":"123 Main St"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4215}]}]
  },
  {
    "action": "create", "target_type": "Property", "target_field": "city",
    "payload": {"city":"Austin"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4215}]}]
  },
  {
    "action": "create", "target_type": "Lease", "target_field": "start_date",
    "payload": {"start_date":"2026-01-01"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4240,"char_start":14,"char_end":72}]}]
  },
  {
    "action": "create", "target_type": "Expense", "target_field": "name",
    "sub_object_group": "$RENT_GROUP",
    "payload": {"name":"Base Rent"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4301,"table_cell_row":2,"table_cell_col":1}]}]
  },
  {
    "action": "create", "target_type": "Expense", "target_field": "amount",
    "sub_object_group": "$RENT_GROUP",
    "payload": {"amount":5000},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4301,"table_cell_row":2,"table_cell_col":2}]}]
  },
  {
    "action": "create", "target_type": "KeyDate", "target_field": "name",
    "sub_object_group": "$EXPIRY_GROUP",
    "payload": {"name":"Expiration"},
    "source_links": [{"document_id": $DOC, "fragments":[{"document_block_id":4280}]}]
  }
]
EOF

kestrel abstractions changes create-batch "$ABS" --file @/tmp/batch.json --agent
# If any item fails, every change rolls back and you get per-item errors
# keyed by input index. Fix the flagged items and retry the whole batch.
```

### Revise a rejected change

```bash
# When the web reviewer rejects a change, the rejection reason is attached.
kestrel abstractions changes list 42 --state rejected --agent | jq '.data[]'

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

### Schema response shape

`kestrel abstractions schema <abs-id>` and `kestrel templates schema <tpl-id>`
return `{data: {models: {<ModelName>: {...}, ...}}}`. Every model drafts the
same way: one change per field, single-key payload, `target_field` explicit.
The distinction between "root Property/Lease" and "sub-object" only matters
for *grouping*, not payload shape:

- Root Property/Lease entries describe fields you write directly against the
  abstraction's single Property/Lease. No `sub_object_group`.
- Sub-object entries hang under their parent model's `sub_objects[]` list.
  Every instance is identified by a server-minted `sub_object_group` UUID
  shared across its field changes.

Each model entry has:
- `fields: [...]` — scalar fields expected on the model. One entry per
  requirement.
- `sub_objects: [...]` — nested types that hang off this model. Each entry
  describes a *type*; create an *instance* by minting a group via
  `changes new-group` (or `--sub-object-group new` on a streamed create)
  and POSTing one change per field stamped with that UUID.

Each `sub_objects[]` entry carries:
- `kind: "sub_object"` — discriminator constant.
- `required: true|false` — whether this sub-object counts for go-live.
  `false` means the abstraction can go live with zero instances.
- `min_count: N` — number of instances required *when counted*. Always ≥ 1.
  `required: false, min_count: 1` means "agents may draft one, but go-live
  doesn't require it" — `required` gates go-live, `min_count` doesn't.
- `condition: {field: value, ...}` — optional jsonb filter. When present,
  instances must satisfy it (e.g. `{name: "Lease Expiration"}` means this
  is specifically the expiration KeyDate). Conditional sub-objects are
  pre-scaffolded as `default`-sourced changes that agents can supersede
  transparently via POST.
- `fields: [...]` — field shape for one instance; same as the top-level
  `fields` array.

Each `fields[]` entry carries:
- `field_name` — use as `target_field` (or omit and let the server infer
  from a single-key payload).
- `type` — one of `string`, `boolean`, `date`, `decimal`, `area`.
- `caption` — org-customized human-readable label.
- `required: true|false` — counts for go-live. Not a POST-time constraint.
- `options: [{value, label}, ...]` — present when the field has a
  constrained list (string with dropdown, or boolean). **Authoritative** —
  don't cross-reference `/field-configs`, the schema is the source of
  truth. Absent ⇒ any value of the declared type is fine.
- `default_value` — pre-resolved from the template → FieldConfig → model-
  default cascade. The abstraction already has a `default`-sourced change
  carrying this; echoing it back POST as a no-op via supersede.
- `guidance` — optional prose from the template author for the drafter.

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

## Error Handling

Structured errors in `--json` / `--agent` mode:

```json
{"ok": false, "error": "<template-id> required", "code": "usage", "hint": "kestrel abstractions create --template-id N --kind greenfield|brownfield"}
```

Generic REST codes (`usage`, `validation`, `unauthorized`, `token_expired`,
`api_disabled`, `forbidden`, `not_found`, `rate_limited`) carry self-evident
meaning — `error` + `hint` on the envelope is usually enough. Rate limits:
reads 200/30s burst, 2000/min; writes 500/min.

Kestrel-specific codes worth knowing:

| Code                      | Meaning                                                                          | Fix                                                               |
|---------------------------|----------------------------------------------------------------------------------|-------------------------------------------------------------------|
| `cited_by_locked_change`  | `remove-doc` on a doc cited by an approved/applied change                        | Reject the change in the web UI first                             |
| `payload_extra_keys`      | Payload has more than one key; must be exactly `{target_field: value}`           | One key per change — always                                       |
| `target_field_required`   | Create/update submitted without an explicit `target_field`                       | Set `--target-field` or use a single-key payload the CLI can lift |
| `channel_locked`          | POSTing an api change on a pending slot held by web_ui                           | Reject the web_ui change in the web UI first, then retry          |
| `sub_object_group_not_allowed`          | Property/Lease create carried a `sub_object_group`                  | Drop `--sub-object-group`; root creates don't carry a group       |
| `sub_object_group_required`             | Sub-object create missing `sub_object_group`                        | Mint via `changes new-group` or pass `--sub-object-group new`     |
| `sub_object_group_unknown`              | UUID wasn't minted on this abstraction (client UUIDs rejected)      | Re-mint with `changes new-group`                                  |
| `sub_object_group_target_type_mismatch` | UUID was minted for a different target_type                         | Mint a fresh group for the current target_type                    |
| `parse_missing`           | Document has never been parsed                                                   | Attach to an abstraction via `add-doc` to trigger                 |
| `parse_failed` / `parse_timeout` | Parse wait reached terminal failure / deadline                            | Inspect `error_message`; fall back to coord-mode citations        |
| `batch_rejected`          | `changes create-batch` — any per-item failure rolls the whole batch back         | Inspect `errors[]` — each entry has `index` + `code` + messages   |

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

`KESTREL_TOKEN` and `KESTREL_BASE_URL` env vars override the saved config at
`~/.config/kestrel/config.json`. Flag `--base-url` wins for a single call.
Verify with `kestrel me --agent` or `kestrel doctor --json`. Full precedence
and per-environment setup in `docs/environments.md`.

## Learn More

- API spec: `../kestrel_portfolio/docs/openapi.yaml` (OpenAPI 3.1)
- API narrative: `../kestrel_portfolio/docs/api.md`
- Abstraction concepts: `../kestrel_portfolio/docs/abstractions.md`
- CLI repo: https://github.com/kestrelportfolio/kestrel-cli
