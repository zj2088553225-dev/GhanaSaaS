package models

type ShopProfile struct {
	BaseModel
	TenantID      uint   `gorm:"not null;uniqueIndex" json:"tenant_id"`
	Tenant        Tenant `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"tenant,omitempty"`
	Slug          string `gorm:"type:varchar(80);not null;uniqueIndex" json:"slug"`
	ShopName      string `gorm:"type:varchar(120);not null" json:"shop_name"`
	LogoURL       string `gorm:"type:varchar(255)" json:"logo_url"`
	BannerURL     string `gorm:"type:varchar(255)" json:"banner_url"`
	Description   string `gorm:"type:text" json:"description"`
	Phone         string `gorm:"type:varchar(64)" json:"phone"`
	Whatsapp      string `gorm:"type:varchar(64)" json:"whatsapp"`
	Address       string `gorm:"type:varchar(255)" json:"address"`
	BusinessHours string `gorm:"type:varchar(255)" json:"business_hours"`
	IsPublic      bool   `gorm:"not null;default:false;index" json:"is_public"`
}
