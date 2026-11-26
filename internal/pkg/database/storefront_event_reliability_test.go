package database

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyStorefrontEvent struct {
	ID        uint   `gorm:"primaryKey"`
	EventName string `gorm:"size:40;not null"`
}

func (legacyStorefrontEvent) TableName() string { return "storefront_events" }

func TestPrepareStorefrontEventReliabilityBackfillsHistoricalRowsInBatches(t *testing.T) {
	db := newStorefrontEventReliabilityTestDB(t)
	rows := make([]legacyStorefrontEvent, storefrontEventBackfillBatchSize+1)
	for index := range rows {
		rows[index].EventName = string(models.StorefrontEventPageView)
	}
	if err := db.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatalf("create legacy rows: %v", err)
	}

	if err := PrepareStorefrontEventReliability(db); err != nil {
		t.Fatalf("PrepareStorefrontEventReliability: %v", err)
	}
	var saved []struct {
		EventID     string
		TrafficType models.StorefrontTrafficType
	}
	if err := db.Table("storefront_events").Order("id ASC").Scan(&saved).Error; err != nil {
		t.Fatalf("read migrated rows: %v", err)
	}
	if len(saved) != len(rows) {
		t.Fatalf("migrated rows = %d, want %d", len(saved), len(rows))
	}
	seen := make(map[string]struct{}, len(saved))
	for index, row := range saved {
		if row.EventID == "" || row.TrafficType != models.StorefrontTrafficHuman {
			t.Fatalf("row %d reliability values = %+v", index, row)
		}
		if _, duplicate := seen[row.EventID]; duplicate {
			t.Fatalf("duplicate generated event_id %q", row.EventID)
		}
		seen[row.EventID] = struct{}{}
	}
	if !db.Migrator().HasIndex(&models.StorefrontEvent{}, StorefrontEventIDUniqueIndexName) {
		t.Fatalf("missing unique index %s", StorefrontEventIDUniqueIndexName)
	}
	if err := PreflightStorefrontEventReliability(db); err != nil {
		t.Fatalf("post-migration preflight: %v", err)
	}
}

func TestStorefrontEventReliabilityPreflightReportsDuplicateAndInvalidRows(t *testing.T) {
	db := newStorefrontEventReliabilityTestDB(t)
	if err := db.Exec("ALTER TABLE storefront_events ADD COLUMN event_id varchar(36)").Error; err != nil {
		t.Fatalf("add event_id: %v", err)
	}
	if err := db.Exec("ALTER TABLE storefront_events ADD COLUMN traffic_type varchar(16)").Error; err != nil {
		t.Fatalf("add traffic_type: %v", err)
	}
	duplicateID := "00000000-0000-4000-8000-000000000001"
	fixtures := []map[string]any{
		{"event_name": "page_view", "event_id": duplicateID, "traffic_type": "human"},
		{"event_name": "page_view", "event_id": duplicateID, "traffic_type": "robot"},
		{"event_name": "page_view", "event_id": nil, "traffic_type": "human"},
	}
	for _, fixture := range fixtures {
		if err := db.Table("storefront_events").Create(fixture).Error; err != nil {
			t.Fatalf("create preflight fixture: %v", err)
		}
	}

	err := PreflightStorefrontEventReliability(db)
	var violations *StorefrontEventReliabilityPreflightError
	if !errors.As(err, &violations) {
		t.Fatalf("preflight error = %v, want StorefrontEventReliabilityPreflightError", err)
	}
	if violations.MissingEventIDCount != 1 || violations.DuplicateEventIDCount != 1 ||
		violations.InvalidTrafficTypeCount != 1 || len(violations.DuplicateEventIDSamples) != 1 {
		t.Fatalf("preflight violations = %+v", violations)
	}
}

func TestStorefrontEventReliabilityPostgreSQLMigrationContract(t *testing.T) {
	source, err := os.ReadFile("storefront_event_reliability.go")
	if err != nil {
		t.Fatalf("read migration source: %v", err)
	}
	normalized := strings.ToLower(string(source))
	steps := []string{
		"add column event_id varchar(36)",
		"backfillstorefronteventids",
		"alter column event_id set not null",
		"create unique index if not exists",
	}
	lastPosition := -1
	for _, step := range steps {
		position := strings.Index(normalized, step)
		if position < 0 || position <= lastPosition {
			t.Fatalf("migration step %q missing or out of order", step)
		}
		lastPosition = position
	}
	file, err := parser.ParseFile(token.NewFileSet(), "storefront_event_reliability.go", source, 0)
	if err != nil {
		t.Fatalf("parse migration source: %v", err)
	}
	var executableStrings strings.Builder
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, unquoteErr := strconv.Unquote(literal.Value)
		if unquoteErr == nil {
			executableStrings.WriteString(strings.ToLower(value))
		}
		return true
	})
	for _, forbidden := range []string{"pgcrypto", "uuid-ossp", "gen_random_uuid", "uuid_generate_v4"} {
		if strings.Contains(executableStrings.String(), forbidden) {
			t.Fatalf("migration SQL unexpectedly assumes PostgreSQL UUID extension %q", forbidden)
		}
	}
	if !strings.Contains(normalized, fmt.Sprintf("=%d", storefrontEventBackfillBatchSize)) &&
		!strings.Contains(normalized, "storefronteventbackfillbatchsize") {
		t.Fatal("migration does not use a bounded backfill batch")
	}
}

func newStorefrontEventReliabilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&legacyStorefrontEvent{}); err != nil {
		t.Fatalf("AutoMigrate legacy storefront event: %v", err)
	}
	return db
}
