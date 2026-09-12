package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration240AddsCompositeClientProtocolScopes(t *testing.T) {
	content, err := FS.ReadFile("240_composite_client_protocol_routes.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS composite_model_routes_endpoint_check")
	require.Contains(t, sql, "'any', 'openai', 'anthropic', 'messages', 'count_tokens'")
}
