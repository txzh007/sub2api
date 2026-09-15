package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const geminiImageFunction = "sub2api_generate_image"

// WrapGeminiImageResponses hosts an image tool inside Responses. Explicit
// hosted-tool requests let the text model decide whether to call it, while a
// prompt-classified Codex fallback forces the internal function. All internal
// calls go through the ordinary gateway handlers.
func (h *GatewayHandler) WrapGeminiImageResponses(next gin.HandlerFunc, resolver *service.CompositeRouteResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/responses") {
			next(c)
			return
		}
		key, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || !service.KeySupportsImageBridge(key) {
			next(c)
			return
		}
		model, bridgeEnabled := h.apiKeyService.EffectiveImageBridgeModelForRequest(c.Request.Context(), key)
		if !bridgeEnabled {
			next(c)
			return
		}
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			if _, ok := extractMaxBytesError(err); ok {
				status = http.StatusRequestEntityTooLarge
			}
			h.responsesErrorResponse(c, status, "invalid_request_error", "Failed to read request body")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var request map[string]any
		if json.Unmarshal(body, &request) != nil {
			next(c)
			return
		}
		tool, hasTool := findGeminiBridgeImageTool(request)
		requestModel, _ := request["model"].(string)
		directImage := service.IsImageProviderModel(requestModel)
		codexRequest := isCodexImageBridgeRequest(c)
		autoInjected := !directImage && shouldAutoInjectGeminiImageTool(c, request)
		passiveHostedTool := hasTool && codexRequest && !imageToolChoiceForced(request)
		// A configured bridge must not turn every Codex Responses request into a
		// buffered hosted-tool workflow. In particular, image_gen is a local Codex
		// extension: Codex calls the active provider's Images API and then persists
		// the returned bytes in its own workspace. Intercepting that tool here turns
		// the result into an inline image_generation_call and loses the local file.
		// Only the Responses-hosted image_generation tool (or a request directly
		// targeting an image model) belongs to this server-side bridge.
		if model == "" {
			next(c)
			return
		}
		if !directImage && !autoInjected && (!hasTool || passiveHostedTool) {
			changed := false
			if passiveHostedTool {
				changed = stripPassiveCodexHostedImageTools(request)
			}
			if normalizeGeminiImageHistory(request) {
				changed = true
			}
			if changed {
				normalizedBody, marshalErr := json.Marshal(request)
				if marshalErr != nil {
					h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to normalize image bridge history")
					return
				}
				c.Request.Body = io.NopCloser(bytes.NewReader(normalizedBody))
				c.Request.ContentLength = int64(len(normalizedBody))
			}
			next(c)
			return
		}
		if autoInjected {
			slog.Info("codex_image_bridge.auto_injected",
				"api_key_id", key.ID,
				"text_model", requestModel,
				"image_model", model,
				"passive_image_tool_declared", hasCodexLocalImageTool(request),
				"passive_hosted_tool_declared", passiveHostedTool,
			)
			tool = map[string]any{"type": "image_generation"}
			forceGeminiImageFunctionChoice(request)
		}
		// Resolve the independent image upstream only when the text model calls
		// the tool. An unavailable image provider must not block ordinary text.
		c.Set(imageBridgeResolverKey, resolver)
		decision := service.CompositeRouteDecision{PublicModel: model, UpstreamModel: model}
		stream, validStream := parseOpenAICompatibleStream(body)
		if !validStream {
			h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", invalidStreamFieldTypeMessage)
			return
		}
		if request["background"] == true {
			h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Gemini image bridge does not support background Responses")
			return
		}
		if previous, _ := request["previous_response_id"].(string); previous != "" {
			h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Gemini image bridge requires full conversation input; previous_response_id is not supported")
			return
		}
		stop := startImageBridgeKeepalive(c, stream)
		defer stop()
		response, capture, err := h.runGeminiImageResponses(c, request, tool, decision, next)
		stop()
		if err != nil {
			writeImageBridgeError(c, capture, err)
			return
		}
		if !stream {
			c.JSON(http.StatusOK, response)
			return
		}
		writeImageBridgeResponses(c, response)
	}
}

// tryGeminiImageResponsesWebSocket applies the HTTP Responses image bridge to
// a response.create WebSocket frame. It only handles explicit image turns;
// ordinary turns are returned (optionally normalized) to the native WS path.
func (h *GatewayHandler) tryGeminiImageResponsesWebSocket(
	c *gin.Context,
	payload []byte,
	resolver *service.CompositeRouteResolver,
	next gin.HandlerFunc,
) ([]byte, map[string]any, *imageBridgeCapture, bool, error) {
	normalized := append([]byte(nil), payload...)
	if h == nil || h.apiKeyService == nil || c == nil || next == nil {
		return normalized, nil, nil, false, nil
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || !service.KeySupportsImageBridge(key) {
		return normalized, nil, nil, false, nil
	}
	model, bridgeEnabled := h.apiKeyService.EffectiveImageBridgeModelForRequest(c.Request.Context(), key)
	if !bridgeEnabled || strings.TrimSpace(model) == "" {
		return normalized, nil, nil, false, nil
	}

	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope == nil {
		return normalized, nil, nil, false, nil
	}
	eventType := strings.TrimSpace(fmt.Sprint(envelope["type"]))
	if eventType != "" && eventType != "response.create" {
		return normalized, nil, nil, false, nil
	}
	tool, hasTool := findGeminiBridgeImageTool(envelope)
	requestModel, _ := envelope["model"].(string)
	directImage := service.IsImageProviderModel(requestModel)
	codexRequest := isCodexImageBridgeRequest(c)
	autoInjected := !directImage && shouldAutoInjectGeminiImageTool(c, envelope)
	passiveHostedTool := hasTool && codexRequest && !imageToolChoiceForced(envelope)

	if !directImage && !autoInjected && (!hasTool || passiveHostedTool) {
		changed := false
		if passiveHostedTool {
			changed = stripPassiveCodexHostedImageTools(envelope)
		}
		if normalizeGeminiImageHistory(envelope) {
			changed = true
		}
		if !changed {
			return normalized, nil, nil, false, nil
		}
		body, err := json.Marshal(envelope)
		if err != nil {
			return normalized, nil, nil, false, err
		}
		return body, nil, nil, false, nil
	}

	request := make(map[string]any, len(envelope))
	for key, value := range envelope {
		request[key] = value
	}
	delete(request, "type")
	delete(request, "generate")
	request["stream"] = false
	if request["background"] == true {
		return normalized, nil, nil, true, fmt.Errorf("Gemini image bridge does not support background Responses")
	}
	if previous, _ := request["previous_response_id"].(string); strings.TrimSpace(previous) != "" {
		return normalized, nil, nil, true, fmt.Errorf("Gemini image bridge requires full conversation input; previous_response_id is not supported")
	}
	if autoInjected {
		slog.Info("codex_image_bridge.websocket_auto_injected",
			"api_key_id", key.ID,
			"text_model", requestModel,
			"image_model", model,
			"passive_image_tool_declared", hasCodexLocalImageTool(envelope),
			"passive_hosted_tool_declared", passiveHostedTool,
		)
		tool = map[string]any{"type": "image_generation"}
		forceGeminiImageFunctionChoice(request)
	}

	bridgeContext := c.Copy()
	bridgeContext.Request = c.Request.Clone(c.Request.Context())
	bridgeContext.Request.Method = http.MethodPost
	bridgeContext.Request.URL.Path = "/v1/responses"
	bridgeContext.Request.URL.RawPath = ""
	bridgeContext.Set(imageBridgeResolverKey, resolver)
	decision := service.CompositeRouteDecision{PublicModel: model, UpstreamModel: model}
	response, capture, err := h.runGeminiImageResponses(bridgeContext, request, tool, decision, next)
	for key, value := range bridgeContext.Keys {
		c.Set(key, value)
	}
	return normalized, response, capture, true, err
}

// shouldAutoInjectGeminiImageTool restores the server-hosted image bridge for
// Codex installations that do not have the local image_gen extension. It is
// deliberately limited to explicit image creation/editing prompts so ordinary
// Codex traffic keeps its native streaming path.
func shouldAutoInjectGeminiImageTool(c *gin.Context, request map[string]any) bool {
	if c == nil || request == nil || codexLocalImageToolChoiceForced(request) {
		return false
	}
	if !isCodexImageBridgeRequest(c) {
		return false
	}
	prompt, currentUserInput := currentImageBridgeUserPrompt(request)
	if !currentUserInput {
		return false
	}
	return hasExplicitImageGenerationPrompt(prompt)
}

func isCodexImageBridgeRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	userAgent := strings.ToLower(c.GetHeader("User-Agent"))
	originator := strings.ToLower(c.GetHeader("originator"))
	return strings.Contains(userAgent, "codex") || strings.Contains(originator, "codex")
}

// currentImageBridgeUserPrompt intentionally looks only at the current input
// boundary. A previous image request in full conversation history must not
// re-enable the buffered bridge for later tool outputs or ordinary text turns.
func currentImageBridgeUserPrompt(request map[string]any) (string, bool) {
	if text, ok := request["input"].(string); ok {
		return text, true
	}
	items, _ := request["input"].([]any)
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(fmt.Sprint(item["type"]))
		role := strings.TrimSpace(fmt.Sprint(item["role"]))
		if itemType == "additional_tools" || itemType == "compaction_trigger" || role == "system" || role == "developer" {
			continue
		}
		if role == "user" {
			return imageBridgeMessageText(item), true
		}
		if itemType == "input_text" {
			text, _ := item["text"].(string)
			return text, true
		}
		// A tool output or assistant item after the most recent user message is
		// a continuation boundary, not a new image request.
		return "", false
	}
	return "", false
}

func imageBridgeMessageText(item map[string]any) string {
	if text, ok := item["content"].(string); ok {
		return text
	}
	content, _ := item["content"].([]any)
	texts := make([]string, 0, len(content))
	for _, rawPart := range content {
		part, _ := rawPart.(map[string]any)
		if text, ok := part["text"].(string); ok && part["type"] == "input_text" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

// codexLocalImageToolChoiceForced distinguishes an explicit client decision to
// execute the local image tool from Codex's passive image_gen advertisement.
// New Codex builds advertise the namespace on ordinary requests even when the
// current host cannot execute it, so declarations alone cannot suppress the
// server-hosted fallback.
func codexLocalImageToolChoiceForced(request map[string]any) bool {
	var selected func(any) bool
	selected = func(raw any) bool {
		switch value := raw.(type) {
		case string:
			return isCodexLocalImageToolName(value)
		case map[string]any:
			toolType := strings.TrimSpace(fmt.Sprint(value["type"]))
			name := strings.TrimSpace(fmt.Sprint(value["name"]))
			namespace := strings.TrimSpace(fmt.Sprint(value["namespace"]))
			if toolType == "namespace" && name == "image_gen" {
				return true
			}
			if namespace == "image_gen" && (name == "" || name == "imagegen") {
				return true
			}
			if isCodexLocalImageToolName(name) {
				return true
			}
			return selected(value["tool"]) || selected(value["function"])
		default:
			return false
		}
	}
	return selected(request["tool_choice"])
}

func isCodexLocalImageToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case "image_gen", "image_gen.imagegen", "image_gen__imagegen":
		return true
	default:
		return false
	}
}

func isCodexLocalImageTool(raw any) bool {
	tool, _ := raw.(map[string]any)
	toolType := strings.TrimSpace(fmt.Sprint(tool["type"]))
	name := strings.TrimSpace(fmt.Sprint(tool["name"]))
	if toolType == "namespace" && name == "image_gen" {
		return true
	}
	return isCodexLocalImageToolName(name)
}

func hasCodexLocalImageTool(request map[string]any) bool {
	containsLocalTool := func(rawTools any) bool {
		tools, _ := rawTools.([]any)
		for _, raw := range tools {
			if isCodexLocalImageTool(raw) {
				return true
			}
		}
		return false
	}
	if containsLocalTool(request["tools"]) || containsLocalTool(request["additional_tools"]) {
		return true
	}
	input, _ := request["input"].([]any)
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(item["type"])) == "additional_tools" && containsLocalTool(item["tools"]) {
			return true
		}
	}
	return false
}

func hasExplicitImageGenerationPrompt(prompt string) bool {
	prompt = strings.ToLower(strings.TrimSpace(prompt))
	if prompt == "" {
		return false
	}

	for _, prefix := range []string{"$imagegen", "/imagegen"} {
		if prompt == prefix || strings.HasPrefix(prompt, prefix+" ") || strings.HasPrefix(prompt, prefix+"：") || strings.HasPrefix(prompt, prefix+":") {
			return true
		}
	}
	for _, prefix := range []string{
		"为什么", "为何", "怎么", "如何", "是否", "是不是", "什么", "哪个", "哪种", "有没有",
		"why ", "how ", "what ", "which ", "does ", "do ", "is ", "are ",
	} {
		if strings.HasPrefix(prompt, prefix) {
			return false
		}
	}
	for _, failure := range []string{"无法生成图片", "不能生成图片", "生图失败", "生成图片失败", "image generation failed", "cannot generate images", "can't generate images"} {
		if strings.Contains(prompt, failure) {
			return false
		}
	}

	for _, phrase := range []string{
		"帮我生成", "请生成", "给我生成", "为我生成", "替我生成",
		"生成一张", "生成一幅", "生成一个", "生成个", "生成图片", "生成图像", "生成照片", "生成海报", "生成头像", "生成壁纸",
		"帮我画", "请画", "给我画", "为我画", "画一张", "画一幅", "画一个", "画个",
		"绘制一张", "绘制一幅", "绘制一个", "绘制图片", "制作一张", "制作一个", "制作图片",
		"做一张图", "做张图", "出一张图", "出张图", "帮我生图", "请生图", "直接生图", "开始生图",
		"编辑这张图", "编辑图片", "修改这张图", "修改图片", "改这张图", "重绘这张图", "扩展这张图", "扩图", "抠图", "去掉背景", "移除背景", "替换背景", "换个背景",
		"generate an image", "generate a picture", "generate image", "generate picture", "create an image", "create a picture", "create image", "create picture",
		"draw me", "draw a", "draw an", "make me an image", "make an image", "make a picture", "render an image", "render a picture",
		"edit this image", "edit the image", "modify this image", "remove the background", "replace the background", "can you generate", "could you generate",
	} {
		if strings.Contains(prompt, phrase) {
			return true
		}
	}
	return prompt == "生图" || prompt == "画图" || prompt == "生成图"
}

func findGeminiBridgeImageTool(request map[string]any) (map[string]any, bool) {
	for _, field := range []string{"tools", "additional_tools"} {
		tools, _ := request[field].([]any)
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			typ, _ := tool["type"].(string)
			if typ == "image_generation" {
				return tool, true
			}
		}
	}
	return map[string]any{}, false
}

// Codex can advertise the hosted image_generation capability on every turn.
// Leaving that passive declaration on an ordinary text request would make the
// bridge buffer the whole response while the text model decides whether to use
// it. Remove only that hosted capability; local Codex tools and all unrelated
// tools continue down the native streaming path unchanged.
func stripPassiveCodexHostedImageTools(request map[string]any) bool {
	changed := false
	for _, field := range []string{"tools", "additional_tools"} {
		items, ok := request[field].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			if isBridgeImageTool(item) {
				changed = true
				continue
			}
			filtered = append(filtered, item)
		}
		if len(filtered) == 0 {
			delete(request, field)
		} else {
			request[field] = filtered
		}
	}

	input, ok := request["input"].([]any)
	if !ok {
		return changed
	}
	filteredInput := make([]any, 0, len(input))
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(item["type"])) != "additional_tools" {
			filteredInput = append(filteredInput, raw)
			continue
		}
		rawTools, _ := item["tools"].([]any)
		filteredTools := make([]any, 0, len(rawTools))
		for _, tool := range rawTools {
			if isBridgeImageTool(tool) {
				changed = true
				continue
			}
			filteredTools = append(filteredTools, tool)
		}
		if len(filteredTools) == 0 {
			changed = true
			continue
		}
		if len(filteredTools) != len(rawTools) {
			copyItem := make(map[string]any, len(item))
			for key, value := range item {
				copyItem[key] = value
			}
			copyItem["tools"] = filteredTools
			filteredInput = append(filteredInput, copyItem)
			continue
		}
		filteredInput = append(filteredInput, raw)
	}
	if changed {
		request["input"] = filteredInput
	}
	return changed
}

func isBridgeImageTool(raw any) bool {
	tool, _ := raw.(map[string]any)
	typ, _ := tool["type"].(string)
	name, _ := tool["name"].(string)
	return typ == "image_generation" || name == geminiImageFunction
}

func imageBridgeInput(request map[string]any) (string, []string) {
	if text, ok := request["input"].(string); ok {
		return text, nil
	}
	items, _ := request["input"].([]any)
	var prompt string
	var images []string
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["type"] == "image_generation_call" {
			if b64, _ := item["result"].(string); b64 != "" {
				format, _ := item["output_format"].(string)
				if format == "" {
					format = "png"
				}
				images = append(images, "data:image/"+format+";base64,"+b64)
			}
		}
		if item["role"] != "user" {
			continue
		}
		if text, ok := item["content"].(string); ok {
			prompt = text
			continue
		}
		content, _ := item["content"].([]any)
		var texts []string
		for _, rawPart := range content {
			part, _ := rawPart.(map[string]any)
			if text, ok := part["text"].(string); ok && part["type"] == "input_text" {
				texts = append(texts, text)
			}
			if image, ok := part["image_url"].(string); ok && part["type"] == "input_image" {
				images = append(images, image)
			}
		}
		if len(texts) > 0 {
			prompt = strings.Join(texts, "\n")
		}
	}
	return prompt, images
}

func prepareGeminiImageFunctionRequest(request map[string]any, referenceCount int) {
	var tools []any
	for _, field := range []string{"tools", "additional_tools"} {
		items, _ := request[field].([]any)
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			if !isBridgeImageTool(item) && !isCodexLocalImageTool(item) {
				filtered = append(filtered, item)
			}
		}
		if field == "tools" {
			tools = filtered
		} else if len(filtered) > 0 {
			request[field] = filtered
		} else {
			delete(request, field)
		}
	}
	stripCodexLocalImageToolInputDeclarations(request)
	tools = append(tools, map[string]any{
		"type": "function", "name": geminiImageFunction, "strict": false,
		"description": fmt.Sprintf("Generate or edit a raster image using the configured image model. This replaces image_gen.imagegen and image_generation for this request. Call only when the user requests image generation or editing. Supply a complete image prompt. There are %d reference images in conversation order; select their zero-based indices for edits. The gateway executes this tool and returns the actual image to the client.", referenceCount),
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"prompt": map[string]any{"type": "string"}, "size": map[string]any{"type": "string", "description": "auto, 1K, 2K, 4K or WIDTHxHEIGHT; default auto"},
			"reference_image_indices": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 0}},
		}, "required": []string{"prompt"}, "additionalProperties": false},
	})
	request["tools"] = tools
	request["stream"] = false
	request["parallel_tool_calls"] = false
}

// A prompt-classified fallback is already an explicit request to generate or
// edit an image. Force the internal bridge function so the text model cannot
// answer that the client-local image_gen tool is unavailable. The function
// call is still used to expand the prompt and extract size/reference options.
func forceGeminiImageFunctionChoice(request map[string]any) {
	request["tool_choice"] = map[string]any{
		"type": "function",
		"name": geminiImageFunction,
	}
}

func stripCodexLocalImageToolInputDeclarations(request map[string]any) {
	input, ok := request["input"].([]any)
	if !ok {
		return
	}
	filteredInput := make([]any, 0, len(input))
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(fmt.Sprint(item["type"])) != "additional_tools" {
			filteredInput = append(filteredInput, raw)
			continue
		}
		rawTools, _ := item["tools"].([]any)
		filteredTools := make([]any, 0, len(rawTools))
		for _, tool := range rawTools {
			if !isCodexLocalImageTool(tool) {
				filteredTools = append(filteredTools, tool)
			}
		}
		if len(filteredTools) == 0 {
			continue
		}
		copyItem := make(map[string]any, len(item))
		for key, value := range item {
			copyItem[key] = value
		}
		copyItem["tools"] = filteredTools
		filteredInput = append(filteredInput, copyItem)
	}
	request["input"] = filteredInput
}

// Generated images belong to this gateway, so an OpenAI account cannot look
// up their IDs. Full-history clients send the image bytes back as vision input.
func normalizeGeminiImageHistory(request map[string]any) bool {
	items, ok := request["input"].([]any)
	if !ok {
		return false
	}
	changed := false
	for i, raw := range items {
		item, _ := raw.(map[string]any)
		id, _ := item["id"].(string)
		if item["type"] != "image_generation_call" || !strings.HasPrefix(id, "ig_sub2api_") {
			continue
		}
		format, _ := item["output_format"].(string)
		if format == "" {
			format = "png"
		}
		data, _ := item["result"].(string)
		if data != "" {
			items[i] = map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "Image generated earlier in this conversation:"},
				map[string]any{"type": "input_image", "image_url": "data:image/" + format + ";base64," + data},
			}}
			changed = true
		}
	}
	return changed
}

func imageToolChoiceForced(request map[string]any) bool {
	choice, _ := request["tool_choice"].(map[string]any)
	return choice["type"] == "image_generation"
}

func (h *GatewayHandler) runGeminiImageResponses(c *gin.Context, request, tool map[string]any, route service.CompositeRouteDecision, next gin.HandlerFunc) (map[string]any, *imageBridgeCapture, error) {
	prompt, references := imageBridgeInput(request)
	requestModel, _ := request["model"].(string)
	if imageToolChoiceForced(request) || (strings.HasPrefix(requestModel, "gemini-") && strings.Contains(requestModel, "-image")) {
		images, capture, err := h.callGeminiImageTool(c, route, tool, prompt, references)
		if err != nil {
			return nil, capture, err
		}
		return newImageBridgeResponse(request, images), capture, nil
	}
	prepareGeminiImageFunctionRequest(request, len(references))
	normalizeGeminiImageHistory(request)
	response, capture, err := callImageBridgeText(c, request, next)
	if err != nil {
		return nil, capture, err
	}
	outputs, _ := response["output"].([]any)
	var imageItems, toolOutputs []any
	var visibleOutputs []any
	internalCalls, otherCalls := 0, 0
	for _, raw := range outputs {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call" && item["name"] == geminiImageFunction {
			internalCalls++
		} else {
			visibleOutputs = append(visibleOutputs, raw)
			if item["type"] == "function_call" || item["type"] == "custom_tool_call" || item["type"] == "local_shell_call" {
				otherCalls++
			}
		}
	}
	if internalCalls > 4 || (internalCalls > 0 && otherCalls > 0) {
		return nil, nil, fmt.Errorf("text model returned parallel image/client tool calls despite parallel_tool_calls=false")
	}
	imageCalls := 0
	for _, raw := range outputs {
		item, _ := raw.(map[string]any)
		if item["type"] != "function_call" || item["name"] != geminiImageFunction {
			continue
		}
		imageCalls++
		if imageCalls > 4 {
			return nil, nil, fmt.Errorf("at most four image tool calls are allowed per response")
		}
		arguments, _ := item["arguments"].(string)
		var args struct {
			Prompt           string `json:"prompt"`
			Size             string `json:"size"`
			ReferenceIndices []int  `json:"reference_image_indices"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil || strings.TrimSpace(args.Prompt) == "" {
			return nil, nil, fmt.Errorf("image tool returned invalid arguments")
		}
		var selected []string
		for _, index := range args.ReferenceIndices {
			if index < 0 || index >= len(references) {
				return nil, nil, fmt.Errorf("image tool selected an invalid reference image")
			}
			selected = append(selected, references[index])
		}
		options := map[string]any{}
		for key, value := range tool {
			options[key] = value
		}
		if args.Size != "" {
			options["size"] = args.Size
		}
		images, imageCapture, err := h.callGeminiImageTool(c, route, options, args.Prompt, selected)
		if err != nil {
			return nil, imageCapture, err
		}
		imageItems = append(imageItems, images...)
		toolOutputs = append(toolOutputs, map[string]any{"type": "function_call_output", "call_id": item["call_id"], "output": "Image generation completed. The generated image is attached to this response and visible to the user. Do not generate it again or invent a local file path."})
	}
	if len(imageItems) == 0 {
		return response, capture, nil
	}
	// Close the upstream function calls before returning a native image item.
	// Each text call is billed normally. Full-history clients can continue using
	// the returned items; generated image IDs are normalized on their next turn.
	var input []any
	switch original := request["input"].(type) {
	case string:
		input = []any{map[string]any{"role": "user", "content": original}}
	case []any:
		input = append(input, original...)
	}
	input = append(input, outputs...)
	input = append(input, toolOutputs...)
	request["input"] = input
	requestTools, ok := request["tools"].([]any)
	if !ok {
		return nil, capture, fmt.Errorf("image bridge tools must be an array")
	}
	var remaining []any
	for _, raw := range requestTools {
		if !isBridgeImageTool(raw) {
			remaining = append(remaining, raw)
		}
	}
	request["tools"] = remaining
	delete(request, "tool_choice")
	final, finalCapture, err := callImageBridgeText(c, request, next)
	if err != nil {
		// The images were successfully generated and charged. Deliver them even
		// when the optional textual follow-up fails instead of losing the result.
		fallback := newImageBridgeResponse(request, append(visibleOutputs, imageItems...))
		fallback["usage"] = response["usage"]
		return fallback, capture, nil
	}
	finalOutputs, _ := final["output"].([]any)
	final["output"] = append(append(visibleOutputs, imageItems...), finalOutputs...)
	mergeImageBridgeUsage(final, response)
	return final, finalCapture, nil
}

func mergeImageBridgeUsage(final, first map[string]any) {
	previous, _ := first["usage"].(map[string]any)
	if previous == nil {
		return
	}
	current, _ := final["usage"].(map[string]any)
	if current == nil {
		final["usage"] = previous
		return
	}
	for key, value := range previous {
		switch v := value.(type) {
		case float64:
			n, _ := current[key].(float64)
			current[key] = n + v
		case map[string]any:
			n, _ := current[key].(map[string]any)
			if n == nil {
				n = map[string]any{}
				current[key] = n
			}
			for field, count := range v {
				a, _ := n[field].(float64)
				b, _ := count.(float64)
				n[field] = a + b
			}
		}
	}
}

func (h *GatewayHandler) callGeminiImageTool(c *gin.Context, route service.CompositeRouteDecision, options map[string]any, prompt string, references []string) ([]any, *imageBridgeCapture, error) {
	if value, ok := c.Get(imageBridgeResolverKey); ok {
		key, _ := middleware.GetAPIKeyFromContext(c)
		resolver, _ := value.(*service.CompositeRouteResolver)
		binding, err := h.resolveKeyImageBridge(c, key, route.PublicModel, resolver)
		if err != nil {
			return nil, nil, err
		}
		c.Set(imageBridgeBindingKey, binding)
		route = binding.route
	}
	payload := map[string]any{"model": route.UpstreamModel, "prompt": prompt, "response_format": "b64_json"}
	for _, field := range []string{"size", "quality", "background", "output_format", "n"} {
		if value, ok := options[field]; ok {
			payload[field] = value
		}
	}
	path := "/v1/images/generations"
	if len(references) > 0 {
		images := make([]any, 0, len(references))
		for _, ref := range references {
			images = append(images, map[string]any{"image_url": ref})
		}
		payload["images"] = images
		path = "/v1/images/edits"
	}
	if options["action"] == "edit" && len(references) == 0 {
		return nil, nil, fmt.Errorf("image editing requires reference image data")
	}
	body, _ := json.Marshal(payload)
	child, capture := imageBridgeBillableChildContext(c, body)
	if value, ok := c.Get(imageBridgeBindingKey); ok {
		if binding, valid := value.(*imageBridgeBinding); valid {
			applyImageBridgeBinding(child, binding)
		}
	}
	child.Request.Header.Set("Content-Type", "application/json")
	child.Request.URL.Path = path
	child.Request.URL.RawPath = ""
	child.Request = child.Request.WithContext(service.WithCompositeRouteDecision(child.Request.Context(), route))
	h.dispatchImageBridge(child, route.TargetPlatform)
	if capture.err != nil {
		return nil, capture, capture.err
	}
	if capture.Status() >= 400 {
		return nil, capture, fmt.Errorf("image generation failed")
	}
	var images service.GeminiCompatibleImagesResponse
	if err := json.Unmarshal(capture.body.Bytes(), &images); err != nil {
		return nil, capture, err
	}
	var output []any
	for _, image := range images.Data {
		if image.MimeType == "" {
			if decoded, err := base64.StdEncoding.DecodeString(image.B64JSON); err == nil {
				image.MimeType = http.DetectContentType(decoded)
			}
		}
		output = append(output, map[string]any{"id": "ig_sub2api_" + uuid.NewString(), "type": "image_generation_call", "status": "completed", "result": image.B64JSON, "output_format": strings.TrimPrefix(image.MimeType, "image/"), "model": route.UpstreamModel, "revised_prompt": prompt})
	}
	if len(output) == 0 {
		return nil, capture, fmt.Errorf("gemini returned no image")
	}
	return output, capture, nil
}

func callImageBridgeText(c *gin.Context, request map[string]any, next gin.HandlerFunc) (map[string]any, *imageBridgeCapture, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	child, capture := imageBridgeBillableChildContext(c, body)
	child.Request = child.Request.WithContext(service.WithExternalImageTool(child.Request.Context()))
	child.Request.Header.Set("Content-Type", "application/json")
	next(child)
	for key, value := range child.Keys {
		c.Set(key, value)
	}
	if capture.err != nil {
		return nil, capture, capture.err
	}
	if capture.Status() >= 400 {
		return nil, capture, fmt.Errorf("text model request failed")
	}
	var response map[string]any
	if err := json.Unmarshal(capture.body.Bytes(), &response); err != nil {
		return nil, capture, fmt.Errorf("invalid Responses result from text model")
	}
	return response, capture, nil
}

func imageBridgeBillableChildContext(c *gin.Context, body []byte) (*gin.Context, *imageBridgeCapture) {
	child, capture := imageBridgeChildContext(c, body)
	// Usage deduplication prefers ClientRequestID over the upstream response ID.
	// Each hosted tool/text call is a separate billable request; sharing the
	// outer ID would reject all but the first charge as a fingerprint conflict.
	// Keep the outer RequestID/logger for tracing, and preserve this new ID
	// through protocol conversion and any retries within the child call.
	ctx := context.WithValue(child.Request.Context(), ctxkey.ClientRequestID, uuid.NewString())
	child.Request = child.Request.WithContext(ctx)
	return child, capture
}

func newImageBridgeResponse(request map[string]any, output []any) map[string]any {
	return map[string]any{"id": "resp_sub2api_" + uuid.NewString(), "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": request["model"], "output": output, "error": nil, "store": false}
}

func writeImageBridgeResponses(c *gin.Context, response map[string]any) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	_ = emitImageBridgeResponseEvents(response, func(event map[string]any) error {
		writeImageBridgeSSE(c, event)
		return nil
	})
}

func emitImageBridgeResponseEvents(response map[string]any, emitEvent func(map[string]any) error) error {
	if emitEvent == nil {
		return errors.New("image bridge event writer is nil")
	}
	sequence := 0
	var firstErr error
	emit := func(event map[string]any) {
		if firstErr != nil {
			return
		}
		event["sequence_number"] = sequence
		sequence++
		firstErr = emitEvent(event)
	}
	created := map[string]any{}
	for key, value := range response {
		created[key] = value
	}
	created["status"] = "in_progress"
	created["output"] = []any{}
	emit(map[string]any{"type": "response.created", "response": created})
	emit(map[string]any{"type": "response.in_progress", "response": created})
	output, _ := response["output"].([]any)
	for index, raw := range output {
		item, _ := raw.(map[string]any)
		added := map[string]any{}
		for key, value := range item {
			added[key] = value
		}
		added["status"] = "in_progress"
		if item["type"] == "image_generation_call" {
			delete(added, "result")
		}
		if item["type"] == "message" {
			added["content"] = []any{}
		}
		if item["type"] == "function_call" {
			added["arguments"] = ""
		}
		if item["type"] == "custom_tool_call" {
			added["input"] = ""
		}
		emit(map[string]any{"type": "response.output_item.added", "output_index": index, "item": added})
		if item["type"] == "function_call" {
			emit(map[string]any{"type": "response.function_call_arguments.delta", "output_index": index, "item_id": item["id"], "delta": item["arguments"]})
			emit(map[string]any{"type": "response.function_call_arguments.done", "output_index": index, "item_id": item["id"], "arguments": item["arguments"]})
		}
		if item["type"] == "custom_tool_call" {
			emit(map[string]any{"type": "response.custom_tool_call_input.delta", "output_index": index, "item_id": item["id"], "delta": item["input"]})
			emit(map[string]any{"type": "response.custom_tool_call_input.done", "output_index": index, "item_id": item["id"], "input": item["input"]})
		}
		if item["type"] == "image_generation_call" {
			for _, state := range []string{"in_progress", "generating", "completed"} {
				emit(map[string]any{"type": "response.image_generation_call." + state, "output_index": index, "item_id": item["id"]})
			}
		}
		if item["type"] == "message" {
			content, _ := item["content"].([]any)
			for ci, rawPart := range content {
				part, _ := rawPart.(map[string]any)
				emptyPart := map[string]any{"type": part["type"], "text": "", "annotations": []any{}}
				if part["type"] == "refusal" {
					emptyPart = map[string]any{"type": "refusal", "refusal": ""}
				}
				emit(map[string]any{"type": "response.content_part.added", "output_index": index, "item_id": item["id"], "content_index": ci, "part": emptyPart})
				if part["type"] == "output_text" {
					emit(map[string]any{"type": "response.output_text.delta", "output_index": index, "item_id": item["id"], "content_index": ci, "delta": part["text"]})
					emit(map[string]any{"type": "response.output_text.done", "output_index": index, "item_id": item["id"], "content_index": ci, "text": part["text"]})
				}
				if part["type"] == "refusal" {
					emit(map[string]any{"type": "response.refusal.delta", "output_index": index, "item_id": item["id"], "content_index": ci, "delta": part["refusal"]})
					emit(map[string]any{"type": "response.refusal.done", "output_index": index, "item_id": item["id"], "content_index": ci, "refusal": part["refusal"]})
				}
				emit(map[string]any{"type": "response.content_part.done", "output_index": index, "item_id": item["id"], "content_index": ci, "part": part})
			}
		}
		emit(map[string]any{"type": "response.output_item.done", "output_index": index, "item": item})
	}
	terminal := "response.completed"
	switch response["status"] {
	case "incomplete":
		terminal = "response.incomplete"
	case "failed":
		terminal = "response.failed"
	}
	emit(map[string]any{"type": terminal, "response": response})
	return firstErr
}

func writeImageBridgeWebSocketResponses(ctx context.Context, conn *coderws.Conn, response map[string]any, timeout time.Duration) error {
	if conn == nil {
		return errors.New("image bridge websocket is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return emitImageBridgeResponseEvents(response, func(event map[string]any) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		writeCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return conn.Write(writeCtx, coderws.MessageText, payload)
	})
}
