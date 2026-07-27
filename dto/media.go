package dto

// GetPresignedURLDTO is used for requesting a presigned URL for media upload.
type GetPresignedURLDTO struct {
	// Intent specifies the purpose of the media upload, e.g., "document".
	Intent string `query:"intent" validate:"required"`

	// Filename is the name of the file to be uploaded.
	Filename string `query:"filename" validate:"required,min=1,max=255"`

	// ContentType is the MIME type of the file to be uploaded.
	ContentType string `query:"content_type" validate:"required,mimetype"`
}

// GetPresignedGETURLDTO is used for requesting a presigned GET URL for accessing uploaded media.
type GetPresignedGETURLDTO struct {
	// S3Key is the full S3 object key (e.g., "documents/abc123.pdf")
	S3Key string `query:"s3_key" validate:"required,s3key"`

	// TTL is the time-to-live for the presigned URL in seconds (default: 900 = 15 minutes)
	TTL int64 `query:"ttl" validate:"omitempty,min=60,max=86400"`
}

// PresignedURL represents the response containing a presigned URL for accessing media.
type PresignedURL struct {
	PresignedURL string `json:"presigned_url"`
	TTL          int64  `json:"ttl"`
}
