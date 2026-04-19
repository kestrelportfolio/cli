package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/spf13/cobra"
)

type abstractionSourceDoc struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Category1    *string `json:"category_1"`
	DocumentDate *string `json:"document_date"`
	DownloadURL  *string `json:"download_url"`
	CreatedAt    string  `json:"created_at"`
}

var (
	addDocName string
)

var abstractionsAddDocCmd = &cobra.Command{
	Use:   "add-doc <abstraction-id> <file>",
	Short: "Upload a file and attach it as a source document",
	Long: `Uploads <file> as a new Document and attaches it to the abstraction as a
source in one step. The filename (from the path, or --name) is stored as the
document's display name. PDF mime type is detected from the .pdf extension;
other files upload as application/octet-stream.

Errors:
  * 403 forbidden   — token lacks lease_abstractions#create
  * 404 not_found   — abstraction missing (or document id mismatch on attach)
  * 422             — validation failure (e.g. file too large)`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		absID, filePath := args[0], args[1]

		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", filePath, err)
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			return fmt.Errorf("stat %s: %w", filePath, err)
		}
		if stat.Size() == 0 {
			return fmt.Errorf("file is empty: %s", filePath)
		}

		name := addDocName
		if name == "" {
			name = filepath.Base(filePath)
		}

		contentType := "application/octet-stream"
		if strings.EqualFold(filepath.Ext(filePath), ".pdf") {
			contentType = "application/pdf"
		}

		uploadPath := "/documents?name=" + url.QueryEscape(name)
		uploadEnv, err := client.Upload(uploadPath, f, contentType, stat.Size())
		if err != nil {
			return err
		}

		var doc document
		if err := json.Unmarshal(uploadEnv.Data, &doc); err != nil {
			return fmt.Errorf("parsing uploaded document: %w", err)
		}

		// Attach to the abstraction.
		attachEnv, err := client.Post(
			"/abstractions/"+absID+"/source_documents",
			map[string]any{"document_id": doc.ID},
		)
		if err != nil {
			// Upload succeeded but attach failed — give the user the escape hatch.
			printer.Errorf("upload succeeded (document #%d) but attach failed: %v", doc.ID, err)
			printer.Breadcrumb(fmt.Sprintf("Retry attach: POST /abstractions/%s/source_documents with {\"document_id\": %d}", absID, doc.ID))
			printer.Breadcrumb(fmt.Sprintf("Or clean up: kestrel abstractions remove-doc %s --document-id %d", absID, doc.ID))
			return err
		}

		if printer.IsJSON() {
			pretty, _ := json.MarshalIndent(attachEnv, "", "  ")
			fmt.Println(string(pretty))
			return nil
		}

		var src abstractionSourceDoc
		if err := json.Unmarshal(attachEnv.Data, &src); err != nil {
			return fmt.Errorf("parsing source document: %w", err)
		}
		printer.Success(fmt.Sprintf("Uploaded %s (%d bytes) as document #%d", name, stat.Size(), doc.ID))
		printer.Success(fmt.Sprintf("Attached to abstraction #%s", absID))
		printer.Breadcrumb(fmt.Sprintf("Draft a change citing this doc: kestrel abstractions changes create %s --source-links '[{\"document_id\":%d}]' ...", absID, doc.ID))
		return nil
	},
}

var (
	removeDocID int
)

var abstractionsRemoveDocCmd = &cobra.Command{
	Use:   "remove-doc <abstraction-id>",
	Short: "Destroy a source document (cascade-removes the join and any pending/rejected citing changes)",
	Long: `Destroys the underlying Document via DELETE /documents/:id. The join to
this abstraction is cleared by FK cascade. Any AbstractionChanges citing the
document are destroyed if pending/rejected; if any are approved or applied,
the API refuses with 422 cited_by_locked_change.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		if removeDocID == 0 {
			return fmt.Errorf("--document-id is required")
		}

		env, err := client.Delete("/documents/" + strconv.Itoa(removeDocID))
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && apiErr.Code == "cited_by_locked_change" {
				printer.Errorf("document #%d is cited by an approved or applied change — reject it in the web UI before removing", removeDocID)
			}
			return err
		}

		if printer.IsJSON() {
			pretty, _ := json.MarshalIndent(env, "", "  ")
			fmt.Println(string(pretty))
			return nil
		}

		printer.Success(fmt.Sprintf("Removed document #%d", removeDocID))
		if env != nil && len(env.Data) > 0 {
			var cascade struct {
				DestroyedDraftedChanges  int `json:"destroyed_drafted_changes"`
				DetachedFromAbstractions int `json:"detached_from_abstractions"`
			}
			if err := json.Unmarshal(env.Data, &cascade); err == nil {
				if cascade.DestroyedDraftedChanges > 0 {
					fmt.Fprintf(os.Stderr, "  Destroyed %d drafted change(s) that cited it\n", cascade.DestroyedDraftedChanges)
				}
				if cascade.DetachedFromAbstractions > 0 {
					fmt.Fprintf(os.Stderr, "  Detached from %d abstraction(s)\n", cascade.DetachedFromAbstractions)
				}
			}
		}
		return nil
	},
}

var abstractionsSourcesPage int
var abstractionsSourcesCmd = &cobra.Command{
	Use:   "sources <abstraction-id>",
	Short: "List source documents attached to an abstraction",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		params := map[string]string{}
		if abstractionsSourcesPage > 1 {
			params["page"] = strconv.Itoa(abstractionsSourcesPage)
		}
		raw, err := client.GetRaw("/abstractions/"+args[0]+"/source_documents", params)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data []abstractionSourceDoc `json:"data"`
			Meta *struct {
				Page     int  `json:"page"`
				NextPage *int `json:"next_page"`
				Count    int  `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		headers := []string{"Doc ID", "Name", "Category", "Date", "Attached"}
		rows := make([][]string, len(resp.Data))
		for i, s := range resp.Data {
			rows[i] = []string{
				strconv.Itoa(s.ID),
				s.Name,
				deref(s.Category1),
				deref(s.DocumentDate),
				s.CreatedAt,
			}
		}
		printer.Table(headers, rows)
		if resp.Meta != nil {
			printer.PaginationHint(resp.Meta.NextPage, resp.Meta.Count)
		}
		return nil
	},
}

func init() {
	abstractionsAddDocCmd.Flags().StringVar(&addDocName, "name", "", "Override the filename stored on the document")

	abstractionsRemoveDocCmd.Flags().IntVar(&removeDocID, "document-id", 0, "ID of the document to destroy (required)")

	abstractionsSourcesCmd.Flags().IntVar(&abstractionsSourcesPage, "page", 1, "Page number")

	abstractionsCmd.AddCommand(abstractionsAddDocCmd)
	abstractionsCmd.AddCommand(abstractionsRemoveDocCmd)
	abstractionsCmd.AddCommand(abstractionsSourcesCmd)
}
