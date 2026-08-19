package repo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/validation"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/cors"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/patrickmn/go-cache"
)

// S3Provider implements MediaUploadProvider using S3-compatible storage.
type S3Provider struct {
	client        *minio.Client
	bucketName    string
	cache         *cache.Cache
	publicExpiry  int64 // 3600 for avatars
	privateExpiry int64 // 900 for submissions
	intentExpiry  map[data.MediaUploadIntent]int64
	logger        *slog.Logger
}

// Ensure S3Provider implements MediaUploadProvider interface
var _ data.MediaUploadProvider = (*S3Provider)(nil)

// Ensure S3Provider implements MediaStorageAdmin interface
var _ data.MediaStorageAdmin = (*S3Provider)(nil)

// Ensure S3Provider implements RawObjectStore interface
var _ data.RawObjectStore = (*S3Provider)(nil)

// NewS3Provider creates a new S3Provider instance.
// It ensures the bucket exists, creating it if necessary.
func NewS3Provider(ctx context.Context, config *appconfig.S3Config) (*S3Provider, error) {
	logger := appconfig.NewConfig().Logger
	// Initialize MinIO client (works with DO/Hetzner Object Storage too)
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure:       config.UseSSL,
		Region:       config.Region,
		BucketLookup: 0,
	})
	if err != nil {
		return nil, err
	}

	// Check if bucket exists, create if it doesn't
	exists, err := client.BucketExists(ctx, config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if !exists {
		logger.Info(
			"Bucket does not exist, creating",
			"bucket_name", config.BucketName,
		)
		err = client.MakeBucket(ctx, config.BucketName, minio.MakeBucketOptions{
			Region: config.Region,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket '%s': %w", config.BucketName, err)
		}
		logger.Info(
			"Bucket created successfully",
			"bucket_name", config.BucketName,
		)
	} else {
		logger.Info(
			"Bucket already exists",
			"bucket_name", config.BucketName,
		)
	}

	// Presigned GET URLs (e.g. for the PDF viewer) are fetched directly by
	// browser JS (pdf.js range requests), which the browser treats as a
	// cross-origin request against this bucket's host — without CORS the
	// fetch is blocked outright. AllowedOrigin "*" is safe here: the
	// presigned URL's signature/expiry is the real access-control boundary,
	// not the requesting origin. Idempotent, so safe to call every startup.
	corsConfig := &cors.Config{
		CORSRules: []cors.Rule{
			{
				AllowedMethod: []string{"GET", "HEAD"},
				AllowedOrigin: []string{"*"},
				AllowedHeader: []string{"Range", "If-None-Match", "If-Modified-Since"},
				ExposeHeader:  []string{"ETag", "Content-Range", "Content-Length", "Accept-Ranges"},
			},
		},
	}
	// Best-effort: some S3-compatible servers (confirmed: this deployment's
	// MinIO, RELEASE.2024-12-18) don't implement PutBucketCors at all, even
	// though the client SDK exposes the call — failing startup over that
	// would take document upload/download down along with it, for a feature
	// (the PDF viewer's direct-to-storage fetch) that isn't essential to core
	// operation.
	if err := client.SetBucketCors(ctx, config.BucketName, corsConfig); err != nil {
		logger.Warn(
			"failed to set bucket CORS — browser-side fetches against presigned URLs (e.g. the PDF viewer) may be blocked by CORS until this is resolved",
			"bucket_name",
			config.BucketName,
			"error",
			err,
		)
	}

	return &S3Provider{
		client:        client,
		bucketName:    config.BucketName,
		cache:         cache.New(cache.NoExpiration, 5*time.Minute),
		publicExpiry:  3600,
		privateExpiry: 900,
		intentExpiry:  map[data.MediaUploadIntent]int64{},
		logger:        logger,
	}, nil
}

// GetPresignedPOSTURL generates a presigned POST URL for uploading media.
func (s *S3Provider) GetPresignedPOSTURL(
	ctx context.Context,
	intent data.MediaUploadIntent,
	filename, contentType string,
) (*url.URL, map[string]string, error) {
	// We combine the intent and filename to create a folder structure.
	// e.g., "avatars/user-123.jpg" or "submissions/video.mp4"
	// Ensure 'intent' is converted to a string appropriate for a folder name.
	objectKey := data.ObjectPrefixForIntent(intent) + filename

	// Check for existing files with the same name to prevent overwriting.
	if intent == data.DocumentUploadIntent {
		_, err := s.client.StatObject(ctx, s.bucketName, objectKey, minio.StatObjectOptions{})
		if err == nil {
			return nil, nil, data.ErrConflict
		}

		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			return nil, nil, fmt.Errorf("failed to check existing object: %w", err)
		}
	}

	policy := minio.NewPostPolicy()
	if err := policy.SetBucket(s.bucketName); err != nil {
		return nil, nil, err
	}

	if err := policy.SetKey(objectKey); err != nil {
		return nil, nil, err
	}

	// Uses per-intent expiry if defined, otherwise falls back to privateExpiry (900s)
	expiry := s.privateExpiry
	if intentExp, ok := s.intentExpiry[intent]; ok {
		expiry = intentExp
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiry) * time.Second)
	if err := policy.SetExpires(expiresAt); err != nil {
		return nil, nil, err
	}

	// This ensures the client uploads exactly the file type they claimed they would.
	if contentType != "" {
		if err := policy.SetContentType(contentType); err != nil {
			return nil, nil, err
		}
	}

	// Set Content-Length Range (1KB to 5MB)
	if err := policy.SetContentLengthRange(1024, 50*1024*1024); err != nil {
		return nil, nil, err
	}

	// Generate the presigned POST policy
	u, formData, err := s.client.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return nil, nil, err
	}

	return u, formData, nil
}

// GetPresignedGETURL generates a presigned GET URL for accessing media.
func (s *S3Provider) GetPresignedGETURL(
	ctx context.Context,
	s3Key string,
	expirySeconds int64,
) (string, error) {
	if s3Key == "" {
		return "", ErrEmptyMediaKey
	}
	// Check if the key is direct URL.
	if strings.HasPrefix(s3Key, "https://") {
		return s3Key, nil
	}
	if !validation.ValidateS3PrefixAndFile(s3Key) {
		return "", fmt.Errorf("%w: %s", ErrInvalidMalformedMediaKey, s3Key)
	}
	cachedURL, found := s.cache.Get(s3Key)
	if found {
		if urlStr, ok := cachedURL.(string); ok {
			return urlStr, nil
		}
		// Invalid cache entry - delete it and regenerate
		s.cache.Delete(s3Key)
	}

	presignedURL, err := s.client.PresignedGetObject(
		ctx,
		s.bucketName,
		s3Key,
		time.Duration(expirySeconds)*time.Second,
		nil, // request params
	)
	if err != nil {
		return "", err
	}
	s.cache.Set(s3Key, presignedURL.String(), time.Duration(expirySeconds)*time.Second)
	return presignedURL.String(), nil
}

// GetPresignedGETURLs generates presigned GET URLs for multiple S3 keys concurrently.
// This method is optimized for batch operations and leverages caching.
// Returns a map of s3Key -> presignedURL. Keys with errors are logged but omitted from the result.
// Uses bounded concurrency (worker pool) to prevent goroutine explosion on large key sets.
func (s *S3Provider) GetPresignedGETURLs(
	ctx context.Context,
	s3Keys []string,
	expirySeconds int64,
) (map[string]string, error) {
	if len(s3Keys) == 0 {
		return make(map[string]string), nil
	}

	// Deduplicate and filter keys (skip empty keys)
	seen := make(map[string]struct{}, len(s3Keys))
	uniqueKeys := make([]string, 0, len(s3Keys))
	for _, key := range s3Keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniqueKeys = append(uniqueKeys, key)
	}

	if len(uniqueKeys) == 0 {
		return make(map[string]string), nil
	}

	// Result map and mutex for concurrent writes
	result := make(map[string]string, len(uniqueKeys))
	var mu sync.Mutex

	// Bounded worker pool for concurrent processing of keys
	const maxWorkers = 10
	numWorkers := min(maxWorkers, len(uniqueKeys))

	jobs := make(chan string, numWorkers)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Start worker goroutines
	for range numWorkers {
		go func() {
			defer wg.Done()
			for s3Key := range jobs {
				// Use the existing GetPresignedGETURL method which has validation and caching
				presignedURL, err := s.GetPresignedGETURL(ctx, s3Key, expirySeconds)
				if err != nil {
					// Log error but don't fail the entire batch
					s.logger.Error(
						"Failed to generate presigned URL for key in batch",
						"key", s3Key,
						"error", err,
					)
					continue
				}

				// Thread-safe write to result map
				mu.Lock()
				result[s3Key] = presignedURL
				mu.Unlock()
			}
		}()
	}

	// Feed jobs to workers
	for _, key := range uniqueKeys {
		jobs <- key
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	return result, nil
}

// ListObjects returns all S3 objects under the given prefix.
func (s *S3Provider) ListObjects(ctx context.Context, prefix string) ([]data.S3ObjectInfo, error) {
	objectCh := s.client.ListObjects(ctx, s.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	objects := make([]data.S3ObjectInfo, 0)
	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("listing objects under prefix %q: %w", prefix, object.Err)
		}
		objects = append(objects, data.S3ObjectInfo{
			Key:          object.Key,
			LastModified: object.LastModified,
		})
	}
	return objects, nil
}

// DeleteObject removes the object with the given key from S3.
func (s *S3Provider) DeleteObject(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucketName, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("deleting object %q: %w", key, err)
	}
	return nil
}

// PutObject writes size bytes from r directly to key — server-side, no
// presigned POST, no content-length cap. Used for content only the server
// itself ever writes (raw parsed-document JSON, extracted images).
func (s *S3Provider) PutObject(
	ctx context.Context,
	key string,
	r io.Reader,
	size int64,
	contentType string,
) error {
	_, err := s.client.PutObject(ctx, s.bucketName, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("putting object %q: %w", key, err)
	}
	return nil
}

// GetObject returns a reader for the object at key — server-side, no
// presigned GET. Callers must close the returned reader.
func (s *S3Provider) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting object %q: %w", key, err)
	}
	return obj, nil
}
