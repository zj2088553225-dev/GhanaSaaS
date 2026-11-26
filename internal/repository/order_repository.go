package repository

import (
	"context"
	"strings"
	"time"

	"saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CheckoutSKU struct {
	ID            uint
	TenantID      uint
	ProductID     uint
	ProductName   string
	RetailPrice   int64
	SellableStock int64
}

type AdminOrderQuery struct {
	TenantID  uint
	OrderNo   string
	Phone     string
	Status    *models.OrderStatus
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	PageSize  int
	NoPage    bool
}

type OrderRepository struct {
	db                   *gorm.DB
	platformMallTenantID uint
}

type OrderTransaction interface {
	FindCheckoutSKUForUpdate(tenantID uint, skuID uint) (*CheckoutSKU, error)
	FindCheckoutSKUForUpdateByID(skuID uint) (*CheckoutSKU, error)
	DeductSKUStock(skuID uint, quantity int) error
	CreateOrder(order *models.Order) error
}

func NewOrderRepository(db *gorm.DB, options ...PublicProductScopeOptions) *OrderRepository {
	repo := &OrderRepository{db: db}
	if len(options) > 0 {
		repo.platformMallTenantID = options[0].PlatformMallTenantID
	}
	return repo
}

func (r *OrderRepository) WithTransaction(ctx context.Context, fn func(tx OrderTransaction) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&OrderTx{tx: tx, platformMallTenantID: r.platformMallTenantID})
	})
}

func (r *OrderRepository) ListAdminOrders(ctx context.Context, query AdminOrderQuery) ([]models.Order, int64, error) {
	db := r.adminOrderBaseQuery(ctx, query)

	var total int64
	if err := db.Session(&gorm.Session{}).Model(&models.Order{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	listDB := db.Session(&gorm.Session{}).
		Preload("Items").
		Preload("Items.SKU").
		Preload("User").
		Preload("Customer").
		Order("id DESC")

	if !query.NoPage {
		listDB = listDB.Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize)
	}

	var orders []models.Order
	if err := listDB.Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

func (r *OrderRepository) ListStorefrontOrders(ctx context.Context, tenantID uint, customerID uint) ([]models.Order, error) {
	db := r.db.WithContext(ctx).
		Preload("Items").
		Preload("Items.SKU").
		Preload("Items.SKU.Product").
		Where("customer_id = ?", customerID)
	if tenantID != 0 {
		db = db.Where("tenant_id = ?", tenantID)
	}

	var orders []models.Order
	if err := db.
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *OrderRepository) FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.Order, error) {
	var order models.Order
	err := r.db.WithContext(ctx).
		Preload("Items").
		Preload("Items.SKU").
		Preload("User").
		Preload("Customer").
		Where("tenant_id = ?", tenantID).
		First(&order, id).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *OrderRepository) UpdateStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.OrderStatus) error {
	result := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *OrderRepository) ShipWithTenant(ctx context.Context, tenantID uint, id uint, trackingNumber string) error {
	result := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(map[string]any{
			"status":          models.OrderStatusShipped,
			"tracking_number": trackingNumber,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *OrderRepository) UpdatePaymentStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.PaymentStatus) error {
	result := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("payment_status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *OrderRepository) adminOrderBaseQuery(ctx context.Context, query AdminOrderQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Where("tenant_id = ?", query.TenantID)

	if orderNo := strings.TrimSpace(query.OrderNo); orderNo != "" {
		db = db.Where("order_no ILIKE ?", "%"+orderNo+"%")
	}
	if phone := strings.TrimSpace(query.Phone); phone != "" {
		db = db.Where("contact_phone ILIKE ?", "%"+phone+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.StartDate != nil {
		db = db.Where("created_at >= ?", *query.StartDate)
	}
	if query.EndDate != nil {
		db = db.Where("created_at < ?", *query.EndDate)
	}

	return db
}

type OrderTx struct {
	tx                   *gorm.DB
	platformMallTenantID uint
}

func (tx *OrderTx) FindCheckoutSKUForUpdate(tenantID uint, skuID uint) (*CheckoutSKU, error) {
	var sku CheckoutSKU
	condition, args := tx.publicCheckoutProductCondition()
	err := tx.tx.
		Table("skus").
		Select("skus.id, spus.tenant_id, skus.product_id, spus.name AS product_name, skus.retail_price, skus.sellable_stock").
		Joins("JOIN spus ON spus.id = skus.product_id").
		Joins("JOIN tenants ON tenants.id = spus.tenant_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("skus.id = ? AND skus.is_active = ? AND spus.tenant_id = ? AND spus.status = ? AND tenants.status = ?", skuID, true, tenantID, models.SPUStatusOnShelf, models.TenantStatusActive).
		Where(condition, args...).
		First(&sku).Error
	if err != nil {
		return nil, err
	}
	return &sku, nil
}

func (tx *OrderTx) FindCheckoutSKUForUpdateByID(skuID uint) (*CheckoutSKU, error) {
	var sku CheckoutSKU
	condition, args := tx.publicCheckoutProductCondition()
	err := tx.tx.
		Table("skus").
		Select("skus.id, spus.tenant_id, skus.product_id, spus.name AS product_name, skus.retail_price, skus.sellable_stock").
		Joins("JOIN spus ON spus.id = skus.product_id").
		Joins("JOIN tenants ON tenants.id = spus.tenant_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("skus.id = ? AND skus.is_active = ? AND spus.status = ? AND tenants.status = ?", skuID, true, models.SPUStatusOnShelf, models.TenantStatusActive).
		Where(condition, args...).
		First(&sku).Error
	if err != nil {
		return nil, err
	}
	return &sku, nil
}

func (tx *OrderTx) publicCheckoutProductCondition() (string, []any) {
	return publicProductSourceCondition("spus", "tenants", ProductSourceAll, tx.platformMallTenantID)
}

func (tx *OrderTx) DeductSKUStock(skuID uint, quantity int) error {
	result := tx.tx.
		Model(&models.SKU{}).
		Where("id = ? AND sellable_stock >= ?", skuID, quantity).
		Update("sellable_stock", gorm.Expr("sellable_stock - ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (tx *OrderTx) CreateOrder(order *models.Order) error {
	return tx.tx.Create(order).Error
}
