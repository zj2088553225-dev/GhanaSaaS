package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type CustomerStatus string

const (
	CustomerStatusActive   CustomerStatus = "active"
	CustomerStatusDisabled CustomerStatus = "disabled"
)

type Customer struct {
	BaseModel
	Email         string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"email"`
	PasswordHash  string         `gorm:"type:varchar(255);not null" json:"-"`
	Phone         *string        `gorm:"type:varchar(32);uniqueIndex" json:"phone,omitempty"`
	Nickname      string         `gorm:"type:varchar(120)" json:"nickname"`
	AvatarURL     string         `gorm:"type:varchar(500)" json:"avatar_url"`
	Status        CustomerStatus `gorm:"type:varchar(32);not null;index" json:"status"`
	EmailVerified bool           `gorm:"not null;default:false" json:"email_verified"`
	LastLoginAt   *time.Time     `json:"last_login_at,omitempty"`
}

func (customer *Customer) BeforeSave(_ *gorm.DB) error {
	customer.Email = strings.ToLower(strings.TrimSpace(customer.Email))
	return nil
}

type PasswordResetToken struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	User      Customer   `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	TokenHash string     `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt    *time.Time `gorm:"index" json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
