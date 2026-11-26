package models

type UserRole string

const (
	UserRoleSuperAdmin  UserRole = "super_admin"
	UserRoleTenantAdmin UserRole = "tenant_admin"
	UserRoleFinance     UserRole = "finance"
	UserRolePicker      UserRole = "picker"
	UserRoleCustomer    UserRole = "customer"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

type User struct {
	BaseModel
	TenantID     *uint      `gorm:"index" json:"tenant_id"`
	Tenant       *Tenant    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"tenant,omitempty"`
	Role         UserRole   `gorm:"type:varchar(32);not null;index" json:"role"`
	Phone        string     `gorm:"type:varchar(32);not null;index" json:"phone"`
	PasswordHash string     `gorm:"type:varchar(255);not null" json:"-"`
	Status       UserStatus `gorm:"type:varchar(32);not null;index" json:"status"`
}
