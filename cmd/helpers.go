package cmd

import (
	"fmt"
	"strconv"
)

// Nullable-value helpers. The API frequently returns null for optional fields,
// which Go decodes into *T. These helpers format them for table/detail output.
// Think of them as the Ruby equivalent of `value&.to_s` with a blank fallback.

// derefInt returns the int as a string, or "" if nil.
func derefInt(i *int) string {
	if i == nil {
		return ""
	}
	return strconv.Itoa(*i)
}

// derefFloat formats a *float64 with 2 decimal places, or "" if nil.
func derefFloat(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *f)
}

// derefBool returns "yes"/"no" for a *bool, or "" if nil.
func derefBool(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "yes"
	}
	return "no"
}

// requireLogin returns an error if no token is configured.
// Every authenticated command calls this first.
func requireLogin() error {
	if cfg.Token == "" {
		return fmt.Errorf("not logged in. Run: kestrel login")
	}
	return nil
}
