package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const maxImageBridgeResponseBytes = 64 << 20

func (h *GatewayHandler) SetImageGateway(gateway *OpenAIGatewayHandler) { h.imageGateway = gateway }

// GeminiImages exposes Gemini's native image generation through the Images
// protocol. Execution reuses the native gateway's auth context, scheduling,
// moderation, concurrency, retries and usage settlement.
func (h *GatewayHandler) GeminiImages(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.responsesErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.responsesErrorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		status := http.StatusBadRequest
		if _, ok := extractMaxBytesError(err); ok {
			status = http.StatusRequestEntityTooLarge
		}
		h.responsesErrorResponse(c, status, "invalid_request_error", "Failed to read image request body")
		return
	}
	parsed, err := service.ParseGeminiImagesRequest(c, body)
	if err != nil {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	nativeBody, err := service.GeminiImagesRequestBody(parsed)
	if err != nil {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if h.imageGateway != nil {
		// Share the same process-wide image limiter as the OpenAI Images route.
		release, acquired := h.imageGateway.acquireImageGenerationSlot(c, false)
		if !acquired {
			return
		}
		if release != nil {
			defer release()
		}
	}
	stop := startImageBridgeKeepalive(c, parsed.Stream)
	defer stop()
	result, capture, err := h.executeGeminiImage(c, parsed.Model, nativeBody, parsed.ResponseFormat)
	stop()
	if err != nil {
		writeImageBridgeError(c, capture, err)
		return
	}
	if id := capture.Header().Get("x-request-id"); id != "" {
		c.Header("x-request-id", id)
	}
	if !parsed.Stream {
		c.JSON(http.StatusOK, result)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	prefix := "image_generation"
	if parsed.IsEdits() {
		prefix = "image_edit"
	}
	for index, img := range result.Data {
		event := map[string]any{"type": prefix + ".completed", "created_at": result.Created, "image_index": index, "model": result.Model, "output_format": strings.TrimPrefix(img.MimeType, "image/"), "usage": result.Usage}
		if img.B64JSON != "" {
			event["b64_json"] = img.B64JSON
		}
		if img.URL != "" {
			event["url"] = img.URL
		}
		writeImageBridgeSSE(c, event)
	}
}

func (h *GatewayHandler) executeGeminiImage(c *gin.Context, model string, body []byte, responseFormat string) (*service.GeminiCompatibleImagesResponse, *imageBridgeCapture, error) {
	child, capture := imageBridgeChildContext(c, body)
	child.Request = child.Request.WithContext(service.WithGeminiImageOutputRequired(child.Request.Context()))
	child.Request.Header.Set("Content-Type", "application/json")
	child.Request.URL.Path = "/v1beta/models/" + model + ":generateContent"
	child.Request.URL.RawPath = ""
	child.Params = gin.Params{{Key: "modelAction", Value: "/" + model + ":generateContent"}}
	h.GeminiV1BetaModels(child)
	// Preserve selected-account attribution for the outer ops middleware.
	for key, value := range child.Keys {
		c.Set(key, value)
	}
	if capture.err != nil {
		return nil, capture, capture.err
	}
	if capture.Status() >= 400 {
		return nil, capture, fmt.Errorf("Gemini request failed")
	}
	result, err := service.GeminiImagesResponse(capture.body.Bytes(), model, responseFormat)
	return result, capture, err
}

// imageBridgeCapture bounds the internal response before translating it. Flush
// commits only this internal writer; it never leaks native Gemini/SSE frames.
type imageBridgeCapture struct {
	gin.ResponseWriter
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
	err     error
}

func (w *imageBridgeCapture) Header() http.Header { return w.header }
func (w *imageBridgeCapture) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *imageBridgeCapture) WriteHeaderNow() { w.written = true }
func (w *imageBridgeCapture) Write(p []byte) (int, error) {
	w.WriteHeaderNow()
	if w.body.Len()+len(p) > maxImageBridgeResponseBytes {
		w.err = fmt.Errorf("image bridge response exceeds size limit")
		return 0, w.err
	}
	return w.body.Write(p)
}
func (w *imageBridgeCapture) WriteString(p string) (int, error) { return w.Write([]byte(p)) }
func (w *imageBridgeCapture) Flush()                            { w.WriteHeaderNow() }
func (w *imageBridgeCapture) Status() int                       { return w.status }
func (w *imageBridgeCapture) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *imageBridgeCapture) Written() bool { return w.written }

func imageBridgeChildContext(c *gin.Context, body []byte) (*gin.Context, *imageBridgeCapture) {
	// Gin Copy also reads its writer state. Serialize it with keepalive writes.
	if value, ok := c.Get(imageBridgeWriterMutexKey); ok {
		mu := value.(*sync.Mutex)
		mu.Lock()
		defer mu.Unlock()
	}
	child := c.Copy()
	child.Request = c.Request.Clone(c.Request.Context())
	child.Request.Body = io.NopCloser(bytes.NewReader(body))
	child.Request.ContentLength = int64(len(body))
	capture := &imageBridgeCapture{ResponseWriter: c.Writer, header: make(http.Header), status: http.StatusOK}
	child.Writer = capture
	return child, capture
}

func writeImageBridgeError(c *gin.Context, capture *imageBridgeCapture, err error) {
	status := http.StatusBadGateway
	message := err.Error()
	typ := "upstream_error"
	if code := infraerrors.Code(err); code >= 400 && code < 500 {
		status = code
		message = infraerrors.Message(err)
		typ = "invalid_request_error"
		if code == http.StatusForbidden {
			typ = "permission_error"
		}
	}
	if capture != nil && capture.Status() >= 400 {
		status = capture.Status()
		if msg := gjson.GetBytes(capture.body.Bytes(), "error.message").String(); msg != "" {
			message = msg
		}
		if t := gjson.GetBytes(capture.body.Bytes(), "error.type").String(); t != "" {
			typ = t
		}
		if status == http.StatusBadRequest {
			typ = "invalid_request_error"
		}
		if retry := capture.Header().Get("Retry-After"); retry != "" {
			c.Header("Retry-After", retry)
		}
	}
	if c.Writer.Written() && strings.Contains(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
		writeImageBridgeSSE(c, map[string]any{"type": "error", "code": typ, "message": message, "status_code": status})
		return
	}
	c.Header("Content-Type", "application/json")
	c.JSON(status, gin.H{"error": gin.H{"type": typ, "message": message}})
}

func writeImageBridgeSSE(c *gin.Context, event map[string]any) {
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event["type"], body)
	c.Writer.Flush()
}

const imageBridgeWriterMutexKey = "gemini_image_bridge_writer_mutex"

// Keepalive owns the downstream writer while internal calls use isolated
// capture writers. stop joins it before any final response/error is written.
func startImageBridgeKeepalive(c *gin.Context, stream bool) func() {
	return startImageBridgeKeepaliveInterval(c, stream, 10*time.Second)
}

func startImageBridgeKeepaliveInterval(c *gin.Context, stream bool, interval time.Duration) func() {
	if !stream {
		return func() {}
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	mu := &sync.Mutex{}
	c.Set(imageBridgeWriterMutexKey, mu)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-c.Request.Context().Done():
				return
			case <-ticker.C:
				mu.Lock()
				_, err := c.Writer.Write([]byte(": image bridge working\n\n"))
				c.Writer.Flush()
				mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	once := &sync.Once{}
	return func() {
		once.Do(func() { close(stop); <-done })
	}
}
