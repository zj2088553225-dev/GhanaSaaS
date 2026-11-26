package service

import (
	"context"
	"errors"
	"strings"

	"saas/internal/models"

	"gorm.io/gorm"
)

const maxInventorySyncItems = 500

type InventorySyncItem struct {
	SKUCode string
	Stock   int64
}

type InventorySyncResult struct {
	Updated int `json:"updated"`
}

type OpenAPIService struct {
	db *gorm.DB
}

func NewOpenAPIService(db *gorm.DB) *OpenAPIService {
	return &OpenAPIService{db: db}
}

func (s *OpenAPIService) ListOrders(ctx context.Context, tenantID uint, status models.OrderStatus) ([]models.Order, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	if status != "" && !isValidOrderStatus(status) {
		return nil, ErrInvalidInput
	}

	db := s.db.WithContext(ctx).
		Preload("User").
		Preload("Items").
		Preload("Items.SKU").
		Where("tenant_id = ?", tenantID).
		Order("id ASC")
	if status != "" {
		db = db.Where("status = ?", status)
	}

	var orders []models.Order
	if err := db.Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *OpenAPIService) UpdateOrderStatus(ctx context.Context, tenantID uint, id uint, nextStatus models.OrderStatus) (*models.Order, error) {
	if tenantID == 0 || id == 0 {
		return nil, ErrInvalidInput
	}
	if nextStatus != models.OrderStatusProcessing && nextStatus != models.OrderStatusShipped {
		return nil, ErrInvalidInput
	}

	var order models.Order
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !canOpenAPITransitionOrderStatus(order.Status, nextStatus) {
		return nil, ErrConflict
	}

	if err := s.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("status", nextStatus).Error; err != nil {
		return nil, err
	}

	var updated models.Order
	if err := s.db.WithContext(ctx).
		Preload("User").
		Preload("Items").
		Preload("Items.SKU").
		Where("tenant_id = ?", tenantID).
		First(&updated, id).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *OpenAPIService) SyncInventory(ctx context.Context, tenantID uint, items []InventorySyncItem) (*InventorySyncResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	if len(items) == 0 || len(items) > maxInventorySyncItems {
		return nil, ErrInvalidInput
	}

	normalized := make([]InventorySyncItem, 0, len(items))
	for _, item := range items {
		item.SKUCode = strings.TrimSpace(item.SKUCode)
		if item.SKUCode == "" || item.Stock < 0 {
			return nil, ErrInvalidInput
		}
		normalized = append(normalized, item)
	}

	updated := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range normalized {
			result := tx.Model(&models.SKU{}).
				Where("erp_sku_code = ? AND is_active = ? AND product_id IN (?)",
					item.SKUCode,
					true,
					tx.Model(&models.SPU{}).Select("id").Where("tenant_id = ?", tenantID),
				).
				Update("sellable_stock", item.Stock)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrNotFound
			}
			updated += int(result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &InventorySyncResult{Updated: updated}, nil
}

func canOpenAPITransitionOrderStatus(current models.OrderStatus, next models.OrderStatus) bool {
	if current == next {
		return true
	}
	switch current {
	case models.OrderStatusPending:
		return next == models.OrderStatusProcessing || next == models.OrderStatusShipped
	case models.OrderStatusProcessing:
		return next == models.OrderStatusShipped
	default:
		return false
	}
}
