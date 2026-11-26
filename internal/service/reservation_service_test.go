package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var reservationTestNow = time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)

func TestReservationProductCRUDVisibilityLocalizationAndSanitization(t *testing.T) {
	db := newReservationServiceTestDB(t)
	linked := &models.SPU{
		TenantID: 1, PlatformCategoryID: 1, Name: "Linked preview product", Status: models.SPUStatusOffShelf,
	}
	if err := db.Create(linked).Error; err != nil {
		t.Fatalf("create linked SPU: %v", err)
	}
	service := newReservationTestService(db)
	start := reservationTestNow.Add(-time.Hour)
	end := reservationTestNow.Add(24 * time.Hour)
	linkedID := linked.ID
	created, err := service.CreateProduct(t.Context(), ReservationProductInput{
		LinkedProductID: &linkedID, Slug: "smart-kettle", TitleEN: "Smart Kettle", TitleZH: stringPointer("智能水壶"),
		SummaryEN: "A preview appliance", SummaryZH: stringPointer("新品家电"),
		DescriptionEN: `<p>Safe details</p><script>alert(1)</script>`,
		DescriptionZH: stringPointer(`<p>安全详情</p><img src="javascript:alert(1)">`),
		MainImageURL:  "/uploads/images/kettle.webp", Gallery: []string{"/uploads/images/kettle.webp", "https://images.example.test/kettle-2.webp"},
		Status: models.ReservationProductStatusOpen, ReservationStartAt: &start, ReservationEndAt: &end,
		DisplayOrder: 4, IsFeatured: true, CreatedBy: 7,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if created.ID == 0 || created.LinkedProductID == nil || *created.LinkedProductID != linked.ID {
		t.Fatalf("created product = %+v", created)
	}
	if strings.Contains(created.DescriptionEN, "script") || strings.Contains(created.DescriptionEN, "alert") || strings.Contains(*created.DescriptionZH, "javascript") {
		t.Fatalf("descriptions were not sanitized: %q / %q", created.DescriptionEN, *created.DescriptionZH)
	}
	if len(created.Gallery) != 2 || created.ExactInterestCount != 0 || created.ExactVerifiedCount != 0 {
		t.Fatalf("created gallery/counts = %+v", created)
	}

	if _, err := service.CreateProduct(t.Context(), ReservationProductInput{Slug: "smart-kettle", TitleEN: "Duplicate", CreatedBy: 7}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slug error = %v", err)
	}
	missingLinked := uint(99999)
	if _, err := service.CreateProduct(t.Context(), ReservationProductInput{LinkedProductID: &missingLinked, Slug: "missing-link", TitleEN: "Missing", CreatedBy: 7}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing linked product error = %v", err)
	}
	if _, err := service.CreateProduct(t.Context(), ReservationProductInput{Slug: "bad-image", TitleEN: "Bad", MainImageURL: "javascript:alert(1)", CreatedBy: 7}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe image error = %v", err)
	}

	publicPage, err := service.PublicProducts(t.Context(), ReservationPublicQuery{Page: 1, PageSize: 20, Locale: "zh", Featured: boolPointer(true)})
	if err != nil {
		t.Fatalf("PublicProducts: %v", err)
	}
	if publicPage.Total != 1 || len(publicPage.Items) != 1 || publicPage.Items[0].Title != "智能水壶" || publicPage.Items[0].Summary != "新品家电" {
		t.Fatalf("localized public page = %+v", publicPage)
	}
	if publicPage.Items[0].HeatLevel != models.ReservationHeatNew {
		t.Fatalf("initial public heat = %q", publicPage.Items[0].HeatLevel)
	}
	publicJSON, err := json.Marshal(publicPage.Items[0])
	if err != nil {
		t.Fatalf("marshal public reservation DTO: %v", err)
	}
	if strings.Contains(string(publicJSON), "linked_product_id") || strings.Contains(string(publicJSON), "created_by") {
		t.Fatalf("public DTO leaked internal product linkage: %s", publicJSON)
	}

	future := reservationTestNow.Add(time.Hour)
	if _, err := service.CreateProduct(t.Context(), ReservationProductInput{
		Slug: "future-open", TitleEN: "Future", Status: models.ReservationProductStatusOpen, ReservationStartAt: &future, CreatedBy: 7,
	}); err != nil {
		t.Fatalf("create future product: %v", err)
	}
	if _, err := service.CreateProduct(t.Context(), ReservationProductInput{
		Slug: "scheduled-no-time", TitleEN: "Scheduled", Status: models.ReservationProductStatusScheduled, CreatedBy: 7,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("scheduled without time error = %v", err)
	}
	publicPage, err = service.PublicProducts(t.Context(), ReservationPublicQuery{})
	if err != nil || publicPage.Total != 1 {
		t.Fatalf("time-window public page = %+v/%v", publicPage, err)
	}

	updated, err := service.UpdateProduct(t.Context(), created.ID, ReservationProductInput{
		Slug: "smart-kettle", TitleEN: "Smart Kettle 2", Status: models.ReservationProductStatusClosed,
		MainImageURL: "/uploads/images/kettle.webp", Gallery: []string{}, CreatedBy: 999,
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.Status != models.ReservationProductStatusClosed || updated.CreatedBy != 7 {
		t.Fatalf("updated product = %+v", updated)
	}
	if _, err := service.PublicProduct(t.Context(), "smart-kettle", "en", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("closed public detail error = %v", err)
	}
	if err := service.ArchiveProduct(t.Context(), created.ID); err != nil {
		t.Fatalf("ArchiveProduct: %v", err)
	}
	archived, err := service.AdminProduct(t.Context(), created.ID)
	if err != nil || archived.Status != models.ReservationProductStatusArchived {
		t.Fatalf("archived product = %+v/%v", archived, err)
	}
	if err := service.ArchiveProduct(t.Context(), created.ID); err != nil {
		t.Fatalf("repeat archive must be idempotent: %v", err)
	}
}

func TestReservationHeatWeightsVerifiedAndInterestSignals(t *testing.T) {
	thresholds := ReservationHeatThresholds{PopularMin: 5, TrendingMin: 20, HighlyRequestedMin: 50}
	if got := reservationAutomaticHeatLevel(4, 5, thresholds); got != models.ReservationHeatPopular {
		t.Fatalf("weighted heat = %q, want popular", got)
	}
	if got := reservationAutomaticHeatLevel(0, 24, thresholds); got != models.ReservationHeatNew {
		t.Fatalf("interest-only heat below threshold = %q", got)
	}
	if got := reservationAutomaticHeatLevel(0, 25, thresholds); got != models.ReservationHeatPopular {
		t.Fatalf("interest-only threshold heat = %q", got)
	}
}

func TestReservationInterestNewDuplicateIdempotencyValidationAndViewerState(t *testing.T) {
	db := newReservationServiceTestDB(t)
	service := newReservationTestService(db)
	product := createOpenReservationProduct(t, service, "interest-product")
	input := ReservationInterestInput{
		VisitorID: "11111111-1111-4111-8111-111111111111", SessionID: "22222222-2222-4222-8222-222222222222",
		IdempotencyKey: "33333333-3333-4333-8333-333333333333", SourceType: models.ReservationSourceList,
	}
	created, err := service.RecordInterest(t.Context(), product.ID, input)
	if err != nil || !created.Interested || !created.Created {
		t.Fatalf("first interest = %+v/%v", created, err)
	}
	repeated, err := service.RecordInterest(t.Context(), product.ID, input)
	if err != nil || !repeated.Interested || repeated.Created {
		t.Fatalf("idempotent interest = %+v/%v", repeated, err)
	}
	newKey := input
	newKey.IdempotencyKey = "44444444-4444-4444-8444-444444444444"
	repeated, err = service.RecordInterest(t.Context(), product.ID, newKey)
	if err != nil || repeated.Created {
		t.Fatalf("visitor duplicate interest = %+v/%v", repeated, err)
	}
	mismatched := input
	mismatched.VisitorID = "55555555-5555-4555-8555-555555555555"
	if _, err := service.RecordInterest(t.Context(), product.ID, mismatched); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused idempotency key error = %v", err)
	}
	invalid := input
	invalid.VisitorID = "not-a-uuid"
	if _, err := service.RecordInterest(t.Context(), product.ID, invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid visitor error = %v", err)
	}
	invalid = input
	invalid.SourceType = "product_detail"
	if _, err := service.RecordInterest(t.Context(), product.ID, invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid source error = %v", err)
	}

	detail, err := service.PublicProduct(t.Context(), product.Slug, "en", input.VisitorID)
	if err != nil || !detail.ViewerState.Interested || detail.ViewerState.VerifiedReserved {
		t.Fatalf("viewer state = %+v/%v", detail, err)
	}
	admin, err := service.AdminProduct(t.Context(), product.ID)
	if err != nil || admin.ExactInterestCount != 1 || admin.ExactVerifiedCount != 0 {
		t.Fatalf("admin exact counts = %+v/%v", admin, err)
	}
	analytics, err := service.ProductAnalytics(t.Context(), product.ID)
	if err != nil || analytics.UniqueInterests != 1 || analytics.VerifiedReservations != 0 || analytics.PageViews != 0 {
		t.Fatalf("analytics = %+v/%v", analytics, err)
	}

	updated, err := service.UpdateProduct(t.Context(), product.ID, ReservationProductInput{
		Slug: product.Slug, TitleEN: product.TitleEN, Status: models.ReservationProductStatusClosed, CreatedBy: product.CreatedBy,
	})
	if err != nil || updated.Status != models.ReservationProductStatusClosed {
		t.Fatalf("close product = %+v/%v", updated, err)
	}
	closedInput := input
	closedInput.VisitorID = "66666666-6666-4666-8666-666666666666"
	closedInput.IdempotencyKey = "77777777-7777-4777-8777-777777777777"
	if _, err := service.RecordInterest(t.Context(), product.ID, closedInput); !errors.Is(err, ErrReservationProductUnavailable) {
		t.Fatalf("closed product interest error = %v", err)
	}
}

func TestReservationInterestConcurrentDuplicateUsesDatabaseConstraint(t *testing.T) {
	db := newReservationServiceTestDB(t)
	service := newReservationTestService(db)
	product := createOpenReservationProduct(t, service, "concurrent-product")
	const workers = 16
	start := make(chan struct{})
	var createdCount atomic.Int64
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			result, err := service.RecordInterest(t.Context(), product.ID, ReservationInterestInput{
				VisitorID:      "88888888-8888-4888-8888-888888888888",
				IdempotencyKey: fmt.Sprintf("%08d-9999-4999-8999-%012d", index+1, index+1),
				SourceType:     models.ReservationSourceDetail,
			})
			if err != nil {
				errorsFound <- err
				return
			}
			if result.Created {
				createdCount.Add(1)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent interest returned error: %v", err)
	}
	if createdCount.Load() != 1 {
		t.Fatalf("created responses = %d, want 1", createdCount.Load())
	}
	var count int64
	if err := db.Model(&models.ReservationInterest{}).Where("reservation_product_id = ?", product.ID).Count(&count).Error; err != nil {
		t.Fatalf("count interests: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted interests = %d, want 1", count)
	}
}

func TestReservationInterestSchemaHasNoRawIPColumn(t *testing.T) {
	db := newReservationServiceTestDB(t)
	columns, err := db.Migrator().ColumnTypes(&models.ReservationInterest{})
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}
	for _, column := range columns {
		name := strings.ToLower(column.Name())
		if name == "ip" || name == "ip_address" || name == "raw_ip" {
			t.Fatalf("ReservationInterest persists raw IP column %q", name)
		}
	}
}

func newReservationServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reservation-service-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.SPU{}, &models.ReservationProduct{}, &models.ReservationInterest{},
		&models.ProductReservation{}, &models.ReservationAttemptEvent{},
	); err != nil {
		t.Fatalf("migrate reservation models: %v", err)
	}
	return db
}

func newReservationTestService(db *gorm.DB) *ReservationService {
	return NewReservationService(
		repository.NewReservationProductRepository(db),
		ReservationServiceOptions{Now: func() time.Time { return reservationTestNow }},
	)
}

func createOpenReservationProduct(t *testing.T, service *ReservationService, slug string) *ReservationProductAdminDTO {
	t.Helper()
	product, err := service.CreateProduct(t.Context(), ReservationProductInput{
		Slug: slug, TitleEN: "Reservation product", Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	})
	if err != nil {
		t.Fatalf("create open reservation product: %v", err)
	}
	return product
}

func boolPointer(value bool) *bool { return &value }
