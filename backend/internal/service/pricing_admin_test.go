package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const adminPricingCatalogJSON = `{
	"qwen3-max": {
		"litellm_provider": "dashscope",
		"mode": "chat",
		"deprecation_date": "2999-12-31",
		"input_cost_per_token": 0.000002,
		"output_cost_per_token": 0.000008,
		"supports_prompt_caching": true
	},
	"gpt-5": {
		"litellm_provider": "openai",
		"mode": "chat",
		"deprecation_date": "2000-01-01",
		"input_cost_per_token": 0.00000125,
		"output_cost_per_token": 0.00001
	}
}`

func newAdminPricingService(t *testing.T) (*PricingService, string, string) {
	t.Helper()
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "model_pricing.json")
	overridePath := filepath.Join(dir, "model_pricing_overrides.json")
	require.NoError(t, os.WriteFile(catalogPath, []byte(adminPricingCatalogJSON), 0o644))
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		DataDir:      dir,
		OverrideFile: overridePath,
	}}, nil)
	require.NoError(t, svc.loadPricingData(catalogPath))
	return svc, catalogPath, overridePath
}

func readAdminPricingOverrides(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &entries))
	return entries
}

func TestListAdminModelPricingReturnsCatalogMetadata(t *testing.T) {
	svc, _, _ := newAdminPricingService(t)
	catalog, err := svc.ListAdminModelPricing()
	require.NoError(t, err)
	require.Equal(t, 2, catalog.ModelCount)
	require.Equal(t, 1, catalog.ActiveModelCount)
	require.Equal(t, 1, catalog.DeprecatedModelCount)
	require.Equal(t, 0, catalog.OverrideCount)
	require.Len(t, catalog.Items, 2)
	require.Equal(t, "gpt-5", catalog.Items[0].Model)
	require.True(t, catalog.Items[0].Deprecated)
	require.Equal(t, "2000-01-01", catalog.Items[0].DeprecationDate)
	require.Equal(t, "qwen3-max", catalog.Items[1].Model)
	require.False(t, catalog.Items[1].Deprecated)
	require.Equal(t, "2999-12-31", catalog.Items[1].DeprecationDate)
}

func TestAdminVideoPricingOverrideAppearsInCatalog(t *testing.T) {
	svc, _, _ := newAdminPricingService(t)
	require.NoError(t, svc.UpsertPricingOverride("grok-imagine-video", map[string]any{
		"litellm_provider":      "xai",
		"mode":                  "video",
		"output_cost_per_video": 0.40,
	}))

	catalog, err := svc.ListAdminModelPricing()
	require.NoError(t, err)
	var found *ModelPricingCatalogEntry
	for i := range catalog.Items {
		if catalog.Items[i].Model == "grok-imagine-video" {
			found = &catalog.Items[i]
			break
		}
	}
	require.NotNil(t, found)
	require.InDelta(t, 0.40, found.OutputCostPerVideo, 1e-12)
	require.Equal(t, "video", found.Mode)
}

func TestIsModelDeprecatedOn(t *testing.T) {
	now := time.Date(2026, time.September, 8, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	require.True(t, isModelDeprecatedOn("2026-09-07", now))
	require.True(t, isModelDeprecatedOn("2026-09-08", now))
	require.False(t, isModelDeprecatedOn("2026-09-09", now))
	require.False(t, isModelDeprecatedOn("", now))
	require.False(t, isModelDeprecatedOn("September 8, 2026", now))
}

func TestUpsertPricingOverrideAppliesImmediatelyAndPreservesFields(t *testing.T) {
	svc, _, overridePath := newAdminPricingService(t)
	require.NoError(t, svc.UpsertPricingOverride("qwen3-max", map[string]any{
		"input_cost_per_token": 3e-6,
		"operator_note":        "September price card",
	}))
	require.NoError(t, svc.UpsertPricingOverride("qwen3-max", map[string]any{
		"output_cost_per_token": 9e-6,
	}))

	pricing := svc.GetModelPricing("qwen3-max")
	require.NotNil(t, pricing)
	require.InDelta(t, 3e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 9e-6, pricing.OutputCostPerToken, 1e-12)
	require.True(t, pricing.SupportsPromptCaching)

	overrides := readAdminPricingOverrides(t, overridePath)
	require.Equal(t, "September price card", overrides["qwen3-max"]["operator_note"])
	require.InDelta(t, 3e-6, overrides["qwen3-max"]["input_cost_per_token"].(float64), 1e-12)
	require.InDelta(t, 9e-6, overrides["qwen3-max"]["output_cost_per_token"].(float64), 1e-12)
}

func TestDeletePricingOverrideRestoresCatalogPrice(t *testing.T) {
	svc, _, overridePath := newAdminPricingService(t)
	require.NoError(t, svc.UpsertPricingOverride("qwen3-max", map[string]any{"input_cost_per_token": 6e-6}))
	require.InDelta(t, 6e-6, svc.GetModelPricing("qwen3-max").InputCostPerToken, 1e-12)

	require.NoError(t, svc.DeletePricingOverride("qwen3-max"))
	require.InDelta(t, 2e-6, svc.GetModelPricing("qwen3-max").InputCostPerToken, 1e-12)
	require.NotContains(t, readAdminPricingOverrides(t, overridePath), "qwen3-max")
}

func TestPricingOverrideValidationRejectsUnsafeOrNegativeRules(t *testing.T) {
	svc, _, _ := newAdminPricingService(t)

	err := svc.UpsertPricingOverride("*", map[string]any{"input_cost_per_token": 1e-6})
	require.Error(t, err)
	require.True(t, IsPricingOverrideValidationError(err))

	err = svc.UpsertPricingOverride("qwen3-*", map[string]any{"input_cost_per_token": -1.0})
	require.Error(t, err)
	require.True(t, IsPricingOverrideValidationError(err))
}

func TestConcurrentPricingOverrideWritesKeepEveryRule(t *testing.T) {
	svc, _, overridePath := newAdminPricingService(t)
	models := []string{"qwen3-*", "deepseek-*", "glm-*", "doubao-*"}
	var wg sync.WaitGroup
	errs := make(chan error, len(models))
	for i, model := range models {
		wg.Add(1)
		go func(model string, price float64) {
			defer wg.Done()
			errs <- svc.UpsertPricingOverride(model, map[string]any{
				"litellm_provider":      "domestic",
				"input_cost_per_token":  price,
				"output_cost_per_token": price * 2,
			})
		}(model, float64(i+1)*1e-6)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	overrides := readAdminPricingOverrides(t, overridePath)
	for _, model := range models {
		require.Contains(t, overrides, model)
		require.NotNil(t, svc.GetModelPricing(model))
	}
}

func TestPricingOverrideSaveRollsBackWhenReloadFails(t *testing.T) {
	svc, catalogPath, overridePath := newAdminPricingService(t)
	require.NoError(t, os.WriteFile(overridePath, []byte(`{"qwen3-max":{"input_cost_per_token":0.000003}}`), 0o644))
	require.NoError(t, svc.reloadCustomPricingLayers())
	before, err := os.ReadFile(overridePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(catalogPath, []byte(`{invalid`), 0o644))

	err = svc.UpsertPricingOverride("qwen3-max", map[string]any{"output_cost_per_token": 10e-6})
	require.Error(t, err)
	after, readErr := os.ReadFile(overridePath)
	require.NoError(t, readErr)
	require.JSONEq(t, string(before), string(after))
}
