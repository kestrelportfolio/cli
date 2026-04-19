package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

type documentVersion struct {
	VersionNumber int    `json:"version_number"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	ByteSize      int    `json:"byte_size"`
	Checksum      string `json:"checksum"`
}

type documentParent struct {
	Type string `json:"type"`
	ID   int    `json:"id"`
}

type document struct {
	ID            int              `json:"id"`
	Name          string           `json:"name"`
	Category1     *string          `json:"category_1"`
	Category2     *string          `json:"category_2"`
	DocumentDate  *string          `json:"document_date"`
	VersionCount  int              `json:"version_count"`
	Parents       []documentParent `json:"parents"`
	LatestVersion *documentVersion `json:"latest_version"`
	DownloadURL   *string          `json:"download_url"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
}

var documentsCmd = &cobra.Command{
	Use:   "documents",
	Short: "Inspect and download documents",
}

var documentsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show document metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}
		raw, err := client.GetRaw("/documents/"+args[0], nil)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			printer.JSON(raw)
			return nil
		}
		var resp struct {
			Data document `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		d := resp.Data
		pairs := [][]string{
			{"ID", strconv.Itoa(d.ID)},
			{"Name", d.Name},
			{"Category 1", deref(d.Category1)},
			{"Category 2", deref(d.Category2)},
			{"Document date", deref(d.DocumentDate)},
			{"Version count", strconv.Itoa(d.VersionCount)},
		}
		if d.LatestVersion != nil {
			v := d.LatestVersion
			pairs = append(pairs,
				[]string{"Latest version", strconv.Itoa(v.VersionNumber)},
				[]string{"Filename", v.Filename},
				[]string{"Content type", v.ContentType},
				[]string{"Byte size", strconv.Itoa(v.ByteSize)},
				[]string{"Checksum", v.Checksum},
			)
		}
		for _, p := range d.Parents {
			pairs = append(pairs, []string{"Parent", fmt.Sprintf("%s #%d", p.Type, p.ID)})
		}
		pairs = append(pairs,
			[]string{"Created", d.CreatedAt},
			[]string{"Updated", d.UpdatedAt},
		)
		printer.Detail(pairs)
		return nil
	},
}

var (
	documentsDownloadVersion int
	documentsDownloadOutput  string
	documentsDownloadURLOnly bool
)

var documentsDownloadCmd = &cobra.Command{
	Use:   "download <id>",
	Short: "Download a document's file (or print the signed URL with --url)",
	Long: `Fetches the file bytes for a document. By default writes to the filename
from the latest version; use -o to override, or - to write to stdout.

Use --version N to fetch a specific version, or --url to print only the
short-lived signed URL without downloading.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLogin(); err != nil {
			return err
		}

		path := "/documents/" + args[0] + "/download"
		if documentsDownloadVersion > 0 {
			path = fmt.Sprintf("/documents/%s/versions/%d/download", args[0], documentsDownloadVersion)
		}

		signedURL, err := client.GetRedirect(path)
		if err != nil {
			return err
		}

		if documentsDownloadURLOnly {
			fmt.Println(signedURL)
			return nil
		}

		// Determine destination filename.
		out := documentsDownloadOutput
		if out == "" {
			// Fetch metadata to get the original filename from latest_version.
			raw, metaErr := client.GetRaw("/documents/"+args[0], nil)
			if metaErr == nil {
				var meta struct {
					Data document `json:"data"`
				}
				if err := json.Unmarshal(raw, &meta); err == nil && meta.Data.LatestVersion != nil {
					out = meta.Data.LatestVersion.Filename
				}
			}
			if out == "" {
				out = "document-" + args[0]
			}
		}

		// Fetch the signed URL's bytes. The signed URL doesn't need our auth header.
		resp, err := http.Get(signedURL)
		if err != nil {
			return fmt.Errorf("fetching signed URL: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("signed URL returned status %d", resp.StatusCode)
		}

		var dst io.Writer
		if out == "-" {
			dst = os.Stdout
		} else {
			f, err := os.Create(out)
			if err != nil {
				return fmt.Errorf("creating %s: %w", out, err)
			}
			defer f.Close()
			dst = f
		}

		n, err := io.Copy(dst, resp.Body)
		if err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		if out != "-" {
			printer.Success(fmt.Sprintf("Wrote %d bytes to %s", n, out))
		}
		return nil
	},
}

func init() {
	documentsDownloadCmd.Flags().IntVar(&documentsDownloadVersion, "version", 0, "Download a specific version (default: latest)")
	documentsDownloadCmd.Flags().StringVarP(&documentsDownloadOutput, "output", "o", "", "Write to this path (- for stdout)")
	documentsDownloadCmd.Flags().BoolVar(&documentsDownloadURLOnly, "url", false, "Print the signed URL instead of downloading")

	documentsCmd.AddCommand(documentsShowCmd)
	documentsCmd.AddCommand(documentsDownloadCmd)
	rootCmd.AddCommand(documentsCmd)
}
