package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHolidayImportDryRunApplyIdempotencyAndCalendar(t *testing.T) {
	db := newHolidayImportTestDB(t)
	manual := models.PublicHoliday{
		CountryCode: "US", HolidayDate: mustHolidayImportDate(t, "2026-08-14"), NameEN: "Manual Store Closure",
		HolidayType: models.HolidayTypeSpecial, DayType: models.HolidayDayTypePublicHoliday, IsOfficial: false,
	}
	ghana := models.PublicHoliday{
		CountryCode: "GH", HolidayDate: mustHolidayImportDate(t, "2026-01-01"), NameEN: "Ghana Fixture",
		HolidayType: models.HolidayTypeFederal, DayType: models.HolidayDayTypePublicHoliday, IsOfficial: true,
		SourceName: stringPointer("Test source"), SourceReference: stringPointer("https://example.test/gh"),
	}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatalf("create manual holiday: %v", err)
	}
	if err := db.Create(&ghana).Error; err != nil {
		t.Fatalf("create Ghana holiday: %v", err)
	}

	data, err := os.ReadFile("../../deploy/holiday-data/us-2026.example.json")
	if err != nil {
		t.Fatalf("read example holiday data: %v", err)
	}
	importer := NewHolidayImportService(db)

	dryRun, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US"})
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if dryRun.Mode != HolidayImportModeDryRun || dryRun.Created != 12 || dryRun.Updated != 0 || dryRun.Unchanged != 0 || dryRun.Rejected != 0 {
		t.Fatalf("dry-run report = %+v", dryRun)
	}
	assertHolidayImportCount(t, db, 2)

	applied, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true})
	if err != nil {
		t.Fatalf("apply import: %v", err)
	}
	if applied.Mode != HolidayImportModeApply || applied.Created != 12 || applied.Updated != 0 || applied.Unchanged != 0 || applied.Rejected != 0 {
		t.Fatalf("apply report = %+v", applied)
	}
	assertHolidayImportCount(t, db, 14)

	repeated, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true})
	if err != nil {
		t.Fatalf("repeat apply import: %v", err)
	}
	if repeated.Created != 0 || repeated.Updated != 0 || repeated.Unchanged != 12 || repeated.Rejected != 0 {
		t.Fatalf("repeat report = %+v", repeated)
	}
	assertHolidayImportCount(t, db, 14)

	var observed models.PublicHoliday
	if err := db.Where("country_code = ? AND holiday_date = ? AND holiday_type = ?", "US", mustHolidayImportDate(t, "2026-07-03"), models.HolidayTypeObserved).Take(&observed).Error; err != nil {
		t.Fatalf("find observed holiday: %v", err)
	}
	if observed.ObservedForDate == nil || observed.ObservedForDate.Format(HolidayCalendarDateLayout) != "2026-07-04" {
		t.Fatalf("observed_for_date = %v", observed.ObservedForDate)
	}

	calendarService := NewPublicHolidayService(
		repository.NewPublicHolidayRepository(db),
		PublicHolidayServiceOptions{Now: func() time.Time { return time.Date(2026, time.July, 2, 23, 30, 0, 0, time.UTC) }},
	)
	calendar, err := calendarService.Calendar(t.Context(), HolidayCalendarQuery{Year: 2026, Month: 7, Locale: "zh"})
	if err != nil {
		t.Fatalf("calendar after import: %v", err)
	}
	if calendar.Timezone != HolidayBusinessTimezone || calendar.Today != "2026-07-02" || len(calendar.Items) != 2 {
		t.Fatalf("calendar response = %+v", calendar)
	}
	if calendar.Items[0].HolidayType != models.HolidayTypeObserved || calendar.Items[0].Name != "美国独立日" || calendar.Items[0].NameEN != "Independence Day" {
		t.Fatalf("localized observed item = %+v", calendar.Items[0])
	}
	if calendar.NextHoliday == nil || calendar.NextHoliday.Date != "2026-07-03" {
		t.Fatalf("next holiday = %+v", calendar.NextHoliday)
	}

	var manualCount, ghanaCount int64
	if err := db.Model(&models.PublicHoliday{}).Where("id = ?", manual.ID).Count(&manualCount).Error; err != nil {
		t.Fatalf("count manual holiday: %v", err)
	}
	if err := db.Model(&models.PublicHoliday{}).Where("id = ? AND country_code = ?", ghana.ID, "GH").Count(&ghanaCount).Error; err != nil {
		t.Fatalf("count Ghana holiday: %v", err)
	}
	if manualCount != 1 || ghanaCount != 1 {
		t.Fatalf("preserved manual/Ghana rows = %d/%d", manualCount, ghanaCount)
	}
}

func TestHolidayImportDetectsUpdatesWithoutDryRunWrites(t *testing.T) {
	db := newHolidayImportTestDB(t)
	data, err := os.ReadFile("../../deploy/holiday-data/us-2026.example.json")
	if err != nil {
		t.Fatalf("read example holiday data: %v", err)
	}
	importer := NewHolidayImportService(db)
	if _, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	if err := db.Model(&models.PublicHoliday{}).
		Where("country_code = ? AND holiday_date = ? AND name_en = ?", "US", mustHolidayImportDate(t, "2026-01-01"), "New Year’s Day").
		Update("name_zh", "旧名称").Error; err != nil {
		t.Fatalf("modify imported row: %v", err)
	}
	if err := db.Where("country_code = ? AND holiday_date = ? AND name_en = ?", "US", mustHolidayImportDate(t, "2026-12-25"), "Christmas Day").
		Delete(&models.PublicHoliday{}).Error; err != nil {
		t.Fatalf("delete row to test atomic conflict: %v", err)
	}

	dryRun, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US"})
	if err != nil {
		t.Fatalf("update dry-run: %v", err)
	}
	if dryRun.Created != 1 || dryRun.Updated != 0 || dryRun.Conflicts != 1 || dryRun.Unchanged != 10 {
		t.Fatalf("update dry-run report = %+v", dryRun)
	}
	if len(dryRun.Changes) == 0 || len(dryRun.Changes[0].ChangedFields) != 1 || dryRun.Changes[0].ChangedFields[0] != "name_zh" {
		t.Fatalf("manual-edit diff fields = %+v", dryRun.Changes)
	}
	var unchanged models.PublicHoliday
	if err := db.Where("country_code = ? AND holiday_date = ? AND name_en = ?", "US", mustHolidayImportDate(t, "2026-01-01"), "New Year’s Day").Take(&unchanged).Error; err != nil {
		t.Fatalf("read dry-run row: %v", err)
	}
	if unchanged.NameZH == nil || *unchanged.NameZH != "旧名称" {
		t.Fatalf("dry-run wrote database: %+v", unchanged)
	}

	applied, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true})
	if !errors.Is(err, ErrHolidayImportConflict) {
		t.Fatalf("protected apply error = %v, want conflict", err)
	}
	if applied.Created != 1 || applied.Updated != 0 || applied.Conflicts != 1 || applied.Unchanged != 10 {
		t.Fatalf("update apply report = %+v", applied)
	}
	if err := db.Where("id = ?", unchanged.ID).Take(&unchanged).Error; err != nil {
		t.Fatalf("read protected row: %v", err)
	}
	if unchanged.NameZH == nil || *unchanged.NameZH != "旧名称" {
		t.Fatalf("default apply overwrote manual edit: %+v", unchanged)
	}
	assertHolidayImportCount(t, db, 11)

	overwritten, err := importer.ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: "US", Apply: true, Overwrite: true})
	if err != nil {
		t.Fatalf("explicit overwrite apply: %v", err)
	}
	if overwritten.Created != 1 || overwritten.Updated != 1 || overwritten.Conflicts != 0 || overwritten.Unchanged != 10 {
		t.Fatalf("overwrite report = %+v", overwritten)
	}
	if err := db.Where("id = ?", unchanged.ID).Take(&unchanged).Error; err != nil || unchanged.NameZH == nil || *unchanged.NameZH != "元旦" {
		t.Fatalf("explicit overwrite did not restore dataset row: %+v/%v", unchanged, err)
	}
	assertHolidayImportCount(t, db, 12)
}

func TestHolidayImportRejectsEntireInvalidBatch(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*holidayImportRecord)
		country  string
		wantText string
	}{
		{
			name: "official missing source", country: "US", wantText: "require source_name",
			mutate: func(record *holidayImportRecord) { record.SourceName = nil },
		},
		{
			name: "country mismatch", country: "US", wantText: "does not match",
			mutate: func(record *holidayImportRecord) { record.CountryCode = "GH" },
		},
		{
			name: "invalid observed date", country: "US", wantText: "observed_for_date",
			mutate: func(record *holidayImportRecord) { record.ObservedForDate = stringPointer("2026-02-30") },
		},
		{
			name: "invalid holiday type", country: "US", wantText: "holiday_type",
			mutate: func(record *holidayImportRecord) { record.HolidayType = "regional" },
		},
		{
			name: "invalid day type", country: "US", wantText: "day_type",
			mutate: func(record *holidayImportRecord) { record.DayType = "weekend_override" },
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := newHolidayImportTestDB(t)
			records := []holidayImportRecord{
				validHolidayImportRecord("2026-03-01", "Valid first row"),
				validHolidayImportRecord("2026-03-02", "Invalid second row"),
			}
			testCase.mutate(&records[1])
			data, err := json.Marshal(records)
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			report, err := NewHolidayImportService(db).ImportJSON(t.Context(), data, HolidayImportOptions{CountryCode: testCase.country, Apply: true})
			if !errors.Is(err, ErrHolidayImportValidation) || !containsHolidayImportText(err.Error(), testCase.wantText) {
				t.Fatalf("ImportJSON error = %v, want validation containing %q", err, testCase.wantText)
			}
			if report.Rejected != 1 || report.Created != 0 || report.Updated != 0 || report.Unchanged != 0 {
				t.Fatalf("rejected report = %+v", report)
			}
			assertHolidayImportCount(t, db, 0)
		})
	}
}

func TestHolidayImportRejectsDuplicateKeysAndUnknownFields(t *testing.T) {
	db := newHolidayImportTestDB(t)
	record := validHolidayImportRecord("2026-04-01", "Duplicate")
	duplicateData, err := json.Marshal([]holidayImportRecord{record, record})
	if err != nil {
		t.Fatalf("marshal duplicate fixture: %v", err)
	}
	if report, err := NewHolidayImportService(db).ImportJSON(t.Context(), duplicateData, HolidayImportOptions{Apply: true}); !errors.Is(err, ErrHolidayImportValidation) || report.Rejected != 1 {
		t.Fatalf("duplicate report/error = %+v/%v", report, err)
	}
	assertHolidayImportCount(t, db, 0)

	unknownField := []byte(`[{"country_code":"US","holiday_date":"2026-04-02","name_en":"Unknown","holiday_type":"federal","day_type":"public_holiday","is_official":false,"unexpected":true}]`)
	if report, err := NewHolidayImportService(db).ImportJSON(t.Context(), unknownField, HolidayImportOptions{Apply: true}); !errors.Is(err, ErrHolidayImportValidation) || report.Rejected != 1 {
		t.Fatalf("unknown-field report/error = %+v/%v", report, err)
	}
	assertHolidayImportCount(t, db, 0)
}

func validHolidayImportRecord(date, name string) holidayImportRecord {
	return holidayImportRecord{
		CountryCode: "US", HolidayDate: date, NameEN: name, NameZH: stringPointer("测试假日"),
		HolidayType: models.HolidayTypeFederal, DayType: models.HolidayDayTypePublicHoliday, IsOfficial: true,
		SourceName: stringPointer("Test source"), SourceReference: stringPointer("https://example.test/source"),
	}
}

func newHolidayImportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:holiday-import-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.PublicHoliday{}); err != nil {
		t.Fatalf("migrate PublicHoliday: %v", err)
	}
	return db
}

func mustHolidayImportDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, ok := parseHolidayCalendarDate(value)
	if !ok {
		t.Fatalf("invalid test date %q", value)
	}
	return parsed
}

func assertHolidayImportCount(t *testing.T, db *gorm.DB, expected int64) {
	t.Helper()
	var count int64
	if err := db.Model(&models.PublicHoliday{}).Count(&count).Error; err != nil {
		t.Fatalf("count holidays: %v", err)
	}
	if count != expected {
		t.Fatalf("holiday count = %d, want %d", count, expected)
	}
}

func containsHolidayImportText(value, expected string) bool {
	return expected == "" || strings.Contains(value, expected)
}
