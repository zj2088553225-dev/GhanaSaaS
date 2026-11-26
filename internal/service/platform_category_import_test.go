package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"saas/internal/models"

	"github.com/xuri/excelize/v2"
)

func TestBuildPlatformCategoryImportTemplates(t *testing.T) {
	tests := []struct {
		name    string
		build   func() ([]byte, error)
		headers []string
	}{
		{name: "levels", build: BuildPlatformCategoryImportTemplate, headers: platformCategoryImportHeaders},
		{name: "path", build: BuildPlatformCategoryPathImportTemplate, headers: platformCategoryBilingualPathImportHeaders},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := test.build()
			if err != nil {
				t.Fatalf("build template returned error: %v", err)
			}
			file, err := excelize.OpenReader(bytes.NewReader(content))
			if err != nil {
				t.Fatalf("OpenReader returned error: %v", err)
			}
			defer func() { _ = file.Close() }()
			rows, err := file.GetRows("平台类目导入")
			if err != nil {
				t.Fatalf("GetRows returned error: %v", err)
			}
			if len(rows) < 2 || strings.Join(rows[0], "|") != strings.Join(test.headers, "|") {
				t.Fatalf("headers = %+v, want %+v", rows, test.headers)
			}
			preview, err := NewPlatformCategoryService(newFakePlatformCategoryRepository()).
				PreviewImport(context.Background(), bytes.NewReader(content))
			if err != nil {
				t.Fatalf("PreviewImport generated template returned error: %v", err)
			}
			if preview.Rows != 3 || preview.InvalidRows != 0 {
				t.Fatalf("generated template preview = %+v, want 3 valid sample rows", preview)
			}
		})
	}
}

func TestPlatformCategoryBilingualPathImportCreatesEnglishCategoriesAndZhTranslations(t *testing.T) {
	repo := newEmptyFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, [][]string{
		{"Home Appliances", "家用电器", "100", "active"},
		{"Home Appliances > Kitchen Appliances", "家用电器 > 厨房电器", "110", "active"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || preview.ValidRows != 2 {
		t.Fatalf("preview = %+v, want two valid rows", preview)
	}

	result, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want two created categories", result)
	}

	root := findFakePlatformCategoryByPath(t, repo, "Home Appliances")
	child := findFakePlatformCategoryByPath(t, repo, "Home Appliances/Kitchen Appliances")
	if root.Name != "Home Appliances" || root.ParentID != nil || root.Level != 1 || root.IsLeaf {
		t.Fatalf("root = %+v, want English root with child", root)
	}
	if child.Name != "Kitchen Appliances" || child.ParentID == nil || *child.ParentID != root.ID || child.Level != 2 || !child.IsLeaf {
		t.Fatalf("child = %+v, want English child with parent", child)
	}

	zhCategories, err := service.ListWithLocale(context.Background(), true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale zh returned error: %v", err)
	}
	zhByID := platformCategoryByID(zhCategories)
	if zhByID[root.ID].Name != "家用电器" || zhByID[child.ID].Path != "家用电器/厨房电器" {
		t.Fatalf("localized categories = %+v, want zh names and dynamic zh path", zhByID)
	}
	enCategories, err := service.ListWithLocale(context.Background(), true, "en")
	if err != nil {
		t.Fatalf("ListWithLocale en returned error: %v", err)
	}
	enByID := platformCategoryByID(enCategories)
	if enByID[root.ID].Name != "Home Appliances" || enByID[child.ID].Path != "Home Appliances/Kitchen Appliances" {
		t.Fatalf("en categories = %+v, want main table English", enByID)
	}

	repeated, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("repeated ConfirmImport returned error: %v", err)
	}
	if repeated.Created != 0 || len(repo.categories) != 2 {
		t.Fatalf("repeated result = %+v categories=%d, want no duplicates", repeated, len(repo.categories))
	}

	updatedZh := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, [][]string{
		{"Home Appliances > Kitchen Appliances", "家用电器 > 厨电", "110", "active"},
	})
	if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(updatedZh)); err != nil {
		t.Fatalf("zh update ConfirmImport returned error: %v", err)
	}
	zhCategories, err = service.ListWithLocale(context.Background(), true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale zh after update returned error: %v", err)
	}
	zhByID = platformCategoryByID(zhCategories)
	if zhByID[child.ID].Name != "厨电" || zhByID[child.ID].Path != "家用电器/厨电" {
		t.Fatalf("updated zh child = %+v, want new translation", zhByID[child.ID])
	}
}

func TestPlatformCategoryBilingualPathImportHandlesLargeWorkbook(t *testing.T) {
	repo := newEmptyFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	rows := make([][]string, 0, 2000)
	for index := 1; index <= 2000; index++ {
		rows = append(rows, []string{
			fmt.Sprintf("Bulk Category %04d", index),
			fmt.Sprintf("批量类目%04d", index),
			fmt.Sprintf("%d", index),
			"active",
		})
	}
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, rows)

	result, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Created != 2000 || result.Updated != 0 || len(repo.categories) != 2000 {
		t.Fatalf("result = %+v categories=%d, want 2000 created categories", result, len(repo.categories))
	}
	if translations.bulkCalls == 0 || len(translations.rows) != 2000 {
		t.Fatalf("bulkCalls=%d translation rows=%d, want bulk zh translation save", translations.bulkCalls, len(translations.rows))
	}
	for _, category := range repo.categories {
		if !category.IsLeaf || category.Level != 1 || category.ParentID != nil {
			t.Fatalf("category = %+v, want root leaf category", category)
		}
	}

	repeated, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("repeated ConfirmImport returned error: %v", err)
	}
	if repeated.Created != 0 || repeated.Updated != 0 || repeated.Unchanged != 2000 || len(repo.categories) != 2000 {
		t.Fatalf("repeated result = %+v categories=%d, want no duplicates", repeated, len(repo.categories))
	}
}

func TestFakePlatformCategoryImportTransactionRollsBackEveryWriteStage(t *testing.T) {
	forcedFailure := errors.New("forced fake import failure")
	tests := []struct {
		name      string
		configure func(*fakePlatformCategoryRepository)
	}{
		{
			name: "category write",
			configure: func(repo *fakePlatformCategoryRepository) {
				repo.importCategoryErr = forcedFailure
				repo.importCategoryFailAfter = 2
			},
		},
		{
			name: "translation write",
			configure: func(repo *fakePlatformCategoryRepository) {
				repo.importTranslationErr = forcedFailure
			},
		},
		{
			name: "search index write",
			configure: func(repo *fakePlatformCategoryRepository) {
				repo.importSearchIndexErr = forcedFailure
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newEmptyFakePlatformCategoryRepository()
			translations := &fakeTranslationReader{}
			repo.translations = translations
			tt.configure(repo)
			service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
				Translations:      translations,
				TranslationEditor: translations,
			})
			content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, [][]string{
				{"Root > Child", "根 > 子", "10", "active"},
			})

			if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content)); !errors.Is(err, forcedFailure) {
				t.Fatalf("ConfirmImport error = %v, want forced failure", err)
			}
			if len(repo.categories) != 0 || len(translations.rows) != 0 || len(repo.searchIndexes) != 0 || repo.nextID != 1 || translations.bulkCalls != 0 {
				t.Fatalf(
					"fake transaction leaked state: categories=%d translations=%d indexes=%d nextID=%d bulkCalls=%d",
					len(repo.categories), len(translations.rows), len(repo.searchIndexes), repo.nextID, translations.bulkCalls,
				)
			}
		})
	}
}

func TestPlatformCategoryImportRejectsSelfAndDescendantParentsWithoutChanges(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "self parent", path: "食品饮料 > 食品饮料"},
		{name: "descendant parent", path: "食品饮料 > 饮料 > 食品饮料"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakePlatformCategoryRepository()
			beforeCategories := cloneFakePlatformCategories(repo.categories)
			beforeIndexes := cloneFakePlatformCategorySearchIndexes(repo.searchIndexes)
			service := NewPlatformCategoryService(repo)
			content := platformCategoryImportWorkbook(t, platformCategoryBilingualExportHeaders, [][]string{
				{"1", tt.path, "", "10", "active"},
			})

			_, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
			if !errors.Is(err, ErrPlatformCategoryImportCycle) || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ConfirmImport error = %v, want cycle validation error", err)
			}
			if !reflect.DeepEqual(repo.categories, beforeCategories) || !reflect.DeepEqual(repo.searchIndexes, beforeIndexes) || repo.nextID != 4 {
				t.Fatalf("fake repository changed after rejected cycle: %+v", repo.categories)
			}
		})
	}
}

func TestPlatformCategoryBilingualPathImportRejectsMismatchedZhDepth(t *testing.T) {
	service := NewPlatformCategoryService(newEmptyFakePlatformCategoryRepository())
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualPathImportHeaders, [][]string{
		{"Home Appliances > Kitchen Appliances", "家用电器", "110", "active"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 1 || len(preview.Errors) == 0 ||
		!strings.Contains(preview.Errors[0].Message, "层级数量") {
		t.Fatalf("preview errors = %+v, want zh depth error", preview.Errors)
	}
}

func TestPlatformCategoryBilingualImportUsesCategoryIDToMigrateHistoricalChineseMainTable(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	translations := &fakeTranslationReader{}
	repo.translations = translations
	service := NewPlatformCategoryService(repo, PlatformCategoryOptions{
		Translations:      translations,
		TranslationEditor: translations,
	})
	ctx := context.Background()
	content := platformCategoryImportWorkbook(t, platformCategoryBilingualExportHeaders, [][]string{
		{"1", "Food & Beverage", "食品饮料", "100", "active"},
		{"2", "Food & Beverage > Drinks", "食品饮料 > 饮料", "110", "active"},
	})

	result, _, err := service.ConfirmImport(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Created != 0 || result.Updated != 2 {
		t.Fatalf("result = %+v, want in-place updates by Category ID", result)
	}
	root := repo.categories[1]
	child := repo.categories[2]
	if root.Name != "Food & Beverage" || root.Path != "Food & Beverage" || root.ParentID != nil {
		t.Fatalf("migrated root = %+v, want English main table root", root)
	}
	if child.Name != "Drinks" || child.Path != "Food & Beverage/Drinks" || child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("migrated child = %+v, want English path under migrated root", child)
	}
	zhCategories, err := service.ListWithLocale(ctx, true, "zh")
	if err != nil {
		t.Fatalf("ListWithLocale zh returned error: %v", err)
	}
	zhByID := platformCategoryByID(zhCategories)
	if zhByID[root.ID].Name != "食品饮料" || zhByID[child.ID].Path != "食品饮料/饮料" {
		t.Fatalf("zh categories = %+v, want translations preserved from import", zhByID)
	}
	enSearch, err := service.Search(ctx, "Food & Beverage > Drinks", 100, true, "en")
	if err != nil {
		t.Fatalf("English search returned error: %v", err)
	}
	if !platformCategoryResultsContainID(enSearch, child.ID) {
		t.Fatalf("English search results = %+v, want migrated child ID %d", enSearch, child.ID)
	}
	zhSearch, err := service.Search(ctx, "食品饮料 > 饮料", 100, true, "zh")
	if err != nil {
		t.Fatalf("Chinese search returned error: %v", err)
	}
	if !platformCategoryResultsContainID(zhSearch, child.ID) {
		t.Fatalf("Chinese search results = %+v, want migrated child ID %d", zhSearch, child.ID)
	}
}

func TestPlatformCategoryImportPreviewAndConfirm(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	initialCategoryCount := len(repo.categories)
	content := platformCategoryImportWorkbook(t, platformCategoryImportHeaders, [][]string{
		{"服装与配饰", "女装", "连衣裙", "", "", "", "", "10", "enabled"},
		{"食品饮料", "饮料", "碳酸饮料", "", "", "", "", "20", "active"},
		{"日用百货", "", "", "", "", "", "", "8", "disabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.Rows != 3 || preview.ValidRows != 3 || preview.InvalidRows != 0 || len(preview.Errors) != 0 {
		t.Fatalf("preview = %+v, want 3 valid rows", preview)
	}
	if len(preview.PreviewCategories) != 3 {
		t.Fatalf("preview categories = %+v, want one item per source row", preview.PreviewCategories)
	}
	if preview.PreviewCategories[0].Row != 2 || preview.PreviewCategories[0].Path != "服装与配饰/女装/连衣裙" {
		t.Fatalf("first preview item = %+v, want original row and parsed path", preview.PreviewCategories[0])
	}
	if len(repo.categories) != initialCategoryCount {
		t.Fatalf("preview wrote categories: got %d, want %d", len(repo.categories), initialCategoryCount)
	}

	result, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Created != 4 || result.Updated != 1 {
		t.Fatalf("result = %+v, want 4 created and 1 updated", result)
	}
	for _, category := range repo.categories {
		if category.Path == "服装与配饰" && category.IsLeaf {
			t.Fatalf("auto-created parent remained leaf: %+v", category)
		}
	}

	repeated, repeatedPreview, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("repeated ConfirmImport returned error: %v", err)
	}
	if repeated.Created != 0 || repeated.Updated != 0 || repeated.Unchanged != 3 {
		t.Fatalf("repeated result = %+v, want no duplicate creation", repeated)
	}
	for _, item := range repeatedPreview.PreviewCategories {
		if item.Action != "skip" {
			t.Fatalf("repeated preview item = %+v, want skip", item)
		}
	}
}

func TestPlatformCategoryImportSupportsPathFormatAndVariableDepth(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, [][]string{
		{"家用电器/大家电配件/吊扇配件/吊扇遥控/射频模块/控制板/芯片/版本A/子型号", "10", "enabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.PreviewCategories) != 1 {
		t.Fatalf("preview = %+v, want one valid path row", preview)
	}
	item := preview.PreviewCategories[0]
	if item.Level != 9 || item.Action != "create" || item.Row != 2 {
		t.Fatalf("preview item = %+v, want variable 9-level path", item)
	}

	result, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Created != 9 {
		t.Fatalf("result = %+v, want all 9 missing path nodes created", result)
	}
}

func TestPlatformCategoryImportSupportsLegacyHeadersAndIgnoresIDs(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	headers := []string{
		"类目ID", "类目名称", "层级", "父类目ID",
		"主类目", "1级类目", "2级类目", "3级类目",
		"4级类目", "5级类目", "6级类目", "7级类目",
	}
	content := platformCategoryImportWorkbook(t, headers, [][]string{
		{"999999", "错误的类目名称", "99", "888888", "电子产品", "配件", "线材", "数据线", "USB", "Type-C", "编织线", "两米"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.PreviewCategories) != 1 {
		t.Fatalf("preview = %+v, want compatible legacy row", preview)
	}
	item := preview.PreviewCategories[0]
	wantPath := "电子产品/配件/线材/数据线/USB/Type-C/编织线/两米"
	if item.Path != wantPath || item.Level != 8 || item.SortOrder != 0 || item.Status != "active" {
		t.Fatalf("preview item = %+v, want legacy mapping to %q", item, wantPath)
	}

	if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	for _, category := range repo.categories {
		if category.ID == 999999 || (category.ParentID != nil && *category.ParentID == 888888) {
			t.Fatalf("import reused source IDs: %+v", category)
		}
	}
}

func TestPlatformCategoryImportNormalizesSlashesInLevelColumns(t *testing.T) {
	service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
	content := platformCategoryImportWorkbook(t, platformCategoryImportHeaders, [][]string{
		{"个护电器", "卷/直发器", "PS/2线", "CD/DVD架", "I/O端口扩展卡", "", "", "10", "enabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.Errors) != 0 || len(preview.PreviewCategories) != 1 {
		t.Fatalf("preview = %+v, want one valid normalized row", preview)
	}
	item := preview.PreviewCategories[0]
	wantPath := "个护电器/卷／直发器/PS／2线/CD／DVD架/I／O端口扩展卡"
	if item.Path != wantPath {
		t.Fatalf("path = %q, want %q", item.Path, wantPath)
	}
	if !item.Normalized || !strings.Contains(item.NormalizationNote, "已自动规范化") {
		t.Fatalf("normalization preview = %+v, want normalization marker", item)
	}

	if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
}

func TestPlatformCategoryImportNormalizesSlashesInLegacyLevelColumns(t *testing.T) {
	service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
	headers := []string{"主类目", "1级类目", "2级类目", "3级类目"}
	content := platformCategoryImportWorkbook(t, headers, [][]string{
		{"电子电气", "电脑配件", "I/O端口扩展卡", "PS/2线"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.PreviewCategories) != 1 {
		t.Fatalf("preview = %+v, want one valid legacy row", preview)
	}
	item := preview.PreviewCategories[0]
	if item.Path != "电子电气/电脑配件/I／O端口扩展卡/PS／2线" || !item.Normalized {
		t.Fatalf("legacy normalized item = %+v", item)
	}
}

func TestPlatformCategoryImportPathFormatKeepsSlashAsSeparator(t *testing.T) {
	service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, [][]string{
		{"个护电器/卷／直发器/CD／DVD架", "10", "enabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.PreviewCategories) != 1 {
		t.Fatalf("preview = %+v, want one valid path row", preview)
	}
	item := preview.PreviewCategories[0]
	if item.Level != 3 || item.Path != "个护电器/卷／直发器/CD／DVD架" || item.Normalized {
		t.Fatalf("path item = %+v, want ASCII slash separators and preserved full-width slashes", item)
	}
}

func TestPlatformCategoryImportRejectsBackslashes(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		row     []string
	}{
		{
			name:    "path format",
			headers: platformCategoryPathImportHeaders,
			row:     []string{`电子电气\电脑配件`, "0", "enabled"},
		},
		{
			name:    "level columns",
			headers: platformCategoryImportHeaders,
			row:     []string{"电子电气", `I\O端口扩展卡`, "", "", "", "", "", "0", "enabled"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
			content := platformCategoryImportWorkbook(t, test.headers, [][]string{test.row})
			preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
			if err != nil {
				t.Fatalf("PreviewImport returned error: %v", err)
			}
			if preview.InvalidRows != 1 || len(preview.Errors) == 0 ||
				!strings.Contains(preview.Errors[0].Message, "反斜杠") {
				t.Fatalf("preview errors = %+v, want backslash error", preview.Errors)
			}
		})
	}
}

func TestPlatformCategoryImportAcceptsNineThousandSixHundredSixtyFiveRowsWithSlashNames(t *testing.T) {
	const (
		totalRows      = 9665
		normalizedRows = 47
	)
	slashNames := []string{"卷/直发器", "PS/2线", "CD/DVD架", "I/O端口扩展卡"}
	dataRows := make([][]string, totalRows)
	for index := range dataRows {
		name := fmt.Sprintf("普通类目%04d", index+1)
		if index < normalizedRows {
			name = fmt.Sprintf("%s-%02d", slashNames[index%len(slashNames)], index+1)
		}
		dataRows[index] = []string{"测试主类目", name, "", "", "", "", "", "0", "enabled"}
	}
	content := platformCategoryImportWorkbook(t, platformCategoryImportHeaders, dataRows)

	preview, err := NewPlatformCategoryService(newFakePlatformCategoryRepository()).
		PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.Rows != totalRows || preview.ValidRows != totalRows || preview.InvalidRows != 0 || len(preview.Errors) != 0 {
		t.Fatalf(
			"preview counts = rows %d, valid %d, invalid %d, errors %d; want %d valid rows",
			preview.Rows,
			preview.ValidRows,
			preview.InvalidRows,
			len(preview.Errors),
			totalRows,
		)
	}
	actualNormalizedRows := 0
	for _, item := range preview.PreviewCategories {
		if item.Normalized {
			actualNormalizedRows++
		}
	}
	if actualNormalizedRows != normalizedRows {
		t.Fatalf("normalized rows = %d, want %d", actualNormalizedRows, normalizedRows)
	}
}

func TestPlatformCategoryImportDuplicatePathUsesLastRowAndSkipsEarlierRow(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	service := NewPlatformCategoryService(repo)
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, [][]string{
		{"数码/手机/智能手机", "10", "enabled"},
		{"数码/手机/智能手机", "30", "disabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 0 || len(preview.PreviewCategories) != 2 {
		t.Fatalf("preview = %+v, want two valid source rows", preview)
	}
	if preview.PreviewCategories[0].Action != "skip" || preview.PreviewCategories[1].Action != "create" {
		t.Fatalf("actions = %+v, want earlier duplicate skipped and last row applied", preview.PreviewCategories)
	}

	if _, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content)); err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	for _, category := range repo.categories {
		if category.Path == "数码/手机/智能手机" && (category.SortOrder != 30 || category.Status != "disabled") {
			t.Fatalf("terminal category = %+v, want values from last duplicate row", category)
		}
	}
}

func TestPlatformCategoryImportRepairsExistingParentAndLevel(t *testing.T) {
	repo := newFakePlatformCategoryRepository()
	corrupted := repo.categories[2]
	corrupted.ParentID = nil
	corrupted.Level = 99
	repo.categories[2] = corrupted
	service := NewPlatformCategoryService(repo)
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, [][]string{
		{"食品饮料/饮料", "1", "enabled"},
	})

	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if len(preview.PreviewCategories) != 1 || preview.PreviewCategories[0].Action != "update" {
		t.Fatalf("preview = %+v, want structural update", preview)
	}
	result, _, err := service.ConfirmImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("result = %+v, want one updated terminal category", result)
	}
	repaired := repo.categories[2]
	if repaired.ParentID == nil || *repaired.ParentID != 1 || repaired.Level != 2 {
		t.Fatalf("repaired category = %+v, want parent 1 and level 2", repaired)
	}
}

func TestPlatformCategoryImportPreviewReportsRowErrors(t *testing.T) {
	service := NewPlatformCategoryService(newFakePlatformCategoryRepository())
	content := platformCategoryImportWorkbook(t, platformCategoryImportHeaders, [][]string{
		{"食品饮料", "", "碳酸饮料", "", "", "", "", "-1", "unknown"},
	})
	preview, err := service.PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 1 || len(preview.Errors) < 3 {
		t.Fatalf("preview errors = %+v, want row-specific validation errors", preview.Errors)
	}
	if len(preview.PreviewCategories) != 1 || preview.PreviewCategories[0].Row != 2 ||
		preview.PreviewCategories[0].Action != "skip" || preview.PreviewCategories[0].Error == "" {
		t.Fatalf("preview categories = %+v, want invalid row with error and skip action", preview.PreviewCategories)
	}
}

func TestPlatformCategoryImportRejectsMoreThanTenThousandRows(t *testing.T) {
	if PlatformCategoryImportMaxRows != 10000 {
		t.Fatalf("PlatformCategoryImportMaxRows = %d, want 10000", PlatformCategoryImportMaxRows)
	}
	dataRows := make([][]string, PlatformCategoryImportMaxRows+1)
	for index := range dataRows {
		dataRows[index] = []string{fmt.Sprintf("批量类目/%05d", index+1), "", "enabled"}
	}
	content := platformCategoryImportWorkbook(t, platformCategoryPathImportHeaders, dataRows)
	preview, err := NewPlatformCategoryService(newFakePlatformCategoryRepository()).
		PreviewImport(context.Background(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.Rows != PlatformCategoryImportMaxRows+1 ||
		preview.ValidRows != PlatformCategoryImportMaxRows ||
		preview.InvalidRows != 1 {
		t.Fatalf("preview counts = rows %d, valid %d, invalid %d", preview.Rows, preview.ValidRows, preview.InvalidRows)
	}
	if len(preview.Errors) != 1 {
		t.Fatalf("preview errors = %+v, want one row-limit error", preview.Errors)
	}
	rowError := preview.Errors[0]
	if rowError.Row != PlatformCategoryImportMaxRows+2 ||
		rowError.Message != PlatformCategoryImportTooManyRowsMessageZH ||
		rowError.MessageEN != PlatformCategoryImportTooManyRowsMessageEN {
		t.Fatalf("row-limit error = %+v", rowError)
	}
}

func platformCategoryImportWorkbook(t *testing.T, headers []string, dataRows [][]string) []byte {
	t.Helper()
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	for columnIndex, header := range headers {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			t.Fatal(err)
		}
	}
	for rowIndex, row := range dataRows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func newEmptyFakePlatformCategoryRepository() *fakePlatformCategoryRepository {
	return &fakePlatformCategoryRepository{
		nextID:        1,
		categories:    make(map[uint]models.PlatformCategory),
		productRefs:   make(map[uint]int64),
		searchIndexes: make(map[string]models.PlatformCategorySearchIndex),
	}
}

func findFakePlatformCategoryByPath(t *testing.T, repo *fakePlatformCategoryRepository, path string) models.PlatformCategory {
	t.Helper()
	for _, category := range repo.categories {
		if category.Path == path {
			return category
		}
	}
	t.Fatalf("category path %q not found in %+v", path, repo.categories)
	return models.PlatformCategory{}
}

func platformCategoryByID(categories []models.PlatformCategory) map[uint]models.PlatformCategory {
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	return byID
}
