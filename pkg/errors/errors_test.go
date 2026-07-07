package apperrors

import (
	"errors"
	"net/http"
	"testing"
)

// TestFromPreservesAppError 验证 From 会保留已经是应用错误的实例。
func TestFromPreservesAppError(t *testing.T) {
	original := New(CodeInvalidArg, "参数错误")
	got := From(original)
	if got != original {
		t.Fatal("From 没有保留原始 AppError")
	}
}

// TestFromWrapsUnknownError 验证未知错误会被包装为内部错误。
func TestFromWrapsUnknownError(t *testing.T) {
	got := From(errors.New("boom"))
	if got.Code != CodeInternal {
		t.Fatalf("Code = %s, want %s", got.Code, CodeInternal)
	}
}

// TestHTTPStatus 验证应用错误码到 HTTP 状态码的映射。
func TestHTTPStatus(t *testing.T) {
	if status := HTTPStatus(CodeNotImplemented); status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", status, http.StatusNotImplemented)
	}
	if status := HTTPStatus(CodeMethodNotAllowed); status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}
