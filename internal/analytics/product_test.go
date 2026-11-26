package analytics

import "testing"

func TestProductAnalyticsParsersUseClosedAllowlists(t *testing.T) {
	for _, value := range []string{
		"impressions", "clicks", "views", "contact_intents", "unique_visitors", "ctr", "detail_view_rate", "contact_intent_rate",
	} {
		if metric, ok := ParseProductMetric(value); !ok || string(metric) != value {
			t.Fatalf("ParseProductMetric(%q) = %q/%v", value, metric, ok)
		}
	}
	if metric, ok := ParseProductMetric(""); !ok || metric != ProductMetricClicks {
		t.Fatalf("default product metric = %q/%v", metric, ok)
	}
	if _, ok := ParseProductMetric("clicks; DROP TABLE storefront_events"); ok {
		t.Fatal("unsafe product metric was accepted")
	}
	if order, ok := ParseSortOrder(""); !ok || order != SortOrderDescending {
		t.Fatalf("default sort order = %q/%v", order, ok)
	}
	if _, ok := ParseSortOrder("DESC NULLS FIRST"); ok {
		t.Fatal("unsafe sort order was accepted")
	}
	for _, status := range []string{"", "on_shelf", "off_shelf", "deleted"} {
		if _, ok := ParseProductStatusFilter(status); !ok {
			t.Fatalf("safe status %q was rejected", status)
		}
	}
	if _, ok := ParseProductStatusFilter("archived' OR 1=1"); ok {
		t.Fatal("unsafe product status was accepted")
	}
}

func TestProductAnalyticsRatesReuseCoreZeroDenominatorSemantics(t *testing.T) {
	rates := (ProductCounts{Impressions: 100, Clicks: 25, Views: 10, ContactIntents: 2}).Rates()
	if rates.ProductCTR != 0.25 || rates.DetailViewRate != 0.4 || rates.ContactIntentRate != 0.2 {
		t.Fatalf("product rates = %+v", rates)
	}
	zero := (ProductCounts{}).Rates()
	if zero.ProductCTR != 0 || zero.DetailViewRate != 0 || zero.ContactIntentRate != 0 {
		t.Fatalf("zero product rates = %+v", zero)
	}
}
