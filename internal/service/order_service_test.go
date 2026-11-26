package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

type fakeOrderRepository struct {
	mu     sync.Mutex
	skus   map[uint]repository.CheckoutSKU
	orders map[uint]models.Order
	nextID uint
}

type fakeOrderTx struct {
	repo   *fakeOrderRepository
	skus   map[uint]repository.CheckoutSKU
	orders map[uint]models.Order
}

func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{
		skus: map[uint]repository.CheckoutSKU{
			101: {
				ID:            101,
				TenantID:      10,
				ProductID:     201,
				ProductName:   "Test Product",
				RetailPrice:   1200,
				SellableStock: 2,
			},
			102: {
				ID:            102,
				TenantID:      10,
				ProductID:     202,
				ProductName:   "Out Of Stock Product",
				RetailPrice:   800,
				SellableStock: 0,
			},
		},
		orders: make(map[uint]models.Order),
		nextID: 1,
	}
}

func (r *fakeOrderRepository) WithTransaction(_ context.Context, fn func(tx repository.OrderTransaction) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx := &fakeOrderTx{
		repo:   r,
		skus:   cloneCheckoutSKUs(r.skus),
		orders: cloneOrdersMap(r.orders),
	}
	if err := fn(tx); err != nil {
		return err
	}

	r.skus = tx.skus
	r.orders = tx.orders
	return nil
}

func (r *fakeOrderRepository) ListAdminOrders(_ context.Context, query repository.AdminOrderQuery) ([]models.Order, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]models.Order, 0)
	for _, order := range r.orders {
		if order.TenantID != query.TenantID {
			continue
		}
		if keyword := strings.TrimSpace(query.OrderNo); keyword != "" && !strings.Contains(order.OrderNo, keyword) {
			continue
		}
		if phone := strings.TrimSpace(query.Phone); phone != "" && !strings.Contains(order.ContactPhone, phone) {
			continue
		}
		if query.Status != nil && order.Status != *query.Status {
			continue
		}
		if query.StartDate != nil && order.CreatedAt.Before(*query.StartDate) {
			continue
		}
		if query.EndDate != nil && !order.CreatedAt.Before(*query.EndDate) {
			continue
		}
		items = append(items, cloneOrder(order))
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	total := int64(len(items))
	if query.NoPage {
		return items, total, nil
	}

	start := (query.Page - 1) * query.PageSize
	if start >= len(items) {
		return []models.Order{}, total, nil
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

func (r *fakeOrderRepository) ListStorefrontOrders(_ context.Context, tenantID uint, customerID uint) ([]models.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]models.Order, 0)
	for _, order := range r.orders {
		if order.CustomerID == nil || *order.CustomerID != customerID {
			continue
		}
		if tenantID == 0 || order.TenantID == tenantID {
			items = append(items, cloneOrder(order))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (r *fakeOrderRepository) FindByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	order, ok := r.orders[id]
	if !ok || order.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := cloneOrder(order)
	return &cloned, nil
}

func (r *fakeOrderRepository) UpdateStatusWithTenant(_ context.Context, tenantID uint, id uint, status models.OrderStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	order, ok := r.orders[id]
	if !ok || order.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	order.Status = status
	r.orders[id] = order
	return nil
}

func (r *fakeOrderRepository) ShipWithTenant(_ context.Context, tenantID uint, id uint, trackingNumber string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	order, ok := r.orders[id]
	if !ok || order.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	order.Status = models.OrderStatusShipped
	order.TrackingNumber = trackingNumber
	r.orders[id] = order
	return nil
}

func (r *fakeOrderRepository) UpdatePaymentStatusWithTenant(_ context.Context, tenantID uint, id uint, status models.PaymentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	order, ok := r.orders[id]
	if !ok || order.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	order.PaymentStatus = status
	r.orders[id] = order
	return nil
}

func (tx *fakeOrderTx) FindCheckoutSKUForUpdate(tenantID uint, skuID uint) (*repository.CheckoutSKU, error) {
	sku, ok := tx.skus[skuID]
	if !ok || sku.TenantID != tenantID {
		return nil, errors.New("record not found")
	}
	return &sku, nil
}

func (tx *fakeOrderTx) FindCheckoutSKUForUpdateByID(skuID uint) (*repository.CheckoutSKU, error) {
	sku, ok := tx.skus[skuID]
	if !ok {
		return nil, errors.New("record not found")
	}
	return &sku, nil
}

func (tx *fakeOrderTx) DeductSKUStock(skuID uint, quantity int) error {
	sku, ok := tx.skus[skuID]
	if !ok || sku.SellableStock < int64(quantity) {
		return errors.New("record not found")
	}
	sku.SellableStock -= int64(quantity)
	tx.skus[skuID] = sku
	return nil
}

func (tx *fakeOrderTx) CreateOrder(order *models.Order) error {
	order.ID = tx.repo.nextID
	tx.repo.nextID++
	order.CreatedAt = time.Now().UTC()
	order.UpdatedAt = order.CreatedAt
	for i := range order.Items {
		order.Items[i].ID = uint(i + 1)
		order.Items[i].OrderID = order.ID
		order.Items[i].CreatedAt = order.CreatedAt
		order.Items[i].UpdatedAt = order.UpdatedAt
	}
	tx.orders[order.ID] = cloneOrder(*order)
	return nil
}

func cloneCheckoutSKUs(input map[uint]repository.CheckoutSKU) map[uint]repository.CheckoutSKU {
	output := make(map[uint]repository.CheckoutSKU, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneOrdersMap(input map[uint]models.Order) map[uint]models.Order {
	output := make(map[uint]models.Order, len(input))
	for key, value := range input {
		output[key] = cloneOrder(value)
	}
	return output
}

func cloneOrder(order models.Order) models.Order {
	order.Items = append([]models.OrderItem(nil), order.Items...)
	return order
}

func newAdminOrderTestService() *OrderService {
	return NewOrderService(newFakeOrderRepository())
}

func seedAdminOrder(repo *fakeOrderRepository, order models.Order) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if order.ID == 0 {
		order.ID = repo.nextID
		repo.nextID++
	}
	order.CreatedAt = time.Now().UTC()
	order.UpdatedAt = order.CreatedAt
	repo.orders[order.ID] = cloneOrder(order)
}

func validCheckoutInput() CheckoutInput {
	return CheckoutInput{
		CustomerID:   1,
		TenantID:     10,
		ContactName:  "Alice",
		ContactPhone: "13800000001",
		Address:      "Test Address",
		Items:        []CheckoutItemInput{{SKUID: 101, Quantity: 2}},
	}
}

func TestOrderServiceCheckoutCreatesOrderAndDeductsStock(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)

	order, err := service.Checkout(context.Background(), validCheckoutInput())
	if err != nil {
		t.Fatalf("Checkout returned error: %v", err)
	}
	if order.TotalAmount != 2400 {
		t.Fatalf("TotalAmount = %d, want 2400", order.TotalAmount)
	}
	if order.CustomerID == nil || *order.CustomerID != 1 {
		t.Fatalf("CustomerID = %v, want 1", order.CustomerID)
	}
	if len(order.Items) != 1 || order.Items[0].Price != 1200 || order.Items[0].Quantity != 2 {
		t.Fatalf("unexpected items: %+v", order.Items)
	}
	if order.FulfillmentMethod != models.FulfillmentMethodPickup {
		t.Fatalf("FulfillmentMethod = %q, want pickup", order.FulfillmentMethod)
	}
	if order.PaymentStatus != models.PaymentStatusUnpaid {
		t.Fatalf("PaymentStatus = %q, want unpaid", order.PaymentStatus)
	}
	if repo.skus[101].SellableStock != 0 {
		t.Fatalf("SellableStock = %d, want 0", repo.skus[101].SellableStock)
	}
	if len(repo.orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(repo.orders))
	}
}

func TestOrderServiceCheckoutRollsBackWhenStockInsufficient(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)

	_, err := service.Checkout(context.Background(), CheckoutInput{
		CustomerID:   1,
		TenantID:     10,
		ContactName:  "Alice",
		ContactPhone: "13800000001",
		Address:      "Test Address",
		Items: []CheckoutItemInput{
			{SKUID: 101, Quantity: 1},
			{SKUID: 102, Quantity: 1},
		},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Checkout error = %v, want ErrConflict", err)
	}
	if repo.skus[101].SellableStock != 2 {
		t.Fatalf("SellableStock = %d, want rollback to 2", repo.skus[101].SellableStock)
	}
	if len(repo.orders) != 0 {
		t.Fatalf("orders len = %d, want 0", len(repo.orders))
	}
}

func TestOrderServiceCheckoutStoresDeliveryFulfillment(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)

	input := validCheckoutInput()
	input.Items[0].Quantity = 1
	input.FulfillmentMethod = models.FulfillmentMethodDelivery

	order, err := service.Checkout(context.Background(), input)
	if err != nil {
		t.Fatalf("Checkout returned error: %v", err)
	}
	if order.FulfillmentMethod != models.FulfillmentMethodDelivery {
		t.Fatalf("FulfillmentMethod = %q, want delivery", order.FulfillmentMethod)
	}

	input.Items[0].Quantity = 1
	input.FulfillmentMethod = models.FulfillmentMethod("drone")
	if _, err := service.Checkout(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Checkout invalid fulfillment error = %v, want ErrInvalidInput", err)
	}
}

func TestOrderServiceAdminStatusTransitions(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)
	seedAdminOrder(repo, models.Order{
		UserID:       orderUintPtr(11),
		TenantID:     1001,
		OrderNo:      "ORD-1001",
		TotalAmount:  5000,
		Status:       models.OrderStatusPending,
		ContactName:  "Admin Buyer",
		ContactPhone: "13800000002",
		Address:      "Tenant A Address",
		Items: []models.OrderItem{{
			SKUID:       101,
			ProductName: "Test Product",
			Price:       1200,
			Quantity:    2,
		}},
	})
	seedAdminOrder(repo, models.Order{
		UserID:       orderUintPtr(12),
		TenantID:     2002,
		OrderNo:      "ORD-2002",
		TotalAmount:  3600,
		Status:       models.OrderStatusShipped,
		ContactName:  "Other Tenant",
		ContactPhone: "13800000003",
		Address:      "Tenant B Address",
	})

	shipped, err := service.UpdateAdminOrderStatus(context.Background(), 1001, 1, models.OrderStatusShipped)
	if err != nil {
		t.Fatalf("UpdateAdminOrderStatus shipped returned error: %v", err)
	}
	if shipped.Status != models.OrderStatusShipped {
		t.Fatalf("Status = %q, want shipped", shipped.Status)
	}

	completed, err := service.UpdateAdminOrderStatus(context.Background(), 1001, 1, models.OrderStatusCompleted)
	if err != nil {
		t.Fatalf("UpdateAdminOrderStatus completed returned error: %v", err)
	}
	if completed.Status != models.OrderStatusCompleted {
		t.Fatalf("Status = %q, want completed", completed.Status)
	}

	if _, err := service.UpdateAdminOrderStatus(context.Background(), 1001, 1, models.OrderStatusShipped); !errors.Is(err, ErrConflict) {
		t.Fatalf("Backwards transition error = %v, want ErrConflict", err)
	}
	if _, err := service.UpdateAdminOrderStatus(context.Background(), 1001, 2, models.OrderStatusCompleted); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cross tenant transition error = %v, want ErrNotFound", err)
	}
}

func TestOrderServiceAdminShipRequiresTrackingForDelivery(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)
	seedAdminOrder(repo, models.Order{
		UserID:            orderUintPtr(11),
		TenantID:          1001,
		OrderNo:           "ORD-DELIVERY",
		Status:            models.OrderStatusProcessing,
		FulfillmentMethod: models.FulfillmentMethodDelivery,
		PaymentStatus:     models.PaymentStatusUnpaid,
	})

	if _, err := service.ShipAdminOrder(context.Background(), 1001, 1, " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ShipAdminOrder missing tracking error = %v, want ErrInvalidInput", err)
	}

	shipped, err := service.ShipAdminOrder(context.Background(), 1001, 1, "  GH-2401  ")
	if err != nil {
		t.Fatalf("ShipAdminOrder returned error: %v", err)
	}
	if shipped.Status != models.OrderStatusShipped || shipped.TrackingNumber != "GH-2401" {
		t.Fatalf("unexpected shipped order: %+v", shipped)
	}
}

func TestOrderServiceAdminPayTransitionsByTenant(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)
	seedAdminOrder(repo, models.Order{
		UserID:        orderUintPtr(11),
		TenantID:      1001,
		OrderNo:       "ORD-UNPAID",
		PaymentStatus: models.PaymentStatusUnpaid,
	})
	seedAdminOrder(repo, models.Order{
		UserID:        orderUintPtr(12),
		TenantID:      1001,
		OrderNo:       "ORD-REFUNDED",
		PaymentStatus: models.PaymentStatusRefunded,
	})

	paid, err := service.MarkAdminOrderPaid(context.Background(), 1001, 1)
	if err != nil {
		t.Fatalf("MarkAdminOrderPaid returned error: %v", err)
	}
	if paid.PaymentStatus != models.PaymentStatusPaid {
		t.Fatalf("PaymentStatus = %q, want paid", paid.PaymentStatus)
	}
	if _, err := service.MarkAdminOrderPaid(context.Background(), 2002, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant MarkAdminOrderPaid error = %v, want ErrNotFound", err)
	}
	if _, err := service.MarkAdminOrderPaid(context.Background(), 1001, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("refunded MarkAdminOrderPaid error = %v, want ErrConflict", err)
	}
}

func TestOrderServiceAdminListFiltersAndExport(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)
	baseTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	repo.orders[1] = models.Order{BaseModel: models.BaseModel{ID: 1, CreatedAt: baseTime}, OrderNo: "ORD-A1", TenantID: 1001, ContactPhone: "13800000011", Status: models.OrderStatusPending}
	repo.orders[2] = models.Order{BaseModel: models.BaseModel{ID: 2, CreatedAt: baseTime.Add(24 * time.Hour)}, OrderNo: "ORD-A2", TenantID: 1001, ContactPhone: "13800000022", Status: models.OrderStatusShipped}
	repo.orders[3] = models.Order{BaseModel: models.BaseModel{ID: 3, CreatedAt: baseTime.Add(48 * time.Hour)}, OrderNo: "ORD-B1", TenantID: 2002, ContactPhone: "13800000033", Status: models.OrderStatusCompleted}

	page, err := service.ListAdminOrders(context.Background(), 1001, AdminOrderQuery{
		OrderNo:  "A2",
		Phone:    "22",
		Status:   ptrOrderStatus(models.OrderStatusShipped),
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListAdminOrders returned error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].OrderNo != "ORD-A2" {
		t.Fatalf("unexpected filtered page: %+v", page)
	}

	exported, err := service.ListAdminOrders(context.Background(), 1001, AdminOrderQuery{NoPage: true})
	if err != nil {
		t.Fatalf("ListAdminOrders export returned error: %v", err)
	}
	if exported.Total != 2 || len(exported.Items) != 2 {
		t.Fatalf("unexpected export page: %+v", exported)
	}
}

func TestOrderServiceListStorefrontOrdersScopesToCustomer(t *testing.T) {
	repo := newFakeOrderRepository()
	service := NewOrderService(repo)
	baseTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	customerID := uint(77)
	otherCustomerID := uint(88)

	repo.orders[1] = models.Order{BaseModel: models.BaseModel{ID: 1, CreatedAt: baseTime}, OrderNo: "ORD-CURRENT-A", TenantID: 1001, CustomerID: &customerID}
	repo.orders[2] = models.Order{BaseModel: models.BaseModel{ID: 2, CreatedAt: baseTime.Add(time.Hour)}, OrderNo: "ORD-CURRENT-B", TenantID: 2002, CustomerID: &customerID}
	repo.orders[3] = models.Order{BaseModel: models.BaseModel{ID: 3, CreatedAt: baseTime.Add(2 * time.Hour)}, OrderNo: "ORD-OTHER", TenantID: 1001, CustomerID: &otherCustomerID}

	all, err := service.ListStorefrontOrders(context.Background(), 0, customerID)
	if err != nil {
		t.Fatalf("ListStorefrontOrders returned error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("orders len = %d, want 2", len(all))
	}

	tenantScoped, err := service.ListStorefrontOrders(context.Background(), 1001, customerID)
	if err != nil {
		t.Fatalf("ListStorefrontOrders tenant-scoped returned error: %v", err)
	}
	if len(tenantScoped) != 1 || tenantScoped[0].OrderNo != "ORD-CURRENT-A" {
		t.Fatalf("unexpected tenant-scoped orders: %+v", tenantScoped)
	}
}

func TestOrderServiceRejectsInvalidAdminTenantAndStatus(t *testing.T) {
	service := newAdminOrderTestService()
	ctx := context.Background()

	if _, err := service.ListAdminOrders(ctx, 0, AdminOrderQuery{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListAdminOrders with zero tenant error = %v, want ErrForbidden", err)
	}
	if _, err := service.UpdateAdminOrderStatus(ctx, 0, 1, models.OrderStatusShipped); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateAdminOrderStatus with zero tenant error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.UpdateAdminOrderStatus(ctx, 1001, 1, models.OrderStatus("invalid")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateAdminOrderStatus invalid status error = %v, want ErrInvalidInput", err)
	}
}

func ptrOrderStatus(status models.OrderStatus) *models.OrderStatus {
	return &status
}

func orderUintPtr(value uint) *uint {
	return &value
}
