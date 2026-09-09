package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type modelPriceSourceManifest struct {
	SchemaVersion int `json:"schema_version"`
	LedgerPolicy  struct {
		DomesticCurrency          string `json:"domestic_currency"`
		InternationalCurrency     string `json:"international_currency"`
		TokenPriceUnit            string `json:"token_price_unit"`
		ForeignExchangeConversion bool   `json:"foreign_exchange_conversion"`
	} `json:"ledger_policy"`
	Sources map[string]struct {
		Kind     string `json:"kind"`
		Currency string `json:"currency"`
		URL      string `json:"url"`
	} `json:"sources"`
	Models map[string]struct {
		Provider    string             `json:"provider"`
		Currency    string             `json:"currency"`
		BillingUnit string             `json:"billing_unit"`
		Status      string             `json:"status"`
		SourceIDs   []string           `json:"source_ids"`
		Inherits    string             `json:"inherits"`
		Notes       string             `json:"notes"`
		Prices      map[string]float64 `json:"prices"`
		Schedule    *struct {
			Timezone       string   `json:"timezone"`
			Weekdays       []string `json:"weekdays"`
			Weekends       string   `json:"weekends"`
			PeakMultiplier float64  `json:"peak_multiplier"`
		} `json:"schedule"`
	} `json:"models"`
}

func loadModelPriceSourceManifest(t *testing.T) modelPriceSourceManifest {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_price_sources.json"))
	require.NoError(t, err)
	var manifest modelPriceSourceManifest
	require.NoError(t, json.Unmarshal(body, &manifest))
	return manifest
}

func TestModelPriceSourcesManifestContract(t *testing.T) {
	manifest := loadModelPriceSourceManifest(t)
	require.Equal(t, 1, manifest.SchemaVersion)
	require.Equal(t, "CNY", manifest.LedgerPolicy.DomesticCurrency)
	require.Equal(t, "USD", manifest.LedgerPolicy.InternationalCurrency)
	require.Equal(t, "per_1m_tokens", manifest.LedgerPolicy.TokenPriceUnit)
	require.False(t, manifest.LedgerPolicy.ForeignExchangeConversion)

	allowedStatuses := map[string]bool{
		"verified":        true,
		"operator_policy": true,
		"provisional":     true,
	}
	for model, entry := range manifest.Models {
		require.True(t, allowedStatuses[entry.Status], "%s has unsupported status %q", model, entry.Status)
		require.NotEmpty(t, entry.SourceIDs, "%s must declare at least one source", model)
		for _, sourceID := range entry.SourceIDs {
			_, ok := manifest.Sources[sourceID]
			require.True(t, ok, "%s refers to unknown source %s", model, sourceID)
		}
		if entry.Inherits != "" {
			_, ok := manifest.Models[entry.Inherits]
			require.True(t, ok, "%s inherits unknown model %s", model, entry.Inherits)
			require.NotEmpty(t, entry.Notes, "%s alias must explain why it inherits", model)
			continue
		}
		require.Equal(t, "CNY", entry.Currency, model)
		require.Equal(t, "per_1m_tokens", entry.BillingUnit, model)
		require.NotEmpty(t, entry.Provider, model)
		require.NotEmpty(t, entry.Prices, model)
		for field, price := range entry.Prices {
			require.GreaterOrEqual(t, price, float64(0), "%s.%s must not be negative", model, field)
		}
		if entry.Status != "verified" {
			require.NotEmpty(t, entry.Notes, "%s non-official price must carry a warning", model)
		}
	}
}

func TestModelPriceSourcesCoverCurrentDomesticOverrides(t *testing.T) {
	manifest := loadModelPriceSourceManifest(t)
	models := []string{
		"MiniMax-M2.7-highspeed", "MiniMax-M3",
		"deepseek-v4-flash", "deepseek-v4-flash-0731", "deepseek-v4-pro", "deepseek-v4-pro-0813",
		"doubao-seed-2-1-pro", "doubao-seed-2-1-turbo",
		"glm-5.2", "glm-5.2-fast-preview", "glm-5.3",
		"kimi-k2.7-code", "kimi-k3",
		"qwen3.6-flash", "qwen3.7-flash", "qwen3.7-max", "qwen3.7-plus",
		"qwen3.8-2.4t-a95b", "qwen3.8-flash", "qwen3.8-max", "qwen3.8-max-0902",
	}
	for _, model := range models {
		_, ok := manifest.Models[model]
		require.True(t, ok, "current domestic override %s has no provenance", model)
	}
}

func TestDeepSeekSourceManifestMatchesRuntimePolicy(t *testing.T) {
	manifest := loadModelPriceSourceManifest(t)
	for model, want := range map[string]struct {
		input, output, cacheRead float64
	}{
		"deepseek-v4-flash": {1.5, 4.5, 0.025},
		"deepseek-v4-pro":   {4.5, 13.5, 0.075},
	} {
		entry := manifest.Models[model]
		require.Equal(t, "operator_policy", entry.Status)
		require.InDelta(t, want.input, entry.Prices["off_peak_input"], 1e-12)
		require.InDelta(t, want.output, entry.Prices["off_peak_output"], 1e-12)
		require.InDelta(t, want.cacheRead, entry.Prices["off_peak_cache_read"], 1e-12)
		require.NotNil(t, entry.Schedule)
		require.Equal(t, "Asia/Shanghai", entry.Schedule.Timezone)
		require.Equal(t, []string{"09:00-12:00", "14:00-18:00"}, entry.Schedule.Weekdays)
		require.Equal(t, "off_peak", entry.Schedule.Weekends)
		require.Equal(t, 2.0, entry.Schedule.PeakMultiplier)
	}
}

func TestManagedPricingBaselineContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "ttoken_model_pricing_overrides.json"))
	require.NoError(t, err)
	var entries map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &entries))
	require.Len(t, entries, 162)

	videoEntries := 0
	domesticEntries := 0
	domesticProviders := map[string]bool{
		"deepseek": true, "volcengine": true, "zhipu": true,
		"moonshot": true, "minimax": true, "dashscope": true,
	}
	for model, entry := range entries {
		require.NotEmpty(t, entry["litellm_provider"], model)
		require.NotEmpty(t, entry["mode"], model)
		if _, ok := entry["output_cost_per_video"]; ok {
			videoEntries++
		}
		if provider, ok := entry["litellm_provider"].(string); ok && domesticProviders[provider] {
			domesticEntries++
		}
	}
	require.Equal(t, 16, videoEntries)
	require.Equal(t, 21, domesticEntries)
}

func TestManagedPricingBaselinePrecedesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	managedPath := filepath.Join(dir, "managed.json")
	localPath := filepath.Join(dir, "local.json")
	require.NoError(t, os.WriteFile(managedPath, []byte(`{
		"test-model": {
			"litellm_provider": "dashscope",
			"mode": "chat",
			"input_cost_per_token": 0.000001,
			"output_cost_per_token": 0.000002
		}
	}`), 0600))
	require.NoError(t, os.WriteFile(localPath, []byte(`{
		"test-model": {"output_cost_per_token": 0.000009}
	}`), 0600))

	svc := &PricingService{cfg: &config.Config{Pricing: config.PricingConfig{
		ManagedOverrideFile: managedPath,
		OverrideFile:        localPath,
	}}}
	pricing, err := svc.parsePricingData([]byte(`{
		"test-model": {
			"litellm_provider": "original",
			"mode": "chat",
			"input_cost_per_token": 0.0000001,
			"output_cost_per_token": 0.0000002
		}
	}`))
	require.NoError(t, err)
	require.Equal(t, "dashscope", pricing["test-model"].LiteLLMProvider)
	require.InDelta(t, 0.000001, pricing["test-model"].InputCostPerToken, 1e-15)
	require.InDelta(t, 0.000009, pricing["test-model"].OutputCostPerToken, 1e-15)
}
