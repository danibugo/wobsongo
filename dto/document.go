package dto

// CreateDocumentDTO represents the payload required to ingest a new document.
type CreateDocumentDTO struct {
	// SHA256 is the SHA256 hash of the document content.
	SHA256 string `json:"sha256" validate:"required"`

	// FileKey is the object storage key (e.g., S3/MinIO) where the document file was uploaded.
	FileKey string `json:"file_key" validate:"required,s3key"`

	// Title is the title of the document.
	Title string `json:"title" validate:"required"`

	// Filename is the name of the document file.
	Filename string `json:"filename" validate:"required"`

	// Filetype is the mime type of the document file (e.g., "application/pdf", "text/plain").
	Filetype string `json:"filetype" validate:"required"`

	// Filesize is the size of the document file in bytes.
	Filesize int64 `json:"filesize" validate:"required,gt=0"`

	// PageCount is the number of pages in the document.
	PageCount int `json:"page_count" validate:"required,gt=0"`

	// PublisherName is the name of the publisher of the document.
	PublisherName string `json:"publisher_name"`

	// PublicationYear is the year the document was published.
	PublicationYear int `json:"publication_year"`

	// Language is the document's language ("en" or "fr") — required, not
	// auto-detected, since French must be first-class for this deployment
	// and silently defaulting risks mis-tagging a document.
	Language string `json:"language" validate:"required,oneof=en fr"`
}

// UpdateDocumentDTO represents the payload for updating a document's descriptive
// metadata. The underlying file, hash, and other physical file attributes are
// treated as immutable once a document has been ingested.
type UpdateDocumentDTO struct {
	// Title is the title of the document.
	Title string `json:"title" validate:"required"`

	// PublisherName is the name of the publisher of the document.
	PublisherName string `json:"publisher_name"`

	// PublicationYear is the year the document was published.
	PublicationYear int `json:"publication_year"`
}
