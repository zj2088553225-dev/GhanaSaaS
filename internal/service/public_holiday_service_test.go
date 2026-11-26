package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPublicHolidayCalendarUsesAccraDefaultsAndKeepsWeekendAndObservedRows(t *testing.T) {
	db := newPublicHolidayTestDB(t)
	service := NewPublicHolidayService(
		repository.NewPublicHolidayRepository(db),
		PublicHolidayServiceOptions{Now: func() time.Time {
			return time.Date(2026, time.July, 16, 8, 30, 0, 0, time.UTC)
		}},
	)

	observedFor := "2026-07-18"
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		HolidayDate: "2026-07-17", NameEN: "Weekend Holiday", NameZH: stringPointer("周末节日"),
		HolidayType: models.HolidayTypeObserved, IsOfficial: true, ObservedForDate: &observedFor,
	})
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		CountryCode: "us", HolidayDate: "2026-07-18", NameEN: "Weekend Holiday", NameZH: stringPointer("周末节日"),
		HolidayType: models.HolidayTypeFederal, IsOfficial: true,
	})
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		HolidayDate: "2026-07-20", NameEN: "Special Closure", NameZH: stringPointer("特别休假"),
		HolidayType: models.HolidayTypeSpecial, IsOfficial: true,
	})
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		HolidayDate: "2026-07-21", NameEN: "Unverified Draft", HolidayType: models.HolidayTypeFederal,
		IsOfficial: false,
	})
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		CountryCode: "GH", HolidayDate: "2026-07-22", NameEN: "Ghana Fixture",
		HolidayType: models.HolidayTypeFederal, IsOfficial: true,
	})

	calendar, err := service.Calendar(t.Context(), HolidayCalendarQuery{})
	if err != nil {
		t.Fatalf("Calendar returned error: %v", err)
	}
	if calendar.CountryCode != "US" || calendar.CountryName != "United States" {
		t.Fatalf("country = %s/%s, want US/United States", calendar.CountryCode, calendar.CountryName)
	}
	if calendar.Timezone != HolidayBusinessTimezone || calendar.Today != "2026-07-16" {
		t.Fatalf("timezone/today = %s/%s", calendar.Timezone, calendar.Today)
	}
	if calendar.Year != 2026 || calendar.Month != 7 {
		t.Fatalf("default year/month = %d/%d", calendar.Year, calendar.Month)
	}
	if len(calendar.Items) != 3 {
		t.Fatalf("official US items = %d, want 3: %+v", len(calendar.Items), calendar.Items)
	}
	if calendar.Items[0].HolidayType != models.HolidayTypeObserved || calendar.Items[0].ObservedForDate == nil || *calendar.Items[0].ObservedForDate != observedFor {
		t.Fatalf("observed item = %+v", calendar.Items[0])
	}
	if calendar.Items[1].HolidayType != models.HolidayTypeFederal || calendar.Items[1].Date != observedFor {
		t.Fatalf("weekend federal item was overwritten: %+v", calendar.Items[1])
	}
	if calendar.Items[2].HolidayType != models.HolidayTypeSpecial {
		t.Fatalf("special item = %+v", calendar.Items[2])
	}
	if calendar.NextHoliday == nil || calendar.NextHoliday.Date != "2026-07-17" || calendar.NextHoliday.HolidayType != models.HolidayTypeObserved {
		t.Fatalf("next holiday = %+v", calendar.NextHoliday)
	}

	zhCalendar, err := service.Calendar(t.Context(), HolidayCalendarQuery{CountryCode: "US", Year: 2026, Month: 7, Locale: "zh"})
	if err != nil {
		t.Fatalf("Chinese Calendar returned error: %v", err)
	}
	if zhCalendar.Items[0].Name != "周末节日" || zhCalendar.Items[0].NameEN != "Weekend Holiday" {
		t.Fatalf("Chinese localized item = %+v", zhCalendar.Items[0])
	}

	var overrideCount int64
	if err := db.Model(&models.PublicHoliday{}).
		Where("day_type = ?", models.HolidayDayTypeWorkingDayOverride).
		Count(&overrideCount).Error; err != nil {
		t.Fatalf("count working-day overrides: %v", err)
	}
	if overrideCount != 0 {
		t.Fatalf("working-day overrides = %d, want no automatically generated rows", overrideCount)
	}
}

func TestPublicHolidayCalendarMonthAndYearBoundary(t *testing.T) {
	db := newPublicHolidayTestDB(t)
	service := NewPublicHolidayService(
		repository.NewPublicHolidayRepository(db),
		PublicHolidayServiceOptions{Now: func() time.Time {
			return time.Date(2026, time.December, 31, 23, 30, 0, 0, time.UTC)
		}},
	)
	createPublicHolidayFixture(t, service, PublicHolidayInput{
		HolidayDate: "2027-01-01", NameEN: "Boundary Holiday",
		HolidayType: models.HolidayTypeFederal, IsOfficial: true,
	})

	december, err := service.Calendar(t.Context(), HolidayCalendarQuery{})
	if err != nil {
		t.Fatalf("default Calendar returned error: %v", err)
	}
	if december.Year != 2026 || december.Month != 12 || december.Today != "2026-12-31" || len(december.Items) != 0 {
		t.Fatalf("December response = %+v", december)
	}
	if december.NextHoliday == nil || december.NextHoliday.Date != "2027-01-01" {
		t.Fatalf("year-boundary next holiday = %+v", december.NextHoliday)
	}

	january, err := service.Calendar(t.Context(), HolidayCalendarQuery{Year: 2027, Month: 1})
	if err != nil {
		t.Fatalf("January Calendar returned error: %v", err)
	}
	if len(january.Items) != 1 || january.Items[0].Date != "2027-01-01" {
		t.Fatalf("January items = %+v", january.Items)
	}
}

func TestPublicHolidaySuperCRUDAndAllowlists(t *testing.T) {
	db := newPublicHolidayTestDB(t)
	service := NewPublicHolidayService(repository.NewPublicHolidayRepository(db))

	created, err := service.Create(t.Context(), PublicHolidayInput{
		CountryCode: "us", HolidayDate: "2030-04-10", NameEN: "Admin Maintained Holiday",
		HolidayType: models.HolidayTypeSpecial, DayType: models.HolidayDayTypePublicHoliday,
		IsOfficial: true,
		SourceName: stringPointer("Admin source"), SourceReference: stringPointer("https://example.test/admin-holiday"),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.ID == 0 || created.CountryCode != "US" {
		t.Fatalf("created holiday = %+v", created)
	}

	_, err = service.Create(t.Context(), PublicHolidayInput{
		HolidayDate: "2030-04-10", NameEN: "Admin Maintained Holiday",
		HolidayType: models.HolidayTypeSpecial, IsOfficial: true,
		SourceName: stringPointer("Admin source"), SourceReference: stringPointer("https://example.test/admin-holiday"),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate Create error = %v, want conflict", err)
	}
	_, err = service.Create(t.Context(), PublicHolidayInput{
		HolidayDate: "2030-04-11", NameEN: "Invalid Type", HolidayType: "regional", IsOfficial: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid holiday type error = %v", err)
	}
	_, err = service.Create(t.Context(), PublicHolidayInput{
		HolidayDate: "2030-04-11", NameEN: "Invalid Day Type", DayType: "make_up_workday", IsOfficial: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid day type error = %v", err)
	}

	updated, err := service.Update(t.Context(), created.ID, PublicHolidayInput{
		HolidayDate: "2030-04-12", NameEN: "Corrected Holiday", NameZH: stringPointer("修正假日"),
		HolidayType: models.HolidayTypeFederal, IsOfficial: true,
		SourceName: stringPointer("Corrected source"), SourceReference: stringPointer("https://example.test/corrected-holiday"),
	})
	if err != nil || updated.HolidayDate != "2030-04-12" || updated.NameZH == nil {
		t.Fatalf("Update result/error = %+v/%v", updated, err)
	}
	listed, err := service.List(t.Context(), PublicHolidayAdminQuery{CountryCode: "US", Year: 2030, Month: 4})
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("List result/error = %+v/%v", listed, err)
	}
	if err := service.Delete(t.Context(), created.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := service.FindByID(t.Context(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByID after delete error = %v", err)
	}
	if _, err := service.Create(t.Context(), PublicHolidayInput{
		HolidayDate: "2030-04-13", NameEN: "Official Without Source",
		HolidayType: models.HolidayTypeFederal, IsOfficial: true,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("official holiday without source error = %v", err)
	}
}

func newPublicHolidayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:holiday-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.PublicHoliday{}); err != nil {
		t.Fatalf("migrate PublicHoliday: %v", err)
	}
	return db
}

func createPublicHolidayFixture(t *testing.T, service *PublicHolidayService, input PublicHolidayInput) PublicHolidayAdminDTO {
	t.Helper()
	if input.IsOfficial {
		if input.SourceName == nil {
			input.SourceName = stringPointer("Fixture source")
		}
		if input.SourceReference == nil {
			input.SourceReference = stringPointer("https://example.test/fixture")
		}
	}
	item, err := service.Create(t.Context(), input)
	if err != nil {
		t.Fatalf("create holiday fixture %+v: %v", input, err)
	}
	return *item
}

func stringPointer(value string) *string {
	return &value
}
