package apperrors

import (
	"errors"
	"net/http"
	"testing"
)

func TestFromPreservesAppError(t *testing.T) {
	original := New(CodeInvalidArg, "参数错误")
	got := From(original)
	if got != original {
		t.Fatal("From 没有保留原始 AppError")
	}
}

func TestFromWrapsUnknownError(t *testing.T) {
	got := From(errors.New("boom"))
	if got.Code != CodeInternal {
		t.Fatalf("Code = %s, want %s", got.Code, CodeInternal)
	}
}

func TestHTTPStatus(t *testing.T) {
	if status := HTTPStatus(CodeNotImplemented); status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", status, http.StatusNotImplemented)
	}
}
