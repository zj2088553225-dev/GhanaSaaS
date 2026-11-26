package tenant

import "gorm.io/gorm"

func WithTenant(db *gorm.DB, tenantID uint) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

func Scope(tenantID uint) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return WithTenant(db, tenantID)
	}
}
