package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"saas/internal/config"
	"saas/internal/pkg/database"
	"saas/internal/service"
)

const defaultHolidayImportCountry = "US"

type holidayImportCLIOptions struct {
	FilePath    string
	CountryCode string
	Apply       bool
	DryRun      bool
	Overwrite   bool
}

func main() {
	log.SetFlags(0)
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Printf("holiday import failed: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	options, err := parseHolidayImportCLIOptions(args, stderr)
	if err != nil {
		return err
	}
	data, err := readHolidayImportFile(options.FilePath)
	if err != nil {
		return fmt.Errorf("read holiday file: %w", err)
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

	report, importErr := service.NewHolidayImportService(db).ImportJSON(ctx, data, service.HolidayImportOptions{
		CountryCode: options.CountryCode,
		Apply:       options.Apply,
		Overwrite:   options.Overwrite,
	})
	writeHolidayImportReport(stdout, report)
	return importErr
}

func parseHolidayImportCLIOptions(args []string, stderr io.Writer) (holidayImportCLIOptions, error) {
	flags := flag.NewFlagSet("holiday-import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options holidayImportCLIOptions
	flags.StringVar(&options.FilePath, "file", "", "path to a holiday JSON file")
	flags.StringVar(&options.CountryCode, "country", defaultHolidayImportCountry, "expected ISO 3166-1 alpha-2 country code")
	flags.BoolVar(&options.Apply, "apply", false, "apply the validated diff in one transaction")
	flags.BoolVar(&options.DryRun, "dry-run", false, "show the diff without changing the database (default)")
	flags.BoolVar(&options.Overwrite, "overwrite", false, "replace differing existing rows (requires --apply, or preview with --dry-run)")
	if err := flags.Parse(args); err != nil {
		return holidayImportCLIOptions{}, err
	}
	if flags.NArg() != 0 {
		return holidayImportCLIOptions{}, errors.New("positional arguments are not allowed")
	}
	if strings.TrimSpace(options.FilePath) == "" {
		return holidayImportCLIOptions{}, errors.New("--file is required")
	}
	if options.Apply && options.DryRun {
		return holidayImportCLIOptions{}, errors.New("--apply and --dry-run are mutually exclusive")
	}
	if options.Overwrite && !options.Apply && !options.DryRun {
		return holidayImportCLIOptions{}, errors.New("--overwrite requires an explicit --apply or --dry-run mode")
	}
	return options, nil
}

func readHolidayImportFile(path string) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, errors.New("holiday import source must be a regular local file (symlinks are not allowed)")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("holiday import source must be a regular local file")
	}
	if !os.SameFile(pathInfo, info) {
		return nil, errors.New("holiday import source changed while it was being opened")
	}
	if info.Size() > service.MaxHolidayImportFileBytes {
		return nil, fmt.Errorf("holiday import file exceeds %d bytes", service.MaxHolidayImportFileBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, service.MaxHolidayImportFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > service.MaxHolidayImportFileBytes {
		return nil, fmt.Errorf("holiday import file exceeds %d bytes", service.MaxHolidayImportFileBytes)
	}
	return data, nil
}

func writeHolidayImportReport(output io.Writer, report service.HolidayImportReport) {
	fmt.Fprintf(output, "mode=%s\n", report.Mode)
	for _, change := range report.Changes {
		fmt.Fprintf(output, "%s row=%d country=%s date=%s name=%q", change.Status, change.Row, change.CountryCode, change.HolidayDate, change.NameEN)
		if len(change.ChangedFields) > 0 {
			fmt.Fprintf(output, " fields=%s", strings.Join(change.ChangedFields, ","))
		}
		if change.Reason != "" {
			fmt.Fprintf(output, " reason=%q", change.Reason)
		}
		fmt.Fprintln(output)
	}
	fmt.Fprintf(output, "created=%d\n", report.Created)
	fmt.Fprintf(output, "updated=%d\n", report.Updated)
	fmt.Fprintf(output, "unchanged=%d\n", report.Unchanged)
	fmt.Fprintf(output, "conflicts=%d\n", report.Conflicts)
	fmt.Fprintf(output, "rejected=%d\n", report.Rejected)
}
