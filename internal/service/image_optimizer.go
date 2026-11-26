package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
)

const (
	OptimizedImageQuality = 80

	ImageVariantOriginal = "original"
	ImageVariantThumb    = "thumb"
	ImageVariantCard     = "card"
	ImageVariantLarge    = "large"
	ImageVariantBase     = "base"
)

type ImageUploadVariants struct {
	Thumb    string `json:"thumb"`
	Card     string `json:"card"`
	Large    string `json:"large"`
	Original string `json:"original"`
}

type ImageUploadResult struct {
	URL        string
	Variants   ImageUploadVariants
	Duplicated bool
}

type ImageStorageService interface {
	UploadImageFile(ctx context.Context, file *multipart.FileHeader) (*ImageUploadResult, error)
}

type OptimizedImage struct {
	Hash              string
	OriginalMimeType  string
	OriginalExtension string
	Width             int
	Height            int
	Files             map[string][]byte
	Original          []byte
}

type imageVariantSpec struct {
	name    string
	suffix  string
	maxSide int
}

var optimizedImageVariantSpecs = []imageVariantSpec{
	{name: ImageVariantBase, suffix: "", maxSide: 1600},
	{name: ImageVariantThumb, suffix: "_thumb", maxSide: 300},
	{name: ImageVariantCard, suffix: "_card", maxSide: 600},
	{name: ImageVariantLarge, suffix: "_large", maxSide: 1600},
}

func OptimizeImageBytes(data []byte, filename string) (*OptimizedImage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty image", ErrInvalidInput)
	}

	extension, mimeType, err := detectImageType(data, filename)
	if err != nil {
		return nil, err
	}

	img, err := decodeUploadImage(data, mimeType)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid image content", ErrInvalidInput)
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: invalid image dimensions", ErrInvalidInput)
	}

	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	files := make(map[string][]byte, len(optimizedImageVariantSpecs))
	for _, spec := range optimizedImageVariantSpecs {
		variant := resizeImage(img, spec.maxSide)
		encoded, err := encodeWebP(variant)
		if err != nil {
			return nil, err
		}
		files[spec.name] = encoded
	}

	return &OptimizedImage{
		Hash:              hash,
		OriginalMimeType:  mimeType,
		OriginalExtension: extension,
		Width:             width,
		Height:            height,
		Files:             files,
		Original:          data,
	}, nil
}

func OptimizedImageFilenameBase(hash string) string {
	if len(hash) > 32 {
		return hash[:32]
	}
	return hash
}

func IsOptimizableImageFilename(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return false
	}

	base := strings.TrimSuffix(filepath.Base(name), extension)
	for _, suffix := range []string{"_thumb", "_card", "_large", "_original"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}

func detectImageType(data []byte, filename string) (extension string, mimeType string, err error) {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return ".jpg", "image/jpeg", nil
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return ".png", "image/png", nil
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return ".webp", "image/webp", nil
	}

	extension = strings.ToLower(filepath.Ext(filename))
	return "", "", fmt.Errorf("%w: unsupported image type %s", ErrInvalidInput, extension)
}

func decodeUploadImage(data []byte, mimeType string) (image.Image, error) {
	reader := bytes.NewReader(data)
	switch mimeType {
	case "image/jpeg":
		return jpeg.Decode(reader)
	case "image/png":
		return png.Decode(reader)
	case "image/webp":
		return webp.Decode(reader, webp.Options{AutoRotate: true})
	default:
		return nil, fmt.Errorf("unsupported image type %s", mimeType)
	}
}

func resizeImage(src image.Image, maxSide int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if maxSide <= 0 || (width <= maxSide && height <= maxSide) {
		return src
	}

	scale := float64(maxSide) / float64(max(width, height))
	targetWidth := max(1, int(math.Round(float64(width)*scale)))
	targetHeight := max(1, int(math.Round(float64(height)*scale)))
	dst := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func encodeWebP(img image.Image) ([]byte, error) {
	var buffer bytes.Buffer
	if err := webp.Encode(&buffer, img, webp.Options{
		Quality: OptimizedImageQuality,
		Method:  webp.DefaultMethod,
		Exact:   true,
	}); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
