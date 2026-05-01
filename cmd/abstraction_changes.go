package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

type abstractionChange struct {
	ID                     int                      `json:"id"`
	Action                 string                   `json:"action"`
	TargetType             string                   `json:"target_type"`
	TargetField            *string                  `json:"target_field"`
	TargetID               *int                     `json:"target_id"`
	Payload                json.RawMessage          `json:"payload"`
	State                  string                   `json:"state"`
	Source                 string                   `json:"source"`
	SubObjectGroup         *string                  `json:"sub_object_group"`
	ParentChangeID         *int                     `json:"parent_change_id"`
	RevisedFromID          *int                     `json:"revised_from_id"`
	OwnerID                *int                     `json:"owner_id"`
	ReviewedByID           *int                     `json:"reviewed_by_id"`
	ReviewedAt             *string                  `json:"reviewed_at"`
	AppliedAt              *string                  `json:"applied_at"`
	RejectionReason        *string                  `json:"rejection_reason"`
	FieldMetadata          json.RawMessage          `json:"field_metadata,omitempty"`
	SourceLinks            json.RawMessage          `json:"source_links,omitempty"`
	SourceLinksPreview     []abstractionLinkPreview `json:"source_links_preview,omitempty"`
	RelativeDateResolution *relativeDateResolution  `json:"relative_date_resolution,omitempty"`
	CreatedAt              string                   `json:"created_at"`
}

// relativeDateResolution mirrors the server's live render of an anchor
// formula. Present only when the change carries a RelativeDatePayload.
// Echoing this back to the agent right after a successful POST is the
// fastest sanity check on a freshly-drafted anchor — a malformed anchor
// that slipped past validation shows up as a formula like "30 days after ."
// (no anchor label) and the agent can re-draft within the same turn
// instead of finding out at workspace render or go-live.
type relativeDateResolution struct {
	Formula       string  `json:"formula"`
	ResolvedValue *string `json:"resolved_value"`
}

type abstractionLinkPreview struct {
	DocumentID   int                   `json:"document_id"`
	DocumentName string                `json:"document_name"`
	Fragments    []abstractionFragPrev `json:"fragments"`
}

type abstractionFragPrev struct {
	PageNumber       int    `json:"page_number"`
	CitedTextPreview string `json:"cited_text_preview"`
}

var abstractionChangesCmd = &cobra.Command{
	Use:     "changes",
	Aliases: []string{"change"},
	Short:   "Draft, inspect, and manage per-field changes on an abstraction",
}

var (
	abstractionChangesListPage   int
	abstractionChangesListStates []string
)

// validChangeStates mirrors AbstractionChange::STATES on the server.
// The server whitelists unknown values; we pre-validate so the user gets
// an immediate, actionable error instead of silent drops.
var validChangeStates = map[string]bool{
	"pending":  true,
	"approved": true,
	"rejected": true,
	"applied":  true,
}

var abstractionChangesListCmd = &cobra.Command{
	Use:   "list <abstraction-id>",
	Short: "List staged changes on an abstraction",
	Long: `Lists per-field changes with citation previews. Filter via --state
(repeatable) to narrow to a specific stage of the review lifecycle:

  pending | approved | rejected | applied

List rows include source_links_preview — document handle, name, and a
truncated cited_text per fragment. Fetch the detailed show for full
geometry and block_id.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		for _, s := range abstractionChangesListStates {
			if !validChangeStates[s] {
				return &UsageError{
					Arg:   "state",
					Usage: "--state must be one of: pending, approved, rejected, applied",
				}
			}
		}
		// url.Values so we can emit repeated `state[]=` for multi-state filters.
		q := url.Values{}
		if abstractionChangesListPage > 1 {
			q.Set("page", strconv.Itoa(abstractionChangesListPage))
		}
		switch len(abstractionChangesListStates) {
		case 0:
			// no filter
		case 1:
			q.Set("state", abstractionChangesListStates[0])
		default:
			for _, s := range abstractionChangesListStates {
				q.Add("state[]", s)
			}
		}
		raw, err := client.GetRawValues("/abstractions/"+args[0]+"/changes", q)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []abstractionChange `json:"data"`
				Meta *paginatedMeta      `json:"meta"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"ID", "Action", "Target", "Field", "State", "Source", "Created", "Citations"}
			rows := make([][]string, len(resp.Data))
			for i, c := range resp.Data {
				target := c.TargetType
				if c.TargetID != nil {
					target = fmt.Sprintf("%s #%d", c.TargetType, *c.TargetID)
				}
				rows[i] = []string{
					strconv.Itoa(c.ID),
					c.Action,
					target,
					deref(c.TargetField),
					c.State,
					c.Source,
					c.CreatedAt,
					summarizeLinkPreview(c.SourceLinksPreview),
				}
			}
			printer.Table(headers, rows)
			if resp.Meta != nil {
				printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var abstractionChangesShowCmd = &cobra.Command{
	Use:   "show <abstraction-id> <change-id>",
	Short: "Show a single change with payload, field metadata, and source links",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstractions/"+args[0]+"/changes/"+args[1], nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data abstractionChange `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			c := resp.Data
			pairs := [][]string{
				{"ID", strconv.Itoa(c.ID)},
				{"Action", c.Action},
				{"Target type", c.TargetType},
				{"Target ID", derefInt(c.TargetID)},
				{"Target field", deref(c.TargetField)},
				{"State", c.State},
				{"Source", c.Source},
				{"Sub-object group", deref(c.SubObjectGroup)},
				{"Parent change ID", derefInt(c.ParentChangeID)},
				{"Revised from", derefInt(c.RevisedFromID)},
				{"Owner ID", derefInt(c.OwnerID)},
				{"Reviewed by", derefInt(c.ReviewedByID)},
				{"Reviewed at", deref(c.ReviewedAt)},
				{"Applied at", deref(c.AppliedAt)},
				{"Rejection reason", deref(c.RejectionReason)},
				{"Created", c.CreatedAt},
			}
			printer.Detail(pairs)
			if len(c.Payload) > 0 {
				fmt.Println()
				fmt.Println("Payload")
				fmt.Println("───────")
				pretty, _ := json.MarshalIndent(json.RawMessage(c.Payload), "", "  ")
				fmt.Println(string(pretty))
			}
			if len(c.FieldMetadata) > 0 && string(c.FieldMetadata) != "null" {
				fmt.Println()
				fmt.Println("Field metadata")
				fmt.Println("──────────────")
				pretty, _ := json.MarshalIndent(json.RawMessage(c.FieldMetadata), "", "  ")
				fmt.Println(string(pretty))
			}
			if len(c.SourceLinks) > 0 && string(c.SourceLinks) != "null" {
				fmt.Println()
				fmt.Println("Source links")
				fmt.Println("────────────")
				pretty, _ := json.MarshalIndent(json.RawMessage(c.SourceLinks), "", "  ")
				fmt.Println(string(pretty))
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var (
	changesCreateAction             string
	changesCreateTargetType         string
	changesCreateTargetID           int
	changesCreateTargetField        string
	changesCreateSubObjectGroup     string
	changesCreateParentChangeID     int
	changesCreateRevisedFromID      int
	changesCreatePayloadInput       string
	changesCreateSourceLinksInput   string
	changesCreateCiteBlocks         []string
	changesCreateAnchorSpecs        []string
	changesCreateAnchorResolution   string
	changesCreateProvisionalDate    string
)

var abstractionChangesCreateCmd = &cobra.Command{
	Use:   "create <abstraction-id>",
	Short: "Create or upsert a staged change",
	Long: `Creates a new per-field change (or upserts if one already exists for the
same action/target/field/sub_object_group tuple — that's how dedup works).

Payload and source-links inputs can be:
  * an inline JSON string:   --payload '{"name":"Acme"}'
  * a file:                  --payload @/tmp/payload.json
  * stdin:                   --payload -

Citations:
  --cite-block <spec>    Repeatable shortcut for block-ref fragments. Each
                         spec is one of:
                           <block-id>                       whole block
                           <block-id>:chars=<start>-<end>   substring
                           <block-id>:cell=<row>,<col>      table cell
                         The CLI resolves each block's document_id via
                         GET /document_blocks/:id and groups fragments by
                         document. Mutually exclusive with --source-links.
  --source-links <json>  Raw JSON when you need full control (e.g. coord-
                         mode fragments, labels, colors).

Examples:
  # Scalar update — cite a single block
  kestrel abstractions changes create 42 \
    --action update --target-type Lease --target-id 7 \
    --payload '{"name":"Acme Lease"}' \
    --cite-block 4821

  # Sub-object create — cite a table cell and a substring
  kestrel abstractions changes create 42 \
    --action create --target-type KeyDate --sub-object-group new \
    --payload '{"name":"Expiration","date":"2030-12-31"}' \
    --cite-block 4830:cell=2,1 \
    --cite-block 4823:chars=14-72

  # Full control via raw JSON (coord-mode highlight)
  kestrel abstractions changes create 42 \
    --action update --target-type Lease --target-id 7 \
    --payload '{"start_date":"2026-01-01"}' \
    --source-links '[{"document_id":87,"fragments":[{"page_number":3,"x":0.1,"y":0.2,"width":0.3,"height":0.05}]}]'

Anchored (relative) date fields — the CLI builds the relative payload for you
when --anchor is supplied. Requires --target-field; --payload must be omitted
(the CLI assembles {target_field: {mode:"relative",...}} itself).

  --anchor <spec>            Repeatable. Comma-separated key=value pairs:
                               target_type=Lease,target_field=start_date,
                               offset_months=12,offset_days=0,inclusive=false
                             Live-record anchors take target_id=N; sibling
                             draft anchors take sub_object_group=<uuid>;
                             primary-target Lease/Property anchors take
                             neither.
  --anchor-resolution        earliest_of | latest_of. Required when 2+ anchors.
                             Defaults to earliest_of for single-anchor specs.
  --provisional-date <date>  Optional override (YYYY-MM-DD). When omitted the
                             CLI computes a best-guess by fetching
                             /anchorable_dates and applying each anchor's
                             offset to the live current_value when available;
                             if every anchor is itself a draft and pending
                             resolution, falls back to today's date so the
                             field's column-NOT-NULL invariant holds at
                             phase 1 of go-live (phase 2 then resolves
                             authoritatively from the dependency graph).

  # Anchored sub-object date — KeyDate.date = Lease.start_date + 90 days
  kestrel abstractions changes create 42 \
    --action create --target-type KeyDate --sub-object-group new \
    --target-field date \
    --anchor "target_type=Lease,target_field=start_date,offset_days=90,inclusive=false" \
    --cite-block 4280

Discover valid anchor refs:

  kestrel abstractions anchorable-dates 42 --agent`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if changesCreateAction == "" {
			return &UsageError{Arg: "action", Usage: "kestrel abstractions changes create <abs-id> --action create|update|destroy --target-type <Model> --payload ..."}
		}
		if changesCreateTargetType == "" {
			return &UsageError{Arg: "target-type", Usage: "kestrel abstractions changes create <abs-id> --action ... --target-type <Model> --payload ..."}
		}

		change := map[string]any{
			"action":      changesCreateAction,
			"target_type": changesCreateTargetType,
		}
		if changesCreateTargetID > 0 {
			change["target_id"] = changesCreateTargetID
		}
		if changesCreateTargetField != "" {
			change["target_field"] = changesCreateTargetField
		}
		if changesCreateParentChangeID > 0 {
			change["parent_change_id"] = changesCreateParentChangeID
		}
		if changesCreateRevisedFromID > 0 {
			change["revised_from_id"] = changesCreateRevisedFromID
		}

		// sub_object_group: "new" → mint server-side, otherwise pass through.
		// Server-issued UUIDs are the only accepted form; client-generated
		// values are rejected with sub_object_group_unknown.
		mintedUUID := ""
		if changesCreateSubObjectGroup != "" {
			if changesCreateSubObjectGroup == "new" {
				u, err := mintSubObjectGroup(args[0], changesCreateTargetType)
				if err != nil {
					return err
				}
				mintedUUID = u
				change["sub_object_group"] = u
			} else {
				change["sub_object_group"] = changesCreateSubObjectGroup
			}
		}

		// Payload assembly — three modes:
		//   1. --anchor supplied → CLI builds relative-date payload around target_field.
		//   2. --payload supplied → caller-controlled JSON.
		//   3. neither → usage error.
		// (1) and (2) are mutually exclusive — anchor mode owns the payload shape.
		hasAnchor := len(changesCreateAnchorSpecs) > 0
		if hasAnchor && changesCreatePayloadInput != "" {
			return &UsageError{
				Arg:   "anchor and payload",
				Usage: "pass --anchor (CLI builds the relative payload) OR --payload (raw JSON), not both",
			}
		}
		if hasAnchor {
			if changesCreateTargetField == "" {
				return &UsageError{
					Arg:   "target-field",
					Usage: "--anchor requires --target-field (the date field being anchored)",
				}
			}
			anchors := make([]*anchorSpec, 0, len(changesCreateAnchorSpecs))
			for _, raw := range changesCreateAnchorSpecs {
				spec, err := parseAnchorSpec(raw)
				if err != nil {
					return err
				}
				anchors = append(anchors, spec)
			}
			rel, provisional, inferred, err := buildRelativePayload(args[0], changesCreateAnchorResolution, anchors, changesCreateProvisionalDate)
			if err != nil {
				return err
			}
			change["payload"] = map[string]any{changesCreateTargetField: rel}
			if inferred {
				printer.Breadcrumb(fmt.Sprintf("provisional_date inferred as %s — pass --provisional-date to override", provisional))
			}
		} else {
			payloadStr, err := readInputValue(changesCreatePayloadInput)
			if err != nil {
				return err
			}
			if payloadStr == "" {
				return &UsageError{Arg: "payload", Usage: "--payload '{\"field\":\"value\"}' or --payload @file.json or --payload - (or use --anchor for a relative-date payload)"}
			}
			var payload any
			if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
				return fmt.Errorf("parsing --payload as JSON: %w", err)
			}
			change["payload"] = payload
		}

		// source_links can come from --cite-block (resolved to JSON) or --source-links
		// (passed through verbatim). Rejecting both keeps the mental model simple.
		if changesCreateSourceLinksInput != "" && len(changesCreateCiteBlocks) > 0 {
			return &UsageError{
				Arg:   "cite-block and source-links",
				Usage: "pass --cite-block OR --source-links, not both",
			}
		}
		if changesCreateSourceLinksInput != "" {
			slStr, err := readInputValue(changesCreateSourceLinksInput)
			if err != nil {
				return err
			}
			var links any
			if err := json.Unmarshal([]byte(slStr), &links); err != nil {
				return fmt.Errorf("parsing --source-links as JSON: %w", err)
			}
			change["source_links"] = links
		}
		if len(changesCreateCiteBlocks) > 0 {
			links, err := resolveCiteBlocks(changesCreateCiteBlocks)
			if err != nil {
				return err
			}
			change["source_links"] = links
		}

		env, err := client.Post(
			"/abstractions/"+args[0]+"/changes",
			map[string]any{"change": change},
		)
		if err != nil {
			hintChangeCreateError(err)
			return err
		}
		if mintedUUID != "" {
			printer.Breadcrumb(fmt.Sprintf("Sub-object group UUID: %s", mintedUUID))
			printer.Breadcrumb(fmt.Sprintf("Add sibling fields to this group: --sub-object-group %s", mintedUUID))
		}
		return renderChangeResult(env, true)
	},
}

var (
	changesUpdatePayloadInput     string
	changesUpdateSourceLinksInput string
)

var abstractionChangesUpdateCmd = &cobra.Command{
	Use:   "update <abstraction-id> <change-id>",
	Short: "Update a change's payload or source links",
	Long: `Only API-sourced pending changes can be updated. Rejected, approved,
and applied changes are terminal — they anchor the revised_from chain and
can't be mutated. To change a rejected value, POST a new create via
'changes create'; revised_from_id auto-links to the rejected predecessor.

Omit --source-links to leave citations untouched; pass an empty array
(--source-links '[]') to clear them. Providing --source-links replaces the
existing set (same semantics as the dedup re-POST path).`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		change := map[string]any{}

		if changesUpdatePayloadInput != "" {
			s, err := readInputValue(changesUpdatePayloadInput)
			if err != nil {
				return err
			}
			var payload any
			if err := json.Unmarshal([]byte(s), &payload); err != nil {
				return fmt.Errorf("parsing --payload as JSON: %w", err)
			}
			change["payload"] = payload
		}
		if changesUpdateSourceLinksInput != "" {
			s, err := readInputValue(changesUpdateSourceLinksInput)
			if err != nil {
				return err
			}
			var links any
			if err := json.Unmarshal([]byte(s), &links); err != nil {
				return fmt.Errorf("parsing --source-links as JSON: %w", err)
			}
			change["source_links"] = links
		}
		if len(change) == 0 {
			return &UsageError{Arg: "payload or source-links", Usage: "kestrel abstractions changes update <abs-id> <change-id> --payload ... [--source-links ...]"}
		}

		env, err := client.Patch(
			"/abstractions/"+args[0]+"/changes/"+args[1],
			map[string]any{"change": change},
		)
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.StatusCode {
				case 403:
					printer.Errorf("update refused — only API-sourced pending changes can be edited via the API")
				case 422:
					// Rejected/approved/applied are terminal — redraft via a
					// new POST instead (revised_from_id auto-links).
					printer.Errorf("update refused — change is in a terminal state (rejected/approved/applied). Draft a replacement via `kestrel abstractions changes create ...`; revised_from_id auto-links to the rejected predecessor.")
				}
			}
			return err
		}
		return renderChangeResult(env, false)
	},
}

var (
	batchCreateFileInput string
)

// batchMaxItems mirrors the server-side max batch size. Pre-validating here
// turns a 422 roundtrip into an immediate usage error.
const batchMaxItems = 500

var abstractionChangesBatchCmd = &cobra.Command{
	Use:   "create-batch <abstraction-id>",
	Short: "Create many changes atomically in one request",
	Long: `Stages N changes against an abstraction in a single transaction.
All-or-nothing: any per-item failure rolls the batch back and returns 422
with per-item errors indexed by input array position.

Each item uses the same write path as single 'changes create' —
supersede_pending_default, revised_from auto-link, dedup, source_links
validation. Broadcasts are coalesced on the server: a batch spanning K
unique target_types fires K websocket jobs, not N.

Max batch size: 500 items.

Input (--file): either a raw array of change objects, or the full
wrapper {"changes": [...]}. Accepts:
  * file path:  --file @/tmp/batch.json
  * stdin:      --file -
  * inline:     --file '[{"action":"create",...}]'

Example batch file:
  [
    {"action":"create","target_type":"Property",
     "payload":{"name":"123 Main"},
     "source_links":[{"document_id":87,"fragments":[{"document_block_id":4215}]}]},
    {"action":"create","target_type":"Lease",
     "payload":{"name":"Ground Floor"},
     "source_links":[{"document_id":87,"fragments":[{"document_block_id":4220}]}]}
  ]`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if batchCreateFileInput == "" {
			return &UsageError{
				Arg:   "file",
				Usage: "kestrel abstractions changes create-batch <abs-id> --file @batch.json",
			}
		}
		raw, err := readInputValue(batchCreateFileInput)
		if err != nil {
			return err
		}
		if raw == "" {
			return &UsageError{Arg: "file", Usage: "--file input is empty"}
		}
		// Accept either a raw array or the {changes: [...]} wrapper.
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return fmt.Errorf("parsing --file as JSON: %w", err)
		}
		var changes []any
		switch v := parsed.(type) {
		case []any:
			changes = v
		case map[string]any:
			c, ok := v["changes"].([]any)
			if !ok {
				return fmt.Errorf("--file object must have a 'changes' array")
			}
			changes = c
		default:
			return fmt.Errorf("--file must be an array or an object with a 'changes' array")
		}
		if len(changes) == 0 {
			return &UsageError{Arg: "file", Usage: "batch must contain at least one change"}
		}
		if len(changes) > batchMaxItems {
			return &UsageError{
				Arg:   "file",
				Usage: fmt.Sprintf("batch size %d exceeds server max of %d — split into multiple batches", len(changes), batchMaxItems),
			}
		}

		status, body, err := client.PostRaw(
			"/abstractions/"+args[0]+"/changes/batch",
			map[string]any{"changes": changes},
		)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			return renderBatchSuccess(body, len(changes))
		}
		return renderBatchFailure(status, body)
	},
}

// renderBatchSuccess prints the created changes compactly in TTY mode and emits
// the full envelope in structured mode. The server preserves input order.
func renderBatchSuccess(body []byte, inputCount int) error {
	if !printer.IsStructured() {
		var resp struct {
			Data []abstractionChange `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("parsing batch response: %w", err)
		}
		headers := []string{"Index", "ID", "Action", "Target", "Field", "State"}
		rows := make([][]string, len(resp.Data))
		for i, c := range resp.Data {
			target := c.TargetType
			if c.TargetID != nil {
				target = fmt.Sprintf("%s #%d", c.TargetType, *c.TargetID)
			}
			rows[i] = []string{
				strconv.Itoa(i),
				strconv.Itoa(c.ID),
				c.Action,
				target,
				deref(c.TargetField),
				c.State,
			}
		}
		printer.Table(headers, rows)
	}
	printer.Summary(fmt.Sprintf("Created %d changes in one batch (one commit, coalesced broadcasts)", inputCount))
	printer.FinishRaw(body)
	return nil
}

// renderBatchFailure parses the per-item error shape and surfaces it as a
// targeted APIError. TTY mode prints a table of (index, errors) before the
// command exits. Structured modes get the raw body unchanged.
func renderBatchFailure(status int, body []byte) error {
	var resp struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Code   string `json:"code"`
		Errors []struct {
			Index  int      `json:"index"`
			Code   string   `json:"code"`
			Errors []string `json:"errors"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Couldn't parse — fall back to a generic APIError with raw body as message.
		return &api.APIError{StatusCode: status, Message: string(body)}
	}
	if !printer.IsStructured() && len(resp.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "Batch rolled back — per-item errors:")
		for _, item := range resp.Errors {
			codeStr := ""
			if item.Code != "" {
				codeStr = " (" + item.Code + ")"
			}
			fmt.Fprintf(os.Stderr, "  [%d]%s %s\n", item.Index, codeStr, strings.Join(item.Errors, "; "))
		}
		// Surface targeted hints for the most actionable codes. Fire at most
		// once per code per batch — a flood of identical hints is noise.
		seenCodes := map[string]bool{}
		for _, item := range resp.Errors {
			if item.Code == "" || seenCodes[item.Code] {
				continue
			}
			seenCodes[item.Code] = true
			switch item.Code {
			case "payload_extra_keys":
				fmt.Fprintln(os.Stderr, "  hint: one payload key per change, always. Split multi-key items into separate batch entries.")
			case "target_field_required":
				fmt.Fprintln(os.Stderr, "  hint: every item needs target_field explicitly set.")
			case "channel_locked":
				fmt.Fprintln(os.Stderr, "  hint: a web_ui change already holds the pending slot for at least one of these fields. Reject it in the web UI before retrying.")
			case "sub_object_group_not_allowed":
				fmt.Fprintln(os.Stderr, "  hint: Property/Lease creates must not carry sub_object_group. Remove it from the offending items.")
			case "sub_object_group_required":
				fmt.Fprintln(os.Stderr, "  hint: sub-object creates need a server-minted group. Mint via `kestrel abstractions changes new-group <abs-id> --target-type <T>` and embed the UUID in each batch item.")
			case "sub_object_group_unknown":
				fmt.Fprintln(os.Stderr, "  hint: at least one sub_object_group UUID wasn't minted on this abstraction. Re-mint with `changes new-group` and update the batch JSON.")
			case "sub_object_group_target_type_mismatch":
				fmt.Fprintln(os.Stderr, "  hint: a group UUID in the batch was minted for a different target_type than the change it's stamped on. Mint one group per target_type and per instance.")
			}
		}
	}
	// Flatten per-item errors into a single []string for APIError so existing
	// structured-error rendering keeps working. The raw body is richer — agents
	// running with --json/--agent get the full per-item shape via stdout.
	flat := make([]string, 0, len(resp.Errors))
	for _, item := range resp.Errors {
		for _, msg := range item.Errors {
			flat = append(flat, fmt.Sprintf("[%d] %s", item.Index, msg))
		}
	}
	msg := resp.Error
	if msg == "" {
		msg = fmt.Sprintf("batch rejected with %d per-item errors", len(resp.Errors))
	}
	code := resp.Code
	if code == "" {
		code = "batch_rejected"
	}
	if printer.IsStructured() {
		fmt.Println(string(body))
	}
	return &api.APIError{StatusCode: status, Message: msg, Code: code, Errors: flat}
}

var newGroupTargetType string

var abstractionChangesNewGroupCmd = &cobra.Command{
	Use:   "new-group <abstraction-id>",
	Short: "Mint a server-issued sub_object_group UUID for a new sub-object instance",
	Long: `Creates a new sub_object_group on the abstraction and returns its UUID.
Every sub-object instance (a new KeyDate, Expense, LeaseClause, etc.) is
identified by a group UUID that must be minted server-side before any field
changes reference it. Client-generated UUIDs are rejected.

For typical single-change drafting, 'kestrel abstractions changes create
--sub-object-group new' mints transparently under the hood. Use this
command when composing a batch file or any flow where you need the UUID
up-front.

TTY output: prints the UUID alone on stdout (easy to capture with $(...)).
Structured output: full {sub_object_group, target_type} envelope.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if newGroupTargetType == "" {
			return &UsageError{
				Arg:   "target-type",
				Usage: "kestrel abstractions changes new-group <abs-id> --target-type KeyDate|Expense|LeaseClause|...",
			}
		}
		uuid, err := mintSubObjectGroup(args[0], newGroupTargetType)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			// Bare UUID on stdout so shell users can capture it with $(...).
			fmt.Println(uuid)
			printer.Success(fmt.Sprintf("Minted sub_object_group %s for %s on abstraction #%s", uuid, newGroupTargetType, args[0]))
		}
		// Build a minimal envelope for --json/--agent consumers — the
		// server's /create_sub_object_group response shape passes straight
		// through without transformation.
		env := map[string]any{
			"ok": true,
			"data": map[string]string{
				"sub_object_group": uuid,
				"target_type":      newGroupTargetType,
			},
		}
		printer.FinishEnvelope(env)
		return nil
	},
}

// mintSubObjectGroup calls the server's create_sub_object_group endpoint and
// returns the minted UUID. Shared by 'changes new-group' and by the
// transparent '--sub-object-group new' path on 'changes create'.
func mintSubObjectGroup(absID, targetType string) (string, error) {
	env, err := client.Post(
		"/abstractions/"+absID+"/changes/create_sub_object_group",
		map[string]any{"target_type": targetType},
	)
	if err != nil {
		return "", err
	}
	var payload struct {
		SubObjectGroup string `json:"sub_object_group"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		return "", fmt.Errorf("decoding mint response: %w", err)
	}
	if payload.SubObjectGroup == "" {
		return "", fmt.Errorf("mint response missing sub_object_group")
	}
	return payload.SubObjectGroup, nil
}

var abstractionChangesDeleteCmd = &cobra.Command{
	Use:   "delete <abstraction-id> <change-id>",
	Short: "Delete a staged change",
	Long: `Only API-sourced pending changes can be deleted. Rejected, approved,
and applied changes are terminal — they're part of the review trail and
revised_from chain, and can't be removed via the API.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		env, err := client.Delete("/abstractions/" + args[0] + "/changes/" + args[1])
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.StatusCode {
				case 403:
					printer.Errorf("delete refused — only API-sourced pending changes can be deleted")
				case 422:
					printer.Errorf("delete refused — change is in a terminal state (rejected/approved/applied); terminal records are preserved for audit and revised_from chains")
				}
			}
			return err
		}
		printer.Summary(fmt.Sprintf("Deleted change #%s", args[1]))
		printer.FinishEnvelope(env)
		return nil
	},
}

// summarizeLinkPreview condenses source_links_preview into one table cell.
// Shape: "doc#12 p3: 'Base Rent…' (+2)" — first fragment rendered inline,
// extra fragments summed into the +N tail. Keeps the list view scannable;
// full geometry is on the detailed show.
func summarizeLinkPreview(previews []abstractionLinkPreview) string {
	if len(previews) == 0 {
		return ""
	}
	fragCount := 0
	for _, p := range previews {
		fragCount += len(p.Fragments)
	}
	var head string
	for _, p := range previews {
		if len(p.Fragments) == 0 {
			continue
		}
		f := p.Fragments[0]
		snippet := truncate(f.CitedTextPreview, 40)
		head = fmt.Sprintf("doc#%d p%d: %q", p.DocumentID, f.PageNumber, snippet)
		break
	}
	if head == "" {
		// Cites exist but no fragments (rare — e.g. doc-only citations).
		head = fmt.Sprintf("doc#%d", previews[0].DocumentID)
	}
	if fragCount > 1 {
		return fmt.Sprintf("%s (+%d)", head, fragCount-1)
	}
	return head
}

// resolveCiteBlocks converts --cite-block specs into the source_links shape the
// API expects. Each spec is one of:
//
//	<block-id>
//	<block-id>:chars=<start>-<end>
//	<block-id>:cell=<row>,<col>
//
// For each block, we fetch GET /document_blocks/:id to learn which document it
// belongs to, then group fragments by document_id so the payload hits one
// source_link per document.
func resolveCiteBlocks(specs []string) ([]map[string]any, error) {
	byDoc := map[int][]map[string]any{}
	docOrder := []int{}
	for _, spec := range specs {
		frag, blockID, err := parseCiteBlockSpec(spec)
		if err != nil {
			return nil, err
		}
		docID, err := fetchBlockDocumentID(blockID)
		if err != nil {
			return nil, err
		}
		if _, seen := byDoc[docID]; !seen {
			docOrder = append(docOrder, docID)
		}
		byDoc[docID] = append(byDoc[docID], frag)
	}
	out := make([]map[string]any, 0, len(docOrder))
	for _, docID := range docOrder {
		out = append(out, map[string]any{
			"document_id": docID,
			"fragments":   byDoc[docID],
		})
	}
	return out, nil
}

func parseCiteBlockSpec(spec string) (map[string]any, int, error) {
	head, rest, _ := strings.Cut(spec, ":")
	blockID, err := strconv.Atoi(head)
	if err != nil || blockID <= 0 {
		return nil, 0, fmt.Errorf("--cite-block %q: block id must be a positive integer", spec)
	}
	frag := map[string]any{"document_block_id": blockID}
	if rest == "" {
		return frag, blockID, nil
	}
	kind, value, ok := strings.Cut(rest, "=")
	if !ok {
		return nil, 0, fmt.Errorf("--cite-block %q: narrowing must be chars=S-E or cell=R,C", spec)
	}
	switch kind {
	case "chars":
		startStr, endStr, ok := strings.Cut(value, "-")
		if !ok {
			return nil, 0, fmt.Errorf("--cite-block %q: chars=S-E (e.g. chars=14-72)", spec)
		}
		start, errS := strconv.Atoi(startStr)
		end, errE := strconv.Atoi(endStr)
		if errS != nil || errE != nil || start < 0 || end < start {
			return nil, 0, fmt.Errorf("--cite-block %q: chars range must be non-negative with end ≥ start", spec)
		}
		frag["char_start"] = start
		frag["char_end"] = end
	case "cell":
		rowStr, colStr, ok := strings.Cut(value, ",")
		if !ok {
			return nil, 0, fmt.Errorf("--cite-block %q: cell=R,C (e.g. cell=2,1)", spec)
		}
		row, errR := strconv.Atoi(rowStr)
		col, errC := strconv.Atoi(colStr)
		if errR != nil || errC != nil || row < 0 || col < 0 {
			return nil, 0, fmt.Errorf("--cite-block %q: cell row/col must be non-negative integers", spec)
		}
		frag["table_cell_row"] = row
		frag["table_cell_col"] = col
	default:
		return nil, 0, fmt.Errorf("--cite-block %q: unknown narrowing %q (want chars= or cell=)", spec, kind)
	}
	return frag, blockID, nil
}

// fetchBlockDocumentID resolves the owning document's org_object_id for a block.
// Uses the block endpoint rather than caching — blocks are small and one extra
// call per cite-block spec is a reasonable price for a clean shortcut.
func fetchBlockDocumentID(blockID int) (int, error) {
	env, err := client.Get("/document_blocks/"+strconv.Itoa(blockID), nil)
	if err != nil {
		return 0, fmt.Errorf("resolving document for block %d: %w", blockID, err)
	}
	var b documentBlock
	if err := json.Unmarshal(env.Data, &b); err != nil {
		return 0, fmt.Errorf("decoding block %d: %w", blockID, err)
	}
	if b.DocumentID == 0 {
		return 0, fmt.Errorf("block %d response missing document_id", blockID)
	}
	return b.DocumentID, nil
}

// hintChangeCreateError inspects a POST /changes error and prints a targeted
// stderr hint before the structured error propagates. Matches the pattern
// used on add-doc / remove-doc. Quiet in --agent / --json mode; those paths
// already carry `code` and `error` on the envelope.
func hintChangeCreateError(err error) {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return
	}
	switch apiErr.Code {
	case "payload_extra_keys":
		printer.Errorf("payload rejected — one payload key per change, always. The key must match target_field exactly.")
	case "target_field_required":
		printer.Errorf("target_field missing — every create/update must specify --target-field (or let the CLI pass it via --payload key).")
	case "channel_locked":
		printer.Errorf("channel_locked — a web_ui change already holds this field's pending slot. Reject it in the web UI (or have the drafter cancel) before POSTing an api replacement.")
	case "sub_object_group_not_allowed":
		printer.Errorf("sub_object_group not allowed — Property/Lease creates don't carry a group. Drop --sub-object-group from this change.")
	case "sub_object_group_required":
		printer.Errorf("sub_object_group required — sub-object creates need a server-minted group. Pass --sub-object-group new (auto-mints), or `kestrel abstractions changes new-group <abs-id> --target-type <T>` to mint explicitly.")
	case "sub_object_group_unknown":
		printer.Errorf("sub_object_group_unknown — this UUID wasn't minted on this abstraction. Client-generated UUIDs are rejected; use --sub-object-group new to mint one.")
	case "sub_object_group_target_type_mismatch":
		printer.Errorf("sub_object_group_target_type_mismatch — this group was minted for a different target_type. Mint a fresh group for the current target_type (--sub-object-group new).")
	}
}

// renderChangeResult prints a created/updated change.
func renderChangeResult(env *api.Envelope, isCreate bool) error {
	if env == nil || len(env.Data) == 0 {
		printer.FinishEnvelope(env)
		return nil
	}
	var c abstractionChange
	parseErr := json.Unmarshal(env.Data, &c)
	if !printer.IsStructured() {
		if parseErr != nil {
			return fmt.Errorf("parsing change: %w", parseErr)
		}
		target := c.TargetType
		if c.TargetID != nil {
			target = fmt.Sprintf("%s #%d", c.TargetType, *c.TargetID)
		}
		printer.Detail([][]string{
			{"ID", strconv.Itoa(c.ID)},
			{"Action", c.Action},
			{"Target", target},
			{"Field", deref(c.TargetField)},
			{"State", c.State},
			{"Source", c.Source},
			{"Sub-object group", deref(c.SubObjectGroup)},
		})
		// Echo the server-rendered anchor formula immediately when the change
		// carries a relative-date payload. The agent's fastest sanity check
		// on a freshly-drafted anchor — a malformed anchor that slipped past
		// validation renders as a partial formula (e.g. "30 days after .")
		// and the agent can re-draft within the same turn.
		if c.RelativeDateResolution != nil {
			renderRelativeDateResolution(c.RelativeDateResolution)
		}
	}
	if parseErr == nil {
		if isCreate {
			printer.Summary(fmt.Sprintf("Change #%d staged", c.ID))
		} else {
			printer.Summary(fmt.Sprintf("Change #%d updated", c.ID))
		}
	}
	printer.FinishEnvelope(env)
	return nil
}

// renderRelativeDateResolution prints the server-rendered formula and
// resolved value for a relative-date change. Both fields come straight
// from `relative_date_resolution` on the change response — the CLI does
// no interpretation of the formula content. If a malformed-anchor case
// reaches the response, the fix belongs in the server's validator or
// formula renderer, not in client-side string sniffing.
func renderRelativeDateResolution(r *relativeDateResolution) {
	if r.Formula == "" && r.ResolvedValue == nil {
		return
	}
	resolved := "(resolves once anchor is set)"
	if r.ResolvedValue != nil && *r.ResolvedValue != "" {
		resolved = *r.ResolvedValue
	}
	printer.Detail([][]string{
		{"Anchor", r.Formula},
		{"Resolves to", resolved},
	})
}

func init() {
	abstractionChangesListCmd.Flags().IntVar(&abstractionChangesListPage, "page", 1, "Page number")
	abstractionChangesListCmd.Flags().StringSliceVar(&abstractionChangesListStates, "state", nil, "Filter by state (repeatable): pending | approved | rejected | applied")

	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateAction, "action", "", "create | update | destroy (required)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateTargetType, "target-type", "", "Model being mutated, e.g. Property, Lease, KeyDate (required)")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateTargetID, "target-id", 0, "Target record ID (required for update/destroy)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateTargetField, "target-field", "", "Scalar field for update (inferred from payload if omitted)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateSubObjectGroup, "sub-object-group", "", "UUID grouping sibling field changes, or 'new' to mint one")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateParentChangeID, "parent-change-id", 0, "Parent change this one depends on")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateRevisedFromID, "revised-from-id", 0, "Change ID this one supersedes (auto-linked to rejected predecessors if omitted)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreatePayloadInput, "payload", "", `Attribute payload as JSON, @file, or - for stdin (required unless --anchor is used)`)
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateSourceLinksInput, "source-links", "", `Source links array as JSON, @file, or - for stdin`)
	// StringArrayVar (not StringSliceVar) — these specs embed commas
	// (cell=R,C; comma-separated anchor key=values), and StringSliceVar
	// auto-splits on commas inside a single quoted value.
	abstractionChangesCreateCmd.Flags().StringArrayVar(&changesCreateCiteBlocks, "cite-block", nil, `Cite a parsed block. Repeatable. Formats: <block-id>, <block-id>:chars=S-E, <block-id>:cell=R,C`)
	abstractionChangesCreateCmd.Flags().StringArrayVar(&changesCreateAnchorSpecs, "anchor", nil, `Anchor spec for a relative-date payload. Repeatable; pass once per anchor (commas inside one --anchor are preserved). Format: target_type=...,target_field=...,[target_id=N|sub_object_group=UUID,]offset_months=N,offset_days=N,inclusive=true|false. Mutually exclusive with --payload; requires --target-field.`)
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateAnchorResolution, "anchor-resolution", "", `earliest_of|latest_of (defaults to earliest_of for single-anchor; required for multi-anchor)`)
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateProvisionalDate, "provisional-date", "", `Override the CLI-computed provisional_date (YYYY-MM-DD)`)

	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdatePayloadInput, "payload", "", `New payload as JSON, @file, or -`)
	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdateSourceLinksInput, "source-links", "", `New source links as JSON, @file, or - (pass '[]' to clear)`)

	abstractionChangesBatchCmd.Flags().StringVar(&batchCreateFileInput, "file", "", "Batch payload as JSON array, @file, or - for stdin (required)")

	abstractionChangesNewGroupCmd.Flags().StringVar(&newGroupTargetType, "target-type", "", "Sub-object target_type (KeyDate, Expense, LeaseClause, …) (required)")

	abstractionChangesCmd.AddCommand(abstractionChangesListCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesShowCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesCreateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesBatchCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesNewGroupCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesUpdateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesDeleteCmd)
	abstractionsCmd.AddCommand(abstractionChangesCmd)
}
