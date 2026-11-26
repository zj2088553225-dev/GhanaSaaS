package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const analyticsPostgresTestDSNEnvironment = "ANALYTICS_POSTGRES_TEST_DSN"

type postgresStorefrontEventPersistenceRepository struct {
	delegate        *repository.StorefrontEventRepository
	createCalls     int
	createManyCalls int
}

func (repo *postgresStorefrontEventPersistenceRepository) Create(ctx context.Context, event *models.StorefrontEvent) error {
	repo.createCalls++
	return repo.delegate.Create(ctx, event)
}

func (repo *postgresStorefrontEventPersistenceRepository) CreateMany(ctx context.Context, events []*models.StorefrontEvent) (int, int, error) {
	repo.createManyCalls++
	return repo.delegate.CreateMany(ctx, events)
}

func (repo *postgresStorefrontEventPersistenceRepository) FindExistingEventIDs(ctx context.Context, eventIDs []string) (map[string]struct{}, error) {
	return repo.delegate.FindExistingEventIDs(ctx, eventIDs)
}

func (*postgresStorefrontEventPersistenceRepository) FindPublicProductTenantID(context.Context, uint) (uint, error) {
	return 0, gorm.ErrRecordNotFound
}

func (*postgresStorefrontEventPersistenceRepository) FindPublicShopTenantID(context.Context, string) (uint, error) {
	return 0, gorm.ErrRecordNotFound
}

func (*postgresStorefrontEventPersistenceRepository) FindPlatformMallTenantID(context.Context) (*uint, error) {
	return nil, nil
}

func TestStorefrontEventPostgresUnicodePersistenceContract(t *testing.T) {
	db := openStorefrontEventPostgresIntegrationDB(t)
	assertStorefrontEventPostgresCharacterColumns(t, db)

	persistence := &postgresStorefrontEventPersistenceRepository{
		delegate: repository.NewStorefrontEventRepository(db),
	}
	tracker := service.NewStorefrontEventService(persistence, service.StorefrontEventOptions{
		Now: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	})

	for _, boundary := range []struct {
		name      string
		character string
	}{
		{name: "three-byte Unicode", character: "界"},
		{name: "four-byte Unicode", character: "😀"},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			input := postgresStorefrontPageViewInput(uuid.NewString())
			input.PagePath = "/" + strings.Repeat(boundary.character, 511)
			referrerPrefix := "https://example.com/"
			input.Referrer = referrerPrefix + strings.Repeat(boundary.character, 1024-utf8.RuneCountInString(referrerPrefix))
			if err := tracker.Track(context.Background(), input); err != nil {
				t.Fatalf("Track maximum Unicode event: %v", err)
			}

			var saved models.StorefrontEvent
			if err := db.Where("event_id = ?", input.EventID).Take(&saved).Error; err != nil {
				t.Fatalf("read maximum Unicode event: %v", err)
			}
			if saved.PagePath != input.PagePath || saved.Referrer != input.Referrer {
				t.Fatalf("stored URL values changed: page=%q referrer=%q", saved.PagePath, saved.Referrer)
			}
			lengths := readStorefrontEventPostgresLengths(t, db, input.EventID)
			if lengths.PageCharacters != 512 || lengths.ReferrerCharacters != 1024 {
				t.Fatalf("character lengths = page %d/referrer %d, want 512/1024", lengths.PageCharacters, lengths.ReferrerCharacters)
			}
			if lengths.PageBytes <= lengths.PageCharacters || lengths.ReferrerBytes <= lengths.ReferrerCharacters {
				t.Fatalf("UTF-8 byte lengths did not exceed character lengths: %+v", lengths)
			}
			t.Logf(
				"%s: page char_length=%d octet_length=%d; referrer char_length=%d octet_length=%d",
				boundary.name,
				lengths.PageCharacters,
				lengths.PageBytes,
				lengths.ReferrerCharacters,
				lengths.ReferrerBytes,
			)
		})
	}

	t.Run("percent-encoded Unicode and reserved path", func(t *testing.T) {
		input := postgresStorefrontPageViewInput(uuid.NewString())
		input.PagePath = "/zh/%E5%95%86%E5%93%81/a%2Fb/%3F/%23/%25?category=123#detail"
		input.Referrer = "https://example.com/%E4%B8%AD%E6%96%87/a%2Fb?utm=x#detail"
		if err := tracker.Track(context.Background(), input); err != nil {
			t.Fatalf("Track encoded Unicode event: %v", err)
		}
		var saved models.StorefrontEvent
		if err := db.Where("event_id = ?", input.EventID).Take(&saved).Error; err != nil {
			t.Fatalf("read encoded Unicode event: %v", err)
		}
		if saved.PagePath != "/zh/商品/a%2Fb/%3F/%23/%25" || saved.Referrer != "https://example.com/中文/a%2Fb" {
			t.Fatalf("stored encoded URL values = %q/%q", saved.PagePath, saved.Referrer)
		}
	})

	t.Run("over-limit values stop before persistence", func(t *testing.T) {
		input := postgresStorefrontPageViewInput(uuid.NewString())
		input.PagePath = "/" + strings.Repeat("界", 512)
		createCalls := persistence.createCalls
		if err := tracker.Track(context.Background(), input); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("513-rune page path error = %v, want ErrInvalidInput", err)
		}
		if persistence.createCalls != createCalls {
			t.Fatal("513-rune page path reached repository Create")
		}

		input = postgresStorefrontPageViewInput(uuid.NewString())
		referrerPrefix := "https://example.com/"
		input.Referrer = referrerPrefix + strings.Repeat("😀", 1025-utf8.RuneCountInString(referrerPrefix))
		if err := tracker.Track(context.Background(), input); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("1025-rune referrer error = %v, want ErrInvalidInput", err)
		}
		if persistence.createCalls != createCalls {
			t.Fatal("1025-rune referrer reached repository Create")
		}
	})

	t.Run("nineteen ASCII and one Unicode batch", func(t *testing.T) {
		inputs := make([]service.StorefrontEventInput, 20)
		for index := range inputs {
			inputs[index] = postgresStorefrontPageViewInput(uuid.NewString())
			inputs[index].PagePath = fmt.Sprintf("/en/page/%d", index)
		}
		inputs[19].PagePath = "/" + strings.Repeat("界", 511)
		referrerPrefix := "https://example.com/"
		inputs[19].Referrer = referrerPrefix + strings.Repeat("界", 1024-utf8.RuneCountInString(referrerPrefix))

		before := countStorefrontEvents(t, db)
		result, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{Events: inputs})
		if err != nil || result.Accepted != 20 || result.Duplicates != 0 {
			t.Fatalf("TrackBatch = %+v/%v, want 20 accepted", result, err)
		}
		if after := countStorefrontEvents(t, db); after-before != 20 {
			t.Fatalf("persisted batch rows = %d, want 20", after-before)
		}
		lengths := readStorefrontEventPostgresLengths(t, db, inputs[19].EventID)
		if lengths.PageCharacters != 512 || lengths.ReferrerCharacters != 1024 {
			t.Fatalf("Unicode batch row lengths = %+v", lengths)
		}
	})

	t.Run("over-limit batch rejects before insert", func(t *testing.T) {
		inputs := make([]service.StorefrontEventInput, 20)
		for index := range inputs {
			inputs[index] = postgresStorefrontPageViewInput(uuid.NewString())
		}
		inputs[19].PagePath = "/" + strings.Repeat("😀", 512)
		beforeRows := countStorefrontEvents(t, db)
		beforeCalls := persistence.createManyCalls
		result, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{Events: inputs})
		if result != (service.StorefrontEventBatchResult{}) || !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("over-limit TrackBatch = %+v/%v, want empty result/ErrInvalidInput", result, err)
		}
		if persistence.createManyCalls != beforeCalls {
			t.Fatal("over-limit batch reached repository CreateMany")
		}
		if afterRows := countStorefrontEvents(t, db); afterRows != beforeRows {
			t.Fatalf("over-limit batch changed row count from %d to %d", beforeRows, afterRows)
		}
	})
}

func TestStorefrontEventPostgresBatchAtomicityAndIdempotency(t *testing.T) {
	db := openStorefrontEventPostgresIntegrationDB(t)
	persistence := &postgresStorefrontEventPersistenceRepository{
		delegate: repository.NewStorefrontEventRepository(db),
	}
	tracker := service.NewStorefrontEventService(persistence, service.StorefrontEventOptions{
		Now: func() time.Time { return time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC) },
	})

	t.Run("row 40 CHECK failure rolls back all 50 rows", func(t *testing.T) {
		if err := db.Exec(`ALTER TABLE storefront_events ADD CONSTRAINT audit_source_check CHECK (source_id IS DISTINCT FROM 'force_check_failure')`).Error; err != nil {
			t.Fatalf("add integration CHECK: %v", err)
		}
		t.Cleanup(func() {
			if err := db.Exec(`ALTER TABLE storefront_events DROP CONSTRAINT IF EXISTS audit_source_check`).Error; err != nil {
				t.Errorf("drop integration CHECK: %v", err)
			}
		})

		inputs := make([]service.StorefrontEventInput, 50)
		for index := range inputs {
			inputs[index] = postgresStorefrontPageViewInput(uuid.NewString())
		}
		inputs[39].SourceID = "force_check_failure"
		before := countStorefrontEvents(t, db)
		result, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{Events: inputs})
		if err == nil || result != (service.StorefrontEventBatchResult{}) {
			t.Fatalf("CHECK-failing TrackBatch = %+v/%v, want empty result/error", result, err)
		}
		if after := countStorefrontEvents(t, db); after != before {
			t.Fatalf("CHECK-failing batch changed rows from %d to %d", before, after)
		}
	})

	t.Run("existing and intra-request duplicates report exact counts", func(t *testing.T) {
		a := postgresStorefrontPageViewInput(uuid.NewString())
		b := postgresStorefrontPageViewInput(uuid.NewString())
		c := postgresStorefrontPageViewInput(uuid.NewString())
		if err := tracker.Track(context.Background(), a); err != nil {
			t.Fatalf("seed existing A: %v", err)
		}
		before := countStorefrontEvents(t, db)
		result, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{
			Events: []service.StorefrontEventInput{a, b, b, c},
		})
		if err != nil || result.Accepted != 2 || result.Duplicates != 2 {
			t.Fatalf("A/B/B/C TrackBatch = %+v/%v, want accepted=2 duplicates=2", result, err)
		}
		if after := countStorefrontEvents(t, db); after-before != 2 {
			t.Fatalf("A/B/B/C added %d rows, want 2", after-before)
		}
	})

	t.Run("duplicate delivery adds only the original 20 rows", func(t *testing.T) {
		inputs := make([]service.StorefrontEventInput, 20)
		for index := range inputs {
			inputs[index] = postgresStorefrontPageViewInput(uuid.NewString())
		}
		before := countStorefrontEvents(t, db)
		first, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{Events: inputs})
		if err != nil || first.Accepted != 20 || first.Duplicates != 0 {
			t.Fatalf("first 20-event delivery = %+v/%v", first, err)
		}
		duplicate, err := tracker.TrackBatch(context.Background(), service.StorefrontEventBatchInput{Events: inputs})
		if err != nil || duplicate.Accepted != 0 || duplicate.Duplicates != 20 {
			t.Fatalf("duplicate 20-event delivery = %+v/%v", duplicate, err)
		}
		if after := countStorefrontEvents(t, db); after-before != 20 {
			t.Fatalf("duplicate delivery added %d rows, want 20", after-before)
		}
	})

	t.Run("concurrent same event ID leaves one row", func(t *testing.T) {
		input := postgresStorefrontPageViewInput(uuid.NewString())
		const workers = 8
		start := make(chan struct{})
		errorsByWorker := make(chan error, workers)
		var wait sync.WaitGroup
		for range workers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				errorsByWorker <- tracker.Track(context.Background(), input)
			}()
		}
		close(start)
		wait.Wait()
		close(errorsByWorker)
		for err := range errorsByWorker {
			if err != nil {
				t.Fatalf("concurrent Track: %v", err)
			}
		}
		var rows int64
		if err := db.Model(&models.StorefrontEvent{}).Where("event_id = ?", input.EventID).Count(&rows).Error; err != nil {
			t.Fatalf("count concurrent event: %v", err)
		}
		if rows != 1 {
			t.Fatalf("concurrent event rows = %d, want 1", rows)
		}
	})
}

func openStorefrontEventPostgresIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(analyticsPostgresTestDSNEnvironment))
	if dsn == "" {
		t.Skipf("%s is not set; PostgreSQL persistence contract not exercised", analyticsPostgresTestDSNEnvironment)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open PostgreSQL integration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL connection pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	schemaName := fmt.Sprintf("analytics_url_contract_%d_%d", os.Getpid(), time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schemaName)).Error; err != nil {
		_ = sqlDB.Close()
		t.Fatalf("create isolated PostgreSQL schema: %v", err)
	}
	if err := db.Exec(fmt.Sprintf(`SET search_path TO "%s"`, schemaName)).Error; err != nil {
		_ = db.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schemaName)).Error
		_ = sqlDB.Close()
		t.Fatalf("select isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`SET search_path TO public`).Error; err != nil {
			t.Errorf("restore PostgreSQL search_path: %v", err)
		}
		if err := db.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schemaName)).Error; err != nil {
			t.Errorf("drop isolated PostgreSQL schema: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close PostgreSQL integration database: %v", err)
		}
	})
	if err := db.AutoMigrate(&models.StorefrontEvent{}, &models.StorefrontEventThrottle{}); err != nil {
		t.Fatalf("migrate fresh StorefrontEvent schema: %v", err)
	}
	return db
}

func assertStorefrontEventPostgresCharacterColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	type columnContract struct {
		ColumnName             string
		CharacterMaximumLength int
	}
	var columns []columnContract
	if err := db.Raw(`
		SELECT column_name, character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'storefront_events'
		  AND column_name IN ('page_path', 'referrer')
		ORDER BY column_name
	`).Scan(&columns).Error; err != nil {
		t.Fatalf("read StorefrontEvent PostgreSQL columns: %v", err)
	}
	want := map[string]int{"page_path": 512, "referrer": 1024}
	if len(columns) != len(want) {
		t.Fatalf("URL character columns = %+v", columns)
	}
	for _, column := range columns {
		if column.CharacterMaximumLength != want[column.ColumnName] {
			t.Fatalf("%s maximum characters = %d, want %d", column.ColumnName, column.CharacterMaximumLength, want[column.ColumnName])
		}
	}
}

type storefrontEventPostgresLengths struct {
	PageCharacters     int
	PageBytes          int
	ReferrerCharacters int
	ReferrerBytes      int
}

func readStorefrontEventPostgresLengths(t *testing.T, db *gorm.DB, eventID string) storefrontEventPostgresLengths {
	t.Helper()
	var result storefrontEventPostgresLengths
	if err := db.Raw(`
		SELECT
			char_length(page_path) AS page_characters,
			octet_length(page_path) AS page_bytes,
			char_length(referrer) AS referrer_characters,
			octet_length(referrer) AS referrer_bytes
		FROM storefront_events
		WHERE event_id = ?
	`, eventID).Scan(&result).Error; err != nil {
		t.Fatalf("read PostgreSQL URL lengths: %v", err)
	}
	return result
}

func postgresStorefrontPageViewInput(eventID string) service.StorefrontEventInput {
	return service.StorefrontEventInput{
		EventID:    eventID,
		EventName:  models.StorefrontEventPageView,
		VisitorID:  "11111111-1111-4111-8111-111111111111",
		SessionID:  "22222222-2222-4222-8222-222222222222",
		EntityType: models.StorefrontEntityStorefront,
		SourceType: models.StorefrontSourceDirect,
		PageType:   models.StorefrontPageOther,
		PagePath:   "/en",
		Locale:     "zh",
		DeviceType: models.StorefrontDeviceDesktop,
	}
}

func countStorefrontEvents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&models.StorefrontEvent{}).Count(&count).Error; err != nil {
		t.Fatalf("count StorefrontEvent rows: %v", err)
	}
	return count
}
