package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fakeControllerOrderRepository struct {
	skus    map[uint]repository.CheckoutSKU
	created *models.Order
}

type fakeControllerOrderTx struct {
	repo *fakeControllerOrderRepository
}

func (r *fakeControllerOrderRepository) WithTransaction(_ context.Context, fn func(tx repository.OrderTransaction) error) error {
	return fn(&fakeControllerOrderTx{repo: r})
}

func (r *fakeControllerOrderRepository) ListAdminOrders(_ context.Context, _ repository.AdminOrderQuery) ([]models.Order, int64, error) {
	return nil, 0, nil
}

func (r *fakeControllerOrderRepository) ListStorefrontOrders(_ context.Context, _ uint, _ uint) ([]models.Order, error) {
	return nil, nil
}

func (r *fakeControllerOrderRepository) FindByIDWithTenant(_ context.Context, _ uint, _ uint) (*models.Order, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeControllerOrderRepository) UpdateStatusWithTenant(_ context.Context, _ uint, _ uint, _ models.OrderStatus) error {
	return gorm.ErrRecordNotFound
}

func (r *fakeControllerOrderRepository) ShipWithTenant(_ context.Context, _ uint, _ uint, _ string) error {
	return gorm.ErrRecordNotFound
}

func (r *fakeControllerOrderRepository) UpdatePaymentStatusWithTenant(_ context.Context, _ uint, _ uint, _ models.PaymentStatus) error {
	return gorm.ErrRecordNotFound
}

func (tx *fakeControllerOrderTx) FindCheckoutSKUForUpdate(tenantID uint, skuID uint) (*repository.CheckoutSKU, error) {
	sku, ok := tx.repo.skus[skuID]
	if !ok || sku.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	return &sku, nil
}

func (tx *fakeControllerOrderTx) FindCheckoutSKUForUpdateByID(skuID uint) (*repository.CheckoutSKU, error) {
	sku, ok := tx.repo.skus[skuID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &sku, nil
}

func (tx *fakeControllerOrderTx) DeductSKUStock(skuID uint, quantity int) error {
	sku, ok := tx.repo.skus[skuID]
	if !ok || sku.SellableStock < int64(quantity) {
		return gorm.ErrRecordNotFound
	}
	sku.SellableStock -= int64(quantity)
	tx.repo.skus[skuID] = sku
	return nil
}

func (tx *fakeControllerOrderTx) CreateOrder(order *models.Order) error {
	order.ID = 1
	now := time.Now().UTC()
	order.CreatedAt = now
	order.UpdatedAt = now
	tx.repo.created = order
	return nil
}

func newStorefrontOrderTestRouter(repo *fakeControllerOrderRepository, storefrontAuthEnabled bool) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/v1")
	controller := NewStorefrontOrderController(service.NewOrderService(repo), storefrontAuthEnabled)
	group.Use(controller.RequireStorefrontAuthEnabled())
	group.Use(middleware.RequireCustomerAuth("secret"))
	controller.RegisterRoutes(group)
	return router
}

func TestStorefrontCheckoutUsesCustomerIDFromJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeControllerOrderRepository{
		skus: map[uint]repository.CheckoutSKU{
			101: {
				ID:            101,
				TenantID:      10,
				ProductID:     201,
				ProductName:   "Test Product",
				RetailPrice:   1200,
				SellableStock: 5,
			},
		},
	}
	router := newStorefrontOrderTestRouter(repo, true)

	token, err := auth.GenerateCustomerToken("secret", time.Hour, 123)
	if err != nil {
		t.Fatalf("GenerateCustomerToken returned error: %v", err)
	}

	body := []byte(`{
		"customer_id": 999,
		"contact_name": "Alice",
		"contact_phone": "233501234567",
		"address": "Accra",
		"fulfillment_method": "pickup",
		"items": [{"sku_id": 101, "quantity": 2}]
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/orders/checkout", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if repo.created == nil || repo.created.CustomerID == nil || *repo.created.CustomerID != 123 {
		t.Fatalf("created customer_id = %v, want 123", repo.created)
	}

	var payload struct {
		Data struct {
			CustomerID uint `json:"customer_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Data.CustomerID != 123 {
		t.Fatalf("response customer_id = %d, want 123", payload.Data.CustomerID)
	}
}

func TestStorefrontOrderRoutesReturnForbiddenWhenStorefrontAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeControllerOrderRepository{}
	router := newStorefrontOrderTestRouter(repo, false)

	token, err := auth.GenerateCustomerToken("secret", time.Hour, 123)
	if err != nil {
		t.Fatalf("GenerateCustomerToken returned error: %v", err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
	}{
		{
			name:   "list without token",
			method: http.MethodGet,
			path:   "/api/v1/storefront/orders",
		},
		{
			name:   "checkout without token",
			method: http.MethodPost,
			path:   "/api/v1/storefront/orders/checkout",
			body:   `{"contact_name":"Alice","contact_phone":"233501234567","address":"Accra","fulfillment_method":"pickup","items":[{"sku_id":101,"quantity":1}]}`,
		},
		{
			name:   "list with token",
			method: http.MethodGet,
			path:   "/api/v1/storefront/orders",
			token:  token,
		},
		{
			name:   "checkout with token",
			method: http.MethodPost,
			path:   "/api/v1/storefront/orders/checkout",
			body:   `{"contact_name":"Alice","contact_phone":"233501234567","address":"Accra","fulfillment_method":"pickup","items":[{"sku_id":101,"quantity":1}]}`,
			token:  token,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(tt.body)))
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusForbidden, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "Storefront checkout is not available yet") {
				t.Fatalf("body = %s, want storefront unavailable message", response.Body.String())
			}
		})
	}
}

func TestStorefrontOrderRoutesRequireCustomerTokenWhenStorefrontAuthEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeControllerOrderRepository{}
	router := newStorefrontOrderTestRouter(repo, true)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "list",
			method: http.MethodGet,
			path:   "/api/v1/storefront/orders",
		},
		{
			name:   "checkout",
			method: http.MethodPost,
			path:   "/api/v1/storefront/orders/checkout",
			body:   `{"contact_name":"Alice","contact_phone":"233501234567","address":"Accra","fulfillment_method":"pickup","items":[{"sku_id":101,"quantity":1}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(tt.body)))
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
		})
	}
}
