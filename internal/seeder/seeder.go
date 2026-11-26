package seeder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"saas/internal/config"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/pkg/credentials"

	"gorm.io/gorm"
)

const (
	InitialSuperAdminPhoneEnv     = "INITIAL_SUPER_ADMIN_PHONE"
	InitialSuperAdminPasswordEnv  = "INITIAL_SUPER_ADMIN_PASSWORD"
	InitialTenantAdminPhoneEnv    = "INITIAL_TENANT_ADMIN_PHONE"
	InitialTenantAdminPasswordEnv = "INITIAL_TENANT_ADMIN_PASSWORD"
	DefaultCategoryName           = "Default Category"
	DemoTenantName                = "Demo Merchant"
)

var knownDefaultAdminPasswords = map[string]struct{}{
	"123456":       {},
	"admin":        {},
	"admin123":     {},
	"admin@123456": {},
	"password":     {},
	"password123":  {},
}

type initialAdminCredentials struct {
	SuperAdminPhone     string
	SuperAdminPassword  string
	TenantAdminPhone    string
	TenantAdminPassword string
}

type Result struct {
	Seeded                              bool
	PlatformTenantID                    uint
	DemoTenantID                        uint
	DefaultCategoryID                   uint
	SelfOperatedAdminID                 uint
	SelfOperatedAdminPhone              string
	SelfOperatedAdminCreated            bool
	SelfOperatedAdminPasswordConfigured bool
	SuperAdminPhone                     string
	TenantAdminPhone                    string
}

func Run(ctx context.Context, db *gorm.DB) (*Result, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}

	var superAdminCount int64
	if err := db.WithContext(ctx).
		Model(&models.User{}).
		Where("role = ?", models.UserRoleSuperAdmin).
		Count(&superAdminCount).Error; err != nil {
		return nil, err
	}

	var initialCredentials initialAdminCredentials
	if superAdminCount == 0 {
		var err error
		initialCredentials, err = loadInitialAdminCredentials(os.Getenv, os.Getenv("APP_ENV"))
		if err != nil {
			return nil, err
		}
	}

	if err := ensureTenantCredentials(ctx, db); err != nil {
		return nil, err
	}
	if err := ensurePlatformCategories(ctx, db); err != nil {
		return nil, err
	}

	if superAdminCount > 0 {
		if err := db.WithContext(ctx).
			Model(&models.User{}).
			Where("role = ?", models.UserRoleSuperAdmin).
			Update("tenant_id", nil).Error; err != nil {
			return nil, err
		}

		selfAdmin, err := EnsureSelfOperatedTenantAdmin(ctx, db)
		if err != nil {
			return nil, err
		}
		if _, err := ensureDefaultCategory(ctx, db, selfAdmin.TenantID); err != nil {
			return nil, err
		}
		if err := ensureDemoTenantDefaultCategory(ctx, db); err != nil {
			return nil, err
		}
		return resultFromSelfOperatedAdmin(&Result{Seeded: false}, selfAdmin), nil
	}

	result := &Result{
		Seeded:           true,
		SuperAdminPhone:  initialCredentials.SuperAdminPhone,
		TenantAdminPhone: initialCredentials.TenantAdminPhone,
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		platformTenant := models.Tenant{
			Name:   "Platform Self Operated",
			Type:   models.TenantTypeSelfOperated,
			Status: models.TenantStatusActive,
		}
		if err := assignTenantCredentials(&platformTenant); err != nil {
			return err
		}
		if err := tx.Create(&platformTenant).Error; err != nil {
			return err
		}
		result.PlatformTenantID = platformTenant.ID

		demoTenant := models.Tenant{
			Name:   DemoTenantName,
			Type:   models.TenantTypeThirdParty,
			Status: models.TenantStatusActive,
		}
		if err := assignTenantCredentials(&demoTenant); err != nil {
			return err
		}
		if err := tx.Create(&demoTenant).Error; err != nil {
			return err
		}
		result.DemoTenantID = demoTenant.ID

		if _, err := ensureDefaultCategory(ctx, tx, platformTenant.ID); err != nil {
			return err
		}
		defaultCategory, err := ensureDefaultCategory(ctx, tx, demoTenant.ID)
		if err != nil {
			return err
		}
		result.DefaultCategoryID = defaultCategory.ID

		superAdminPasswordHash, err := auth.HashPassword(initialCredentials.SuperAdminPassword)
		if err != nil {
			return err
		}
		superAdmin := models.User{
			Role:         models.UserRoleSuperAdmin,
			Phone:        initialCredentials.SuperAdminPhone,
			PasswordHash: superAdminPasswordHash,
			Status:       models.UserStatusActive,
		}
		if err := tx.Create(&superAdmin).Error; err != nil {
			return err
		}

		tenantAdminPasswordHash, err := auth.HashPassword(initialCredentials.TenantAdminPassword)
		if err != nil {
			return err
		}
		tenantAdminTenantID := demoTenant.ID
		tenantAdmin := models.User{
			TenantID:     &tenantAdminTenantID,
			Role:         models.UserRoleTenantAdmin,
			Phone:        initialCredentials.TenantAdminPhone,
			PasswordHash: tenantAdminPasswordHash,
			Status:       models.UserStatusActive,
		}
		if err := tx.Create(&tenantAdmin).Error; err != nil {
			return err
		}

		selfAdmin, err := EnsureSelfOperatedTenantAdmin(ctx, tx)
		if err != nil {
			return err
		}
		resultFromSelfOperatedAdmin(result, selfAdmin)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func loadInitialAdminCredentials(getenv func(string) string, appEnv string) (initialAdminCredentials, error) {
	credentials := initialAdminCredentials{
		SuperAdminPhone:     strings.TrimSpace(getenv(InitialSuperAdminPhoneEnv)),
		SuperAdminPassword:  getenv(InitialSuperAdminPasswordEnv),
		TenantAdminPhone:    strings.TrimSpace(getenv(InitialTenantAdminPhoneEnv)),
		TenantAdminPassword: getenv(InitialTenantAdminPasswordEnv),
	}
	missing := make([]string, 0, 4)
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: InitialSuperAdminPhoneEnv, value: credentials.SuperAdminPhone},
		{name: InitialSuperAdminPasswordEnv, value: credentials.SuperAdminPassword},
		{name: InitialTenantAdminPhoneEnv, value: credentials.TenantAdminPhone},
		{name: InitialTenantAdminPasswordEnv, value: credentials.TenantAdminPassword},
	} {
		if strings.TrimSpace(item.value) == "" {
			missing = append(missing, item.name)
		}
	}
	if len(missing) > 0 {
		return initialAdminCredentials{}, fmt.Errorf("initial administrator credentials are required: set %s", strings.Join(missing, ", "))
	}
	for name, password := range map[string]string{
		InitialSuperAdminPasswordEnv:  credentials.SuperAdminPassword,
		InitialTenantAdminPasswordEnv: credentials.TenantAdminPassword,
	} {
		if _, rejected := knownDefaultAdminPasswords[strings.ToLower(strings.TrimSpace(password))]; rejected {
			return initialAdminCredentials{}, fmt.Errorf("%s uses a known default password", name)
		}
		if config.IsProductionEnvironment(appEnv) && len([]byte(password)) < 12 {
			return initialAdminCredentials{}, fmt.Errorf("%s must be at least 12 bytes in production", name)
		}
	}
	return credentials, nil
}

func ensureDemoTenantDefaultCategory(ctx context.Context, db *gorm.DB) error {
	var demoTenant models.Tenant
	err := db.WithContext(ctx).
		Where("name = ?", DemoTenantName).
		Order("id ASC").
		First(&demoTenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = ensureDefaultCategory(ctx, db, demoTenant.ID)
	return err
}

func ensurePlatformCategories(ctx context.Context, db *gorm.DB) error {
	trees := []platformCategorySeed{
		{
			Name: "食品饮料",
			Children: []platformCategorySeed{
				{
					Name: "饮料",
					Children: []platformCategorySeed{
						{Name: "碳酸饮料"},
						{Name: "果汁饮料"},
					},
				},
				{Name: "零食"},
				{Name: "调味品"},
			},
		},
		{
			Name: "日用百货",
			Children: []platformCategorySeed{
				{Name: "清洁用品"},
				{Name: "厨房用品"},
			},
		},
	}

	for index, item := range trees {
		if _, err := ensurePlatformCategory(ctx, db, nil, item, index+1); err != nil {
			return err
		}
	}
	return nil
}

type platformCategorySeed struct {
	Name     string
	Children []platformCategorySeed
}

func ensurePlatformCategory(
	ctx context.Context,
	db *gorm.DB,
	parent *models.PlatformCategory,
	seed platformCategorySeed,
	sortOrder int,
) (*models.PlatformCategory, error) {
	name := strings.TrimSpace(seed.Name)
	if name == "" {
		return nil, errors.New("platform category name is required")
	}
	var parentID *uint
	path := name
	level := 1
	if parent != nil {
		parentID = &parent.ID
		path = parent.Path + "/" + name
		level = parent.Level + 1
	}

	var category models.PlatformCategory
	query := db.WithContext(ctx).Where("LOWER(name) = LOWER(?)", name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.Order("id ASC").First(&category).Error
	if err == nil {
		updates := map[string]any{}
		if category.Path != path {
			updates["path"] = path
			category.Path = path
		}
		if category.Level != level {
			updates["level"] = level
			category.Level = level
		}
		if category.SortOrder != sortOrder {
			updates["sort_order"] = sortOrder
			category.SortOrder = sortOrder
		}
		if category.Status == "" {
			updates["status"] = models.PlatformCategoryStatusActive
			category.Status = models.PlatformCategoryStatusActive
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(&models.PlatformCategory{}).
				Where("id = ?", category.ID).
				Updates(updates).Error; err != nil {
				return nil, err
			}
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		category = models.PlatformCategory{
			ParentID:  parentID,
			Name:      name,
			Path:      path,
			Level:     level,
			SortOrder: sortOrder,
			Status:    models.PlatformCategoryStatusActive,
			IsLeaf:    true,
		}
		if err := db.WithContext(ctx).Create(&category).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	for index, child := range seed.Children {
		if _, err := ensurePlatformCategory(ctx, db, &category, child, index+1); err != nil {
			return nil, err
		}
	}
	isLeaf := len(seed.Children) == 0
	if category.IsLeaf != isLeaf {
		if err := db.WithContext(ctx).Model(&models.PlatformCategory{}).
			Where("id = ?", category.ID).
			Update("is_leaf", isLeaf).Error; err != nil {
			return nil, err
		}
		category.IsLeaf = isLeaf
	}
	return &category, nil
}

func ensureDefaultCategory(ctx context.Context, db *gorm.DB, tenantID uint) (*models.Category, error) {
	if tenantID == 0 {
		return nil, errors.New("tenant id is required for default category")
	}

	var category models.Category
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND LOWER(name) = LOWER(?)", tenantID, DefaultCategoryName).
		Order("id ASC").
		First(&category).Error
	if err == nil {
		updates := map[string]any{}
		if !category.IsActive {
			category.IsActive = true
			updates["is_active"] = true
		}
		if category.Sort == 0 {
			category.Sort = 1
			updates["sort"] = 1
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).
				Model(&models.Category{}).
				Where("id = ? AND tenant_id = ?", category.ID, tenantID).
				Updates(updates).Error; err != nil {
				return nil, err
			}
		}
		return &category, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	category = models.Category{
		TenantID: tenantID,
		Name:     DefaultCategoryName,
		Sort:     1,
		IsActive: true,
	}
	if err := db.WithContext(ctx).Create(&category).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			if findErr := db.WithContext(ctx).
				Where("tenant_id = ? AND LOWER(name) = LOWER(?)", tenantID, DefaultCategoryName).
				Order("id ASC").
				First(&category).Error; findErr == nil {
				return &category, nil
			}
		}
		return nil, err
	}
	return &category, nil
}

func resultFromSelfOperatedAdmin(result *Result, selfAdmin *SelfOperatedAdminResult) *Result {
	if result == nil || selfAdmin == nil {
		return result
	}
	result.PlatformTenantID = selfAdmin.TenantID
	result.SelfOperatedAdminID = selfAdmin.AdminID
	result.SelfOperatedAdminPhone = selfAdmin.AdminPhone
	result.SelfOperatedAdminCreated = selfAdmin.Created
	result.SelfOperatedAdminPasswordConfigured = selfAdmin.PasswordConfigured
	return result
}

func ensureTenantCredentials(ctx context.Context, db *gorm.DB) error {
	var tenants []models.Tenant
	if err := db.WithContext(ctx).
		Where("app_key = '' OR app_secret = '' OR app_key IS NULL OR app_secret IS NULL").
		Find(&tenants).Error; err != nil {
		return err
	}

	for i := range tenants {
		tenant := tenants[i]
		if err := assignTenantCredentials(&tenant); err != nil {
			return err
		}
		if err := db.WithContext(ctx).
			Model(&models.Tenant{}).
			Where("id = ?", tenant.ID).
			Updates(map[string]any{
				"app_key":    tenant.AppKey,
				"app_secret": tenant.AppSecret,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func assignTenantCredentials(tenant *models.Tenant) error {
	if tenant.AppKey == "" {
		appKey, err := credentials.GenerateAppKey()
		if err != nil {
			return err
		}
		tenant.AppKey = appKey
	}
	if tenant.AppSecret == "" {
		appSecret, err := credentials.GenerateAppSecret()
		if err != nil {
			return err
		}
		tenant.AppSecret = appSecret
	}
	return nil
}
