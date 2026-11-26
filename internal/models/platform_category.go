package models

type PlatformCategoryStatus string

const (
	PlatformCategoryStatusActive   PlatformCategoryStatus = "active"
	PlatformCategoryStatusDisabled PlatformCategoryStatus = "disabled"
)

type PlatformCategory struct {
	BaseModel
	ParentID    *uint                  `gorm:"index;uniqueIndex:idx_platform_categories_parent_name" json:"parent_id"`
	Parent      *PlatformCategory      `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Name        string                 `gorm:"type:varchar(120);not null;index;uniqueIndex:idx_platform_categories_parent_name" json:"name"`
	Path        string                 `gorm:"type:varchar(500);not null;index" json:"path"`
	Level       int                    `gorm:"not null;default:1;index" json:"level"`
	SortOrder   int                    `gorm:"not null;default:0;index" json:"sort_order"`
	Status      PlatformCategoryStatus `gorm:"type:varchar(32);not null;default:active;index" json:"status"`
	IsLeaf      bool                   `gorm:"not null;default:true;index" json:"is_leaf"`
	HasChildren *bool                  `gorm:"-" json:"-"`
	ZhName      string                 `gorm:"-" json:"zh_name,omitempty"`
	ZhPath      string                 `gorm:"-" json:"zh_path,omitempty"`
}
