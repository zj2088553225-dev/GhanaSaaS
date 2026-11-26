package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/service"

	"github.com/gen2brain/webp"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newUploadControllerTestRouter(storage *service.LocalStorage) *gin.Engine {
	return newUploadControllerTestRouterWithVideo(storage, storage)
}

func newUploadControllerTestRouterWithVideo(imageStorage service.StorageService, videoStorage service.VideoStorageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin, models.UserRoleFinance))
	NewUploadController(imageStorage, videoStorage).RegisterRoutes(group)
	return router
}

func TestUploadImageRejectsUnauthenticatedRequest(t *testing.T) {
	router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(t.TempDir(), "uploads", "images")))
	body, contentType := multipartBody(t, "file", "product.png", pngBytes())
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/image", body)
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestUploadImageRejectsNonImageFile(t *testing.T) {
	router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(t.TempDir(), "uploads", "images")))
	body, contentType := multipartBody(t, "file", "note.txt", []byte("not an image"))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/image", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestUploadImageAllowsTenantScopedAdmins(t *testing.T) {
	cases := []struct {
		name     string
		role     models.UserRole
		tenantID *uint
	}{
		{name: "tenant admin", role: models.UserRoleTenantAdmin, tenantID: uploadUintPtr(1001)},
		{name: "super admin with tenant", role: models.UserRoleSuperAdmin, tenantID: uploadUintPtr(1001)},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")))
			body, contentType := multipartBody(t, "file", "product.png", pngBytes())
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/image", body)
			request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, tt.role, tt.tenantID))
			request.Header.Set("Content-Type", contentType)

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			var payload struct {
				Data struct {
					URL string `json:"url"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("Unmarshal returned error: %v", err)
			}
			if !strings.HasPrefix(payload.Data.URL, "/uploads/") {
				t.Fatalf("url = %q, want /uploads path", payload.Data.URL)
			}
			if strings.Contains(payload.Data.URL, tempDir) || strings.Contains(payload.Data.URL, `\`) || strings.HasPrefix(payload.Data.URL, "file:") {
				t.Fatalf("url = %q, should not expose local filesystem path", payload.Data.URL)
			}
		})
	}
}

func TestUploadImageAllowsGlobalSuperAdminForCmsImages(t *testing.T) {
	router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(t.TempDir(), "uploads", "images")))
	body, contentType := multipartBody(t, "file", "product.png", pngBytes())
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/image", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleSuperAdmin, nil))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestUploadImageGeneratesWebPVariants(t *testing.T) {
	cases := []struct {
		name         string
		filename     string
		contentType  string
		content      []byte
		originalSuff string
	}{
		{
			name:         "jpeg",
			filename:     "product.jpg",
			contentType:  "image/jpeg",
			content:      jpegBytes(),
			originalSuff: "_original.jpg",
		},
		{
			name:         "png",
			filename:     "product.png",
			contentType:  "image/png",
			content:      pngBytes(),
			originalSuff: "_original.png",
		},
		{
			name:         "webp",
			filename:     "product.webp",
			contentType:  "image/webp",
			content:      webpBytes(t),
			originalSuff: "_original.webp",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")))

			result := uploadImageRequest(t, router, tt.filename, tt.contentType, tt.content)

			if !strings.HasPrefix(result.URL, "/uploads/images/") || !strings.HasSuffix(result.URL, "_large.webp") {
				t.Fatalf("url = %q, want optimized large WebP upload path", result.URL)
			}
			if result.Variants.Large != result.URL {
				t.Fatalf("large variant = %q, want url %q", result.Variants.Large, result.URL)
			}
			wantSuffixes := map[string]string{
				"thumb":    "_thumb.webp",
				"card":     "_card.webp",
				"large":    "_large.webp",
				"original": tt.originalSuff,
			}
			variantURLs := map[string]string{
				"thumb":    result.Variants.Thumb,
				"card":     result.Variants.Card,
				"large":    result.Variants.Large,
				"original": result.Variants.Original,
			}
			for name, url := range variantURLs {
				if !strings.HasPrefix(url, "/uploads/images/") || !strings.HasSuffix(url, wantSuffixes[name]) {
					t.Fatalf("%s url = %q, want suffix %q", name, url, wantSuffixes[name])
				}
				path := filepath.Join(tempDir, strings.TrimPrefix(url, "/"))
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("%s Stat returned error: %v", name, err)
				}
				if info.Mode().Perm() != 0o644 {
					t.Fatalf("%s mode = %o, want 644", name, info.Mode().Perm())
				}
				if name != "original" {
					file, err := os.Open(path)
					if err != nil {
						t.Fatalf("%s Open returned error: %v", name, err)
					}
					if _, err := webp.Decode(file); err != nil {
						file.Close()
						t.Fatalf("%s Decode returned error: %v", name, err)
					}
					file.Close()
				}
			}
			dirInfo, err := os.Stat(filepath.Dir(filepath.Join(tempDir, strings.TrimPrefix(result.URL, "/"))))
			if err != nil {
				t.Fatalf("variant directory Stat returned error: %v", err)
			}
			if dirInfo.Mode().Perm() != 0o755 {
				t.Fatalf("variant directory mode = %o, want 755", dirInfo.Mode().Perm())
			}
		})
	}
}

func TestUploadImageDeduplicatesSameOriginalContent(t *testing.T) {
	tempDir := t.TempDir()
	assetStore := newMemoryUploadAssetStore()
	router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images"), assetStore))

	first := uploadImageRequest(t, router, "first.png", "image/png", pngBytes())
	second := uploadImageRequest(t, router, "renamed.png", "image/png", pngBytes())

	if first.URL == "" || second.URL != first.URL {
		t.Fatalf("first URL = %q, second URL = %q, want reused URL", first.URL, second.URL)
	}
	if first.Duplicated {
		t.Fatal("first duplicated = true, want false")
	}
	if !second.Duplicated {
		t.Fatal("second duplicated = false, want true")
	}
	largeFiles, err := filepath.Glob(filepath.Join(tempDir, "uploads", "images", "*", "*", "*_large.webp"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(largeFiles) != 1 {
		t.Fatalf("large WebP count = %d, want 1", len(largeFiles))
	}
}

func TestUploadImageRegeneratesWhenDeduplicatedFileIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	assetStore := newMemoryUploadAssetStore()
	router := newUploadControllerTestRouter(service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images"), assetStore))

	first := uploadImageRequest(t, router, "product.png", "image/png", pngBytes())
	missingPath := filepath.Join(tempDir, strings.TrimPrefix(first.Variants.Card, "/"))
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	second := uploadImageRequest(t, router, "product-again.png", "image/png", pngBytes())
	if second.Duplicated {
		t.Fatal("duplicated = true, want false when recorded variant is missing")
	}
	if _, err := os.Stat(missingPath); err != nil {
		t.Fatalf("regenerated card variant Stat returned error: %v", err)
	}

	third := uploadImageRequest(t, router, "product-third.png", "image/png", pngBytes())
	if !third.Duplicated || third.URL != second.URL {
		t.Fatalf("third = %+v, want duplicate of regenerated URL %q", third, second.URL)
	}
}

func TestUploadVideoAllowsMP4AndUsesVideoPath(t *testing.T) {
	tempDir := t.TempDir()
	content := mp4Bytes()
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
	)
	body, contentType := multipartBodyWithFileContentType(t, "file", "product.mp4", "video/mp4", content)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var payload struct {
		Data struct {
			URL        string `json:"url"`
			Duplicated bool   `json:"duplicated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !strings.HasPrefix(payload.Data.URL, "/uploads/videos/") || !strings.HasSuffix(payload.Data.URL, ".mp4") {
		t.Fatalf("url = %q, want dated video upload path", payload.Data.URL)
	}
	if payload.Data.Duplicated {
		t.Fatal("duplicated = true, want false for first upload")
	}
	savedContent, err := os.ReadFile(filepath.Join(tempDir, strings.TrimPrefix(payload.Data.URL, "/")))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !bytes.Equal(savedContent, content) {
		t.Fatalf("saved content = %v, want %v", savedContent, content)
	}
	savedInfo, err := os.Stat(filepath.Join(tempDir, strings.TrimPrefix(payload.Data.URL, "/")))
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if savedInfo.Mode().Perm() != 0o644 {
		t.Fatalf("saved mode = %o, want 644", savedInfo.Mode().Perm())
	}
}

func TestUploadVideoDeduplicatesSameContentAcrossFileNames(t *testing.T) {
	tempDir := t.TempDir()
	assetStore := newMemoryUploadAssetStore()
	videoStorage := service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos"), assetStore)
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		videoStorage,
	)

	first := uploadVideoRequest(t, router, "first.mp4", mp4Bytes())
	second := uploadVideoRequest(t, router, "renamed.MP4", mp4Bytes())

	if first.URL == "" || second.URL != first.URL {
		t.Fatalf("first URL = %q, second URL = %q, want reused URL", first.URL, second.URL)
	}
	if first.Duplicated {
		t.Fatal("first duplicated = true, want false")
	}
	if !second.Duplicated {
		t.Fatal("second duplicated = false, want true")
	}
	if second.Message != "检测到相同视频，已复用已上传视频" {
		t.Fatalf("message = %q, want duplicate reuse message", second.Message)
	}
	if second.MessageEN != "Duplicate video detected. Existing uploaded video was reused." {
		t.Fatalf("message_en = %q, want English duplicate reuse message", second.MessageEN)
	}

	videoFiles, err := filepath.Glob(filepath.Join(tempDir, "uploads", "videos", "*", "*", "*.mp4"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(videoFiles) != 1 {
		t.Fatalf("saved video count = %d, want 1", len(videoFiles))
	}
}

func TestUploadVideoDoesNotDeduplicateDifferentContentWithSameName(t *testing.T) {
	tempDir := t.TempDir()
	assetStore := newMemoryUploadAssetStore()
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos"), assetStore),
	)

	first := uploadVideoRequest(t, router, "product.mp4", mp4Bytes())
	second := uploadVideoRequest(t, router, "product.mp4", append(mp4Bytes(), 0x01))

	if first.URL == second.URL {
		t.Fatalf("both URLs = %q, want different URLs for different content", first.URL)
	}
	if second.Duplicated {
		t.Fatal("second duplicated = true, want false for different content")
	}
}

func TestUploadVideoReplacesStaleDeduplicationRecord(t *testing.T) {
	tempDir := t.TempDir()
	assetStore := newMemoryUploadAssetStore()
	videoStorage := service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos"), assetStore)
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		videoStorage,
	)

	first := uploadVideoRequest(t, router, "product.mp4", mp4Bytes())
	if err := os.Remove(filepath.Join(tempDir, strings.TrimPrefix(first.URL, "/"))); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	second := uploadVideoRequest(t, router, "product-again.mp4", mp4Bytes())
	if second.Duplicated {
		t.Fatal("duplicated = true, want false when recorded file is missing")
	}
	if second.URL == first.URL {
		t.Fatalf("second URL = %q, want replacement URL", second.URL)
	}
	if _, err := os.Stat(filepath.Join(tempDir, strings.TrimPrefix(second.URL, "/"))); err != nil {
		t.Fatalf("replacement file Stat returned error: %v", err)
	}

	third := uploadVideoRequest(t, router, "product-third.mp4", mp4Bytes())
	if !third.Duplicated || third.URL != second.URL {
		t.Fatalf("third = %+v, want duplicate of replacement URL %q", third, second.URL)
	}
}

func TestVideoStorageRepairsExistingPublicReadPermissions(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "uploads", "videos")
	videoDir := filepath.Join(rootDir, "2026", "06")
	if err := os.MkdirAll(videoDir, 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	videoPath := filepath.Join(videoDir, "existing.mp4")
	if err := os.WriteFile(videoPath, mp4Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Chmod(videoDir, 0o700); err != nil {
		t.Fatalf("Chmod directory returned error: %v", err)
	}

	storage := service.NewLocalVideoStorage(rootDir)
	if err := storage.EnsurePublicReadPermissions(); err != nil {
		t.Fatalf("EnsurePublicReadPermissions returned error: %v", err)
	}

	dirInfo, err := os.Stat(videoDir)
	if err != nil {
		t.Fatalf("directory Stat returned error: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o755 {
		t.Fatalf("directory mode = %o, want 755", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		t.Fatalf("file Stat returned error: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %o, want 644", fileInfo.Mode().Perm())
	}
}

func TestUploadVideoAllowsSupportedFormatsAndContentTypes(t *testing.T) {
	cases := []struct {
		name        string
		filename    string
		contentType string
		content     []byte
		wantSuffix  string
	}{
		{
			name:        "mp4 with content type parameters",
			filename:    "product.mp4",
			contentType: "video/mp4; charset=binary",
			content:     mp4Bytes(),
			wantSuffix:  ".mp4",
		},
		{
			name:        "uppercase mp4 with octet stream",
			filename:    "product.MP4",
			contentType: "application/octet-stream",
			content:     mp4Bytes(),
			wantSuffix:  ".mp4",
		},
		{
			name:        "mp4 with empty content type",
			filename:    "product.mp4",
			contentType: "",
			content:     mp4Bytes(),
			wantSuffix:  ".mp4",
		},
		{
			name:        "m4v",
			filename:    "product.m4v",
			contentType: "video/x-m4v",
			content:     mp4Bytes(),
			wantSuffix:  ".m4v",
		},
		{
			name:        "mov",
			filename:    "product.mov",
			contentType: "video/quicktime",
			content:     mp4Bytes(),
			wantSuffix:  ".mov",
		},
		{
			name:        "webm",
			filename:    "product.webm",
			contentType: "video/webm",
			content:     webmBytes(),
			wantSuffix:  ".webm",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			router := newUploadControllerTestRouterWithVideo(
				service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
				service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
			)
			body, contentType := multipartBodyWithFileContentType(t, "file", tt.filename, tt.contentType, tt.content)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
			request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
			request.Header.Set("Content-Type", contentType)

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "/uploads/videos/") || !strings.Contains(response.Body.String(), tt.wantSuffix) {
				t.Fatalf("body = %s, want video URL ending in %s", response.Body.String(), tt.wantSuffix)
			}
		})
	}
}

func TestUploadVideoRejectsExecutableWithOctetStream(t *testing.T) {
	tempDir := t.TempDir()
	var logOutput bytes.Buffer
	previousLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	defer log.SetOutput(previousLogOutput)

	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
	)
	body, contentType := multipartBodyWithFileContentType(
		t,
		"file",
		"malware.exe",
		"application/octet-stream",
		mp4Bytes(),
	)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "不支持的视频格式，请上传 MP4、WebM、MOV 或 M4V 视频") {
		t.Fatalf("body = %s, want localized unsupported format error", response.Body.String())
	}
	for _, field := range []string{
		`filename="malware.exe"`,
		`ext=".exe"`,
		`content_type="application/octet-stream"`,
		`detected_content_type="video/mp4"`,
		"size=24",
	} {
		if !strings.Contains(logOutput.String(), field) {
			t.Fatalf("log = %q, want field %q", logOutput.String(), field)
		}
	}
}

func TestUploadVideoReturnsEnglishUnsupportedFormatError(t *testing.T) {
	tempDir := t.TempDir()
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
	)
	body, contentType := multipartBodyWithFileContentType(t, "file", "malware.exe", "application/octet-stream", mp4Bytes())
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Unsupported video format. Please upload MP4, WebM, MOV or M4V video.") {
		t.Fatalf("body = %s, want English unsupported format error", response.Body.String())
	}
}

func TestUploadVideoRejectsFileLargerThan50MB(t *testing.T) {
	tempDir := t.TempDir()
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
	)
	body, contentType, contentLength := multipartFileBody(
		t,
		"file",
		"oversized.mp4",
		"video/mp4",
		service.MaxUploadVideoSize+1,
	)
	defer body.Close()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.ContentLength = contentLength
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)
	if request.MultipartForm != nil {
		defer request.MultipartForm.RemoveAll()
	}

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "上传文件超过大小限制") {
		t.Fatalf("body = %s, want size limit error", response.Body.String())
	}
}

func TestUploadVideoRejectsScriptDisguisedAsMP4(t *testing.T) {
	tempDir := t.TempDir()
	router := newUploadControllerTestRouterWithVideo(
		service.NewLocalStorage(filepath.Join(tempDir, "uploads", "images")),
		service.NewLocalVideoStorage(filepath.Join(tempDir, "uploads", "videos")),
	)
	body, contentType := multipartBody(t, "file", "attack.mp4", []byte("<script>alert(1)</script>"))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func multipartBody(t *testing.T, fieldName string, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	return body, writer.FormDataContentType()
}

func multipartBodyWithFileContentType(
	t *testing.T,
	fieldName string,
	filename string,
	fileContentType string,
	content []byte,
) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	if fileContentType != "" {
		header.Set("Content-Type", fileContentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart returned error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	return body, writer.FormDataContentType()
}

func multipartFileBody(
	t *testing.T,
	fieldName string,
	filename string,
	fileContentType string,
	size int64,
) (*os.File, string, int64) {
	t.Helper()
	body, err := os.CreateTemp(t.TempDir(), "multipart-upload-*")
	if err != nil {
		t.Fatalf("CreateTemp returned error: %v", err)
	}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	header.Set("Content-Type", fileContentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		body.Close()
		t.Fatalf("CreatePart returned error: %v", err)
	}
	if _, err := io.CopyN(part, repeatedByteReader{}, size); err != nil {
		body.Close()
		t.Fatalf("CopyN returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		body.Close()
		t.Fatalf("Close returned error: %v", err)
	}
	info, err := body.Stat()
	if err != nil {
		body.Close()
		t.Fatalf("Stat returned error: %v", err)
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		body.Close()
		t.Fatalf("Seek returned error: %v", err)
	}
	return body, writer.FormDataContentType(), info.Size()
}

type repeatedByteReader struct{}

func (repeatedByteReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

type uploadImageResponse struct {
	URL        string                      `json:"url"`
	Variants   service.ImageUploadVariants `json:"variants"`
	Duplicated bool                        `json:"duplicated"`
}

func uploadImageRequest(
	t *testing.T,
	router *gin.Engine,
	filename string,
	fileContentType string,
	content []byte,
) uploadImageResponse {
	t.Helper()
	body, contentType := multipartBodyWithFileContentType(t, "file", filename, fileContentType, content)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/image", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Data uploadImageResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	return payload.Data
}

type uploadVideoResponse struct {
	URL        string `json:"url"`
	Duplicated bool   `json:"duplicated"`
	Message    string `json:"message"`
	MessageEN  string `json:"message_en"`
}

func uploadVideoRequest(t *testing.T, router *gin.Engine, filename string, content []byte) uploadVideoResponse {
	t.Helper()
	body, contentType := multipartBodyWithFileContentType(t, "file", filename, "video/mp4", content)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads/video", body)
	request.Header.Set("Authorization", "Bearer "+uploadTestToken(t, models.UserRoleTenantAdmin, uploadUintPtr(1001)))
	request.Header.Set("Content-Type", contentType)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Data uploadVideoResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	return payload.Data
}

type memoryUploadAssetStore struct {
	assets map[string]models.UploadAsset
}

func newMemoryUploadAssetStore() *memoryUploadAssetStore {
	return &memoryUploadAssetStore{assets: make(map[string]models.UploadAsset)}
}

func (s *memoryUploadAssetStore) FindByHash(
	_ context.Context,
	hash string,
	assetType string,
) (*models.UploadAsset, bool, error) {
	asset, ok := s.assets[assetType+":"+hash]
	if !ok {
		return nil, false, nil
	}
	cloned := asset
	return &cloned, true, nil
}

func (s *memoryUploadAssetStore) Create(_ context.Context, asset *models.UploadAsset) error {
	s.assets[asset.AssetType+":"+asset.Hash] = *asset
	return nil
}

func (s *memoryUploadAssetStore) UpdateByHash(_ context.Context, asset *models.UploadAsset) error {
	key := asset.AssetType + ":" + asset.Hash
	if _, ok := s.assets[key]; !ok {
		return gorm.ErrRecordNotFound
	}
	s.assets[key] = *asset
	return nil
}

func pngBytes() []byte {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, testImage(64, 48)); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

func jpegBytes() []byte {
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, testImage(80, 60), &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

func webpBytes(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := webp.Encode(&buffer, testImage(72, 54), webp.Options{Quality: 90}); err != nil {
		t.Fatalf("webp Encode returned error: %v", err)
	}
	return buffer.Bytes()
}

func testImage(width int, height int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			alpha := uint8(255)
			if x < width/3 && y < height/3 {
				alpha = 160
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(40 + x%180),
				G: uint8(80 + y%140),
				B: uint8(120 + (x+y)%100),
				A: alpha,
			})
		}
	}
	return img
}

func mp4Bytes() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm',
		0x00, 0x00, 0x00, 0x00,
		'i', 's', 'o', 'm',
		'm', 'p', '4', '2',
	}
}

func webmBytes() []byte {
	return []byte{
		0x1a, 0x45, 0xdf, 0xa3,
		0x9f, 0x42, 0x86, 0x81,
		0x01, 0x42, 0xf7, 0x81,
		0x01, 0x42, 0xf2, 0x81,
		0x04, 0x42, 0xf3, 0x81,
		0x08, 0x42, 0x82, 0x84,
		'w', 'e', 'b', 'm',
	}
}

func uploadTestToken(t *testing.T, role models.UserRole, tenantID *uint) string {
	t.Helper()
	token, err := auth.GenerateToken("secret", time.Hour, &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  tenantID,
		Role:      role,
	})
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	return token
}

func uploadUintPtr(value uint) *uint {
	return &value
}
