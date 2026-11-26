package models

type PlatformCategorySearchIndex struct {
	BaseModel
	CategoryID uint                   `gorm:"not null;uniqueIndex:idx_platform_category_search_indexes_category_locale,priority:1;index" json:"category_id"`
	Category   PlatformCategory       `gorm:"foreignKey:CategoryID" json:"-"`
	Locale     string                 `gorm:"type:varchar(8);not null;uniqueIndex:idx_platform_category_search_indexes_category_locale,priority:2;index" json:"locale"`
	Name       string                 `gorm:"type:varchar(120);not null" json:"name"`
	Path       string                 `gorm:"type:varchar(500);not null" json:"path"`
	SearchText string                 `gorm:"type:text;not null" json:"search_text"`
	ParentID   *uint                  `gorm:"index" json:"parent_id"`
	Level      int                    `gorm:"not null;default:1;index" json:"level"`
	Status     PlatformCategoryStatus `gorm:"type:varchar(32);not null;default:active;index" json:"status"`
	SortOrder  int                    `gorm:"not null;default:0;index" json:"sort_order"`
}

func (PlatformCategorySearchIndex) TableName() string {
	return "platform_category_search_indexes"
}
