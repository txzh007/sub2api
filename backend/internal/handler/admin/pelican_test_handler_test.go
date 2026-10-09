package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pelicanKeyRepositoryStub struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (s pelicanKeyRepositoryStub) GetByID(_ context.Context, _ int64) (*service.APIKey, error) {
	return s.key, nil
}

func TestExtractSafePelicanSVG(t *testing.T) {
	valid := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="20"/></svg>`
	got, err := extractSafePelicanSVG("```svg\n" + valid + "\n```")
	require.NoError(t, err)
	require.Equal(t, valid, got)
	_, err = extractSafePelicanSVG(`<svg><style>.wheel{fill:#334155}</style><defs><circle id="wheel" r="5"/></defs><use href="#wheel"/></svg>`)
	require.NoError(t, err)
	_, err = extractSafePelicanSVG(`<svg><defs><linearGradient id="skyGrad"/></defs><rect fill="url(#skyGrad)"/></svg>`)
	require.NoError(t, err)

	for _, input := range []string{
		"no drawing",
		`<svg><path></svg>`,
		`<svg><script>alert(1)</script></svg>`,
		`<svg onload="alert(1)"><path/></svg>`,
		`<svg><image href="https://example.com/tracker"/></svg>`,
		`<svg><rect style="fill:url(https://example.com/x)"/></svg>`,
		`<svg><style>@import url(https://example.com/x)</style></svg>`,
		`<svg><foreignObject><body/></foreignObject></svg>`,
		`<svg><animateMotion path="M 0 0 L 1 1"/></svg>`,
	} {
		_, err := extractSafePelicanSVG(input)
		require.Error(t, err, input)
	}
}

func TestPelicanModelsUsesOwnKeyAndFiltersMedia(t *testing.T) {
	key := &service.APIKey{
		ID: 2, UserID: 10, Key: "secret", Status: service.StatusActive,
		GroupID: ptrPelicanInt64(7), Group: &service.Group{ID: 7, Status: service.StatusActive},
	}
	h := &PelicanTestHandler{keys: pelicanKeyRepositoryStub{key: key}}
	h.SetGateway(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-5"},{"id":"gpt-image-2"}]}`))
	}))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/pelican-tests/models?key_id=2", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 10})
	h.Models(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var result struct {
		Data struct {
			Models []string `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Equal(t, []string{"claude-sonnet-4-5"}, result.Data.Models)

	unauthorized := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(unauthorized)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/pelican-tests/models?key_id=2", nil)
	h.Models(c)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
}

func TestPelicanCreateRejectsUnavailableModelBeforeBilling(t *testing.T) {
	key := &service.APIKey{
		ID: 2, UserID: 10, Key: "secret", Status: service.StatusActive,
		GroupID: ptrPelicanInt64(7), Group: &service.Group{ID: 7, Status: service.StatusActive},
	}
	h := NewPelicanTestHandler(nil, pelicanKeyRepositoryStub{key: key})
	calls := 0
	h.SetGateway(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-5"}]}`))
	}))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/pelican-tests/runs", strings.NewReader(`{"key_id":2,"model":"unknown-model"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 10})
	h.CreateRun(c)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, 1, calls, "only model listing may reach the gateway")
}

func TestPelicanKeyMustBelongToAdminAndGroup(t *testing.T) {
	key := &service.APIKey{
		ID: 2, UserID: 10, Key: "secret", Status: service.StatusActive,
		GroupID: ptrPelicanInt64(7), Group: &service.Group{ID: 7, Status: service.StatusActive},
	}
	h := &PelicanTestHandler{keys: pelicanKeyRepositoryStub{key: key}}
	_, err := h.ownKey(context.Background(), 10, 2)
	require.NoError(t, err)
	_, err = h.ownKey(context.Background(), 11, 2)
	require.Error(t, err)
	key.IPWhitelist = []string{"192.0.2.1"}
	_, err = h.ownKey(context.Background(), 10, 2)
	require.ErrorContains(t, err, "IP restrictions")
}

func TestPelicanGatewayUsesOneRealGatewayRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewPelicanTestHandler(db, nil)
	posts := 0
	h.SetGateway(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.Equal(t, "pelican-request", r.Header.Get("X-Request-ID"))
		require.Equal(t, "pelican-request", r.Header.Get("X-Client-Request-ID"))
		require.Equal(t, "pelican-request", r.Context().Value(ctxkey.ClientRequestID))
		posts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"<svg><circle r=\"10\"/></svg>"}}]}`))
	}))
	mock.ExpectExec("UPDATE pelican_test_runs SET status").
		WithArgs(int64(1), "success", `<svg><circle r="10"/></svg>`, `<svg><circle r="10"/></svg>`, "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.slots <- struct{}{}
	h.executeRun(1, "secret", "test-model", "pelican-request")
	require.Equal(t, 1, posts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPelicanGatewayErrorAndInvalidSVGAreRecorded(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status           int
		body, wantStatus string
	}{
		{"upstream error", 502, `{"error":{"message":"upstream unavailable"}}`, "error"},
		{"not svg", 200, `{"choices":[{"message":{"content":"a bird"}}]}`, "invalid_svg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			h := NewPelicanTestHandler(db, nil)
			h.SetGateway(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			mock.ExpectExec("UPDATE pelican_test_runs SET status").
				WithArgs(int64(1), tc.wantStatus, sqlmock.AnyArg(), "", sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			h.slots <- struct{}{}
			h.executeRun(1, "secret", "test-model", "pelican-request")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPelicanGatewayTimeoutIsRecorded(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewPelicanTestHandler(db, nil)
	h.timeout = 10 * time.Millisecond
	h.SetGateway(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	mock.ExpectExec("UPDATE pelican_test_runs SET status").
		WithArgs(int64(1), "error", "", "", "test timed out", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.slots <- struct{}{}
	h.executeRun(1, "secret", "test-model", "pelican-request")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPelicanGatewayResponseSizeIsBounded(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewPelicanTestHandler(db, nil)
	h.SetGateway(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", pelicanMaxResponse+1)))
	}))
	mock.ExpectExec("UPDATE pelican_test_runs SET status").
		WithArgs(int64(1), "error", "", "", "gateway response exceeded 1 MiB", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.slots <- struct{}{}
	h.executeRun(1, "secret", "test-model", "pelican-request")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPelicanTextModelExcludesMedia(t *testing.T) {
	require.True(t, pelicanTextModel("claude-sonnet-4-5"))
	for _, name := range []string{"gpt-image-2", "seedance-2", "text-embedding-3-small", "grok-imagine-video"} {
		require.False(t, pelicanTextModel(name), name)
	}
	require.Equal(t, "ab", truncatePelican(strings.Repeat("a", 1)+"bcd", 2))
}

func TestPelicanHistoryCursorReturnsOlderPage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := NewPelicanTestHandler(db, nil)
	mock.ExpectExec("UPDATE pelican_test_runs SET status").WillReturnResult(sqlmock.NewResult(0, 0))
	columns := []string{"id", "admin_user_id", "api_key_id", "group_id", "group_name", "model",
		"prompt_version", "request_id", "status", "response_text", "svg", "error_message",
		"duration_ms", "created_at", "finished_at", "usage_request_id", "input_tokens", "output_tokens", "total_cost", "actual_cost"}
	rows := sqlmock.NewRows(columns)
	for id := int64(60); id >= 10; id-- {
		rows.AddRow(id, 1, 9, 3, "Test group", "test-model", pelicanPromptVer,
			fmt.Sprintf("request-%d", id), "success", "", "", "", 100, time.Now(), nil, nil, nil, nil, nil, nil)
	}
	mock.ExpectQuery("FROM pelican_test_runs r").WithArgs(int64(3), "test-model", int64(61)).WillReturnRows(rows)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/pelican-tests/runs?group_id=3&model=test-model&before_id=61", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	h.ListRuns(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var result struct {
		Data struct {
			Items        []pelicanRun `json:"items"`
			NextBeforeID int64        `json:"next_before_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Len(t, result.Data.Items, 50)
	require.Equal(t, int64(11), result.Data.NextBeforeID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func ptrPelicanInt64(v int64) *int64 { return &v }
