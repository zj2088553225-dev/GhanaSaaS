package models

import (
	"errors"

	"saas/internal/pkg/locale"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ContentTranslationStatus string

const (
	ContentTranslationStatusDraft       ContentTranslationStatus = "draft"
	ContentTranslationStatusPublished   ContentTranslationStatus = "published"
	ContentTranslationStatusRejected    ContentTranslationStatus = "rejected"
	ContentTranslationStatusUnpublished ContentTranslationStatus = "unpublished"
)

type ContentTranslation struct {
	BaseModel
	EntityType string `gorm:"type:varchar(64);not null;uniqueIndex:idx_content_translations_unique,priority:1;index:idx_content_translations_entity_locale,priority:1;index:idx_content_translations_field_locale,priority:1" json:"entity_type"`
	EntityID   uint   `gorm:"not null;uniqueIndex:idx_content_translations_unique,priority:2;index:idx_content_translations_entity_locale,priority:2" json:"entity_id"`
	FieldKey   string `gorm:"type:varchar(128);not null;uniqueIndex:idx_content_translations_unique,priority:3;index:idx_content_translations_field_locale,priority:2" json:"field_key"`
	Locale     string `gorm:"type:varchar(8);not null;uniqueIndex:idx_content_translations_unique,priority:4;index:idx_content_translations_entity_locale,priority:3;index:idx_content_translations_field_locale,priority:3" json:"locale"`

	ValueText string         `gorm:"type:text" json:"value_text"`
	ValueHTML string         `gorm:"type:text" json:"value_html"`
	ValueJSON datatypes.JSON `gorm:"type:jsonb" json:"value_json"`

	Status ContentTranslationStatus `gorm:"type:varchar(32);not null;default:'published';index" json:"status"`
}

func (t *ContentTranslation) BeforeSave(*gorm.DB) error {
	t.Locale = locale.NormalizeLocale(t.Locale)
	switch t.Status {
	case "":
		t.Status = ContentTranslationStatusPublished
	case ContentTranslationStatusDraft,
		ContentTranslationStatusPublished,
		ContentTranslationStatusRejected,
		ContentTranslationStatusUnpublished:
	default:
		return errors.New("invalid content translation status")
	}
	return nil
}
