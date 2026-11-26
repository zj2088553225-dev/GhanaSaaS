package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"saas/internal/models"
)

const (
	MaxUploadImageSize int64 = 5 << 20
	MaxUploadVideoSize int64 = 50 << 20
)

var allowedVideoContentTypes = map[string]struct{}{
	"":                         {},
	"application/octet-stream": {},
	"video/mp4":                {},
	"video/quicktime":          {},
	"video/webm":               {},
	"video/x-m4v":              {},
}

type StorageService interface {
	UploadFile(ctx context.Context, file *multipart.FileHeader) (string, error)
}

type VideoUploadResult struct {
	URL        string
	Duplicated bool
}

type VideoStorageService interface {
	UploadVideoFile(ctx context.Context, file *multipart.FileHeader) (*VideoUploadResult, error)
}

type uploadAssetStore interface {
	FindByHash(ctx context.Context, hash string, assetType string) (*models.UploadAsset, bool, error)
	Create(ctx context.Context, asset *models.UploadAsset) error
	UpdateByHash(ctx context.Context, asset *models.UploadAsset) error
}

type LocalStorage struct {
	rootDir           string
	maxSize           int64
	allowedExtensions map[string]struct{}
	kind              string
	assetStore        uploadAssetStore
}

func NewLocalStorage(rootDir string, assetStore ...uploadAssetStore) *LocalStorage {
	storage := &LocalStorage{
		rootDir: rootDir,
		maxSize: MaxUploadImageSize,
		kind:    "image",
	}
	if len(assetStore) > 0 {
		storage.assetStore = assetStore[0]
	}
	return storage
}

func NewLocalVideoStorage(rootDir string, assetStore ...uploadAssetStore) *LocalStorage {
	storage := &LocalStorage{
		rootDir: rootDir,
		maxSize: MaxUploadVideoSize,
		allowedExtensions: map[string]struct{}{
			".mp4":  {},
			".webm": {},
			".mov":  {},
			".m4v":  {},
		},
		kind: "video",
	}
	if len(assetStore) > 0 {
		storage.assetStore = assetStore[0]
	}
	return storage
}

func (s *LocalStorage) EnsurePublicReadPermissions() error {
	if s.kind != "video" {
		return nil
	}
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(s.rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}

func (s *LocalStorage) UploadFile(ctx context.Context, fileHeader *multipart.FileHeader) (string, error) {
	if s.kind == "video" {
		result, err := s.UploadVideoFile(ctx, fileHeader)
		if err != nil {
			return "", err
		}
		return result.URL, nil
	}

	result, err := s.UploadImageFile(ctx, fileHeader)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (s *LocalStorage) UploadImageFile(ctx context.Context, fileHeader *multipart.FileHeader) (*ImageUploadResult, error) {
	if s.kind != "image" {
		return nil, fmt.Errorf("%w: storage is not configured for image", ErrInvalidInput)
	}
	if fileHeader == nil {
		return nil, fmt.Errorf("%w: missing file", ErrInvalidInput)
	}
	if fileHeader.Size <= 0 || fileHeader.Size > s.maxSize {
		return nil, fmt.Errorf("%w: image exceeds upload size limit", ErrInvalidInput)
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, s.maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != fileHeader.Size {
		return nil, fmt.Errorf("%w: uploaded image size changed during processing", ErrInvalidInput)
	}
	if int64(len(data)) > s.maxSize {
		return nil, fmt.Errorf("%w: image exceeds upload size limit", ErrInvalidInput)
	}

	optimized, err := OptimizeImageBytes(data, fileHeader.Filename)
	if err != nil {
		return nil, err
	}

	existingAsset, assetRecordFound, err := s.findImageAsset(ctx, optimized.Hash)
	if err != nil {
		return nil, err
	}
	if assetRecordFound {
		existingResult := s.imageResultFromAsset(existingAsset)
		if s.imageAssetExists(existingResult) {
			existingResult.Duplicated = true
			return existingResult, nil
		}
	}

	result, paths, err := s.writeOptimizedImageFiles(optimized)
	if err != nil {
		return nil, err
	}

	if s.assetStore != nil {
		largeInfo, statErr := os.Stat(paths[ImageVariantLarge])
		if statErr != nil {
			s.removeImageFiles(paths)
			return nil, statErr
		}
		asset := &models.UploadAsset{
			Hash:      optimized.Hash,
			URL:       result.URL,
			FileName:  fileHeader.Filename,
			MimeType:  "image/webp",
			FileSize:  largeInfo.Size(),
			AssetType: "image",
		}
		if assetRecordFound {
			if err := s.assetStore.UpdateByHash(ctx, asset); err != nil {
				s.removeImageFiles(paths)
				return nil, err
			}
		} else if err := s.assetStore.Create(ctx, asset); err != nil {
			if existing, found, findErr := s.findExistingImage(ctx, optimized.Hash); findErr == nil && found {
				s.removeImageFiles(paths)
				existing.Duplicated = true
				return existing, nil
			}
			s.removeImageFiles(paths)
			return nil, err
		}
	}

	return result, nil
}

func (s *LocalStorage) writeOptimizedImageFiles(optimized *OptimizedImage) (*ImageUploadResult, map[string]string, error) {
	now := time.Now()
	relativeDir := filepath.Join(s.rootDir, fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", int(now.Month())))
	if err := os.MkdirAll(relativeDir, 0o755); err != nil {
		return nil, nil, err
	}

	baseName := OptimizedImageFilenameBase(optimized.Hash)
	paths := map[string]string{
		ImageVariantOriginal: filepath.Join(relativeDir, baseName+"_original"+optimized.OriginalExtension),
	}
	for _, spec := range optimizedImageVariantSpecs {
		paths[spec.name] = filepath.Join(relativeDir, baseName+spec.suffix+".webp")
	}

	if err := writeFileAtomic(paths[ImageVariantOriginal], optimized.Original); err != nil {
		return nil, paths, err
	}
	for _, spec := range optimizedImageVariantSpecs {
		if err := writeFileAtomic(paths[spec.name], optimized.Files[spec.name]); err != nil {
			return nil, paths, err
		}
	}

	variants := ImageUploadVariants{
		Thumb:    publicUploadPath(paths[ImageVariantThumb]),
		Card:     publicUploadPath(paths[ImageVariantCard]),
		Large:    publicUploadPath(paths[ImageVariantLarge]),
		Original: publicUploadPath(paths[ImageVariantOriginal]),
	}
	return &ImageUploadResult{
		URL:      variants.Large,
		Variants: variants,
	}, paths, nil
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (s *LocalStorage) findImageAsset(ctx context.Context, hash string) (*models.UploadAsset, bool, error) {
	if s.assetStore == nil {
		return nil, false, nil
	}
	return s.assetStore.FindByHash(ctx, hash, "image")
}

func (s *LocalStorage) findExistingImage(ctx context.Context, hash string) (*ImageUploadResult, bool, error) {
	asset, found, err := s.findImageAsset(ctx, hash)
	if err != nil || !found {
		return nil, found, err
	}
	result := s.imageResultFromAsset(asset)
	if !s.imageAssetExists(result) {
		return nil, false, nil
	}
	return result, true, nil
}

func (s *LocalStorage) imageResultFromAsset(asset *models.UploadAsset) *ImageUploadResult {
	if asset == nil {
		return nil
	}
	variants := imageVariantsFromLargeURL(asset.URL, asset.FileName)
	return &ImageUploadResult{
		URL:      variants.Large,
		Variants: variants,
	}
}

func imageVariantsFromLargeURL(largeURL string, originalFilename string) ImageUploadVariants {
	baseURL := strings.TrimSuffix(largeURL, "_large.webp")
	if baseURL == largeURL {
		baseURL = strings.TrimSuffix(largeURL, ".webp")
	}
	originalExtension := normalizedOriginalImageExtension(originalFilename)
	return ImageUploadVariants{
		Thumb:    baseURL + "_thumb.webp",
		Card:     baseURL + "_card.webp",
		Large:    baseURL + "_large.webp",
		Original: baseURL + "_original" + originalExtension,
	}
}

func normalizedOriginalImageExtension(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpeg", ".jpg":
		return ".jpg"
	case ".png":
		return ".png"
	case ".webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func (s *LocalStorage) imageAssetExists(result *ImageUploadResult) bool {
	if result == nil {
		return false
	}
	urls := []string{
		result.Variants.Thumb,
		result.Variants.Card,
		result.Variants.Large,
		result.Variants.Original,
	}
	if baseURL := strings.TrimSuffix(result.Variants.Large, "_large.webp"); baseURL != result.Variants.Large {
		urls = append(urls, baseURL+".webp")
	}

	seen := make(map[string]struct{}, len(urls))
	for _, url := range urls {
		if url == "" {
			return false
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		if !s.uploadURLExists(url) {
			return false
		}
	}
	return true
}

func (s *LocalStorage) removeImageFiles(paths map[string]string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func (s *LocalStorage) UploadVideoFile(ctx context.Context, fileHeader *multipart.FileHeader) (*VideoUploadResult, error) {
	if s.kind != "video" {
		return nil, fmt.Errorf("%w: storage is not configured for video", ErrInvalidInput)
	}
	if fileHeader == nil {
		return nil, fmt.Errorf("%w: missing file", ErrInvalidInput)
	}

	extension := strings.ToLower(filepath.Ext(fileHeader.Filename))
	contentType := normalizeContentType(fileHeader.Header.Get("Content-Type"))
	if fileHeader.Size <= 0 || fileHeader.Size > s.maxSize {
		s.logVideoRejection(fileHeader, extension, contentType, "")
		return nil, fmt.Errorf("%w: video exceeds upload size limit", ErrInvalidInput)
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	header := make([]byte, 512)
	n, err := io.ReadFull(src, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	detectedContentType := normalizeContentType(http.DetectContentType(header[:n]))
	if !s.validVideo(extension, contentType, detectedContentType, header[:n]) {
		s.logVideoRejection(fileHeader, extension, contentType, detectedContentType)
		return nil, ErrUnsupportedVideoFormat
	}
	if seeker, ok := src.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("%w: uploaded file is not seekable", ErrInvalidInput)
	}

	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return nil, err
	}
	tempFile, err := os.CreateTemp(s.rootDir, ".video-upload-*")
	if err != nil {
		return nil, err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tempFile, hasher), src)
	closeErr := tempFile.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if written != fileHeader.Size {
		return nil, fmt.Errorf("%w: uploaded video size changed during processing", ErrInvalidInput)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	existingAsset, assetRecordFound, err := s.findVideoAsset(ctx, hash)
	if err != nil {
		return nil, err
	}
	if assetRecordFound && s.videoAssetExists(existingAsset.URL) {
		return &VideoUploadResult{URL: existingAsset.URL, Duplicated: true}, nil
	}

	now := time.Now()
	relativeDir := filepath.Join(s.rootDir, fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", int(now.Month())))
	if err := os.MkdirAll(relativeDir, 0o755); err != nil {
		return nil, err
	}
	filename, err := randomFilename(extension)
	if err != nil {
		return nil, err
	}
	destinationPath := filepath.Join(relativeDir, filename)
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, destinationPath); err != nil {
		return nil, err
	}
	url := publicUploadPath(destinationPath)

	if s.assetStore != nil {
		asset := &models.UploadAsset{
			Hash:      hash,
			URL:       url,
			FileName:  fileHeader.Filename,
			MimeType:  detectedContentType,
			FileSize:  fileHeader.Size,
			AssetType: "video",
		}
		if assetRecordFound {
			if err := s.assetStore.UpdateByHash(ctx, asset); err != nil {
				_ = os.Remove(destinationPath)
				return nil, err
			}
		} else if err := s.assetStore.Create(ctx, asset); err != nil {
			if existing, found, findErr := s.findExistingVideo(ctx, hash); findErr == nil && found {
				_ = os.Remove(destinationPath)
				return &VideoUploadResult{URL: existing.URL, Duplicated: true}, nil
			}
			_ = os.Remove(destinationPath)
			return nil, err
		}
	}

	return &VideoUploadResult{URL: url, Duplicated: false}, nil
}

func (s *LocalStorage) findVideoAsset(ctx context.Context, hash string) (*models.UploadAsset, bool, error) {
	if s.assetStore == nil {
		return nil, false, nil
	}
	return s.assetStore.FindByHash(ctx, hash, "video")
}

func (s *LocalStorage) findExistingVideo(ctx context.Context, hash string) (*models.UploadAsset, bool, error) {
	asset, found, err := s.findVideoAsset(ctx, hash)
	if err != nil || !found {
		return asset, found, err
	}
	if !s.videoAssetExists(asset.URL) {
		return nil, false, nil
	}
	return asset, true, nil
}

func (s *LocalStorage) videoAssetExists(url string) bool {
	return s.uploadURLExists(url)
}

func (s *LocalStorage) uploadURLExists(url string) bool {
	rootURL := strings.TrimSuffix(publicUploadPath(s.rootDir), "/")
	if !strings.HasPrefix(url, rootURL+"/") {
		return false
	}
	relativePath := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(url, rootURL+"/")))
	if relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(filepath.Join(s.rootDir, relativePath))
	return err == nil && !info.IsDir()
}

func (s *LocalStorage) validVideo(extension string, contentType string, detectedContentType string, header []byte) bool {
	if _, ok := s.allowedExtensions[extension]; !ok {
		return false
	}
	if _, ok := allowedVideoContentTypes[contentType]; !ok {
		return false
	}

	switch extension {
	case ".mp4", ".mov", ".m4v":
		return bytes.Contains(header, []byte("ftyp"))
	case ".webm":
		return detectedContentType == "video/webm" || hasWebMHeader(header)
	default:
		return false
	}
}

func hasWebMHeader(header []byte) bool {
	return len(header) >= 4 &&
		header[0] == 0x1a &&
		header[1] == 0x45 &&
		header[2] == 0xdf &&
		header[3] == 0xa3 &&
		bytes.Contains(bytes.ToLower(header), []byte("webm"))
}

func normalizeContentType(contentType string) string {
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func (s *LocalStorage) logVideoRejection(fileHeader *multipart.FileHeader, extension string, contentType string, detectedContentType string) {
	if s.kind != "video" {
		return
	}
	log.Printf(
		"video upload rejected filename=%q ext=%q content_type=%q detected_content_type=%q size=%d",
		fileHeader.Filename,
		extension,
		contentType,
		detectedContentType,
		fileHeader.Size,
	)
}

func randomFilename(extension string) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes) + extension, nil
}

func publicUploadPath(path string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for index, part := range parts {
		if part == "uploads" {
			return "/" + strings.Join(parts[index:], "/")
		}
	}
	return "/uploads/" + filepath.Base(path)
}
