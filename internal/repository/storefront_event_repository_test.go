package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestStorefrontEventRepositoryInsertsEvent(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	repo := NewStorefrontEventRepository(db)
	tenantID := uint(7)
	productID := uint(42)
	entityID := "42"
	sourceID := "all"
	now := time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC)
	event := &models.StorefrontEvent{
		EventName:  models.StorefrontEventProductClick,
		VisitorID:  "11111111-1111-4111-8111-111111111111",
		SessionID:  "22222222-2222-4222-8222-222222222222",
		TenantID:   &tenantID,
		EntityType: models.StorefrontEntityProduct,
		EntityID:   &entityID,
		ProductID:  &productID,
		SourceType: models.StorefrontSourceMall,
		SourceID:   &sourceID,
		PageType:   models.StorefrontPageMall,
		PagePath:   "/en/mall",
		Locale:     "en",
		DeviceType: models.StorefrontDeviceDesktop,
		OccurredAt: now,
		CreatedAt:  now,
	}

	if err := repo.Create(context.Background(), event); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if event.ID == 0 {
		t.Fatal("Create did not assign an ID")
	}

	var saved models.StorefrontEvent
	if err := db.First(&saved, event.ID).Error; err != nil {
		t.Fatalf("read inserted event: %v", err)
	}
	if saved.EventName != models.StorefrontEventProductClick || saved.TenantID == nil || *saved.TenantID != tenantID {
		t.Fatalf("saved event = %+v", saved)
	}
}

func TestStorefrontEventRepositoryCreatesBatchesInOneAtomicOperation(t *testing.T) {
	for _, count := range []int{1, 50} {
		t.Run(fmt.Sprintf("%d_events", count), func(t *testing.T) {
			db := newStorefrontEventRepositoryTestDB(t)
			repo := NewStorefrontEventRepository(db)
			events := make([]*models.StorefrontEvent, count)
			for index := range events {
				events[index] = repositoryTestStorefrontEvent(index)
			}
			accepted, duplicates, err := repo.CreateMany(context.Background(), events)
			if err != nil {
				t.Fatalf("CreateMany returned error: %v", err)
			}
			if accepted != count || duplicates != 0 {
				t.Fatalf("CreateMany result = %d/%d, want %d/0", accepted, duplicates, count)
			}
			var saved int64
			if err := db.Model(&models.StorefrontEvent{}).Count(&saved).Error; err != nil {
				t.Fatalf("count inserted events: %v", err)
			}
			if saved != int64(count) {
				t.Fatalf("saved = %d, want %d", saved, count)
			}
		})
	}
}

func TestStorefrontEventRepositoryTreatsDuplicateEventIDAsAcceptedWithoutDuplicateRow(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	repo := NewStorefrontEventRepository(db)
	first := repositoryTestStorefrontEvent(10)
	first.EventID = "00000000-0000-4000-8000-000000000010"
	duplicate := repositoryTestStorefrontEvent(11)
	duplicate.EventID = first.EventID

	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(context.Background(), duplicate); err != nil {
		t.Fatalf("duplicate Create: %v", err)
	}
	var saved int64
	if err := db.Model(&models.StorefrontEvent{}).Where("event_id = ?", first.EventID).Count(&saved).Error; err != nil {
		t.Fatalf("count duplicate event ID: %v", err)
	}
	if saved != 1 {
		t.Fatalf("saved duplicate event rows = %d, want 1", saved)
	}
}

func TestStorefrontEventRepositoryBatchReportsDuplicates(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	repo := NewStorefrontEventRepository(db)
	events := []*models.StorefrontEvent{
		repositoryTestStorefrontEvent(20),
		repositoryTestStorefrontEvent(21),
		repositoryTestStorefrontEvent(22),
		repositoryTestStorefrontEvent(23),
	}
	events[0].EventID = "00000000-0000-4000-8000-000000000020"
	events[1].EventID = "00000000-0000-4000-8000-000000000021"
	events[2].EventID = events[1].EventID
	events[3].EventID = "00000000-0000-4000-8000-000000000023"

	accepted, duplicates, err := repo.CreateMany(context.Background(), events)
	if err != nil {
		t.Fatalf("CreateMany: %v", err)
	}
	if accepted != 3 || duplicates != 1 {
		t.Fatalf("CreateMany result = %d/%d, want 3/1", accepted, duplicates)
	}
	var saved int64
	if err := db.Model(&models.StorefrontEvent{}).Count(&saved).Error; err != nil {
		t.Fatalf("count batch: %v", err)
	}
	if saved != 3 {
		t.Fatalf("saved batch rows = %d, want 3", saved)
	}
}

func TestStorefrontEventRepositoryConcurrentDuplicateUsesDatabaseUniqueIndex(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	repo := NewStorefrontEventRepository(db)
	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			event := repositoryTestStorefrontEvent(30 + index)
			event.EventID = "00000000-0000-4000-8000-000000000030"
			errorsByWorker <- repo.Create(context.Background(), event)
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent Create: %v", err)
		}
	}
	var saved int64
	if err := db.Model(&models.StorefrontEvent{}).
		Where("event_id = ?", "00000000-0000-4000-8000-000000000030").Count(&saved).Error; err != nil {
		t.Fatalf("count concurrent duplicate: %v", err)
	}
	if saved != 1 {
		t.Fatalf("concurrent duplicate rows = %d, want 1", saved)
	}
}

func TestStorefrontEventRepositoryAtomicallyThrottlesPopularityEvents(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	repo := NewStorefrontEventRepository(db)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	const workers = 12
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			event := repositoryTestStorefrontEvent(100 + index)
			event.EventID = fmt.Sprintf("00000000-0000-4000-8000-%012d", 100+index)
			event.OccurredAt = now
			event.CreatedAt = now
			event.ThrottleKey = strings.Repeat("a", 64)
			event.ThrottleUntil = now.Add(30 * time.Second)
			errs <- repo.Create(context.Background(), event)
		}(index)
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	var saved int64
	if err := db.Model(&models.StorefrontEvent{}).Count(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatalf("concurrent throttled rows = %d, want 1", saved)
	}

	afterWindow := repositoryTestStorefrontEvent(200)
	afterWindow.EventID = "00000000-0000-4000-8000-000000000200"
	afterWindow.OccurredAt = now.Add(30 * time.Second)
	afterWindow.CreatedAt = afterWindow.OccurredAt
	afterWindow.ThrottleKey = strings.Repeat("a", 64)
	afterWindow.ThrottleUntil = afterWindow.OccurredAt.Add(30 * time.Second)
	if err := repo.Create(context.Background(), afterWindow); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.StorefrontEvent{}).Count(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved != 2 {
		t.Fatalf("rows after throttle expiry = %d, want 2", saved)
	}
}

func TestStorefrontEventRepositoryBatchFailureLeavesNoPartialRows(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	repo := NewStorefrontEventRepository(db)
	events := []*models.StorefrontEvent{
		repositoryTestStorefrontEvent(1),
		repositoryTestStorefrontEvent(2),
		repositoryTestStorefrontEvent(3),
	}
	events[1].EventName = models.StorefrontEventName("purchase")
	if _, _, err := repo.CreateMany(context.Background(), events); err == nil {
		t.Fatal("CreateMany accepted a row violating the event-name constraint")
	}
	var saved int64
	if err := db.Model(&models.StorefrontEvent{}).Count(&saved).Error; err != nil {
		t.Fatalf("count rows after failed batch: %v", err)
	}
	if saved != 0 {
		t.Fatalf("failed batch left %d rows, want 0", saved)
	}
}

func TestStorefrontEventRepositoryFailedWriteLogsParameterizedSQL(t *testing.T) {
	var output bytes.Buffer
	databaseLogger := logger.New(log.New(&output, "", 0), logger.Config{LogLevel: logger.Error})
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s-parameterized?mode=memory&cache=shared", t.Name())),
		&gorm.Config{Logger: databaseLogger},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.StorefrontEvent{}, &models.StorefrontEventThrottle{}); err != nil {
		t.Fatalf("AutoMigrate storefront event: %v", err)
	}
	output.Reset()

	event := repositoryTestStorefrontEvent(700)
	event.EventName = models.StorefrontEventName("purchase")
	event.PagePath = "/patient/alice"
	event.Referrer = "https://secret.example/patient/alice"
	if _, _, err := NewStorefrontEventRepository(db).CreateMany(context.Background(), []*models.StorefrontEvent{event}); err == nil {
		t.Fatal("CreateMany accepted a CHECK-violating event")
	}

	logged := output.String()
	for _, secret := range []string{
		event.EventID,
		event.VisitorID,
		event.SessionID,
		event.PagePath,
		event.Referrer,
	} {
		if secret != "" && strings.Contains(logged, secret) {
			t.Fatalf("failed analytics write log contained %q: %s", secret, logged)
		}
	}
	if !strings.Contains(logged, "INSERT INTO") || !strings.Contains(logged, "VALUES") {
		t.Fatalf("parameterized error diagnostic omitted SQL shape: %s", logged)
	}
}

func repositoryTestStorefrontEvent(index int) *models.StorefrontEvent {
	entityID := "42"
	sourceID := "all"
	productID := uint(42)
	visitorID := fmt.Sprintf("%08x-1111-4111-8111-%012x", index+1, index+1)
	now := time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC)
	return &models.StorefrontEvent{
		EventName: models.StorefrontEventProductClick, VisitorID: visitorID,
		SessionID: "22222222-2222-4222-8222-222222222222", EntityType: models.StorefrontEntityProduct,
		EntityID: &entityID, ProductID: &productID, SourceType: models.StorefrontSourceMall,
		SourceID: &sourceID, PageType: models.StorefrontPageMall, PagePath: "/en/mall",
		Locale: "en", DeviceType: models.StorefrontDeviceDesktop, OccurredAt: now, CreatedAt: now,
	}
}

func TestStorefrontEventModelContractAndIndexes(t *testing.T) {
	db := newStorefrontEventRepositoryTestDB(t)
	indexNames := []string{
		"idx_storefront_events_event_id_unique",
		"idx_storefront_events_occurred_at",
		"idx_storefront_events_event_occurred",
		"idx_storefront_events_product_occurred",
		"idx_storefront_events_entity_occurred",
		"idx_storefront_events_tenant_occurred",
		"idx_storefront_events_visitor_occurred",
	}
	for _, indexName := range indexNames {
		if !db.Migrator().HasIndex(&models.StorefrontEvent{}, indexName) {
			t.Errorf("missing index %s", indexName)
		}
	}

	parsed, err := schema.Parse(&models.StorefrontEvent{}, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	fieldSizes := map[string]int{
		"EventID":     36,
		"EventName":   40,
		"VisitorID":   64,
		"SessionID":   64,
		"EntityType":  40,
		"EntityID":    128,
		"SourceType":  40,
		"SourceID":    128,
		"PageType":    24,
		"PagePath":    512,
		"Referrer":    1024,
		"DeviceType":  16,
		"TrafficType": 16,
	}
	for fieldName, expectedSize := range fieldSizes {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Errorf("missing model field %s", fieldName)
			continue
		}
		if field.Size != expectedSize {
			t.Errorf("%s size = %d, want %d", fieldName, field.Size, expectedSize)
		}
	}
	if parsed.LookUpField("ProductID") == nil || !parsed.LookUpField("ProductID").IndirectFieldType.Comparable() {
		t.Fatal("ProductID model contract is missing")
	}
	if !models.StorefrontEventPageView.IsValid() || models.StorefrontEventName("purchase").IsValid() {
		t.Fatal("event name allowlist contract is invalid")
	}
}

func TestStorefrontEventRepositoryFindsOnlyPublicProductTenant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-public?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.ShopProfile{},
		&models.PlatformCategory{},
		&models.SPU{},
	); err != nil {
		t.Fatalf("AutoMigrate public product fixtures: %v", err)
	}

	platform := models.Tenant{Name: models.PlatformMallTenantName, Type: models.TenantTypeSelfOperated, Status: models.TenantStatusActive, AppKey: "platform-events"}
	merchant := models.Tenant{Name: "Merchant", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "merchant-events"}
	category := models.PlatformCategory{Name: "Phones", Path: "Phones", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: true}
	for _, fixture := range []any{&platform, &merchant, &category} {
		if err := db.Create(fixture).Error; err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
	platformProduct := models.SPU{TenantID: platform.ID, PlatformCategoryID: category.ID, Name: "Platform Product", Status: models.SPUStatusOnShelf}
	privateMerchantProduct := models.SPU{TenantID: merchant.ID, PlatformCategoryID: category.ID, Name: "Private Product", Status: models.SPUStatusOnShelf}
	if err := db.Create(&platformProduct).Error; err != nil {
		t.Fatalf("create platform product: %v", err)
	}
	if err := db.Create(&privateMerchantProduct).Error; err != nil {
		t.Fatalf("create merchant product: %v", err)
	}

	repo := NewStorefrontEventRepository(db, PublicProductScopeOptions{PlatformMallTenantID: platform.ID})
	if tenantID, err := repo.FindPublicProductTenantID(context.Background(), platformProduct.ID); err != nil || tenantID != platform.ID {
		t.Fatalf("platform product tenant = %d, err = %v", tenantID, err)
	}
	if _, err := repo.FindPublicProductTenantID(context.Background(), privateMerchantProduct.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("private merchant product error = %v, want record not found", err)
	}

	profile := models.ShopProfile{TenantID: merchant.ID, Slug: "merchant", ShopName: "Merchant", IsPublic: true}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create public shop: %v", err)
	}
	if tenantID, err := repo.FindPublicProductTenantID(context.Background(), privateMerchantProduct.ID); err != nil || tenantID != merchant.ID {
		t.Fatalf("public merchant product tenant = %d, err = %v", tenantID, err)
	}
	if err := db.Model(&privateMerchantProduct).Update("status", models.SPUStatusOffShelf).Error; err != nil {
		t.Fatalf("take product off shelf: %v", err)
	}
	if _, err := repo.FindPublicProductTenantID(context.Background(), privateMerchantProduct.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("off-shelf product error = %v, want record not found", err)
	}
}

func newStorefrontEventRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.StorefrontEvent{}, &models.StorefrontEventThrottle{}); err != nil {
		t.Fatalf("AutoMigrate storefront event: %v", err)
	}
	return db
}
