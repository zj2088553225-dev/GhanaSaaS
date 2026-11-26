package models

import "time"

type OpenAPIRequestNonce struct {
	ID        uint64    `gorm:"primaryKey" json:"-"`
	AppKey    string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_openapi_request_nonces_app_key_nonce" json:"-"`
	Nonce     string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_openapi_request_nonces_app_key_nonce" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"-"`
}
