package admin

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type copyToImageProviderRequest struct {
	GroupID int64 `json:"group_id" binding:"required,gt=0"`
}

// CopyToImageProvider creates a paused, credential-owning copy that belongs
// only to the dedicated image group. Secrets never leave the backend.
func (h *AccountHandler) CopyToImageProvider(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req copyToImageProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid image provider copy request")
		return
	}

	actorScope := adminActorScope(c)
	requestKey := c.GetHeader("Idempotency-Key")
	duplicateKey := ""
	if requestKey != "" {
		duplicateKey = fmt.Sprintf("image-provider:%d:%s", req.GroupID, requestKey)
	}
	copyAndBind := func(ctx context.Context) (any, error) {
		source, sourceErr := h.adminService.GetAccount(ctx, accountID)
		if sourceErr != nil {
			return nil, sourceErr
		}
		if !isImageProviderCopySource(source) {
			return nil, infraerrors.BadRequest(
				"IMAGE_PROVIDER_COPY_SOURCE_UNSUPPORTED",
				"only OpenAI, Gemini, or Grok API key accounts can be copied as image providers",
			)
		}
		group, groupErr := h.adminService.GetGroup(ctx, req.GroupID)
		if groupErr != nil {
			return nil, groupErr
		}
		if group == nil || strings.TrimSpace(group.Name) != "生图" || !group.AllowImageGeneration {
			return nil, infraerrors.BadRequest(
				"IMAGE_PROVIDER_COPY_GROUP_INVALID",
				"target group must be the image generation group",
			)
		}
		copied, copyErr := h.adminService.DuplicateAccount(ctx, accountID, actorScope, duplicateKey)
		if copyErr != nil {
			return nil, copyErr
		}
		return h.bindCopiedImageProvider(ctx, copied, req.GroupID)
	}
	result, err := executeAdminIdempotent(
		c,
		"admin.accounts.copy_to_image_provider",
		struct {
			AccountID int64 `json:"account_id"`
			GroupID   int64 `json:"group_id"`
		}{AccountID: accountID, GroupID: req.GroupID},
		service.DefaultWriteIdempotencyTTL(),
		copyAndBind,
	)
	if err != nil {
		reason := infraerrors.Reason(err)
		if duplicateKey != "" && (reason == infraerrors.Reason(service.ErrIdempotencyInProgress) || reason == infraerrors.Reason(service.ErrIdempotencyStoreUnavail)) {
			recovered, recoverErr := h.adminService.RecoverDuplicateAccount(c.Request.Context(), accountID, actorScope, duplicateKey)
			if recoverErr == nil && recovered != nil {
				data, bindErr := h.bindCopiedImageProvider(c.Request.Context(), recovered, req.GroupID)
				if bindErr == nil {
					c.Header("X-Idempotency-Recovered", "true")
					response.Success(c, data)
					return
				}
			}
		}
		response.ErrorFrom(c, err)
		return
	}
	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

func (h *AccountHandler) bindCopiedImageProvider(ctx context.Context, copied *service.Account, groupID int64) (any, error) {
	if copied == nil {
		return nil, fmt.Errorf("copied image provider account is missing")
	}
	credentials := maps.Clone(copied.Credentials)
	if credentials == nil {
		credentials = make(map[string]any)
	}
	imageMapping := service.ImageProviderModelMapping(copied)
	serializedMapping := make(map[string]any, len(imageMapping))
	for requestedModel, upstreamModel := range imageMapping {
		serializedMapping[requestedModel] = upstreamModel
	}
	credentials["model_mapping"] = serializedMapping

	groupIDs := []int64{groupID}
	updated, err := h.adminService.UpdateAccount(ctx, copied.ID, &service.UpdateAccountInput{
		Credentials:           credentials,
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})
	if err != nil {
		return nil, err
	}
	return h.buildAccountResponseWithRuntime(ctx, updated), nil
}

func isImageProviderCopySource(account *service.Account) bool {
	if account == nil || account.Type != service.AccountTypeAPIKey || account.IsCredentialShadow() {
		return false
	}
	switch account.Platform {
	case service.PlatformOpenAI, service.PlatformGemini, service.PlatformGrok:
		return true
	default:
		return false
	}
}
