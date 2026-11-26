package controller

import (
	"net/http"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type StorefrontOrderController struct {
	service               *service.OrderService
	storefrontAuthEnabled bool
}

type checkoutPayload struct {
	ContactName       string                   `json:"contact_name" binding:"required"`
	ContactPhone      string                   `json:"contact_phone" binding:"required"`
	Address           string                   `json:"address" binding:"required"`
	FulfillmentMethod models.FulfillmentMethod `json:"fulfillment_method"`
	Items             []checkoutItemPayload    `json:"items" binding:"required"`
}

type checkoutItemPayload struct {
	SKUID    uint `json:"sku_id" binding:"required"`
	Quantity int  `json:"quantity" binding:"required"`
}

type orderResponse struct {
	ID                uint                     `json:"id"`
	OrderNo           string                   `json:"order_no"`
	UserID            uint                     `json:"user_id"`
	CustomerID        uint                     `json:"customer_id"`
	TenantID          uint                     `json:"tenant_id"`
	TotalAmount       int64                    `json:"total_amount"`
	Status            models.OrderStatus       `json:"status"`
	FulfillmentMethod models.FulfillmentMethod `json:"fulfillment_method"`
	TrackingNumber    string                   `json:"tracking_number"`
	PaymentStatus     models.PaymentStatus     `json:"payment_status"`
	ContactName       string                   `json:"contact_name"`
	ContactPhone      string                   `json:"contact_phone"`
	Address           string                   `json:"address"`
	Items             []orderItemResponse      `json:"items"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type orderItemResponse struct {
	ID          uint             `json:"id"`
	OrderID     uint             `json:"order_id"`
	SKUID       uint             `json:"sku_id"`
	ProductName string           `json:"product_name"`
	Price       int64            `json:"price"`
	Quantity    int              `json:"quantity"`
	SKU         orderSKUResponse `json:"sku"`
	Product     orderSPUResponse `json:"product"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type orderSKUResponse struct {
	ID         uint           `json:"id"`
	Name       string         `json:"name"`
	ERPSKUCode string         `json:"erp_sku_code"`
	Specs      datatypes.JSON `json:"specs"`
}

type orderSPUResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	MainImage string `json:"main_image"`
}

func NewStorefrontOrderController(service *service.OrderService, storefrontAuthEnabled ...bool) *StorefrontOrderController {
	enabled := true
	if len(storefrontAuthEnabled) > 0 {
		enabled = storefrontAuthEnabled[0]
	}
	return &StorefrontOrderController{service: service, storefrontAuthEnabled: enabled}
}

func (ctl *StorefrontOrderController) RegisterRoutes(group *gin.RouterGroup) {
	orders := group.Group("/storefront/orders")
	orders.GET("", ctl.List)
	orders.POST("/checkout", ctl.Checkout)
}

func (ctl *StorefrontOrderController) RequireStorefrontAuthEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ctl.storefrontAuthEnabled {
			writeStorefrontUnavailable(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (ctl *StorefrontOrderController) List(c *gin.Context) {
	if !ctl.storefrontAuthEnabled {
		writeStorefrontUnavailable(c)
		return
	}

	customerID, tenantID, ok := storefrontOrderScope(c)
	if !ok {
		return
	}

	orders, err := ctl.service.ListStorefrontOrders(c.Request.Context(), tenantID, customerID)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]orderResponse, 0, len(orders))
	for i := range orders {
		items = append(items, toOrderResponse(&orders[i]))
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *StorefrontOrderController) Checkout(c *gin.Context) {
	if !ctl.storefrontAuthEnabled {
		writeStorefrontUnavailable(c)
		return
	}

	customerID, tenantID, ok := storefrontOrderScope(c)
	if !ok {
		return
	}

	var payload checkoutPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	items := make([]service.CheckoutItemInput, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, service.CheckoutItemInput{
			SKUID:    item.SKUID,
			Quantity: item.Quantity,
		})
	}

	order, err := ctl.service.Checkout(c.Request.Context(), service.CheckoutInput{
		CustomerID:        customerID,
		TenantID:          tenantID,
		ContactName:       payload.ContactName,
		ContactPhone:      payload.ContactPhone,
		Address:           payload.Address,
		FulfillmentMethod: payload.FulfillmentMethod,
		Items:             items,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: toOrderResponse(order)})
}

func storefrontOrderScope(c *gin.Context) (uint, uint, bool) {
	customerID, ok := middleware.CustomerIDFromContext(c)
	if !ok || customerID == 0 {
		writeError(c, service.ErrUnauthorized)
		return 0, 0, false
	}
	tenantID, ok := tenantIDFromRequest(c)
	if !ok {
		return 0, 0, false
	}
	if tenantID == nil {
		return customerID, 0, true
	}
	return customerID, *tenantID, true
}

func toOrderResponse(order *models.Order) orderResponse {
	if order == nil {
		return orderResponse{}
	}

	items := make([]orderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemResponse{
			ID:          item.ID,
			OrderID:     item.OrderID,
			SKUID:       item.SKUID,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
			SKU: orderSKUResponse{
				ID:         item.SKU.ID,
				Name:       item.SKU.Name,
				ERPSKUCode: item.SKU.ERPSKUCode,
				Specs:      item.SKU.Specs,
			},
			Product: orderSPUResponse{
				ID:        item.SKU.Product.ID,
				Name:      item.SKU.Product.Name,
				MainImage: item.SKU.Product.MainImage,
			},
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}

	return orderResponse{
		ID:                order.ID,
		OrderNo:           order.OrderNo,
		UserID:            orderUserID(order),
		CustomerID:        orderCustomerID(order),
		TenantID:          order.TenantID,
		TotalAmount:       order.TotalAmount,
		Status:            order.Status,
		FulfillmentMethod: order.FulfillmentMethod,
		TrackingNumber:    order.TrackingNumber,
		PaymentStatus:     order.PaymentStatus,
		ContactName:       order.ContactName,
		ContactPhone:      order.ContactPhone,
		Address:           order.Address,
		Items:             items,
		CreatedAt:         order.CreatedAt,
		UpdatedAt:         order.UpdatedAt,
	}
}

func orderUserID(order *models.Order) uint {
	if order == nil || order.UserID == nil {
		return 0
	}
	return *order.UserID
}

func orderCustomerID(order *models.Order) uint {
	if order == nil || order.CustomerID == nil {
		return 0
	}
	return *order.CustomerID
}
