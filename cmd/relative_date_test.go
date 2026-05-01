package cmd

import (
	"testing"
	"time"
)

func TestParseAnchorSpec(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, s *anchorSpec)
	}{
		{
			name: "minimal primary-target",
			raw:  "target_type=Lease,target_field=start_date",
			check: func(t *testing.T, s *anchorSpec) {
				if s.TargetType != "Lease" || s.TargetField != "start_date" {
					t.Fatalf("got %+v", s)
				}
				if !s.InclusiveOffset {
					t.Fatalf("inclusive should default true")
				}
				if s.OffsetMonths != 0 || s.OffsetDays != 0 {
					t.Fatalf("offsets should default 0, got %+v", s)
				}
			},
		},
		{
			name: "live record",
			raw:  "target_type=Expense,target_field=start_date,target_id=42,offset_months=6,offset_days=15,inclusive=false",
			check: func(t *testing.T, s *anchorSpec) {
				if s.TargetID == nil || *s.TargetID != 42 {
					t.Fatalf("target_id wrong: %+v", s.TargetID)
				}
				if s.SubObjectGroup != nil {
					t.Fatalf("sub_object_group should be nil")
				}
				if s.OffsetMonths != 6 || s.OffsetDays != 15 || s.InclusiveOffset {
					t.Fatalf("offsets wrong: %+v", s)
				}
			},
		},
		{
			name: "draft sibling",
			raw:  "target_type=KeyDate,target_field=date,sub_object_group=abc-123,offset_days=-30",
			check: func(t *testing.T, s *anchorSpec) {
				if s.SubObjectGroup == nil || *s.SubObjectGroup != "abc-123" {
					t.Fatalf("sub_object_group wrong")
				}
				if s.OffsetDays != -30 {
					t.Fatalf("days should be -30")
				}
			},
		},
		{name: "missing target_type", raw: "target_field=start_date", wantErr: true},
		{name: "missing target_field", raw: "target_type=Lease", wantErr: true},
		{name: "id and group", raw: "target_type=KeyDate,target_field=date,target_id=1,sub_object_group=abc", wantErr: true},
		{name: "bad offset", raw: "target_type=Lease,target_field=start_date,offset_months=foo", wantErr: true},
		{name: "unknown key", raw: "target_type=Lease,target_field=start_date,foo=bar", wantErr: true},
		{name: "duplicate key", raw: "target_type=Lease,target_field=start_date,target_field=end_date", wantErr: true},
		{name: "no equals", raw: "target_type=Lease,target_field", wantErr: true},
		{name: "unknown target_type", raw: "target_type=Leases,target_field=start_date", wantErr: true},
		{name: "non-primary missing id and group", raw: "target_type=KeyDate,target_field=date", wantErr: true},
		{name: "non-primary with id ok", raw: "target_type=KeyDate,target_field=date,target_id=42"},
		{name: "non-primary with group ok", raw: "target_type=Expense,target_field=start_date,sub_object_group=abc"},
		{name: "primary target without id ok", raw: "target_type=Property,target_field=acquired_date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAnchorSpec(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestApplyOffset(t *testing.T) {
	mustDate := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("bad date %q: %v", s, err)
		}
		return d
	}
	tests := []struct {
		name      string
		base      string
		months    int
		days      int
		inclusive bool
		want      string
	}{
		// Non-inclusive (point in time): just shift.
		{"30 days after", "2026-01-01", 0, 30, false, "2026-01-31"},
		{"-30 days", "2026-02-01", 0, -30, false, "2026-01-02"},
		{"6 months point", "2026-01-15", 6, 0, false, "2026-07-15"},

		// Inclusive: positive net subtracts a day, negative adds one, zero unchanged.
		{"5 year term inclusive", "2026-01-01", 60, 0, true, "2030-12-31"},
		{"1 year inclusive", "2026-01-01", 12, 0, true, "2026-12-31"},
		{"-1 year inclusive", "2026-01-01", -12, 0, true, "2025-01-02"},
		{"zero inclusive", "2026-04-01", 0, 0, true, "2026-04-01"},

		// EOM advance: source on last day of month → result on last day of target month.
		{"EOM Jan 31 + 1 month", "2026-01-31", 1, 0, false, "2026-02-28"},
		{"EOM Jan 31 + 1 month leap", "2024-01-31", 1, 0, false, "2024-02-29"},
		{"EOM Mar 31 + 1 month", "2026-03-31", 1, 0, false, "2026-04-30"},

		// Day clamping when source isn't EOM but target month is shorter.
		{"non-EOM Jan 30 + 1 month", "2026-01-30", 1, 0, false, "2026-02-28"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyOffset(mustDate(tt.base), tt.months, tt.days, tt.inclusive)
			gotStr := got.Format("2006-01-02")
			if gotStr != tt.want {
				t.Fatalf("apply_offset(%s, %d months, %d days, inclusive=%v) = %s, want %s", tt.base, tt.months, tt.days, tt.inclusive, gotStr, tt.want)
			}
		})
	}
}

func TestParseTierSpecs(t *testing.T) {
	tests := []struct {
		name    string
		specs   []string
		wantErr bool
		nTiers  int
	}{
		{"no tiers", nil, false, 0},
		{"single open both", []string{"::100"}, false, 1},
		{"two tier blended", []string{":4:100", "4::50"}, false, 2},
		{"three tier", []string{":4:100", "4:8:50", "8::25"}, false, 3},
		{"non-contiguous", []string{":4:100", "5::50"}, true, 0},
		{"first interior bounded then open", []string{"0:4:100", "4::50"}, false, 2},
		{"second tier open lower", []string{":4:100", ":8:50"}, true, 0},
		{"last tier closed", []string{":4:100", "4:8:50"}, true, 0},
		{"rate too high", []string{"::150"}, true, 0},
		{"lo>=hi", []string{"4:4:100"}, true, 0},
		{"bad format", []string{"4-8-100"}, true, 0},
		{"bad rate", []string{"::abc"}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTierSpecs(tt.specs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.nTiers {
				t.Fatalf("got %d tiers, want %d (tiers=%+v)", len(got), tt.nTiers, got)
			}
		})
	}
}

func TestValidatePeriod(t *testing.T) {
	good := []string{"2024-01", "2024-12", "2024-Q1", "2024-Q4", "2024-q1"}
	bad := []string{"2024", "2024-13", "2024-00", "24-01", "2024-Q5", "2024-Q0", "abc-01", "2024-Jan"}
	for _, s := range good {
		if err := validatePeriod("start-period", s); err != nil {
			t.Fatalf("expected %q to validate, got %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validatePeriod("start-period", s); err == nil {
			t.Fatalf("expected %q to fail validation", s)
		}
	}
}

func TestMatchAnchorOption(t *testing.T) {
	id7 := 7
	groupA := "uuid-A"
	groupB := "uuid-B"
	options := []anchorOption{
		{TargetType: "Lease", TargetField: "start_date", Label: "Lease: Start", Kind: "primary_target_live"},
		{TargetType: "Expense", TargetField: "start_date", TargetID: &id7, Label: "Expense start"},
		{TargetType: "KeyDate", TargetField: "date", SubObjectGroup: &groupA, Label: "Lease Expiration"},
	}
	id := func(n int) *int { return &n }
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		spec *anchorSpec
		want string // option Label, "" if no match
	}{
		{"primary target", &anchorSpec{TargetType: "Lease", TargetField: "start_date"}, "Lease: Start"},
		{"by id", &anchorSpec{TargetType: "Expense", TargetField: "start_date", TargetID: id(7)}, "Expense start"},
		{"by id miss", &anchorSpec{TargetType: "Expense", TargetField: "start_date", TargetID: id(8)}, ""},
		{"by group", &anchorSpec{TargetType: "KeyDate", TargetField: "date", SubObjectGroup: &groupA}, "Lease Expiration"},
		{"by group miss", &anchorSpec{TargetType: "KeyDate", TargetField: "date", SubObjectGroup: str(groupB)}, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := matchAnchorOption(tt.spec, options)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected no match, got %+v", got)
				}
				return
			}
			if got == nil || got.Label != tt.want {
				t.Fatalf("got %+v, want label %q", got, tt.want)
			}
		})
	}
}

func TestGuessProvisionalDate(t *testing.T) {
	dec1 := "2026-01-01"
	jul1 := "2026-07-01"
	options := []anchorOption{
		{TargetType: "Lease", TargetField: "start_date", CurrentValue: &dec1, Label: "Lease start"},
		{TargetType: "Expense", TargetField: "start_date", CurrentValue: &jul1, Label: "Expense start"},
	}
	specs := []*anchorSpec{
		{TargetType: "Lease", TargetField: "start_date", OffsetDays: 30, InclusiveOffset: false},
		{TargetType: "Expense", TargetField: "start_date", OffsetDays: 0, InclusiveOffset: false},
	}
	gotEarliest, _ := guessProvisionalDate(specs, "earliest_of", options)
	if gotEarliest != "2026-01-31" {
		t.Fatalf("earliest_of: got %s, want 2026-01-31", gotEarliest)
	}
	gotLatest, _ := guessProvisionalDate(specs, "latest_of", options)
	if gotLatest != "2026-07-01" {
		t.Fatalf("latest_of: got %s, want 2026-07-01", gotLatest)
	}
	// Pending anchors → fallback to today
	got, src := guessProvisionalDate(specs, "earliest_of", []anchorOption{})
	if src != "fallback_today" {
		t.Fatalf("source label should be fallback_today, got %s", src)
	}
	if got == "" {
		t.Fatalf("expected non-empty fallback date")
	}
}
