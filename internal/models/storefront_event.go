package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StorefrontEventName string

const (
	StorefrontEventPageView          StorefrontEventName = "page_view"
	StorefrontEventProductImpression StorefrontEventName = "product_impression"
	StorefrontEventProductClick      StorefrontEventName = "product_click"
	StorefrontEventProductView       StorefrontEventName = "product_view"
	StorefrontEventContentImpression StorefrontEventName = "content_impression"
	StorefrontEventContentClick      StorefrontEventName = "content_click"
	StorefrontEventWhatsAppClick     StorefrontEventName = "whatsapp_click"
	StorefrontEventPhoneClick        StorefrontEventName = "phone_click"
)

var allowedStorefrontEventNames = map[StorefrontEventName]struct{}{
	StorefrontEventPageView:          {},
	StorefrontEventProductImpression: {},
	StorefrontEventProductClick:      {},
	StorefrontEventProductView:       {},
	StorefrontEventContentImpression: {},
	StorefrontEventContentClick:      {},
	StorefrontEventWhatsAppClick:     {},
	StorefrontEventPhoneClick:        {},
}

func (name StorefrontEventName) IsValid() bool {
	_, ok := allowedStorefrontEventNames[name]
	return ok
}

func (name StorefrontEventName) IsProductEvent() bool {
	switch name {
	case StorefrontEventProductImpression, StorefrontEventProductClick, StorefrontEventProductView:
		return true
	default:
		return false
	}
}

func (name StorefrontEventName) IsContentEvent() bool {
	return name == StorefrontEventContentImpression || name == StorefrontEventContentClick
}

type StorefrontEntityType string

const (
	StorefrontEntityProduct          StorefrontEntityType = "product"
	StorefrontEntityMarketingContent StorefrontEntityType = "marketing_content"
	StorefrontEntityShop             StorefrontEntityType = "shop"
	StorefrontEntityStorefront       StorefrontEntityType = "storefront"
)

func (entityType StorefrontEntityType) IsValid() bool {
	switch entityType {
	case "", StorefrontEntityProduct, StorefrontEntityMarketingContent, StorefrontEntityShop, StorefrontEntityStorefront:
		return true
	default:
		return false
	}
}

type StorefrontSourceType string

const (
	StorefrontSourceHomeProductFloor StorefrontSourceType = "home_product_floor"
	StorefrontSourceMall             StorefrontSourceType = "mall"
	StorefrontSourceCategory         StorefrontSourceType = "category"
	StorefrontSourceSearch           StorefrontSourceType = "search"
	StorefrontSourceShop             StorefrontSourceType = "shop"
	StorefrontSourceProductDetail    StorefrontSourceType = "product_detail"
	StorefrontSourceMarketingContent StorefrontSourceType = "marketing_content"
	StorefrontSourceDirect           StorefrontSourceType = "direct"
)

func (sourceType StorefrontSourceType) IsValid() bool {
	switch sourceType {
	case StorefrontSourceHomeProductFloor,
		StorefrontSourceMall,
		StorefrontSourceCategory,
		StorefrontSourceSearch,
		StorefrontSourceShop,
		StorefrontSourceProductDetail,
		StorefrontSourceMarketingContent,
		StorefrontSourceDirect:
		return true
	default:
		return false
	}
}

type StorefrontPageType string

const (
	StorefrontPageHome    StorefrontPageType = "home"
	StorefrontPageMall    StorefrontPageType = "mall"
	StorefrontPageProduct StorefrontPageType = "product"
	StorefrontPageShop    StorefrontPageType = "shop"
	StorefrontPageSearch  StorefrontPageType = "search"
	StorefrontPageOther   StorefrontPageType = "other"
)

func (pageType StorefrontPageType) IsValid() bool {
	switch pageType {
	case StorefrontPageHome, StorefrontPageMall, StorefrontPageProduct, StorefrontPageShop, StorefrontPageSearch, StorefrontPageOther:
		return true
	default:
		return false
	}
}

type StorefrontDeviceType string

const (
	StorefrontDeviceDesktop StorefrontDeviceType = "desktop"
	StorefrontDeviceMobile  StorefrontDeviceType = "mobile"
	StorefrontDeviceTablet  StorefrontDeviceType = "tablet"
	StorefrontDeviceUnknown StorefrontDeviceType = "unknown"
)

func (deviceType StorefrontDeviceType) IsValid() bool {
	switch deviceType {
	case StorefrontDeviceDesktop, StorefrontDeviceMobile, StorefrontDeviceTablet, StorefrontDeviceUnknown:
		return true
	default:
		return false
	}
}

type StorefrontTrafficType string

const (
	StorefrontTrafficHuman StorefrontTrafficType = "human"
	StorefrontTrafficBot   StorefrontTrafficType = "bot"
)

func (trafficType StorefrontTrafficType) IsValid() bool {
	return trafficType == StorefrontTrafficHuman || trafficType == StorefrontTrafficBot
}

// StorefrontEvent intentionally stores only normalized analytics context. Raw IP,
// User-Agent, authorization credentials, JWTs, and request bodies are never kept.
// A nil TenantID means an event is in platform scope because no platform mall
// tenant was configured or resolvable; clients can never choose this value.
type StorefrontEvent struct {
	ID            uint                  `gorm:"primaryKey" json:"id"`
	EventID       string                `gorm:"size:36;not null;uniqueIndex:idx_storefront_events_event_id_unique" json:"event_id"`
	EventName     StorefrontEventName   `gorm:"size:40;not null;check:chk_storefront_events_event_name,event_name IN ('page_view','product_impression','product_click','product_view','content_impression','content_click','whatsapp_click','phone_click');index:idx_storefront_events_event_occurred,priority:1" json:"event_name"`
	VisitorID     string                `gorm:"size:64;not null;index:idx_storefront_events_visitor_occurred,priority:1" json:"visitor_id"`
	SessionID     string                `gorm:"size:64;not null" json:"session_id"`
	TenantID      *uint                 `gorm:"index:idx_storefront_events_tenant_occurred,priority:1" json:"tenant_id,omitempty"`
	EntityType    StorefrontEntityType  `gorm:"size:40;not null;default:'';index:idx_storefront_events_entity_occurred,priority:1" json:"entity_type"`
	EntityID      *string               `gorm:"size:128;index:idx_storefront_events_entity_occurred,priority:2" json:"entity_id,omitempty"`
	ProductID     *uint                 `gorm:"index:idx_storefront_events_product_occurred,priority:1" json:"product_id,omitempty"`
	SourceType    StorefrontSourceType  `gorm:"size:40;not null" json:"source_type"`
	SourceID      *string               `gorm:"size:128" json:"source_id,omitempty"`
	PageType      StorefrontPageType    `gorm:"size:24;not null" json:"page_type"`
	PagePath      string                `gorm:"size:512;not null" json:"page_path"`
	Locale        string                `gorm:"size:8;not null" json:"locale"`
	Referrer      string                `gorm:"size:1024;not null;default:''" json:"referrer"`
	DeviceType    StorefrontDeviceType  `gorm:"size:16;not null" json:"device_type"`
	TrafficType   StorefrontTrafficType `gorm:"size:16;not null;default:human;check:chk_storefront_events_traffic_type,traffic_type IN ('human','bot')" json:"traffic_type"`
	OccurredAt    time.Time             `gorm:"not null;index:idx_storefront_events_occurred_at;index:idx_storefront_events_event_occurred,priority:2;index:idx_storefront_events_product_occurred,priority:2;index:idx_storefront_events_entity_occurred,priority:3;index:idx_storefront_events_tenant_occurred,priority:2;index:idx_storefront_events_visitor_occurred,priority:2" json:"occurred_at"`
	CreatedAt     time.Time             `gorm:"not null" json:"created_at"`
	ThrottleKey   string                `gorm:"-" json:"-"`
	ThrottleUntil time.Time             `gorm:"-" json:"-"`
}

// StorefrontEventThrottle stores only a server-generated hash. It provides
// database-backed, cross-process throttling without retaining a raw visitor ID.
type StorefrontEventThrottle struct {
	ThrottleKey string    `gorm:"primaryKey;size:64" json:"-"`
	ExpiresAt   time.Time `gorm:"not null;index" json:"-"`
	CreatedAt   time.Time `gorm:"not null" json:"-"`
	UpdatedAt   time.Time `gorm:"not null" json:"-"`
}

// BeforeCreate keeps internal fixtures and migration-safe code paths valid. The
// public tracking service still requires clients to provide EventID, so retries
// retain their original idempotency key instead of receiving a new one here.
func (event *StorefrontEvent) BeforeCreate(_ *gorm.DB) error {
	event.EventID = strings.TrimSpace(event.EventID)
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.TrafficType == "" {
		event.TrafficType = StorefrontTrafficHuman
	}
	return nil
}
