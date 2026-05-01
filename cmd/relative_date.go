package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// anchorSpec is the parsed form of a single --anchor flag value.
//
// Spec syntax (comma-separated key=value pairs, no spaces around `=`):
//
//	target_type=Lease,target_field=start_date,offset_months=12,offset_days=0,inclusive=false
//	target_type=KeyDate,target_field=date,sub_object_group=<uuid>,offset_days=-30
//	target_type=Expense,target_field=start_date,target_id=42,offset_months=6
//
// target_id and sub_object_group are mutually exclusive. Either is also fine
// to omit when anchoring to a primary-target Lease/Property — the API resolves
// those at apply time.
type anchorSpec struct {
	TargetType      string
	TargetField     string
	TargetID        *int    // mutually exclusive with SubObjectGroup
	SubObjectGroup  *string // mutually exclusive with TargetID
	OffsetMonths    int
	OffsetDays      int
	InclusiveOffset bool // default true
}

// parseAnchorSpec parses one --anchor value into an anchorSpec. Errors are
// returned with the original spec embedded so the user can spot which flag
// value tripped the parse.
func parseAnchorSpec(raw string) (*anchorSpec, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("--anchor: empty spec")
	}
	spec := &anchorSpec{InclusiveOffset: true}
	parts := strings.Split(raw, ",")
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key, value, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("--anchor %q: each part must be key=value", raw)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if seen[key] {
			return nil, fmt.Errorf("--anchor %q: duplicate key %q", raw, key)
		}
		seen[key] = true
		switch key {
		case "target_type":
			spec.TargetType = value
		case "target_field":
			spec.TargetField = value
		case "target_id":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("--anchor %q: target_id must be a positive integer", raw)
			}
			spec.TargetID = &n
		case "sub_object_group":
			v := value
			spec.SubObjectGroup = &v
		case "offset_months":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("--anchor %q: offset_months must be an integer", raw)
			}
			spec.OffsetMonths = n
		case "offset_days":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("--anchor %q: offset_days must be an integer", raw)
			}
			spec.OffsetDays = n
		case "inclusive":
			b, err := parseBool(value)
			if err != nil {
				return nil, fmt.Errorf("--anchor %q: inclusive must be true|false", raw)
			}
			spec.InclusiveOffset = b
		default:
			return nil, fmt.Errorf("--anchor %q: unknown key %q (want target_type, target_field, target_id, sub_object_group, offset_months, offset_days, inclusive)", raw, key)
		}
	}
	if spec.TargetType == "" {
		return nil, fmt.Errorf("--anchor %q: target_type is required", raw)
	}
	if spec.TargetField == "" {
		return nil, fmt.Errorf("--anchor %q: target_field is required", raw)
	}
	if spec.TargetID != nil && spec.SubObjectGroup != nil {
		return nil, fmt.Errorf("--anchor %q: target_id and sub_object_group are mutually exclusive", raw)
	}
	return spec, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "t", "yes", "y", "1":
		return true, nil
	case "false", "f", "no", "n", "0":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean")
}

// anchorOption mirrors the AnchorOption schema returned by
// GET /abstractions/:id/anchorable_dates. Only the fields the CLI needs.
type anchorOption struct {
	Kind           string  `json:"kind"`
	TargetType     string  `json:"target_type"`
	TargetField    string  `json:"target_field"`
	TargetID       *int    `json:"target_id"`
	SubObjectGroup *string `json:"sub_object_group"`
	Label          string  `json:"label"`
	CurrentValue   *string `json:"current_value"`
	State          *string `json:"state"`
}

// fetchAnchorableDates calls GET /abstractions/:id/anchorable_dates and
// returns the parsed anchor list. Used both by the anchorable-dates command
// (display) and by the relative-date payload builder (best-guess
// provisional_date computation).
func fetchAnchorableDates(absID string) ([]anchorOption, error) {
	env, err := client.Get("/abstractions/"+absID+"/anchorable_dates", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Anchors []anchorOption `json:"anchors"`
	}
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		return nil, fmt.Errorf("decoding anchorable_dates: %w", err)
	}
	return resp.Anchors, nil
}

// buildRelativePayload assembles the {mode:"relative",...} payload value for a
// single date field. If provisionalOverride is non-empty the caller's value is
// taken verbatim; otherwise the CLI computes a best-guess provisional_date by
// fetching the abstraction's anchorable_dates and applying each anchor's
// offset to the live current_value when available.
//
// The fallback chain when current_values are unavailable:
//  1. If at least one anchor resolved to a candidate date → combine via the
//     resolution method (earliest_of/latest_of) and use that.
//  2. Otherwise → use today's date in UTC. The caller gets a notice on stderr
//     so they can override with --provisional-date if desired.
//
// The resulting payload can be wrapped under target_field for non-Increase
// changes, or used as the value of effective_date inside an Increase opaque
// payload.
func buildRelativePayload(absID, resolution string, anchors []*anchorSpec, provisionalOverride string) (map[string]any, string, bool, error) {
	if len(anchors) == 0 {
		return nil, "", false, fmt.Errorf("--anchor: at least one anchor is required for a relative-date payload")
	}
	if resolution == "" {
		// Single-anchor specs still need resolution per the API contract;
		// default earliest_of for ergonomic single-anchor use.
		if len(anchors) == 1 {
			resolution = "earliest_of"
		} else {
			return nil, "", false, fmt.Errorf("--anchor-resolution: required when more than one --anchor is supplied (earliest_of|latest_of)")
		}
	}
	if resolution != "earliest_of" && resolution != "latest_of" {
		return nil, "", false, fmt.Errorf("--anchor-resolution: must be earliest_of or latest_of (got %q)", resolution)
	}

	// Provisional date: caller wins, otherwise compute from anchors.
	var provisional string
	var inferred bool
	if provisionalOverride != "" {
		if _, err := time.Parse("2006-01-02", provisionalOverride); err != nil {
			return nil, "", false, fmt.Errorf("--provisional-date: must be YYYY-MM-DD (got %q)", provisionalOverride)
		}
		provisional = provisionalOverride
	} else {
		options, err := fetchAnchorableDates(absID)
		if err != nil {
			return nil, "", false, fmt.Errorf("looking up anchor current_values for provisional date: %w", err)
		}
		date, src := guessProvisionalDate(anchors, resolution, options)
		provisional = date
		inferred = true
		_ = src // kept for future telemetry; not surfaced to caller today
	}

	// Anchors array — one entry per spec.
	anchorJSON := make([]map[string]any, 0, len(anchors))
	for _, a := range anchors {
		ref := map[string]any{
			"target_type":  a.TargetType,
			"target_field": a.TargetField,
		}
		if a.TargetID != nil {
			ref["target_id"] = *a.TargetID
		}
		if a.SubObjectGroup != nil {
			ref["sub_object_group"] = *a.SubObjectGroup
		}
		anchorJSON = append(anchorJSON, map[string]any{
			"anchor":           ref,
			"offset_months":    a.OffsetMonths,
			"offset_days":      a.OffsetDays,
			"inclusive_offset": a.InclusiveOffset,
		})
	}

	payload := map[string]any{
		"mode":             "relative",
		"resolution":       resolution,
		"provisional_date": provisional,
		"anchors":          anchorJSON,
	}
	return payload, provisional, inferred, nil
}

// guessProvisionalDate returns the best-effort provisional_date for the supplied
// anchor specs. Walks each spec, looks up its matching anchor in the supplied
// options list (live API data), and applies the spec's offset to the option's
// current_value when known. Combines candidates via the resolution method.
//
// Falls back to today's date (UTC) when no anchor in the spec list has a
// resolvable current_value — a required-field fallback so the API accepts the
// payload at draft time even when every anchor is itself a draft sibling. The
// resolved date is overwritten in phase 2 of go-live anyway.
func guessProvisionalDate(specs []*anchorSpec, resolution string, options []anchorOption) (string, string) {
	candidates := []time.Time{}
	for _, spec := range specs {
		opt := matchAnchorOption(spec, options)
		if opt == nil || opt.CurrentValue == nil {
			continue
		}
		base, err := time.Parse("2006-01-02", *opt.CurrentValue)
		if err != nil {
			continue
		}
		candidates = append(candidates, applyOffset(base, spec.OffsetMonths, spec.OffsetDays, spec.InclusiveOffset))
	}
	if len(candidates) == 0 {
		// All anchors pending — fall back to today. The reviewer (or phase 2
		// of go-live) will overwrite once anchors resolve.
		return time.Now().UTC().Format("2006-01-02"), "fallback_today"
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	var picked time.Time
	if resolution == "latest_of" {
		picked = candidates[len(candidates)-1]
	} else {
		picked = candidates[0]
	}
	return picked.Format("2006-01-02"), "from_anchors"
}

// matchAnchorOption finds the AnchorOption that corresponds to an anchor spec.
// Match keys: target_type + target_field + (target_id|sub_object_group).
// Primary-target refs (no id/group) match against the option that also has
// neither set — the API surfaces these as `primary_target_*` kinds.
func matchAnchorOption(spec *anchorSpec, options []anchorOption) *anchorOption {
	for i := range options {
		opt := &options[i]
		if opt.TargetType != spec.TargetType || opt.TargetField != spec.TargetField {
			continue
		}
		switch {
		case spec.TargetID != nil:
			if opt.TargetID != nil && *opt.TargetID == *spec.TargetID {
				return opt
			}
		case spec.SubObjectGroup != nil:
			if opt.SubObjectGroup != nil && *opt.SubObjectGroup == *spec.SubObjectGroup {
				return opt
			}
		default:
			// Primary-target ref — option has neither id nor group set.
			if opt.TargetID == nil && opt.SubObjectGroup == nil {
				return opt
			}
		}
	}
	return nil
}

// applyOffset mirrors DateDependency#apply_offset on the Rails side. Adds
// `months` and `days` to `base` with end-of-month preservation (a base date on
// the last day of its month always lands on the last day of the target month
// after a months shift), then applies the inclusive-offset convention: a
// positive net offset subtracts 1 day, a negative net offset adds 1 day, and
// a zero offset is unchanged.
//
// This is a "best guess" helper — phase 2 of go-live re-resolves the date
// authoritatively from the dependency graph, so small differences with the
// Ruby implementation don't matter for correctness, only for draft-time UX.
func applyOffset(base time.Time, months, days int, inclusive bool) time.Time {
	advanced := eomAdvance(base, months)
	advanced = advanced.AddDate(0, 0, days)
	if !inclusive {
		return advanced
	}
	cmp := advanced.Compare(base)
	switch {
	case cmp > 0:
		return advanced.AddDate(0, 0, -1)
	case cmp < 0:
		return advanced.AddDate(0, 0, 1)
	}
	return advanced
}

// eomAdvance shifts `d` by `months` while preserving end-of-month-ness:
// if the source is the last day of its month, the result lands on the last
// day of the target month even if the target month is shorter (Jan 31 → Feb
// 28/29, Mar 31 → Apr 30).
//
// Go's time.AddDate normalizes overflows differently — Jan 31 + 1 month is
// Mar 3 — so we hand-roll the month math.
func eomAdvance(d time.Time, months int) time.Time {
	advanced := addMonthsClamped(d, months)
	if isEndOfMonth(d) {
		return lastDayOfMonth(advanced)
	}
	return advanced
}

// addMonthsClamped advances `d` by `months` and clamps the day to the last
// day of the target month if needed. Mirrors Ruby Date#advance(months: n).
func addMonthsClamped(d time.Time, months int) time.Time {
	year, month, day := d.Date()
	totalMonth := int(month) - 1 + months // 0-indexed
	targetYear := year + totalMonth/12
	targetMonth := totalMonth % 12
	if targetMonth < 0 {
		targetMonth += 12
		targetYear--
	}
	tm := time.Month(targetMonth + 1)
	dim := daysInMonth(targetYear, tm)
	if day > dim {
		day = dim
	}
	return time.Date(targetYear, tm, day, d.Hour(), d.Minute(), d.Second(), d.Nanosecond(), d.Location())
}

// isEndOfMonth returns true when d is the last day of its calendar month.
func isEndOfMonth(d time.Time) bool {
	year, month, day := d.Date()
	return day == daysInMonth(year, month)
}

// lastDayOfMonth returns d shifted to the last day of d's month.
func lastDayOfMonth(d time.Time) time.Time {
	year, month, _ := d.Date()
	dim := daysInMonth(year, month)
	return time.Date(year, month, dim, d.Hour(), d.Minute(), d.Second(), d.Nanosecond(), d.Location())
}

// daysInMonth returns the number of days in the given calendar month.
func daysInMonth(year int, month time.Month) int {
	// Construct day 0 of the *next* month — Go normalizes that to the last
	// day of the current month.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
