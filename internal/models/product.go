package models

import "gorm.io/datatypes"

type SPUStatus string

const (
	SPUStatusOnShelf  SPUStatus = "on_shelf"
	SPUStatusOffShelf SPUStatus = "off_shelf"
)

type SPU struct {
	BaseModel
	TenantID           uint             `gorm:"not null;index" json:"tenant_id"`
	Tenant             Tenant           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"tenant,omitempty"`
	PlatformCategoryID uint             `gorm:"not null;index" json:"platform_category_id"`
	PlatformCategory   PlatformCategory `gorm:"foreignKey:PlatformCategoryID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
	CategoryID         *uint            `gorm:"index" json:"category_id"`
	Category           *Category        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"category,omitempty"`
	Name               string           `gorm:"type:varchar(200);not null;index" json:"name"`
	Description        string           `gorm:"type:text" json:"description"`
	DetailHTML         string           `gorm:"column:detail_html;type:text" json:"detail_html"`
	MainImage          string           `gorm:"type:varchar(255)" json:"main_image"`
	VideoURL           string           `gorm:"column:video_url;size:512" json:"video_url"`
	Material           string           `gorm:"column:material;size:255" json:"material"`
	Dimensions         string           `gorm:"column:dimensions;size:255" json:"dimensions"`
	Weight             string           `gorm:"column:weight;size:255" json:"weight"`
	SliderImages       datatypes.JSON   `gorm:"type:jsonb" json:"slider_images"`
	SpecTemplate       datatypes.JSON   `gorm:"type:jsonb" json:"spec_template"`
	Status             SPUStatus        `gorm:"type:varchar(32);not null;index" json:"status"`
	SKUs               []SKU            `gorm:"foreignKey:ProductID" json:"skus,omitempty"`
}

type SKU struct {
	BaseModel
	ProductID     uint           `gorm:"column:product_id;not null;index" json:"product_id"`
	Product       SPU            `gorm:"foreignKey:ProductID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"product,omitempty"`
	Name          string         `gorm:"type:varchar(160);not null" json:"name"`
	ERPSKUCode    string         `gorm:"column:erp_sku_code;type:varchar(120);not null;default:'';index" json:"erp_sku_code"`
	ImageURL      string         `gorm:"column:image_url;size:512" json:"image_url"`
	Specs         datatypes.JSON `gorm:"type:jsonb" json:"specs"`
	RetailPrice   int64          `gorm:"not null" json:"retail_price"`
	OriginalPrice int64          `gorm:"not null;default:0" json:"original_price"`
	SellableStock int64          `gorm:"not null;default:0" json:"sellable_stock"`
	IsActive      bool           `gorm:"not null;default:true;index" json:"is_active"`
}
