package models

type MerchantLeadStatus string

const (
	MerchantLeadStatusNew       MerchantLeadStatus = "new"
	MerchantLeadStatusContacted MerchantLeadStatus = "contacted"
	MerchantLeadStatusApproved  MerchantLeadStatus = "approved"
	MerchantLeadStatusRejected  MerchantLeadStatus = "rejected"
)

type MerchantLead struct {
	BaseModel
	ContactName    string             `gorm:"type:varchar(120);not null" json:"contact_name"`
	ShopName       string             `gorm:"type:varchar(160);not null" json:"shop_name"`
	Whatsapp       string             `gorm:"type:varchar(64);not null" json:"whatsapp"`
	Phone          string             `gorm:"type:varchar(64)" json:"phone"`
	Category       string             `gorm:"type:varchar(120);not null" json:"category"`
	HasOfflineShop bool               `gorm:"not null;default:false" json:"has_offline_shop"`
	City           string             `gorm:"type:varchar(120)" json:"city"`
	ProductCount   string             `gorm:"type:varchar(64)" json:"product_count"`
	Message        string             `gorm:"type:varchar(1000)" json:"message"`
	Status         MerchantLeadStatus `gorm:"type:varchar(32);not null;default:new;index" json:"status"`
}
