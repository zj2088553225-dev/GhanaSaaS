package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAnalyticsStatusAggregatesHumanBotRecentAndEventNames(t *testing.T) {
	db := newAnalyticsStatusTestDB(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	fixtures := []struct {
		name    models.StorefrontEventName
		traffic models.StorefrontTrafficType
		at      time.Time
	}{
		{name: models.StorefrontEventPageView, traffic: models.StorefrontTrafficHuman, at: now.Add(-48 * time.Hour)},
		{name: models.StorefrontEventProductClick, traffic: models.StorefrontTrafficHuman, at: now.Add(-24 * time.Hour)},
		{name: models.StorefrontEventContentClick, traffic: models.StorefrontTrafficBot, at: now.Add(-time.Hour)},
		{name: models.StorefrontEventPageView, traffic: models.StorefrontTrafficBot, at: now},
	}
	for index, fixture := range fixtures {
		event := statusTestEvent(index, fixture.name, fixture.traffic, fixture.at)
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("create status fixture: %v", err)
		}
	}

	status, err := NewStorefrontAnalyticsRepository(db).AnalyticsStatus(context.Background(), now)
	if err != nil {
		t.Fatalf("AnalyticsStatus: %v", err)
	}
	if status.TotalEvents != 4 || status.HumanEvents != 2 || status.BotEvents != 2 || status.EventsLast24h != 2 {
		t.Fatalf("status counts = %+v", status)
	}
	if status.OldestEventAt == nil || !status.OldestEventAt.Equal(now.Add(-48*time.Hour)) ||
		status.LatestEventAt == nil || !status.LatestEventAt.Equal(now) {
		t.Fatalf("status boundaries = oldest %v latest %v", status.OldestEventAt, status.LatestEventAt)
	}
	if status.EventsByName[string(models.StorefrontEventPageView)] != 2 ||
		status.EventsByName[string(models.StorefrontEventProductClick)] != 1 ||
		status.EventsByName[string(models.StorefrontEventContentClick)] != 1 ||
		status.EventsByName[string(models.StorefrontEventPhoneClick)] != 0 {
		t.Fatalf("events_by_name = %+v", status.EventsByName)
	}
}

func TestAnalyticsStatusEmptyTableReturnsZerosAndNullTimes(t *testing.T) {
	status, err := NewStorefrontAnalyticsRepository(newAnalyticsStatusTestDB(t)).AnalyticsStatus(
		context.Background(), time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("AnalyticsStatus empty: %v", err)
	}
	if status.TotalEvents != 0 || status.HumanEvents != 0 || status.BotEvents != 0 || status.EventsLast24h != 0 ||
		status.OldestEventAt != nil || status.LatestEventAt != nil {
		t.Fatalf("empty status = %+v", status)
	}
	if len(status.EventsByName) != 8 {
		t.Fatalf("empty events_by_name = %+v", status.EventsByName)
	}
}

func newAnalyticsStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.StorefrontEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func statusTestEvent(index int, name models.StorefrontEventName, traffic models.StorefrontTrafficType, occurredAt time.Time) models.StorefrontEvent {
	return models.StorefrontEvent{
		EventName: name, VisitorID: fmt.Sprintf("visitor-%d", index), SessionID: fmt.Sprintf("session-%d", index),
		SourceType: models.StorefrontSourceDirect, PageType: models.StorefrontPageOther, PagePath: "/en",
		Locale: "en", DeviceType: models.StorefrontDeviceDesktop, TrafficType: traffic,
		OccurredAt: occurredAt, CreatedAt: occurredAt,
	}
}
