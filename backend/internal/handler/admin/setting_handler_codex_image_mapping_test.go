//go:build unit

package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codexImageMappingSettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *codexImageMappingSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *codexImageMappingSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestCodexImageModelMappingSettingsRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &codexImageMappingSettingRepo{values: map[string]string{}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/codex-image-model-mapping", bytes.NewBufferString(`{"builtin_model":"gpt-image-2","target_model":"gemini-3.1-flash-image"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateCodexImageModelMappingSettings(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "gpt-image-2", gjson.Get(recorder.Body.String(), "data.builtin_model").String())
	require.Equal(t, "gemini-3.1-flash-image", gjson.Get(recorder.Body.String(), "data.target_model").String())

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/codex-image-model-mapping", nil)
	handler.GetCodexImageModelMappingSettings(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "gemini-3.1-flash-image", gjson.Get(recorder.Body.String(), "data.target_model").String())
}

func TestCodexImageModelMappingSettingsRejectsInvalidBuiltinModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSettingHandler(service.NewSettingService(&codexImageMappingSettingRepo{values: map[string]string{}}, &config.Config{}), nil, nil, nil, nil, nil, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/codex-image-model-mapping", bytes.NewBufferString(`{"builtin_model":"gpt-5.4","target_model":"gemini-3.1-flash-image"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateCodexImageModelMappingSettings(c)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
