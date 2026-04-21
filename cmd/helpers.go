package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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
// Every authenticated command calls this first. Returns an authMissingError
// so the root error handler can map to ExitAuth and emit code:"unauthorized".
func requireLogin() error {
	if cfg.Token == "" {
		return &authMissingError{}
	}
	return nil
}

// readInputValue interprets a flag value as one of:
//   - "@/path/to/file" — read file contents
//   - "-"              — read stdin
//   - anything else    — treat as literal
//
// Used for --payload, --source-links, and similar free-form inputs where
// the CLI user might have a JSON file on disk or pipe something in.
func readInputValue(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if v == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(b), nil
	}
	if strings.HasPrefix(v, "@") {
		path := v[1:]
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		return string(b), nil
	}
	return v, nil
}
