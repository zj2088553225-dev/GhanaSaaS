package models

import (
	"time"

	"gorm.io/datatypes"
)

type SitePageStatus string

const (
	SitePageStatusDraft     SitePageStatus = "draft"
	SitePageStatusPublished SitePageStatus = "published"
)

type SitePage struct {
	BaseModel
	PageKey          string         `gorm:"type:varchar(64);not null;uniqueIndex" json:"page_key"`
	Title            string         `gorm:"type:varchar(200);not null;default:''" json:"title"`
	DraftContent     datatypes.JSON `gorm:"type:jsonb;not null" json:"draft_content"`
	PublishedContent datatypes.JSON `gorm:"type:jsonb" json:"published_content"`
	Status           SitePageStatus `gorm:"type:varchar(32);not null;default:'draft';index" json:"status"`
	CreatedBy        *uint          `gorm:"index" json:"created_by"`
	UpdatedBy        *uint          `gorm:"index" json:"updated_by"`
	PublishedAt      *time.Time     `json:"published_at"`
}
