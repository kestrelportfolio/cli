package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

type abstractionChange struct {
	ID              int             `json:"id"`
	Action          string          `json:"action"`
	TargetType      string          `json:"target_type"`
	TargetField     *string         `json:"target_field"`
	TargetID        *int            `json:"target_id"`
	Payload         json.RawMessage `json:"payload"`
	State           string          `json:"state"`
	Source          string          `json:"source"`
	SubObjectGroup  *string         `json:"sub_object_group"`
	ParentChangeID  *int            `json:"parent_change_id"`
	RevisedFromID   *int            `json:"revised_from_id"`
	OwnerID         *int            `json:"owner_id"`
	ReviewedByID    *int            `json:"reviewed_by_id"`
	ReviewedAt      *string         `json:"reviewed_at"`
	AppliedAt       *string         `json:"applied_at"`
	RejectionReason *string         `json:"rejection_reason"`
	FieldMetadata   json.RawMessage `json:"field_metadata,omitempty"`
	SourceLinks     json.RawMessage `json:"source_links,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

var abstractionChangesCmd = &cobra.Command{
	Use:     "changes",
	Aliases: []string{"change"},
	Short:   "Draft, inspect, and manage per-field changes on an abstraction",
}

var abstractionChangesListPage int
var abstractionChangesListCmd = &cobra.Command{
	Use:   "list <abstraction-id>",
	Short: "List staged changes on an abstraction",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if abstractionChangesListPage > 1 {
			params["page"] = strconv.Itoa(abstractionChangesListPage)
		}
		raw, err := client.GetRaw("/abstractions/"+args[0]+"/changes", params)
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
			headers := []string{"ID", "Action", "Target", "Field", "State", "Source", "Created"}
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

Examples:
  # Scalar update — target_field is inferred from payload keys
  kestrel abstractions changes create 42 \
    --action update --target-type Lease --target-id 7 \
    --payload '{"name":"Acme Lease"}'

  # Sub-object create (new KeyDate) — mint a group UUID, cite a source doc
  kestrel abstractions changes create 42 \
    --action create --target-type KeyDate \
    --sub-object-group new \
    --payload '{"name":"Expiration","date":"2030-12-31"}' \
    --source-links '[{"document_id":87}]'`,
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

		// source_links is optional.
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
			if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
				printer.Errorf("update refused — only API-sourced changes in pending/rejected state can be edited via the API")
			}
			return err
		}
		return renderChangeResult(env, false)
	},
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

	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateAction, "action", "", "create | update | destroy (required)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateTargetType, "target-type", "", "Model being mutated, e.g. Property, Lease, KeyDate (required)")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateTargetID, "target-id", 0, "Target record ID (required for update/destroy)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateTargetField, "target-field", "", "Scalar field for update (inferred from payload if omitted)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateSubObjectGroup, "sub-object-group", "", "UUID grouping sibling field changes, or 'new' to mint one")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateParentChangeID, "parent-change-id", 0, "Parent change this one depends on")
	abstractionChangesCreateCmd.Flags().IntVar(&changesCreateRevisedFromID, "revised-from-id", 0, "Change ID this one supersedes (auto-linked to rejected predecessors if omitted)")
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreatePayloadInput, "payload", "", `Attribute payload as JSON, @file, or - for stdin (required)`)
	abstractionChangesCreateCmd.Flags().StringVar(&changesCreateSourceLinksInput, "source-links", "", `Source links array as JSON, @file, or - for stdin`)

	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdatePayloadInput, "payload", "", `New payload as JSON, @file, or -`)
	abstractionChangesUpdateCmd.Flags().StringVar(&changesUpdateSourceLinksInput, "source-links", "", `New source links as JSON, @file, or - (pass '[]' to clear)`)

	abstractionChangesCmd.AddCommand(abstractionChangesListCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesShowCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesCreateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesUpdateCmd)
	abstractionChangesCmd.AddCommand(abstractionChangesDeleteCmd)
	abstractionsCmd.AddCommand(abstractionChangesCmd)
}
