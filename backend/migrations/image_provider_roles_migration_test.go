package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration239AddsStableImageProviderIdentities(t *testing.T) {
	content, err := FS.ReadFile("239_image_provider_roles.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS system_role VARCHAR(32) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS purpose VARCHAR(32) NOT NULL DEFAULT 'general'")
	require.Contains(t, sql, "SET system_role = 'image_generation'")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS groups_unique_live_system_role")
	require.Contains(t, sql, "SET purpose = 'image_provider'")
	require.Contains(t, sql, "a.type = 'apikey'")
	require.Contains(t, sql, "a.platform IN ('openai', 'gemini', 'grok')")
}

func TestMigration239SeparatesVideoAndRepairsKnownGrokImageAliasSafely(t *testing.T) {
	content, err := FS.ReadFile("239_image_provider_roles.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "lower(model) NOT LIKE '%grok-imagine-video%'")
	require.Contains(t, sql, "lower(model) NOT LIKE '%grok-video%'")
	require.Contains(t, sql, "THEN 'grok-imagine-image-2.0'")
	require.Contains(t, sql, "a.status = 'active'")
	require.Contains(t, sql, "a.schedulable = TRUE")
	require.Contains(t, sql, "? 'grok-imagine-image-2.0'")
	require.Contains(t, sql, "? 'grok-imagine-image'")
}
