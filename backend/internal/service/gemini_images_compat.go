package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const DefaultGeminiImageModel = "gemini-3.1-flash-image"

type geminiImageOutputRequiredKey struct{}

// WithGeminiImageOutputRequired makes Images requests charge only observed
// images. Native text-only/refused results must not fall back to one image.
func WithGeminiImageOutputRequired(ctx context.Context) context.Context {
	return context.WithValue(ctx, geminiImageOutputRequiredKey{}, true)
}

// ParseGeminiImagesRequest shares the Images wire format without applying the
// GPT-only model whitelist. Group/account model mappings remain authoritative.
func ParseGeminiImagesRequest(c *gin.Context, body []byte) (*OpenAIImagesRequest, error) {
	r := &OpenAIImagesRequest{Endpoint: normalizeOpenAIImagesEndpointPath(c.Request.URL.Path), ContentType: c.GetHeader("Content-Type"), N: 1, Body: body}
	if r.Endpoint == "" {
		return nil, fmt.Errorf("unsupported images endpoint")
	}
	mediaType, _, _ := mime.ParseMediaType(r.ContentType)
	r.Multipart = mediaType == "multipart/form-data"
	var err error
	if r.Multipart {
		err = parseOpenAIImagesMultipartRequest(body, r.ContentType, r)
	} else if !gjson.ValidBytes(body) {
		err = fmt.Errorf("invalid JSON request body")
	} else {
		for _, field := range []string{"model", "prompt", "size", "quality", "response_format", "background", "output_format"} {
			if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.String {
				return nil, fmt.Errorf("%s must be a string", field)
			}
		}
		if n := gjson.GetBytes(body, "n"); n.Exists() && (n.Type != gjson.Number || n.Float() != 1) {
			return nil, fmt.Errorf("Gemini Images currently supports n=1")
		}
		err = parseOpenAIImagesJSONRequest(body, r)
	}
	if err != nil {
		return nil, err
	}
	if r.Model == "" {
		r.Model = DefaultGeminiImageModel
	}
	if !IsSafeGeminiModelPathSegment(r.Model) {
		return nil, fmt.Errorf("invalid image model")
	}
	if r.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if r.N != 1 {
		return nil, fmt.Errorf("Gemini Images currently supports n=1; submit separate requests for multiple images")
	}
	if r.HasMask || r.MaskUpload != nil || r.MaskImageURL != "" {
		return nil, fmt.Errorf("Gemini Images does not support mask; describe the edit in the prompt")
	}
	if r.ResponseFormat != "" && r.ResponseFormat != "b64_json" && r.ResponseFormat != "url" {
		return nil, fmt.Errorf("response_format must be b64_json or url")
	}
	if r.OutputFormat != "" && r.OutputFormat != "png" {
		return nil, fmt.Errorf("Gemini Images currently supports output_format=png")
	}
	if r.Background == "transparent" {
		return nil, fmt.Errorf("Gemini Images does not support transparent backgrounds")
	}
	if r.Quality != "" && r.Quality != "auto" {
		return nil, fmt.Errorf("Gemini Images does not support quality; use size to select resolution")
	}
	if r.PartialImages != nil && *r.PartialImages != 0 {
		return nil, fmt.Errorf("Gemini Images does not support partial_images")
	}
	r.SizeTier = normalizeOpenAIImageSizeTier(r.Size)
	return r, nil
}

// GeminiImagesRequestBody translates Images input into generateContent input.
// Reference files stay in memory and reuse the existing upload size limits.
func GeminiImagesRequestBody(r *OpenAIImagesRequest) ([]byte, error) {
	parts := []any{map[string]any{"text": r.Prompt}}
	for _, upload := range r.Uploads {
		contentType := upload.ContentType
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(upload.Data)
		}
		if len(upload.Data) == 0 || !strings.HasPrefix(contentType, "image/") {
			return nil, fmt.Errorf("reference image must contain image data")
		}
		parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": contentType, "data": base64.StdEncoding.EncodeToString(upload.Data)}})
	}
	for _, imageURL := range r.InputImageURLs {
		part, err := GeminiImageReferencePart(imageURL)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if r.IsEdits() && len(parts) == 1 {
		return nil, fmt.Errorf("image input is required")
	}
	imageConfig, err := geminiImagesSizeConfig(r.Size)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}, "imageConfig": imageConfig},
	})
}

func GeminiImageReferencePart(value string) (map[string]any, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
		mimeType, encoding, _ := strings.Cut(header, ";")
		if !ok || !strings.HasPrefix(mimeType, "image/") || encoding != "base64" || data == "" {
			return nil, fmt.Errorf("reference image must be an image base64 data URL")
		}
		if len(data) > base64.StdEncoding.EncodedLen(openAIImageMaxUploadPartSize) {
			return nil, fmt.Errorf("reference image exceeds size limit")
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return nil, fmt.Errorf("invalid reference image base64")
		}
		return map[string]any{"inlineData": map[string]any{"mimeType": mimeType, "data": data}}, nil
	}
	// A remote URL has no reliable MIME type and ordinary public URLs are not
	// portable across Gemini backends. Accept bytes without a gateway URL fetch.
	return nil, fmt.Errorf("Gemini reference images must be uploaded as multipart files or base64 data URLs")
}

func geminiImagesSizeConfig(size string) (map[string]any, error) {
	config := map[string]any{}
	size = strings.TrimSpace(size)
	if size == "" || size == "auto" {
		return config, nil
	}
	if size == "1K" || size == "2K" || size == "4K" {
		config["imageSize"] = size
		return config, nil
	}
	x, y, ok := strings.Cut(strings.ToLower(size), "x")
	w, ew := strconv.Atoi(x)
	h, eh := strconv.Atoi(y)
	if !ok || ew != nil || eh != nil || w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("size must be auto, 1K, 2K, 4K or WIDTHxHEIGHT")
	}
	a, b := w, h
	for b != 0 {
		a, b = b, a%b
	}
	ratio := fmt.Sprintf("%d:%d", w/a, h/a)
	if ratio == "7:3" {
		ratio = "21:9"
	}
	switch ratio {
	case "1:1", "1:4", "1:8", "2:3", "3:2", "3:4", "4:1", "4:3", "4:5", "5:4", "8:1", "9:16", "16:9", "21:9":
	default:
		return nil, fmt.Errorf("aspect ratio %s is not supported by Gemini Images", ratio)
	}
	config["aspectRatio"] = ratio
	config["imageSize"] = normalizeOpenAIImageSizeTier(size)
	return config, nil
}

type GeminiCompatibleImage struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	MimeType      string `json:"mime_type,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type GeminiCompatibleImagesResponse struct {
	Created int64                   `json:"created"`
	Model   string                  `json:"model"`
	Data    []GeminiCompatibleImage `json:"data"`
	Usage   map[string]any          `json:"usage,omitempty"`
}

func GeminiImagesResponse(body []byte, model, responseFormat string) (*GeminiCompatibleImagesResponse, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid Gemini image response")
	}
	root := gjson.ParseBytes(body)
	if wrapped := root.Get("response"); wrapped.IsObject() {
		root = wrapped
	}
	r := &GeminiCompatibleImagesResponse{Created: time.Now().Unix(), Model: model, Data: []GeminiCompatibleImage{}}
	for _, candidate := range root.Get("candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if part.Get("thought").Bool() {
				continue
			}
			inline := part.Get("inlineData")
			if !inline.Exists() {
				inline = part.Get("inline_data")
			}
			mimeType := inline.Get("mimeType").String()
			if mimeType == "" {
				mimeType = inline.Get("mime_type").String()
			}
			data := inline.Get("data").String()
			if !strings.HasPrefix(mimeType, "image/") || data == "" {
				continue
			}
			if _, err := base64.StdEncoding.DecodeString(data); err != nil {
				return nil, fmt.Errorf("invalid Gemini image data")
			}
			item := GeminiCompatibleImage{B64JSON: data, MimeType: mimeType}
			if responseFormat == "url" {
				item.URL = "data:" + mimeType + ";base64," + data
				item.B64JSON = ""
			}
			r.Data = append(r.Data, item)
		}
	}
	if len(r.Data) == 0 {
		reason := root.Get("promptFeedback.blockReason").String()
		if reason == "" {
			reason = root.Get("candidates.0.finishReason").String()
		}
		return nil, fmt.Errorf("Gemini returned no image (finish reason: %s)", reason)
	}
	u := root.Get("usageMetadata")
	if u.Exists() {
		r.Usage = map[string]any{"input_tokens": u.Get("promptTokenCount").Int(), "output_tokens": u.Get("candidatesTokenCount").Int(), "total_tokens": u.Get("totalTokenCount").Int()}
	}
	return r, nil
}
