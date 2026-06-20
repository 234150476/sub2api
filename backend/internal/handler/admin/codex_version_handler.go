package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// CodexVersionHandler 处理 Codex CLI 指纹版本相关的管理接口
type CodexVersionHandler struct {
	svc *service.CodexVersionService
}

// NewCodexVersionHandler 构造 handler
func NewCodexVersionHandler(svc *service.CodexVersionService) *CodexVersionHandler {
	return &CodexVersionHandler{svc: svc}
}

// GetStatus 返回当前 Codex CLI 指纹状态
// GET /api/v1/admin/codex-version
func (h *CodexVersionHandler) GetStatus(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.Error(c, http.StatusServiceUnavailable, "codex_version service unavailable")
		return
	}
	response.Success(c, h.svc.Status())
}

// Refresh 强制从 GitHub 拉一次最新版本
// POST /api/v1/admin/codex-version/refresh
//
// 返回成功后的最新状态；如果 GitHub 不可达则返回 502，
// 当前生效版本不变（保留 DB 缓存或配置兜底）。
func (h *CodexVersionHandler) Refresh(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.Error(c, http.StatusServiceUnavailable, "codex_version service unavailable")
		return
	}
	version, err := h.svc.Refresh(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadGateway, "fetch from GitHub failed: "+err.Error())
		return
	}
	response.Success(c, gin.H{
		"version": version,
		"status":  h.svc.Status(),
	})
}
