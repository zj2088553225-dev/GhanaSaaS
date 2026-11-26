package database

import (
	"io"
	"log"
	"os"
	"time"

	"saas/internal/config"
	"saas/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := Connect(cfg)
	if err != nil {
		return nil, err
	}

	if err := migrateLegacySKUColumns(db); err != nil {
		return nil, err
	}
	if err := migrateProductCategoryColumns(db); err != nil {
		return nil, err
	}
	if err := migrateCustomerOrderColumns(db); err != nil {
		return nil, err
	}
	if err := PreflightProductInvariants(db); err != nil {
		return nil, err
	}
	if err := PrepareStorefrontEventReliability(db); err != nil {
		return nil, err
	}
	if err := prepareAnonymousReservationSchema(db); err != nil {
		return nil, err
	}
	if err := prepareCustomerEmailAuthSchema(db); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.OpenAPIRequestNonce{},
		&models.ShopProfile{},
		&models.User{},
		&models.Customer{},
		&models.PasswordResetToken{},
		&models.PickupStation{},
		&models.Category{},
		&models.PlatformCategory{},
		&models.PlatformCategorySearchIndex{},
		&models.SPU{},
		&models.SKU{},
		&models.Order{},
		&models.OrderItem{},
		&models.SitePage{},
		&models.StorefrontTrackingSetting{},
		&models.StorefrontEvent{},
		&models.StorefrontEventThrottle{},
		&models.UploadAsset{},
		&models.MerchantLead{},
		&models.ContentTranslation{},
		&models.PublicHoliday{},
		&models.ReservationProduct{},
		&models.ReservationInterest{},
		&models.ProductReservation{},
		&models.ReservationAttemptEvent{},
	); err != nil {
		return nil, err
	}
	if err := EnsurePlatformCategorySearchInfrastructure(db); err != nil {
		return nil, err
	}
	if err := BackfillSitePageTrackingKeys(db); err != nil {
		return nil, err
	}
	if err := EnsureProductInvariants(db); err != nil {
		return nil, err
	}
	if err := EnsureStorefrontEventReliability(db); err != nil {
		return nil, err
	}

	return db, nil
}

// Connect opens and verifies the configured database without running migrations.
// Maintenance commands use it so read-only modes cannot change schema or data.
func Connect(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{Logger: newParameterizedGORMLogger(os.Stderr)})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func newParameterizedGORMLogger(output io.Writer) logger.Interface {
	return logger.New(log.New(output, "gorm ", log.LstdFlags), logger.Config{
		SlowThreshold:             time.Second,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	})
}

func migrateLegacySKUColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable("skus") {
		return nil
	}

	hasSPUID, err := hasColumn(db, "skus", "spu_id")
	if err != nil {
		return err
	}
	hasProductID, err := hasColumn(db, "skus", "product_id")
	if err != nil {
		return err
	}
	switch {
	case hasSPUID && !hasProductID:
		if err := db.Exec("ALTER TABLE skus RENAME COLUMN spu_id TO product_id").Error; err != nil {
			return err
		}
	case hasSPUID && hasProductID:
		if err := db.Exec("UPDATE skus SET product_id = spu_id WHERE product_id IS NULL AND spu_id IS NOT NULL").Error; err != nil {
			return err
		}
	}

	hasLegacyERPCode, err := hasColumn(db, "skus", "erpsku_code")
	if err != nil {
		return err
	}
	hasERPCode, err := hasColumn(db, "skus", "erp_sku_code")
	if err != nil {
		return err
	}
	switch {
	case hasLegacyERPCode && !hasERPCode:
		if err := db.Exec("ALTER TABLE skus RENAME COLUMN erpsku_code TO erp_sku_code").Error; err != nil {
			return err
		}
	case hasLegacyERPCode && hasERPCode:
		if err := db.Exec("UPDATE skus SET erp_sku_code = erpsku_code WHERE (erp_sku_code IS NULL OR erp_sku_code = '') AND erpsku_code <> ''").Error; err != nil {
			return err
		}
	}

	return nil
}

func migrateProductCategoryColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable("spus") {
		return nil
	}
	hasCategoryID, err := hasColumn(db, "spus", "category_id")
	if err != nil {
		return err
	}
	if hasCategoryID {
		if err := db.Exec("ALTER TABLE spus ALTER COLUMN category_id DROP NOT NULL").Error; err != nil {
			return err
		}
	}

	hasPlatformCategoryID, err := hasColumn(db, "spus", "platform_category_id")
	if err != nil {
		return err
	}
	if !hasPlatformCategoryID {
		return db.Exec("ALTER TABLE spus ADD COLUMN platform_category_id bigint").Error
	}
	return nil
}

func migrateCustomerOrderColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable("orders") {
		return nil
	}
	hasUserID, err := hasColumn(db, "orders", "user_id")
	if err != nil {
		return err
	}
	if hasUserID {
		if err := db.Exec("ALTER TABLE orders ALTER COLUMN user_id DROP NOT NULL").Error; err != nil {
			return err
		}
	}
	return nil
}

// prepareCustomerEmailAuthSchema upgrades legacy phone-first customer rows
// without deleting accounts. Placeholder addresses use the reserved .invalid
// TLD and cannot receive email or be used for password reset delivery.
func prepareCustomerEmailAuthSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable("customers") {
		return nil
	}

	hasEmail, err := hasColumn(db, "customers", "email")
	if err != nil {
		return err
	}
	if !hasEmail {
		if err := db.Exec("ALTER TABLE customers ADD COLUMN email varchar(255)").Error; err != nil {
			return err
		}
	}

	if err := db.Exec(`
		WITH ranked AS (
			SELECT id,
				ROW_NUMBER() OVER (
					PARTITION BY LOWER(BTRIM(email))
					ORDER BY id
				) AS duplicate_rank
			FROM customers
			WHERE NULLIF(BTRIM(email), '') IS NOT NULL
		)
		UPDATE customers AS customer
		SET email = 'legacy-customer-' || customer.id || '-' ||
			SUBSTRING(MD5(RANDOM()::text), 1, 12) || '@users.invalid'
		FROM ranked
		WHERE customer.id = ranked.id
			AND ranked.duplicate_rank > 1
	`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
		UPDATE customers
		SET email = 'legacy-customer-' || id || '-' ||
			SUBSTRING(MD5(RANDOM()::text), 1, 12) || '@users.invalid'
		WHERE email IS NULL OR BTRIM(email) = ''
	`).Error; err != nil {
		return err
	}
	if err := db.Exec("UPDATE customers SET email = LOWER(BTRIM(email)) WHERE email <> LOWER(BTRIM(email))").Error; err != nil {
		return err
	}
	return db.Exec("ALTER TABLE customers ALTER COLUMN email SET NOT NULL").Error
}

func hasColumn(db *gorm.DB, tableName string, columnName string) (bool, error) {
	var exists bool
	err := db.Raw(
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
				AND table_name = ?
				AND column_name = ?
		)`,
		tableName,
		columnName,
	).Scan(&exists).Error
	return exists, err
}
