package models

type StorefrontTrackingSetting struct {
	BaseModel
	MetaPixelEnabled  bool   `gorm:"not null;default:false" json:"meta_pixel_enabled"`
	MetaPixelID       string `gorm:"size:64;not null;default:''" json:"meta_pixel_id"`
	CustomHeadEnabled bool   `gorm:"not null;default:false" json:"custom_head_enabled"`
	CustomHeadHTML    string `gorm:"type:text;not null;default:''" json:"custom_head_html"`
}
