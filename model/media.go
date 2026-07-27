package model

// S3Key represents a key (path) to an object stored in Amazon S3.
// For S3 backend, store only the key/path, not the full URL.
type S3Key string

// POSTUploadPolicy represents the presigned POST URL and form fields for uploading media.
type POSTUploadPolicy struct {
	URL        string            `json:"url"        example:"https://s3.planetleague.co.uk/bucket/key" validate:"required,url"`
	Prefix     string            `json:"prefix"     example:"avatars/"                                 validate:"required"`
	FormFields map[string]string `json:"formFields"                                                    validate:"required,dive,keys,required,endkeys,required"`
}
