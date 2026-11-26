package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"saas/internal/config"
	"saas/internal/pkg/database"
	"saas/internal/repository"
	"saas/internal/service"
)

type platformCategorySearchRebuildCLIOptions struct {
	Apply      bool
	DryRun     bool
	Verify     bool
	BatchSize  int
	CategoryID *uint
}

func main() {
	log.SetFlags(0)
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Printf("platform category search rebuild failed: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	options, err := parsePlatformCategorySearchRebuildCLIOptions(args, stderr)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("access database connection failed")
	}
	defer sqlDB.Close()

	if options.Apply {
		if err := database.EnsurePlatformCategorySearchInfrastructure(db); err != nil {
			return err
		}
	}
	infrastructure, err := database.CheckPlatformCategorySearchInfrastructure(db)
	if err != nil {
		return err
	}

	platformCategoryRepo := repository.NewPlatformCategoryRepository(db)
	platformCategoryService := service.NewPlatformCategoryService(platformCategoryRepo)
	summary, rebuildErr := platformCategoryService.MaintainSearchIndex(ctx, repository.PlatformCategorySearchMaintenanceOptions{
		Apply:      options.Apply,
		Verify:     options.Verify,
		BatchSize:  options.BatchSize,
		CategoryID: options.CategoryID,
		TryLock:    true,
	})
	writePlatformCategorySearchRebuildReport(stdout, options, infrastructure, summary)
	if rebuildErr != nil {
		return rebuildErr
	}
	if options.Verify && !infrastructure.Ready() {
		return errors.New("platform category search infrastructure verification failed")
	}
	return nil
}

func parsePlatformCategorySearchRebuildCLIOptions(args []string, stderr io.Writer) (platformCategorySearchRebuildCLIOptions, error) {
	flags := flag.NewFlagSet("platform-category-search-rebuild", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options platformCategorySearchRebuildCLIOptions
	var categoryID uint64
	flags.BoolVar(&options.Apply, "apply", false, "write the audited search index changes")
	flags.BoolVar(&options.DryRun, "dry-run", false, "show changes without writing (default)")
	flags.BoolVar(&options.Verify, "verify", false, "fail when the resulting index contract is invalid")
	flags.IntVar(&options.BatchSize, "batch-size", repository.PlatformCategorySearchRebuildDefaultBatchSize, "bounded category batch size")
	flags.Uint64Var(&categoryID, "category-id", 0, "rebuild one category subtree")
	if err := flags.Parse(args); err != nil {
		return platformCategorySearchRebuildCLIOptions{}, err
	}
	if flags.NArg() != 0 {
		return platformCategorySearchRebuildCLIOptions{}, errors.New("positional arguments are not allowed")
	}
	if options.Apply && options.DryRun {
		return platformCategorySearchRebuildCLIOptions{}, errors.New("--apply and --dry-run are mutually exclusive")
	}
	if options.BatchSize <= 0 || options.BatchSize > repository.PlatformCategorySearchRebuildMaxBatchSize {
		return platformCategorySearchRebuildCLIOptions{}, fmt.Errorf("--batch-size must be between 1 and %d", repository.PlatformCategorySearchRebuildMaxBatchSize)
	}
	if categoryID > uint64(^uint(0)) {
		return platformCategorySearchRebuildCLIOptions{}, errors.New("--category-id exceeds the supported integer range")
	}
	if categoryID > 0 {
		value := uint(categoryID)
		options.CategoryID = &value
	}
	if !options.Apply {
		options.DryRun = true
	}
	return options, nil
}

func writePlatformCategorySearchRebuildReport(
	output io.Writer,
	options platformCategorySearchRebuildCLIOptions,
	infrastructure database.PlatformCategorySearchInfrastructureStatus,
	summary repository.PlatformCategorySearchMaintenanceSummary,
) {
	mode := "dry-run"
	if options.Apply {
		mode = "apply"
	}
	fmt.Fprintf(output, "mode=%s\n", mode)
	fmt.Fprintf(output, "batch_size=%d\n", options.BatchSize)
	if options.CategoryID != nil {
		fmt.Fprintf(output, "category_id=%d\n", *options.CategoryID)
	}
	fmt.Fprintf(output, "pg_trgm=%t\n", infrastructure.TrigramExtension)
	fmt.Fprintf(output, "search_text_gin=%t\n", infrastructure.SearchTextGIN)
	fmt.Fprintf(output, "order_btree=%t\n", infrastructure.OrderBTree)
	fmt.Fprintf(output, "categories=%d\n", summary.Categories)
	fmt.Fprintf(output, "expected_rows=%d\n", summary.ExpectedRows)
	fmt.Fprintf(output, "created=%d\n", summary.Created)
	fmt.Fprintf(output, "updated=%d\n", summary.Updated)
	fmt.Fprintf(output, "unchanged=%d\n", summary.Unchanged)
	fmt.Fprintf(output, "deleted_orphans=%d\n", summary.DeletedOrphans)
	fmt.Fprintf(output, "missing_translations=%d\n", summary.MissingTranslations)
	fmt.Fprintf(output, "fallback_zh_rows=%d\n", summary.FallbackZhRows)
	fmt.Fprintf(output, "invalid_rows=%d\n", summary.InvalidRows)
	fmt.Fprintf(output, "verified=%t\n", summary.Verified)
}
