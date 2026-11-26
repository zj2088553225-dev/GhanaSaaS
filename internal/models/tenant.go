package models

import "strings"

const PlatformTenantID uint = 1
const PlatformMallTenantName = "Platform Self Operated"

type TenantType string

const (
	TenantTypeSelfOperated TenantType = "self_operated"
	TenantTypeThirdParty   TenantType = "3rd_party"
)

type TenantStatus string

const (
	TenantStatusActive   TenantStatus = "active"
	TenantStatusDisabled TenantStatus = "disabled"
)

type Tenant struct {
	BaseModel
	Name      string       `gorm:"type:varchar(120);not null" json:"name"`
	Type      TenantType   `gorm:"type:varchar(32);not null;index" json:"type"`
	Status    TenantStatus `gorm:"type:varchar(32);not null;index" json:"status"`
	AppKey    string       `gorm:"type:varchar(32);uniqueIndex" json:"app_key"`
	AppSecret string       `gorm:"type:varchar(64)" json:"app_secret"`
}

func IsPlatformMallTenant(tenant Tenant) bool {
	return tenant.Type == TenantTypeSelfOperated &&
		strings.EqualFold(strings.TrimSpace(tenant.Name), PlatformMallTenantName)
}

func IsPlatformMallTenantWithID(tenant Tenant, platformMallTenantID uint) bool {
	if platformMallTenantID > 0 {
		return tenant.ID == platformMallTenantID
	}
	return IsPlatformMallTenant(tenant)
}
