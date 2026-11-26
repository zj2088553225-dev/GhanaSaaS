package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"saas/internal/models"
	"saas/internal/repository"

	"github.com/xuri/excelize/v2"
)

func TestBuildProductImportTemplateReturnsXLSX(t *testing.T) {
	content, err := BuildProductImportTemplate()
	if err != nil {
		t.Fatalf("BuildProductImportTemplate returned error: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("template content is empty")
	}
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("OpenReader returned error: %v", err)
	}
	defer func() { _ = file.Close() }()
	rows, err := file.GetRows("商品导入模板")
	if err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	if !headersMatch(rows[0]) {
		t.Fatalf("template headers = %+v, want product import headers", rows[0])
	}
	if containsString(rows[0], "SKU编码") {
		t.Fatalf("template headers = %+v, should not contain SKU编码", rows[0])
	}
	if containsString(rows[0], "店铺分类名称") {
		t.Fatalf("template headers = %+v, should not contain 店铺分类名称", rows[0])
	}
	instructionRows, err := file.GetRows("说明")
	if err != nil {
		t.Fatalf("GetRows instructions returned error: %v", err)
	}
	instructions := flattenRows(instructionRows)
	if !strings.Contains(instructions, "平台类目路径必须填写到最末级") {
		t.Fatalf("instructions = %q, want most-specific platform category wording", instructions)
	}
	if strings.Contains(instructions, "店铺分类名称") {
		t.Fatalf("instructions = %q, should not mention shop category", instructions)
	}
	if !strings.Contains(instructions, "GHS") {
		t.Fatalf("instructions = %q, want GHS price unit wording", instructions)
	}
	legacyMajorUnit := string(rune(0x5143))
	legacyCurrencySymbol := string(rune(0x00a5))
	if strings.Contains(instructions, legacyMajorUnit) || strings.Contains(instructions, legacyCurrencySymbol) {
		t.Fatalf("instructions = %q, should not mention legacy price units", instructions)
	}
	technicalLeafWording := "叶" + "子"
	if strings.Contains(instructions, technicalLeafWording) {
		t.Fatalf("instructions = %q, should not contain technical leaf wording", instructions)
	}
}

func TestProductImportPreviewRejectsMissingHeader(t *testing.T) {
	service := NewAdminProductService(newFakeProductRepository())
	content := buildProductImportWorkbook(t, []string{"商品名称"}, [][]string{{"可口可乐 330ml"}})

	preview, err := service.PreviewImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows == 0 || !importErrorsContain(preview.Errors, "Excel 表头缺失或不匹配") {
		t.Fatalf("preview = %+v, want header error", preview)
	}
}

func TestProductImportPreviewReturnsValidationErrors(t *testing.T) {
	service := NewAdminProductService(newFakeProductRepository())
	content := buildProductImportWorkbook(t, productImportHeaders, [][]string{
		validImportRow(func(row []string) { row[1] = "食品饮料/不存在类目" }),
		validImportRow(func(row []string) { row[12] = "abc" }),
		validImportRow(func(row []string) { row[14] = "1.5" }),
	})

	preview, err := service.PreviewImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 3 {
		t.Fatalf("invalid rows = %d, want 3, errors = %+v", preview.InvalidRows, preview.Errors)
	}
	for _, message := range []string{"平台类目不存在", "零售价格式错误", "库存必须是整数"} {
		if !importErrorsContain(preview.Errors, message) {
			t.Fatalf("errors = %+v, want message containing %q", preview.Errors, message)
		}
	}
}

func TestProductImportPreviewValidatesPlatformCategoryPath(t *testing.T) {
	service := NewAdminProductService(newFakeProductRepository())
	content := buildProductImportWorkbook(t, productImportHeaders, [][]string{
		validImportRow(func(row []string) { row[1] = "" }),
		validImportRow(func(row []string) { row[1] = "食品饮料/不存在类目" }),
		validImportRow(func(row []string) { row[1] = "食品饮料" }),
		validImportRow(func(row []string) { row[1] = "食品饮料/停用类目" }),
	})

	preview, err := service.PreviewImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PreviewImport returned error: %v", err)
	}
	if preview.InvalidRows != 4 {
		t.Fatalf("invalid rows = %d, want 4, errors = %+v", preview.InvalidRows, preview.Errors)
	}
	for _, message := range []string{"平台类目路径不能为空", "平台类目不存在", "平台类目不是最末级，请选择更具体的类目", "平台类目已停用"} {
		if !importErrorsContain(preview.Errors, message) {
			t.Fatalf("errors = %+v, want message containing %q", preview.Errors, message)
		}
	}
	technicalLeafWording := "叶" + "子"
	for _, rowError := range preview.Errors {
		if strings.Contains(rowError.Message, technicalLeafWording) {
			t.Fatalf("error message = %q, should not contain technical leaf wording", rowError.Message)
		}
	}
}

func TestProductImportConfirmCreatesSingleProductWithMultipleSKUs(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	content := buildProductImportWorkbook(t, productImportHeaders, [][]string{
		validImportRow(func(row []string) {
			row[6] = "容量"
			row[7] = "330ml"
			row[8] = "包装"
			row[9] = "单罐"
			row[12] = "5.00"
			row[14] = "100"
		}),
		validImportRow(func(row []string) {
			row[6] = "容量"
			row[7] = "500ml"
			row[8] = "包装"
			row[9] = "单瓶"
			row[12] = "7.50"
			row[14] = "60"
		}),
	})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v, preview = %+v", err, preview)
	}
	if result.ImportedProducts != 1 || result.ImportedSKUs != 2 {
		t.Fatalf("result = %+v, want one product with two SKUs", result)
	}
	if len(repo.products) != 1 {
		t.Fatalf("products = %d, want 1", len(repo.products))
	}
	product := onlyProduct(t, repo)
	if product.TenantID != 1001 {
		t.Fatalf("tenant_id = %d, want JWT tenant 1001", product.TenantID)
	}
	if product.PlatformCategoryID != 5001 {
		t.Fatalf("platform_category_id = %d, want 5001", product.PlatformCategoryID)
	}
	if product.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", product.CategoryID)
	}
	if len(product.SKUs) != 2 {
		t.Fatalf("skus = %+v, want 2", product.SKUs)
	}
	if product.SKUs[0].RetailPrice != 500 || product.SKUs[1].RetailPrice != 750 {
		t.Fatalf("sku prices = %+v, want GHS minor units", product.SKUs)
	}
	if product.SKUs[0].ERPSKUCode == "" || product.SKUs[1].ERPSKUCode == "" {
		t.Fatalf("sku codes should be generated: %+v", product.SKUs)
	}
	if product.SKUs[0].ERPSKUCode == product.SKUs[1].ERPSKUCode {
		t.Fatalf("generated SKU codes should be different: %+v", product.SKUs)
	}
	if !strings.HasPrefix(product.SKUs[0].ERPSKUCode, "SKU-1001-") || !strings.HasPrefix(product.SKUs[1].ERPSKUCode, "SKU-1001-") {
		t.Fatalf("generated SKU codes = %q, %q, want tenant prefix", product.SKUs[0].ERPSKUCode, product.SKUs[1].ERPSKUCode)
	}
}

func TestProductImportRetriesDatabaseSKUCodeConflict(t *testing.T) {
	repo := newFakeProductRepository()
	repo.createErrors = []error{repository.ErrDuplicateSKUCode}
	service := NewAdminProductService(repo)
	content := buildProductImportWorkbook(t, productImportHeaders, [][]string{validImportRow(nil)})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error after retryable SKU conflict: %v, preview = %+v", err, preview)
	}
	if result.ImportedProducts != 1 || result.ImportedSKUs != 1 {
		t.Fatalf("result = %+v, want one imported product and SKU", result)
	}
	if repo.createCalls != 2 {
		t.Fatalf("create calls = %d, want 2", repo.createCalls)
	}
	product := onlyProduct(t, repo)
	if product.ID == 0 || len(product.SKUs) != 1 || product.SKUs[0].ID == 0 || product.SKUs[0].ERPSKUCode == "" {
		t.Fatalf("imported product was not persisted after retry: %+v", product)
	}
}

func TestImportSKUNameUsesCanonicalDefault(t *testing.T) {
	if got := importSKUName(&productImportRow{Specs: map[string]string{}}); got != canonicalDefaultSKUName {
		t.Fatalf("importSKUName without specs = %q, want %q", got, canonicalDefaultSKUName)
	}
	if got := importSKUName(&productImportRow{
		Specs:     map[string]string{"Size": "Large"},
		SpecNames: []string{"Size"},
	}); got != "Large" {
		t.Fatalf("importSKUName with specs = %q, want Large", got)
	}
}

func TestProductImportDoesNotUseShopCategoryColumn(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	headers := append(append([]string{}, productImportHeaders...), "店铺分类名称")
	content := buildProductImportWorkbook(t, headers, [][]string{
		append(validImportRow(nil), "其他租户分类"),
	})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v, result = %+v, preview = %+v", err, result, preview)
	}
	product := onlyProduct(t, repo)
	if result.ImportedProducts != 1 || result.ImportedSKUs != 1 {
		t.Fatalf("result = %+v, want one product and one SKU", result)
	}
	if product.PlatformCategoryID != 5001 {
		t.Fatalf("platform_category_id = %d, want 5001", product.PlatformCategoryID)
	}
	if product.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", product.CategoryID)
	}
}

func TestProductImportGroupsByProductNameAndPlatformCategoryOnly(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	headers := append(append([]string{}, productImportHeaders...), "店铺分类名称")
	content := buildProductImportWorkbook(t, headers, [][]string{
		append(validImportRow(func(row []string) {
			row[6] = "容量"
			row[7] = "330ml"
		}), "分类 A"),
		append(validImportRow(func(row []string) {
			row[6] = "容量"
			row[7] = "500ml"
		}), "分类 B"),
	})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v, result = %+v, preview = %+v", err, result, preview)
	}
	if result.ImportedProducts != 1 || result.ImportedSKUs != 2 {
		t.Fatalf("result = %+v, want one product grouped by name and platform category", result)
	}
	product := onlyProduct(t, repo)
	if product.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", product.CategoryID)
	}
}

func TestProductImportConfirmUsesJWTTenantAndIgnoresExtraTenantColumn(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	headers := append(append([]string{}, productImportHeaders...), "tenant_id")
	row := append(validImportRow(nil), "9999")
	content := buildProductImportWorkbook(t, headers, [][]string{row})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v, result = %+v, preview = %+v", err, result, preview)
	}
	product := onlyProduct(t, repo)
	if product.TenantID != 1001 {
		t.Fatalf("tenant_id = %d, want JWT tenant 1001", product.TenantID)
	}
	if product.SKUs[0].ERPSKUCode == "" {
		t.Fatal("blank SKU code should be auto-generated for import")
	}
}

func TestProductImportIgnoresExtraSKUCodeColumn(t *testing.T) {
	repo := newFakeProductRepository()
	service := NewAdminProductService(repo)
	headers := append(append([]string{}, productImportHeaders...), "SKU编码")
	row := append(validImportRow(nil), "USER-SUPPLIED-SKU")
	content := buildProductImportWorkbook(t, headers, [][]string{row})

	result, preview, err := service.ConfirmImport(context.Background(), 1001, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ConfirmImport returned error: %v, result = %+v, preview = %+v", err, result, preview)
	}
	product := onlyProduct(t, repo)
	if product.SKUs[0].ERPSKUCode == "USER-SUPPLIED-SKU" {
		t.Fatalf("import used user supplied SKU code: %+v", product.SKUs[0])
	}
	if !strings.HasPrefix(product.SKUs[0].ERPSKUCode, "SKU-1001-") {
		t.Fatalf("generated SKU code = %q, want tenant prefix", product.SKUs[0].ERPSKUCode)
	}
}

func buildProductImportWorkbook(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	file := excelize.NewFile()
	const sheet = "Sheet1"
	for columnIndex, header := range headers {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, 1)
		if err != nil {
			t.Fatalf("CoordinatesToCellName returned error: %v", err)
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			t.Fatalf("SetCellValue returned error: %v", err)
		}
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				t.Fatalf("CoordinatesToCellName returned error: %v", err)
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				t.Fatalf("SetCellValue returned error: %v", err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer returned error: %v", err)
	}
	return buffer.Bytes()
}

func validImportRow(mutate func([]string)) []string {
	row := []string{
		"可口可乐 330ml",
		"食品饮料/饮料/碳酸饮料",
		"下架",
		"https://example.com/coke-main.jpg",
		"https://example.com/coke-1.jpg,https://example.com/coke-2.jpg",
		"经典碳酸饮料",
		"",
		"",
		"",
		"",
		"",
		"",
		"5.00",
		"6.00",
		"100",
	}
	if mutate != nil {
		mutate(row)
	}
	return row
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func importErrorsContain(errors []AdminProductImportRowError, message string) bool {
	for _, err := range errors {
		if strings.Contains(err.Message, message) {
			return true
		}
	}
	return false
}

func flattenRows(rows [][]string) string {
	var builder strings.Builder
	for _, row := range rows {
		for _, cell := range row {
			builder.WriteString(cell)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func onlyProduct(t *testing.T, repo *fakeProductRepository) models.SPU {
	t.Helper()
	if len(repo.products) != 1 {
		t.Fatalf("products = %d, want 1", len(repo.products))
	}
	for _, product := range repo.products {
		return product
	}
	t.Fatal("product not found")
	return models.SPU{}
}
