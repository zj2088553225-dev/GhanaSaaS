package service

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHolidayImportPostgresAtomicManualEditProtection(t *testing.T) {
	if os.Getenv("RESERVATION_TEST_ALLOW_ISOLATED") != "1" {
		t.Skip("set RESERVATION_TEST_ALLOW_ISOLATED=1 with an isolated reservation_test PostgreSQL database")
	}
	dsn := strings.TrimSpace(os.Getenv("RESERVATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Fatal("RESERVATION_TEST_POSTGRES_DSN is required")
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	var databaseName string
	if err := adminDB.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil || databaseName != "reservation_test" {
		t.Fatalf("refusing non-isolated database %q: %v", databaseName, err)
	}
	schema := "holiday_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schema + `"`
	if err := adminDB.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create holiday schema: %v", err)
	}
	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error })
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open holiday schema: %v", err)
	}
	if err := db.AutoMigrate(&models.PublicHoliday{}); err != nil {
		t.Fatalf("migrate PublicHoliday: %v", err)
	}

	data, err := os.ReadFile("../../deploy/holiday-data/us-2026.example.json")
	if err != nil {
		t.Fatalf("read holiday dataset: %v", err)
	}
	importer := NewHolidayImportService(db)
	if report, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true}); err != nil || report.Created != 12 {
		t.Fatalf("initial PostgreSQL apply = %+v/%v", report, err)
	}
	if err := db.Model(&models.PublicHoliday{}).
		Where("country_code = ? AND holiday_date = ? AND name_en = ?", "US", "2026-01-01", "New Year’s Day").
		Update("name_zh", "人工修正").Error; err != nil {
		t.Fatalf("manual holiday edit: %v", err)
	}
	protected, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true})
	if !errors.Is(err, ErrHolidayImportConflict) || protected.Conflicts != 1 || protected.Updated != 0 {
		t.Fatalf("protected PostgreSQL apply = %+v/%v", protected, err)
	}
	var protectedRow models.PublicHoliday
	if err := db.Where("country_code = ? AND holiday_date = ? AND name_en = ?", "US", "2026-01-01", "New Year’s Day").Take(&protectedRow).Error; err != nil || protectedRow.NameZH == nil || *protectedRow.NameZH != "人工修正" {
		t.Fatalf("manual edit was overwritten = %+v/%v", protectedRow, err)
	}

	records := []holidayImportRecord{
		validHolidayImportRecord("2026-03-01", "Atomic valid row"),
		validHolidayImportRecord("2026-03-02", "Atomic invalid row"),
	}
	records[1].SourceName = nil
	invalidData, _ := json.Marshal(records)
	if _, err := importer.ImportJSON(t.Context(), invalidData, HolidayImportOptions{CountryCode: "US", Apply: true}); !errors.Is(err, ErrHolidayImportValidation) {
		t.Fatalf("invalid PostgreSQL batch error = %v", err)
	}
	var invalidBatchRows int64
	if err := db.Model(&models.PublicHoliday{}).Where("name_en IN ?", []string{"Atomic valid row", "Atomic invalid row"}).Count(&invalidBatchRows).Error; err != nil || invalidBatchRows != 0 {
		t.Fatalf("invalid PostgreSQL batch wrote %d rows: %v", invalidBatchRows, err)
	}

	if report, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true, Overwrite: true}); err != nil || report.Updated != 1 {
		t.Fatalf("explicit PostgreSQL overwrite = %+v/%v", report, err)
	}
	calendar, err := NewPublicHolidayService(
		repository.NewPublicHolidayRepository(db),
		PublicHolidayServiceOptions{Now: func() time.Time { return time.Date(2026, time.July, 2, 23, 30, 0, 0, time.UTC) }},
	).Calendar(t.Context(), HolidayCalendarQuery{CountryCode: "US", Year: 2026, Month: 7, Locale: "zh"})
	if err != nil || calendar.Timezone != HolidayBusinessTimezone || len(calendar.Items) != 2 || calendar.Items[0].HolidayType != models.HolidayTypeObserved {
		t.Fatalf("PostgreSQL holiday calendar = %+v/%v", calendar, err)
	}
	empty := ""
	if err := db.Create(&models.PublicHoliday{
		CountryCode: "US", HolidayDate: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		NameEN: "Missing official source", HolidayType: models.HolidayTypeFederal,
		DayType: models.HolidayDayTypePublicHoliday, IsOfficial: true, SourceName: &empty, SourceReference: &empty,
	}).Error; err == nil {
		t.Fatal("PostgreSQL accepted blank official source")
	}
}
