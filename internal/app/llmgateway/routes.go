package llmgateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"stableagent/internal/config"
	"stableagent/internal/domain/runtimeplan"
	httpserver "stableagent/internal/transport/http"
	llmusecase "stableagent/internal/usecase/llmgateway"
	apperrors "stableagent/pkg/errors"
)

// RegisterRoutes 注册 LLM Gateway 当前阶段的状态路由。
func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	service := llmusecase.NewService()

	router.Get("/llm/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "LLM 统一入口，当前使用可替换 Mock Provider，负责模型路由、token 统计和审计字段",
			"status":  "ready",
		})
	})
	router.Post("/llm/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		var req runtimeplan.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInvalidArg, "请求 JSON 格式错误", err))
			return
		}
		resp, err := service.Chat(r.Context(), req)
		if err != nil {
			httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInternal, "生成 Mock LLM 响应失败", err))
			return
		}
		httpserver.WriteData(w, http.StatusOK, resp)
	})

	logger.Info("llm-gateway 路由注册完成")
	return nil, nil
}
