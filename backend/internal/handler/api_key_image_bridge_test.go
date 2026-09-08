//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyImageBridgeUpdateJSONPreservesOmittedNullAndDisabled(t *testing.T) {
	for _, tc := range []struct {
		body    string
		present bool
		value   *string
	}{
		{`{}`, false, nil},
		{`{"image_bridge_model":null}`, true, nil},
		{`{"image_bridge_model":""}`, true, stringPointer("")},
		{`{"image_bridge_model":"gemini-3.1-flash-image"}`, true, stringPointer("gemini-3.1-flash-image")},
	} {
		var req UpdateAPIKeyRequest
		require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
		require.Equal(t, tc.present, req.ImageBridgeModel.Present)
		require.Equal(t, tc.value, req.ImageBridgeModel.Value)
	}
	var req UpdateAPIKeyRequest
	require.Error(t, json.Unmarshal([]byte(`{"image_bridge_model":123}`), &req))
}

func stringPointer(s string) *string { return &s }

func TestAPIKeyImageBridgeCodexKeepsTextInMainGroup(t *testing.T) {
	imageCalls, textCalls := 0, 0
	h, imageKey := newGeminiImagesTestHandler(t, service.PlatformGemini, func(req *http.Request) (*http.Response, error) {
		imageCalls++
		group, ok := req.Context().Value(ctxkey.Group).(*service.Group)
		require.True(t, ok)
		require.Equal(t, int64(42), group.ID)
		return geminiTestImageResponse(), nil
	})
	mainGroup := &service.Group{ID: 5, Platform: service.PlatformOpenAI, Hydrated: true, AllowImageGeneration: false}
	mainKey := *imageKey
	mainKey.GroupID, mainKey.Group = &mainGroup.ID, mainGroup
	mainKey.ImageBridgeModel = stringPointer(service.DefaultGeminiImageModel)
	c, _ := geminiImagesTestContext(&mainKey, "/v1/responses", `{}`)
	mainSub := &service.UserSubscription{ID: 99, GroupID: mainGroup.ID}
	c.Set(string(middleware.ContextKeySubscription), mainSub)
	route := service.CompositeRouteDecision{Matched: true, GroupID: 42, TargetPlatform: service.PlatformGemini, UpstreamModel: service.DefaultGeminiImageModel}
	c.Set(imageBridgeBindingKey, &imageBridgeBinding{key: imageKey, route: route})
	response, _, err := h.runGeminiImageResponses(c, map[string]any{"model": "gpt-5.4", "input": "draw a cup"}, map[string]any{}, route, func(child *gin.Context) {
		textCalls++
		key, ok := middleware.GetAPIKeyFromContext(child)
		require.True(t, ok)
		require.Equal(t, int64(5), *key.GroupID)
		require.Same(t, mainSub, child.MustGet(string(middleware.ContextKeySubscription)))
		if textCalls == 1 {
			child.JSON(200, gin.H{"output": []any{gin.H{"type": "function_call", "name": geminiImageFunction, "call_id": "image_call", "arguments": `{"prompt":"a cup"}`}}})
		} else {
			child.JSON(200, gin.H{"output": []any{}, "status": "completed"})
		}
	})
	require.NoError(t, err)
	require.Equal(t, 1, imageCalls)
	require.Equal(t, 2, textCalls)
	require.Len(t, response["output"], 1)
	require.Same(t, &mainKey, c.MustGet(string(middleware.ContextKeyAPIKey)))
	require.Same(t, mainSub, c.MustGet(string(middleware.ContextKeySubscription)))
}

func TestAPIKeyImageBridgeDisabledOverridesGlobalDefault(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("disabled key must not call image upstream")
		return nil, nil
	})
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	key.ImageBridgeModel = stringPointer("")
	c, w := geminiImagesTestContext(key, "/v1/responses", `{"model":"gpt-5.4","input":"draw a cup","tools":[{"type":"image_generation"}]}`)
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	called := false
	h.WrapGeminiImageResponses(func(child *gin.Context) {
		called = true
		child.JSON(200, gin.H{"unchanged": true})
	}, nil)(c)
	require.True(t, called)
	require.Equal(t, 200, w.Code)
}

func TestAPIKeyImageBridgeSkipsNonOpenAIGroups(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformComposite, service.PlatformAnthropic, service.PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			key := &service.APIKey{Group: &service.Group{ID: 42, Platform: platform}, ImageBridgeModel: stringPointer(service.DefaultGeminiImageModel)}
			h := &GatewayHandler{}
			for _, endpoint := range []string{"/v1/images/generations", "/v1/responses"} {
				c, w := geminiImagesTestContext(key, endpoint, `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}]}`)
				c.Request.Header.Set("User-Agent", "codex_cli_rs")
				next := func(child *gin.Context) { child.JSON(200, gin.H{"unchanged": true}) }
				if endpoint == "/v1/responses" {
					h.WrapGeminiImageResponses(next, nil)(c)
				} else {
					h.WrapKeyImageBridge(next, nil)(c)
				}
				require.JSONEq(t, `{"unchanged":true}`, w.Body.String())
			}
		})
	}
}
