package repository

import (
	"context"
	"strings"
	"time"

	"saas/internal/analytics"
	"saas/internal/models"
)

func (r *StorefrontAnalyticsRepository) AnalyticsStatus(ctx context.Context, now time.Time) (analytics.DataStatus, error) {
	now = now.UTC()
	query := `
SELECT
    COUNT(*) AS total_events,
    COALESCE(SUM(CASE WHEN traffic_type = ? THEN 1 ELSE 0 END), 0) AS human_events,
    COALESCE(SUM(CASE WHEN traffic_type = ? THEN 1 ELSE 0 END), 0) AS bot_events,
    MIN(occurred_at) AS oldest_event_at,
    MAX(occurred_at) AS latest_event_at,
    COALESCE(SUM(CASE WHEN occurred_at >= ? AND occurred_at < ? THEN 1 ELSE 0 END), 0) AS events_last24h
FROM storefront_events`
	args := []any{models.StorefrontTrafficHuman, models.StorefrontTrafficBot, now.Add(-24 * time.Hour), now}
	var status analytics.DataStatus
	if r.db.Dialector.Name() == "sqlite" {
		var row struct {
			TotalEvents   int64
			HumanEvents   int64
			BotEvents     int64
			OldestEventAt *string
			LatestEventAt *string
			EventsLast24h int64
		}
		if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&row).Error; err != nil {
			return analytics.DataStatus{}, err
		}
		status.TotalEvents = row.TotalEvents
		status.HumanEvents = row.HumanEvents
		status.BotEvents = row.BotEvents
		status.EventsLast24h = row.EventsLast24h
		status.OldestEventAt = parseSQLiteAnalyticsTime(row.OldestEventAt)
		status.LatestEventAt = parseSQLiteAnalyticsTime(row.LatestEventAt)
	} else {
		var row struct {
			TotalEvents   int64
			HumanEvents   int64
			BotEvents     int64
			OldestEventAt *time.Time
			LatestEventAt *time.Time
			EventsLast24h int64
		}
		if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&row).Error; err != nil {
			return analytics.DataStatus{}, err
		}
		status.TotalEvents = row.TotalEvents
		status.HumanEvents = row.HumanEvents
		status.BotEvents = row.BotEvents
		status.EventsLast24h = row.EventsLast24h
		status.OldestEventAt = row.OldestEventAt
		status.LatestEventAt = row.LatestEventAt
	}

	status.EventsByName = make(map[string]int64, len(analytics.EventAllowlist))
	for _, eventName := range analytics.EventAllowlist {
		status.EventsByName[string(eventName)] = 0
	}
	var counts []struct {
		EventName string
		Count     int64
	}
	if err := r.db.WithContext(ctx).
		Table("storefront_events").
		Select("event_name, COUNT(*) AS count").
		Where("event_name IN ?", analytics.EventAllowlist).
		Group("event_name").
		Scan(&counts).Error; err != nil {
		return analytics.DataStatus{}, err
	}
	for _, count := range counts {
		name := strings.TrimSpace(count.EventName)
		if _, allowed := status.EventsByName[name]; allowed {
			status.EventsByName[name] = count.Count
		}
	}
	return status, nil
}

func parseSQLiteAnalyticsTime(value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, *value); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}
