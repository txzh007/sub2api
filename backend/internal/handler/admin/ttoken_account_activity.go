package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetTTokenAccountActivity batches the visible rows, avoiding one query per account.
func (h *OpsHandler) GetTTokenAccountActivity(c *gin.Context) {
	var req struct {
		AccountIDs    []int64 `json:"account_ids"`
		Hours         int     `json:"hours"`
		IncludeRecent bool    `json:"include_recent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AccountIDs) == 0 || len(req.AccountIDs) > 100 || (req.Hours != 1 && req.Hours != 24) || (req.IncludeRecent && len(req.AccountIDs) != 1) {
		response.BadRequest(c, "Use 1-100 account IDs, hours 1 or 24, and one account for recent requests")
		return
	}
	for _, id := range req.AccountIDs {
		if id <= 0 {
			response.BadRequest(c, "Invalid account ID")
			return
		}
	}
	if h.opsService == nil {
		response.Error(c, 503, "Account activity is unavailable")
		return
	}
	result, err := h.opsService.GetTTokenAccountActivity(c.Request.Context(), req.AccountIDs, req.Hours, req.IncludeRecent)
	if err != nil {
		response.Error(c, 500, "Unable to load recent account activity")
		return
	}
	response.Success(c, result)
}
