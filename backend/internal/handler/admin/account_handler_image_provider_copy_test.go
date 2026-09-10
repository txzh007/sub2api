package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageProviderCopyAdminService struct {
	service.AdminService
	source             *service.Account
	group              *service.Group
	duplicate          *service.Account
	duplicateCalls     int
	duplicateKey       string
	updatedAccountID   int64
	updatedGroupIDs    []int64
	updatedCredentials map[string]any
	skipMixedRiskCheck bool
}

func (s *imageProviderCopyAdminService) GetAccount(context.Context, int64) (*service.Account, error) {
	return s.source, nil
}

func (s *imageProviderCopyAdminService) GetGroup(context.Context, int64) (*service.Group, error) {
	return s.group, nil
}

func (s *imageProviderCopyAdminService) DuplicateAccount(_ context.Context, _ int64, _, operationKey string) (*service.Account, error) {
	s.duplicateCalls++
	s.duplicateKey = operationKey
	return s.duplicate, nil
}

func (s *imageProviderCopyAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updatedAccountID = id
	s.updatedGroupIDs = append([]int64(nil), (*input.GroupIDs)...)
	s.updatedCredentials = input.Credentials
	s.skipMixedRiskCheck = input.SkipMixedChannelCheck
	updated := *s.duplicate
	updated.GroupIDs = append([]int64(nil), s.updatedGroupIDs...)
	updated.Credentials = input.Credentials
	return &updated, nil
}

func setupImageProviderCopyRouter(t *testing.T, svc service.AdminService) *gin.Engine {
	t.Helper()
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/copy-to-image-provider", handler.CopyToImageProvider)
	return router
}

func TestCopyToImageProviderCopiesServerSideAndRebindsOnlyImageGroup(t *testing.T) {
	svc := &imageProviderCopyAdminService{
		source: &service.Account{ID: 42, Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey},
		group:  &service.Group{ID: 24, Name: "生图", Status: service.StatusActive, AllowImageGeneration: true},
		duplicate: &service.Account{
			ID: 43, Name: "Grok (Copy)", Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: false, Credentials: map[string]any{
				"api_key": "secret",
				"model_mapping": map[string]any{
					"grok-4.6":                    "grok-4.6",
					"grok-imagine":                "grok-imagine-image-quality",
					"draw-alias":                  "grok-imagine-image-2.0",
					"grok-imagine-video-1.5":      "grok-imagine-video-1.5",
					"xai/grok-imagine-image":      "grok-imagine-image",
					"x-ai/grok-imagine-video-1.5": "grok-imagine-video-1.5",
				},
			},
		},
	}
	router := setupImageProviderCopyRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/copy-to-image-provider", strings.NewReader(`{"group_id":24}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "copy-42-to-24")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, svc.duplicateCalls)
	require.Equal(t, "image-provider:24:copy-42-to-24", svc.duplicateKey)
	require.Equal(t, int64(43), svc.updatedAccountID)
	require.Equal(t, []int64{24}, svc.updatedGroupIDs)
	require.True(t, svc.skipMixedRiskCheck)
	require.Equal(t, map[string]any{
		"grok-imagine":           "grok-imagine-image-quality",
		"draw-alias":             "grok-imagine-image-2.0",
		"xai/grok-imagine-image": "grok-imagine-image",
	}, svc.updatedCredentials["model_mapping"])
	require.Contains(t, recorder.Body.String(), `"name":"Grok (Copy)"`)
	require.NotContains(t, recorder.Body.String(), "grok-4.6")
	require.NotContains(t, recorder.Body.String(), "grok-imagine-video")
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestCopyToImageProviderUsesOnlyDefaultOpenAIImageModels(t *testing.T) {
	svc := &imageProviderCopyAdminService{
		source:    &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		group:     &service.Group{ID: 24, Name: "生图", Status: service.StatusActive, AllowImageGeneration: true},
		duplicate: &service.Account{ID: 43, Name: "OpenAI (Copy)", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}},
	}
	router := setupImageProviderCopyRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/copy-to-image-provider", strings.NewReader(`{"group_id":24}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	mapping, ok := svc.updatedCredentials["model_mapping"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, mapping, "gpt-image-2")
	require.NotContains(t, mapping, "gpt-5.6-sol")
	for model, target := range mapping {
		require.True(t, service.IsImageProviderModel(model) || service.IsImageProviderModel(target.(string)))
	}
}

func TestCopyToImageProviderRejectsUnsupportedSource(t *testing.T) {
	svc := &imageProviderCopyAdminService{
		source: &service.Account{ID: 42, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey},
		group:  &service.Group{ID: 24, Name: "生图", Status: service.StatusActive, AllowImageGeneration: true},
	}
	router := setupImageProviderCopyRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/copy-to-image-provider", strings.NewReader(`{"group_id":24}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "IMAGE_PROVIDER_COPY_SOURCE_UNSUPPORTED")
	require.Zero(t, svc.duplicateCalls)
}

func TestCopyToImageProviderReplaysSameOperation(t *testing.T) {
	svc := &imageProviderCopyAdminService{
		source:    &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		group:     &service.Group{ID: 24, Name: "生图", Status: service.StatusActive, AllowImageGeneration: true},
		duplicate: &service.Account{ID: 43, Name: "Images (Copy)", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	router := setupImageProviderCopyRouter(t, svc)
	repo := newMemoryIdempotencyRepoStub()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, service.DefaultIdempotencyConfig()))
	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/copy-to-image-provider", strings.NewReader(`{"group_id":24}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "copy-replay")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, svc.duplicateCalls)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
}
