package service

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const dashboardTrendDays = 7
const lowStockLimit = 10
const lowStockThreshold = 5

type RevenueTrendItem struct {
	Date   string `json:"date"`
	Amount int64  `json:"amount"`
}

type LowStockSKU struct {
	ID            uint           `json:"id"`
	ProductID     uint           `json:"product_id"`
	ProductName   string         `json:"product_name"`
	Name          string         `json:"name"`
	Specs         datatypes.JSON `json:"specs"`
	SellableStock int64          `json:"sellable_stock"`
}

type DashboardSummary struct {
	TodayOrders   int64              `json:"today_orders"`
	TodayRevenue  int64              `json:"today_revenue"`
	PendingOrders int64              `json:"pending_orders"`
	RevenueTrend  []RevenueTrendItem `json:"revenue_trend"`
	LowStockSKUs  []LowStockSKU      `json:"low_stock_skus"`
}

type DashboardService struct {
	db *gorm.DB
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return &DashboardService{db: db}
}

func (s *DashboardService) Summary(ctx context.Context, tenantID uint) (*DashboardSummary, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	trendStart := todayStart.AddDate(0, 0, -(dashboardTrendDays - 1))

	summary := DashboardSummary{
		RevenueTrend: make([]RevenueTrendItem, 0, dashboardTrendDays),
		LowStockSKUs: make([]LowStockSKU, 0),
	}

	if err := s.db.WithContext(ctx).
		Table("orders").
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, todayStart, tomorrowStart).
		Count(&summary.TodayOrders).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Table("orders").
		Select("COALESCE(SUM(total_amount), 0)").
		Where("tenant_id = ? AND status <> ? AND created_at >= ? AND created_at < ?", tenantID, "cancelled", todayStart, tomorrowStart).
		Scan(&summary.TodayRevenue).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Table("orders").
		Where("tenant_id = ? AND status = ?", tenantID, "pending").
		Count(&summary.PendingOrders).Error; err != nil {
		return nil, err
	}

	trendByDate := make(map[string]int64, dashboardTrendDays)
	var trendRows []RevenueTrendItem
	if err := s.db.WithContext(ctx).
		Table("orders").
		Select("TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date, COALESCE(SUM(total_amount), 0) AS amount").
		Where("tenant_id = ? AND status <> ? AND created_at >= ? AND created_at < ?", tenantID, "cancelled", trendStart, tomorrowStart).
		Group("(created_at AT TIME ZONE 'UTC')::date").
		Order("(created_at AT TIME ZONE 'UTC')::date ASC").
		Scan(&trendRows).Error; err != nil {
		return nil, err
	}
	for _, row := range trendRows {
		trendByDate[row.Date] = row.Amount
	}
	for i := 0; i < dashboardTrendDays; i++ {
		date := trendStart.AddDate(0, 0, i).Format("2006-01-02")
		summary.RevenueTrend = append(summary.RevenueTrend, RevenueTrendItem{
			Date:   date,
			Amount: trendByDate[date],
		})
	}

	if err := s.db.WithContext(ctx).
		Table("skus").
		Select("skus.id, skus.product_id, spus.name AS product_name, skus.name, skus.specs, skus.sellable_stock").
		Joins("JOIN spus ON spus.id = skus.product_id").
		Where("spus.tenant_id = ? AND skus.is_active = ? AND skus.sellable_stock <= ?", tenantID, true, lowStockThreshold).
		Order("skus.sellable_stock ASC, skus.id ASC").
		Limit(lowStockLimit).
		Scan(&summary.LowStockSKUs).Error; err != nil {
		return nil, err
	}

	return &summary, nil
}
