package main

import (
	"context"
	"log"

	"saas/internal/config"
	"saas/internal/pkg/database"
	"saas/internal/seeder"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	result, err := seeder.Run(context.Background(), db)
	if err != nil {
		log.Fatalf("seed database: %v", err)
	}
	if !result.Seeded {
		log.Printf(
			"seed skipped: super_admin already exists; self_operated_tenant_id=%d self_operated_admin_id=%d password_configured=%t",
			result.PlatformTenantID,
			result.SelfOperatedAdminID,
			result.SelfOperatedAdminPasswordConfigured,
		)
		if !result.SelfOperatedAdminPasswordConfigured {
			log.Printf("self operated admin login password was not configured; set %s and run cmd/reset_self_operated_admin when direct login is needed", seeder.SelfOperatedAdminPasswordEnv)
		}
		return
	}

	log.Printf(
		"seed completed: platform_tenant_id=%d demo_tenant_id=%d default_category_id=%d self_operated_admin_id=%d self_operated_admin_password_configured=%t",
		result.PlatformTenantID,
		result.DemoTenantID,
		result.DefaultCategoryID,
		result.SelfOperatedAdminID,
		result.SelfOperatedAdminPasswordConfigured,
	)
	if !result.SelfOperatedAdminPasswordConfigured {
		log.Printf("self operated admin login password was not configured; set %s and run cmd/reset_self_operated_admin when direct login is needed", seeder.SelfOperatedAdminPasswordEnv)
	}
}
