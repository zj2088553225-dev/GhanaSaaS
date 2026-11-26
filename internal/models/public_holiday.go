package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type HolidayType string

const (
	HolidayTypeFederal  HolidayType = "federal"
	HolidayTypeObserved HolidayType = "observed"
	HolidayTypeSpecial  HolidayType = "special"
)

type HolidayDayType string

const (
	HolidayDayTypePublicHoliday      HolidayDayType = "public_holiday"
	HolidayDayTypeWorkingDayOverride HolidayDayType = "working_day_override"
)

// PublicHoliday is global calendar data. It intentionally has no tenant or user
// foreign key: the same verified calendar is served to every storefront.
type PublicHoliday struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	CountryCode     string         `gorm:"type:char(2);not null;default:US;uniqueIndex:uidx_public_holidays_identity,priority:1;index;check:chk_public_holidays_country_upper,country_code = UPPER(country_code)" json:"country_code"`
	HolidayDate     time.Time      `gorm:"type:date;not null;uniqueIndex:uidx_public_holidays_identity,priority:2;index" json:"holiday_date"`
	NameEN          string         `gorm:"type:varchar(160);not null;uniqueIndex:uidx_public_holidays_identity,priority:3" json:"name_en"`
	NameZH          *string        `gorm:"type:varchar(160)" json:"name_zh"`
	HolidayType     HolidayType    `gorm:"type:varchar(16);not null;check:chk_public_holidays_holiday_type,holiday_type IN ('federal','observed','special')" json:"holiday_type"`
	DayType         HolidayDayType `gorm:"type:varchar(32);not null;default:public_holiday;check:chk_public_holidays_day_type,day_type IN ('public_holiday','working_day_override')" json:"day_type"`
	IsOfficial      bool           `gorm:"not null;default:false;index;check:chk_public_holidays_official_source,NOT is_official OR (source_name IS NOT NULL AND length(trim(source_name)) > 0 AND source_reference IS NOT NULL AND length(trim(source_reference)) > 0)" json:"is_official"`
	SourceName      *string        `gorm:"type:varchar(200)" json:"source_name"`
	SourceReference *string        `gorm:"type:varchar(2048)" json:"source_reference"`
	ObservedForDate *time.Time     `gorm:"type:date;index" json:"observed_for_date"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (holiday *PublicHoliday) BeforeSave(_ *gorm.DB) error {
	holiday.CountryCode = strings.ToUpper(strings.TrimSpace(holiday.CountryCode))
	holiday.HolidayDate = calendarDateOnly(holiday.HolidayDate)
	if holiday.ObservedForDate != nil {
		value := calendarDateOnly(*holiday.ObservedForDate)
		holiday.ObservedForDate = &value
	}
	return nil
}

func calendarDateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
