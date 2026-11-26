package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPublicHolidayDatabaseConstraintsAndUppercaseNormalization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:model-holiday-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&PublicHoliday{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	holiday := &PublicHoliday{
		CountryCode: "us", HolidayDate: time.Date(2031, time.January, 2, 14, 0, 0, 0, time.FixedZone("test", 8*60*60)),
		NameEN: "Constraint Fixture", HolidayType: HolidayTypeFederal, DayType: HolidayDayTypePublicHoliday,
	}
	if err := db.Create(holiday).Error; err != nil {
		t.Fatalf("create valid holiday: %v", err)
	}
	if holiday.CountryCode != "US" || holiday.HolidayDate.Location() != time.UTC || holiday.HolidayDate.Hour() != 0 {
		t.Fatalf("normalized holiday = %+v", holiday)
	}
	defaultCountry := &PublicHoliday{
		HolidayDate: time.Date(2031, time.January, 6, 0, 0, 0, 0, time.UTC),
		NameEN:      "Default Country Fixture", HolidayType: HolidayTypeFederal, DayType: HolidayDayTypePublicHoliday,
	}
	if err := db.Create(defaultCountry).Error; err != nil {
		t.Fatalf("create default-country holiday: %v", err)
	}
	if defaultCountry.CountryCode != "US" {
		t.Fatalf("default country_code = %q, want US", defaultCountry.CountryCode)
	}

	duplicate := *holiday
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("database accepted duplicate country_code + holiday_date + name_en")
	}

	invalidType := &PublicHoliday{
		CountryCode: "US", HolidayDate: time.Date(2031, time.January, 3, 0, 0, 0, 0, time.UTC),
		NameEN: "Invalid Type", HolidayType: "regional", DayType: HolidayDayTypePublicHoliday,
	}
	if err := db.Create(invalidType).Error; err == nil {
		t.Fatal("database accepted a holiday_type outside the allowlist")
	}
	invalidDayType := &PublicHoliday{
		CountryCode: "US", HolidayDate: time.Date(2031, time.January, 4, 0, 0, 0, 0, time.UTC),
		NameEN: "Invalid Day Type", HolidayType: HolidayTypeFederal, DayType: "make_up_workday",
	}
	if err := db.Create(invalidDayType).Error; err == nil {
		t.Fatal("database accepted a day_type outside the allowlist")
	}
	missingOfficialSource := &PublicHoliday{
		CountryCode: "US", HolidayDate: time.Date(2031, time.January, 7, 0, 0, 0, 0, time.UTC),
		NameEN: "Official Without Source", HolidayType: HolidayTypeFederal,
		DayType: HolidayDayTypePublicHoliday, IsOfficial: true,
	}
	if err := db.Create(missingOfficialSource).Error; err == nil {
		t.Fatal("database accepted an official holiday without source fields")
	}
	emptySource := " "
	emptyOfficialSource := &PublicHoliday{
		CountryCode: "US", HolidayDate: time.Date(2031, time.January, 8, 0, 0, 0, 0, time.UTC),
		NameEN: "Official With Empty Source", HolidayType: HolidayTypeFederal,
		DayType: HolidayDayTypePublicHoliday, IsOfficial: true,
		SourceName: &emptySource, SourceReference: &emptySource,
	}
	if err := db.Create(emptyOfficialSource).Error; err == nil {
		t.Fatal("database accepted blank official source fields")
	}

	rawLowercase := db.Exec(
		"INSERT INTO public_holidays (country_code, holiday_date, name_en, holiday_type, day_type, is_official) VALUES (?, ?, ?, ?, ?, ?)",
		"us", "2031-01-05", "Raw Lowercase", HolidayTypeFederal, HolidayDayTypePublicHoliday, true,
	).Error
	if rawLowercase == nil {
		t.Fatal("database accepted a lowercase country_code outside model hooks")
	}
}
