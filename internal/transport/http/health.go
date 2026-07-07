package httpserver

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	"agent-runtime/pkg/version"
)

type HealthPayload struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Env       string `json:"env"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Uptime    string `json:"uptime"`
}

func RegisterHealthRoutes(router chi.Router, cfg config.Config, startedAt time.Time) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, HealthPayload{
			Status:    "ok",
			Service:   cfg.ServiceName,
			Env:       cfg.Env,
			Version:   version.Version,
			Commit:    version.Commit,
			BuildTime: version.BuildTime,
			Uptime:    time.Since(startedAt).Round(time.Second).String(),
		})
	}

	router.Get("/healthz", handler)
	router.Get("/livez", handler)
	router.Get("/readyz", handler)
}
