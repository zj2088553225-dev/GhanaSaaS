package main

import (
	"bytes"
	"strings"
	"testing"

	"saas/internal/pkg/database"
	"saas/internal/repository"
)

func TestParsePlatformCategorySearchRebuildCLIOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
		check   func(*testing.T, platformCategorySearchRebuildCLIOptions)
	}{
		{name: "default dry run", check: func(t *testing.T, options platformCategorySearchRebuildCLIOptions) {
			if !options.DryRun || options.Apply || options.BatchSize != 500 {
				t.Fatalf("options = %+v", options)
			}
		}},
		{name: "apply verify subtree", args: []string{"--apply", "--verify", "--batch-size=750", "--category-id=42"}, check: func(t *testing.T, options platformCategorySearchRebuildCLIOptions) {
			if !options.Apply || options.DryRun || !options.Verify || options.BatchSize != 750 || options.CategoryID == nil || *options.CategoryID != 42 {
				t.Fatalf("options = %+v", options)
			}
		}},
		{name: "mutually exclusive", args: []string{"--dry-run", "--apply"}, wantErr: "mutually exclusive"},
		{name: "zero batch", args: []string{"--batch-size=0"}, wantErr: "between 1"},
		{name: "oversized batch", args: []string{"--batch-size=5001"}, wantErr: "between 1"},
		{name: "positional", args: []string{"unexpected"}, wantErr: "positional"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options, err := parsePlatformCategorySearchRebuildCLIOptions(test.args, &bytes.Buffer{})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse options: %v", err)
			}
			test.check(t, options)
		})
	}
}

func TestPlatformCategorySearchRebuildReportContainsAuditableCounters(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writePlatformCategorySearchRebuildReport(
		&output,
		platformCategorySearchRebuildCLIOptions{Apply: true, BatchSize: 500, Verify: true},
		database.PlatformCategorySearchInfrastructureStatus{TrigramExtension: true, SearchTextGIN: true, OrderBTree: true},
		repository.PlatformCategorySearchMaintenanceSummary{
			Categories: 3, ExpectedRows: 6, Created: 2, Updated: 1, Unchanged: 3,
			DeletedOrphans: 1, MissingTranslations: 2, FallbackZhRows: 2, InvalidRows: 0, Verified: true,
		},
	)
	for _, expected := range []string{
		"mode=apply", "categories=3", "expected_rows=6", "created=2", "updated=1", "unchanged=3",
		"deleted_orphans=1", "missing_translations=2", "fallback_zh_rows=2", "invalid_rows=0", "verified=true",
	} {
		if !strings.Contains(output.String(), expected+"\n") {
			t.Fatalf("report missing %q:\n%s", expected, output.String())
		}
	}
}
