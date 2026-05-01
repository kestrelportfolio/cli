package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var abstractionAnchorableDatesCmd = &cobra.Command{
	Use:   "anchorable-dates <abstraction-id>",
	Short: "List valid anchor refs for relative-date payloads on this abstraction",
	Long: `Returns the dynamic list of anchor options that can be used when
constructing a relative-date payload for a change in this abstraction —
combining live DateEntries on the target Property/Lease, sibling draft
sub_object_groups in the same abstraction, and the org's primary-target
Lease anchors. Empty when ` + "`date_dependencies_enabled`" + ` is off for the org.

Each entry is a valid anchor ref the agent can drop straight into a
` + "`--anchor`" + ` flag (or RelativeDateAnchor.anchor in raw JSON):

  - ` + "`target_type`" + ` and ` + "`target_field`" + ` — always present.
  - ` + "`target_id`" + ` — live record (kind: live).
  - ` + "`sub_object_group`" + ` — sibling draft (kind: draft).
  - both nil — primary-target Lease/Property (kind: primary_target_*).

Example:

  kestrel abstractions anchorable-dates 42 --agent
  # → {"anchors": [
  #     {"kind":"primary_target_live","target_type":"Lease","target_field":"start_date",
  #      "label":"Lease: Start Date","current_value":"2026-01-01"},
  #     {"kind":"draft","target_type":"KeyDate","target_field":"date",
  #      "sub_object_group":"e8c2…","label":"Lease Expiration","current_value":null,"state":"pending"},
  #     ...
  #   ]}`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/abstractions/"+args[0]+"/anchorable_dates", nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data struct {
					Anchors []anchorOption `json:"anchors"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			anchors := resp.Data.Anchors
			if len(anchors) == 0 {
				printer.Errorf("No anchorable dates returned. The org may not have date_dependencies enabled, or this abstraction has no eligible anchors yet.")
			} else {
				headers := []string{"Kind", "Type", "Field", "ID/Group", "Current value", "State", "Label"}
				rows := make([][]string, len(anchors))
				for i, a := range anchors {
					rows[i] = []string{
						a.Kind,
						a.TargetType,
						a.TargetField,
						anchorRefIDOrGroup(a),
						deref(a.CurrentValue),
						deref(a.State),
						a.Label,
					}
				}
				printer.Table(headers, rows)
				printer.Breadcrumb(fmt.Sprintf("Use as --anchor on `kestrel abstractions changes create %s` or `kestrel abstractions increase create %s`", args[0], args[0]))
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

// anchorRefIDOrGroup formats an AnchorOption's identifying ref for display.
// Live entries show their numeric id; draft entries show a short prefix of the
// sub_object_group UUID; primary-target entries show "—".
func anchorRefIDOrGroup(a anchorOption) string {
	if a.TargetID != nil {
		return "#" + strconv.Itoa(*a.TargetID)
	}
	if a.SubObjectGroup != nil {
		s := *a.SubObjectGroup
		if len(s) > 8 {
			return s[:8] + "…"
		}
		return s
	}
	return "—"
}

func init() {
	abstractionsCmd.AddCommand(abstractionAnchorableDatesCmd)
}
