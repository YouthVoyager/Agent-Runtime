package httpserver

import (
	"encoding/json"
	"net/http"

	apperrors "agent-runtime/pkg/errors"
)

type SuccessResponse struct {
	Data any `json:"data"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteData(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, SuccessResponse{Data: data})
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := apperrors.From(err)
	if appErr == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	WriteJSON(w, apperrors.HTTPStatus(appErr.Code), ErrorResponse{
		Error: ErrorBody{
			Code:      string(appErr.Code),
			Message:   appErr.Message,
			RequestID: RequestIDFromContext(r.Context()),
		},
	})
}
