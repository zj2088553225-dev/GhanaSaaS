package models

type Category struct {
	BaseModel
	TenantID uint      `gorm:"index;uniqueIndex:idx_categories_tenant_name" json:"tenant_id"`
	Tenant   Tenant    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"tenant,omitempty"`
	ParentID *uint     `gorm:"index" json:"parent_id"`
	Parent   *Category `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Name     string    `gorm:"type:varchar(120);not null;index;uniqueIndex:idx_categories_tenant_name" json:"name"`
	Sort     int       `gorm:"not null;default:0;index" json:"sort"`
	Icon     string    `gorm:"type:varchar(255)" json:"icon"`
	IsActive bool      `gorm:"not null;default:true;index" json:"is_active"`
}
