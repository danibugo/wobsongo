package cmd

import (
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/spf13/cobra"
)

// staticDumpDir is where captured Docling JSON fixtures are written.
const staticDumpDir = "static"

var (
	doclingDumpFile  string
	doclingDumpURL   string
	doclingDumpOut   string
	doclingDumpApply bool
)

// doclingDumpCmd calls Docling Serve directly (bypassing the ingestion
// pipeline entirely) and saves its raw JSON response — for building up a
// set of fixtures to use in tests later, not for inserting real documents.
var doclingDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Fetch Docling Serve's raw JSON output for a document and save it as a test fixture",
	Long: "Sends a local file or URL to Docling Serve and writes its raw JSON\n" +
		"response under static/ for later use as a test fixture. A local file is\n" +
		"uploaded directly to Docling Serve's file-upload endpoint (never via\n" +
		"S3) since a cloud-hosted Docling instance can't reach a local dev\n" +
		"S3/MinIO; a --url source is instead fetched by Docling Serve itself.\n\n" +
		"Without --apply, this is a dry run: the source is read and validated,\n" +
		"but Docling Serve is never called and no file is written.",
	Run: runDoclingDump,
}

func init() {
	doclingDumpCmd.Flags().
		StringVarP(&doclingDumpFile, "file", "f", "", "Local path to the document file")
	doclingDumpCmd.Flags().
		StringVarP(&doclingDumpURL, "url", "u", "", "Direct URL to the document file")
	doclingDumpCmd.Flags().
		StringVarP(&doclingDumpOut, "out", "o", "", "Output filename under static/ (default: <source-basename>.json)")
	doclingDumpCmd.Flags().
		BoolVar(&doclingDumpApply, "apply", false, "Actually call Docling and write the output file (default is a dry run)")

	doclingDumpCmd.MarkFlagsOneRequired("file", "url")
	doclingDumpCmd.MarkFlagsMutuallyExclusive("file", "url")

	doclingCmd.AddCommand(doclingDumpCmd)
}

func runDoclingDump(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()
	cfg := appconfig.NewConfig(EnvFile)

	f, originalFilename, cleanup, err := openDocumentSource(ctx, doclingDumpFile, doclingDumpURL)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		cmd.PrintErrf("Failed to read source: %s\n", err.Error())
		os.Exit(1) //nolint:gocritic // process exit; same accepted pattern as cmd/server.go
		return
	}

	outPath := doclingDumpOutputPath(originalFilename)

	if !doclingDumpApply {
		cmd.Printf(
			"Dry run: would send %s to Docling Serve and write its output to %s\n",
			originalFilename,
			outPath,
		)
		cmd.Println("Re-run with --apply to actually call Docling and write the file.")
		return
	}

	doclingClient := external.NewDoclingClient(cfg.DoclingBaseURL)

	var raw []byte
	if doclingDumpFile != "" {
		raw, err = doclingClient.ConvertFileRaw(ctx, originalFilename, f)
	} else {
		raw, err = doclingClient.FetchRawFromURL(ctx, doclingDumpURL)
	}
	if err != nil {
		cmd.PrintErrf("Docling request failed: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		cmd.PrintErrf("Failed to create output directory: %s\n", err.Error())
		os.Exit(1)
		return
	}
	if err := os.WriteFile(outPath, raw, 0o600); err != nil {
		cmd.PrintErrf("Failed to write output file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	cmd.Printf("Wrote Docling output to %s (%d bytes)\n", outPath, len(raw))
}

// doclingDumpOutputPath resolves the destination path under static/ for the
// captured Docling response, defaulting to the source's basename with a
// .json extension when --out isn't given.
func doclingDumpOutputPath(originalFilename string) string {
	name := doclingDumpOut
	if name == "" {
		base := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
		name = base + ".json"
	}
	return filepath.Join(staticDumpDir, name)
}
