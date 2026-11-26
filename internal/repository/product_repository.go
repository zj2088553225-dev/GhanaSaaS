package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"saas/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProductHasOrderRecords   = errors.New("product has order records")
	ErrDuplicateSKUCode         = errors.New("sku code already exists")
	ErrProductCategoryMissing   = errors.New("product platform category does not exist")
	ErrProductTenantUnavailable = errors.New("product tenant is unavailable")
)

type SKUCodeLookup func(codes []string) (map[string]struct{}, error)
type SKUCodeGenerator func(lookup SKUCodeLookup) error

type ProductTranslationMutation struct {
	Provided    bool
	Name        string
	Description string
	DetailHTML  string
	Material    string
	Dimensions  string
	Weight      string
}

type AdminProductQuery struct {
	TenantID           uint
	CategoryID         *uint
	PlatformCategoryID *uint
	Keyword            string
	Status             *models.SPUStatus
	Page               int
	PageSize           int
}

type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) CreateAtomic(
	ctx context.Context,
	tenantID uint,
	product *models.SPU,
	translation ProductTranslationMutation,
	generateCodes SKUCodeGenerator,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateProductWriteScope(tx, tenantID, product); err != nil {
			return err
		}
		if generateCodes != nil {
			if err := generateCodes(func(codes []string) (map[string]struct{}, error) {
				return findSKUCodesWithDB(tx, codes)
			}); err != nil {
				return err
			}
		}
		if err := createProductWithSKUs(tx, product); err != nil {
			return err
		}
		return saveProductTranslationMutation(tx, product.ID, translation)
	})
	return normalizeProductWriteError(err)
}

func (r *ProductRepository) CreateManyAtomic(
	ctx context.Context,
	tenantID uint,
	products []models.SPU,
	generateCodes SKUCodeGenerator,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range products {
			if err := validateProductWriteScope(tx, tenantID, &products[i]); err != nil {
				return err
			}
		}
		if generateCodes != nil {
			if err := generateCodes(func(codes []string) (map[string]struct{}, error) {
				return findSKUCodesWithDB(tx, codes)
			}); err != nil {
				return err
			}
		}
		for i := range products {
			if err := createProductWithSKUs(tx, &products[i]); err != nil {
				return err
			}
		}
		return nil
	})
	return normalizeProductWriteError(err)
}

func (r *ProductRepository) FindByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.SPU, error) {
	var product models.SPU
	err := r.db.WithContext(ctx).
		Preload("SKUs", "is_active = ?", true).
		Where("tenant_id = ?", tenantID).
		First(&product, id).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductRepository) ListByTenant(ctx context.Context, query AdminProductQuery) ([]models.SPU, int64, error) {
	db := r.adminProductBaseQuery(ctx, query)

	var total int64
	if err := db.Session(&gorm.Session{}).Model(&models.SPU{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []models.SPU
	err := db.Session(&gorm.Session{}).
		Preload("SKUs", "is_active = ?", true).
		Order("id DESC").
		Limit(query.PageSize).
		Offset((query.Page - 1) * query.PageSize).
		Find(&products).Error
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (r *ProductRepository) FindCategoryByIDWithTenant(ctx context.Context, tenantID uint, id uint) (*models.Category, error) {
	var category models.Category
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *ProductRepository) FindPlatformCategoryByID(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	var category models.PlatformCategory
	if err := r.db.WithContext(ctx).First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *ProductRepository) FindPlatformCategoriesByPaths(ctx context.Context, paths []string) (map[string]models.PlatformCategory, error) {
	if len(paths) == 0 {
		return map[string]models.PlatformCategory{}, nil
	}
	normalizedPaths := make([]string, 0, len(paths))
	seenPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		key := strings.ToLower(strings.TrimSpace(path))
		if key == "" {
			continue
		}
		if _, exists := seenPaths[key]; exists {
			continue
		}
		seenPaths[key] = struct{}{}
		normalizedPaths = append(normalizedPaths, key)
	}
	if len(normalizedPaths) == 0 {
		return map[string]models.PlatformCategory{}, nil
	}

	var categories []models.PlatformCategory
	if err := r.db.WithContext(ctx).
		Where("LOWER(path) IN ?", normalizedPaths).
		Order("id ASC").
		Find(&categories).Error; err != nil {
		return nil, err
	}

	byPath := make(map[string]models.PlatformCategory, len(categories))
	for _, category := range categories {
		key := strings.ToLower(strings.TrimSpace(category.Path))
		if _, exists := byPath[key]; !exists {
			byPath[key] = category
		}
	}
	return byPath, nil
}

func (r *ProductRepository) HasPlatformCategoryChildren(ctx context.Context, id uint) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.PlatformCategory{}).
		Where("parent_id = ?", id).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ProductRepository) FindCategoriesByName(ctx context.Context, tenantID uint, names []string) (map[string]models.Category, error) {
	if len(names) == 0 {
		return map[string]models.Category{}, nil
	}
	normalizedNames := make([]string, 0, len(names))
	seenNames := make(map[string]struct{}, len(names))
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, exists := seenNames[key]; exists {
			continue
		}
		seenNames[key] = struct{}{}
		normalizedNames = append(normalizedNames, key)
	}
	if len(normalizedNames) == 0 {
		return map[string]models.Category{}, nil
	}

	var categories []models.Category
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND LOWER(name) IN ?", tenantID, normalizedNames).
		Order("id ASC").
		Find(&categories).Error; err != nil {
		return nil, err
	}

	byName := make(map[string]models.Category, len(categories))
	for _, category := range categories {
		key := strings.ToLower(strings.TrimSpace(category.Name))
		if _, exists := byName[key]; !exists {
			byName[key] = category
		}
	}
	return byName, nil
}

func findSKUCodesWithDB(db *gorm.DB, codes []string) (map[string]struct{}, error) {
	if len(codes) == 0 {
		return map[string]struct{}{}, nil
	}

	var skus []models.SKU
	if err := db.
		Model(&models.SKU{}).
		Where("erp_sku_code IN ?", codes).
		Find(&skus).Error; err != nil {
		return nil, err
	}

	existing := make(map[string]struct{}, len(skus))
	for _, sku := range skus {
		code := strings.TrimSpace(sku.ERPSKUCode)
		if code != "" {
			existing[code] = struct{}{}
		}
	}
	return existing, nil
}

func (r *ProductRepository) UpdateAtomic(
	ctx context.Context,
	tenantID uint,
	product *models.SPU,
	translation ProductTranslationMutation,
	generateCodes SKUCodeGenerator,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.SPU
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ?", tenantID).First(&existing, product.ID).Error; err != nil {
			return err
		}
		if err := validateProductWriteScope(tx, tenantID, product); err != nil {
			return err
		}

		var existingSKUs []models.SKU
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ?", product.ID).Find(&existingSKUs).Error; err != nil {
			return err
		}
		existingByID := make(map[uint]models.SKU, len(existingSKUs))
		for _, sku := range existingSKUs {
			existingByID[sku.ID] = sku
		}
		for i := range product.SKUs {
			if product.SKUs[i].ID == 0 {
				product.SKUs[i].ERPSKUCode = ""
				continue
			}
			existingSKU, ok := existingByID[product.SKUs[i].ID]
			if !ok || existingSKU.ProductID != product.ID {
				return gorm.ErrRecordNotFound
			}
			product.SKUs[i].ERPSKUCode = existingSKU.ERPSKUCode
		}
		if generateCodes != nil {
			if err := generateCodes(func(codes []string) (map[string]struct{}, error) {
				return findSKUCodesWithDB(tx, codes)
			}); err != nil {
				return err
			}
		}

		updates := map[string]any{
			"platform_category_id": product.PlatformCategoryID,
			"category_id":          product.CategoryID,
			"name":                 product.Name,
			"description":          product.Description,
			"detail_html":          product.DetailHTML,
			"main_image":           product.MainImage,
			"video_url":            product.VideoURL,
			"material":             product.Material,
			"dimensions":           product.Dimensions,
			"weight":               product.Weight,
			"slider_images":        product.SliderImages,
			"spec_template":        product.SpecTemplate,
			"status":               product.Status,
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}

		activeIDs := make([]uint, 0, len(product.SKUs))
		for i := range product.SKUs {
			sku := product.SKUs[i]
			sku.ProductID = product.ID
			sku.IsActive = true
			if sku.ID == 0 {
				if err := tx.Create(&sku).Error; err != nil {
					return err
				}
				activeIDs = append(activeIDs, sku.ID)
				continue
			}

			if existingSKU, ok := existingByID[sku.ID]; !ok || existingSKU.ProductID != product.ID {
				return gorm.ErrRecordNotFound
			}
			updates := map[string]any{
				"name":           sku.Name,
				"image_url":      sku.ImageURL,
				"specs":          sku.Specs,
				"retail_price":   sku.RetailPrice,
				"original_price": sku.OriginalPrice,
				"sellable_stock": sku.SellableStock,
				"is_active":      true,
			}
			if err := tx.Model(&models.SKU{}).
				Where("id = ? AND product_id = ?", sku.ID, product.ID).
				Updates(updates).Error; err != nil {
				return err
			}
			activeIDs = append(activeIDs, sku.ID)
		}

		deactivate := tx.Model(&models.SKU{}).
			Where("product_id = ? AND is_active = ?", product.ID, true)
		if len(activeIDs) > 0 {
			deactivate = deactivate.Where("id NOT IN ?", activeIDs)
		}
		if err := deactivate.Update("is_active", false).Error; err != nil {
			return err
		}
		return saveProductTranslationMutation(tx, product.ID, translation)
	})
	return normalizeProductWriteError(err)
}

func validateProductWriteScope(tx *gorm.DB, tenantID uint, product *models.SPU) error {
	if tenantID == 0 || product == nil || product.TenantID != tenantID {
		return ErrProductTenantUnavailable
	}
	var tenant models.Tenant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", tenantID, models.TenantStatusActive).
		First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductTenantUnavailable
		}
		return err
	}

	var platformCategory models.PlatformCategory
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", product.PlatformCategoryID, models.PlatformCategoryStatusActive).
		First(&platformCategory).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductCategoryMissing
		}
		return err
	}
	var childCount int64
	if err := tx.Model(&models.PlatformCategory{}).
		Where("parent_id = ?", platformCategory.ID).Count(&childCount).Error; err != nil {
		return err
	}
	if childCount > 0 {
		return ErrProductCategoryMissing
	}

	if product.CategoryID != nil {
		var category models.Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", *product.CategoryID, tenantID).
			First(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrProductCategoryMissing
			}
			return err
		}
	}
	return nil
}

func createProductWithSKUs(tx *gorm.DB, product *models.SPU) error {
	if err := tx.Omit(clause.Associations).Create(product).Error; err != nil {
		return err
	}
	for i := range product.SKUs {
		product.SKUs[i].ProductID = product.ID
		product.SKUs[i].IsActive = true
		if err := tx.Create(&product.SKUs[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

type productTranslationField struct {
	key       string
	textValue string
	htmlValue string
}

func saveProductTranslationMutation(tx *gorm.DB, productID uint, mutation ProductTranslationMutation) error {
	if !mutation.Provided {
		return nil
	}
	fields := []productTranslationField{
		{key: "name", textValue: strings.TrimSpace(mutation.Name)},
		{key: "description", textValue: strings.TrimSpace(mutation.Description)},
		{key: "detail_html", htmlValue: strings.TrimSpace(mutation.DetailHTML)},
		{key: "material", textValue: strings.TrimSpace(mutation.Material)},
		{key: "dimensions", textValue: strings.TrimSpace(mutation.Dimensions)},
		{key: "weight", textValue: strings.TrimSpace(mutation.Weight)},
	}
	now := time.Now()
	for _, field := range fields {
		if field.textValue == "" && field.htmlValue == "" {
			if err := tx.Where(
				"entity_type = ? AND entity_id = ? AND field_key = ? AND locale = ?",
				"product", productID, field.key, "zh",
			).Delete(&models.ContentTranslation{}).Error; err != nil {
				return err
			}
			continue
		}
		translation := models.ContentTranslation{
			BaseModel:  models.BaseModel{CreatedAt: now, UpdatedAt: now},
			EntityType: "product",
			EntityID:   productID,
			FieldKey:   field.key,
			Locale:     "zh",
			ValueText:  field.textValue,
			ValueHTML:  field.htmlValue,
			Status:     models.ContentTranslationStatusPublished,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "field_key"},
				{Name: "locale"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"value_text": gorm.Expr("excluded.value_text"),
				"value_html": gorm.Expr("excluded.value_html"),
				"value_json": nil,
				"status":     models.ContentTranslationStatusPublished,
				"updated_at": now,
			}),
		}).Create(&translation).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeProductWriteError(err error) error {
	if err == nil {
		return nil
	}
	if isDuplicateSKUCodeError(err) {
		return ErrDuplicateSKUCode
	}
	if isProductCategoryForeignKeyViolation(err) {
		return ErrProductCategoryMissing
	}
	return err
}

func isDuplicateSKUCodeError(err error) bool {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		constraint := strings.ToLower(pgError.ConstraintName)
		detail := strings.ToLower(pgError.Detail)
		return constraint == "idx_skus_erp_sku_code_unique" ||
			strings.Contains(constraint, "erp_sku_code") ||
			strings.Contains(detail, "erp_sku_code")
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_skus_erp_sku_code_unique") ||
		(strings.Contains(message, "unique") && strings.Contains(message, "skus.erp_sku_code"))
}

func isProductCategoryForeignKeyViolation(err error) bool {
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "23503" {
		return false
	}
	constraint := strings.ToLower(pgError.ConstraintName)
	detail := strings.ToLower(pgError.Detail)
	return constraint == "fk_spus_platform_category" ||
		(strings.Contains(constraint, "spus") && strings.Contains(constraint, "platform_category")) ||
		(strings.Contains(detail, "platform_category_id") && strings.Contains(detail, "spus"))
}

func (r *ProductRepository) UpdateStatusWithTenant(ctx context.Context, tenantID uint, id uint, status models.SPUStatus) error {
	result := r.db.WithContext(ctx).
		Model(&models.SPU{}).
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

func (r *ProductRepository) DeleteWithTenant(ctx context.Context, tenantID uint, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product models.SPU
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ?", tenantID).
			First(&product, id).Error; err != nil {
			return err
		}

		var skus []models.SKU
		if err := tx.Model(&models.SKU{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ?", product.ID).
			Select("id", "erp_sku_code").
			Find(&skus).Error; err != nil {
			return err
		}
		hasOrderItems, err := productHasOrderItemReferences(tx, product.ID, skus)
		if err != nil {
			return err
		}
		if hasOrderItems {
			return ErrProductHasOrderRecords
		}

		if len(skus) > 0 {
			if err := tx.Where("product_id = ?", product.ID).Delete(&models.SKU{}).Error; err != nil {
				if isOrderItemReferenceForeignKeyViolation(err) {
					return ErrProductHasOrderRecords
				}
				return err
			}
		}
		if err := tx.Delete(&product).Error; err != nil {
			if isOrderItemReferenceForeignKeyViolation(err) {
				return ErrProductHasOrderRecords
			}
			return err
		}
		return nil
	})
}

type orderItemReferenceKind string

const (
	orderItemReferenceSKUId      orderItemReferenceKind = "sku_id"
	orderItemReferenceProductID  orderItemReferenceKind = "product_id"
	orderItemReferenceSPUID      orderItemReferenceKind = "spu_id"
	orderItemReferenceSKUCode    orderItemReferenceKind = "sku_code"
	orderItemReferenceERPSKUCode orderItemReferenceKind = "erp_sku_code"
)

func productHasOrderItemReferences(tx *gorm.DB, productID uint, skus []models.SKU) (bool, error) {
	columnTypes, err := tx.Migrator().ColumnTypes(&models.OrderItem{})
	if err != nil {
		return false, err
	}
	columnNames := make([]string, 0, len(columnTypes))
	for _, columnType := range columnTypes {
		columnNames = append(columnNames, columnType.Name())
	}

	skuIDs := make([]uint, 0, len(skus))
	skuCodes := make([]string, 0, len(skus))
	for _, sku := range skus {
		skuIDs = append(skuIDs, sku.ID)
		if code := strings.TrimSpace(sku.ERPSKUCode); code != "" {
			skuCodes = append(skuCodes, code)
		}
	}

	for _, reference := range availableOrderItemReferences(columnNames) {
		filter, values, ok := orderItemReferenceFilter(reference, productID, skuIDs, skuCodes)
		if !ok {
			continue
		}
		var count int64
		if err := tx.Table("order_items").
			Where(filter, values).
			Limit(1).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func availableOrderItemReferences(columnNames []string) []orderItemReferenceKind {
	available := make(map[string]struct{}, len(columnNames))
	for _, name := range columnNames {
		available[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	candidates := []orderItemReferenceKind{
		orderItemReferenceSKUId,
		orderItemReferenceProductID,
		orderItemReferenceSPUID,
		orderItemReferenceSKUCode,
		orderItemReferenceERPSKUCode,
	}
	references := make([]orderItemReferenceKind, 0, len(candidates))
	for _, candidate := range candidates {
		if _, exists := available[string(candidate)]; exists {
			references = append(references, candidate)
		}
	}
	return references
}

func orderItemReferenceFilter(
	reference orderItemReferenceKind,
	productID uint,
	skuIDs []uint,
	skuCodes []string,
) (string, any, bool) {
	switch reference {
	case orderItemReferenceSKUId:
		return "sku_id IN ?", skuIDs, len(skuIDs) > 0
	case orderItemReferenceProductID:
		return "product_id = ?", productID, true
	case orderItemReferenceSPUID:
		return "spu_id = ?", productID, true
	case orderItemReferenceSKUCode:
		return "sku_code IN ?", skuCodes, len(skuCodes) > 0
	case orderItemReferenceERPSKUCode:
		return "erp_sku_code IN ?", skuCodes, len(skuCodes) > 0
	default:
		return "", nil, false
	}
}

func isOrderItemReferenceForeignKeyViolation(err error) bool {
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "23503" {
		return false
	}
	constraint := strings.ToLower(pgError.ConstraintName)
	table := strings.ToLower(pgError.TableName)
	detail := strings.ToLower(pgError.Detail)
	return constraint == "fk_order_items_sku" ||
		strings.Contains(constraint, "order_items") ||
		table == "order_items" ||
		strings.Contains(detail, "order_items")
}

func (r *ProductRepository) adminProductBaseQuery(ctx context.Context, query AdminProductQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Where("tenant_id = ?", query.TenantID)

	if query.CategoryID != nil {
		db = db.Where("category_id = ?", *query.CategoryID)
	}
	if query.PlatformCategoryID != nil {
		db = db.Where("platform_category_id = ?", *query.PlatformCategoryID)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		db = db.Where("name ILIKE ?", "%"+keyword+"%")
	}

	return db
}
