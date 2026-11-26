package service

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("not found")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrConflict           = errors.New("conflict")
	ErrRateLimited        = errors.New("rate limited")
	ErrServiceUnavailable = errors.New("service unavailable")

	ErrInvalidCredentials     = &contextualError{message: "invalid account or password", target: ErrUnauthorized}
	ErrPhoneExists            = &contextualError{message: "phone already registered", target: ErrConflict}
	ErrEmailExists            = &contextualError{message: "email already registered", target: ErrConflict}
	ErrInvalidEmail           = &contextualError{message: "enter a valid email address", target: ErrInvalidInput}
	ErrWeakPassword           = &contextualError{message: "password must be 8 to 72 bytes and at least 8 characters", target: ErrInvalidInput}
	ErrInvalidResetToken      = &contextualError{message: "reset token is invalid or expired", target: ErrInvalidInput}
	ErrUnsupportedVideoFormat = &contextualError{
		message: "unsupported video format",
		target:  ErrInvalidInput,
	}

	ErrPlatformCategoryNotFound    = &contextualError{message: "类目不存在，请刷新后重试", target: ErrNotFound}
	ErrPlatformCategoryDuplicate   = &contextualError{message: "同级类目已存在", target: ErrConflict}
	ErrPlatformCategoryHasProducts = &contextualError{
		message: "该类目或子类目已被商品使用，不能删除",
		target:  ErrInvalidInput,
	}
	ErrPlatformCategoryImportCycle = &contextualError{
		message: "类目层级不能指向自身或自己的子类目",
		target:  ErrInvalidInput,
	}
	ErrPlatformCategorySearchRebuildInProgress = &contextualError{
		message: "平台类目搜索索引正在重建，请稍后重试",
		target:  ErrConflict,
	}
	ErrAdminProductHasOrderRecords = &contextualError{
		message: "该商品已有订单记录，不能删除，请下架或归档",
		target:  ErrConflict,
	}
	ErrAnalyticsExportTooLarge = &contextualError{
		message: "Export result exceeds 10,000 rows. Narrow the date range or filters.",
		target:  ErrInvalidInput,
	}
)

type RateLimitError struct {
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string { return "too many requests" }
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

type contextualError struct {
	message string
	target  error
}

func (e *contextualError) Error() string {
	return e.message
}

func (e *contextualError) Unwrap() error {
	return e.target
}
