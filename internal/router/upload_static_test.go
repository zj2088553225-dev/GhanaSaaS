package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadStaticRouteServesVideoMIMETypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registerUploadMIMETypes()

	rootDir := t.TempDir()
	videoDir := filepath.Join(rootDir, "videos")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	cases := []struct {
		filename    string
		contentType string
	}{
		{filename: "product.webp", contentType: "image/webp"},
		{filename: "product.mp4", contentType: "video/mp4"},
		{filename: "product.webm", contentType: "video/webm"},
		{filename: "product.mov", contentType: "video/quicktime"},
		{filename: "product.m4v", contentType: "video/mp4"},
	}
	for _, tt := range cases {
		if err := os.WriteFile(filepath.Join(videoDir, tt.filename), []byte("video-content"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) returned error: %v", tt.filename, err)
		}
	}

	engine := gin.New()
	engine.Static("/uploads", rootDir)

	for _, tt := range cases {
		t.Run(tt.filename, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodHead, "/uploads/videos/"+tt.filename, nil)
			engine.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, tt.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", contentType, tt.contentType)
			}
			if response.Header().Get("Accept-Ranges") != "bytes" {
				t.Fatalf("Accept-Ranges = %q, want bytes", response.Header().Get("Accept-Ranges"))
			}
		})
	}
}

func TestUploadStaticRouteSupportsVideoRangeRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registerUploadMIMETypes()

	rootDir := t.TempDir()
	videoDir := filepath.Join(rootDir, "videos")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(videoDir, "product.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	engine := gin.New()
	engine.Static("/uploads", rootDir)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/uploads/videos/product.mp4", nil)
	request.Header.Set("Range", "bytes=2-5")
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d, body = %q", response.Code, http.StatusPartialContent, response.Body.String())
	}
	if response.Body.String() != "2345" {
		t.Fatalf("body = %q, want %q", response.Body.String(), "2345")
	}
	if response.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q, want bytes 2-5/10", response.Header().Get("Content-Range"))
	}
}
