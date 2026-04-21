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
	ModeAgent             // --agent flag — data-only on success, {ok:false,...} on error
)

// Printer handles rendering output in the appropriate format.
//
// The printer accumulates breadcrumbs and a summary line during a command's
// execution. At the end, call Finish to emit them:
//   - in JSON mode: merged into the output envelope
//   - in TTY mode: flushed to stderr after the main render
type Printer struct {
	Mode Mode

	breadcrumbs []string
	summary     string
}

// IsJSON returns true if output should be raw JSON (envelope form).
func (p *Printer) IsJSON() bool {
	if p.Mode == ModeJSON {
		return true
	}
	if p.Mode == ModeQuiet || p.Mode == ModeAgent {
		return false
	}
	// Auto-detect: piped stdout → JSON
	return !term.IsTerminal(int(os.Stdout.Fd()))
}

// IsAgent returns true if we're in --agent mode (data-only success envelope).
func (p *Printer) IsAgent() bool {
	return p.Mode == ModeAgent
}

// IsStructured reports whether stdout should be machine-readable JSON
// (either JSON envelope or agent data-only).
func (p *Printer) IsStructured() bool {
	return p.IsJSON() || p.IsAgent()
}

// Breadcrumb records a "next step" suggestion. Rendered with the envelope
// in JSON/agent mode or flushed to stderr in TTY mode at Finish time.
func (p *Printer) Breadcrumb(msg string) {
	p.breadcrumbs = append(p.breadcrumbs, msg)
}

// Summary records a one-line human-readable result for the command.
// Only the most recent call wins.
func (p *Printer) Summary(msg string) {
	p.summary = msg
}

// Breadcrumbs returns the accumulated breadcrumbs (used by --agent error shaping).
func (p *Printer) Breadcrumbs() []string {
	return p.breadcrumbs
}

// SummaryText returns the accumulated summary string.
func (p *Printer) SummaryText() string {
	return p.summary
}

// FinishRaw emits a command's final output when the command holds a raw JSON
// envelope (typical for read commands that call client.GetRaw).
//
//   - JSON mode: decodes the envelope, merges accumulated summary/breadcrumbs,
//     re-encodes to stdout.
//   - Agent mode: emits just the `data` field (or `{ok:false,...}` on error).
//   - TTY mode: flushes summary + breadcrumbs to stderr (the command has
//     already rendered its table/detail).
func (p *Printer) FinishRaw(raw []byte) {
	if p.IsAgent() {
		p.emitAgent(raw)
		return
	}
	if p.IsJSON() {
		p.emitJSON(raw)
		return
	}
	p.flushTTY()
}

// FinishEnvelope is the same contract as FinishRaw but for commands that
// already hold a parsed *api-style envelope (typical for write commands).
// The envelope shape is kept loose (any struct with JSON tags) to avoid an
// import cycle — the caller marshals it, we re-parse.
func (p *Printer) FinishEnvelope(env any) {
	if env == nil {
		// No body (e.g. 204). TTY mode just flushes breadcrumbs; structured
		// modes get a minimal `{ok:true}` so agents have a consistent contract.
		if !p.IsStructured() {
			p.flushTTY()
			return
		}
		minimal := map[string]any{"ok": true}
		raw, _ := json.Marshal(minimal)
		p.FinishRaw(raw)
		return
	}
	raw, err := json.Marshal(env)
	if err != nil {
		// Marshal shouldn't fail for our own envelope; just flush breadcrumbs.
		p.flushTTY()
		return
	}
	p.FinishRaw(raw)
}

func (p *Printer) emitJSON(raw []byte) {
	if len(raw) == 0 {
		return
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		// Not valid JSON — emit as-is.
		fmt.Println(string(raw))
		return
	}
	if p.summary != "" {
		env["summary"] = p.summary
	}
	if len(p.breadcrumbs) > 0 {
		env["breadcrumbs"] = p.breadcrumbs
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		fmt.Println(string(raw))
		return
	}
	fmt.Println(string(out))
}

func (p *Printer) emitAgent(raw []byte) {
	if len(raw) == 0 {
		return
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		fmt.Println(string(raw))
		return
	}
	// Success: emit just the `data` field by default. For paginated list
	// responses, wrap as {data, meta} so agents can detect truncation — a
	// bare array can't tell the difference between "50 of 50" and "50 of
	// 500". Detail/scalar responses (no meta) stay bare. Errors always emit
	// the full {ok:false,...} shape so agents can branch on failure.
	if ok, _ := env["ok"].(bool); ok {
		data, exists := env["data"]
		if !exists {
			data = nil
		}
		if meta, hasMeta := env["meta"].(map[string]any); hasMeta && isPaginationMeta(meta) {
			wrapped := map[string]any{"data": data, "meta": meta}
			out, _ := json.MarshalIndent(wrapped, "", "  ")
			fmt.Println(string(out))
			return
		}
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
		return
	}
	out, _ := json.MarshalIndent(env, "", "  ")
	fmt.Println(string(out))
}

// isPaginationMeta reports whether a meta object carries pagination signals
// (next_page or count). Endpoints with meta unrelated to pagination — e.g.
// /documents/.../blocks returning {count, limit, next_since_order} — also
// return true; wrapping is the conservative choice since agents benefit from
// the signal either way.
func isPaginationMeta(meta map[string]any) bool {
	for _, key := range []string{"next_page", "count", "next_since_order"} {
		if _, ok := meta[key]; ok {
			return true
		}
	}
	return false
}

func (p *Printer) flushTTY() {
	if p.summary != "" {
		fmt.Fprintln(os.Stderr, "✓ "+p.summary)
	}
	for _, b := range p.breadcrumbs {
		fmt.Fprintln(os.Stderr, "→ "+b)
	}
}

// JSON prints raw JSON bytes to stdout with a trailing newline.
// Prefer FinishRaw for command-final output; this is for unusual cases
// (e.g. `kestrel commands --json` dumping a static catalog).
func (p *Printer) JSON(data []byte) {
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
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("─", len(h))
	}
	fmt.Fprintln(w, strings.Join(seps, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// Detail prints key-value pairs for a single record.
// Like a Ruby hash pretty-printed with aligned keys.
func (p *Printer) Detail(pairs [][]string) {
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

// Success prints a one-off success message to stderr.
// Prefer Summary for results that should also show up in the JSON envelope.
func (p *Printer) Success(msg string) {
	if p.IsStructured() {
		return
	}
	fmt.Fprintln(os.Stderr, "✓ "+msg)
}

// Errorf prints an error message to stderr. Used for targeted hints before
// a RunE returns the error; the root error handler still emits the structured
// envelope in --json/--agent mode.
func (p *Printer) Errorf(format string, args ...any) {
	if p.IsStructured() {
		return
	}
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// PaginationHint prints a hint about more pages, if applicable (TTY only).
func (p *Printer) PaginationHint(nextPage *int, count int) {
	if p.IsStructured() || nextPage == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "\nShowing page results (%d total). Next page: --page %d\n", count, *nextPage)
}
