package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// `kestrel abstractions increase ...` is the dedicated write surface for
// Increase abstraction changes. Increase is the API's first opaque target
// type (see abstractions.md → "Opaque target types — series-grained changes
// (Increase)") — one change carries the whole series spec as a multi-key
// payload with `target_field: null`. The generic `changes create` flow could
// technically accept the same opaque payload, but every assistive layer
// fights you (single-key validation, target_field errors, no tier helpers),
// so we route increase authoring through this dedicated command.
var abstractionIncreaseCmd = &cobra.Command{
	Use:     "increase",
	Aliases: []string{"increases"},
	Short:   "Author increase (rent escalation) abstraction changes",
	Long: `An increase models a rent escalation or other periodic amount change on
an Expense (or LeaseSecurity, in a future API revision). Three increase
types are supported:

  fixed       — the amount is set to a new absolute value on effective_date
  percentage  — a % step on the previously-resolved amount
  indexation  — CPI-linked: derived from the change in an inflation index
                between two observation periods, optionally clamped by floor
                and ceiling and shaped by tier rules

Increase is an opaque abstraction target type. One change carries the whole
series; the API accepts a multi-key payload with target_field set to null.`,
}

var (
	increaseCreateIncreasableSubObjectGroup string
	increaseCreateIncreasableTargetID       int
	increaseCreateType                      string
	increaseCreateEffectiveDate             string
	increaseCreateAnchorSpecs               []string
	increaseCreateAnchorResolution          string
	increaseCreateProvisionalDate           string
	increaseCreateFixedAmount               string
	increaseCreatePercentage                string
	increaseCreateInflationSeriesID         int
	increaseCreateStartPeriod               string
	increaseCreateEndPeriod                 string
	increaseCreateFloor                     string
	increaseCreateCeiling                   string
	increaseCreateTiers                     []string
	increaseCreateRecurrenceCadence         string
	increaseCreateRecurrenceIntervalAmount  int
	increaseCreateRecurrenceIntervalUnit    string
	increaseCreateNotes                     string
	increaseCreateSourceLinksInput          string
	increaseCreateCiteBlocks                []string
	increaseCreateParentChangeID            int
	increaseCreateRevisedFromID             int
)

var abstractionIncreaseCreateCmd = &cobra.Command{
	Use:   "create <abstraction-id>",
	Short: "Draft an Increase change on an abstraction",
	Long: `Builds the opaque Increase payload and POSTs it as
` + "`action=create, target_type=Increase, target_field=null`" + ` against the
abstraction. Dedup, channel-lock, and revised_from auto-link semantics are
identical to a generic ` + "`changes create`" + `.

Required:
  --type fixed|percentage|indexation
  exactly one of:
    --increasable-sub-object-group <uuid>   (parent Expense's draft group)
    --increasable-target-id <expense-id>    (brownfield: existing live Expense)
  exactly one of:
    --effective-date YYYY-MM-DD             (absolute)
    --anchor "<spec>" + --anchor-resolution + (optional) --provisional-date
  one source-document citation (--cite-block or --source-links)

Type-specific:
  fixed:       --fixed-amount <decimal>
  percentage:  --percentage <decimal>
  indexation:  --inflation-series-id <id>
               --start-period YYYY-MM | YYYY-Q[1-4]
               --end-period   YYYY-MM | YYYY-Q[1-4]
               [--floor <decimal>]    [--ceiling <decimal>]
               [--tier "<lo>:<hi>:<rate>" ...]   (repeatable; lo/hi blank = open-ended)

Recurrence (optional, expanded at go-live):
  --recurrence-cadence lease_anniversary|expense_start_anniversary|january_1|custom
  --recurrence-interval-amount N --recurrence-interval-unit years|quarters|months
                                          (custom cadence only)

Examples:

  # Greenfield draft Expense — recurring CPI escalation, anchored to lease start
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

  # Brownfield existing Expense — fixed bump on a known date
  kestrel abstractions increase create 42 \
    --increasable-target-id 17 \
    --type fixed --fixed-amount 12500 \
    --effective-date 2027-04-01 \
    --cite-block 4980

Tier specs (--tier):
  ":4:100"   open-ended below, capped at 4%, applied at 100%
  "4:8:50"   between 4% and 8%, applied at 50%
  "8::25"    above 8%, applied at 25%

Tier validation runs locally before the round-trip:
  • lo<hi within each tier (or blank = open-ended)
  • contiguous: each tier's lo must equal previous tier's hi
  • last tier's hi must be blank (open above)
  • application_rate in [0, 100]

Empty/zero tiers means 100% pass-through of the index change.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		absID := args[0]

		if increaseCreateType == "" {
			return &UsageError{Arg: "type", Usage: "kestrel abstractions increase create <abs-id> --type fixed|percentage|indexation ..."}
		}
		if increaseCreateType != "fixed" && increaseCreateType != "percentage" && increaseCreateType != "indexation" {
			return &UsageError{Arg: "type", Usage: "--type must be one of fixed|percentage|indexation"}
		}

		// Increasable parent — exactly one of group/id.
		if increaseCreateIncreasableSubObjectGroup == "" && increaseCreateIncreasableTargetID <= 0 {
			return &UsageError{
				Arg:   "increasable-sub-object-group or increasable-target-id",
				Usage: "exactly one of --increasable-sub-object-group <uuid> (greenfield/draft expense) OR --increasable-target-id <id> (brownfield/live expense)",
			}
		}
		if increaseCreateIncreasableSubObjectGroup != "" && increaseCreateIncreasableTargetID > 0 {
			return &UsageError{
				Arg:   "increasable-sub-object-group and increasable-target-id",
				Usage: "pass exactly one of --increasable-sub-object-group OR --increasable-target-id, not both",
			}
		}

		// Effective date — absolute or anchored, mutually exclusive.
		hasAnchor := len(increaseCreateAnchorSpecs) > 0
		if hasAnchor && increaseCreateEffectiveDate != "" {
			return &UsageError{
				Arg:   "anchor and effective-date",
				Usage: "pass --effective-date (absolute) OR --anchor (relative), not both",
			}
		}
		if !hasAnchor && increaseCreateEffectiveDate == "" {
			return &UsageError{
				Arg:   "effective-date or anchor",
				Usage: "pass --effective-date YYYY-MM-DD OR one or more --anchor specs",
			}
		}

		// Source document citation is required (matches the generic API rule).
		if increaseCreateSourceLinksInput == "" && len(increaseCreateCiteBlocks) == 0 {
			return &UsageError{
				Arg:   "cite-block or source-links",
				Usage: "Increase changes must cite at least one source document — pass --cite-block or --source-links",
			}
		}
		if increaseCreateSourceLinksInput != "" && len(increaseCreateCiteBlocks) > 0 {
			return &UsageError{
				Arg:   "cite-block and source-links",
				Usage: "pass --cite-block OR --source-links, not both",
			}
		}

		// Build the opaque Increase payload.
		payload := map[string]any{
			"increase_type": increaseCreateType,
		}

		// Parent linkage. Greenfield draft: pass the Expense's sub_object_group.
		// Brownfield live: pass increasable_target_id (the applier resolves it).
		if increaseCreateIncreasableSubObjectGroup != "" {
			payload["increasable_sub_object_group"] = increaseCreateIncreasableSubObjectGroup
		}
		if increaseCreateIncreasableTargetID > 0 {
			payload["increasable_target_id"] = increaseCreateIncreasableTargetID
		}

		// Effective date — relative or absolute.
		var inferredProvisional string
		var didInfer bool
		if hasAnchor {
			anchors := make([]*anchorSpec, 0, len(increaseCreateAnchorSpecs))
			for _, raw := range increaseCreateAnchorSpecs {
				spec, err := parseAnchorSpec(raw)
				if err != nil {
					return err
				}
				anchors = append(anchors, spec)
			}
			rel, provisional, inferred, err := buildRelativePayload(absID, increaseCreateAnchorResolution, anchors, increaseCreateProvisionalDate)
			if err != nil {
				return err
			}
			payload["effective_date"] = rel
			inferredProvisional = provisional
			didInfer = inferred
		} else {
			payload["effective_date"] = increaseCreateEffectiveDate
		}

		// Type-specific fields.
		switch increaseCreateType {
		case "fixed":
			if increaseCreateFixedAmount == "" {
				return &UsageError{Arg: "fixed-amount", Usage: "--type fixed requires --fixed-amount <decimal>"}
			}
			if err := validateDecimal("fixed-amount", increaseCreateFixedAmount); err != nil {
				return err
			}
			payload["fixed_amount"] = increaseCreateFixedAmount
			if increaseCreatePercentage != "" || increaseCreateInflationSeriesID > 0 {
				return &UsageError{Arg: "type fixed", Usage: "--type fixed accepts only --fixed-amount; drop --percentage / indexation flags"}
			}
		case "percentage":
			if increaseCreatePercentage == "" {
				return &UsageError{Arg: "percentage", Usage: "--type percentage requires --percentage <decimal>"}
			}
			if err := validateDecimal("percentage", increaseCreatePercentage); err != nil {
				return err
			}
			payload["percentage_value"] = increaseCreatePercentage
			if increaseCreateFixedAmount != "" || increaseCreateInflationSeriesID > 0 {
				return &UsageError{Arg: "type percentage", Usage: "--type percentage accepts only --percentage; drop --fixed-amount / indexation flags"}
			}
		case "indexation":
			if increaseCreateInflationSeriesID <= 0 {
				return &UsageError{Arg: "inflation-series-id", Usage: "--type indexation requires --inflation-series-id <id>"}
			}
			if increaseCreateStartPeriod == "" || increaseCreateEndPeriod == "" {
				return &UsageError{Arg: "start-period and end-period", Usage: "--type indexation requires --start-period and --end-period (YYYY-MM or YYYY-Q[1-4])"}
			}
			if err := validatePeriod("start-period", increaseCreateStartPeriod); err != nil {
				return err
			}
			if err := validatePeriod("end-period", increaseCreateEndPeriod); err != nil {
				return err
			}
			payload["inflation_series_id"] = increaseCreateInflationSeriesID
			payload["start_index_period"] = increaseCreateStartPeriod
			payload["end_index_period"] = increaseCreateEndPeriod
			if increaseCreateFloor != "" {
				if err := validateDecimal("floor", increaseCreateFloor); err != nil {
					return err
				}
				payload["floor_percentage"] = increaseCreateFloor
			}
			if increaseCreateCeiling != "" {
				if err := validateDecimal("ceiling", increaseCreateCeiling); err != nil {
					return err
				}
				payload["ceiling_percentage"] = increaseCreateCeiling
			}
			tiers, err := parseTierSpecs(increaseCreateTiers)
			if err != nil {
				return err
			}
			if len(tiers) > 0 {
				payload["indexation_tiers"] = tiers
			}
			if increaseCreateFixedAmount != "" || increaseCreatePercentage != "" {
				return &UsageError{Arg: "type indexation", Usage: "--type indexation accepts only indexation flags; drop --fixed-amount / --percentage"}
			}
		}

		// Recurrence (optional). Validation mirrors the live form's cadences.
		if increaseCreateRecurrenceCadence != "" {
			rec, err := buildRecurrencePayload()
			if err != nil {
				return err
			}
			payload["recurrence"] = rec
		}

		if increaseCreateNotes != "" {
			payload["notes"] = increaseCreateNotes
		}

		// Assemble the change. Increase is opaque: target_field stays null.
		change := map[string]any{
			"action":      "create",
			"target_type": "Increase",
			"payload":     payload,
		}
		if increaseCreateParentChangeID > 0 {
			change["parent_change_id"] = increaseCreateParentChangeID
		}
		if increaseCreateRevisedFromID > 0 {
			change["revised_from_id"] = increaseCreateRevisedFromID
		}

		// Source links — same handling as the generic create path.
		if increaseCreateSourceLinksInput != "" {
			slStr, err := readInputValue(increaseCreateSourceLinksInput)
			if err != nil {
				return err
			}
			var links any
			if err := json.Unmarshal([]byte(slStr), &links); err != nil {
				return fmt.Errorf("parsing --source-links as JSON: %w", err)
			}
			change["source_links"] = links
		}
		if len(increaseCreateCiteBlocks) > 0 {
			links, err := resolveCiteBlocks(increaseCreateCiteBlocks)
			if err != nil {
				return err
			}
			change["source_links"] = links
		}

		env, err := client.Post(
			"/abstractions/"+absID+"/changes",
			map[string]any{"change": change},
		)
		if err != nil {
			hintChangeCreateError(err)
			return err
		}
		if didInfer {
			printer.Breadcrumb(fmt.Sprintf("provisional_date inferred as %s — pass --provisional-date to override", inferredProvisional))
		}
		return renderChangeResult(env, true)
	},
}

// buildRecurrencePayload validates the cadence flags and returns the JSON
// shape the applier expects.
func buildRecurrencePayload() (map[string]any, error) {
	cadence := increaseCreateRecurrenceCadence
	switch cadence {
	case "lease_anniversary", "expense_start_anniversary", "january_1":
		// Annual cadences — interval flags should not be set.
		if increaseCreateRecurrenceIntervalAmount > 0 || increaseCreateRecurrenceIntervalUnit != "" {
			return nil, &UsageError{
				Arg:   "recurrence-interval-amount and recurrence-interval-unit",
				Usage: "interval flags are only valid with --recurrence-cadence custom",
			}
		}
		return map[string]any{"cadence": cadence}, nil
	case "custom":
		if increaseCreateRecurrenceIntervalAmount <= 0 {
			return nil, &UsageError{Arg: "recurrence-interval-amount", Usage: "--recurrence-cadence custom requires --recurrence-interval-amount > 0"}
		}
		switch increaseCreateRecurrenceIntervalUnit {
		case "years", "quarters", "months":
			// ok
		default:
			return nil, &UsageError{Arg: "recurrence-interval-unit", Usage: "--recurrence-interval-unit must be years|quarters|months"}
		}
		return map[string]any{
			"cadence":         "custom",
			"interval_amount": increaseCreateRecurrenceIntervalAmount,
			"interval_unit":   increaseCreateRecurrenceIntervalUnit,
		}, nil
	default:
		return nil, &UsageError{Arg: "recurrence-cadence", Usage: "--recurrence-cadence must be lease_anniversary|expense_start_anniversary|january_1|custom"}
	}
}

// parseTierSpecs converts repeated --tier "lo:hi:rate" into the
// indexation_tiers array shape. Validates contiguity, rate range, and the
// open-ended-below/above conventions locally so the user gets immediate
// feedback rather than a 422 round-trip.
func parseTierSpecs(specs []string) ([]map[string]any, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tiers := make([]map[string]any, 0, len(specs))
	var prevUpper *string
	for i, raw := range specs {
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("--tier %q: format is <lo>:<hi>:<rate> (use blank for open-ended bounds, e.g. \":4:100\")", raw)
		}
		loStr := strings.TrimSpace(parts[0])
		hiStr := strings.TrimSpace(parts[1])
		rateStr := strings.TrimSpace(parts[2])

		// First tier's lower_bound may be nil (open below) or bounded; interior
		// tiers must equal the previous tier's upper_bound.
		if i > 0 {
			if loStr == "" {
				return nil, fmt.Errorf("--tier %q: only the first tier may have an open lower bound; this tier #%d must equal the previous tier's upper bound", raw, i)
			}
			if prevUpper == nil || *prevUpper != loStr {
				return nil, fmt.Errorf("--tier %q: lower bound must equal previous tier's upper bound (got %q, expected %q)", raw, loStr, derefDefault(prevUpper, "<open>"))
			}
		}

		// Last tier's upper_bound must be nil (open above).
		if i == len(specs)-1 && hiStr != "" {
			return nil, fmt.Errorf("--tier %q: last tier must have an open upper bound (use \"%s::%s\" instead)", raw, loStr, rateStr)
		}

		// Bounds: numeric and ordered when both present.
		if loStr != "" {
			if err := validateDecimal("tier lower_bound", loStr); err != nil {
				return nil, fmt.Errorf("--tier %q: %w", raw, err)
			}
		}
		if hiStr != "" {
			if err := validateDecimal("tier upper_bound", hiStr); err != nil {
				return nil, fmt.Errorf("--tier %q: %w", raw, err)
			}
		}
		if loStr != "" && hiStr != "" {
			lo, _ := strconv.ParseFloat(loStr, 64)
			hi, _ := strconv.ParseFloat(hiStr, 64)
			if lo >= hi {
				return nil, fmt.Errorf("--tier %q: lower bound must be < upper bound", raw)
			}
		}

		// Rate: 0–100.
		if err := validateDecimal("application_rate", rateStr); err != nil {
			return nil, fmt.Errorf("--tier %q: %w", raw, err)
		}
		rate, _ := strconv.ParseFloat(rateStr, 64)
		if rate < 0 || rate > 100 {
			return nil, fmt.Errorf("--tier %q: application_rate %s must be between 0 and 100", raw, rateStr)
		}

		entry := map[string]any{
			"position":         i,
			"application_rate": rateStr,
		}
		if loStr == "" {
			entry["lower_bound"] = nil
		} else {
			entry["lower_bound"] = loStr
		}
		if hiStr == "" {
			entry["upper_bound"] = nil
			prevUpper = nil
		} else {
			entry["upper_bound"] = hiStr
			s := hiStr
			prevUpper = &s
		}
		tiers = append(tiers, entry)
	}
	return tiers, nil
}

// validateDecimal returns a UsageError when s isn't a valid decimal number.
// We pass the value through to the API as a string so trailing zeros and
// formatting choices survive (the Rails BigDecimal coercion handles strings).
func validateDecimal(arg, s string) error {
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s must be a decimal number (got %q)", arg, s)}
	}
	return nil
}

// validatePeriod accepts YYYY-MM or YYYY-Qn (n in 1..4). Mirrors the server's
// Increase::PERIOD_FORMAT regex but kept simple — the API still has the final
// say.
func validatePeriod(arg, s string) error {
	year, tail, ok := strings.Cut(s, "-")
	if !ok {
		return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s: expected YYYY-MM or YYYY-Q[1-4] (got %q)", arg, s)}
	}
	yr, err := strconv.Atoi(year)
	if err != nil || yr < 1900 || yr > 2999 {
		return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s: year must be 1900–2999 (got %q)", arg, s)}
	}
	upper := strings.ToUpper(tail)
	switch {
	case len(upper) == 2 && upper[0] >= '0' && upper[0] <= '9':
		m, err := strconv.Atoi(upper)
		if err != nil || m < 1 || m > 12 {
			return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s: month must be 01–12 (got %q)", arg, s)}
		}
		return nil
	case len(upper) == 2 && upper[0] == 'Q':
		q, err := strconv.Atoi(upper[1:])
		if err != nil || q < 1 || q > 4 {
			return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s: quarter must be Q1–Q4 (got %q)", arg, s)}
		}
		return nil
	}
	return &UsageError{Arg: arg, Usage: fmt.Sprintf("--%s: expected YYYY-MM or YYYY-Q[1-4] (got %q)", arg, s)}
}

// derefDefault returns *p when non-nil, else the supplied default. Small
// helper used in tier validation messages.
func derefDefault(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func init() {
	c := abstractionIncreaseCreateCmd
	c.Flags().StringVar(&increaseCreateIncreasableSubObjectGroup, "increasable-sub-object-group", "", "Parent Expense's draft sub_object_group UUID (greenfield)")
	c.Flags().IntVar(&increaseCreateIncreasableTargetID, "increasable-target-id", 0, "Parent Expense's live record ID (brownfield)")
	c.Flags().StringVar(&increaseCreateType, "type", "", "fixed | percentage | indexation (required)")
	c.Flags().StringVar(&increaseCreateEffectiveDate, "effective-date", "", "Effective date (YYYY-MM-DD). Mutually exclusive with --anchor.")
	c.Flags().StringSliceVar(&increaseCreateAnchorSpecs, "anchor", nil, "Anchor spec for an anchored effective_date. Repeatable. Same syntax as `changes create --anchor`.")
	c.Flags().StringVar(&increaseCreateAnchorResolution, "anchor-resolution", "", "earliest_of|latest_of (defaults to earliest_of for single-anchor; required for multi-anchor)")
	c.Flags().StringVar(&increaseCreateProvisionalDate, "provisional-date", "", "Override the CLI-computed provisional_date (YYYY-MM-DD)")
	c.Flags().StringVar(&increaseCreateFixedAmount, "fixed-amount", "", "New amount (--type fixed)")
	c.Flags().StringVar(&increaseCreatePercentage, "percentage", "", "Percentage step (--type percentage)")
	c.Flags().IntVar(&increaseCreateInflationSeriesID, "inflation-series-id", 0, "InflationSeries id (--type indexation)")
	c.Flags().StringVar(&increaseCreateStartPeriod, "start-period", "", "Start observation period: YYYY-MM or YYYY-Q[1-4] (--type indexation)")
	c.Flags().StringVar(&increaseCreateEndPeriod, "end-period", "", "End observation period: YYYY-MM or YYYY-Q[1-4] (--type indexation)")
	c.Flags().StringVar(&increaseCreateFloor, "floor", "", "Floor percentage (--type indexation)")
	c.Flags().StringVar(&increaseCreateCeiling, "ceiling", "", "Ceiling percentage (--type indexation)")
	c.Flags().StringSliceVar(&increaseCreateTiers, "tier", nil, "Indexation tier: <lo>:<hi>:<rate> (repeatable; lo/hi blank = open-ended)")
	c.Flags().StringVar(&increaseCreateRecurrenceCadence, "recurrence-cadence", "", "lease_anniversary|expense_start_anniversary|january_1|custom")
	c.Flags().IntVar(&increaseCreateRecurrenceIntervalAmount, "recurrence-interval-amount", 0, "Custom cadence interval amount (years/quarters/months)")
	c.Flags().StringVar(&increaseCreateRecurrenceIntervalUnit, "recurrence-interval-unit", "", "Custom cadence interval unit: years|quarters|months")
	c.Flags().StringVar(&increaseCreateNotes, "notes", "", "Notes carried on the increase")
	c.Flags().StringVar(&increaseCreateSourceLinksInput, "source-links", "", `Source links array as JSON, @file, or - for stdin`)
	c.Flags().StringSliceVar(&increaseCreateCiteBlocks, "cite-block", nil, `Cite a parsed block. Repeatable. Formats: <block-id>, <block-id>:chars=S-E, <block-id>:cell=R,C`)
	c.Flags().IntVar(&increaseCreateParentChangeID, "parent-change-id", 0, "Parent change this one depends on")
	c.Flags().IntVar(&increaseCreateRevisedFromID, "revised-from-id", 0, "Change ID this one supersedes (auto-linked to rejected predecessors if omitted)")

	abstractionIncreaseCmd.AddCommand(abstractionIncreaseCreateCmd)
	abstractionsCmd.AddCommand(abstractionIncreaseCmd)
}
