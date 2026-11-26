package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"saas/internal/service"
)

func TestHolidayImportCLIIsDryRunByDefault(t *testing.T) {
	options, err := parseHolidayImportCLIOptions([]string{"--file", "/tmp/holidays.json", "--country", "US"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if options.Apply || options.DryRun || options.FilePath != "/tmp/holidays.json" || options.CountryCode != "US" {
		t.Fatalf("default options = %+v", options)
	}
}

func TestHolidayImportCLIFileReaderIsBoundedAndLocal(t *testing.T) {
	directory := t.TempDir()
	oversized := filepath.Join(directory, "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), service.MaxHolidayImportFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHolidayImportFile(oversized); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized file error = %v", err)
	}
	regular := filepath.Join(directory, "regular.json")
	if err := os.WriteFile(regular, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "link.json")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readHolidayImportFile(symlink); err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("symlink file error = %v", err)
	}
}

func TestHolidayImportCLIRequiresExplicitSingleMode(t *testing.T) {
	if _, err := parseHolidayImportCLIOptions([]string{"--country", "US"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("missing file error = %v", err)
	}
	if _, err := parseHolidayImportCLIOptions([]string{"--file", "data.json", "--dry-run", "--apply"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("conflicting mode error = %v", err)
	}
	if _, err := parseHolidayImportCLIOptions([]string{"--file", "data.json", "--overwrite"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("implicit overwrite error = %v", err)
	}
	options, err := parseHolidayImportCLIOptions([]string{"--file", "data.json", "--apply", "--overwrite"}, &bytes.Buffer{})
	if err != nil || !options.Apply || !options.Overwrite {
		t.Fatalf("explicit overwrite options = %+v/%v", options, err)
	}
}

func TestHolidayImportCLIReportIncludesDiffAndCounts(t *testing.T) {
	var output bytes.Buffer
	writeHolidayImportReport(&output, service.HolidayImportReport{
		Mode: service.HolidayImportModeDryRun, Created: 1, Updated: 2, Unchanged: 3, Conflicts: 4, Rejected: 0,
		Changes: []service.HolidayImportChange{{
			Row: 1, CountryCode: "US", HolidayDate: "2026-01-01", NameEN: "New Year’s Day", Status: service.HolidayImportStatusCreated,
		}},
	})
	for _, expected := range []string{"mode=dry-run", "created row=1", "created=1", "updated=2", "unchanged=3", "conflicts=4", "rejected=0"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("report %q does not contain %q", output.String(), expected)
		}
	}
}
