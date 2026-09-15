package handler

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	codexBridgeImageDirectory = "codex-generated-images"
	codexBridgeImageRoute     = "/v1/sub2api/generated-images/"
	codexBridgeImageURLField  = "_sub2api_image_url"
)

var codexBridgeImageFormats = map[string]string{
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"webp": "image/webp",
	"gif":  "image/gif",
}

func (h *GatewayHandler) saveCodexBridgeImage(c *gin.Context, encoded, contentType string) (string, error) {
	root := h.codexBridgeImageRoot()
	if root == "" {
		return "", errors.New("Codex bridge image data directory is not configured")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("decode Codex bridge image: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("Codex bridge image is empty")
	}
	extension, _ := codexBridgeImageFormat(contentType, data)
	if extension == "" {
		return "", fmt.Errorf("unsupported Codex bridge image content type %q", contentType)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", fmt.Errorf("create Codex bridge image directory: %w", err)
	}
	name := uuid.NewString() + "." + extension
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", fmt.Errorf("store Codex bridge image: %w", err)
	}
	return codexBridgeImagePublicBaseURL(c) + codexBridgeImageRoute + name, nil
}

func (h *GatewayHandler) codexBridgeImageRoot() string {
	if dir := strings.TrimSpace(os.Getenv("DATA_DIR")); dir != "" {
		return filepath.Join(filepath.Clean(dir), codexBridgeImageDirectory)
	}
	if h != nil && h.cfg != nil {
		if dir := strings.TrimSpace(h.cfg.Pricing.DataDir); dir != "" {
			return filepath.Join(filepath.Clean(dir), codexBridgeImageDirectory)
		}
	}
	return ""
}

func codexBridgeImageFormat(contentType string, data []byte) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	for extension, candidate := range codexBridgeImageFormats {
		if normalized == candidate {
			if extension == "jpg" {
				continue
			}
			return extension, candidate
		}
	}
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	for extension, candidate := range codexBridgeImageFormats {
		if detected == candidate {
			if extension == "jpg" {
				continue
			}
			return extension, candidate
		}
	}
	return "", ""
}

func codexBridgeImagePublicBaseURL(c *gin.Context) string {
	scheme := "http"
	if c != nil && c.Request != nil {
		if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		} else if c.Request.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + c.Request.Host
	}
	return ""
}

// CodexBridgeImage serves the persisted result referenced by the Markdown
// fallback. Names are random UUIDs; no API key is required because image loads
// initiated by Markdown renderers do not carry the Responses authorization.
func (h *GatewayHandler) CodexBridgeImage(c *gin.Context) {
	name := strings.TrimSpace(c.Param("image_id"))
	if !validCodexBridgeImageName(name) {
		c.Status(http.StatusNotFound)
		return
	}
	root := h.codexBridgeImageRoot()
	if root == "" {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(root, name)
	file, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	c.Header("Content-Type", codexBridgeImageFormats[extension])
	c.Header("Content-Disposition", `inline; filename="`+name+`"`)
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, name, stat.ModTime(), file)
}

func validCodexBridgeImageName(name string) bool {
	if filepath.Base(name) != name {
		return false
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if _, ok := codexBridgeImageFormats[extension]; !ok {
		return false
	}
	id := strings.TrimSuffix(name, filepath.Ext(name))
	_, err := uuid.Parse(id)
	return err == nil
}

func takeCodexBridgeImageURLs(items []any) []string {
	urls := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		if value, _ := item[codexBridgeImageURLField].(string); strings.TrimSpace(value) != "" {
			urls = append(urls, strings.TrimSpace(value))
		}
		delete(item, codexBridgeImageURLField)
	}
	return urls
}

func appendCodexBridgeImageMarkdown(output []any, urls []string) []any {
	if len(urls) == 0 {
		return output
	}
	lines := make([]string, 0, len(urls))
	for i, imageURL := range urls {
		label := "generated image"
		if len(urls) > 1 {
			label = fmt.Sprintf("generated image %d", i+1)
		}
		lines = append(lines, fmt.Sprintf("![%s](%s)", label, imageURL))
	}
	return append(output, map[string]any{
		"id":     "msg_sub2api_image_" + uuid.NewString(),
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []any{map[string]any{
			"type":        "output_text",
			"text":        strings.Join(lines, "\n\n"),
			"annotations": []any{},
		}},
	})
}
