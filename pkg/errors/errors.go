package apperrors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	CodeInternal         Code = "INTERNAL_ERROR"
	CodeInvalidArg       Code = "INVALID_ARGUMENT"
	CodeNotFound         Code = "NOT_FOUND"
	CodeMethodNotAllowed Code = "METHOD_NOT_ALLOWED"
	CodeUnauthorized     Code = "UNAUTHORIZED"
	CodeForbidden        Code = "FORBIDDEN"
	CodeConflict         Code = "CONFLICT"
	CodeUnavailable      Code = "SERVICE_UNAVAILABLE"
	CodeNotImplemented   Code = "NOT_IMPLEMENTED"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	cause   error
}

// New 创建不包裹底层原因的应用错误。
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 创建带底层原因的应用错误，便于上层统一响应且保留排查上下文。
func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}

// Error 返回带错误码、业务消息和底层原因的可读错误文本。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
}

// Unwrap 返回底层错误，支持 errors.Is 和 errors.As 继续匹配根因。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// From 将任意错误转换为应用错误，未知错误默认归类为系统内部错误。
func From(err error) *Error {
	if err == nil {
		return nil
	}

	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return Wrap(CodeInternal, "系统内部错误", err)
}

// HTTPStatus 将应用错误码映射为 HTTP 状态码。
func HTTPStatus(code Code) int {
	switch code {
	case CodeInvalidArg:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodeConflict:
		return http.StatusConflict
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON 按统一错误结构把应用错误写入 HTTP 响应。
func WriteJSON(w http.ResponseWriter, err error) {
	appErr := From(err)
	if appErr == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(HTTPStatus(appErr.Code))
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    string(appErr.Code),
			"message": appErr.Message,
		},
	})
}
