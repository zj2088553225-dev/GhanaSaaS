package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"saas/internal/analytics"
	"saas/internal/models"
)

func TestProductAnalyticsCSVExportUsesScopeFiltersBOMAndSafeCells(t *testing.T) {
	repo := &fakeProductAnalyticsRepository{
		listRows: []analytics.ProductAggregate{{
			ProductMetadata: analytics.ProductMetadata{
				ProductID: 19, ProductName: `=HYPERLINK("https://evil")`, TenantID: 7,
				TenantName: "+Tenant", Status: models.SPUStatusOffShelf, CurrentlyExists: true,
			},
			ProductCounts: analytics.ProductCounts{Impressions: 200, Clicks: 25, Views: 10, ContactIntents: 2, UniqueVisitors: 18},
		}},
		listTotal: 1,
	}
	tracker := newFixedAnalyticsService(repo)
	export, err := tracker.ProductExportForTenant(context.Background(), 7, ProductAnalyticsListRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-14", Timezone: "Africa/Accra"},
		Page:      99, PageSize: 50, Search: " lamp ", Status: "off_shelf", Sort: "ctr", Order: "asc",
	})
	if err != nil {
		t.Fatalf("ProductExportForTenant: %v", err)
	}
	if repo.listScope != analytics.TenantScope(7) || repo.listQuery.Page != 1 || repo.listQuery.PageSize != AnalyticsExportMaxRows+1 ||
		repo.listQuery.Search != "lamp" || repo.listQuery.Status != analytics.ProductStatusFilter(models.SPUStatusOffShelf) ||
		repo.listQuery.Sort != analytics.ProductMetricCTR || repo.listQuery.Order != analytics.SortOrderAscending {
		t.Fatalf("product export scope/query = %+v/%+v", repo.listScope, repo.listQuery)
	}
	if export.Filename != "product-analytics-20260701-20260714.csv" || !bytes.HasPrefix(export.Content, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("product export filename/BOM = %q/% x", export.Filename, export.Content[:3])
	}
	records := readAnalyticsCSVForTest(t, export.Content)
	if len(records) != 2 || records[1][1] != `'=HYPERLINK("https://evil")` || records[1][3] != "'+Tenant" ||
		records[1][6] != "200" || records[1][11] != "0.125" {
		t.Fatalf("product CSV records = %#v", records)
	}
	content := string(export.Content)
	for _, forbidden := range []string{"visitor_id", "session_id", "referrer"} {
		if strings.Contains(strings.ToLower(content), forbidden) {
			t.Fatalf("product CSV leaked %s", forbidden)
		}
	}
}

func TestProductAnalyticsExportRejectsOverTenThousandAndValidatesSuperTenant(t *testing.T) {
	repo := &fakeProductAnalyticsRepository{
		fakeStorefrontAnalyticsRepository: fakeStorefrontAnalyticsRepository{tenantExists: map[uint]bool{9: true}},
		listTotal:                         AnalyticsExportMaxRows + 1,
	}
	tracker := newFixedAnalyticsService(repo)
	tenantID := uint(9)
	_, err := tracker.ProductExportForSuper(context.Background(), &tenantID, ProductAnalyticsListRequest{})
	if !errors.Is(err, ErrAnalyticsExportTooLarge) || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("large product export error = %v", err)
	}
	if repo.listScope != analytics.TenantScope(9) {
		t.Fatalf("product export scope = %+v", repo.listScope)
	}
	missing := uint(10)
	if _, err := tracker.ProductExportForSuper(context.Background(), &missing, ProductAnalyticsListRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing export tenant error = %v", err)
	}
}

func TestContentAnalyticsCSVExportUsesAggregateMetadataFilterAndSafeCells(t *testing.T) {
	const key = "11111111-1111-4111-8111-111111111111"
	repo := &fakeContentAnalyticsRepository{rows: []analytics.ContentAggregate{{
		TrackingKey:   key,
		ContentCounts: analytics.ContentCounts{Impressions: 100, Clicks: 20, UniqueVisitors: 15},
	}}}
	resolver := &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{
		key: {
			TrackingKey: key, ContentType: analytics.ContentTypePromoBanner, Title: "@Summer, Sale",
			TargetType: analytics.ContentTargetExternal, TargetValue: "=https://example.com", Published: true,
		},
	}}
	tracker := newFixedContentAnalyticsService(repo, resolver)
	exists := true
	export, err := tracker.ContentExportForSuper(context.Background(), ContentAnalyticsListRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-14"},
		Search:    "summer", ContentType: "promo_banner", CurrentlyExists: &exists, Sort: "ctr", Order: "desc",
	})
	if err != nil {
		t.Fatalf("ContentExportForSuper: %v", err)
	}
	if resolver.calls != 1 || repo.listQuery.Paginate || repo.listQuery.PageSize != AnalyticsExportMaxRows+1 || repo.listQuery.MinImpressions != 100 {
		t.Fatalf("content export resolver/query = %d/%+v", resolver.calls, repo.listQuery)
	}
	if export.Filename != "content-analytics-20260701-20260714.csv" || !bytes.HasPrefix(export.Content, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("content export filename/BOM = %q/% x", export.Filename, export.Content[:3])
	}
	records := readAnalyticsCSVForTest(t, export.Content)
	if len(records) != 2 || records[1][2] != "'@Summer, Sale" || records[1][4] != "'=https://example.com" ||
		records[1][7] != "100" || records[1][10] != "0.2" {
		t.Fatalf("content CSV records = %#v", records)
	}
	for _, forbidden := range []string{"visitor_id", "session_id", "referrer"} {
		if strings.Contains(strings.ToLower(string(export.Content)), forbidden) {
			t.Fatalf("content CSV leaked %s", forbidden)
		}
	}
}

func TestContentAnalyticsExportRejectsOverTenThousandRows(t *testing.T) {
	repo := &fakeContentAnalyticsRepository{total: AnalyticsExportMaxRows + 1}
	tracker := newFixedContentAnalyticsService(repo, &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{}})
	_, err := tracker.ContentExportForSuper(context.Background(), ContentAnalyticsListRequest{})
	if !errors.Is(err, ErrAnalyticsExportTooLarge) {
		t.Fatalf("large content export error = %v", err)
	}
}

func TestAnalyticsExportsAllowExactlyTenThousandRows(t *testing.T) {
	productRows := make([]analytics.ProductAggregate, AnalyticsExportMaxRows)
	for index := range productRows {
		productRows[index].ProductID = uint(index + 1)
	}
	productRepo := &fakeProductAnalyticsRepository{listRows: productRows, listTotal: AnalyticsExportMaxRows}
	productExport, err := newFixedAnalyticsService(productRepo).ProductExportForTenant(
		context.Background(),
		7,
		ProductAnalyticsListRequest{},
	)
	if err != nil {
		t.Fatalf("10,000-row product export: %v", err)
	}
	if records := readAnalyticsCSVForTest(t, productExport.Content); len(records) != AnalyticsExportMaxRows+1 {
		t.Fatalf("10,000-row product CSV records = %d", len(records))
	}

	contentRows := make([]analytics.ContentAggregate, AnalyticsExportMaxRows)
	for index := range contentRows {
		contentRows[index].TrackingKey = "11111111-1111-4111-8111-111111111111"
	}
	contentRepo := &fakeContentAnalyticsRepository{rows: contentRows, total: AnalyticsExportMaxRows}
	contentExport, err := newFixedContentAnalyticsService(
		contentRepo,
		&fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{}},
	).ContentExportForSuper(context.Background(), ContentAnalyticsListRequest{})
	if err != nil {
		t.Fatalf("10,000-row content export: %v", err)
	}
	if records := readAnalyticsCSVForTest(t, contentExport.Content); len(records) != AnalyticsExportMaxRows+1 {
		t.Fatalf("10,000-row content CSV records = %d", len(records))
	}
}

func TestAnalyticsCSVFormulaProtectionHandlesRequiredPrefixesAndCSVQuotes(t *testing.T) {
	formulas := []string{`=HYPERLINK("https://evil")`, "+SUM(A1:A2)", "-1+2", "@SUM(A1:A2)"}
	leadingWhitespace := []string{
		"", " ", "\t", "\r", "\n", " \t\r\n  ",
		"\u00A0", "\u2000", "\u2001", "\u2002", "\u2003", "\u2007",
		"\u202F", "\u205F", "\u3000", "\uFEFF",
	}
	for _, whitespace := range leadingWhitespace {
		for _, formula := range formulas {
			value := whitespace + formula
			if protected := safeAnalyticsCSVCell(value); protected != "'"+value {
				t.Fatalf("safeAnalyticsCSVCell(%q) = %q, want apostrophe plus original value", value, protected)
			}
		}
	}
	for _, value := range []string{"=1+1", "+SUM(A1:A2)", "-1+2", "@SUM(A1:A2)"} {
		if protected := safeAnalyticsCSVCell(value); protected != "'"+value {
			t.Fatalf("safeAnalyticsCSVCell(%q) = %q", value, protected)
		}
	}
	for _, safe := range []string{"", "plain text", "'=@already-safe", " \t'@already-safe", "123"} {
		if protected := safeAnalyticsCSVCell(safe); protected != safe {
			t.Fatalf("safeAnalyticsCSVCell(%q) = %q, want unchanged", safe, protected)
		}
	}
	encoded, err := encodeAnalyticsCSV([][]string{{"Name"}, {"comma,quote\"line\nnext"}})
	if err != nil {
		t.Fatalf("encodeAnalyticsCSV: %v", err)
	}
	records := readAnalyticsCSVForTest(t, encoded)
	if records[1][0] != "comma,quote\"line\nnext" {
		t.Fatalf("CSV escaping round trip = %#v", records)
	}
}

func readAnalyticsCSVForTest(t *testing.T, content []byte) [][]string {
	t.Helper()
	if len(content) < 3 {
		t.Fatalf("CSV is too short: %d", len(content))
	}
	records, err := csv.NewReader(bytes.NewReader(content[3:])).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	return records
}
