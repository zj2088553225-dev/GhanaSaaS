package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"saas/internal/service"
)

func main() {
	root := flag.String("root", "uploads/images", "image upload root to scan")
	write := flag.Bool("write", false, "write generated WebP variants; dry-run by default")
	printSQL := flag.Bool("sql", true, "print suggested SQL updates for product image columns")
	flag.Parse()

	stats := optimizerStats{}
	err := filepath.WalkDir(*root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !service.IsOptimizableImageFilename(entry.Name()) {
			return nil
		}
		if existingVariantSet(path) {
			stats.skippedExisting++
			return nil
		}
		if err := optimizeOne(path, *write, *printSQL); err != nil {
			stats.failed++
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", path, err)
			return nil
		}
		stats.processed++
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	mode := "dry-run"
	if *write {
		mode = "write"
	}
	fmt.Printf(
		"image optimizer finished mode=%s processed=%d skipped_existing=%d failed=%d\n",
		mode,
		stats.processed,
		stats.skippedExisting,
		stats.failed,
	)
}

type optimizerStats struct {
	processed       int
	skippedExisting int
	failed          int
}

func optimizeOne(path string, write bool, printSQL bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	optimized, err := service.OptimizeImageBytes(data, filepath.Base(path))
	if err != nil {
		return err
	}

	targets := imageTargets(path, optimized)
	targetNames := sortedTargetNames(targets)
	action := "would write"
	if write {
		action = "wrote"
		for _, name := range targetNames {
			if err := os.WriteFile(targets[name], targetBytes(name, optimized), 0o644); err != nil {
				return err
			}
		}
	}

	fmt.Printf("%s %s\n", action, path)
	for _, name := range targetNames {
		fmt.Printf("  %s: %s\n", name, targets[name])
	}
	if printSQL {
		oldURL := publicUploadURL(path)
		largeURL := publicUploadURL(targets[service.ImageVariantLarge])
		fmt.Printf("  suggested SQL:\n")
		fmt.Printf("    UPDATE spus SET main_image = '%s' WHERE main_image = '%s';\n", sqlString(largeURL), sqlString(oldURL))
		fmt.Printf("    UPDATE skus SET image_url = '%s' WHERE image_url = '%s';\n", sqlString(largeURL), sqlString(oldURL))
		fmt.Printf("    CMS/site-page JSON image fields should be reviewed manually for %s\n", oldURL)
	}
	return nil
}

func imageTargets(path string, optimized *service.OptimizedImage) map[string]string {
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	return map[string]string{
		service.ImageVariantBase:     base + ".webp",
		service.ImageVariantThumb:    base + "_thumb.webp",
		service.ImageVariantCard:     base + "_card.webp",
		service.ImageVariantLarge:    base + "_large.webp",
		service.ImageVariantOriginal: base + "_original" + optimized.OriginalExtension,
	}
}

func targetBytes(name string, optimized *service.OptimizedImage) []byte {
	if name == service.ImageVariantOriginal {
		return optimized.Original
	}
	return optimized.Files[name]
}

func sortedTargetNames(targets map[string]string) []string {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func existingVariantSet(path string) bool {
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	for _, suffix := range []string{".webp", "_thumb.webp", "_card.webp", "_large.webp"} {
		if _, err := os.Stat(base + suffix); err != nil {
			return false
		}
	}
	return true
}

func publicUploadURL(path string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for index, part := range parts {
		if part == "uploads" {
			return "/" + strings.Join(parts[index:], "/")
		}
	}
	return "/uploads/images/" + filepath.Base(path)
}

func sqlString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
