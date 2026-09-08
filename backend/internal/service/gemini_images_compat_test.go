package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiImagesJSONGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gemini-3.1-flash-image","prompt":"画一只猫","size":"1536x1024"}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	parsed, err := ParseGeminiImagesRequest(c, body)
	require.NoError(t, err)
	upstream, err := GeminiImagesRequestBody(parsed)
	require.NoError(t, err)
	require.Equal(t, "画一只猫", gjson.GetBytes(upstream, "contents.0.parts.0.text").String())
	require.Equal(t, "IMAGE", gjson.GetBytes(upstream, "generationConfig.responseModalities.1").String())
	require.Equal(t, "3:2", gjson.GetBytes(upstream, "generationConfig.imageConfig.aspectRatio").String())
}

func TestGeminiImagesMultipartEdit(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "gemini-2.5-flash-image"))
	require.NoError(t, w.WriteField("prompt", "把背景改成蓝色"))
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="reference.png"`)
	h.Set("Content-Type", "image/png")
	part, err := w.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write([]byte("test-image"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	parsed, err := ParseGeminiImagesRequest(c, body.Bytes())
	require.NoError(t, err)
	upstream, err := GeminiImagesRequestBody(parsed)
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("test-image")), gjson.GetBytes(upstream, "contents.0.parts.1.inlineData.data").String())
}

func TestGeminiImagesRejectsUnsupportedInput(t *testing.T) {
	for _, payload := range []string{
		`{"prompt":"draw","n":2}`,
		`{"prompt":"draw","n":1.5}`,
		`{"prompt":12}`,
		`{"prompt":"draw","partial_images":1}`,
		`{"prompt":"draw","quality":"high"}`,
		`{"prompt":"draw","stream":"true"}`,
		`{"prompt":"draw","mask":{"image_url":"https://example.test/mask.png"}}`,
		`{"prompt":"draw","output_format":"webp"}`,
		`{"prompt":"draw","model":"../escape"}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		_, err := ParseGeminiImagesRequest(c, []byte(payload))
		require.Error(t, err, payload)
	}
	for _, ref := range []string{"https://example.test/image.png", "file:///etc/passwd", "http://127.0.0.1/image", "data:text/plain;base64,YQ==", "data:image/png;base64,%%%"} {
		_, err := GeminiImageReferencePart(ref)
		require.Error(t, err, ref)
	}
}

func TestGeminiImagesSizesAndUntypedUpload(t *testing.T) {
	config, err := geminiImagesSizeConfig("2520x1080")
	require.NoError(t, err)
	require.Equal(t, "21:9", config["aspectRatio"])
	_, err = geminiImagesSizeConfig("1200x1000")
	require.Error(t, err)
	// A valid JPEG signature is enough for MIME sniffing. Real image validation
	// remains upstream, as for the existing OpenAI multipart upload path.
	body, err := GeminiImagesRequestBody(&OpenAIImagesRequest{Prompt: "edit", Uploads: []OpenAIImagesUpload{{ContentType: "application/octet-stream", Data: []byte{0xff, 0xd8, 0xff, 0xe0}}}})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", gjson.GetBytes(body, "contents.0.parts.1.inlineData.mimeType").String())
}

func TestGeminiImagesEmptyOutputDoesNotChargeAnImage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/test:generateContent", nil).WithContext(WithGeminiImageOutputRequired(context.Background()))
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, []byte(`{"candidates":[{"finishReason":"SAFETY","content":{"parts":[{"text":"refused"}]}}]}`))
	require.Zero(t, resolveGeminiImageCount(c, DefaultGeminiImageModel, DefaultGeminiImageModel))
	observeGeminiImageOutputs(c, []byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`))
	require.Equal(t, 1, resolveGeminiImageCount(c, DefaultGeminiImageModel, DefaultGeminiImageModel))
}

func TestGeminiImagesExternalToolPreventsDuplicateNativeInjection(t *testing.T) {
	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{featureKeyCodexImageGenerationBridge: true}}
	require.True(t, svc.isCodexImageGenerationBridgeEnabled(context.Background(), account, nil))
	require.False(t, svc.isCodexImageGenerationBridgeEnabled(WithExternalImageTool(context.Background()), account, nil))
}

func TestGeminiImagesResponsePreservesImagesAndUsage(t *testing.T) {
	body := []byte(`{"candidates":[{"content":{"parts":[{"text":"done"},{"thought":true,"inlineData":{"mimeType":"image/png","data":"dGhvdWdodA=="}},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2Ux"}},{"inline_data":{"mime_type":"image/jpeg","data":"aW1hZ2Uy"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`)
	result, err := GeminiImagesResponse(body, DefaultGeminiImageModel, "b64_json")
	require.NoError(t, err)
	require.Len(t, result.Data, 2)
	require.Equal(t, "aW1hZ2Ux", result.Data[0].B64JSON)
	require.Equal(t, int64(30), result.Usage["total_tokens"])
	wrapped, _ := json.Marshal(map[string]json.RawMessage{"response": body})
	result, err = GeminiImagesResponse(wrapped, DefaultGeminiImageModel, "url")
	require.NoError(t, err)
	require.Equal(t, "data:image/png;base64,aW1hZ2Ux", result.Data[0].URL)
	_, err = GeminiImagesResponse([]byte(`{"candidates":[{"finishReason":"SAFETY","content":{"parts":[{"text":"refused"}]}}]}`), DefaultGeminiImageModel, "")
	require.ErrorContains(t, err, "no image")
}
