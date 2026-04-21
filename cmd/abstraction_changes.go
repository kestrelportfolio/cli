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
	ID                 int                      `json:"id"`
	Action             string                   `json:"action"`
	TargetType         string                   `json:"target_type"`
	TargetField        *string                  `json:"target_field"`
	TargetID           *int                     `json:"target_id"`
	Payload            json.RawMessage          `json:"payload"`
	State              string                   `json:"state"`
	Source             string                   `json:"source"`
	SubObjectGroup     *string                  `json:"sub_object_group"`
	ParentChangeID     *int                     `json:"parent_change_id"`
	RevisedFromID      *int                     `json:"revised_from_id"`
	OwnerID            *int                     `json:"owner_id"`
	ReviewedByID       *int                     `json:"reviewed_by_id"`
	ReviewedAt         *string                  `json:"reviewed_at"`
	AppliedAt          *string                  `json:"applied_at"`
	RejectionReason    *string                  `json:"rejection_reason"`
	FieldMetadata      json.RawMessage          `json:"field_metadata,omitempty"`
	SourceLinks        json.RawMessage          `json:"source_links,omitempty"`
	SourceLinksPreview []abstractionLinkPreview `json:"source_links_preview,omitempty"`
	CreatedAt          string                   `json:"created_at"`
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
	changesCreateAction           string
	changesCreateTargetType       string
	changesCreateTargetID         int
	changesCreateTargetField      string
	changesCreateSubObjectGroup   string
	changesCreateParentChangeID   int
	changesCreateRevisedFromID    int
	changesCreatePayloadInput     string
	changesCreateSourceLinksInput string
	changesCreateCiteBlocks       []string
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
    --source-links '[{"document_id":87,"fragments":[{"page_number":3,"x":0.1,"y":0.2,"width":0.3,"height":0.05}]}]'`,
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

		// sub_object_group: "new" → mint a UUID, otherwise pass through literal.
		mintedUUID := ""
		if changesCreateSubObjectGroup != "" {
			if changesCreateSubObjectGroup == "new" {
				u, err := newUUIDv4()
				if err != nil {
					return fmt.Errorf("minting UUID: %w", err)
				}
				mintedUUID = u
				change["sub_object_group"] = u
			} else {
				change["sub_object_group"] = changesCreateSubObjectGroup
			}
		}

		// Payload is required — parse input into a generic JSON value.
		payloadStr, err := readInputValue(changesCreatePayloadInput)
		if err != nil {
			return err
		}
		if payloadStr == "" {
			return &UsageError{Arg: "payload", Usage: "--payload '{\"field\":\"value\"}' or --payload @file.json or --payload -"}
		}
		var payload any
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			return fmt.Errorf("parsing --payload as JSON: %w", err)
		}
		change["payload"] = payload

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
			merged, count := mergeSourceLinks(links)
			if count > 0 {
				printer.Breadcrumb(fmt.Sprintf("Merged %d duplicate source_links entry/entries into one per document_id", count))
			}
			change["source_links"] = merged
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
	Long: `Only changes authored via the API (source: "api") in pending/rejected state
can be updated. Omit --source-links to leave citations untouched; pass an
empty array (--source-links '[]') to clear them.`,
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
			merged, count := mergeSourceLinks(links)
			if count > 0 {
				printer.Breadcrumb(fmt.Sprintf("Merged %d duplicate source_links entry/entries into one per document_id", count))
			}
			change["source_links"] = merged
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
			if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
				printer.Errorf("update refused — only API-sourced changes in pending/rejected state can be edited via the API")
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

		// Pre-merge duplicate source_links per item so agents don't trip the
		// server's "Linkable is already linked" rejection — which rolls back
		// the whole batch.
		totalMerges := 0
		for _, item := range changes {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if links, exists := m["source_links"]; exists {
				merged, count := mergeSourceLinks(links)
				if count > 0 {
					m["source_links"] = merged
					totalMerges += count
				}
			}
		}
		if totalMerges > 0 {
			printer.Breadcrumb(fmt.Sprintf("Merged %d duplicate source_links entry/entries across batch items", totalMerges))
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
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", item.Index, strings.Join(item.Errors, "; "))
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

var abstractionChangesDeleteCmd = &cobra.Command{
	Use:   "delete <abstraction-id> <change-id>",
	Short: "Delete a staged change",
	Long:  `Only API-sourced changes in pending/rejected state can be deleted.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		env, err := client.Delete("/abstractions/" + args[0] + "/changes/" + args[1])
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
				printer.Errorf("delete refused — only API-sourced changes in pending/rejected state can be deleted")
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

// mergeSourceLinks collapses duplicate document_id entries into one, with the
// fragments arrays concatenated in input order. The server rejects a batch
// when multiple source_links entries cite the same document ("Linkable is
// already linked"), so agents writing {doc_id: 1, fragments: [A]} and
// {doc_id: 1, fragments: [B]} separately would otherwise hit 422.
//
// Returns (merged, mergedCount). mergedCount > 0 means duplicates were
// collapsed; the caller can breadcrumb a warning.
func mergeSourceLinks(raw any) (any, int) {
	arr, ok := raw.([]any)
	if !ok {
		return raw, 0
	}
	byDoc := map[int]map[string]any{}
	docOrder := []int{}
	mergeCount := 0
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return raw, 0
		}
		docIDRaw, exists := m["document_id"]
		if !exists {
			return raw, 0
		}
		docID, ok := coerceInt(docIDRaw)
		if !ok {
			return raw, 0
		}
		if existing, seen := byDoc[docID]; seen {
			mergeCount++
			efrags, _ := existing["fragments"].([]any)
			nfrags, _ := m["fragments"].([]any)
			existing["fragments"] = append(efrags, nfrags...)
			continue
		}
		// Shallow-copy so we don't mutate the caller's input when we
		// later append fragments.
		copied := map[string]any{}
		for k, v := range m {
			copied[k] = v
		}
		if frags, ok := copied["fragments"].([]any); ok {
			dup := make([]any, len(frags))
			copy(dup, frags)
			copied["fragments"] = dup
		}
		byDoc[docID] = copied
		docOrder = append(docOrder, docID)
	}
	if mergeCount == 0 {
		return raw, 0
	}
	out := make([]any, 0, len(docOrder))
	for _, docID := range docOrder {
		out = append(out, byDoc[docID])
	}
	return out, mergeCount
}

// coerceInt handles the two shapes a numeric JSON value takes in an
// any-typed map: float64 (default unmarshal) or json.Number (decoder with
// UseNumber). Neither is guaranteed by our input, so accept both.
func coerceInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
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

// renderChangeResult prints a created/updated change.
func renderChangeResult(env *api.Envelope, isCreate bool) error {
	if env == nil || len(env.Data) == 0 {
		printer.FinishEnvelope(env)
		return nil
	}
	if !printer.IsStructured() {
		var c abstractionChange
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return fmt.Errorf("parsing change: %w", err)
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
	}
	// Pull ID for summary (both modes want it).
	var c abstractionChange
	if err := json.Unmarshal(env.Data, &c); err == nil {
		if isCreate {
			printer.Summary(fmt.Sprintf("Change #%d staged", c.ID))
		} else {
			printer.Summary(fmt.Sprintf("Change #%d updated", c.ID))
		}
	}
	printer.FinishEnvelope(env)
	return nil
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
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreatePayloadInput, "payload", "", `Attribute payload as JSON, @file, or - for stdin (required)`)
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateSourceLinksInput, "source-links", "", `Source links array as JSON, @file, or - for stdin`)
	abstractionChangesCreateCmd.Flags().StringSliceVar(&changesCreateCiteBlocks, "cite-block", nil, `Cite a parsed block. Repeatable. Formats: <block-id>, <block-id>:chars=S-E, <block-id>:cell=R,C`)

	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdatePayloadInput, "payload", "", `New payload as JSON, @file, or -`)
	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdateSourceLinksInput, "source-links", "", `New source links as JSON, @file, or - (pass '[]' to clear)`)

	abstractionChangesBatchCmd.Flags().StringVar(&batchCreateFileInput, "file", "", "Batch payload as JSON array, @file, or - for stdin (required)")

	abstractionChangesCmd.AddCommand(abstractionChangesListCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesShowCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesCreateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesBatchCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesUpdateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesDeleteCmd)
	abstractionsCmd.AddCommand(abstractionChangesCmd)
}
