package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type AdminOrderController struct {
	service *service.OrderService
}

type adminOrderStatusPayload struct {
	Status models.OrderStatus `json:"status" binding:"required"`
}

type adminOrderShipPayload struct {
	TrackingNumber string `json:"tracking_number"`
}

type adminOrderUserResponse struct {
	ID    uint   `json:"id"`
	Phone string `json:"phone"`
}

type adminOrderCustomerResponse struct {
	ID       uint   `json:"id"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
	Nickname string `json:"nickname"`
}

type adminOrderSKUResponse struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	ERPSKUCode string `json:"erp_sku_code"`
	Barcode    string `json:"barcode"`
	Location   string `json:"location"`
}

type adminOrderItemResponse struct {
	ID          uint                  `json:"id"`
	OrderID     uint                  `json:"order_id"`
	SKUID       uint                  `json:"sku_id"`
	ProductName string                `json:"product_name"`
	Price       int64                 `json:"price"`
	Quantity    int                   `json:"quantity"`
	SKU         adminOrderSKUResponse `json:"sku"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

type adminOrderResponse struct {
	ID                uint                       `json:"id"`
	OrderNo           string                     `json:"order_no"`
	UserID            uint                       `json:"user_id"`
	User              adminOrderUserResponse     `json:"user"`
	CustomerID        uint                       `json:"customer_id"`
	Customer          adminOrderCustomerResponse `json:"customer"`
	TenantID          uint                       `json:"tenant_id"`
	TotalAmount       int64                      `json:"total_amount"`
	Status            models.OrderStatus         `json:"status"`
	FulfillmentMethod models.FulfillmentMethod   `json:"fulfillment_method"`
	TrackingNumber    string                     `json:"tracking_number"`
	PaymentStatus     models.PaymentStatus       `json:"payment_status"`
	ContactName       string                     `json:"contact_name"`
	ContactPhone      string                     `json:"contact_phone"`
	Address           string                     `json:"address"`
	Items             []adminOrderItemResponse   `json:"items"`
	CreatedAt         time.Time                  `json:"created_at"`
	UpdatedAt         time.Time                  `json:"updated_at"`
}

type adminOrderPageResponse struct {
	Items    []adminOrderResponse `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

func NewAdminOrderController(service *service.OrderService) *AdminOrderController {
	return &AdminOrderController{service: service}
}

func (ctl *AdminOrderController) RegisterRoutes(group *gin.RouterGroup) {
	orders := group.Group("/admin/orders")
	orders.GET("", ctl.List)
	orders.PATCH("/:id/status", ctl.UpdateStatus)
	orders.POST("/:id/ship", ctl.Ship)
	orders.POST("/:id/pay", ctl.Pay)
}

func (ctl *AdminOrderController) List(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	query, ok := parseAdminOrderQuery(c)
	if !ok {
		return
	}

	page, err := ctl.service.ListAdminOrders(c.Request.Context(), tenantID, query)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]adminOrderResponse, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, toAdminOrderResponse(&page.Items[i]))
	}

	c.JSON(http.StatusOK, response{Data: adminOrderPageResponse{
		Items:    items,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}})
}

func (ctl *AdminOrderController) UpdateStatus(c *gin.Context) {
	if !requireAdminOrderRole(c, models.UserRoleTenantAdmin, models.UserRolePicker) {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload adminOrderStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	order, err := ctl.service.UpdateAdminOrderStatus(c.Request.Context(), tenantID, id, payload.Status)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: toAdminOrderResponse(order)})
}

func (ctl *AdminOrderController) Ship(c *gin.Context) {
	if !requireAdminOrderRole(c, models.UserRoleTenantAdmin, models.UserRolePicker) {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload adminOrderShipPayload
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	order, err := ctl.service.ShipAdminOrder(c.Request.Context(), tenantID, id, payload.TrackingNumber)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: toAdminOrderResponse(order)})
}

func (ctl *AdminOrderController) Pay(c *gin.Context) {
	if !requireAdminOrderRole(c, models.UserRoleTenantAdmin, models.UserRoleFinance) {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	order, err := ctl.service.MarkAdminOrderPaid(c.Request.Context(), tenantID, id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: toAdminOrderResponse(order)})
}

func requireAdminOrderRole(c *gin.Context, roles ...models.UserRole) bool {
	role, ok := middleware.RoleFromContext(c)
	if !ok {
		writeError(c, service.ErrUnauthorized)
		return false
	}
	for _, allowed := range roles {
		if role == allowed {
			return true
		}
	}
	writeError(c, service.ErrForbidden)
	return false
}

func parseAdminOrderQuery(c *gin.Context) (service.AdminOrderQuery, bool) {
	query := service.AdminOrderQuery{
		OrderNo:  c.Query("order_no"),
		Phone:    c.Query("phone"),
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: parsePositiveInt(c.Query("page_size"), 20),
		NoPage:   isTruthyQuery(c.Query("export")) || isTruthyQuery(c.Query("no_page")),
	}

	if statusValue := c.Query("status"); statusValue != "" {
		status := models.OrderStatus(statusValue)
		query.Status = &status
	}

	if startValue := c.Query("start_date"); startValue != "" {
		start, err := parseAdminOrderDate(startValue, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, response{Error: "invalid start_date"})
			return query, false
		}
		query.StartDate = &start
	}
	if endValue := c.Query("end_date"); endValue != "" {
		end, err := parseAdminOrderDate(endValue, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, response{Error: "invalid end_date"})
			return query, false
		}
		query.EndDate = &end
	}

	return query, true
}

func parseAdminOrderDate(value string, endOfDay bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return parsed.AddDate(0, 0, 1), nil
	}
	return parsed, nil
}

func isTruthyQuery(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func toAdminOrderResponse(order *models.Order) adminOrderResponse {
	if order == nil {
		return adminOrderResponse{}
	}

	items := make([]adminOrderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, adminOrderItemResponse{
			ID:          item.ID,
			OrderID:     item.OrderID,
			SKUID:       item.SKUID,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
			SKU: adminOrderSKUResponse{
				ID:         item.SKU.ID,
				Name:       item.SKU.Name,
				ERPSKUCode: item.SKU.ERPSKUCode,
				Barcode:    item.SKU.ERPSKUCode,
				Location:   "",
			},
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}

	return adminOrderResponse{
		ID:                order.ID,
		OrderNo:           order.OrderNo,
		UserID:            orderUserID(order),
		User:              adminOrderUserResponse{ID: order.User.ID, Phone: order.User.Phone},
		CustomerID:        orderCustomerID(order),
		Customer:          toAdminOrderCustomerResponse(order),
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

func toAdminOrderCustomerResponse(order *models.Order) adminOrderCustomerResponse {
	if order == nil || order.CustomerID == nil {
		return adminOrderCustomerResponse{}
	}

	response := adminOrderCustomerResponse{
		ID:       order.Customer.ID,
		Nickname: order.Customer.Nickname,
	}
	if order.Customer.Phone != nil {
		response.Phone = *order.Customer.Phone
	}
	response.Email = order.Customer.Email
	return response
}
