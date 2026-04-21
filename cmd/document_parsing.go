package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

// documentParse mirrors the DocumentParse schema in openapi.yaml.
// Progress is free-form JSON — we expose a typed view for TTY rendering
// but pass raw bytes through in structured modes.
type documentParse struct {
	DocumentID            int             `json:"document_id"`
	DocumentVersionNumber int             `json:"document_version_number"`
	Status                string          `json:"status"`
	Engine                string          `json:"engine"`
	EngineVersion         *string         `json:"engine_version"`
	PageCount             *int            `json:"page_count"`
	HasCoordinates        bool            `json:"has_coordinates"`
	HasRenderedPDF        bool            `json:"has_rendered_pdf"`
	Progress              *parseProgress  `json:"progress"`
	ParseOptions          json.RawMessage `json:"parse_options,omitempty"`
	ErrorMessage          *string         `json:"error_message"`
	StartedAt             *string         `json:"started_at"`
	CompletedAt           *string         `json:"completed_at"`
}

type parseProgress struct {
	Stage                string   `json:"stage"`
	StagePct             *float64 `json:"stage_pct"`
	OverallPct           *float64 `json:"overall_pct"`
	CurrentPage          *int     `json:"current_page"`
	TotalPages           *int     `json:"total_pages"`
	ElapsedMs            *int     `json:"elapsed_ms"`
	EstimatedRemainingMs *int     `json:"estimated_remaining_ms"`
}

// documentPage mirrors the DocumentPage schema.
type documentPage struct {
	PageNumber int      `json:"page_number"`
	WidthPt    *float64 `json:"width_pt"`
	HeightPt   *float64 `json:"height_pt"`
	Rotation   int      `json:"rotation"`
}

// documentBlock mirrors the DocumentBlock schema.
type documentBlock struct {
	ID                    int             `json:"id"`
	DocumentID            int             `json:"document_id"`
	DocumentVersionNumber int             `json:"document_version_number"`
	PageNumber            *int            `json:"page_number"`
	ReadingOrder          int             `json:"reading_order"`
	BlockType             string          `json:"block_type"`
	HeadingLevel          *int            `json:"heading_level"`
	Text                  string          `json:"text"`
	CharLength            int             `json:"char_length"`
	Bbox                  json.RawMessage `json:"bbox"`
	Metadata              json.RawMessage `json:"metadata"`
	ImageKey              *string         `json:"image_key"`
	Anchor                *string         `json:"anchor"`
	AnchorGroup           *string         `json:"anchor_group"`
}

var (
	parseWait        bool
	parseTimeoutSecs int
)

var documentsParseCmd = &cobra.Command{
	Use:   "parse <doc-id>",
	Short: "Show document parse status for the latest version",
	Long: `Fetches the structured-parse status for a document's latest version.

Parses are triggered lazily when a document is attached to an abstraction's
source set — upload alone does not parse. If the doc has never been attached
anywhere, you'll get 404 code:parse_missing.

Status values:
  queued | processing | complete | failed | stale

Use --wait to poll until the parse reaches a terminal state (complete/failed).
Terminal failed parses still exit 0 here — inspect the 'error_message' field
to branch on failure in scripts.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}

		if parseWait {
			return runParseWait(args[0], parseTimeoutSecs)
		}

		raw, err := client.GetRaw("/documents/"+args[0]+"/parse", nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data documentParse `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			renderParseDetail(resp.Data)
			if resp.Data.Status == "complete" {
				printer.Breadcrumb(fmt.Sprintf("Browse pages: kestrel documents pages %s", args[0]))
				printer.Breadcrumb(fmt.Sprintf("Browse blocks: kestrel documents blocks %s", args[0]))
			} else if resp.Data.Status == "queued" || resp.Data.Status == "processing" {
				printer.Breadcrumb(fmt.Sprintf("Wait for completion: kestrel documents parse %s --wait", args[0]))
			}
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var (
	pagesVersion int
)

var documentsPagesCmd = &cobra.Command{
	Use:   "pages <doc-id>",
	Short: "List pages of a parsed document version",
	Long: `Returns one row per page with width/height (in PDF points) and rotation.
Defaults to the latest version; pass --version to pin a specific version.

Empty array (not 404) when the document has no parse yet — check 'kestrel
documents parse <doc-id>' first if you want a readiness signal.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}

		versionNumber, err := resolveVersionNumber(args[0], pagesVersion)
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/documents/%s/versions/%d/pages", args[0], versionNumber)
		raw, err := client.GetRaw(path, nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data []documentPage `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			headers := []string{"Page", "Width (pt)", "Height (pt)", "Rotation"}
			rows := make([][]string, len(resp.Data))
			for i, p := range resp.Data {
				rows[i] = []string{
					strconv.Itoa(p.PageNumber),
					derefFloat(p.WidthPt),
					derefFloat(p.HeightPt),
					strconv.Itoa(p.Rotation),
				}
			}
			printer.Table(headers, rows)
		}
		printer.FinishRaw(raw)
		return nil
	},
}

var (
	blocksVersion    int
	blocksPage       int
	blocksType       string
	blocksSearch     string
	blocksSinceOrder int
	blocksNear       int
	blocksWindow     int
	blocksLimit      int
)

// blocksSearchMinLen is the client-side floor for --search queries. pg_trgm
// indexes on 3-character n-grams, so shorter inputs generate noisy ILIKE scans
// with no index benefit. 4 chars is the smallest query that meaningfully
// narrows results on typical lease corpora.
const blocksSearchMinLen = 4

var documentsBlocksCmd = &cobra.Command{
	Use:   "blocks <doc-id>",
	Short: "List parsed blocks for a document version",
	Long: `STRUCTURAL navigation of a parsed document's block graph.

If you're looking for a specific value — a date, a rent amount, a party
name, an address — use 'kestrel documents search <doc-id> <query>' instead.
Trigram search is typically 5–10× cheaper than walking structure.

This command is the right tool when you want to browse by structure: walk
through headings, scope to a page, examine blocks around one you already
found, or page through the full document. Default returns up to 500 blocks
starting at the top.

Filters:
  --page N              only blocks on page N (1-indexed)
  --type heading|…      filter by block_type
  --near <block-id>     neighborhood: blocks within ±window of this one
  --window K            reading-order distance around --near (default 5)
  --since-order N       cursor: return blocks with reading_order > N
  --limit N             cap records (default 500, max 1000)
  --search <substring>  text-match filter (same backend as the search
                        command; use this variant only to combine with
                        structural filters in one call)

Cursor pagination: use the last returned block's reading_order as
--since-order on the next call. The JSON envelope's meta.next_since_order
is the same value.

Blocks carry 'document_id' + 'document_version_number' for back-navigation.
Cite a block in a change via --cite-block or with a {"document_block_id": N}
fragment in --source-links.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}

		// Validate flags before any network I/O — avoid wasting a version-
		// resolution call when the user's args can't succeed anyway.
		if blocksSearch != "" && len(blocksSearch) < blocksSearchMinLen {
			return &UsageError{
				Arg:   "search",
				Usage: fmt.Sprintf("--search requires at least %d characters (pg_trgm indexes 3-char n-grams; shorter queries skip the index)", blocksSearchMinLen),
			}
		}

		params := map[string]string{}
		if blocksPage > 0 {
			params["page"] = strconv.Itoa(blocksPage)
		}
		if blocksType != "" {
			params["type"] = blocksType
		}
		if blocksSearch != "" {
			params["q"] = blocksSearch
		}
		if blocksSinceOrder > 0 {
			params["since_order"] = strconv.Itoa(blocksSinceOrder)
		}
		if blocksNear > 0 {
			params["near"] = strconv.Itoa(blocksNear)
		}
		if blocksWindow > 0 {
			params["window"] = strconv.Itoa(blocksWindow)
		}
		if blocksLimit > 0 {
			params["limit"] = strconv.Itoa(blocksLimit)
		}
		return runBlocksRequest(args[0], blocksVersion, params)
	},
}

// runBlocksRequest is the shared code path for 'documents blocks' and
// 'documents search'. Both hit the same /blocks endpoint; only the argument
// convention differs (blocks is filter-driven, search takes the query as a
// required positional).
func runBlocksRequest(docID string, version int, params map[string]string) error {
	versionNumber, err := resolveVersionNumber(docID, version)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/documents/%s/versions/%d/blocks", docID, versionNumber)
	raw, err := client.GetRaw(path, params)
	if err != nil {
		return err
	}
	if !printer.IsStructured() {
		var resp struct {
			Data []documentBlock `json:"data"`
			Meta struct {
				Count          int  `json:"count"`
				Limit          int  `json:"limit"`
				NextSinceOrder *int `json:"next_since_order"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"ID", "Order", "Page", "Type", "H-Lvl", "Chars", "Text preview"}
		rows := make([][]string, len(resp.Data))
		for i, b := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(b.ID),
				strconv.Itoa(b.ReadingOrder),
				derefInt(b.PageNumber),
				b.BlockType,
				derefInt(b.HeadingLevel),
				strconv.Itoa(b.CharLength),
				truncate(b.Text, 60),
			}
		}
		printer.Table(headers, rows)
		if resp.Meta.NextSinceOrder != nil && resp.Meta.Count >= resp.Meta.Limit {
			printer.Breadcrumb(fmt.Sprintf("Next page: --since-order %d", *resp.Meta.NextSinceOrder))
		}
		if len(resp.Data) == 0 {
			printer.Breadcrumb(fmt.Sprintf("No blocks matched. If you're looking for a value, try: kestrel documents search %s \"<phrase>\"", docID))
		}
	}
	printer.FinishRaw(raw)
	return nil
}

var (
	searchVersion int
	searchPage    int
	searchType    string
	searchLimit   int
)

var documentsSearchCmd = &cobra.Command{
	Use:   "search <doc-id> <query>",
	Short: "Find blocks by text content (the preferred way to locate values)",
	Long: `Trigram-indexed text search across block prose AND table cell text. The
primary way to locate a specific value (date, rent, party name, address,
expiration clause) in a parsed document — typically 5–10× cheaper than
walking the structure block-by-block.

Query must be at least 4 characters. Typos score partial matches, so
"commencment" will still hit "commencement". Returns results in reading
order (not similarity-ranked).

Scope with --page or --type when you already know the region
(e.g. --type table for rent schedules). To walk structurally when you
don't know what you're looking for, use 'kestrel documents blocks'
with --type, --page, --near, or --since-order.

Examples:
  kestrel documents search 42 "commencement"
  kestrel documents search 42 "base rent" --type table --agent
  kestrel documents search 42 "guarantor" --page 3`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		docID, query := args[0], args[1]
		if len(query) < blocksSearchMinLen {
			return &UsageError{
				Arg:   "query",
				Usage: fmt.Sprintf("search query must be at least %d characters (pg_trgm indexes 3-char n-grams; shorter queries skip the index)", blocksSearchMinLen),
			}
		}
		params := map[string]string{"q": query}
		if searchPage > 0 {
			params["page"] = strconv.Itoa(searchPage)
		}
		if searchType != "" {
			params["type"] = searchType
		}
		if searchLimit > 0 {
			params["limit"] = strconv.Itoa(searchLimit)
		}
		return runBlocksRequest(docID, searchVersion, params)
	},
}

var documentsBlockCmd = &cobra.Command{
	Use:     "block <block-id>",
	Aliases: []string{"block-show"},
	Short:   "Fetch a single document block",
	Long: `Confirm-before-cite: fetch one block by its opaque id. The response
includes the owning document's id + version_number so you can navigate back
to the version's full block list with 'kestrel documents blocks <doc-id>'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/document_blocks/"+args[0], nil)
		if err != nil {
			return err
		}
		if !printer.IsStructured() {
			var resp struct {
				Data documentBlock `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			renderBlockDetail(resp.Data)
		}
		printer.FinishRaw(raw)
		return nil
	},
}

// resolveVersionNumber returns the explicit --version if set, otherwise
// reads `current_version_number` from GET /documents/:id. This works even
// for unparsed documents — pages/blocks will return an empty array rather
// than 404 when no parse exists, which is a clearer signal than crashing
// out of the resolver.
func resolveVersionNumber(docID string, explicit int) (int, error) {
	if explicit > 0 {
		return explicit, nil
	}
	env, err := client.Get("/documents/"+docID, nil)
	if err != nil {
		return 0, err
	}
	var doc document
	if err := json.Unmarshal(env.Data, &doc); err != nil {
		return 0, fmt.Errorf("decoding document envelope: %w", err)
	}
	if doc.CurrentVersionNumber == nil || *doc.CurrentVersionNumber == 0 {
		return 0, fmt.Errorf("document %s has no current_version_number — pass --version explicitly", docID)
	}
	return *doc.CurrentVersionNumber, nil
}

// runParseWait polls GET /documents/:id/parse until the status is terminal
// or the timeout elapses. Renders a live progress line in TTY mode; emits
// a single envelope at the end in structured modes.
func runParseWait(docID string, timeoutSecs int) error {
	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}
	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	const pollInterval = 2 * time.Second
	tty := !printer.IsStructured()
	var lastRaw []byte

	for {
		raw, err := client.GetRaw("/documents/"+docID+"/parse", nil)
		if err != nil {
			return err
		}
		lastRaw = raw

		var resp struct {
			Data documentParse `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		p := resp.Data

		if tty {
			fmt.Fprint(os.Stderr, "\r\033[K", parseProgressLine(p))
		}

		switch p.Status {
		case "complete":
			if tty {
				fmt.Fprintln(os.Stderr) // finalize progress line
				renderParseDetail(p)
				printer.Breadcrumb(fmt.Sprintf("Browse blocks: kestrel documents blocks %s", docID))
			}
			printer.FinishRaw(raw)
			return nil
		case "failed":
			if tty {
				fmt.Fprintln(os.Stderr)
				renderParseDetail(p)
			}
			// Emit the final envelope so structured callers see the error_message,
			// but also raise a non-zero exit via APIError.
			printer.FinishRaw(raw)
			msg := "parse failed"
			if p.ErrorMessage != nil && *p.ErrorMessage != "" {
				msg = "parse failed: " + *p.ErrorMessage
			}
			return &api.APIError{StatusCode: 422, Message: msg, Code: "parse_failed"}
		}

		if time.Now().After(deadline) {
			if tty {
				fmt.Fprintln(os.Stderr)
			}
			printer.FinishRaw(lastRaw)
			return &api.APIError{
				StatusCode: 504,
				Message:    fmt.Sprintf("parse did not complete within %ds (last status: %s)", timeoutSecs, p.Status),
				Code:       "parse_timeout",
			}
		}
		time.Sleep(pollInterval)
	}
}

func parseProgressLine(p documentParse) string {
	line := fmt.Sprintf("Parse %s", p.Status)
	if p.Progress != nil {
		prog := p.Progress
		if prog.Stage != "" {
			line += " — " + prog.Stage
		}
		if prog.CurrentPage != nil && prog.TotalPages != nil && *prog.TotalPages > 0 {
			line += fmt.Sprintf(" page %d/%d", *prog.CurrentPage, *prog.TotalPages)
		}
		if prog.OverallPct != nil {
			line += fmt.Sprintf(" (%.0f%%)", *prog.OverallPct)
		}
	}
	return line
}

func renderParseDetail(p documentParse) {
	pairs := [][]string{
		{"Document", strconv.Itoa(p.DocumentID)},
		{"Version", strconv.Itoa(p.DocumentVersionNumber)},
		{"Status", p.Status},
		{"Engine", p.Engine},
		{"Engine version", deref(p.EngineVersion)},
		{"Pages", derefInt(p.PageCount)},
		{"Coordinates", yesNo(p.HasCoordinates)},
		{"Rendered PDF", yesNo(p.HasRenderedPDF)},
		{"Started", deref(p.StartedAt)},
		{"Completed", deref(p.CompletedAt)},
	}
	if p.ErrorMessage != nil && *p.ErrorMessage != "" {
		pairs = append(pairs, []string{"Error", *p.ErrorMessage})
	}
	printer.Detail(pairs)
	if len(p.ParseOptions) > 0 && string(p.ParseOptions) != "null" && string(p.ParseOptions) != "{}" {
		fmt.Println()
		fmt.Println("Parse options")
		fmt.Println("─────────────")
		pretty, _ := json.MarshalIndent(json.RawMessage(p.ParseOptions), "", "  ")
		fmt.Println(string(pretty))
	}
}

func renderBlockDetail(b documentBlock) {
	pairs := [][]string{
		{"ID", strconv.Itoa(b.ID)},
		{"Document", strconv.Itoa(b.DocumentID)},
		{"Version", strconv.Itoa(b.DocumentVersionNumber)},
		{"Page", derefInt(b.PageNumber)},
		{"Reading order", strconv.Itoa(b.ReadingOrder)},
		{"Block type", b.BlockType},
		{"Heading level", derefInt(b.HeadingLevel)},
		{"Characters", strconv.Itoa(b.CharLength)},
	}
	if b.Anchor != nil && *b.Anchor != "" {
		pairs = append(pairs, []string{"Anchor", *b.Anchor})
	}
	printer.Detail(pairs)
	if b.Text != "" {
		fmt.Println()
		fmt.Println("Text")
		fmt.Println("────")
		fmt.Println(b.Text)
	}
	if len(b.Bbox) > 0 && string(b.Bbox) != "null" {
		fmt.Println()
		fmt.Println("Bbox (normalized 0–1, top-left origin)")
		fmt.Println("──────────────────────────────────────")
		pretty, _ := json.MarshalIndent(json.RawMessage(b.Bbox), "", "  ")
		fmt.Println(string(pretty))
	}
	if len(b.Metadata) > 0 && string(b.Metadata) != "null" {
		fmt.Println()
		fmt.Println("Metadata")
		fmt.Println("────────")
		pretty, _ := json.MarshalIndent(json.RawMessage(b.Metadata), "", "  ")
		fmt.Println(string(pretty))
	}
}

// yesNo renders a bool as "yes"/"no" for detail tables.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// truncate shortens a string for table cells. Preserves runes (not bytes).
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func init() {
	documentsParseCmd.Flags().BoolVar(&parseWait, "wait", false, "Poll until the parse reaches complete or failed")
	documentsParseCmd.Flags().IntVar(&parseTimeoutSecs, "timeout", 300, "Seconds to wait when --wait is set")

	documentsPagesCmd.Flags().IntVar(&pagesVersion, "version", 0, "Specific version number (default: latest)")

	documentsBlocksCmd.Flags().IntVar(&blocksVersion, "version", 0, "Specific version number (default: latest)")
	documentsBlocksCmd.Flags().IntVar(&blocksPage, "page", 0, "Only blocks on this page (1-indexed)")
	documentsBlocksCmd.Flags().StringVar(&blocksType, "type", "", "Filter by block_type (heading|paragraph|list_item|table|figure|caption|code|formula|footnote|page_header|page_footer)")
	documentsBlocksCmd.Flags().StringVar(&blocksSearch, "search", "", "Case-insensitive text search (min 4 chars). For pure search, prefer `documents search <doc-id> <query>`")

	documentsSearchCmd.Flags().IntVar(&searchVersion, "version", 0, "Specific version number (default: latest)")
	documentsSearchCmd.Flags().IntVar(&searchPage, "page", 0, "Only match blocks on this page (1-indexed)")
	documentsSearchCmd.Flags().StringVar(&searchType, "type", "", "Only match blocks of this block_type (heading|paragraph|list_item|table|figure|caption|code|formula|footnote|page_header|page_footer)")
	documentsSearchCmd.Flags().IntVar(&searchLimit, "limit", 0, "Max matches per response (default 500, max 1000)")
	documentsBlocksCmd.Flags().IntVar(&blocksSinceOrder, "since-order", 0, "Return blocks with reading_order > N (cursor pagination)")
	documentsBlocksCmd.Flags().IntVar(&blocksNear, "near", 0, "Anchor block id for a neighborhood fetch")
	documentsBlocksCmd.Flags().IntVar(&blocksWindow, "window", 0, "Reading-order distance around --near (default 5)")
	documentsBlocksCmd.Flags().IntVar(&blocksLimit, "limit", 0, "Max blocks per response (default 500, max 1000)")

	documentsCmd.AddCommand(documentsParseCmd)
	documentsCmd.AddCommand(documentsPagesCmd)
	documentsCmd.AddCommand(documentsSearchCmd)
	documentsCmd.AddCommand(documentsBlocksCmd)
	documentsCmd.AddCommand(documentsBlockCmd)
}
