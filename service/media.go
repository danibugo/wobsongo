package service

import (
	"context"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// MediaService provides media-related services.
type MediaService struct {
	// provider is the media upload provider.
	provider data.MediaUploadProvider
}

// NewMediaService creates a new instance of MediaService.
func NewMediaService(uploader data.MediaUploadProvider) *MediaService {
	return &MediaService{provider: uploader}
}

// GetPresignedPOSTURL generates a presigned POST URL for uploading media.
func (s *MediaService) GetPresignedPOSTURL(
	ctx context.Context,
	intent data.MediaUploadIntent,
	filename, contentType string,
) (*model.POSTUploadPolicy, error) {
	presignedURL, form, err := s.provider.GetPresignedPOSTURL(
		ctx,
		intent,
		filename,
		contentType,
	)
	if err != nil {
		return nil, err
	}
	prefix := data.ObjectPrefixForIntent(intent)
	return &model.POSTUploadPolicy{
		URL:        presignedURL.String(),
		Prefix:     prefix,
		FormFields: form,
	}, nil
}

// GetPresignedGETURL generates a presigned GET URL for accessing uploaded media.
func (s *MediaService) GetPresignedGETURL(
	ctx context.Context,
	s3Key string,
	ttl int64,
) (string, error) {
	// Default TTL: 15 minutes
	if ttl == 0 {
		ttl = 900
	}

	presignedURL, err := s.provider.GetPresignedGETURL(ctx, s3Key, ttl)
	if err != nil {
		return "", err
	}

	return presignedURL, nil
}
