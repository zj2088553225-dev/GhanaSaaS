package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/xuri/excelize/v2"
)

const (
	PlatformCategoryImportMaxRows = 10000

	platformCategoryTemplateLevelCount = 7
	platformCategoryMaxNameLength      = 120
	platformCategoryMaxPathLength      = 500
)

const (
	PlatformCategoryImportTooManyRowsMessageZH = "类目导入最多支持 10000 行，请拆分文件后再导入。"
	PlatformCategoryImportTooManyRowsMessageEN = "Category import supports up to 10,000 rows. Please split the file and try again."
)

var (
	platformCategoryLevelHeaders = []string{
		"一级类目", "二级类目", "三级类目", "四级类目",
		"五级类目", "六级类目", "七级类目", "八级类目",
	}
	platformCategoryLegacyLevelHeaders = []string{
		"主类目", "1级类目", "2级类目", "3级类目",
		"4级类目", "5级类目", "6级类目", "7级类目",
	}
	platformCategoryImportHeaders = append(
		append([]string(nil), platformCategoryLevelHeaders[:platformCategoryTemplateLevelCount]...),
		"排序",
		"状态",
	)
	platformCategoryPathImportHeaders          = []string{"类目路径", "排序", "状态"}
	platformCategoryBilingualPathImportHeaders = []string{"English Path", "中文路径", "Sort Order", "Status"}
	platformCategoryBilingualExportHeaders     = []string{"Category ID", "English Path", "中文路径", "Sort Order", "Status"}
)

type PlatformCategoryImportPreview struct {
	Rows              int                              `json:"rows"`
	ValidRows         int                              `json:"valid_rows"`
	InvalidRows       int                              `json:"invalid_rows"`
	Errors            []PlatformCategoryImportRowError `json:"errors"`
	PreviewCategories []PlatformCategoryImportItem     `json:"preview_categories"`
	operations        []repository.PlatformCategoryImportOperation
}

type PlatformCategoryImportRowError struct {
	Row       int    `json:"row"`
	Field     string `json:"field"`
	Message   string `json:"message"`
	MessageEN string `json:"message_en,omitempty"`
}

type PlatformCategoryImportItem struct {
	Row               int                           `json:"row"`
	Path              string                        `json:"path"`
	Level             int                           `json:"level"`
	SortOrder         int                           `json:"sort_order"`
	Status            models.PlatformCategoryStatus `json:"status"`
	Action            string                        `json:"action"`
	Normalized        bool                          `json:"normalized"`
	NormalizationNote string                        `json:"normalization_note,omitempty"`
	Error             string                        `json:"error,omitempty"`
}

type PlatformCategoryImportResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

type platformCategoryImportRow struct {
	ExcelRow         int
	ID               *uint
	Names            []string
	ZhNames          []string
	ZhPathProvided   bool
	Sort             int
	Status           models.PlatformCategoryStatus
	NormalizedFields []string
}

type platformCategoryImportLevelColumn struct {
	Header string
	Index  int
}

type platformCategoryImportColumns struct {
	IDIndex         int
	PathIndex       int
	ZhPathIndex     int
	Levels          []platformCategoryImportLevelColumn
	SortIndex       int
	StatusIndex     int
	PathSeparator   string
	PathFieldName   string
	ZhPathFieldName string
}

func BuildPlatformCategoryImportTemplate() ([]byte, error) {
	samples := [][]any{
		{"家用电器", "大家电配件", "吊扇配件", "吊扇遥控", "", "", "", 10, "enabled"},
		{"服装与配饰", "女装", "连衣裙", "", "", "", "", 20, "enabled"},
		{"食品饮料", "饮料", "碳酸饮料", "", "", "", "", 30, "disabled"},
	}
	notes := [][]string{
		{"说明", "内容"},
		{"导入限制", "Excel 文件最大 10MB，最多支持 10000 行类目数据。"},
		{"类目层级", "默认模板支持一级至七级；导入器同时兼容可选的八级类目列。父级不存在时会按完整路径自动创建。"},
		{"排序", "填写非负整数；为空默认 0。排序和状态仅应用到该行最后一个非空类目。"},
		{"状态", "支持 enabled、active、启用、disabled、停用；为空默认 enabled。"},
		{"重复导入", "同级同名和同一完整路径不会重复创建；已存在路径会更新排序和状态，无变化时跳过。"},
		{"名称规范化", "多级列版中，类目名称内的英文 / 会自动转换为全角 ／；反斜杠 \\ 仍不允许。"},
		{"旧表兼容", "主类目映射为一级类目，1级类目至7级类目依次映射为二级类目至八级类目。"},
		{"忽略字段", "类目ID、父类目ID、层级、类目名称会被忽略；id、parent_id、path、level、is_leaf 均由系统维护。"},
	}
	return buildPlatformCategoryImportTemplate(platformCategoryImportHeaders, samples, notes)
}

func BuildPlatformCategoryPathImportTemplate() ([]byte, error) {
	samples := [][]any{
		{"Home Appliances", "家用电器", 100, "active"},
		{"Home Appliances > Kitchen Appliances", "家用电器 > 厨房电器", 110, "active"},
		{"Electronics > Mobile Accessories", "电子产品 > 手机配件", 210, "active"},
	}
	notes := [][]string{
		{"说明", "内容"},
		{"导入限制", "Excel 文件最大 10MB，最多支持 10000 行类目数据。"},
		{"English Path", "必填，使用 > 分隔层级，例如 Home Appliances > Kitchen Appliances。主表 name/path 写入英文。"},
		{"中文路径", "可选，使用 > 分隔层级；填写时层级数量必须与 English Path 一致。中文名称写入 content_translations.zh。"},
		{"Sort Order", "填写非负整数；为空默认 0。排序仅应用到该行最后一级类目。"},
		{"Status", "支持 active、enabled、disabled；为空默认 active。"},
		{"重复导入", "同级同名和同一完整路径不会重复创建；已存在路径会更新排序、状态和中文翻译。"},
		{"历史类目迁移", "从导出文件再次导入时，系统会读取可选 Category ID 并原地更新历史中文主表类目的英文 name/path。"},
		{"系统字段", "parent_id、level、is_leaf 由系统根据 English Path 自动维护；不会直接保存中文 path。"},
	}
	return buildPlatformCategoryImportTemplate(platformCategoryBilingualPathImportHeaders, samples, notes)
}

func (s *PlatformCategoryService) ExportBilingualImportWorkbook(ctx context.Context) ([]byte, error) {
	categories, err := s.repo.List(ctx, false)
	if err != nil {
		return nil, err
	}
	translations := map[uint]map[string]models.ContentTranslation{}
	if s.translations != nil && len(categories) > 0 {
		ids := make([]uint, 0, len(categories))
		for _, category := range categories {
			ids = append(ids, category.ID)
		}
		translations, err = s.translations.GetTranslations(ctx, TranslationEntityPlatformCategory, ids, "zh")
		if err != nil {
			return nil, err
		}
	}
	zhPathByID := platformCategoryExportZhPaths(categories, translations)

	file := excelize.NewFile()
	const sheet = "平台类目双语"
	defaultSheet := file.GetSheetName(0)
	index, err := file.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	if defaultSheet != "" {
		if err := file.DeleteSheet(defaultSheet); err != nil {
			return nil, err
		}
	}
	file.SetActiveSheet(index)
	for columnIndex, header := range platformCategoryBilingualExportHeaders {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
	}
	for rowIndex, category := range categories {
		values := []any{
			category.ID,
			strings.ReplaceAll(category.Path, "/", " > "),
			zhPathByID[category.ID],
			category.SortOrder,
			string(category.Status),
		}
		for columnIndex, value := range values {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"DBEAFE"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheet, "A1", "E1", headerStyle); err != nil {
		return nil, err
	}
	_ = file.SetColWidth(sheet, "A", "A", 12)
	_ = file.SetColWidth(sheet, "B", "C", 48)
	_ = file.SetColWidth(sheet, "D", "E", 14)

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func platformCategoryExportZhPaths(
	categories []models.PlatformCategory,
	translations map[uint]map[string]models.ContentTranslation,
) map[uint]string {
	byID := make(map[uint]models.PlatformCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	memo := make(map[uint]string, len(categories))
	var build func(id uint, visiting map[uint]struct{}) string
	build = func(id uint, visiting map[uint]struct{}) string {
		if path, ok := memo[id]; ok {
			return path
		}
		category, ok := byID[id]
		if !ok {
			return ""
		}
		if _, ok := visiting[id]; ok {
			return ""
		}
		visiting[id] = struct{}{}
		name := platformCategoryExportZhName(category, translations[category.ID])
		if name == "" {
			delete(visiting, id)
			memo[id] = ""
			return ""
		}
		path := name
		if category.ParentID != nil {
			parentPath := build(*category.ParentID, visiting)
			if parentPath == "" {
				delete(visiting, id)
				memo[id] = ""
				return ""
			}
			path = parentPath + " > " + name
		}
		delete(visiting, id)
		memo[id] = path
		return path
	}
	paths := make(map[uint]string, len(categories))
	for _, category := range categories {
		paths[category.ID] = build(category.ID, map[uint]struct{}{})
	}
	return paths
}

func platformCategoryExportZhName(category models.PlatformCategory, translations map[string]models.ContentTranslation) string {
	value := translatedText("", translations, TranslationFieldName)
	if value != "" {
		return value
	}
	if containsHan(category.Name) {
		return strings.TrimSpace(category.Name)
	}
	return ""
}

func containsHan(value string) bool {
	for _, char := range value {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}

func buildPlatformCategoryImportTemplate(headers []string, samples [][]any, notes [][]string) ([]byte, error) {
	file := excelize.NewFile()
	const sheet = "平台类目导入"
	defaultSheet := file.GetSheetName(0)
	index, err := file.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	if defaultSheet != "" {
		if err := file.DeleteSheet(defaultSheet); err != nil {
			return nil, err
		}
	}
	file.SetActiveSheet(index)

	for columnIndex, header := range headers {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
	}
	for rowIndex, row := range samples {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}

	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FED7AA"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}
	lastHeaderCell, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheet, "A1", lastHeaderCell, headerStyle); err != nil {
		return nil, err
	}
	if strings.Join(headers, "|") == strings.Join(platformCategoryBilingualPathImportHeaders, "|") {
		_ = file.SetColWidth(sheet, "A", "B", 42)
		_ = file.SetColWidth(sheet, "C", "D", 14)
	} else if strings.Join(headers, "|") == strings.Join(platformCategoryPathImportHeaders, "|") {
		_ = file.SetColWidth(sheet, "A", "A", 56)
		_ = file.SetColWidth(sheet, "B", "C", 14)
	} else {
		_ = file.SetColWidth(sheet, "A", "G", 22)
		_ = file.SetColWidth(sheet, "H", "I", 14)
	}

	const instructions = "说明"
	if _, err := file.NewSheet(instructions); err != nil {
		return nil, err
	}
	for rowIndex, row := range notes {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(instructions, cell, value); err != nil {
				return nil, err
			}
		}
	}
	if err := file.SetCellStyle(instructions, "A1", "B1", headerStyle); err != nil {
		return nil, err
	}
	_ = file.SetColWidth(instructions, "A", "A", 16)
	_ = file.SetColWidth(instructions, "B", "B", 100)

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *PlatformCategoryService) PreviewImport(ctx context.Context, reader io.Reader) (*PlatformCategoryImportPreview, error) {
	return s.parseImport(ctx, reader)
}

func (s *PlatformCategoryService) ConfirmImport(ctx context.Context, reader io.Reader) (*PlatformCategoryImportResult, *PlatformCategoryImportPreview, error) {
	preview, err := s.parseImport(ctx, reader)
	if err != nil {
		return nil, nil, err
	}
	if preview.InvalidRows > 0 || len(preview.Errors) > 0 {
		return nil, preview, ErrInvalidInput
	}
	result, err := s.repo.Import(ctx, preview.operations)
	if err != nil {
		if errors.Is(err, repository.ErrPlatformCategoryImportCycle) {
			return nil, preview, ErrPlatformCategoryImportCycle
		}
		return nil, preview, err
	}
	return &PlatformCategoryImportResult{
		Created:   result.Created,
		Updated:   result.Updated,
		Unchanged: result.Unchanged,
	}, preview, nil
}

func (s *PlatformCategoryService) parseImport(ctx context.Context, reader io.Reader) (*PlatformCategoryImportPreview, error) {
	file, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, ErrInvalidInput
	}
	defer func() { _ = file.Close() }()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return platformCategoryPreviewWithError(1, "表头", "Excel 文件缺少工作表"), nil
	}
	rows, err := file.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return platformCategoryPreviewWithError(1, "表头", "Excel 表头缺失或不匹配，请使用标准模板"), nil
	}
	columns, ok := platformCategoryImportColumnIndexes(rows[0])
	if !ok {
		return platformCategoryPreviewWithError(
			1,
			"表头",
			"未找到类目路径、一级类目或主类目，请使用路径版、多级列版或兼容旧表字段",
		), nil
	}

	existing, err := s.repo.List(ctx, false)
	if err != nil {
		return nil, err
	}
	existingByPath := make(map[string]models.PlatformCategory, len(existing))
	existingByID := make(map[uint]models.PlatformCategory, len(existing))
	existingIDs := make([]uint, 0, len(existing))
	for _, category := range existing {
		existingByPath[normalizeLookupKey(category.Path)] = category
		existingByID[category.ID] = category
		existingIDs = append(existingIDs, category.ID)
	}
	existingTranslations := map[uint]map[string]models.ContentTranslation{}
	if s.translations != nil && len(existingIDs) > 0 {
		existingTranslations, err = s.translations.GetTranslations(ctx, TranslationEntityPlatformCategory, existingIDs, "zh")
		if err != nil {
			return nil, err
		}
	}

	parsedRows := make([]platformCategoryImportRow, 0)
	rowErrors := make(map[int][]PlatformCategoryImportRowError)
	lastRowByPath := make(map[string]int)
	nonEmptyRows := 0
	for rowIndex := 1; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		if rowIsEmpty(row) {
			continue
		}
		nonEmptyRows++
		excelRow := rowIndex + 1
		if nonEmptyRows > PlatformCategoryImportMaxRows {
			rowErrors[excelRow] = append(rowErrors[excelRow], PlatformCategoryImportRowError{
				Row:       excelRow,
				Field:     "行数",
				Message:   PlatformCategoryImportTooManyRowsMessageZH,
				MessageEN: PlatformCategoryImportTooManyRowsMessageEN,
			})
			parsedRows = append(parsedRows, platformCategoryImportRow{
				ExcelRow: excelRow,
				Status:   models.PlatformCategoryStatusActive,
			})
			break
		}
		parsed, errors := parsePlatformCategoryImportRow(excelRow, row, columns)
		parsedRows = append(parsedRows, parsed)
		if len(errors) > 0 {
			rowErrors[excelRow] = append(rowErrors[excelRow], errors...)
			continue
		}
		lastRowByPath[normalizeLookupKey(strings.Join(parsed.Names, "/"))] = excelRow
	}
	if nonEmptyRows == 0 {
		rowErrors[2] = append(rowErrors[2], PlatformCategoryImportRowError{
			Row: 2, Field: "数据", Message: "Excel 中没有可导入的平台类目数据",
		})
	}

	operationByPath := make(map[string]repository.PlatformCategoryImportOperation)
	operationOrder := make([]string, 0)
	for _, row := range parsedRows {
		if len(rowErrors[row.ExcelRow]) > 0 {
			continue
		}
		for index, name := range row.Names {
			path := strings.Join(row.Names[:index+1], "/")
			key := normalizeLookupKey(path)
			parentPath := ""
			if index > 0 {
				parentPath = strings.Join(row.Names[:index], "/")
			}
			operation, exists := operationByPath[key]
			if !exists {
				operation = repository.PlatformCategoryImportOperation{
					Path:       path,
					ParentPath: parentPath,
					Name:       name,
					ZhProvided: row.ZhPathProvided,
					Level:      index + 1,
					Status:     models.PlatformCategoryStatusActive,
				}
				operationOrder = append(operationOrder, key)
			}
			if row.ZhPathProvided && index < len(row.ZhNames) {
				operation.ZhName = row.ZhNames[index]
				operation.ZhProvided = true
			}
			if index == len(row.Names)-1 {
				operation.ID = cloneUintPtr(row.ID)
				operation.SortOrder = row.Sort
				operation.Status = row.Status
				operation.Explicit = true
			}
			operationByPath[key] = operation
		}
	}

	preview := &PlatformCategoryImportPreview{
		Rows:       nonEmptyRows,
		Errors:     flattenPlatformCategoryImportErrors(rowErrors),
		operations: make([]repository.PlatformCategoryImportOperation, 0, len(operationOrder)),
	}
	preview.InvalidRows = countPlatformCategoryInvalidRows(rowErrors)
	preview.ValidRows = preview.Rows - preview.InvalidRows
	if preview.ValidRows < 0 {
		preview.ValidRows = 0
	}

	for _, key := range operationOrder {
		preview.operations = append(preview.operations, operationByPath[key])
	}
	sort.SliceStable(preview.operations, func(i, j int) bool {
		return preview.operations[i].Level < preview.operations[j].Level
	})

	preview.PreviewCategories = make([]PlatformCategoryImportItem, 0, len(parsedRows))
	for _, row := range parsedRows {
		path := strings.Join(row.Names, "/")
		key := normalizeLookupKey(path)
		item := PlatformCategoryImportItem{
			Row:        row.ExcelRow,
			Path:       path,
			Level:      len(row.Names),
			SortOrder:  row.Sort,
			Status:     row.Status,
			Action:     "skip",
			Normalized: len(row.NormalizedFields) > 0,
		}
		if item.Normalized {
			item.NormalizationNote = "已自动规范化：" + strings.Join(row.NormalizedFields, "、") + "中的 / 已转换为 ／"
		}
		if errors := rowErrors[row.ExcelRow]; len(errors) > 0 {
			item.Error = joinPlatformCategoryImportErrors(errors)
		} else if lastRowByPath[key] != row.ExcelRow {
			item.Action = "skip"
		} else if category, exists := platformCategoryImportPreviewExistingCategory(row, key, existingByPath, existingByID); exists {
			if platformCategoryImportNeedsUpdate(category, row, existingByPath, existingByID, existingTranslations) {
				item.Action = "update"
			}
		} else {
			item.Action = "create"
		}
		preview.PreviewCategories = append(preview.PreviewCategories, item)
	}
	return preview, nil
}

func platformCategoryImportNeedsUpdate(
	category models.PlatformCategory,
	row platformCategoryImportRow,
	existingByPath map[string]models.PlatformCategory,
	existingByID map[uint]models.PlatformCategory,
	existingTranslations map[uint]map[string]models.ContentTranslation,
) bool {
	if category.Level != len(row.Names) || category.SortOrder != row.Sort || category.Status != row.Status {
		return true
	}
	if category.Name != row.Names[len(row.Names)-1] || category.Path != strings.Join(row.Names, "/") {
		return true
	}
	if len(row.Names) == 1 {
		if category.ParentID != nil {
			return true
		}
		return platformCategoryImportRowTranslationNeedsUpdate(row, existingByPath, existingByID, existingTranslations)
	}
	parentPath := strings.Join(row.Names[:len(row.Names)-1], "/")
	parent, exists := existingByPath[normalizeLookupKey(parentPath)]
	if !exists || category.ParentID == nil || *category.ParentID != parent.ID {
		return true
	}
	return platformCategoryImportRowTranslationNeedsUpdate(row, existingByPath, existingByID, existingTranslations)
}

func platformCategoryImportPreviewExistingCategory(
	row platformCategoryImportRow,
	key string,
	existingByPath map[string]models.PlatformCategory,
	existingByID map[uint]models.PlatformCategory,
) (models.PlatformCategory, bool) {
	if row.ID != nil && *row.ID > 0 {
		if category, exists := existingByID[*row.ID]; exists {
			return category, true
		}
	}
	category, exists := existingByPath[key]
	return category, exists
}

func platformCategoryImportRowTranslationNeedsUpdate(
	row platformCategoryImportRow,
	existingByPath map[string]models.PlatformCategory,
	existingByID map[uint]models.PlatformCategory,
	existingTranslations map[uint]map[string]models.ContentTranslation,
) bool {
	if !row.ZhPathProvided {
		return false
	}
	for index, zhName := range row.ZhNames {
		path := strings.Join(row.Names[:index+1], "/")
		var category models.PlatformCategory
		var exists bool
		if index == len(row.Names)-1 && row.ID != nil && *row.ID > 0 {
			category, exists = existingByID[*row.ID]
		}
		if !exists {
			category, exists = existingByPath[normalizeLookupKey(path)]
		}
		if !exists {
			continue
		}
		current := translatedText("", existingTranslations[category.ID], TranslationFieldName)
		if strings.TrimSpace(current) != strings.TrimSpace(zhName) {
			return true
		}
	}
	return false
}

func parsePlatformCategoryImportRow(
	excelRow int,
	cells []string,
	columns platformCategoryImportColumns,
) (platformCategoryImportRow, []PlatformCategoryImportRowError) {
	field := func(index int) string {
		if index < 0 || index >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[index])
	}
	errors := make([]PlatformCategoryImportRowError, 0)
	addError := func(fieldName, message string) {
		errors = append(errors, PlatformCategoryImportRowError{Row: excelRow, Field: fieldName, Message: message})
	}

	var id *uint
	if value := field(columns.IDIndex); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			addError("Category ID", "Category ID must be a positive integer")
		} else {
			idValue := uint(parsed)
			id = &idValue
		}
	}

	names := make([]string, 0)
	zhNames := make([]string, 0)
	zhPathProvided := false
	nameFields := make([]string, 0)
	normalizedFields := make([]string, 0)
	if columns.PathIndex >= 0 {
		pathValue := field(columns.PathIndex)
		parsedNames := splitPlatformCategoryImportPath(pathValue, columns.PathSeparator)
		if len(parsedNames) == 0 {
			addError(columns.PathFieldName, columns.PathFieldName+"不能为空")
		} else {
			names = parsedNames
			for range names {
				nameFields = append(nameFields, columns.PathFieldName)
			}
		}
		if columns.ZhPathIndex >= 0 {
			zhPathValue := field(columns.ZhPathIndex)
			if zhPathValue != "" {
				zhPathProvided = true
				zhNames = splitPlatformCategoryImportPath(zhPathValue, columns.PathSeparator)
				if len(zhNames) != len(names) {
					addError(columns.ZhPathFieldName, "中文路径层级数量必须和 English Path 一致")
				}
			}
		}
	} else {
		hasGap := false
		for _, column := range columns.Levels {
			name := field(column.Index)
			if name == "" {
				hasGap = true
				continue
			}
			if hasGap {
				addError(column.Header, fmt.Sprintf("填写%s前必须连续填写上一级类目", column.Header))
			}
			if strings.Contains(name, "/") {
				name = strings.ReplaceAll(name, "/", "／")
				normalizedFields = append(normalizedFields, column.Header)
			}
			names = append(names, name)
			nameFields = append(nameFields, column.Header)
		}
		if field(columns.Levels[0].Index) == "" {
			addError(columns.Levels[0].Header, columns.Levels[0].Header+"不能为空")
		}
	}

	for index, name := range names {
		fieldName := nameFields[index]
		if strings.Contains(name, `\`) {
			addError(fieldName, "类目名称不能包含反斜杠 \\")
		}
		if utf8.RuneCountInString(name) > platformCategoryMaxNameLength {
			addError(fieldName, fmt.Sprintf("类目名称不能超过 %d 个字符", platformCategoryMaxNameLength))
		}
	}
	for _, name := range zhNames {
		if strings.Contains(name, `\`) {
			addError(columns.ZhPathFieldName, "中文名称不能包含反斜杠 \\")
		}
		if utf8.RuneCountInString(name) > platformCategoryMaxNameLength {
			addError(columns.ZhPathFieldName, fmt.Sprintf("中文名称不能超过 %d 个字符", platformCategoryMaxNameLength))
		}
	}
	if path := strings.Join(names, "/"); utf8.RuneCountInString(path) > platformCategoryMaxPathLength {
		addError(columns.PathFieldName, fmt.Sprintf("完整路径不能超过 %d 个字符", platformCategoryMaxPathLength))
	}

	sortOrder := 0
	if value := field(columns.SortIndex); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			addError("排序", "排序必须是非负整数")
		} else {
			sortOrder = parsed
		}
	}
	status, err := parsePlatformCategoryImportStatus(field(columns.StatusIndex))
	if err != nil {
		addError("状态", err.Error())
		status = models.PlatformCategoryStatusActive
	}
	return platformCategoryImportRow{
		ExcelRow:         excelRow,
		ID:               id,
		Names:            names,
		ZhNames:          zhNames,
		ZhPathProvided:   zhPathProvided,
		Sort:             sortOrder,
		Status:           status,
		NormalizedFields: normalizedFields,
	}, errors
}

func parsePlatformCategoryImportStatus(value string) (models.PlatformCategoryStatus, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "enabled", "active", "启用":
		return models.PlatformCategoryStatusActive, nil
	case "disabled", "停用":
		return models.PlatformCategoryStatusDisabled, nil
	default:
		return "", fmt.Errorf("状态仅支持 enabled、active、启用、disabled、停用")
	}
}

func splitPlatformCategoryImportPath(value string, separator string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if separator == ">" {
		parts := strings.Split(trimmed, ">")
		names := make([]string, 0, len(parts))
		for _, part := range parts {
			name := strings.TrimSpace(part)
			if name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	normalizedPath := normalizePlatformCategoryPath(trimmed)
	if normalizedPath == "" {
		return nil
	}
	return strings.Split(normalizedPath, "/")
}

func platformCategoryImportColumnIndexes(row []string) (platformCategoryImportColumns, bool) {
	indexes := make(map[string]int, len(row))
	for index, header := range row {
		header = strings.TrimSpace(header)
		if header != "" {
			indexes[header] = index
		}
	}
	columns := platformCategoryImportColumns{
		IDIndex:         -1,
		PathIndex:       -1,
		ZhPathIndex:     -1,
		SortIndex:       -1,
		StatusIndex:     -1,
		PathSeparator:   "/",
		PathFieldName:   "类目路径",
		ZhPathFieldName: "中文路径",
	}
	if index, exists := indexes["Category ID"]; exists {
		columns.IDIndex = index
	}
	if index, exists := indexes["排序"]; exists {
		columns.SortIndex = index
	}
	if index, exists := indexes["Sort Order"]; exists {
		columns.SortIndex = index
	}
	if index, exists := indexes["状态"]; exists {
		columns.StatusIndex = index
	}
	if index, exists := indexes["Status"]; exists {
		columns.StatusIndex = index
	}
	if index, exists := indexes["English Path"]; exists {
		columns.PathIndex = index
		columns.PathSeparator = ">"
		columns.PathFieldName = "English Path"
		if zhIndex, zhExists := indexes["中文路径"]; zhExists {
			columns.ZhPathIndex = zhIndex
		}
		return columns, true
	}
	if index, exists := indexes["类目路径"]; exists {
		columns.PathIndex = index
		return columns, true
	}

	levelHeaders := platformCategoryLevelHeaders
	if _, exists := indexes[levelHeaders[0]]; !exists {
		levelHeaders = platformCategoryLegacyLevelHeaders
		if _, legacyExists := indexes[levelHeaders[0]]; !legacyExists {
			return platformCategoryImportColumns{}, false
		}
	}
	columns.Levels = make([]platformCategoryImportLevelColumn, 0, len(levelHeaders))
	missingLevelHeader := false
	for _, header := range levelHeaders {
		index, exists := indexes[header]
		if !exists {
			missingLevelHeader = true
			continue
		}
		if missingLevelHeader {
			return platformCategoryImportColumns{}, false
		}
		columns.Levels = append(columns.Levels, platformCategoryImportLevelColumn{Header: header, Index: index})
	}
	return columns, len(columns.Levels) > 0
}

func flattenPlatformCategoryImportErrors(errorsByRow map[int][]PlatformCategoryImportRowError) []PlatformCategoryImportRowError {
	rows := make([]int, 0, len(errorsByRow))
	for row := range errorsByRow {
		rows = append(rows, row)
	}
	sort.Ints(rows)
	errors := make([]PlatformCategoryImportRowError, 0)
	for _, row := range rows {
		errors = append(errors, errorsByRow[row]...)
	}
	return errors
}

func countPlatformCategoryInvalidRows(errorsByRow map[int][]PlatformCategoryImportRowError) int {
	count := 0
	for _, errors := range errorsByRow {
		if len(errors) > 0 {
			count++
		}
	}
	return count
}

func joinPlatformCategoryImportErrors(errors []PlatformCategoryImportRowError) string {
	messages := make([]string, 0, len(errors))
	for _, rowError := range errors {
		messages = append(messages, rowError.Field+"："+rowError.Message)
	}
	return strings.Join(messages, "；")
}

func platformCategoryPreviewWithError(row int, field, message string) *PlatformCategoryImportPreview {
	return &PlatformCategoryImportPreview{
		Errors:      []PlatformCategoryImportRowError{{Row: row, Field: field, Message: message}},
		InvalidRows: 1,
	}
}
