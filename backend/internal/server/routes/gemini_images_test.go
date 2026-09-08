package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiImagesRoutesReachPermissionGate(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformComposite} {
		router := newGatewayRoutesTestRouter(platform)
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits", "/images/generations", "/images/edits"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gemini-3.1-flash-image","prompt":"draw a cat"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			// The fixture disables images. Reaching the permission gate verifies
			// routing through the actual middleware without contacting upstream.
			require.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", platform, path, w.Body.String())
			require.Contains(t, w.Body.String(), service.ImageGenerationPermissionMessage())
		}
	}
}
