package models

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusCompleted  OrderStatus = "completed"
)

type FulfillmentMethod string

const (
	FulfillmentMethodPickup   FulfillmentMethod = "pickup"
	FulfillmentMethodDelivery FulfillmentMethod = "delivery"
)

type PaymentStatus string

const (
	PaymentStatusUnpaid   PaymentStatus = "unpaid"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

type Order struct {
	BaseModel
	OrderNo           string            `gorm:"type:varchar(64);not null;uniqueIndex" json:"order_no"`
	UserID            *uint             `gorm:"index" json:"user_id,omitempty"`
	User              User              `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"user,omitempty"`
	CustomerID        *uint             `gorm:"index" json:"customer_id,omitempty"`
	Customer          Customer          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"customer,omitempty"`
	TenantID          uint              `gorm:"not null;index" json:"tenant_id"`
	Tenant            Tenant            `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"tenant,omitempty"`
	TotalAmount       int64             `gorm:"not null" json:"total_amount"`
	Status            OrderStatus       `gorm:"type:varchar(32);not null;index" json:"status"`
	FulfillmentMethod FulfillmentMethod `gorm:"type:varchar(32);not null;default:pickup;index" json:"fulfillment_method"`
	TrackingNumber    string            `gorm:"type:varchar(128)" json:"tracking_number"`
	PaymentStatus     PaymentStatus     `gorm:"type:varchar(32);not null;default:unpaid;index" json:"payment_status"`
	ContactName       string            `gorm:"type:varchar(120);not null" json:"contact_name"`
	ContactPhone      string            `gorm:"type:varchar(32);not null" json:"contact_phone"`
	Address           string            `gorm:"type:text;not null" json:"address"`
	Items             []OrderItem       `gorm:"foreignKey:OrderID" json:"items,omitempty"`
}

type OrderItem struct {
	BaseModel
	OrderID     uint   `gorm:"not null;index" json:"order_id"`
	Order       Order  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"order,omitempty"`
	SKUID       uint   `gorm:"not null;index" json:"sku_id"`
	SKU         SKU    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"sku,omitempty"`
	ProductName string `gorm:"type:varchar(200);not null" json:"product_name"`
	Price       int64  `gorm:"not null" json:"price"`
	Quantity    int    `gorm:"not null" json:"quantity"`
}
