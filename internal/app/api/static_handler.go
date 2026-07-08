package api

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	httpserver "agent-runtime/internal/transport/http"
	apperrors "agent-runtime/pkg/errors"
)

// registerWebRoutes 注册 React build 静态文件路由，API 路由未命中仍返回 JSON 错误。
func registerWebRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) {
	staticDir := strings.TrimSpace(cfg.WebStaticDir)
	if staticDir == "" {
		return
	}
	if info, err := os.Stat(staticDir); err != nil || !info.IsDir() {
		logger.Info("React 静态目录不存在，跳过前端路由注册", "web_static_dir", staticDir)
		return
	}

	fileServer := http.FileServer(http.Dir(staticDir))
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "接口不存在"))
			return
		}

		cleanPath := path.Clean("/" + r.URL.Path)
		fullPath := filepath.Join(staticDir, strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "前端入口文件不存在"))
			return
		}
		http.ServeFile(w, r, indexPath)
	}
	router.Get("/", handler)
	router.Head("/", handler)
	router.Get("/*", handler)
	router.Head("/*", handler)
}
