package controller

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type UploadController struct {
	imageStorage service.StorageService
	videoStorage service.VideoStorageService
}

type uploadResponse struct {
	URL        string                       `json:"url"`
	Variants   *service.ImageUploadVariants `json:"variants,omitempty"`
	Duplicated *bool                        `json:"duplicated,omitempty"`
	Message    string                       `json:"message,omitempty"`
	MessageEN  string                       `json:"message_en,omitempty"`
}

func NewUploadController(imageStorage service.StorageService, videoStorage ...service.VideoStorageService) *UploadController {
	controller := &UploadController{imageStorage: imageStorage}
	if fallbackVideoStorage, ok := imageStorage.(service.VideoStorageService); ok {
		controller.videoStorage = fallbackVideoStorage
	}
	if len(videoStorage) > 0 && videoStorage[0] != nil {
		controller.videoStorage = videoStorage[0]
	}
	return controller
}

func (ctl *UploadController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/admin/upload", ctl.UploadImage)
	group.POST("/admin/uploads/image", ctl.UploadImage)
	group.POST("/admin/uploads/video", ctl.UploadVideo)
}

func (ctl *UploadController) UploadImage(c *gin.Context) {
	ctl.uploadImage(c)
}

func (ctl *UploadController) UploadVideo(c *gin.Context) {
	if !canUploadAdminMedia(c) {
		writeError(c, service.ErrForbidden)
		return
	}
	file, ok := uploadFileHeader(c, service.MaxUploadVideoSize)
	if !ok {
		return
	}
	if ctl.videoStorage == nil {
		c.JSON(http.StatusInternalServerError, response{Error: "video storage is not configured"})
		return
	}

	result, err := ctl.videoStorage.UploadVideoFile(c.Request.Context(), file)
	if err != nil {
		if errors.Is(err, service.ErrUnsupportedVideoFormat) {
			c.JSON(http.StatusBadRequest, response{Error: unsupportedVideoFormatMessage(c)})
			return
		}
		writeError(c, err)
		return
	}

	duplicated := result.Duplicated
	payload := uploadResponse{URL: result.URL, Duplicated: &duplicated}
	if result.Duplicated {
		payload.Message = "检测到相同视频，已复用已上传视频"
		payload.MessageEN = "Duplicate video detected. Existing uploaded video was reused."
	}
	c.JSON(http.StatusOK, response{Data: payload})
}

func (ctl *UploadController) uploadImage(c *gin.Context) {
	if !canUploadAdminMedia(c) {
		writeError(c, service.ErrForbidden)
		return
	}
	file, ok := uploadFileHeader(c, service.MaxUploadImageSize)
	if !ok {
		return
	}

	if imageStorage, ok := ctl.imageStorage.(service.ImageStorageService); ok {
		result, err := imageStorage.UploadImageFile(c.Request.Context(), file)
		if err != nil {
			writeError(c, err)
			return
		}
		duplicated := result.Duplicated
		payload := uploadResponse{
			URL:        result.URL,
			Variants:   &result.Variants,
			Duplicated: &duplicated,
		}
		c.JSON(http.StatusOK, response{Data: payload})
		return
	}

	path, err := ctl.imageStorage.UploadFile(c.Request.Context(), file)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: uploadResponse{URL: path}})
}

func uploadFileHeader(c *gin.Context, maxSize int64) (*multipart.FileHeader, bool) {
	if c.Request.ContentLength > maxSize+(1<<20) {
		c.JSON(http.StatusBadRequest, response{Error: "上传文件超过大小限制"})
		return nil, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize+(1<<20))
	file, err := c.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusBadRequest, response{Error: "上传文件超过大小限制"})
			return nil, false
		}
		c.JSON(http.StatusBadRequest, response{Error: "file is required"})
		return nil, false
	}
	if file.Size <= 0 || file.Size > maxSize {
		c.JSON(http.StatusBadRequest, response{Error: "上传文件超过大小限制"})
		return nil, false
	}
	return file, true
}

func unsupportedVideoFormatMessage(c *gin.Context) string {
	language := strings.ToLower(strings.TrimSpace(c.GetHeader("Accept-Language")))
	if strings.HasPrefix(language, "en") {
		return "Unsupported video format. Please upload MP4, WebM, MOV or M4V video."
	}
	return "不支持的视频格式，请上传 MP4、WebM、MOV 或 M4V 视频"
}

func canUploadAdminMedia(c *gin.Context) bool {
	role, ok := middleware.RoleFromContext(c)
	if !ok {
		return false
	}
	if role == models.UserRoleSuperAdmin {
		return true
	}
	if role != models.UserRoleTenantAdmin {
		return false
	}
	tenantID, ok := middleware.TenantIDFromContext(c)
	return ok && tenantID != nil && *tenantID != 0
}
