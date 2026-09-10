package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const imageBridgeBindingKey = "sub2api_image_bridge_binding"
const imageBridgeResolverKey = "sub2api_image_bridge_resolver"

type imageBridgeBinding struct {
	key          *service.APIKey
	subscription *service.UserSubscription
	route        service.CompositeRouteDecision
}

func (h *GatewayHandler) resolveKeyImageBridge(c *gin.Context, key *service.APIKey, model string, resolver *service.CompositeRouteResolver) (*imageBridgeBinding, error) {
	if h.apiKeyService == nil {
		return nil, fmt.Errorf("image bridge configuration service unavailable")
	}
	model = strings.TrimSpace(model)
	imageKey, sub, err := h.apiKeyService.ImageBridgeAPIKey(c.Request.Context(), key, model)
	if err != nil {
		return nil, err
	}
	route := service.CompositeRouteDecision{Matched: true, GroupID: imageKey.Group.ID, PublicModel: model, UpstreamModel: model, TargetPlatform: imageKey.Group.Platform}
	if imageKey.Group.Platform == service.PlatformComposite {
		if resolver == nil {
			resolver = service.NewCompositeRouteResolver(nil)
		}
		route, err = resolver.Resolve(c.Request.Context(), imageKey.Group.ID, model, service.CompositeRouteEndpointImages)
		if err != nil {
			return nil, err
		}
	}
	if !route.Matched || (route.TargetPlatform != service.PlatformGemini && route.TargetPlatform != service.PlatformOpenAI && route.TargetPlatform != service.PlatformGrok) {
		return nil, fmt.Errorf("所选生图模型没有可用的 Images 路由")
	}
	return &imageBridgeBinding{key: imageKey, subscription: sub, route: route}, nil
}

func applyImageBridgeBinding(c *gin.Context, binding *imageBridgeBinding) {
	c.Set(string(middleware.ContextKeyAPIKey), binding.key)
	c.Set(string(middleware.ContextKeySubscription), binding.subscription)
	ctx := context.WithValue(c.Request.Context(), ctxkey.Group, binding.key.Group)
	c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(ctx, binding.route))
}

func (h *GatewayHandler) dispatchImageBridge(c *gin.Context, platform string) {
	if platform == service.PlatformGemini {
		h.GeminiImages(c)
		return
	}
	if h.imageGateway == nil {
		h.responsesErrorResponse(c, http.StatusServiceUnavailable, "api_error", "Image gateway unavailable")
		return
	}
	if platform == service.PlatformGrok {
		h.imageGateway.GrokImages(c)
		return
	}
	h.imageGateway.Images(c)
}

// A per-key selection routes Images requests through the separate image group.
// Key ID, owner, quota and IP restrictions remain those of the original key.
func (h *GatewayHandler) WrapKeyImageBridge(next gin.HandlerFunc, resolver *service.CompositeRouteResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || !service.KeySupportsImageBridge(key) {
			next(c)
			return
		}
		model, enabled := h.apiKeyService.EffectiveImageBridgeModel(key)
		if !enabled {
			next(c)
			return
		}
		binding, err := h.resolveKeyImageBridge(c, key, model, resolver)
		if err != nil {
			h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			if _, tooLarge := extractMaxBytesError(err); tooLarge {
				status = http.StatusRequestEntityTooLarge
			}
			h.responsesErrorResponse(c, status, "invalid_request_error", "Failed to read image request body")
			return
		}
		body, contentType, err := service.RewriteImageBridgeModel(body, c.GetHeader("Content-Type"), binding.route.UpstreamModel)
		if err != nil {
			h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Request.Header.Set("Content-Type", contentType)
		applyImageBridgeBinding(c, binding)
		h.dispatchImageBridge(c, binding.route.TargetPlatform)
	}
}
