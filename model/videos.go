package model

import (
	"time"

	"github.com/google/uuid"
)

// Video represents the schema for the videos table in the database.
type Video struct {
	// ID is the unique identifier for the video record.
	ID uuid.UUID `json:"id" validate:"required" format:"uuid"`

	// VideoURL is the link to the video (e.g., TikTok or Instagram reels).
	VideoURL string `db:"video_url" json:"video_url"`

	// AuthorUsername is the username of the content creator.
	AuthorUsername string `db:"author_username" json:"author_username"`

	// AuthorProfileURL is the link to the content creator's profile.
	AuthorProfileURL string `db:"author_profile_url" json:"author_profile_url"`

	// Caption is the description of the video content.
	Caption string `db:"caption" json:"caption"`

	// PlayCount is the number of the video plays.
	PlayCount int64 `db:"play_count" json:"play_count"`

	// LikeCount is the number of likes the video has received.
	LikeCount int64 `db:"like_count" json:"like_count"`

	// ThumbnailURL is the link to the video's thumbnail image.
	ThumbnailURL string `db:"thumbnail_url" json:"thumbnail_url"`

	// LocationCreated is the location where the video was created.
	LocationCreated string `db:"location_created" json:"location_created"`

	// VideoCreatedAt is the timestamp when the video was created.
	VideoCreatedAt time.Time `db:"video_created_at" json:"video_created_at"`

	// TranscriptionText is the text extracted from the video.
	TranscriptionText *string `db:"transcription_text" json:"transcription_text"`

	// VideoType indicates the type of video (e.g., TikTok, Instagram).
	VideoType string `db:"video_type" json:"video_type"`

	// Hashtags contains the list of hashtags extracted from the video.
	Hashtags []string `json:"hashtags"`

	// CreatedAt is the timestamp when the record was created in the database.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is the timestamp when the record was last updated in the database.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
