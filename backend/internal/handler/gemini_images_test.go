//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type geminiImagesTestUpstream struct {
	service.HTTPUpstream
	call func(*http.Request) (*http.Response, error)
}

func (u *geminiImagesTestUpstream) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.call(r)
}

func newGeminiImagesTestHandler(t *testing.T, platform string, upstream func(*http.Request) (*http.Response, error)) (*GatewayHandler, *service.APIKey) {
	t.Helper()
	group := &service.Group{ID: 42, Name: service.ImageBridgeGroupName, SystemRole: service.GroupSystemRoleImageGeneration, Platform: platform, Status: service.StatusActive, Hydrated: true, AllowImageGeneration: true}
	if platform == service.PlatformOpenAI {
		group.Platform = service.PlatformGemini
	}
	account := &service.Account{ID: 9, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Purpose: service.AccountPurposeImageProvider, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{42}, Credentials: map[string]any{"api_key": "test-key", "model_mapping": map[string]any{
		service.DefaultGeminiImageModel: service.DefaultGeminiImageModel,
	}}}
	h, cleanup := newTestGatewayHandler(t, group, []*service.Account{account})
	t.Cleanup(cleanup)
	h.cfg = &config.Config{RunMode: config.RunModeSimple}
	h.geminiCompatService = service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, &geminiImagesTestUpstream{call: upstream}, nil, h.cfg)
	key := &service.APIKey{ID: 7, UserID: 8, GroupID: &group.ID, Group: group, User: &service.User{ID: 8, Balance: 100}}
	if platform == service.PlatformOpenAI {
		settingService := service.NewSettingService(&bridgeSettingRepo{values: map[string]string{
			service.SettingKeyCodexImageModelMapping: `{"builtin_model":"gpt-image-2","target_model":"gemini-3.1-flash-image"}`,
		}}, h.cfg)
		h.apiKeyService = service.ProvideAPIKeyService(&bridgeAccountRepo{account: account}, nil, &bridgeUserRepo{user: key.User}, &bridgeGroupRepo{group: group}, &bridgeSubRepo{}, nil, nil, h.cfg, nil, nil, settingService)
		key.Group = &service.Group{ID: 5, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}
		key.GroupID = &key.Group.ID
	}
	return h, key
}

type bridgeSettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *bridgeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

type bridgeAccountRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *bridgeAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return []service.Account{*r.account}, nil
}

func (r *bridgeAccountRepo) ListByGroup(context.Context, int64) ([]service.Account, error) {
	return []service.Account{*r.account}, nil
}

type bridgeUserRepo struct {
	service.UserRepository
	user *service.User
}

func (r *bridgeUserRepo) GetByID(context.Context, int64) (*service.User, error) { return r.user, nil }

type bridgeGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *bridgeGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}
func (r *bridgeGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return []service.Group{*r.group}, nil
}

type bridgeSubRepo struct {
	service.UserSubscriptionRepository
}

func (*bridgeSubRepo) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

func geminiImagesTestContext(key *service.APIKey, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.UserID, Concurrency: 2})
	return c, recorder
}

func geminiTestImageResponse() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":10,"totalTokenCount":12}}`))}
}

func TestGeminiImagesGatewayGeneration(t *testing.T) {
	calls := 0
	h, key := newGeminiImagesTestHandler(t, service.PlatformGemini, func(req *http.Request) (*http.Response, error) {
		calls++
		require.Contains(t, req.URL.Path, "gemini-3.1-flash-image:generateContent")
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "draw a cat", gjson.GetBytes(body, "contents.0.parts.0.text").String())
		require.Equal(t, "IMAGE", gjson.GetBytes(body, "generationConfig.responseModalities.1").String())
		return geminiTestImageResponse(), nil
	})
	c, w := geminiImagesTestContext(key, "/v1/images/generations", `{"model":"gemini-3.1-flash-image","prompt":"draw a cat"}`)
	h.GeminiImages(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
	require.Equal(t, "aW1hZ2U=", gjson.Get(w.Body.String(), "data.0.b64_json").String())
}

func TestGeminiImagesGatewayPermissionAndUpstreamError(t *testing.T) {
	calls := 0
	h, key := newGeminiImagesTestHandler(t, service.PlatformGemini, func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"code":400,"message":"unsupported image size","status":"INVALID_ARGUMENT"}}`))}, nil
	})
	key.Group.AllowImageGeneration = false
	c, w := geminiImagesTestContext(key, "/v1/images/generations", `{"prompt":"draw"}`)
	h.GeminiImages(c)
	require.Equal(t, 403, w.Code)
	require.Zero(t, calls)
	key.Group.AllowImageGeneration = true
	c, w = geminiImagesTestContext(key, "/v1/images/generations", `{"prompt":"draw"}`)
	h.GeminiImages(c)
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Equal(t, "invalid_request_error", gjson.Get(w.Body.String(), "error.type").String())
	require.Contains(t, w.Body.String(), "unsupported image size")
}

func TestGeminiImagesCodexHostedTool(t *testing.T) {
	imageCalls, textCalls := 0, 0
	var billingRequestIDs []string
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(req *http.Request) (*http.Response, error) {
		imageCalls++
		billingID, _ := req.Context().Value(ctxkey.ClientRequestID).(string)
		billingRequestIDs = append(billingRequestIDs, billingID)
		return geminiTestImageResponse(), nil
	})
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	text := func(c *gin.Context) {
		textCalls++
		billingID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		billingRequestIDs = append(billingRequestIDs, billingID)
		body, _ := io.ReadAll(c.Request.Body)
		require.False(t, gjson.GetBytes(body, "stream").Bool())
		if textCalls == 1 {
			require.Contains(t, string(body), geminiImageFunction)
			require.NotContains(t, string(body), `"type":"image_generation"`)
			c.JSON(200, gin.H{"id": "resp_first", "status": "completed", "output": []any{gin.H{"id": "fc_image", "type": "function_call", "name": geminiImageFunction, "call_id": "call_image", "arguments": `{"prompt":"an orange cat","size":"1024x1024"}`}}})
		} else {
			require.Contains(t, string(body), `"type":"function_call_output"`)
			require.Equal(t, "call_image", gjson.GetBytes(body, "input.2.call_id").String())
			c.JSON(200, gin.H{"id": "resp_final", "object": "response", "status": "completed", "output": []any{gin.H{"id": "msg_final", "type": "message", "role": "assistant", "status": "completed", "content": []any{gin.H{"type": "output_text", "text": "Here is your image.", "annotations": []any{}}}}}})
		}
	}
	c, w := geminiImagesTestContext(key, "/v1/responses", `{"model":"gpt-5.4","input":"draw a cat","stream":true,"tools":[{"type":"image_generation"}]}`)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, "outer-client-request"))
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	h.WrapGeminiImageResponses(text, service.NewCompositeRouteResolver(nil))(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, imageCalls)
	require.Equal(t, 2, textCalls)
	require.Len(t, billingRequestIDs, 3)
	require.Len(t, map[string]bool{billingRequestIDs[0]: true, billingRequestIDs[1]: true, billingRequestIDs[2]: true}, 3)
	for _, id := range billingRequestIDs {
		require.NotEmpty(t, id)
		require.NotEqual(t, "outer-client-request", id)
	}
	require.Equal(t, "outer-client-request", c.Request.Context().Value(ctxkey.ClientRequestID))
	require.Contains(t, w.Body.String(), "response.image_generation_call.completed")
	require.Contains(t, w.Body.String(), `"result":"aW1hZ2U="`)
	require.Contains(t, w.Body.String(), "response.completed")
	require.NotContains(t, w.Body.String(), geminiImageFunction)
	sequence := int64(0)
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		require.Equal(t, sequence, gjson.Get(strings.TrimPrefix(line, "data: "), "sequence_number").Int())
		sequence++
	}
}

func TestGeminiImagesCodexAutoInjectsForExplicitImagePrompt(t *testing.T) {
	imageCalls, textCalls := 0, 0
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		imageCalls++
		return geminiTestImageResponse(), nil
	})
	key.ImageBridgeModel = stringPointer(service.DefaultGeminiImageModel)
	text := func(c *gin.Context) {
		textCalls++
		body, _ := io.ReadAll(c.Request.Body)
		require.False(t, gjson.GetBytes(body, "stream").Bool())
		if textCalls == 1 {
			require.Contains(t, string(body), geminiImageFunction)
			require.NotContains(t, string(body), `"name":"image_gen"`)
			c.JSON(200, gin.H{"id": "resp_first", "status": "completed", "output": []any{gin.H{"id": "fc_image", "type": "function_call", "name": geminiImageFunction, "call_id": "call_image", "arguments": `{"prompt":"a blue robot cat","size":"2K"}`}}})
			return
		}
		require.Contains(t, string(body), `"type":"function_call_output"`)
		c.JSON(200, gin.H{"id": "resp_final", "object": "response", "status": "completed", "output": []any{gin.H{"id": "msg_final", "type": "message", "role": "assistant", "status": "completed", "content": []any{gin.H{"type": "output_text", "text": "图片已生成。", "annotations": []any{}}}}}})
	}

	c, w := geminiImagesTestContext(key, "/v1/responses", `{"model":"gpt-5.4","input":[{"role":"user","content":"帮我生成个蓝色机器猫，2K"},{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}],"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"stream":true}`)
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	c.Request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
	h.WrapGeminiImageResponses(text, service.NewCompositeRouteResolver(nil))(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, imageCalls)
	require.Equal(t, 2, textCalls)
	require.Contains(t, w.Body.String(), "response.image_generation_call.completed")
	require.Contains(t, w.Body.String(), `"result":"aW1hZ2U="`)
}

func TestGeminiImagesCodexAutoInjectionIntentClassification(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prompt string
		want   bool
	}{
		{name: "Chinese generation", prompt: "请生成一张太空城市图片", want: true},
		{name: "Chinese short command", prompt: "生图", want: true},
		{name: "English generation", prompt: "Generate an image of a moon base", want: true},
		{name: "explicit skill command", prompt: "$imagegen: watercolor fox", want: true},
		{name: "ordinary coding", prompt: "修复登录页面的空指针问题", want: false},
		{name: "image troubleshooting", prompt: "为什么当前环境无法生成图片", want: false},
		{name: "bridge question", prompt: "现在的生图桥接模式是如何实现的", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, hasExplicitImageGenerationPrompt(tt.prompt))
		})
	}
}

func TestGeminiImagesCodexAutoInjectionUsesCurrentTurnOnly(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want bool
	}{
		{
			name: "latest ordinary user ignores historical image request",
			body: `{"input":[{"role":"user","content":"帮我生成一张猫图"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},{"role":"user","content":"解释一下这段代码"}]}`,
			want: false,
		},
		{
			name: "tool continuation does not repeat historical image request",
			body: `{"input":[{"role":"user","content":"帮我生成一张猫图"},{"type":"function_call","name":"shell","call_id":"call_1","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`,
			want: false,
		},
		{
			name: "latest explicit image request is detected",
			body: `{"input":[{"role":"user","content":"解释一下这段代码"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},{"role":"user","content":"现在帮我生成一张猫图"},{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}]}`,
			want: true,
		},
		{
			name: "standalone input text is current user input",
			body: `{"previous_response_id":"resp_1","input":[{"type":"input_text","text":"生成一张月球基地图片"}]}`,
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := map[string]any{}
			require.NoError(t, json.Unmarshal([]byte(tt.body), &request))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "codex_cli_rs")
			require.Equal(t, tt.want, shouldAutoInjectGeminiImageTool(c, request))
		})
	}
}

func TestGeminiImagesCodexPassiveHostedToolKeepsOrdinaryTextStreaming(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("ordinary text must not call the image provider")
		return nil, nil
	})
	key.ImageBridgeModel = stringPointer(service.DefaultGeminiImageModel)

	body := `{"model":"gpt-5.4","input":[{"role":"user","content":"解释一下这段代码"},{"type":"additional_tools","tools":[{"type":"image_generation"},{"type":"namespace","name":"image_gen"}]}],"tools":[{"type":"image_generation"},{"type":"function","name":"shell","parameters":{"type":"object"}},{"type":"namespace","name":"image_gen"}],"stream":true}`
	c, w := geminiImagesTestContext(key, "/v1/responses", body)
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	nextCalled := false
	h.WrapGeminiImageResponses(func(child *gin.Context) {
		nextCalled = true
		forwarded, err := io.ReadAll(child.Request.Body)
		require.NoError(t, err)
		require.True(t, gjson.GetBytes(forwarded, "stream").Bool())
		require.NotContains(t, string(forwarded), `"type":"image_generation"`)
		require.Contains(t, string(forwarded), `"name":"shell"`)
		require.Contains(t, string(forwarded), `"name":"image_gen"`)
		child.Header("Content-Type", "text/event-stream")
		child.Status(http.StatusOK)
		_, _ = child.Writer.WriteString("event: response.output_text.delta\ndata: {\"delta\":\"native\"}\n\n")
	}, service.NewCompositeRouteResolver(nil))(c)

	require.True(t, nextCalled)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"delta":"native"`)
}

func TestGeminiImagesCodexNativeImageToolPassesThroughForLocalFileDelivery(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("the Responses bridge must not execute Codex's local image tool")
		return nil, nil
	})
	key.ImageBridgeModel = stringPointer(service.DefaultGeminiImageModel)

	for _, body := range []string{
		`{"model":"gpt-5.4","input":"draw a cat","stream":true,"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"tool_choice":{"type":"namespace","name":"image_gen"}}`,
		`{"model":"gpt-5.4","input":"draw a cat","stream":true,"tools":[{"type":"function","name":"image_gen.imagegen","parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"image_gen.imagegen"}}`,
		`{"model":"gpt-5.4","input":"draw a cat","stream":true,"additional_tools":[{"type":"function","name":"image_gen__imagegen","parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"image_gen__imagegen"}}`,
	} {
		c, w := geminiImagesTestContext(key, "/v1/responses", body)
		c.Request.Header.Set("User-Agent", "codex_cli_rs")
		nextCalled := false
		h.WrapGeminiImageResponses(func(child *gin.Context) {
			nextCalled = true
			forwarded, _ := io.ReadAll(child.Request.Body)
			require.JSONEq(t, body, string(forwarded))
			child.Header("Content-Type", "text/event-stream")
			child.Status(http.StatusOK)
			_, _ = child.Writer.WriteString("event: response.output_text.delta\ndata: {\"delta\":\"native\"}\n\n")
		}, nil)(c)
		require.True(t, nextCalled)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"delta":"native"`)
		require.NotContains(t, w.Body.String(), "image bridge working")
	}
}

func TestCodexNativeImageToolImagesRequestUsesSelectedBridgeModel(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(req *http.Request) (*http.Response, error) {
		require.Contains(t, req.URL.Path, service.DefaultGeminiImageModel+":generateContent")
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "draw a cat", gjson.GetBytes(body, "contents.0.parts.0.text").String())
		return geminiTestImageResponse(), nil
	})
	// Codex always requests its canonical image model. The key selects that
	// public alias, while the provider's existing model_mapping resolves the
	// actual upstream model.
	key.ImageBridgeModel = stringPointer("gpt-image-2")

	c, w := geminiImagesTestContext(key, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw a cat","response_format":"b64_json"}`)
	h.WrapKeyImageBridge(func(*gin.Context) {
		t.Fatal("a selected bridge model must own the Images request")
	}, nil)(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(w.Body.String(), "data.0.b64_json").String())
	require.Equal(t, "gpt-image-2", gjson.Get(w.Body.String(), "model").String())
}

func TestGeminiImagesCodexForcedToolAndReferences(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		require.Equal(t, "cmVm", gjson.GetBytes(body, "contents.0.parts.1.inlineData.data").String())
		return geminiTestImageResponse(), nil
	})
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	c, w := geminiImagesTestContext(key, "/responses", `{"model":"gpt-5.4","input":[{"role":"user","content":[{"type":"input_text","text":"edit this cat"},{"type":"input_image","image_url":"data:image/png;base64,cmVm"}]}],"tools":[{"type":"image_generation","model":"gemini-3.1-flash-image","action":"edit"}],"tool_choice":{"type":"image_generation"}}`)
	h.WrapGeminiImageResponses(func(*gin.Context) { t.Fatal("forced tool must skip the text model") }, service.NewCompositeRouteResolver(nil))(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "image_generation_call", gjson.Get(w.Body.String(), "output.0.type").String())
}

func TestGeminiImagesCodexOrdinaryRequestsRemainUnchanged(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("ordinary Codex text must not contact the image upstream")
		return nil, nil
	})
	key.ImageBridgeModel = stringPointer(service.DefaultGeminiImageModel)
	body := `{"model":"gpt-5.4","input":"fix this bug","stream":true}`
	c, w := geminiImagesTestContext(key, "/responses", body)
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	nextCalled := false
	h.WrapGeminiImageResponses(func(c *gin.Context) {
		nextCalled = true
		b, _ := io.ReadAll(c.Request.Body)
		require.Equal(t, body, string(b))
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString("event: response.output_text.delta\ndata: {\"delta\":\"live\"}\n\n")
	}, nil)(c)
	require.True(t, nextCalled)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), `"delta":"live"`)
	require.NotContains(t, w.Body.String(), "image bridge working")
}

func TestGeminiImagesCaptureLimitAndStreamingErrors(t *testing.T) {
	c, w := geminiImagesTestContext(&service.APIKey{}, "/responses", `{}`)
	_, capture := imageBridgeChildContext(c, nil)
	_, err := capture.Write(bytes.Repeat([]byte("x"), maxImageBridgeResponseBytes+1))
	require.Error(t, err)
	require.Zero(t, capture.body.Len())
	c.Header("Content-Type", "text/event-stream")
	c.Writer.Flush()
	writeImageBridgeError(c, nil, err)
	require.Contains(t, w.Body.String(), "event: error")
	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.Split(strings.Split(w.Body.String(), "data: ")[1], "\n")[0]), &obj))
}

func TestGeminiImagesCodexHistoryAndTextToolEvents(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("ordinary client tool call must not generate images")
		return nil, nil
	})
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	c, w := geminiImagesTestContext(key, "/v1/responses", `{"model":"gpt-5.4","input":[{"id":"ig_sub2api_previous","type":"image_generation_call","result":"aW1hZ2U="},{"role":"user","content":"save this file"}],"stream":true}`)
	c.Request.Header.Set("User-Agent", "codex_cli_rs")
	upstreamSSE := "event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{\\\"path\\\":\\\"image.png\\\"}\"}\n\n" +
		"event: response.custom_tool_call_input.delta\ndata: {\"type\":\"response.custom_tool_call_input.delta\",\"input\":\"*** Begin Patch\\n*** End Patch\"}\n\n"
	h.WrapGeminiImageResponses(func(child *gin.Context) {
		body, _ := io.ReadAll(child.Request.Body)
		require.Equal(t, "data:image/png;base64,aW1hZ2U=", gjson.GetBytes(body, "input.0.content.1.image_url").String())
		require.True(t, gjson.GetBytes(body, "stream").Bool())
		require.False(t, gjson.GetBytes(body, "parallel_tool_calls").Exists())
		require.NotContains(t, string(body), "ig_sub2api_previous")
		child.Header("Content-Type", "text/event-stream")
		child.Status(http.StatusOK)
		_, _ = child.Writer.WriteString(upstreamSSE)
	}, service.NewCompositeRouteResolver(nil))(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), "response.function_call_arguments.delta")
	require.Contains(t, w.Body.String(), "response.custom_tool_call_input.delta")
	require.Equal(t, upstreamSSE, w.Body.String())
}

func TestGeminiImagesCodexFollowupFailureKeepsImage(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) { return geminiTestImageResponse(), nil })
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	c, w := geminiImagesTestContext(key, "/responses", `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}]}`)
	calls := 0
	h.WrapGeminiImageResponses(func(child *gin.Context) {
		calls++
		if calls == 1 {
			child.JSON(200, gin.H{"output": []any{gin.H{"type": "function_call", "name": geminiImageFunction, "call_id": "call_image", "arguments": `{"prompt":"cat"}`}}, "usage": gin.H{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}})
		} else {
			child.JSON(502, gin.H{"error": gin.H{"message": "text upstream failed"}})
		}
	}, service.NewCompositeRouteResolver(nil))(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(w.Body.String(), "output.0.result").String())
	require.Equal(t, int64(15), gjson.Get(w.Body.String(), "usage.total_tokens").Int())
	require.False(t, gjson.Get(w.Body.String(), "store").Bool())
}

func TestGeminiImagesCodexRejectsUnsupportedState(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformOpenAI, func(*http.Request) (*http.Response, error) {
		t.Fatal("must not contact image upstream")
		return nil, nil
	})
	h.cfg.Gateway.CodexGeminiImageModel = service.DefaultGeminiImageModel
	for _, field := range []string{`"previous_response_id":"resp_previous"`, `"background":true`} {
		c, w := geminiImagesTestContext(key, "/responses", `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}],`+field+`}`)
		h.WrapGeminiImageResponses(func(*gin.Context) { t.Fatal("must not contact text upstream") }, service.NewCompositeRouteResolver(nil))(c)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
}

func TestGeminiImagesTextUsageAndIncompleteStatus(t *testing.T) {
	var first, final map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":2}}}`), &first))
	require.NoError(t, json.Unmarshal([]byte(`{"status":"incomplete","output":[],"usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":8}}}`), &final))
	mergeImageBridgeUsage(final, first)
	c, w := geminiImagesTestContext(&service.APIKey{}, "/responses", `{}`)
	writeImageBridgeResponses(c, final)
	require.Contains(t, w.Body.String(), `"total_tokens":38`)
	require.Contains(t, w.Body.String(), `"cached_tokens":10`)
	require.Contains(t, w.Body.String(), "event: response.incomplete")
	require.NotContains(t, w.Body.String(), "event: response.completed")
}

func TestGeminiImagesKeepaliveAndChildCopies(t *testing.T) {
	c, w := geminiImagesTestContext(&service.APIKey{}, "/responses", `{}`)
	stop := startImageBridgeKeepaliveInterval(c, true, time.Millisecond)
	for i := 0; i < 20; i++ {
		child, capture := imageBridgeChildContext(c, nil)
		child.JSON(200, gin.H{"internal": true})
		require.Contains(t, capture.body.String(), "internal")
		time.Sleep(time.Millisecond)
	}
	stop()
	stop()
	require.Contains(t, w.Body.String(), ": image bridge working")
	require.NotContains(t, w.Body.String(), "internal")
	writeImageBridgeError(c, nil, io.ErrUnexpectedEOF)
	require.Contains(t, w.Body.String(), "event: error")
}

func TestGeminiImagesSharesOpenAIConcurrencyLimit(t *testing.T) {
	h, key := newGeminiImagesTestHandler(t, service.PlatformGemini, func(*http.Request) (*http.Response, error) {
		t.Fatal("blocked image request must not reach upstream")
		return nil, nil
	})
	h.SetImageGateway(&OpenAIGatewayHandler{
		cfg: &config.Config{Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{
			Enabled: true, MaxConcurrentRequests: 1, OverflowMode: config.ImageConcurrencyOverflowModeReject,
		}}},
		imageLimiter: &imageConcurrencyLimiter{},
	})
	c, w := geminiImagesTestContext(key, "/v1/images/generations", `{"prompt":"draw a cat"}`)
	release, acquired := h.imageGateway.acquireImageGenerationSlot(c, false)
	require.True(t, acquired)
	defer release()
	h.GeminiImages(c)
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Image generation concurrency limit exceeded")
}
