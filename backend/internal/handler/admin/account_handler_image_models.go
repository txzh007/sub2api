package admin

import (
	"errors"
	"maps"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PreviewImageModels reads the live upstream list without changing account
// credentials, mappings or capability snapshots. Saved secrets stay server-side.
func (h *AccountHandler) PreviewImageModels(c *gin.Context) {
	var req struct {
		AccountID int64  `json:"account_id" binding:"omitempty,gt=0"`
		Platform  string `json:"platform" binding:"required,oneof=openai gemini grok"`
		BaseURL   string `json:"base_url" binding:"required"`
		APIKey    string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid image provider request")
		return
	}
	account := &service.Account{Platform: req.Platform, Type: service.AccountTypeAPIKey}
	if req.AccountID > 0 {
		saved, err := h.adminService.GetAccount(c.Request.Context(), req.AccountID)
		if err != nil {
			response.NotFound(c, "Account not found")
			return
		}
		if saved.Type != service.AccountTypeAPIKey || saved.Platform != req.Platform {
			response.BadRequest(c, "Image provider must use the saved account type")
			return
		}
		copy := *saved
		account = &copy
		account.Credentials = maps.Clone(saved.Credentials)
	}
	if account.Credentials == nil {
		account.Credentials = make(map[string]any)
	}
	account.Credentials["base_url"] = strings.TrimSpace(req.BaseURL)
	if key := strings.TrimSpace(req.APIKey); key != "" {
		account.Credentials["api_key"] = key
	}
	if account.GetCredential("base_url") == "" || strings.TrimSpace(account.GetCredential("api_key")) == "" {
		response.BadRequest(c, "Base URL and API key are required")
		return
	}
	models, err := h.accountTestService.FetchUpstreamSupportedModels(c.Request.Context(), account)
	if err != nil {
		var syncErr *service.UpstreamModelSyncError
		if errors.As(err, &syncErr) {
			status := http.StatusBadGateway
			switch syncErr.Kind {
			case service.UpstreamModelSyncErrorConfiguration, service.UpstreamModelSyncErrorUnsupported:
				status = http.StatusBadRequest
			case service.UpstreamModelSyncErrorInternal:
				status = http.StatusInternalServerError
			}
			response.Error(c, status, syncErr.SafeMessage())
			return
		}
		response.Error(c, http.StatusBadGateway, "Failed to fetch upstream image models")
		return
	}
	response.Success(c, gin.H{"models": models})
}
