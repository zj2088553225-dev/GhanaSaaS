package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestProductPopularityPostgresRanking(t *testing.T) {
	db := openPlatformCategorySearchPostgresDB(t, false)
	createProductPopularityTables(t, db)

	scoredAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	seedProductPopularityFixture(t, db, scoredAt)
	repository := NewProductPopularityRepository(db)
	ctx := t.Context()
	tenantOne := uint(1)

	query := func(popularityQuery ProductPopularityQuery) []ProductPopularityRow {
		t.Helper()
		popularityQuery.ScoredAt = scoredAt
		rows, err := repository.ListPopularProducts(ctx, popularityQuery)
		if err != nil {
			t.Fatalf("list popular products: %v", err)
		}
		return rows
	}

	t.Run("no data returns an empty array", func(t *testing.T) {
		categoryID := uint(999)
		rows := query(ProductPopularityQuery{
			Scope: ProductPopularityScopeAll, CategoryID: &categoryID, Limit: 20,
		})
		if rows == nil || len(rows) != 0 {
			t.Fatalf("empty rows = %#v, want non-nil empty slice", rows)
		}
	})

	allTenantOne := query(ProductPopularityQuery{
		Scope: ProductPopularityScopeTenant, TenantID: &tenantOne, Limit: 100,
	})
	byID := popularityRowsByID(allTenantOne)

	t.Run("view score and time decay are exact", func(t *testing.T) {
		if row := byID[1]; row == nil || row.Views != 1 || row.HotScore != 1 {
			t.Fatalf("view-only row = %+v, want one view and score 1", row)
		}
		if row := byID[6]; row == nil || row.Views != 3 || row.HotScore != 1.7 {
			t.Fatalf("decayed row = %+v, want 1 + .5 + .2 = 1.7", row)
		}
	})

	t.Run("click is weighted above view", func(t *testing.T) {
		if byID[2] == nil || byID[1] == nil || byID[2].HotScore <= byID[1].HotScore {
			t.Fatalf("click/view scores = %+v/%+v", byID[2], byID[1])
		}
		if byID[2].Clicks != 1 || byID[2].HotScore != 3 {
			t.Fatalf("click row = %+v, want one click and score 3", byID[2])
		}
	})

	t.Run("reservation raises ranking and cancellation deducts score", func(t *testing.T) {
		reservation := byID[3]
		cancelled := byID[4]
		verified := byID[5]
		if reservation == nil || reservation.InterestCount != 1 || reservation.VerifiedCount != 1 || reservation.HotScore != 45 {
			t.Fatalf("reservation row = %+v, want interest 15 + verified 30", reservation)
		}
		if cancelled == nil || cancelled.VerifiedCount != 0 || cancelled.HotScore != -10 {
			t.Fatalf("cancelled row = %+v, want cancellation-only score -10", cancelled)
		}
		if verified == nil || verified.HotScore != 30 || cancelled.HotScore >= verified.HotScore {
			t.Fatalf("cancelled/verified rows = %+v/%+v", cancelled, verified)
		}
	})

	t.Run("tenant and bot isolation", func(t *testing.T) {
		if row := byID[7]; row == nil || row.Views != 0 || row.HotScore != 0 {
			t.Fatalf("bot-only product contributed to ranking: %+v", row)
		}
		if byID[8] != nil {
			t.Fatalf("other tenant product entered tenant ranking: %+v", byID[8])
		}
		allRows := query(ProductPopularityQuery{Scope: ProductPopularityScopeAll, Limit: 100})
		if popularityRowsByID(allRows)[8] == nil {
			t.Fatal("super-admin all-tenant ranking did not include tenant 2")
		}
	})

	t.Run("products without signals provide a stable fallback", func(t *testing.T) {
		row := byID[10]
		if row == nil || row.HotScore != 0 || row.Views != 0 || row.Clicks != 0 ||
			row.Contacts != 0 || row.InterestCount != 0 || row.VerifiedCount != 0 {
			t.Fatalf("no-signal fallback row = %+v", row)
		}
	})

	t.Run("limit and category filter", func(t *testing.T) {
		limited := query(ProductPopularityQuery{
			Scope: ProductPopularityScopeTenant, TenantID: &tenantOne, Limit: 2,
		})
		if len(limited) != 2 || limited[0].ProductID != 3 || limited[1].ProductID != 5 {
			t.Fatalf("limited ranking = %+v, want reservation leaders", limited)
		}
		categoryID := uint(2)
		filtered := query(ProductPopularityQuery{
			Scope: ProductPopularityScopeTenant, TenantID: &tenantOne, CategoryID: &categoryID, Limit: 20,
		})
		if len(filtered) != 1 || filtered[0].ProductID != 9 {
			t.Fatalf("category ranking = %+v, want product 9", filtered)
		}
	})

	t.Run("concurrent queries are safe", func(t *testing.T) {
		const workers = 12
		var waitGroup sync.WaitGroup
		errors := make(chan error, workers)
		for worker := 0; worker < workers; worker++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				rows, err := repository.ListPopularProducts(context.Background(), ProductPopularityQuery{
					Scope:    ProductPopularityScopeTenant,
					TenantID: &tenantOne,
					Limit:    100,
					ScoredAt: scoredAt,
				})
				if err != nil {
					errors <- err
					return
				}
				if len(rows) != len(allTenantOne) || rows[0].ProductID != allTenantOne[0].ProductID {
					errors <- fmt.Errorf("unstable concurrent ranking: %+v", rows)
				}
			}()
		}
		waitGroup.Wait()
		close(errors)
		for err := range errors {
			t.Error(err)
		}
	})
}

func createProductPopularityTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE tenants (
			id bigint PRIMARY KEY,
			name varchar(120) NOT NULL,
			type varchar(32) NOT NULL,
			status varchar(32) NOT NULL
		);
		CREATE TABLE spus (
			id bigint PRIMARY KEY,
			tenant_id bigint NOT NULL,
			category_id bigint,
			platform_category_id bigint NOT NULL,
			name varchar(200) NOT NULL,
			main_image varchar(255) NOT NULL DEFAULT '',
			status varchar(32) NOT NULL,
			created_at timestamptz NOT NULL DEFAULT NOW()
		);
		CREATE TABLE shop_profiles (
			id bigint PRIMARY KEY,
			tenant_id bigint NOT NULL,
			is_public boolean NOT NULL
		);
		CREATE TABLE skus (
			id bigint PRIMARY KEY,
			product_id bigint NOT NULL,
			retail_price bigint NOT NULL,
			original_price bigint NOT NULL DEFAULT 0,
			is_active boolean NOT NULL
		);
		CREATE TABLE storefront_events (
			id bigint PRIMARY KEY,
			product_id bigint,
			tenant_id bigint,
			event_name varchar(40) NOT NULL,
			traffic_type varchar(16) NOT NULL,
			occurred_at timestamptz NOT NULL
		);
		CREATE TABLE reservation_products (
			id bigint PRIMARY KEY,
			linked_product_id bigint,
			status varchar(16) NOT NULL
		);
		CREATE TABLE reservation_interests (
			id bigint PRIMARY KEY,
			reservation_product_id bigint NOT NULL,
			created_at timestamptz NOT NULL
		);
		CREATE TABLE product_reservations (
			id bigint PRIMARY KEY,
			reservation_product_id bigint NOT NULL,
			identity_type varchar(20) NOT NULL,
			verification_status varchar(16) NOT NULL,
			status varchar(16) NOT NULL,
			verified_at timestamptz,
			cancelled_at timestamptz,
			expires_at timestamptz,
			created_at timestamptz NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create product popularity tables: %v", err)
	}
}

func seedProductPopularityFixture(t *testing.T, db *gorm.DB, scoredAt time.Time) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO tenants (id, name, type, status) VALUES
			(1, 'Tenant One', 'self_operated', 'active'),
			(2, 'Tenant Two', '3rd_party', 'active');
		INSERT INTO spus (id, tenant_id, category_id, platform_category_id, name, status) VALUES
			(1, 1, 1, 11, 'View only', 'on_shelf'),
			(2, 1, 1, 11, 'Click only', 'on_shelf'),
			(3, 1, 1, 11, 'Interest and verified', 'on_shelf'),
			(4, 1, 1, 11, 'Verified then cancellation', 'on_shelf'),
			(5, 1, 1, 11, 'Verified only', 'on_shelf'),
			(6, 1, 1, 11, 'Decayed views', 'on_shelf'),
			(7, 1, 1, 11, 'Bot only', 'on_shelf'),
			(8, 2, 1, 11, 'Other tenant', 'on_shelf'),
			(9, 1, 2, 12, 'Other category', 'on_shelf'),
			(10, 1, 1, 11, 'No signals', 'on_shelf'),
			(11, 1, 1, 11, 'Off shelf', 'off_shelf');
		INSERT INTO skus (id, product_id, retail_price, original_price, is_active)
		SELECT id, id, id * 100, id * 120, TRUE FROM spus;
		INSERT INTO shop_profiles (id, tenant_id, is_public) VALUES (1, 2, TRUE)
	`).Error; err != nil {
		t.Fatalf("seed product popularity products: %v", err)
	}

	event := func(id, productID int, name, traffic string, occurredAt time.Time) {
		t.Helper()
		tenantID := 1
		if productID == 8 {
			tenantID = 2
		}
		if err := db.Exec(
			"INSERT INTO storefront_events (id, product_id, tenant_id, event_name, traffic_type, occurred_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, productID, tenantID, name, traffic, occurredAt,
		).Error; err != nil {
			t.Fatalf("seed storefront event %d: %v", id, err)
		}
	}
	event(1, 1, "product_view", "human", scoredAt.Add(-24*time.Hour))
	event(2, 2, "product_click", "human", scoredAt.Add(-24*time.Hour))
	event(3, 6, "product_view", "human", scoredAt.Add(-24*time.Hour))
	event(4, 6, "product_view", "human", scoredAt.Add(-10*24*time.Hour))
	event(5, 6, "product_view", "human", scoredAt.Add(-40*24*time.Hour))
	event(6, 7, "product_view", "bot", scoredAt.Add(-time.Hour))
	event(7, 8, "product_view", "human", scoredAt.Add(-time.Hour))
	event(8, 9, "whatsapp_click", "human", scoredAt.Add(-time.Hour))
	event(9, 11, "phone_click", "human", scoredAt.Add(-time.Hour))
	if err := db.Exec(`
		INSERT INTO storefront_events (id, product_id, tenant_id, event_name, traffic_type, occurred_at)
		VALUES
			(10, 1, 2, 'product_view', 'human', ?),
			(11, 1, 1, 'product_view', 'human', ?)
	`, scoredAt.Add(-time.Hour), scoredAt.Add(time.Hour)).Error; err != nil {
		t.Fatalf("seed rejected analytics events: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO reservation_products (id, linked_product_id, status) VALUES
			(1, 3, 'open'),
			(2, 4, 'open'),
			(3, 5, 'open'),
			(4, 1, 'open')
	`).Error; err != nil {
		t.Fatalf("seed reservation products: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO reservation_interests (id, reservation_product_id, created_at)
		VALUES (1, 1, ?)
	`, scoredAt.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("seed reservation interests: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO product_reservations (
			id, reservation_product_id, identity_type, verification_status, status, verified_at, cancelled_at, expires_at, created_at
		) VALUES
			(1, 1, 'phone_verified', 'verified', 'active', ?, NULL, NULL, ?),
			(2, 2, 'phone_verified', 'cancelled', 'cancelled', ?, ?, NULL, ?),
			(3, 3, 'phone_verified', 'verified', 'active', ?, NULL, NULL, ?),
			(4, 4, 'phone_verified', 'expired', 'expired', NULL, NULL, ?, ?)
	`, scoredAt.Add(-30*time.Minute), scoredAt.Add(-time.Hour),
		scoredAt.Add(-2*time.Hour), scoredAt.Add(-30*time.Minute), scoredAt.Add(-time.Hour),
		scoredAt.Add(-30*time.Minute), scoredAt.Add(-time.Hour),
		scoredAt.Add(-time.Minute), scoredAt.Add(-time.Hour),
	).Error; err != nil {
		t.Fatalf("seed product popularity reservations: %v", err)
	}
}

func popularityRowsByID(rows []ProductPopularityRow) map[uint]*ProductPopularityRow {
	result := make(map[uint]*ProductPopularityRow, len(rows))
	for index := range rows {
		result[rows[index].ProductID] = &rows[index]
	}
	return result
}
