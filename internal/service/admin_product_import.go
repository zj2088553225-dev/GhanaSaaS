package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"saas/internal/models"

	"github.com/xuri/excelize/v2"
	"gorm.io/datatypes"
)

const (
	ProductImportMaxRows = 1000
)

var (
	productImportHeaders = []string{
		"商品名称",
		"平台类目路径",
		"商品状态",
		"商品主图URL",
		"轮播图URL",
		"商品描述",
		"规格1名称",
		"规格1值",
		"规格2名称",
		"规格2值",
		"规格3名称",
		"规格3值",
		"零售价",
		"原价",
		"库存",
	}
	pricePattern = regexp.MustCompile(`^\d+(\.\d{1,2})?$`)
)

type AdminProductImportPreview struct {
	Rows            int                          `json:"rows"`
	ValidRows       int                          `json:"valid_rows"`
	InvalidRows     int                          `json:"invalid_rows"`
	Errors          []AdminProductImportRowError `json:"errors"`
	PreviewProducts []AdminProductImportProduct  `json:"preview_products"`
	products        []AdminProductInput
}

type AdminProductImportRowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type AdminProductImportProduct struct {
	Name                 string           `json:"name"`
	PlatformCategoryPath string           `json:"platform_category_path"`
	Status               models.SPUStatus `json:"status"`
	SKUCount             int              `json:"sku_count"`
	MinPrice             int64            `json:"min_price"`
	MaxPrice             int64            `json:"max_price"`
	TotalStock           int64            `json:"total_stock"`
}

type AdminProductImportResult struct {
	ImportedProducts int `json:"imported_products"`
	ImportedSKUs     int `json:"imported_skus"`
}

type productImportRow struct {
	ExcelRow             int
	ProductName          string
	PlatformCategoryPath string
	Status               models.SPUStatus
	MainImage            string
	SliderImages         []string
	Description          string
	Specs                map[string]string
	SpecNames            []string
	RetailPrice          int64
	OriginalPrice        int64
	SellableStock        int64
}

type productImportGroup struct {
	Name                 string
	PlatformCategoryPath string
	PlatformCategoryID   uint
	Rows                 []*productImportRow
}

type specTemplateItem struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

func BuildProductImportTemplate() ([]byte, error) {
	file := excelize.NewFile()
	const sheet = "商品导入模板"
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

	for columnIndex, header := range productImportHeaders {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
	}
	sample := []any{
		"可口可乐 330ml",
		"食品饮料/饮料/碳酸饮料",
		"下架",
		"https://example.com/coke-main.jpg",
		"https://example.com/coke-1.jpg,https://example.com/coke-2.jpg",
		"经典碳酸饮料",
		"容量",
		"330ml",
		"包装",
		"单罐",
		"",
		"",
		"5.00",
		"6.00",
		"100",
	}
	for columnIndex, value := range sample {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 2)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			return nil, err
		}
	}

	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FDE68A"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheet, "A1", "O1", headerStyle); err != nil {
		return nil, err
	}
	_ = file.SetColWidth(sheet, "A", "O", 18)
	_ = file.SetColWidth(sheet, "B", "B", 34)
	_ = file.SetColWidth(sheet, "D", "F", 34)

	const instructions = "说明"
	if _, err := file.NewSheet(instructions); err != nil {
		return nil, err
	}
	notes := [][]string{
		{"说明", "内容"},
		{"图片字段", "第一版只支持 URL，不支持 Excel 内嵌图片。轮播图多个 URL 用英文逗号分隔。"},
		{"平台类目路径", "平台类目路径必须填写到最末级，例如：食品饮料/饮料/碳酸饮料，且必须存在并启用。"},
		{"价格", "零售价：GHS，例如 80.00；原价：GHS，例如 100.00。后端仍转换成 minor unit 保存。"},
		{"库存", "库存必须是整数，不能小于 0。"},
		{"SKU 编码", "SKU 编码由系统自动生成，无需填写。"},
		{"多 SKU", "多 SKU 商品通过多行填写相同商品名称和平台类目路径，并填写不同规格值。"},
		{"商品状态", "支持 上架、下架、on_shelf、off_shelf；为空默认下架。"},
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
	_ = file.SetColWidth(instructions, "B", "B", 90)

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *AdminProductService) PreviewImport(ctx context.Context, tenantID uint, reader io.Reader) (*AdminProductImportPreview, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.parseImport(ctx, tenantID, reader)
}

func (s *AdminProductService) ConfirmImport(ctx context.Context, tenantID uint, reader io.Reader) (*AdminProductImportResult, *AdminProductImportPreview, error) {
	if tenantID == 0 {
		return nil, nil, ErrForbidden
	}

	preview, err := s.parseImport(ctx, tenantID, reader)
	if err != nil {
		return nil, nil, err
	}
	if preview.InvalidRows > 0 || len(preview.Errors) > 0 {
		return nil, preview, ErrInvalidInput
	}

	products := make([]models.SPU, 0, len(preview.products))
	for _, input := range preview.products {
		product, err := buildProductFromInput(tenantID, 0, input, models.SPUStatusOffShelf, nil, false)
		if err != nil {
			return nil, preview, err
		}
		products = append(products, *product)
	}
	if err := s.createProductsWithUniqueSKUCodes(ctx, tenantID, products); err != nil {
		return nil, preview, err
	}

	importedSKUs := 0
	for _, product := range products {
		importedSKUs += len(product.SKUs)
	}
	return &AdminProductImportResult{
		ImportedProducts: len(products),
		ImportedSKUs:     importedSKUs,
	}, preview, nil
}

func (s *AdminProductService) parseImport(ctx context.Context, tenantID uint, reader io.Reader) (*AdminProductImportPreview, error) {
	file, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, ErrInvalidInput
	}
	defer func() { _ = file.Close() }()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return importPreviewWithError(1, "表头", "Excel 文件缺少工作表"), nil
	}

	rows, err := file.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return importPreviewWithError(1, "表头", "Excel 表头缺失或不匹配，请使用标准模板"), nil
	}
	columnIndexes, ok := productImportColumnIndexes(rows[0])
	if !ok {
		return importPreviewWithError(1, "表头", "Excel 表头缺失或不匹配，请使用标准模板"), nil
	}

	parsedRows := make([]*productImportRow, 0)
	rowErrors := make(map[int][]AdminProductImportRowError)
	platformCategoryPaths := make(map[string]string)
	nonEmptyRows := 0

	for rowIndex := 1; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		if rowIsEmpty(row) {
			continue
		}
		nonEmptyRows++
		excelRow := rowIndex + 1
		if len(parsedRows) >= ProductImportMaxRows {
			rowErrors[excelRow] = append(rowErrors[excelRow], AdminProductImportRowError{
				Row:     excelRow,
				Field:   "行数",
				Message: fmt.Sprintf("最多支持 %d 行商品数据", ProductImportMaxRows),
			})
			break
		}

		parsed, errors := parseProductImportRow(excelRow, row, columnIndexes)
		if parsed != nil {
			parsedRows = append(parsedRows, parsed)
			if parsed.PlatformCategoryPath != "" {
				platformCategoryPaths[normalizeLookupKey(parsed.PlatformCategoryPath)] = parsed.PlatformCategoryPath
			}
		}
		if len(errors) > 0 {
			rowErrors[excelRow] = append(rowErrors[excelRow], errors...)
		}
	}

	platformCategoryList := make([]string, 0, len(platformCategoryPaths))
	for _, path := range platformCategoryPaths {
		platformCategoryList = append(platformCategoryList, path)
	}
	platformCategoriesByPath, err := s.repo.FindPlatformCategoriesByPaths(ctx, platformCategoryList)
	if err != nil {
		return nil, err
	}

	platformCategoryChildren := make(map[uint]bool)
	platformCategoryHasChildren := func(category models.PlatformCategory) (bool, error) {
		if hasChildren, exists := platformCategoryChildren[category.ID]; exists {
			return hasChildren, nil
		}
		hasChildren, err := s.repo.HasPlatformCategoryChildren(ctx, category.ID)
		if err != nil {
			return false, err
		}
		platformCategoryChildren[category.ID] = hasChildren
		return hasChildren, nil
	}

	groups := make(map[string]*productImportGroup)
	groupOrder := make([]string, 0)
	for _, row := range parsedRows {
		platformCategory, exists := platformCategoriesByPath[normalizeLookupKey(row.PlatformCategoryPath)]
		if !exists && row.PlatformCategoryPath != "" {
			rowErrors[row.ExcelRow] = append(rowErrors[row.ExcelRow], AdminProductImportRowError{
				Row:     row.ExcelRow,
				Field:   "平台类目路径",
				Message: "平台类目不存在",
			})
		} else if exists && platformCategory.Status != models.PlatformCategoryStatusActive {
			rowErrors[row.ExcelRow] = append(rowErrors[row.ExcelRow], AdminProductImportRowError{
				Row:     row.ExcelRow,
				Field:   "平台类目路径",
				Message: "平台类目已停用",
			})
		} else if exists {
			hasChildren, err := platformCategoryHasChildren(platformCategory)
			if err != nil {
				return nil, err
			}
			if hasChildren {
				rowErrors[row.ExcelRow] = append(rowErrors[row.ExcelRow], AdminProductImportRowError{
					Row:     row.ExcelRow,
					Field:   "平台类目路径",
					Message: "平台类目不是最末级，请选择更具体的类目",
				})
			}
		}
		if len(rowErrors[row.ExcelRow]) > 0 {
			continue
		}

		key := normalizeLookupKey(row.ProductName) + "\x00" + normalizeLookupKey(row.PlatformCategoryPath)
		group := groups[key]
		if group == nil {
			group = &productImportGroup{
				Name:                 row.ProductName,
				PlatformCategoryPath: row.PlatformCategoryPath,
				PlatformCategoryID:   platformCategory.ID,
				Rows:                 make([]*productImportRow, 0, 1),
			}
			groups[key] = group
			groupOrder = append(groupOrder, key)
		}
		group.Rows = append(group.Rows, row)
	}

	preview := &AdminProductImportPreview{
		Rows:   nonEmptyRows,
		Errors: flattenImportErrors(rowErrors),
	}
	preview.InvalidRows = countInvalidRows(rowErrors)
	preview.ValidRows = preview.Rows - preview.InvalidRows
	if preview.ValidRows < 0 {
		preview.ValidRows = 0
	}

	preview.PreviewProducts = make([]AdminProductImportProduct, 0, len(groupOrder))
	preview.products = make([]AdminProductInput, 0, len(groupOrder))
	for _, key := range groupOrder {
		group := groups[key]
		input, productPreview, err := buildImportProductInput(group)
		if err != nil {
			return nil, err
		}
		preview.products = append(preview.products, input)
		preview.PreviewProducts = append(preview.PreviewProducts, productPreview)
	}

	return preview, nil
}

func parseProductImportRow(excelRow int, cells []string, columnIndexes map[string]int) (*productImportRow, []AdminProductImportRowError) {
	field := func(name string) string {
		index, ok := columnIndexes[name]
		if !ok {
			return ""
		}
		if index >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[index])
	}
	errors := make([]AdminProductImportRowError, 0)
	addError := func(fieldName string, message string) {
		errors = append(errors, AdminProductImportRowError{Row: excelRow, Field: fieldName, Message: message})
	}

	productName := field("商品名称")
	platformCategoryPath := normalizePlatformCategoryPath(field("平台类目路径"))
	statusValue := field("商品状态")
	mainImage := field("商品主图URL")
	sliderImageValue := field("轮播图URL")
	description := field("商品描述")
	retailPriceValue := field("零售价")
	originalPriceValue := field("原价")
	stockValue := field("库存")

	if productName == "" {
		addError("商品名称", "商品名称不能为空")
	}
	if platformCategoryPath == "" {
		addError("平台类目路径", "平台类目路径不能为空")
	}
	status, err := parseImportStatus(statusValue)
	if err != nil {
		addError("商品状态", err.Error())
	}
	if mainImage == "" {
		addError("商品主图URL", "主图URL不能为空")
	}
	retailPrice, err := parseRequiredImportPrice(retailPriceValue, "零售价")
	if err != nil {
		addError("零售价", err.Error())
	}
	originalPrice, err := parseOptionalImportPrice(originalPriceValue, "原价")
	if err != nil {
		addError("原价", err.Error())
	}
	stock, err := parseImportStock(stockValue)
	if err != nil {
		addError("库存", err.Error())
	}

	specs := make(map[string]string)
	specNames := make([]string, 0, 3)
	for index := 0; index < 3; index++ {
		specName := field(fmt.Sprintf("规格%d名称", index+1))
		specValue := field(fmt.Sprintf("规格%d值", index+1))
		if specName == "" && specValue == "" {
			continue
		}
		fieldLabel := fmt.Sprintf("规格%d", index+1)
		if specName == "" || specValue == "" {
			addError(fieldLabel, "规格名称和值必须同时填写")
			continue
		}
		if _, exists := specs[specName]; exists {
			addError(fieldLabel, "规格名称不能重复")
			continue
		}
		specs[specName] = specValue
		specNames = append(specNames, specName)
	}

	return &productImportRow{
		ExcelRow:             excelRow,
		ProductName:          productName,
		PlatformCategoryPath: platformCategoryPath,
		Status:               status,
		MainImage:            mainImage,
		SliderImages:         splitImageURLs(sliderImageValue),
		Description:          description,
		Specs:                specs,
		SpecNames:            specNames,
		RetailPrice:          retailPrice,
		OriginalPrice:        originalPrice,
		SellableStock:        stock,
	}, errors
}

func buildImportProductInput(group *productImportGroup) (AdminProductInput, AdminProductImportProduct, error) {
	first := group.Rows[0]
	specTemplate, err := buildImportSpecTemplate(group.Rows)
	if err != nil {
		return AdminProductInput{}, AdminProductImportProduct{}, err
	}
	specTemplateJSON, err := json.Marshal(specTemplate)
	if err != nil {
		return AdminProductInput{}, AdminProductImportProduct{}, err
	}
	specTemplateValue := datatypes.JSON(specTemplateJSON)

	sliderImagesJSON, err := json.Marshal(first.SliderImages)
	if err != nil {
		return AdminProductInput{}, AdminProductImportProduct{}, err
	}
	sliderImagesValue := datatypes.JSON(sliderImagesJSON)

	skus := make([]AdminSKUInput, 0, len(group.Rows))
	preview := AdminProductImportProduct{
		Name:                 group.Name,
		PlatformCategoryPath: group.PlatformCategoryPath,
		Status:               first.Status,
		SKUCount:             len(group.Rows),
		MinPrice:             group.Rows[0].RetailPrice,
		MaxPrice:             group.Rows[0].RetailPrice,
	}
	for _, row := range group.Rows {
		specsJSON, err := json.Marshal(row.Specs)
		if err != nil {
			return AdminProductInput{}, AdminProductImportProduct{}, err
		}
		specsValue := datatypes.JSON(specsJSON)
		skus = append(skus, AdminSKUInput{
			Name:          importSKUName(row),
			Specs:         &specsValue,
			RetailPrice:   row.RetailPrice,
			OriginalPrice: row.OriginalPrice,
			SellableStock: row.SellableStock,
		})
		if row.RetailPrice < preview.MinPrice {
			preview.MinPrice = row.RetailPrice
		}
		if row.RetailPrice > preview.MaxPrice {
			preview.MaxPrice = row.RetailPrice
		}
		preview.TotalStock += row.SellableStock
	}

	return AdminProductInput{
		PlatformCategoryID: group.PlatformCategoryID,
		Name:               group.Name,
		Description:        first.Description,
		MainImage:          first.MainImage,
		SliderImages:       &sliderImagesValue,
		SpecTemplate:       &specTemplateValue,
		Status:             first.Status,
		SKUs:               skus,
	}, preview, nil
}

func buildImportSpecTemplate(rows []*productImportRow) ([]specTemplateItem, error) {
	valuesByName := make(map[string][]string)
	order := make([]string, 0, 3)
	seenValue := make(map[string]map[string]struct{})

	for _, row := range rows {
		for _, name := range row.SpecNames {
			value := row.Specs[name]
			if _, exists := valuesByName[name]; !exists {
				order = append(order, name)
				valuesByName[name] = []string{}
				seenValue[name] = map[string]struct{}{}
			}
			if _, exists := seenValue[name][value]; exists {
				continue
			}
			seenValue[name][value] = struct{}{}
			valuesByName[name] = append(valuesByName[name], value)
		}
	}

	template := make([]specTemplateItem, 0, len(order))
	for _, name := range order {
		template = append(template, specTemplateItem{Name: name, Values: valuesByName[name]})
	}
	return template, nil
}

func importSKUName(row *productImportRow) string {
	if len(row.Specs) == 0 {
		return canonicalDefaultSKUName
	}
	values := make([]string, 0, len(row.SpecNames))
	for _, name := range row.SpecNames {
		values = append(values, row.Specs[name])
	}
	return strings.Join(values, " / ")
}

func parseImportStatus(value string) (models.SPUStatus, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "下架", "off_shelf":
		return models.SPUStatusOffShelf, nil
	case "上架", "on_shelf":
		return models.SPUStatusOnShelf, nil
	default:
		return "", fmt.Errorf("商品状态仅支持 上架、下架、on_shelf、off_shelf")
	}
}

func parseRequiredImportPrice(value string, field string) (int64, error) {
	cents, err := parseImportPrice(value, field)
	if err != nil {
		return 0, err
	}
	if cents <= 0 {
		return 0, fmt.Errorf("%s必须大于 0", field)
	}
	return cents, nil
}

func parseOptionalImportPrice(value string, field string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return parseImportPrice(value, field)
}

func parseImportPrice(value string, field string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || !pricePattern.MatchString(value) {
		return 0, fmt.Errorf("%s格式错误", field)
	}
	parts := strings.SplitN(value, ".", 2)
	majorUnits, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s格式错误", field)
	}
	fen := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		fen, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s格式错误", field)
		}
	}
	return majorUnits*100 + fen, nil
}

func parseImportStock(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("库存不能为空")
	}
	stock, err := strconv.ParseInt(value, 10, 64)
	if err != nil || stock < 0 {
		return 0, fmt.Errorf("库存必须是整数且不能小于 0")
	}
	return stock, nil
}

func splitImageURLs(value string) []string {
	parts := strings.Split(value, ",")
	images := make([]string, 0, len(parts))
	for _, part := range parts {
		if image := strings.TrimSpace(part); image != "" {
			images = append(images, image)
		}
	}
	return images
}

func headersMatch(row []string) bool {
	_, ok := productImportColumnIndexes(row)
	return ok
}

func productImportColumnIndexes(row []string) (map[string]int, bool) {
	indexes := make(map[string]int, len(row))
	for index, header := range row {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if _, exists := indexes[header]; !exists {
			indexes[header] = index
		}
	}
	for _, expected := range productImportHeaders {
		if _, exists := indexes[expected]; !exists {
			return nil, false
		}
	}
	return indexes, true
}

func rowIsEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func normalizeLookupKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func flattenImportErrors(rowErrors map[int][]AdminProductImportRowError) []AdminProductImportRowError {
	rows := make([]int, 0, len(rowErrors))
	for row := range rowErrors {
		rows = append(rows, row)
	}
	sort.Ints(rows)
	errors := make([]AdminProductImportRowError, 0)
	for _, row := range rows {
		errors = append(errors, rowErrors[row]...)
	}
	return errors
}

func countInvalidRows(rowErrors map[int][]AdminProductImportRowError) int {
	count := 0
	for _, errors := range rowErrors {
		if len(errors) > 0 {
			count++
		}
	}
	return count
}

func importPreviewWithError(row int, field string, message string) *AdminProductImportPreview {
	return &AdminProductImportPreview{
		InvalidRows: 1,
		Errors: []AdminProductImportRowError{{
			Row:     row,
			Field:   field,
			Message: message,
		}},
		PreviewProducts: []AdminProductImportProduct{},
	}
}
