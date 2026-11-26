package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"gorm.io/gorm"
)

const maxCheckoutItems = 100
const maxAdminOrderPageSize = 100

type CheckoutItemInput struct {
	SKUID    uint
	Quantity int
}

type CheckoutInput struct {
	CustomerID        uint
	TenantID          uint
	ContactName       string
	ContactPhone      string
	Address           string
	FulfillmentMethod models.FulfillmentMethod
	Items             []CheckoutItemInput
}

type AdminOrderQuery struct {
	OrderNo   string
	Phone     string
	Status    *models.OrderStatus
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	PageSize  int
	NoPage    bool
}

type AdminOrderPage struct {
	Items    []models.Order
	Total    int64
	Page     int
	PageSize int
}

type orderRepository interface {
	WithTransaction(ctx context.Context, fn func(tx repository.OrderTransaction) error) error
	ListAdminOrders(ctx context.Context, query repository.AdminOrderQuery) ([]models.Order, int64, error)
	ListStorefrontOrders(ctx context.Context, tenantID uint, customerID uint) ([]models.Order, error)
	FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.Order, error)
	UpdateStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.OrderStatus) error
	ShipWithTenant(ctx context.Context, tenantID uint, id uint, trackingNumber string) error
	UpdatePaymentStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.PaymentStatus) error
}

type OrderService struct {
	repo orderRepository
}

func NewOrderService(repo orderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) Checkout(ctx context.Context, input CheckoutInput) (*models.Order, error) {
	normalized, err := normalizeCheckoutInput(input)
	if err != nil {
		return nil, err
	}

	var created *models.Order
	err = s.repo.WithTransaction(ctx, func(tx repository.OrderTransaction) error {
		customerID := normalized.CustomerID
		tenantID := normalized.TenantID
		order := models.Order{
			OrderNo:           generateOrderNo(customerID),
			CustomerID:        &customerID,
			Status:            models.OrderStatusPending,
			FulfillmentMethod: normalized.FulfillmentMethod,
			PaymentStatus:     models.PaymentStatusUnpaid,
			ContactName:       normalized.ContactName,
			ContactPhone:      normalized.ContactPhone,
			Address:           normalized.Address,
			Items:             make([]models.OrderItem, 0, len(normalized.Items)),
		}

		for _, item := range normalized.Items {
			sku, err := findCheckoutSKUForOrder(tx, tenantID, item.SKUID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if sku.TenantID == 0 {
				return ErrInvalidInput
			}
			if tenantID == 0 {
				tenantID = sku.TenantID
			}
			if sku.TenantID != tenantID {
				return ErrInvalidInput
			}
			if sku.SellableStock < int64(item.Quantity) {
				return ErrConflict
			}
			if err := tx.DeductSKUStock(item.SKUID, item.Quantity); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrConflict
				}
				return err
			}

			lineTotal := sku.RetailPrice * int64(item.Quantity)
			if lineTotal <= 0 || order.TotalAmount > (1<<63-1)-lineTotal {
				return ErrInvalidInput
			}
			order.TotalAmount += lineTotal
			order.Items = append(order.Items, models.OrderItem{
				SKUID:       item.SKUID,
				ProductName: sku.ProductName,
				Price:       sku.RetailPrice,
				Quantity:    item.Quantity,
			})
		}

		if tenantID == 0 {
			return ErrInvalidInput
		}
		order.TenantID = tenantID
		if err := tx.CreateOrder(&order); err != nil {
			return err
		}
		created = &order
		return nil
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *OrderService) ListAdminOrders(ctx context.Context, tenantID uint, query AdminOrderQuery) (*AdminOrderPage, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	if query.Status != nil && !isValidOrderStatus(*query.Status) {
		return nil, ErrInvalidInput
	}
	query.Page, query.PageSize = normalizeAdminOrderPagination(query.Page, query.PageSize)

	orders, total, err := s.repo.ListAdminOrders(ctx, repository.AdminOrderQuery{
		TenantID:  tenantID,
		OrderNo:   query.OrderNo,
		Phone:     query.Phone,
		Status:    query.Status,
		StartDate: query.StartDate,
		EndDate:   query.EndDate,
		Page:      query.Page,
		PageSize:  query.PageSize,
		NoPage:    query.NoPage,
	})
	if err != nil {
		return nil, err
	}

	return &AdminOrderPage{
		Items:    orders,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *OrderService) ListStorefrontOrders(ctx context.Context, tenantID uint, customerID uint) ([]models.Order, error) {
	if customerID == 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListStorefrontOrders(ctx, tenantID, customerID)
}

func (s *OrderService) UpdateAdminOrderStatus(ctx context.Context, tenantID uint, id uint, nextStatus models.OrderStatus) (*models.Order, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}
	if !isValidOrderStatus(nextStatus) {
		return nil, ErrInvalidInput
	}

	order, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !canTransitionOrderStatus(order.Status, nextStatus) {
		return nil, ErrConflict
	}
	if nextStatus == models.OrderStatusShipped &&
		normalizedFulfillmentMethod(order.FulfillmentMethod) == models.FulfillmentMethodDelivery &&
		strings.TrimSpace(order.TrackingNumber) == "" {
		return nil, ErrInvalidInput
	}

	if err := s.repo.UpdateStatusWithTenant(ctx, tenantID, id, nextStatus); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	updated, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return updated, err
}

func (s *OrderService) ShipAdminOrder(ctx context.Context, tenantID uint, id uint, trackingNumber string) (*models.Order, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}

	order, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !canTransitionOrderStatus(order.Status, models.OrderStatusShipped) {
		return nil, ErrConflict
	}

	method := normalizedFulfillmentMethod(order.FulfillmentMethod)
	if !isValidFulfillmentMethod(method) {
		return nil, ErrConflict
	}

	trackingNumber = strings.TrimSpace(trackingNumber)
	if method == models.FulfillmentMethodDelivery && trackingNumber == "" {
		return nil, ErrInvalidInput
	}

	if err := s.repo.ShipWithTenant(ctx, tenantID, id, trackingNumber); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	updated, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return updated, err
}

func (s *OrderService) MarkAdminOrderPaid(ctx context.Context, tenantID uint, id uint) (*models.Order, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}

	order, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !canTransitionPaymentStatus(normalizedPaymentStatus(order.PaymentStatus), models.PaymentStatusPaid) {
		return nil, ErrConflict
	}

	if err := s.repo.UpdatePaymentStatusWithTenant(ctx, tenantID, id, models.PaymentStatusPaid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	updated, err := s.repo.FindByIDWithTenant(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return updated, err
}

func normalizeCheckoutInput(input CheckoutInput) (CheckoutInput, error) {
	input.ContactName = strings.TrimSpace(input.ContactName)
	input.ContactPhone = strings.TrimSpace(input.ContactPhone)
	input.Address = strings.TrimSpace(input.Address)
	input.FulfillmentMethod = normalizedFulfillmentMethod(input.FulfillmentMethod)

	if input.CustomerID == 0 || input.ContactName == "" || input.ContactPhone == "" || input.Address == "" {
		return CheckoutInput{}, ErrInvalidInput
	}
	if !isValidFulfillmentMethod(input.FulfillmentMethod) {
		return CheckoutInput{}, ErrInvalidInput
	}
	if len(input.Items) == 0 || len(input.Items) > maxCheckoutItems {
		return CheckoutInput{}, ErrInvalidInput
	}

	merged := make(map[uint]int, len(input.Items))
	order := make([]uint, 0, len(input.Items))
	for _, item := range input.Items {
		if item.SKUID == 0 || item.Quantity <= 0 {
			return CheckoutInput{}, ErrInvalidInput
		}
		if _, ok := merged[item.SKUID]; !ok {
			order = append(order, item.SKUID)
		}
		merged[item.SKUID] += item.Quantity
		if merged[item.SKUID] <= 0 {
			return CheckoutInput{}, ErrInvalidInput
		}
	}

	items := make([]CheckoutItemInput, 0, len(order))
	for _, skuID := range order {
		items = append(items, CheckoutItemInput{SKUID: skuID, Quantity: merged[skuID]})
	}
	input.Items = items
	return input, nil
}

func findCheckoutSKUForOrder(tx repository.OrderTransaction, tenantID uint, skuID uint) (*repository.CheckoutSKU, error) {
	if tenantID == 0 {
		return tx.FindCheckoutSKUForUpdateByID(skuID)
	}
	return tx.FindCheckoutSKUForUpdate(tenantID, skuID)
}

func normalizeAdminOrderPagination(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > maxAdminOrderPageSize {
		pageSize = maxAdminOrderPageSize
	}
	return page, pageSize
}

func isValidOrderStatus(status models.OrderStatus) bool {
	return status == models.OrderStatusPending ||
		status == models.OrderStatusProcessing ||
		status == models.OrderStatusShipped ||
		status == models.OrderStatusCompleted
}

func isValidFulfillmentMethod(method models.FulfillmentMethod) bool {
	return method == models.FulfillmentMethodPickup || method == models.FulfillmentMethodDelivery
}

func normalizedFulfillmentMethod(method models.FulfillmentMethod) models.FulfillmentMethod {
	if method == "" {
		return models.FulfillmentMethodPickup
	}
	return method
}

func normalizedPaymentStatus(status models.PaymentStatus) models.PaymentStatus {
	if status == "" {
		return models.PaymentStatusUnpaid
	}
	return status
}

func canTransitionPaymentStatus(current models.PaymentStatus, next models.PaymentStatus) bool {
	if current == next {
		return true
	}
	return current == models.PaymentStatusUnpaid && next == models.PaymentStatusPaid
}

func canTransitionOrderStatus(current models.OrderStatus, next models.OrderStatus) bool {
	if current == next {
		return true
	}
	switch current {
	case models.OrderStatusPending:
		return next == models.OrderStatusProcessing || next == models.OrderStatusShipped
	case models.OrderStatusProcessing:
		return next == models.OrderStatusShipped
	case models.OrderStatusShipped:
		return next == models.OrderStatusCompleted
	default:
		return false
	}
}

func generateOrderNo(userID uint) string {
	return fmt.Sprintf("ORD%d%06d", time.Now().UTC().UnixNano(), userID)
}
