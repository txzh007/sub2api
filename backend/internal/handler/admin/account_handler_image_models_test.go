package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type imageModelsUpstream struct {
	syncUpstreamHTTPUpstream
	requests []*http.Request
}

func (u *imageModelsUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	u.requests = append(u.requests, req)
	return u.syncUpstreamHTTPUpstream.Do(req, proxyURL, accountID, concurrency)
}

func (u *imageModelsUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestPreviewImageModelsUsesDraftSettingsWithoutSaving(t *testing.T) {
	for _, tc := range []struct {
		name, body, url, header, key, upstreamBody string
	}{
		{"create OpenAI", `{"platform":"openai","base_url":"https://images.example/v1","api_key":"new-key"}`, "https://images.example/v1/models", "Authorization", "Bearer new-key", `{"data":[{"id":"gpt-image-2"},{"id":"gemini-3.1-flash-image"}]}`},
		{"create Gemini", `{"platform":"gemini","base_url":"https://images.example/v1beta","api_key":"gemini-key"}`, "https://images.example/v1beta/models", "x-goog-api-key", "gemini-key", `{"models":[{"name":"models/gemini-3.1-flash-image"}]}`},
		{"edit with saved key and changed URL", `{"account_id":44,"platform":"openai","base_url":"https://changed.example/v1"}`, "https://changed.example/v1/models", "Authorization", "Bearer saved-key", `{"data":[{"id":"gpt-image-2"}]}`},
		{"edit with replacement key", `{"account_id":44,"platform":"openai","base_url":"https://changed.example/v1","api_key":"replacement-key"}`, "https://changed.example/v1/models", "Authorization", "Bearer replacement-key", `{"data":[{"id":"gpt-image-2"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credentials := map[string]any{"api_key": "saved-key", "base_url": "https://original.example/v1", "model_mapping": map[string]any{"gpt-image-old": "gpt-image-old"}}
			svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{
				ID: 44, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: credentials,
			}}
			upstream := &imageModelsUpstream{syncUpstreamHTTPUpstream: syncUpstreamHTTPUpstream{resp: &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.upstreamBody)),
			}}}
			router := setupSyncUpstreamModelsRouter(svc, upstream)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/image-preview", strings.NewReader(tc.body)))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Len(t, upstream.requests, 1, "must only fetch the live list, without registry enrichment")
			require.Equal(t, tc.url, upstream.requests[0].URL.String())
			require.Equal(t, tc.key, upstream.requests[0].Header.Get(tc.header))
			require.Equal(t, http.MethodGet, upstream.requests[0].Method)
			var result struct {
				Data struct {
					Models []string `json:"models"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.NotEmpty(t, result.Data.Models)
			require.NotContains(t, result.Data.Models, "gpt-image-old")
			require.NotContains(t, rec.Body.String(), "saved-key")
			require.NotContains(t, rec.Body.String(), "replacement-key")
			require.Equal(t, "saved-key", credentials["api_key"])
			require.Equal(t, "https://original.example/v1", credentials["base_url"])
		})
	}
}

func TestPreviewImageModelsRejectsInvalidCredentials(t *testing.T) {
	for _, body := range []string{
		`{"platform":"openai","base_url":"https://images.example"}`,
		`{"platform":"openai","base_url":" ","api_key":"key"}`,
		`{"platform":"claude","base_url":"https://images.example","api_key":"key"}`,
		`{"account_id":44,"platform":"gemini","base_url":"https://images.example"}`,
	} {
		svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{
			ID: 44, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		}}
		upstream := &imageModelsUpstream{}
		router := setupSyncUpstreamModelsRouter(svc, upstream)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/image-preview", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Empty(t, upstream.requests)
	}
}

func TestPreviewImageModelsDoesNotFallbackOrExposeUpstreamErrorBody(t *testing.T) {
	svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{
		ID: 44, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "saved-key", "model_mapping": map[string]any{"gpt-image-old": "gpt-image-old"}},
	}}
	upstream := &imageModelsUpstream{syncUpstreamHTTPUpstream: syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"error":"SECRET_TOKEN"}`)),
	}}}
	router := setupSyncUpstreamModelsRouter(svc, upstream)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/image-preview", strings.NewReader(`{"account_id":44,"platform":"openai","base_url":"https://images.example/v1"}`)))
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "HTTP 404")
	require.NotContains(t, rec.Body.String(), "SECRET_TOKEN")
	require.NotContains(t, rec.Body.String(), "gpt-image-old")
	require.Len(t, upstream.requests, 1)
}
