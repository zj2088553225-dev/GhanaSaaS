package main

import (
	"context"
	"log"
	"os"

	"saas/internal/config"
	"saas/internal/pkg/database"
	"saas/internal/seeder"
)

func main() {
	password := os.Getenv(seeder.SelfOperatedAdminPasswordEnv)
	if password == "" {
		log.Fatalf("%s is required", seeder.SelfOperatedAdminPasswordEnv)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	result, err := seeder.EnsureSelfOperatedTenantAdminWithCredentials(
		context.Background(),
		db,
		os.Getenv(seeder.SelfOperatedAdminPhoneEnv),
		password,
	)
	if err != nil {
		log.Fatalf("reset self operated admin: %v", err)
	}

	log.Printf(
		"self operated admin reset succeeded: tenant_id=%d admin_id=%d password_configured=%t",
		result.TenantID,
		result.AdminID,
		result.PasswordConfigured,
	)
}
