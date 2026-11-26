package controller

import (
	"net/http"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type OpenAPIController struct {
	service *service.OpenAPIService
}

type openAPIOrderStatusPayload struct {
	Status models.OrderStatus `json:"status" binding:"required"`
}

type inventorySyncPayload struct {
	SKUCode string `json:"sku_code" binding:"required"`
	Stock   int64  `json:"stock"`
}

func NewOpenAPIController(service *service.OpenAPIService) *OpenAPIController {
	return &OpenAPIController{service: service}
}

func (ctl *OpenAPIController) RegisterRoutes(group *gin.RouterGroup) {
	orders := group.Group("/orders")
	orders.GET("", ctl.ListOrders)
	orders.POST("/:id/status", ctl.UpdateOrderStatus)

	inventory := group.Group("/inventory")
	inventory.POST("/sync", ctl.SyncInventory)
}

func (ctl *OpenAPIController) ListOrders(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	status := models.OrderStatus(c.Query("status"))
	orders, err := ctl.service.ListOrders(c.Request.Context(), tenantID, status)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]adminOrderResponse, 0, len(orders))
	for i := range orders {
		items = append(items, toAdminOrderResponse(&orders[i]))
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *OpenAPIController) UpdateOrderStatus(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var payload openAPIOrderStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	order, err := ctl.service.UpdateOrderStatus(c.Request.Context(), tenantID, id, payload.Status)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: toAdminOrderResponse(order)})
}

func (ctl *OpenAPIController) SyncInventory(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload []inventorySyncPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	items := make([]service.InventorySyncItem, 0, len(payload))
	for _, item := range payload {
		items = append(items, service.InventorySyncItem{
			SKUCode: item.SKUCode,
			Stock:   item.Stock,
		})
	}

	result, err := ctl.service.SyncInventory(c.Request.Context(), tenantID, items)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: result})
}
