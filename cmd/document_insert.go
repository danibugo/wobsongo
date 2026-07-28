package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/spf13/cobra"
)

// allowedDocumentExtensions mirrors internal/validation/s3.go's
// regexSHA256Filename whitelist — the S3 key must match documents/<sha256>.<ext>.
var allowedDocumentExtensions = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "webp": true, "avif": true,
	"pdf": true, "docx": true, "doc": true, "rtf": true, "html": true, "md": true,
}

var (
	documentInsertFile      string
	documentInsertURL       string
	documentInsertTitle     string
	documentInsertPublisher string
	documentInsertYear      int
	documentInsertLanguage  string
	documentInsertApply     bool
)

// documentInsertCmd inserts a document using the service layer directly —
// no HTTP handler is involved.
var documentInsertCmd = &cobra.Command{
	Use:   "insert",
	Short: "Insert a document from a local file or a URL",
	Long: "Uploads the file to storage and registers it as a document by calling\n" +
		"the service layer directly. Page count is left at 0 and backfilled by\n" +
		"the ingestion pipeline once Docling actually parses the file. If --title\n" +
		"is omitted, it is also backfilled from Docling's title once parsed.\n\n" +
		"Without --apply, this is a dry run: the file is read, hashed, and\n" +
		"validated (including a live conflict check against S3), but nothing\n" +
		"is uploaded and no document is inserted. If a document with the same\n" +
		"SHA256 already exists, insertion is skipped (no upload, no duplicate\n" +
		"row) and the existing document is reported instead.",
	Run: runDocumentInsert,
}

func init() {
	documentInsertCmd.Flags().
		StringVarP(&documentInsertFile, "file", "f", "", "Local path to the document file")
	documentInsertCmd.Flags().
		StringVarP(&documentInsertURL, "url", "u", "", "Direct URL to the document file")
	documentInsertCmd.Flags().
		StringVarP(&documentInsertTitle, "title", "t", "", "Document title (backfilled from Docling if omitted)")
	documentInsertCmd.Flags().
		StringVar(&documentInsertPublisher, "publisher", "", "Publisher name")
	documentInsertCmd.Flags().IntVar(&documentInsertYear, "year", 0, "Publication year")
	documentInsertCmd.Flags().
		StringVar(&documentInsertLanguage, "language", "", "Document language: \"en\" or \"fr\" (required — never auto-detected)")
	documentInsertCmd.Flags().
		BoolVar(&documentInsertApply, "apply", false, "Actually upload the file and insert the document (default is a dry run)")

	documentInsertCmd.MarkFlagsOneRequired("file", "url")
	documentInsertCmd.MarkFlagsMutuallyExclusive("file", "url")
	_ = documentInsertCmd.MarkFlagRequired("language")

	documentCmd.AddCommand(documentInsertCmd)
}

// registrationOnlyParseDocumentWorker satisfies River's client-side check
// that a job's kind is registered before Insert/InsertTx is allowed to
// enqueue it. This CLI process never calls riverClient.Start(), so Work is
// never actually invoked — it exists purely to make the registration check pass.
type registrationOnlyParseDocumentWorker struct {
	river.WorkerDefaults[queue.ParseDocumentDTO]
}

func (*registrationOnlyParseDocumentWorker) Work(
	context.Context,
	*river.Job[queue.ParseDocumentDTO],
) error {
	return nil
}

func runDocumentInsert(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()
	cfg := appconfig.NewConfig(EnvFile)

	pool, err := pgxpool.New(ctx, cfg.PostgresURI)
	if err != nil {
		cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
		os.Exit(1)
		return
	}
	defer pool.Close()

	if err := appconfig.IsS3OK(cfg.S3Config); err != nil {
		cmd.PrintErrf("Config error: %s\n", err.Error())
		os.Exit(1) //nolint:gocritic // process exit; same accepted pattern as cmd/server.go
		return
	}
	mediaProvider, err := repo.NewS3Provider(ctx, cfg.S3Config)
	if err != nil {
		cmd.PrintErrf("Failed to initialize S3 media provider: %s\n", err.Error())
		os.Exit(1)
		return
	}
	mediaService := service.NewMediaService(mediaProvider)

	workers := river.NewWorkers()
	river.AddWorker(workers, &registrationOnlyParseDocumentWorker{})
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		cmd.PrintErrf("Failed to initialize job queue client: %s\n", err.Error())
		os.Exit(1)
		return
	}

	documentRepo := repo.NewDocumentRepo(db.New(pool), pool, riverClient)
	documentService := service.NewDocumentService(documentRepo)

	req, existing, cleanup, err := buildCreateDocumentDTO(
		ctx,
		documentService,
		mediaService,
		documentInsertApply,
	)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		cmd.PrintErrf("Failed to prepare document: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if existing != nil {
		cmd.Printf(
			"Document already exists as %s (title=%q, sha256=%s) — skipping upload and insert.\n",
			existing.ID, existing.Title, existing.SHA256,
		)
		return
	}

	if !documentInsertApply {
		cmd.Printf(
			"Dry run: would insert document (title=%q, filename=%s, file_key=%s, sha256=%s, filetype=%s, filesize=%d, language=%s)\n",
			req.Title,
			req.Filename,
			req.FileKey,
			req.SHA256,
			req.Filetype,
			req.Filesize,
			req.Language,
		)
		cmd.Println("Re-run with --apply to actually upload the file and insert the document.")
		return
	}

	doc, err := documentService.Create(ctx, req)
	if err != nil {
		cmd.PrintErrf("Failed to insert document: %s\n", err.Error())
		os.Exit(1)
		return
	}

	cmd.Printf(
		"Inserted document %s (title=%q, file_key=%s, sha256=%s)\n",
		doc.ID, doc.Title, doc.FileURL, doc.SHA256,
	)
}

// buildCreateDocumentDTO reads the source file (local path or URL) and
// assembles the DTO for DocumentService.Create. Before doing any S3 work it
// checks whether a document with the same SHA256 already exists; if so, it
// returns that document (with a nil DTO) so the caller can skip the upload
// and insert entirely. Otherwise it always presigns the upload (which also
// runs S3's live conflict check for the computed key), but only uploads the
// file when apply is true — callers wanting a dry run pass apply=false and
// get validation without any S3 write. The returned cleanup func (never nil
// once a source was opened) should always be deferred, even on error.
func buildCreateDocumentDTO(
	ctx context.Context,
	documentService *service.DocumentService,
	mediaService *service.MediaService,
	apply bool,
) (*dto.CreateDocumentDTO, *model.Document, func(), error) {
	f, originalFilename, cleanup, err := openDocumentSource(
		ctx,
		documentInsertFile,
		documentInsertURL,
	)
	if err != nil {
		return nil, nil, cleanup, err
	}

	sha256Hex, contentType, size, err := hashAndSniff(f)
	if err != nil {
		return nil, nil, cleanup, err
	}

	existing, err := documentService.GetBySHA256(ctx, sha256Hex)
	if err == nil {
		return nil, existing, cleanup, nil
	}
	if !errors.Is(err, data.ErrNotFound) {
		return nil, nil, cleanup, fmt.Errorf("failed to check for existing document: %w", err)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(originalFilename), "."))
	if !allowedDocumentExtensions[ext] {
		return nil, nil, cleanup, fmt.Errorf(
			"unsupported file extension %q (allowed: jpg, jpeg, png, webp, avif, pdf, docx, doc, rtf, html, md)",
			ext,
		)
	}

	storageFilename := sha256Hex + "." + ext
	policy, err := mediaService.GetPresignedPOSTURL(
		ctx,
		data.DocumentUploadIntent,
		storageFilename,
		contentType,
	)
	if err != nil {
		return nil, nil, cleanup, fmt.Errorf("failed to get presigned upload URL: %w", err)
	}

	if apply {
		if err := uploadViaPresignedPost(ctx, policy, storageFilename, f); err != nil {
			return nil, nil, cleanup, fmt.Errorf("failed to upload file: %w", err)
		}
	}

	return &dto.CreateDocumentDTO{
		SHA256:          sha256Hex,
		FileKey:         policy.Prefix + storageFilename,
		Title:           documentInsertTitle,
		Filename:        originalFilename,
		Filetype:        contentType,
		Filesize:        size,
		PageCount:       0, // backfilled by ParseDocumentWorker once Docling parses it
		PublisherName:   documentInsertPublisher,
		PublicationYear: documentInsertYear,
		Language:        documentInsertLanguage,
	}, nil, cleanup, nil
}

// openDocumentSource returns a seekable handle to the source file (opened
// directly for filePath; downloaded to a temp file for sourceURL, since HTTP
// response bodies aren't seekable), its original filename, and a cleanup
// func that closes (and, for downloads, removes) it. Exactly one of
// filePath/sourceURL is expected to be set. Shared by every CLI command
// that accepts a --file/--url document source (document insert, docling dump).
func openDocumentSource(
	ctx context.Context,
	filePath, sourceURL string,
) (*os.File, string, func(), error) {
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to open %s: %w", filePath, err)
		}
		return f, filepath.Base(filePath), func() { _ = f.Close() }, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to build download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to download %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", nil, fmt.Errorf(
			"failed to download %s: status %d",
			sourceURL,
			resp.StatusCode,
		)
	}

	tmp, err := os.CreateTemp("", "wob-document-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to download %s: %w", sourceURL, err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to rewind downloaded file: %w", err)
	}

	// Prefer the real filename from Content-Disposition when the server
	// sends one — some download endpoints (e.g. MinIO console's "Share"
	// links, which wrap the actual presigned URL behind an opaque
	// /api/v1/download-shared-object/<base64> path) have no usable
	// extension in the URL path itself, so falling back to path.Base(URL)
	// unconditionally would reject a perfectly valid file.
	filename := filenameFromContentDisposition(resp.Header.Get("Content-Disposition"))
	if filename == "" {
		parsed, err := url.Parse(sourceURL)
		if err != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("failed to parse URL: %w", err)
		}
		filename = path.Base(parsed.Path)
	}

	return tmp, filename, cleanup, nil
}

// filenameFromContentDisposition extracts and returns the base filename from
// a Content-Disposition header value, or "" if header is empty, unparseable,
// or carries no filename. The filename param is unescaped and path.Base'd
// before returning — some servers (e.g. MinIO console) send it
// percent-encoded and/or including a bucket/folder prefix.
func filenameFromContentDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	filename := params["filename"]
	if filename == "" {
		return ""
	}
	if unescaped, err := url.QueryUnescape(filename); err == nil {
		filename = unescaped
	}
	return path.Base(filename)
}

// hashAndSniff computes the SHA256 hash, detects the content type, and
// reports the size of f, leaving f positioned at the start.
func hashAndSniff(f *os.File) (sha256Hex, contentType string, size int64, err error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", "", 0, fmt.Errorf("failed to hash file: %w", err)
	}
	sha256Hex = hex.EncodeToString(hasher.Sum(nil))

	info, err := f.Stat()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to stat file: %w", err)
	}
	size = info.Size()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, fmt.Errorf("failed to rewind file: %w", err)
	}
	sniff := make([]byte, 512)
	n, err := f.Read(sniff)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", 0, fmt.Errorf("failed to sniff content type: %w", err)
	}
	contentType = http.DetectContentType(sniff[:n])

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, fmt.Errorf("failed to rewind file: %w", err)
	}
	return sha256Hex, contentType, size, nil
}

// uploadViaPresignedPost uploads r's contents to policy.URL using the same
// multipart/form-data presigned-POST flow a browser client would use —
// this CLI is just another client of the existing media upload API.
func uploadViaPresignedPost(
	ctx context.Context,
	policy *model.POSTUploadPolicy,
	filename string,
	r io.Reader,
) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for k, v := range policy.FormFields {
		if err := writer.WriteField(k, v); err != nil {
			return fmt.Errorf("failed to write form field %q: %w", k, err)
		}
	}

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, policy.URL, body)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
