package httpserver

import (
	"net/http"
	"time"

	"agent-runtime/internal/config"
	"agent-runtime/internal/version"
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

func RegisterHealthRoutes(mux *http.ServeMux, cfg config.Config, startedAt time.Time) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

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

	mux.HandleFunc("/healthz", handler)
	mux.HandleFunc("/livez", handler)
	mux.HandleFunc("/readyz", handler)
}
