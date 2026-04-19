// Package output handles switching between styled TTY output and raw JSON.
//
// This is like checking $stdout.tty? in Ruby and changing your puts/pp behavior.
// When the user is at a terminal, we show pretty tables. When piped (e.g.,
// kestrel properties list | jq '.data[0]'), we emit raw JSON.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"
)

// Mode controls how output is rendered.
type Mode int

const (
	ModeAuto  Mode = iota // detect TTY
	ModeJSON              // --json flag
	ModeQuiet             // --quiet flag
)

// Printer handles rendering output in the appropriate format.
type Printer struct {
	Mode Mode
}

// IsJSON returns true if output should be raw JSON (either --json flag or piped).
func (p *Printer) IsJSON() bool {
	if p.Mode == ModeJSON {
		return true
	}
	if p.Mode == ModeQuiet {
		return false
	}
	// Auto-detect: if stdout is not a terminal, output JSON
	return !term.IsTerminal(int(os.Stdout.Fd()))
}

// JSON prints raw JSON bytes to stdout with a trailing newline.
func (p *Printer) JSON(data []byte) {
	// Pretty-print the JSON for readability
	var pretty json.RawMessage
	if err := json.Unmarshal(data, &pretty); err == nil {
		if formatted, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			fmt.Println(string(formatted))
			return
		}
	}
	fmt.Println(string(data))
}

// Table prints data as a formatted table.
// headers is the column names, rows is the data.
// This uses Go's tabwriter — similar to Ruby's printf formatting
// but it auto-calculates column widths.
func (p *Printer) Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header row
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Separator
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("─", len(h))
	}
	fmt.Fprintln(w, strings.Join(seps, "\t"))

	// Data rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

// Detail prints key-value pairs for a single record.
// Like a Ruby hash pretty-printed with aligned keys.
func (p *Printer) Detail(pairs [][]string) {
	// Find the longest key for alignment
	maxKey := 0
	for _, pair := range pairs {
		if len(pair[0]) > maxKey {
			maxKey = len(pair[0])
		}
	}

	for _, pair := range pairs {
		fmt.Printf("%-*s  %s\n", maxKey, pair[0]+":", pair[1])
	}
}

// Success prints a success message to stderr (so it doesn't pollute piped JSON).
func (p *Printer) Success(msg string) {
	fmt.Fprintln(os.Stderr, "✓ "+msg)
}

// Errorf prints an error message to stderr.
func (p *Printer) Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// PaginationHint prints a hint about more pages, if applicable.
func (p *Printer) PaginationHint(nextPage *int, count int) {
	if nextPage != nil {
		fmt.Fprintf(os.Stderr, "\nShowing page results (%d total). Next page: --page %d\n", count, *nextPage)
	}
}

// Breadcrumb prints a "next step" suggestion to stderr for humans.
// Suppressed in JSON mode — agents discover the workflow via `kestrel commands --json`.
func (p *Printer) Breadcrumb(msg string) {
	if p.IsJSON() {
		return
	}
	fmt.Fprintln(os.Stderr, "→ "+msg)
}
