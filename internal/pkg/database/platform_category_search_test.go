package database

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPlatformCategorySearchInfrastructureDoesNotRunPostgresSQLOnSQLite(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := EnsurePlatformCategorySearchInfrastructure(db); err != nil {
		t.Fatalf("SQLite infrastructure should be a no-op: %v", err)
	}
	status, err := CheckPlatformCategorySearchInfrastructure(db)
	if err != nil {
		t.Fatalf("check SQLite infrastructure: %v", err)
	}
	if status.Ready() || status.TrigramExtension || status.SearchTextGIN || status.OrderBTree {
		t.Fatalf("SQLite reported PostgreSQL infrastructure: %+v", status)
	}
}

func TestPlatformCategorySearchInfrastructureRejectsNilDatabase(t *testing.T) {
	t.Parallel()
	if err := EnsurePlatformCategorySearchInfrastructure(nil); err == nil {
		t.Fatal("EnsurePlatformCategorySearchInfrastructure(nil) returned nil")
	}
	if _, err := CheckPlatformCategorySearchInfrastructure(nil); err == nil {
		t.Fatal("CheckPlatformCategorySearchInfrastructure(nil) returned nil")
	}
}
