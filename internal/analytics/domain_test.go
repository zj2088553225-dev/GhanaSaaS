package analytics

import (
	"errors"
	"testing"
	"time"
)

func TestCoreMetricRatesAndChangeSemantics(t *testing.T) {
	counts := EventCounts{ProductImpressions: 200, ProductClicks: 25, ProductViews: 10, ContactIntents: 2}
	rates := counts.Rates()
	if rates.ProductCTR != 0.125 || rates.DetailViewRate != 0.4 || rates.ContactIntentRate != 0.2 {
		t.Fatalf("rates = %+v", rates)
	}
	if Ratio(1, 0) != 0 {
		t.Fatal("zero denominator did not return zero")
	}
	if ChangeRate(10, 0) != nil {
		t.Fatal("positive current against zero previous must return nil")
	}
	zeroChange := ChangeRate(0, 0)
	if zeroChange == nil || *zeroChange != 0 {
		t.Fatalf("zero-to-zero change = %v, want 0", zeroChange)
	}
}

func TestParseDateRangeUsesInclusiveLocalDatesAndUTCExclusiveEnd(t *testing.T) {
	dateRange, err := ParseDateRange(DateRangeInput{
		From:     "2026-07-01",
		To:       "2026-07-14",
		Timezone: "Asia/Shanghai",
	}, time.Time{}, MaxStandardRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	if got, want := dateRange.FromUTC.Format(time.RFC3339), "2026-06-30T16:00:00Z"; got != want {
		t.Fatalf("FromUTC = %s, want %s", got, want)
	}
	if got, want := dateRange.ToUTC.Format(time.RFC3339), "2026-07-14T16:00:00Z"; got != want {
		t.Fatalf("ToUTC = %s, want %s", got, want)
	}
	if dateRange.Days != 14 {
		t.Fatalf("Days = %d, want 14", dateRange.Days)
	}
}

func TestParseDateRangeDefaultsToSevenAccraCalendarDays(t *testing.T) {
	now := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)
	dateRange, err := ParseDateRange(DateRangeInput{}, now, MaxStandardRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	if dateRange.From != "2026-07-08" || dateRange.To != "2026-07-14" || dateRange.Timezone != DefaultTimezone {
		t.Fatalf("default range = %+v", dateRange)
	}
	previous := dateRange.PreviousWindow()
	if previous.FromUTC.Format("2006-01-02") != "2026-07-01" || previous.ToUTC.Format("2006-01-02") != "2026-07-08" {
		t.Fatalf("previous window = %+v", previous)
	}
}

func TestParseDateRangeRejectsInvalidInputs(t *testing.T) {
	tests := []DateRangeInput{
		{From: "2026-07-14", To: "2026-07-01"},
		{From: "2026-07-01"},
		{From: "2026-01-01", To: "2027-01-02"},
		{From: "2026-07-01", To: "2026-07-14", Timezone: "Mars/Olympus"},
	}
	for _, input := range tests {
		if _, err := ParseDateRange(input, time.Now(), MaxStandardRangeDays); !errors.Is(err, ErrInvalidDateRange) {
			t.Errorf("ParseDateRange(%+v) error = %v", input, err)
		}
	}
}

func TestGranularityLimits(t *testing.T) {
	if got, ok := ResolveGranularity("", 2); !ok || got != GranularityHour {
		t.Fatalf("two-day default = %q/%v", got, ok)
	}
	if got, ok := ResolveGranularity("", 3); !ok || got != GranularityDay {
		t.Fatalf("three-day default = %q/%v", got, ok)
	}
	if _, ok := ResolveGranularity("hour", MaxHourlyRangeDays+1); ok {
		t.Fatal("hour granularity accepted more than 31 days")
	}
	if _, ok := ResolveGranularity("month", 30); ok {
		t.Fatal("unsupported month granularity accepted")
	}
}

func TestScopesAndSourceNormalization(t *testing.T) {
	if !AllScope().IsValid() || !TenantScope(7).IsValid() || TenantScope(0).IsValid() {
		t.Fatal("analytics scope validation is incorrect")
	}
	if NormalizeSource("mall") != "mall" || NormalizeSource("unbounded-new-source") != "other" {
		t.Fatal("source normalization is incorrect")
	}
}
