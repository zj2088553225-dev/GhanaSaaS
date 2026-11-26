package analytics

import (
	"os"
	"strings"
	"testing"

	"saas/internal/models"
)

func TestContentAnalyticsDomainAllowlistsAndZeroDenominator(t *testing.T) {
	if len(ContentEventAllowlist) != 2 || ContentEventAllowlist[0] != models.StorefrontEventContentImpression || ContentEventAllowlist[1] != models.StorefrontEventContentClick {
		t.Fatalf("content event allowlist = %+v", ContentEventAllowlist)
	}
	if (ContentCounts{Clicks: 3}).CTR() != 0 {
		t.Fatal("zero-impression content CTR must be zero")
	}
	for _, contentType := range []string{"hero", "hero_banner", "hero_slide", "promo_banner", "image", "image_text", "contact", "category_item", "feature_grid", "link", "other"} {
		if parsed, ok := ParseContentType(contentType); !ok || string(parsed) != contentType {
			t.Errorf("ParseContentType(%q) = %q/%v", contentType, parsed, ok)
		}
	}
	if _, ok := ParseContentType("dynamic-unbounded"); ok {
		t.Fatal("unbounded content type was accepted")
	}
	for _, metric := range []string{"impressions", "clicks", "unique_visitors", "ctr"} {
		if parsed, ok := ParseContentMetric(metric); !ok || string(parsed) != metric {
			t.Errorf("ParseContentMetric(%q) = %q/%v", metric, parsed, ok)
		}
	}
}

func TestStorefrontAnalyticsDocumentationKeepsContentIdentityAndAttributionBoundaries(t *testing.T) {
	data, err := os.ReadFile("../../docs/storefront-analytics-metrics.md")
	if err != nil {
		t.Fatalf("read metrics documentation: %v", err)
	}
	document := strings.ToLower(string(data))
	for _, contract := range []string{
		"gc_visitor_id",
		"tracking_key",
		"locale` is only",
		"currently_exists=true",
		"published=false",
		"currently_exists=false",
		"click_id",
		"first-touch",
		"last-touch",
		"multi-touch",
		"must not be summed",
		"payment-conversion",
	} {
		if !strings.Contains(document, contract) {
			t.Errorf("metrics documentation is missing %q", contract)
		}
	}
}
