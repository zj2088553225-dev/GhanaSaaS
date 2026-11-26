package analytics

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidDateRange = errors.New("invalid analytics date range")

const analyticsDateLayout = "2006-01-02"

type DateRangeInput struct {
	From     string
	To       string
	Timezone string
}

// DateRange represents inclusive local input dates and an exclusive UTC query
// boundary. OccurredAt is always filtered as [FromUTC, ToUTC).
type DateRange struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Timezone  string         `json:"timezone"`
	FromUTC   time.Time      `json:"-"`
	ToUTC     time.Time      `json:"-"`
	Days      int            `json:"-"`
	Location  *time.Location `json:"-"`
	fromLocal time.Time
}

type TimeWindow struct {
	FromUTC time.Time
	ToUTC   time.Time
}

func (dateRange DateRange) Window() TimeWindow {
	return TimeWindow{FromUTC: dateRange.FromUTC, ToUTC: dateRange.ToUTC}
}

func (dateRange DateRange) PreviousWindow() TimeWindow {
	return TimeWindow{
		FromUTC: dateRange.fromLocal.AddDate(0, 0, -dateRange.Days).UTC(),
		ToUTC:   dateRange.FromUTC,
	}
}

func ParseDateRange(input DateRangeInput, now time.Time, maxDays int) (DateRange, error) {
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if len(timezone) > 64 || strings.Contains(timezone, "..") || strings.HasPrefix(timezone, "/") {
		return DateRange{}, ErrInvalidDateRange
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return DateRange{}, ErrInvalidDateRange
	}

	fromValue := strings.TrimSpace(input.From)
	toValue := strings.TrimSpace(input.To)
	if (fromValue == "") != (toValue == "") {
		return DateRange{}, ErrInvalidDateRange
	}
	if fromValue == "" {
		today := now.In(location)
		toLocal := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
		fromLocal := toLocal.AddDate(0, 0, -(DefaultDateRangeDays - 1))
		return newDateRange(fromLocal, toLocal, timezone, location, maxDays)
	}

	fromLocal, err := time.ParseInLocation(analyticsDateLayout, fromValue, location)
	if err != nil {
		return DateRange{}, ErrInvalidDateRange
	}
	toLocal, err := time.ParseInLocation(analyticsDateLayout, toValue, location)
	if err != nil {
		return DateRange{}, ErrInvalidDateRange
	}
	return newDateRange(fromLocal, toLocal, timezone, location, maxDays)
}

func newDateRange(fromLocal time.Time, toLocal time.Time, timezone string, location *time.Location, maxDays int) (DateRange, error) {
	if fromLocal.After(toLocal) || maxDays <= 0 {
		return DateRange{}, ErrInvalidDateRange
	}
	fromCalendar := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day(), 0, 0, 0, 0, time.UTC)
	toCalendar := time.Date(toLocal.Year(), toLocal.Month(), toLocal.Day(), 0, 0, 0, 0, time.UTC)
	days := int(toCalendar.Sub(fromCalendar)/(24*time.Hour)) + 1
	if days <= 0 || days > maxDays {
		return DateRange{}, ErrInvalidDateRange
	}
	return DateRange{
		From:      fromLocal.Format(analyticsDateLayout),
		To:        toLocal.Format(analyticsDateLayout),
		Timezone:  timezone,
		FromUTC:   fromLocal.UTC(),
		ToUTC:     toLocal.AddDate(0, 0, 1).UTC(),
		Days:      days,
		Location:  location,
		fromLocal: fromLocal,
	}, nil
}
