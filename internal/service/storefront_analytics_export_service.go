package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"unicode"

	"saas/internal/analytics"
)

const AnalyticsExportMaxRows = 10_000

type AnalyticsCSVExport struct {
	Filename string
	Content  []byte
}

func (s *StorefrontAnalyticsService) ProductExportForSuper(
	ctx context.Context,
	tenantID *uint,
	request ProductAnalyticsListRequest,
) (*AnalyticsCSVExport, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.productExport(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) ProductExportForTenant(
	ctx context.Context,
	tenantID uint,
	request ProductAnalyticsListRequest,
) (*AnalyticsCSVExport, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.productExport(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) productExport(
	ctx context.Context,
	scope analytics.Scope,
	request ProductAnalyticsListRequest,
) (*AnalyticsCSVExport, error) {
	if s.productRepo == nil {
		return nil, ErrInvalidInput
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	request.Page = 1
	request.PageSize = analytics.MaxReportPageSize
	query, err := normalizeProductAnalyticsListRequest(request)
	if err != nil {
		return nil, err
	}
	query.Page = 1
	query.PageSize = AnalyticsExportMaxRows + 1
	rows, total, err := s.productRepo.ProductAnalyticsList(ctx, scope, dateRange.Window(), query)
	if err != nil {
		return nil, err
	}
	if total > AnalyticsExportMaxRows || len(rows) > AnalyticsExportMaxRows {
		return nil, ErrAnalyticsExportTooLarge
	}
	content, err := productAnalyticsCSV(productAnalyticsItems(rows))
	if err != nil {
		return nil, err
	}
	return &AnalyticsCSVExport{
		Filename: analyticsExportFilename("product", dateRange),
		Content:  content,
	}, nil
}

func (s *StorefrontAnalyticsService) ContentExportForSuper(
	ctx context.Context,
	request ContentAnalyticsListRequest,
) (*AnalyticsCSVExport, error) {
	if s.contentRepo == nil || s.contentResolver == nil {
		return nil, ErrInvalidInput
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	request.Page = 1
	request.PageSize = analytics.MaxReportPageSize
	query, metadataFilter, err := normalizeContentAnalyticsListRequest(request)
	if err != nil {
		return nil, err
	}
	metadata, err := s.contentResolver.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	query.Page = 1
	query.PageSize = AnalyticsExportMaxRows + 1
	query.Paginate = !metadataFilter.Active()
	rows, total, err := s.contentRepo.ListContentAnalytics(ctx, dateRange.Window(), query)
	if err != nil {
		return nil, err
	}
	items := contentAnalyticsItems(rows, metadata)
	if metadataFilter.Active() {
		items = filterContentAnalyticsItems(items, metadataFilter)
		total = int64(len(items))
	}
	if total > AnalyticsExportMaxRows || len(items) > AnalyticsExportMaxRows {
		return nil, ErrAnalyticsExportTooLarge
	}
	content, err := contentAnalyticsCSV(items)
	if err != nil {
		return nil, err
	}
	return &AnalyticsCSVExport{
		Filename: analyticsExportFilename("content", dateRange),
		Content:  content,
	}, nil
}

func productAnalyticsCSV(items []ProductAnalyticsItem) ([]byte, error) {
	records := make([][]string, 0, len(items)+1)
	records = append(records, []string{
		"Product ID", "Product Name", "Slug", "Tenant", "Status", "Currently Exists",
		"Impressions", "Clicks", "Product Views", "Contact Intents", "Unique Visitors",
		"CTR", "Detail View Rate", "Contact Intent Rate",
	})
	for _, item := range items {
		records = append(records, []string{
			strconv.FormatUint(uint64(item.ProductID), 10),
			safeAnalyticsCSVCell(item.ProductName),
			safeAnalyticsCSVCell(item.ProductSlug),
			safeAnalyticsCSVCell(item.TenantName),
			safeAnalyticsCSVCell(string(item.Status)),
			strconv.FormatBool(item.CurrentlyExists),
			strconv.FormatInt(item.Impressions, 10),
			strconv.FormatInt(item.Clicks, 10),
			strconv.FormatInt(item.Views, 10),
			strconv.FormatInt(item.ContactIntents, 10),
			strconv.FormatInt(item.UniqueVisitors, 10),
			formatAnalyticsCSVRate(item.CTR),
			formatAnalyticsCSVRate(item.DetailViewRate),
			formatAnalyticsCSVRate(item.ContactIntentRate),
		})
	}
	return encodeAnalyticsCSV(records)
}

func contentAnalyticsCSV(items []ContentAnalyticsItem) ([]byte, error) {
	records := make([][]string, 0, len(items)+1)
	records = append(records, []string{
		"Tracking Key", "Content Type", "Title", "Target Type", "Target Value",
		"Currently Exists", "Published", "Impressions", "Clicks", "Unique Visitors", "CTR",
	})
	for _, item := range items {
		records = append(records, []string{
			safeAnalyticsCSVCell(item.TrackingKey),
			safeAnalyticsCSVCell(string(item.ContentType)),
			safeAnalyticsCSVCell(item.Title),
			safeAnalyticsCSVCell(string(item.TargetType)),
			safeAnalyticsCSVCell(item.TargetValue),
			strconv.FormatBool(item.CurrentlyExists),
			strconv.FormatBool(item.Published),
			strconv.FormatInt(item.Impressions, 10),
			strconv.FormatInt(item.Clicks, 10),
			strconv.FormatInt(item.UniqueVisitors, 10),
			formatAnalyticsCSVRate(item.CTR),
		})
	}
	return encodeAnalyticsCSV(records)
}

func safeAnalyticsCSVCell(value string) string {
	for _, current := range value {
		if unicode.IsSpace(current) || current == '\uFEFF' {
			continue
		}
		switch current {
		case '=', '+', '-', '@':
			return "'" + value
		default:
			return value
		}
	}
	return value
}

func formatAnalyticsCSVRate(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func encodeAnalyticsCSV(records [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(records); err != nil {
		return nil, err
	}
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func analyticsExportFilename(kind string, dateRange analytics.DateRange) string {
	from := strings.ReplaceAll(dateRange.From, "-", "")
	to := strings.ReplaceAll(dateRange.To, "-", "")
	return kind + "-analytics-" + from + "-" + to + ".csv"
}
