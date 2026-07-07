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

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

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
